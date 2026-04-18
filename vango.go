package vango

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func clamp8(x int) uint8 {
	if x < 0 {
		return 0
	}
	if x > 255 {
		return 255
	}
	return uint8(x)
}
func clampF01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func idx(n *image.NRGBA, x, y int) int { return (y-n.Rect.Min.Y)*n.Stride + (x-n.Rect.Min.X)*4 }

// ToNRGBA converts any image.Image to tightly-packed *image.NRGBA
func ToNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok && n.Stride == n.Rect.Dx()*4 {
		out := image.NewNRGBA(n.Rect)
		copy(out.Pix, n.Pix)
		return out
	}
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst
}
func CloneNRGBA(src *image.NRGBA) *image.NRGBA {
	dst := image.NewNRGBA(src.Rect)
	copy(dst.Pix, src.Pix)
	return dst
}

// parallel rows helper (cancellable)
func parallelRows(ctx context.Context, h int, fn func(y int)) error {
	workers := runtime.GOMAXPROCS(0)
	if workers < 2 || h < 64 {
		for y := 0; y < h; y++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			fn(y)
		}
		return nil
	}
	var wg sync.WaitGroup
	rowCh := make(chan int, 64)
	errCh := make(chan error, 1)
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for y := range rowCh {
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				default:
				}
				fn(y)
			}
		}()
	}
	for y := 0; y < h; y++ {
		select {
		case <-ctx.Done():
			close(rowCh)
			wg.Wait()
			return ctx.Err()
		default:
		}
		rowCh <- y
	}
	close(rowCh)
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

var bufPool = sync.Pool{New: func() any { b := make([]float64, 0, 8192); return &b }}

func gaussianKernel(sigma float64, radius int) []float64 {
	if sigma <= 0 {
		return []float64{1}
	}
	if radius <= 0 {
		radius = int(math.Ceil(3 * sigma))
	}
	k := make([]float64, 2*radius+1)
	var sum float64
	sigma2 := 2 * sigma * sigma
	for i := -radius; i <= radius; i++ {
		v := math.Exp(-float64(i*i) / sigma2)
		k[i+radius] = v
		sum += v
	}
	for i := range k {
		k[i] /= sum
	}
	return k
}

func convolve1DHorizontal(ctx context.Context, src *image.NRGBA, k []float64) (*image.NRGBA, error) {
	r := src.Rect
	dst := image.NewNRGBA(r)
	radius := (len(k) - 1) / 2
	err := parallelRows(ctx, r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			var rr, gg, bb, aa float64
			for i := -radius; i <= radius; i++ {
				xx := x + i
				if xx < r.Min.X {
					xx = r.Min.X
				} else if xx >= r.Max.X {
					xx = r.Max.X - 1
				}
				p := idx(src, xx, y)
				w := k[i+radius]
				rr += float64(src.Pix[p+0]) * w
				gg += float64(src.Pix[p+1]) * w
				bb += float64(src.Pix[p+2]) * w
				aa += float64(src.Pix[p+3]) * w
			}
			q := idx(dst, x, y)
			dst.Pix[q+0] = clamp8(int(math.Round(rr)))
			dst.Pix[q+1] = clamp8(int(math.Round(gg)))
			dst.Pix[q+2] = clamp8(int(math.Round(bb)))
			dst.Pix[q+3] = clamp8(int(math.Round(aa)))
		}
	})
	return dst, err
}

func convolve1DVertical(ctx context.Context, src *image.NRGBA, k []float64) (*image.NRGBA, error) {
	r := src.Rect
	dst := image.NewNRGBA(r)
	radius := (len(k) - 1) / 2
	err := parallelRows(ctx, r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			var rr, gg, bb, aa float64
			for i := -radius; i <= radius; i++ {
				yy2 := y + i
				if yy2 < r.Min.Y {
					yy2 = r.Min.Y
				} else if yy2 >= r.Max.Y {
					yy2 = r.Max.Y - 1
				}
				p := idx(src, x, yy2)
				w := k[i+radius]
				rr += float64(src.Pix[p+0]) * w
				gg += float64(src.Pix[p+1]) * w
				bb += float64(src.Pix[p+2]) * w
				aa += float64(src.Pix[p+3]) * w
			}
			q := idx(dst, x, y)
			dst.Pix[q+0] = clamp8(int(math.Round(rr)))
			dst.Pix[q+1] = clamp8(int(math.Round(gg)))
			dst.Pix[q+2] = clamp8(int(math.Round(bb)))
			dst.Pix[q+3] = clamp8(int(math.Round(aa)))
		}
	})
	return dst, err
}

// GaussianBlur (context-aware). radius<=0 chooses ceil(3*sigma).
func GaussianBlur(src image.Image, sigma float64, radius int) *image.NRGBA {
	ctx := context.Background()
	return GaussianBlurCtx(ctx, src, sigma, radius)
}
func GaussianBlurCtx(ctx context.Context, src image.Image, sigma float64, radius int) *image.NRGBA {
	n := ToNRGBA(src)
	k := gaussianKernel(sigma, radius)
	tmp, _ := convolve1DHorizontal(ctx, n, k)
	out, _ := convolve1DVertical(ctx, tmp, k)
	return out
}

// UnsharpMask: out = src + amount*(src - blur(src))
func UnsharpMask(src image.Image, amount, sigma float64, radius int) *image.NRGBA {
	base := ToNRGBA(src)
	blur := GaussianBlur(base, sigma, radius)
	r := base.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(base, x, y)
			j := idx(blur, x, y)
			dr := float64(base.Pix[i+0]) + amount*(float64(base.Pix[i+0])-float64(blur.Pix[j+0]))
			dg := float64(base.Pix[i+1]) + amount*(float64(base.Pix[i+1])-float64(blur.Pix[j+1]))
			db := float64(base.Pix[i+2]) + amount*(float64(base.Pix[i+2])-float64(blur.Pix[j+2]))
			o := idx(out, x, y)
			out.Pix[o+0] = clamp8(int(math.Round(dr)))
			out.Pix[o+1] = clamp8(int(math.Round(dg)))
			out.Pix[o+2] = clamp8(int(math.Round(db)))
			out.Pix[o+3] = base.Pix[i+3]
		}
	})
	return out
}

// SobelEdges → *image.Gray gradient magnitude (0..255)
func SobelEdges(src image.Image) *image.Gray {
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewGray(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			var gx, gy float64
			for j := -1; j <= 1; j++ {
				yy2 := y + j
				if yy2 < r.Min.Y {
					yy2 = r.Min.Y
				} else if yy2 >= r.Max.Y {
					yy2 = r.Max.Y - 1
				}
				for i := -1; i <= 1; i++ {
					xx := x + i
					if xx < r.Min.X {
						xx = r.Min.X
					} else if xx >= r.Max.X {
						xx = r.Max.X - 1
					}
					p := idx(n, xx, yy2)
					l := 0.2126*float64(n.Pix[p+0]) + 0.7152*float64(n.Pix[p+1]) + 0.0722*float64(n.Pix[p+2])
					sx := []int{-1, 0, 1}
					sy := []int{1, 2, 1}
					if j == 0 {
						sy = []int{0, 0, 0}
						sx = []int{-2, 0, 2}
					} else if j == 1 {
						sy = []int{-1, 0, 1}
						sx = []int{-1, 0, 1}
					}
					gx += float64(sx[i+1]) * l
					gy += float64(sy[i+1]) * l
				}
			}
			mag := math.Sqrt(gx*gx + gy*gy)
			if mag > 255 {
				mag = 255
			}
			out.SetGray(x, y, color.Gray{Y: uint8(mag + 0.5)})
		}
	})
	return out
}

//
// ------------------------------ Color transforms -----------------------------
//

func rgbToHSL(r, g, b float64) (h, s, l float64) {
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	l = (max + min) / 2
	if max == min {
		return 0, 0, l
	}
	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	case b:
		h = (r-g)/d + 4
	}
	h /= 6
	return
}
func hue2rgb(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}
func hslToRGB(h, s, l float64) (r, g, b float64) {
	if s == 0 {
		return l, l, l
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	r = hue2rgb(p, q, h+1.0/3.0)
	g = hue2rgb(p, q, h)
	b = hue2rgb(p, q, h-1.0/3.0)
	return
}

func AdjustBrightness(src image.Image, delta float64) *image.NRGBA {
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			rf := float64(n.Pix[i+0]) / 255
			gf := float64(n.Pix[i+1]) / 255
			bf := float64(n.Pix[i+2]) / 255
			h, s, l := rgbToHSL(rf, gf, bf)
			l = clampF01(l + delta)
			rr, gg, bb := hslToRGB(h, s, l)
			out.Pix[i+0] = clamp8(int(math.Round(rr * 255)))
			out.Pix[i+1] = clamp8(int(math.Round(gg * 255)))
			out.Pix[i+2] = clamp8(int(math.Round(bb * 255)))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}
func AdjustContrast(src image.Image, factor float64) *image.NRGBA {
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			for c := 0; c < 3; c++ {
				v := (float64(n.Pix[i+c])/255 - 0.5) * factor
				out.Pix[i+c] = clamp8(int(math.Round((v + 0.5) * 255)))
			}
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}
func AdjustSaturation(src image.Image, factor float64) *image.NRGBA {
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			rf := float64(n.Pix[i+0]) / 255
			gf := float64(n.Pix[i+1]) / 255
			bf := float64(n.Pix[i+2]) / 255
			h, s, l := rgbToHSL(rf, gf, bf)
			s = clampF01(s * factor)
			rr, gg, bb := hslToRGB(h, s, l)
			out.Pix[i+0] = clamp8(int(math.Round(rr * 255)))
			out.Pix[i+1] = clamp8(int(math.Round(gg * 255)))
			out.Pix[i+2] = clamp8(int(math.Round(bb * 255)))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}
func AdjustHue(src image.Image, degrees float64) *image.NRGBA {
	shift := ((degrees / 360.0) - math.Floor(degrees/360.0))
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			rf := float64(n.Pix[i+0]) / 255
			gf := float64(n.Pix[i+1]) / 255
			bf := float64(n.Pix[i+2]) / 255
			h, s, l := rgbToHSL(rf, gf, bf)
			h = math.Mod(h+shift+1, 1)
			rr, gg, bb := hslToRGB(h, s, l)
			out.Pix[i+0] = clamp8(int(math.Round(rr * 255)))
			out.Pix[i+1] = clamp8(int(math.Round(gg * 255)))
			out.Pix[i+2] = clamp8(int(math.Round(bb * 255)))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

func Invert(src image.Image) *image.NRGBA {
	n := ToNRGBA(src)
	out := CloneNRGBA(n)
	for i := 0; i < len(out.Pix); i += 4 {
		out.Pix[i+0] = 255 - out.Pix[i+0]
		out.Pix[i+1] = 255 - out.Pix[i+1]
		out.Pix[i+2] = 255 - out.Pix[i+2]
	}
	return out
}

func Sepia(src image.Image, amount float64) *image.NRGBA {
	amount = clampF01(amount)
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			rf := float64(n.Pix[i+0])
			gf := float64(n.Pix[i+1])
			bf := float64(n.Pix[i+2])
			tr := 0.393*rf + 0.769*gf + 0.189*bf
			tg := 0.349*rf + 0.686*gf + 0.168*bf
			tb := 0.272*rf + 0.534*gf + 0.131*bf
			rr := (1-amount)*rf + amount*tr
			gg := (1-amount)*gf + amount*tg
			bb := (1-amount)*bf + amount*tb
			out.Pix[i+0] = clamp8(int(rr + 0.5))
			out.Pix[i+1] = clamp8(int(gg + 0.5))
			out.Pix[i+2] = clamp8(int(bb + 0.5))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}
func Vintage(src image.Image) *image.NRGBA {
	o := AdjustContrast(src, 0.9)
	o = AdjustSaturation(o, 0.85)
	o = Sepia(o, 0.25)
	return o
}

func Pixelate(src image.Image, block int) *image.NRGBA {
	if block <= 1 {
		return ToNRGBA(src)
	}
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	for by := r.Min.Y; by < r.Max.Y; by += block {
		for bx := r.Min.X; bx < r.Max.X; bx += block {
			var rs, gs, bs, as, c int
			for y := by; y < by+block && y < r.Max.Y; y++ {
				for x := bx; x < bx+block && x < r.Max.X; x++ {
					i := idx(n, x, y)
					rs += int(n.Pix[i+0])
					gs += int(n.Pix[i+1])
					bs += int(n.Pix[i+2])
					as += int(n.Pix[i+3])
					c++
				}
			}
			if c == 0 {
				continue
			}
			cr, cg, cb, ca := uint8(rs/c), uint8(gs/c), uint8(bs/c), uint8(as/c)
			for y := by; y < by+block && y < r.Max.Y; y++ {
				for x := bx; x < bx+block && x < r.Max.X; x++ {
					i := idx(out, x, y)
					out.Pix[i+0] = cr
					out.Pix[i+1] = cg
					out.Pix[i+2] = cb
					out.Pix[i+3] = ca
				}
			}
		}
	}
	return out
}

func Posterize(src image.Image, levels int) *image.NRGBA {
	if levels < 2 {
		levels = 2
	}
	n := ToNRGBA(src)
	out := CloneNRGBA(n)
	step := 255.0 / float64(levels-1)
	for i := 0; i < len(out.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			v := float64(out.Pix[i+c])
			q := math.Round(v/step) * step
			out.Pix[i+c] = uint8(q)
		}
	}
	return out
}

func Threshold(src image.Image, cutoff uint8) *image.Gray {
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewGray(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			l := 0.2126*float64(n.Pix[i+0]) + 0.7152*float64(n.Pix[i+1]) + 0.0722*float64(n.Pix[i+2])
			if l >= float64(cutoff) {
				out.SetGray(x, y, color.Gray{255})
			} else {
				out.SetGray(x, y, color.Gray{0})
			}
		}
	})
	return out
}

//
// ------------------------------ White balance -------------------------------
//

// WhiteBalanceByRect: average color from ref rect (or whole image if empty)
func WhiteBalanceByRect(src image.Image, ref image.Rectangle) *image.NRGBA {
	n := ToNRGBA(src)
	r := n.Rect
	if ref.Empty() {
		ref = r
	} else {
		ref = ref.Intersect(r)
	}
	if ref.Empty() {
		return CloneNRGBA(n)
	}
	var rs, gs, bs, cnt int
	for y := ref.Min.Y; y < ref.Max.Y; y++ {
		for x := ref.Min.X; x < ref.Max.X; x++ {
			i := idx(n, x, y)
			if n.Pix[i+3] == 0 {
				continue
			}
			rs += int(n.Pix[i+0])
			gs += int(n.Pix[i+1])
			bs += int(n.Pix[i+2])
			cnt++
		}
	}
	if cnt == 0 {
		return CloneNRGBA(n)
	}
	avgR := float64(rs) / float64(cnt)
	avgG := float64(gs) / float64(cnt)
	avgB := float64(bs) / float64(cnt)
	if avgR <= 0 || avgG <= 0 || avgB <= 0 {
		return CloneNRGBA(n)
	}
	target := (avgR + avgG + avgB) / 3
	gainR := target / avgR
	gainG := target / avgG
	gainB := target / avgB

	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			out.Pix[i+0] = clamp8(int(math.Round(float64(n.Pix[i+0]) * gainR)))
			out.Pix[i+1] = clamp8(int(math.Round(float64(n.Pix[i+1]) * gainG)))
			out.Pix[i+2] = clamp8(int(math.Round(float64(n.Pix[i+2]) * gainB)))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

//
// ------------------------------ Noise reduction -----------------------------
//

// NoiseReduction applies a median filter with a square kernel of size (2*radius+1).
// radius=1 gives a 3×3 window; radius=2 gives 5×5, etc.
func NoiseReduction(src image.Image, radius int) *image.NRGBA {
	if radius < 1 {
		radius = 1
	}
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	windowSize := (2*radius + 1) * (2*radius + 1)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		bufR := make([]uint8, windowSize)
		bufG := make([]uint8, windowSize)
		bufB := make([]uint8, windowSize)
		for x := r.Min.X; x < r.Max.X; x++ {
			k := 0
			for j := -radius; j <= radius; j++ {
				yy2 := y + j
				if yy2 < r.Min.Y {
					yy2 = r.Min.Y
				} else if yy2 >= r.Max.Y {
					yy2 = r.Max.Y - 1
				}
				for ii := -radius; ii <= radius; ii++ {
					xx := x + ii
					if xx < r.Min.X {
						xx = r.Min.X
					} else if xx >= r.Max.X {
						xx = r.Max.X - 1
					}
					p := idx(n, xx, yy2)
					bufR[k] = n.Pix[p+0]
					bufG[k] = n.Pix[p+1]
					bufB[k] = n.Pix[p+2]
					k++
				}
			}
			sort.Slice(bufR[:k], func(a, b int) bool { return bufR[a] < bufR[b] })
			sort.Slice(bufG[:k], func(a, b int) bool { return bufG[a] < bufG[b] })
			sort.Slice(bufB[:k], func(a, b int) bool { return bufB[a] < bufB[b] })
			mid := k / 2
			q := idx(out, x, y)
			out.Pix[q+0] = bufR[mid]
			out.Pix[q+1] = bufG[mid]
			out.Pix[q+2] = bufB[mid]
			out.Pix[q+3] = n.Pix[idx(n, x, y)+3]
		}
	})
	return out
}

//
// ------------------------------ Geometry ------------------------------------
//

func sampleNearest(n *image.NRGBA, x, y int, bg color.NRGBA, b image.Rectangle) (r, g, bb, a uint8) {
	if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
		return bg.R, bg.G, bg.B, bg.A
	}
	i := idx(n, x, y)
	return n.Pix[i+0], n.Pix[i+1], n.Pix[i+2], n.Pix[i+3]
}
func sampleBilinear(n *image.NRGBA, x, y float64, bg color.NRGBA, b image.Rectangle) (r, g, bb, a uint8) {
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	tx := x - float64(x0)
	ty := y - float64(y0)
	r00, g00, b00, a00 := sampleNearest(n, x0, y0, bg, b)
	r10, g10, b10, a10 := sampleNearest(n, x0+1, y0, bg, b)
	r01, g01, b01, a01 := sampleNearest(n, x0, y0+1, bg, b)
	r11, g11, b11, a11 := sampleNearest(n, x0+1, y0+1, bg, b)
	rf := lerp(lerp(float64(r00), float64(r10), tx), lerp(float64(r01), float64(r11), tx), ty)
	gf := lerp(lerp(float64(g00), float64(g10), tx), lerp(float64(g01), float64(g11), tx), ty)
	bf := lerp(lerp(float64(b00), float64(b10), tx), lerp(float64(b01), float64(b11), tx), ty)
	af := lerp(lerp(float64(a00), float64(a10), tx), lerp(float64(a01), float64(a11), tx), ty)
	return clamp8(int(math.Round(rf))), clamp8(int(math.Round(gf))), clamp8(int(math.Round(bf))), clamp8(int(math.Round(af)))
}

// Affine: inverse mapping; mat [a b c; d e f]
func Affine(src image.Image, mat [6]float64, w, h int, method string, bg color.NRGBA) *image.NRGBA {
	n := ToNRGBA(src)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	a, b, c, d, e, f := mat[0], mat[1], mat[2], mat[3], mat[4], mat[5]
	den := a*e - b*d
	if den == 0 {
		draw.Draw(dst, dst.Rect, n, n.Rect.Min, draw.Src)
		return dst
	}
	ia := e / den
	ib := -b / den
	id := -d / den
	ie := a / den
	ic := (b*f - e*c) / den
	if_ := (d*c - a*f) / den

	bounds := n.Rect
	switch strings.ToLower(method) {
	case "bilinear", "linear":
		_ = parallelRows(context.Background(), h, func(yy int) {
			y := float64(yy)
			for x := 0; x < w; x++ {
				fx := float64(x)
				sx := ia*fx + ib*y + ic
				sy := id*fx + ie*y + if_
				r, g, b2, a2 := sampleBilinear(n, sx, sy, bg, bounds)
				i := yy*dst.Stride + x*4
				dst.Pix[i+0] = r
				dst.Pix[i+1] = g
				dst.Pix[i+2] = b2
				dst.Pix[i+3] = a2
			}
		})
	default:
		_ = parallelRows(context.Background(), h, func(yy int) {
			y := float64(yy)
			for x := 0; x < w; x++ {
				fx := float64(x)
				sx := ia*fx + ib*y + ic
				sy := id*fx + ie*y + if_
				r, g, b2, a2 := sampleNearest(n, int(math.Round(sx)), int(math.Round(sy)), bg, bounds)
				i := yy*dst.Stride + x*4
				dst.Pix[i+0] = r
				dst.Pix[i+1] = g
				dst.Pix[i+2] = b2
				dst.Pix[i+3] = a2
			}
		})
	}
	return dst
}

// Rotate: automatic bounds, center rotation
func Rotate(src image.Image, degrees float64, method string, bg color.NRGBA) *image.NRGBA {
	r := src.Bounds()
	cx := float64(r.Dx()) / 2
	cy := float64(r.Dy()) / 2
	theta := degrees * math.Pi / 180
	ct := math.Cos(theta)
	st := math.Sin(theta)
	corners := [][2]float64{
		{0 - cx, 0 - cy},
		{float64(r.Dx()) - cx, 0 - cy},
		{0 - cx, float64(r.Dy()) - cy},
		{float64(r.Dx()) - cx, float64(r.Dy()) - cy},
	}
	var minx, miny, maxx, maxy float64
	for i, c := range corners {
		x := c[0]*ct - c[1]*st
		y := c[0]*st + c[1]*ct
		if i == 0 {
			minx, maxx, miny, maxy = x, x, y, y
		} else {
			if x < minx {
				minx = x
			}
			if x > maxx {
				maxx = x
			}
			if y < miny {
				miny = y
			}
			if y > maxy {
				maxy = y
			}
		}
	}
	w := int(math.Ceil(maxx - minx))
	h := int(math.Ceil(maxy - miny))
	offx := -minx
	offy := -miny
	mat := [6]float64{
		ct, -st, cx - ct*cx + st*cy + offx,
		st, ct, cy - st*cx - ct*cy + offy,
	}
	return Affine(src, mat, w, h, method, bg)
}

func FlipX(src image.Image) *image.NRGBA {
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, r.Min.X+(r.Max.X-1-x), y)
			o := idx(out, x, y)
			copy(out.Pix[o:o+4], n.Pix[i:i+4])
		}
	})
	return out
}
func FlipY(src image.Image) *image.NRGBA {
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, r.Min.Y+(r.Max.Y-1-y))
			o := idx(out, x, y)
			copy(out.Pix[o:o+4], n.Pix[i:i+4])
		}
	})
	return out
}

// Skew by shears
func Skew(src image.Image, sx, sy float64, method string, bg color.NRGBA) *image.NRGBA {
	r := src.Bounds()
	w := r.Dx() + int(math.Ceil(math.Abs(sx)*float64(r.Dy())))
	h := r.Dy() + int(math.Ceil(math.Abs(sy)*float64(r.Dx())))
	mat := [6]float64{1, sx, 0, sy, 1, 0}
	return Affine(src, mat, w, h, method, bg)
}

func ResizeNearest(src image.Image, w, h int) *image.NRGBA {
	n := ToNRGBA(src)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	sx := float64(n.Rect.Dx()) / float64(w)
	sy := float64(n.Rect.Dy()) / float64(h)
	bg := color.NRGBA{0, 0, 0, 0}
	_ = parallelRows(context.Background(), h, func(yy int) {
		y := yy
		for x := 0; x < w; x++ {
			ix := int(math.Floor(float64(x)*sx)) + n.Rect.Min.X
			iy := int(math.Floor(float64(y)*sy)) + n.Rect.Min.Y
			r, g, b, a := sampleNearest(n, ix, iy, bg, n.Rect)
			i := y*dst.Stride + x*4
			dst.Pix[i+0] = r
			dst.Pix[i+1] = g
			dst.Pix[i+2] = b
			dst.Pix[i+3] = a
		}
	})
	return dst
}
func ResizeBilinear(src image.Image, w, h int) *image.NRGBA {
	n := ToNRGBA(src)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	sx := float64(n.Rect.Dx()) / float64(w)
	sy := float64(n.Rect.Dy()) / float64(h)
	bg := color.NRGBA{0, 0, 0, 0}
	_ = parallelRows(context.Background(), h, func(yy int) {
		y := yy
		for x := 0; x < w; x++ {
			fx := float64(x)*sx + float64(n.Rect.Min.X)
			fy := float64(y)*sy + float64(n.Rect.Min.Y)
			r, g, b, a := sampleBilinear(n, fx, fy, bg, n.Rect)
			i := y*dst.Stride + x*4
			dst.Pix[i+0] = r
			dst.Pix[i+1] = g
			dst.Pix[i+2] = b
			dst.Pix[i+3] = a
		}
	})
	return dst
}

func Crop(src image.Image, rect image.Rectangle) *image.NRGBA {
	n := ToNRGBA(src)
	rect = rect.Intersect(n.Rect)
	if rect.Empty() {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	dst := image.NewNRGBA(rect.Sub(rect.Min))
	draw.Draw(dst, dst.Rect, n, rect.Min, draw.Src)
	return dst
}

func TrimByColor(src image.Image, col color.NRGBA, tol uint8) *image.NRGBA {
	n := ToNRGBA(src)
	r := n.Rect
	isMatch := func(i int) bool {
		dr := int(n.Pix[i+0]) - int(col.R)
		dg := int(n.Pix[i+1]) - int(col.G)
		db := int(n.Pix[i+2]) - int(col.B)
		da := int(n.Pix[i+3]) - int(col.A)
		if dr < 0 {
			dr = -dr
		}
		if dg < 0 {
			dg = -dg
		}
		if db < 0 {
			db = -db
		}
		if da < 0 {
			da = -da
		}
		t := int(tol)
		return dr <= t && dg <= t && db <= t && da <= t
	}
	minX, minY, maxX, maxY := r.Max.X, r.Max.Y, r.Min.X, r.Min.Y
	for y := r.Min.Y; y < r.Max.Y; y++ {
		all := true
		for x := r.Min.X; x < r.Max.X; x++ {
			if !isMatch(idx(n, x, y)) {
				all = false
				break
			}
		}
		if all {
			continue
		}
		minY = y
		break
	}
	for y := r.Max.Y - 1; y >= r.Min.Y; y-- {
		all := true
		for x := r.Min.X; x < r.Max.X; x++ {
			if !isMatch(idx(n, x, y)) {
				all = false
				break
			}
		}
		if all {
			continue
		}
		maxY = y + 1
		break
	}
	for x := r.Min.X; x < r.Max.X; x++ {
		all := true
		for y := minY; y < maxY; y++ {
			if !isMatch(idx(n, x, y)) {
				all = false
				break
			}
		}
		if all {
			continue
		}
		minX = x
		break
	}
	for x := r.Max.X - 1; x >= r.Min.X; x-- {
		all := true
		for y := minY; y < maxY; y++ {
			if !isMatch(idx(n, x, y)) {
				all = false
				break
			}
		}
		if all {
			continue
		}
		maxX = x + 1
		break
	}
	if minX >= maxX || minY >= maxY {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	return Crop(n, image.Rect(minX, minY, maxX, maxY))
}

//
// ------------------------------ Image alignment / collage -------------------
//

// AlignImages tiles images side by side (horizontal) or stacked (vertical).
// direction: "horizontal"/"h" (or "left"/"right") – images in a row matched by height.
// direction: "vertical"/"v" (or "top"/"bottom") – images in a column matched by width.
// All images are scaled to share the same height (horizontal) or same width (vertical).
// bg fills the canvas background.
func AlignImages(imgs []image.Image, direction string, bg color.NRGBA) *image.NRGBA {
	if len(imgs) == 0 {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	dir := strings.ToLower(direction)
	isHoriz := dir == "" || dir == "h" || dir == "horizontal" || dir == "left" || dir == "right"

	if isHoriz {
		maxH := 0
		for _, im := range imgs {
			if im.Bounds().Dy() > maxH {
				maxH = im.Bounds().Dy()
			}
		}
		if maxH == 0 {
			return image.NewNRGBA(image.Rect(0, 0, 0, 0))
		}
		nrgbas := make([]*image.NRGBA, len(imgs))
		totalW := 0
		for i, im := range imgs {
			h := im.Bounds().Dy()
			w := im.Bounds().Dx()
			if h == 0 || w == 0 {
				nrgbas[i] = image.NewNRGBA(image.Rect(0, 0, 0, maxH))
				continue
			}
			newW := int(math.Round(float64(w) * float64(maxH) / float64(h)))
			if newW < 1 {
				newW = 1
			}
			nrgbas[i] = ResizeBilinear(im, newW, maxH)
			totalW += newW
		}
		out := image.NewNRGBA(image.Rect(0, 0, totalW, maxH))
		for j := 0; j < len(out.Pix); j += 4 {
			out.Pix[j+0] = bg.R
			out.Pix[j+1] = bg.G
			out.Pix[j+2] = bg.B
			out.Pix[j+3] = bg.A
		}
		xOff := 0
		for _, nn := range nrgbas {
			if nn.Rect.Dx() == 0 {
				continue
			}
			draw.Draw(out, image.Rect(xOff, 0, xOff+nn.Rect.Dx(), maxH), nn, nn.Rect.Min, draw.Over)
			xOff += nn.Rect.Dx()
		}
		return out
	}

	// Vertical: scale to same width
	maxW := 0
	for _, im := range imgs {
		if im.Bounds().Dx() > maxW {
			maxW = im.Bounds().Dx()
		}
	}
	if maxW == 0 {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	nrgbas := make([]*image.NRGBA, len(imgs))
	totalH := 0
	for i, im := range imgs {
		w := im.Bounds().Dx()
		h := im.Bounds().Dy()
		if w == 0 || h == 0 {
			nrgbas[i] = image.NewNRGBA(image.Rect(0, 0, maxW, 0))
			continue
		}
		newH := int(math.Round(float64(h) * float64(maxW) / float64(w)))
		if newH < 1 {
			newH = 1
		}
		nrgbas[i] = ResizeBilinear(im, maxW, newH)
		totalH += newH
	}
	out := image.NewNRGBA(image.Rect(0, 0, maxW, totalH))
	for j := 0; j < len(out.Pix); j += 4 {
		out.Pix[j+0] = bg.R
		out.Pix[j+1] = bg.G
		out.Pix[j+2] = bg.B
		out.Pix[j+3] = bg.A
	}
	yOff := 0
	for _, nn := range nrgbas {
		if nn.Rect.Dy() == 0 {
			continue
		}
		draw.Draw(out, image.Rect(0, yOff, maxW, yOff+nn.Rect.Dy()), nn, nn.Rect.Min, draw.Over)
		yOff += nn.Rect.Dy()
	}
	return out
}

//
// ------------------------------ Watermark -----------------------------------
//

func WatermarkImage(base image.Image, mark image.Image, pos image.Point, opacity float64) *image.NRGBA {
	opacity = clampF01(opacity)
	dst := ToNRGBA(base)
	m := ToNRGBA(mark)
	if opacity < 1 {
		for i := 0; i < len(m.Pix); i += 4 {
			a := float64(m.Pix[i+3]) * opacity
			if a > 255 {
				a = 255
			}
			m.Pix[i+3] = uint8(a + 0.5)
		}
	}
	off := image.Rectangle{Min: pos, Max: pos.Add(m.Rect.Size())}
	draw.Draw(dst, off, m, m.Rect.Min, draw.Over)
	return dst
}

//
// ------------------------------ LUTs ----------------------------------------
//

type LUT1D struct{ R, G, B [256]uint8 }

func ApplyLUT1D(src image.Image, lut LUT1D) *image.NRGBA {
	n := ToNRGBA(src)
	out := CloneNRGBA(n)
	for i := 0; i < len(out.Pix); i += 4 {
		out.Pix[i+0] = lut.R[out.Pix[i+0]]
		out.Pix[i+1] = lut.G[out.Pix[i+1]]
		out.Pix[i+2] = lut.B[out.Pix[i+2]]
	}
	return out
}

type LUT3D struct {
	Size      int
	DomainMin [3]float64
	DomainMax [3]float64
	Data      []float64 // len = N^3 * 3
}

func ParseCube(r io.Reader) (*LUT3D, error) {
	sc := bufio.NewScanner(r)
	lut := &LUT3D{DomainMin: [3]float64{0, 0, 0}, DomainMax: [3]float64{1, 1, 1}}
	var values []float64
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		switch strings.ToUpper(parts[0]) {
		case "TITLE":
		case "DOMAIN_MIN":
			if len(parts) >= 4 {
				for i := 0; i < 3; i++ {
					v, _ := strconv.ParseFloat(parts[i+1], 64)
					lut.DomainMin[i] = v
				}
			}
		case "DOMAIN_MAX":
			if len(parts) >= 4 {
				for i := 0; i < 3; i++ {
					v, _ := strconv.ParseFloat(parts[i+1], 64)
					lut.DomainMax[i] = v
				}
			}
		case "LUT_3D_SIZE":
			if len(parts) >= 2 {
				s, err := strconv.Atoi(parts[1])
				if err != nil || s <= 1 {
					return nil, errors.New("invalid LUT_3D_SIZE")
				}
				lut.Size = s
			}
		default:
			if len(parts) >= 3 {
				for i := 0; i < 3; i++ {
					v, _ := strconv.ParseFloat(parts[i], 64)
					values = append(values, v)
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if lut.Size == 0 {
		return nil, errors.New("missing LUT_3D_SIZE")
	}
	if len(values) != lut.Size*lut.Size*lut.Size*3 {
		return nil, errors.New("unexpected 3D LUT size")
	}
	lut.Data = values
	return lut, nil
}

func trilerp(c000, c100, c010, c110, c001, c101, c011, c111, dx, dy, dz float64) float64 {
	c00 := lerp(c000, c100, dx)
	c10 := lerp(c010, c110, dx)
	c01 := lerp(c001, c101, dx)
	c11 := lerp(c011, c111, dx)
	c0 := lerp(c00, c10, dy)
	c1 := lerp(c01, c11, dy)
	return lerp(c0, c1, dz)
}
func lutAt(l *LUT3D, ri, gi, bi int) [3]float64 {
	N := l.Size
	i := ((bi*N+gi)*N + ri) * 3
	return [3]float64{l.Data[i+0], l.Data[i+1], l.Data[i+2]}
}
func ApplyLUT3D(src image.Image, lut *LUT3D) *image.NRGBA {
	if lut == nil || lut.Size <= 1 {
		return ToNRGBA(src)
	}
	n := ToNRGBA(src)
	out := image.NewNRGBA(n.Rect)
	s := float64(lut.Size - 1)
	_ = parallelRows(context.Background(), n.Rect.Dy(), func(yy int) {
		y := n.Rect.Min.Y + yy
		for x := n.Rect.Min.X; x < n.Rect.Max.X; x++ {
			i := idx(n, x, y)
			r := float64(n.Pix[i+0]) / 255
			g := float64(n.Pix[i+1]) / 255
			b := float64(n.Pix[i+2]) / 255
			r = (r - lut.DomainMin[0]) / (lut.DomainMax[0] - lut.DomainMin[0])
			g = (g - lut.DomainMin[1]) / (lut.DomainMax[1] - lut.DomainMin[1])
			b = (b - lut.DomainMin[2]) / (lut.DomainMax[2] - lut.DomainMin[2])
			r = clampF01(r)
			g = clampF01(g)
			b = clampF01(b)
			fr := r * s
			fg := g * s
			fb := b * s
			r0 := int(math.Floor(fr))
			g0 := int(math.Floor(fg))
			b0 := int(math.Floor(fb))
			dr := fr - float64(r0)
			dg := fg - float64(g0)
			db := fb - float64(b0)
			r1 := minInt(r0+1, lut.Size-1)
			g1 := minInt(g0+1, lut.Size-1)
			b1 := minInt(b0+1, lut.Size-1)

			c000 := lutAt(lut, r0, g0, b0)
			c100 := lutAt(lut, r1, g0, b0)
			c010 := lutAt(lut, r0, g1, b0)
			c110 := lutAt(lut, r1, g1, b0)
			c001 := lutAt(lut, r0, g0, b1)
			c101 := lutAt(lut, r1, g0, b1)
			c011 := lutAt(lut, r0, g1, b1)
			c111 := lutAt(lut, r1, g1, b1)

			rc := trilerp(c000[0], c100[0], c010[0], c110[0], c001[0], c101[0], c011[0], c111[0], dr, dg, db)
			gc := trilerp(c000[1], c100[1], c010[1], c110[1], c001[1], c101[1], c011[1], c111[1], dr, dg, db)
			bc := trilerp(c000[2], c100[2], c010[2], c110[2], c001[2], c101[2], c011[2], c111[2], dr, dg, db)

			out.Pix[i+0] = clamp8(int(math.Round(rc * 255)))
			out.Pix[i+1] = clamp8(int(math.Round(gc * 255)))
			out.Pix[i+2] = clamp8(int(math.Round(bc * 255)))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

//
// ------------------------------ Analysis ------------------------------------
//

type Analysis struct {
	Width, Height int
	Aspect        float64
	Average       color.NRGBA
	Histogram     [256]uint32
	Brightest     struct {
		Point image.Point
		Value uint8
	}
	Darkest struct {
		Point image.Point
		Value uint8
	}
	Palette []color.NRGBA
	EXIF    map[string]string
}

func Analyze(src image.Image, paletteK int) Analysis {
	n := ToNRGBA(src)
	r := n.Rect
	var a Analysis
	a.Width, a.Height = r.Dx(), r.Dy()
	if a.Height > 0 {
		a.Aspect = float64(a.Width) / float64(a.Height)
	}
	var rs, gs, bs, as uint64
	brightV, darkV := uint8(0), uint8(255)
	brightP, darkP := image.Point{}, image.Point{}
	var bins [4][4][4]struct {
		sumR, sumG, sumB, sumA uint64
		count                  uint64
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			R, G, B, A := n.Pix[i+0], n.Pix[i+1], n.Pix[i+2], n.Pix[i+3]
			rs += uint64(R)
			gs += uint64(G)
			bs += uint64(B)
			as += uint64(A)
			l := uint8(0.2126*float64(R) + 0.7152*float64(G) + 0.0722*float64(B) + 0.5)
			a.Histogram[l]++
			if l > brightV {
				brightV = l
				brightP = image.Point{x, y}
			}
			if l < darkV {
				darkV = l
				darkP = image.Point{x, y}
			}
			ri := int(R) >> 6
			gi := int(G) >> 6
			bi := int(B) >> 6
			bin := &bins[ri][gi][bi]
			bin.sumR += uint64(R)
			bin.sumG += uint64(G)
			bin.sumB += uint64(B)
			bin.sumA += uint64(A)
			bin.count++
		}
	}
	pix := uint64(a.Width * a.Height)
	if pix > 0 {
		a.Average = color.NRGBA{uint8(rs / pix), uint8(gs / pix), uint8(bs / pix), uint8(as / pix)}
	}
	a.Brightest.Point, a.Brightest.Value = brightP, brightV
	a.Darkest.Point, a.Darkest.Value = darkP, darkV

	type pe struct {
		c     color.NRGBA
		count uint64
	}
	var entries []pe
	for r1 := 0; r1 < 4; r1++ {
		for g1 := 0; g1 < 4; g1++ {
			for b1 := 0; b1 < 4; b1++ {
				bin := bins[r1][g1][b1]
				if bin.count == 0 {
					continue
				}
				entries = append(entries, pe{
					c:     color.NRGBA{uint8(bin.sumR / bin.count), uint8(bin.sumG / bin.count), uint8(bin.sumB / bin.count), 255},
					count: bin.count,
				})
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].count > entries[j].count })
	if paletteK <= 0 {
		paletteK = 5
	}
	if paletteK > len(entries) {
		paletteK = len(entries)
	}
	a.Palette = make([]color.NRGBA, paletteK)
	for i := 0; i < paletteK; i++ {
		a.Palette[i] = entries[i].c
	}
	return a
}

//
// ------------------------------ Minimal EXIF (JPEG) --------------------------
//

func ReadJPEGEXIF(r io.Reader) (map[string]string, error) {
	br := bufio.NewReader(r)
	m0, _ := br.ReadByte()
	m1, _ := br.ReadByte()
	if m0 != 0xFF || m1 != 0xD8 {
		return nil, errors.New("not a JPEG")
	}
	for {
		b, err := br.ReadByte()
		if err != nil {
			return nil, errors.New("no EXIF found")
		}
		if b != 0xFF {
			continue
		}
		m, err := br.ReadByte()
		if err != nil {
			return nil, errors.New("no EXIF found")
		}
		if m == 0xDA {
			return nil, errors.New("no EXIF before SOS")
		}
		if m == 0xD9 {
			return nil, errors.New("no EXIF found")
		}
		lengthBytes := make([]byte, 2)
		if _, err := io.ReadFull(br, lengthBytes); err != nil {
			return nil, err
		}
		segLen := int(binary.BigEndian.Uint16(lengthBytes))
		if segLen < 2 {
			return nil, errors.New("bad seg")
		}
		dataLen := segLen - 2
		data := make([]byte, dataLen)
		if _, err := io.ReadFull(br, data); err != nil {
			return nil, err
		}
		if m == 0xE1 && len(data) > 6 && string(data[:6]) == "Exif\x00\x00" {
			return parseTIFFEXIF(data[6:])
		}
	}
}
func parseTIFFEXIF(tiff []byte) (map[string]string, error) {
	if len(tiff) < 8 {
		return nil, errors.New("short TIFF")
	}
	var order binary.ByteOrder
	if string(tiff[:2]) == "II" {
		order = binary.LittleEndian
	} else if string(tiff[:2]) == "MM" {
		order = binary.BigEndian
	} else {
		return nil, errors.New("bad order")
	}
	if order.Uint16(tiff[2:4]) != 0x2A {
		return nil, errors.New("bad magic")
	}
	ifd0 := int(order.Uint32(tiff[4:8]))
	if ifd0 <= 0 || ifd0 >= len(tiff) {
		return nil, errors.New("bad IFD0")
	}
	tags := make(map[uint16][]byte)
	var readIFD func(off int)
	readIFD = func(off int) {
		if off <= 0 || off+2 > len(tiff) {
			return
		}
		n := int(order.Uint16(tiff[off : off+2]))
		p := off + 2
		for i := 0; i < n; i++ {
			if p+12 > len(tiff) {
				break
			}
			tag := order.Uint16(tiff[p : p+2])
			typ := order.Uint16(tiff[p+2 : p+4])
			cou := order.Uint32(tiff[p+4 : p+8])
			val := tiff[p+8 : p+12]
			size := typeSize(typ) * int(cou)
			var data []byte
			if size <= 4 {
				data = make([]byte, size)
				copy(data, val[:size])
			} else {
				off := int(order.Uint32(val))
				if off >= 0 && off+size <= len(tiff) {
					data = tiff[off : off+size]
				}
			}
			if data != nil {
				tags[tag] = data
			}
			p += 12
		}
		if p+4 <= len(tiff) {
			next := int(order.Uint32(tiff[p : p+4]))
			if next > 0 {
				readIFD(next)
			}
		}
	}
	readIFD(ifd0)

	rdASCII := func(tag uint16) string {
		if b, ok := tags[tag]; ok {
			return strings.TrimRight(string(b), "\x00")
		}
		return ""
	}
	rdShort := func(tag uint16) (uint16, bool) {
		if b, ok := tags[tag]; ok && len(b) >= 2 {
			return order.Uint16(b[:2]), true
		}
		return 0, false
	}
	rdRat := func(tag uint16) (num, den uint32, ok bool) {
		if b, ok2 := tags[tag]; ok2 && len(b) >= 8 {
			return order.Uint32(b[:4]), order.Uint32(b[4:8]), true
		}
		return 0, 0, false
	}
	res := map[string]string{}
	if v, ok := rdShort(0x0112); ok {
		res["Orientation"] = strconv.Itoa(int(v))
	}
	if s := rdASCII(0x010F); s != "" {
		res["Make"] = s
	}
	if s := rdASCII(0x0110); s != "" {
		res["Model"] = s
	}
	if s := rdASCII(0x0132); s != "" {
		res["DateTime"] = s
	}
	if num, den, ok := rdRat(0x920A); ok && den != 0 {
		res["FocalLength"] = strconv.FormatFloat(float64(num)/float64(den), 'f', 2, 64) + " mm"
	}
	if num, den, ok := rdRat(0x829A); ok && den != 0 {
		res["ExposureTime"] = strconv.Itoa(int(num)) + "/" + strconv.Itoa(int(den)) + " s"
	}
	if v, ok := rdShort(0x8827); ok {
		res["ISO"] = strconv.Itoa(int(v))
	}
	return res, nil
}
func typeSize(t uint16) int {
	switch t {
	case 1, 2:
		return 1
	case 3:
		return 2
	case 4, 9:
		return 4
	case 5, 10:
		return 8
	case 7:
		return 1
	case 6, 8:
		return 1
	case 11:
		return 4
	case 12:
		return 8
	default:
		return 1
	}
}

//
// ------------------------------ Extras --------------------------------------
//

// Gamma correction (gamma>0; 1=no change).
func Gamma(src image.Image, gamma float64) *image.NRGBA {
	if gamma <= 0 {
		gamma = 1
	}
	n := ToNRGBA(src)
	lut := LUT1D{}
	for i := 0; i < 256; i++ {
		v := math.Pow(float64(i)/255.0, 1.0/gamma)
		u := clamp8(int(math.Round(v * 255)))
		lut.R[i], lut.G[i], lut.B[i] = u, u, u
	}
	return ApplyLUT1D(n, lut)
}

// Histogram equalization (on luma), preserves chroma via HSL lightness remap.
func EqualizeLuma(src image.Image) *image.NRGBA {
	n := ToNRGBA(src)
	r := n.Rect
	var hist [256]uint32
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			l := uint8(0.2126*float64(n.Pix[i+0]) + 0.7152*float64(n.Pix[i+1]) + 0.0722*float64(n.Pix[i+2]) + 0.5)
			hist[l]++
		}
	}
	// CDF
	var cdf [256]uint32
	cdf[0] = hist[0]
	for i := 1; i < 256; i++ {
		cdf[i] = cdf[i-1] + hist[i]
	}
	total := float64(r.Dx() * r.Dy())
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			rf := float64(n.Pix[i+0]) / 255
			gf := float64(n.Pix[i+1]) / 255
			bf := float64(n.Pix[i+2]) / 255
			h, s, l := rgbToHSL(rf, gf, bf)
			_ = l
			ll := uint8(clamp8(int(0.2126*float64(n.Pix[i+0]) + 0.7152*float64(n.Pix[i+1]) + 0.0722*float64(n.Pix[i+2]))))
			newL := float64(cdf[ll]) / total
			rr, gg, bb := hslToRGB(h, s, clampF01(newL))
			out.Pix[i+0] = uint8(rr * 255)
			out.Pix[i+1] = uint8(gg * 255)
			out.Pix[i+2] = uint8(bb * 255)
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// Reinhard tonemap with exposure parameter e (e>0).
func TonemapReinhard(src image.Image, exposure float64) *image.NRGBA {
	if exposure <= 0 {
		exposure = 1
	}
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			for c := 0; c < 3; c++ {
				v := exposure * float64(n.Pix[i+c]) / 255.0
				v = v / (1 + v)
				out.Pix[i+c] = clamp8(int(math.Round(v * 255)))
			}
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// Morphology (grayscale): mode "erode" / "dilate", radius>=1 uses 3x3 iterated.
func MorphologyGray(src image.Image, radius int, mode string) *image.Gray {
	g := image.NewGray(ToNRGBA(src).Rect)
	draw.Draw(g, g.Rect, src, g.Rect.Min, draw.Src)
	if radius < 1 {
		radius = 1
	}
	op := strings.ToLower(mode)
	for k := 0; k < radius; k++ {
		tmp := image.NewGray(g.Rect)
		for y := g.Rect.Min.Y; y < g.Rect.Max.Y; y++ {
			for x := g.Rect.Min.X; x < g.Rect.Max.X; x++ {
				var best uint8
				if op == "erode" {
					best = 255
				}
				for j := -1; j <= 1; j++ {
					yy := y + j
					if yy < g.Rect.Min.Y {
						yy = g.Rect.Min.Y
					} else if yy >= g.Rect.Max.Y {
						yy = g.Rect.Max.Y - 1
					}
					for i := -1; i <= 1; i++ {
						xx := x + i
						if xx < g.Rect.Min.X {
							xx = g.Rect.Min.X
						} else if xx >= g.Rect.Max.X {
							xx = g.Rect.Max.X - 1
						}
						v := g.GrayAt(xx, yy).Y
						if op == "erode" {
							if v < best {
								best = v
							}
						} else {
							if v > best {
								best = v
							}
						}
					}
				}
				tmp.SetGray(x, y, color.Gray{best})
			}
		}
		g = tmp
	}
	return g
}

// Dither Floyd–Steinberg to N levels per channel (>=2).
func DitherFS(src image.Image, levels int) *image.NRGBA {
	if levels < 2 {
		levels = 2
	}
	n := ToNRGBA(src)
	r := n.Rect
	out := CloneNRGBA(n)
	step := 255.0 / float64(levels-1)
	errBuf := make([][3]float64, r.Dx()*r.Dy())
	get := func(x, y int) int { return (y-r.Min.Y)*r.Dx() + (x - r.Min.X) }
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(out, x, y)
			ei := get(x, y)
			for c := 0; c < 3; c++ {
				val := float64(out.Pix[i+c]) + errBuf[ei][c]
				if val < 0 {
					val = 0
				}
				if val > 255 {
					val = 255
				}
				q := math.Round(val/step) * step
				out.Pix[i+c] = clamp8(int(q))
				err := val - q
				// diffuse
				if x+1 < r.Max.X {
					errBuf[get(x+1, y)][c] += err * 7 / 16
				}
				if x-1 >= r.Min.X && y+1 < r.Max.Y {
					errBuf[get(x-1, y+1)][c] += err * 3 / 16
				}
				if y+1 < r.Max.Y {
					errBuf[get(x, y+1)][c] += err * 5 / 16
				}
				if x+1 < r.Max.X && y+1 < r.Max.Y {
					errBuf[get(x+1, y+1)][c] += err * 1 / 16
				}
			}
		}
	}
	return out
}

//
// ------------------------------ Tiny bitmap font -----------------------------
//

// 5x7 ASCII (32..126), each row packed into 5 bits (LSB left). Source: tiny public-domain table.
var tiny5x7 = map[rune][7]uint8{
	'A': {0x1E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'B': {0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E},
	'C': {0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E},
	'D': {0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E},
	'E': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F},
	'F': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
	'G': {0x0F, 0x10, 0x10, 0x17, 0x11, 0x11, 0x0F},
	'H': {0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'I': {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x1F},
	'J': {0x01, 0x01, 0x01, 0x01, 0x11, 0x11, 0x0E},
	'K': {0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11},
	'L': {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
	'M': {0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11},
	'N': {0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11},
	'O': {0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'P': {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
	'Q': {0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D},
	'R': {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
	'S': {0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E},
	'T': {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'U': {0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'V': {0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04},
	'W': {0x11, 0x11, 0x11, 0x15, 0x15, 0x1B, 0x11},
	'X': {0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11},
	'Y': {0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04},
	'Z': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F},
	'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
	'1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'2': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F},
	'3': {0x1F, 0x02, 0x04, 0x02, 0x01, 0x11, 0x0E},
	'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
	'5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
	'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E},
	'7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
	'9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	' ': {0, 0, 0, 0, 0, 0, 0},
}

// DrawString5x7 renders ASCII in a solid color (scale>=1 expands pixels).
func DrawString5x7(dst *image.NRGBA, text string, at image.Point, col color.NRGBA, scale int) {
	if scale < 1 {
		scale = 1
	}
	x0, y0 := at.X, at.Y
	for _, ch := range text {
		if ch == '\n' {
			y0 += 8 * scale
			x0 = at.X
			continue
		}
		g, ok := tiny5x7[ch]
		if !ok {
			g = tiny5x7[' ']
		}
		for ry := 0; ry < 7; ry++ {
			row := g[ry]
			for rx := 0; rx < 5; rx++ {
				if (row>>uint(4-rx))&1 == 1 {
					for sy := 0; sy < scale; sy++ {
						for sx := 0; sx < scale; sx++ {
							x := x0 + rx*scale + sx
							y := y0 + ry*scale + sy
							if x >= dst.Rect.Min.X && x < dst.Rect.Max.X && y >= dst.Rect.Min.Y && y < dst.Rect.Max.Y {
								i := idx(dst, x, y)
								dst.Pix[i+0] = col.R
								dst.Pix[i+1] = col.G
								dst.Pix[i+2] = col.B
								dst.Pix[i+3] = col.A
							}
						}
					}
				}
			}
		}
		x0 += 6 * scale
	}
}

//
// ------------------------------ I/O helpers ---------------------------------
//

// Decode (basic)
func Decode(r io.Reader) (image.Image, string, error) { return image.Decode(r) }

// Encode helpers
func EncodePNG(w io.Writer, img image.Image) error { return png.Encode(w, img) }
func EncodeGIF(w io.Writer, img image.Image) error { return gif.Encode(w, img, nil) }
func EncodeJPEG(w io.Writer, img image.Image, q int) error {
	if q <= 0 || q > 100 {
		q = 90
	}
	return jpeg.Encode(w, img, &jpeg.Options{Quality: q})
}

// Decode options
type decodeOptions struct {
	AutoOrient bool
	MaxPixels  int // 0=unlimited, else reject if w*h > MaxPixels
}

// WithAutoOrient applies EXIF orientation for JPEGs after decode.
func WithAutoOrient() func(*decodeOptions) { return func(o *decodeOptions) { o.AutoOrient = true } }

// WithMaxPixels rejects huge images
func WithMaxPixels(n int) func(*decodeOptions) { return func(o *decodeOptions) { o.MaxPixels = n } }

// DecodeWithOptions reads entire stream (to allow EXIF scan), applies options.
func DecodeWithOptions(r io.Reader, opts ...func(*decodeOptions)) (image.Image, string, error) {
	var o decodeOptions
	for _, f := range opts {
		f(&o)
	}
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, r); err != nil {
		return nil, "", err
	}
	br := bytes.NewReader(buf.Bytes())
	img, format, err := image.Decode(br)
	if err != nil {
		return nil, "", err
	}
	b := img.Bounds()
	if o.MaxPixels > 0 && b.Dx()*b.Dy() > o.MaxPixels {
		return nil, "", errors.New("image too large")
	}
	if o.AutoOrient && strings.EqualFold(format, "jpeg") {
		// Scan EXIF
		exif, err := ReadJPEGEXIF(bytes.NewReader(buf.Bytes()))
		if err == nil {
			if s := exif["Orientation"]; s != "" {
				code, _ := strconv.Atoi(s)
				img = applyEXIFOrientation(img, code)
			}
		}
	}
	return img, format, nil
}

func applyEXIFOrientation(img image.Image, code int) image.Image {
	switch code {
	case 1:
		return img
	case 2:
		return FlipX(img)
	case 3:
		return Rotate(img, 180, "nearest", color.NRGBA{})
	case 4:
		return FlipY(img)
	case 5:
		return Rotate(FlipX(img), 90, "nearest", color.NRGBA{})
	case 6:
		return Rotate(img, 90, "nearest", color.NRGBA{})
	case 7:
		return Rotate(FlipX(img), 270, "nearest", color.NRGBA{})
	case 8:
		return Rotate(img, 270, "nearest", color.NRGBA{})
	default:
		return img
	}
}

//
// ------------------------------ Fluent pipeline -----------------------------
//
// The Pipeline supports chainable methods.  It lazily records steps,
// and when you call Image/Do/Encode it executes them.  Consecutive
// per-pixel steps are fused into a single pass.
//

// PixelFunc transforms a single pixel (r,g,b,a -> r,g,b,a)
type PixelFunc func(r, g, b, a uint8) (uint8, uint8, uint8, uint8)

type step struct {
	name  string
	pf    PixelFunc                       // if set, is a per-pixel op (fusible)
	apply func(*image.NRGBA) *image.NRGBA // generic (blur, geometry, LUT3D, etc.)
}

type Pipeline struct {
	img   *image.NRGBA
	steps []step
}

// From wraps an image into a chainable pipeline.
func From(src image.Image) *Pipeline { return &Pipeline{img: ToNRGBA(src)} }

// fusePerPixel collapses consecutive pf steps into one.
func (p *Pipeline) fusePerPixel() {
	if len(p.steps) == 0 {
		return
	}
	var fused []step
	var cur PixelFunc
	var names []string
	flush := func() {
		if cur != nil {
			f := cur
			title := strings.Join(names, "+")
			fused = append(fused, step{
				name: title,
				pf:   func(r, g, b, a uint8) (uint8, uint8, uint8, uint8) { return f(r, g, b, a) },
			})
			cur, names = nil, nil
		}
	}
	for _, s := range p.steps {
		if s.pf != nil {
			if cur == nil {
				cur = s.pf
			} else {
				prev := cur
				// capture s.pf into local to avoid closing over loop variable 's'
				spf := s.pf
				cur = func(r, g, b, a uint8) (uint8, uint8, uint8, uint8) {
					r, g, b, a = prev(r, g, b, a)
					return spf(r, g, b, a)
				}
			}
			names = append(names, s.name)
			continue
		}
		flush()
		fused = append(fused, s)
	}
	flush()
	p.steps = fused
}

// execute applies all steps, honoring context cancellation.
func (p *Pipeline) execute(ctx context.Context) (*image.NRGBA, error) {
	p.fusePerPixel()
	img := p.img
	for _, s := range p.steps {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if s.pf != nil {
			r := img.Rect
			out := image.NewNRGBA(r)
			err := parallelRows(ctx, r.Dy(), func(yy int) {
				y := r.Min.Y + yy
				for x := r.Min.X; x < r.Max.X; x++ {
					i := idx(img, x, y)
					rr, gg, bb, aa := s.pf(img.Pix[i+0], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3])
					out.Pix[i+0], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = rr, gg, bb, aa
				}
			})
			if err != nil {
				return nil, err
			}
			img = out
			continue
		}
		img = s.apply(img)
	}
	p.img = img
	p.steps = nil
	return img, nil
}

// Do runs the pipeline now (no context).
func (p *Pipeline) Do() *Pipeline { _, _ = p.execute(context.Background()); return p }

// Execute runs with context.
func (p *Pipeline) Execute(ctx context.Context) (*Pipeline, error) {
	_, err := p.execute(ctx)
	return p, err
}

// Image returns the final image, executing pending steps.
func (p *Pipeline) Image() *image.NRGBA { img, _ := p.execute(context.Background()); return img }

// ---- Chainable per-pixel helpers ----

func pfBrightness(delta float64) PixelFunc {
	return func(r, g, b, a uint8) (uint8, uint8, uint8, uint8) {
		// HSL lightness shift
		rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
		h, s, l := rgbToHSL(rf, gf, bf)
		l = clampF01(l + delta)
		rr, gg, bb := hslToRGB(h, s, l)
		return uint8(rr * 255), uint8(gg * 255), uint8(bb * 255), a
	}
}
func pfContrast(factor float64) PixelFunc {
	return func(r, g, b, a uint8) (uint8, uint8, uint8, uint8) {
		fr := (float64(r)/255 - 0.5) * factor
		fg := (float64(g)/255 - 0.5) * factor
		fb := (float64(b)/255 - 0.5) * factor
		return clamp8(int((fr + 0.5) * 255)), clamp8(int((fg + 0.5) * 255)), clamp8(int((fb + 0.5) * 255)), a
	}
}
func pfSaturation(factor float64) PixelFunc {
	return func(r, g, b, a uint8) (uint8, uint8, uint8, uint8) {
		rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
		h, s, l := rgbToHSL(rf, gf, bf)
		s = clampF01(s * factor)
		rr, gg, bb := hslToRGB(h, s, l)
		return uint8(rr * 255), uint8(gg * 255), uint8(bb * 255), a
	}
}
func pfHue(deg float64) PixelFunc {
	shift := ((deg / 360.0) - math.Floor(deg/360.0))
	return func(r, g, b, a uint8) (uint8, uint8, uint8, uint8) {
		rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
		h, s, l := rgbToHSL(rf, gf, bf)
		h = math.Mod(h+shift+1, 1)
		rr, gg, bb := hslToRGB(h, s, l)
		return uint8(rr * 255), uint8(gg * 255), uint8(bb * 255), a
	}
}
func pfInvert() PixelFunc {
	return func(r, g, b, a uint8) (uint8, uint8, uint8, uint8) {
		return 255 - r, 255 - g, 255 - b, a
	}
}
func pfSepia(amount float64) PixelFunc {
	amount = clampF01(amount)
	return func(r, g, b, a uint8) (uint8, uint8, uint8, uint8) {
		rf, gf, bf := float64(r), float64(g), float64(b)
		tr := 0.393*rf + 0.769*gf + 0.189*bf
		tg := 0.349*rf + 0.686*gf + 0.168*bf
		tb := 0.272*rf + 0.534*gf + 0.131*bf
		rr := (1-amount)*rf + amount*tr
		gg := (1-amount)*gf + amount*tg
		bb := (1-amount)*bf + amount*tb
		return clamp8(int(rr + 0.5)), clamp8(int(gg + 0.5)), clamp8(int(bb + 0.5)), a
	}
}
func pfGamma(gamma float64) PixelFunc {
	if gamma <= 0 {
		gamma = 1
	}
	var table [256]uint8
	for i := 0; i < 256; i++ {
		table[i] = clamp8(int(math.Pow(float64(i)/255.0, 1.0/gamma)*255 + 0.5))
	}
	return func(r, g, b, a uint8) (uint8, uint8, uint8, uint8) {
		return table[r], table[g], table[b], a
	}
}

// New per-pixel effects
func pfGrayscale() PixelFunc {
	return func(r, g, b, a uint8) (uint8, uint8, uint8, uint8) {
		l := uint8(0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b) + 0.5)
		return l, l, l, a
	}
}
func pfSolarize(cutoff uint8) PixelFunc {
	return func(r, g, b, a uint8) (uint8, uint8, uint8, uint8) {
		rr, gg, bb := r, g, b
		if r > cutoff {
			rr = 255 - r
		}
		if g > cutoff {
			gg = 255 - g
		}
		if b > cutoff {
			bb = 255 - b
		}
		return rr, gg, bb, a
	}
}

// Emboss (simple convolution) - strength 0..1 (recommended ~0.5)
func Emboss(src image.Image, strength float64) *image.NRGBA {
	if strength < 0 {
		strength = 0
	}
	if strength > 1 {
		strength = 1
	}
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	// kernel: [-2 -1 0; -1 1 1; 0 1 2]
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			var accR, accG, accB float64
			offs := [3][3]int{{-2, -1, 0}, {-1, 1, 1}, {0, 1, 2}}
			for j := -1; j <= 1; j++ {
				yy2 := y + j
				if yy2 < r.Min.Y {
					yy2 = r.Min.Y
				} else if yy2 >= r.Max.Y {
					yy2 = r.Max.Y - 1
				}
				for i := -1; i <= 1; i++ {
					xx := x + i
					if xx < r.Min.X {
						xx = r.Min.X
					} else if xx >= r.Max.X {
						xx = r.Max.X - 1
					}
					p := idx(n, xx, yy2)
					k := float64(offs[j+1][i+1])
					accR += k * float64(n.Pix[p+0])
					accG += k * float64(n.Pix[p+1])
					accB += k * float64(n.Pix[p+2])
				}
			}
			i := idx(out, x, y)
			// normalize and mix with original based on strength
			nr := clamp8(int(math.Round(128 + accR*strength)))
			ng := clamp8(int(math.Round(128 + accG*strength)))
			nb := clamp8(int(math.Round(128 + accB*strength)))
			out.Pix[i+0], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = nr, ng, nb, n.Pix[idx(n, x, y)+3]
		}
	})
	return out
}

// Vignette: darken edges by strength (0..1)
func Vignette(src image.Image, strength float64) *image.NRGBA {
	if strength < 0 {
		strength = 0
	}
	if strength > 1 {
		strength = 1
	}
	n := ToNRGBA(src)
	r := n.Rect
	out := CloneNRGBA(n)
	cx := float64(r.Min.X + r.Dx()/2)
	cy := float64(r.Min.Y + r.Dy()/2)
	maxd := math.Hypot(float64(r.Dx())/2, float64(r.Dy())/2)
	_ = parallelRows(context.Background(), r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			dx := float64(x) - cx
			dy := float64(y) - cy
			d := math.Hypot(dx, dy)
			t := d / maxd
			factor := 1 - strength*(t*t)
			if factor < 0 {
				factor = 0
			}
			out.Pix[i+0] = clamp8(int(math.Round(float64(n.Pix[i+0]) * factor)))
			out.Pix[i+1] = clamp8(int(math.Round(float64(n.Pix[i+1]) * factor)))
			out.Pix[i+2] = clamp8(int(math.Round(float64(n.Pix[i+2]) * factor)))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// Convenience one-shot functions for grayscale and solarize
func Grayscale(src image.Image) *image.NRGBA { return ToNRGBA(ApplyLUT1D(src, LUT1D{})) }
// Note: Implement Grayscale properly (using pf)
func GrayscaleProper(src image.Image) *image.NRGBA {
	n := ToNRGBA(src)
	p := From(n).Brightness(0) // no-op starter
	p.steps = append([]step{{name: "grayscale", pf: pfGrayscale()}}, p.steps...)
	img, _ := p.execute(context.Background())
	return img
}
func Solarize(src image.Image, cutoff uint8) *image.NRGBA {
	n := ToNRGBA(src)
	p := From(n)
	p.steps = append(p.steps, step{name: "solarize", pf: pfSolarize(cutoff)})
	img, _ := p.execute(context.Background())
	return img
}

// ---- Chainable API methods ----

func (p *Pipeline) Brightness(delta float64) *Pipeline {
	p.steps = append(p.steps, step{"brightness", pfBrightness(delta), nil})
	return p
}
func (p *Pipeline) Contrast(factor float64) *Pipeline {
	p.steps = append(p.steps, step{"contrast", pfContrast(factor), nil})
	return p
}
func (p *Pipeline) Saturation(factor float64) *Pipeline {
	p.steps = append(p.steps, step{"saturation", pfSaturation(factor), nil})
	return p
}
func (p *Pipeline) Hue(deg float64) *Pipeline {
	p.steps = append(p.steps, step{"hue", pfHue(deg), nil})
	return p
}
func (p *Pipeline) Invert() *Pipeline {
	p.steps = append(p.steps, step{"invert", pfInvert(), nil})
	return p
}
func (p *Pipeline) Sepia(amount float64) *Pipeline {
	p.steps = append(p.steps, step{"sepia", pfSepia(amount), nil})
	return p
}
func (p *Pipeline) Gamma(g float64) *Pipeline {
	p.steps = append(p.steps, step{"gamma", pfGamma(g), nil})
	return p
}

func (p *Pipeline) GaussianBlur(sigma float64, radius int) *Pipeline {
	p.steps = append(p.steps, step{name: "gaussianBlur", apply: func(in *image.NRGBA) *image.NRGBA { return GaussianBlur(in, sigma, radius) }})
	return p
}
func (p *Pipeline) Unsharp(amount, sigma float64, radius int) *Pipeline {
	p.steps = append(p.steps, step{name: "unsharp", apply: func(in *image.NRGBA) *image.NRGBA { return UnsharpMask(in, amount, sigma, radius) }})
	return p
}
func (p *Pipeline) LUT3D(l *LUT3D) *Pipeline {
	p.steps = append(p.steps, step{name: "lut3d", apply: func(in *image.NRGBA) *image.NRGBA { return ApplyLUT3D(in, l) }})
	return p
}
func (p *Pipeline) Pixelate(block int) *Pipeline {
	p.steps = append(p.steps, step{name: "pixelate", apply: func(in *image.NRGBA) *image.NRGBA { return Pixelate(in, block) }})
	return p
}
func (p *Pipeline) Posterize(levels int) *Pipeline {
	p.steps = append(p.steps, step{name: "posterize", apply: func(in *image.NRGBA) *image.NRGBA { return Posterize(in, levels) }})
	return p
}
func (p *Pipeline) Threshold(cutoff uint8) *Pipeline {
	p.steps = append(p.steps, step{name: "threshold", apply: func(in *image.NRGBA) *image.NRGBA { return ToNRGBA(Threshold(in, cutoff)) }})
	return p
}
func (p *Pipeline) WhiteBalance(rect image.Rectangle) *Pipeline {
	p.steps = append(p.steps, step{name: "whitebalance", apply: func(in *image.NRGBA) *image.NRGBA { return WhiteBalanceByRect(in, rect) }})
	return p
}
func (p *Pipeline) Rotate(deg float64, method string, bg color.NRGBA) *Pipeline {
	p.steps = append(p.steps, step{name: "rotate", apply: func(in *image.NRGBA) *image.NRGBA { return Rotate(in, deg, method, bg) }})
	return p
}
func (p *Pipeline) Skew(sx, sy float64, method string, bg color.NRGBA) *Pipeline {
	p.steps = append(p.steps, step{name: "skew", apply: func(in *image.NRGBA) *image.NRGBA { return Skew(in, sx, sy, method, bg) }})
	return p
}
func (p *Pipeline) ResizeBilinear(w, h int) *Pipeline {
	p.steps = append(p.steps, step{name: "resizeBilinear", apply: func(in *image.NRGBA) *image.NRGBA { return ResizeBilinear(in, w, h) }})
	return p
}
func (p *Pipeline) ResizeNearest(w, h int) *Pipeline {
	p.steps = append(p.steps, step{name: "resizeNearest", apply: func(in *image.NRGBA) *image.NRGBA { return ResizeNearest(in, w, h) }})
	return p
}
func (p *Pipeline) Crop(rect image.Rectangle) *Pipeline {
	p.steps = append(p.steps, step{name: "crop", apply: func(in *image.NRGBA) *image.NRGBA { return Crop(in, rect) }})
	return p
}
func (p *Pipeline) Trim(col color.NRGBA, tol uint8) *Pipeline {
	p.steps = append(p.steps, step{name: "trim", apply: func(in *image.NRGBA) *image.NRGBA { return TrimByColor(in, col, tol) }})
	return p
}
func (p *Pipeline) Watermark(mark image.Image, pos image.Point, opacity float64) *Pipeline {
	p.steps = append(p.steps, step{name: "watermark", apply: func(in *image.NRGBA) *image.NRGBA { return WatermarkImage(in, mark, pos, opacity) }})
	return p
}
func (p *Pipeline) Equalize() *Pipeline {
	p.steps = append(p.steps, step{name: "equalize", apply: func(in *image.NRGBA) *image.NRGBA { return EqualizeLuma(in) }})
	return p
}
func (p *Pipeline) Tonemap(exposure float64) *Pipeline {
	p.steps = append(p.steps, step{name: "tonemap", apply: func(in *image.NRGBA) *image.NRGBA { return TonemapReinhard(in, exposure) }})
	return p
}
func (p *Pipeline) Dither(levels int) *Pipeline {
	p.steps = append(p.steps, step{name: "ditherFS", apply: func(in *image.NRGBA) *image.NRGBA { return DitherFS(in, levels) }})
	return p
}
func (p *Pipeline) DrawText(s string, at image.Point, col color.NRGBA, scale int) *Pipeline {
	p.steps = append(p.steps, step{name: "text", apply: func(in *image.NRGBA) *image.NRGBA {
		out := CloneNRGBA(in)
		DrawString5x7(out, s, at, col, scale)
		return out
	}})
	return p
}

// New chainable effects
func (p *Pipeline) Grayscale() *Pipeline { p.steps = append(p.steps, step{name: "grayscale", pf: pfGrayscale()}); return p }
func (p *Pipeline) Solarize(cutoff uint8) *Pipeline { p.steps = append(p.steps, step{name: "solarize", pf: pfSolarize(cutoff)}); return p }
func (p *Pipeline) Emboss(strength float64) *Pipeline { p.steps = append(p.steps, step{name: "emboss", apply: func(in *image.NRGBA) *image.NRGBA { return Emboss(in, strength) }}); return p }
func (p *Pipeline) Vignette(strength float64) *Pipeline { p.steps = append(p.steps, step{name: "vignette", apply: func(in *image.NRGBA) *image.NRGBA { return Vignette(in, strength) }}); return p }
func (p *Pipeline) NoiseReduction(radius int) *Pipeline {
	p.steps = append(p.steps, step{name: "noiseReduction", apply: func(in *image.NRGBA) *image.NRGBA { return NoiseReduction(in, radius) }})
	return p
}

// Collage places the current pipeline image and other side by side or stacked.
// direction: "horizontal"/"h" (default) or "vertical"/"v". bg fills any gap.
func (p *Pipeline) Collage(other image.Image, direction string, bg color.NRGBA) *Pipeline {
	p.steps = append(p.steps, step{name: "collage", apply: func(in *image.NRGBA) *image.NRGBA {
		return AlignImages([]image.Image{in, other}, direction, bg)
	}})
	return p
}

// Plugin system
type FilterFunc func(*image.NRGBA) *image.NRGBA

var Registry = map[string]FilterFunc{}

func Register(name string, f FilterFunc) { Registry[name] = f }

func (p *Pipeline) Apply(name string) *Pipeline {
	p.steps = append(p.steps, step{name: "apply:" + name, apply: func(in *image.NRGBA) *image.NRGBA {
		if f, ok := Registry[name]; ok {
			return f(in)
		}
		return in
	}})
	return p
}

// Save helpers on Pipeline
func (p *Pipeline) EncodePNG(w io.Writer) error {
	_, _ = p.execute(context.Background())
	return EncodePNG(w, p.img)
}
func (p *Pipeline) EncodeGIF(w io.Writer) error {
	_, _ = p.execute(context.Background())
	return EncodeGIF(w, p.img)
}
func (p *Pipeline) EncodeJPEG(w io.Writer, q int) error {
	_, _ = p.execute(context.Background())
	return EncodeJPEG(w, p.img, q)
}

// Execute with timeout for long chains (optional helper)
func (p *Pipeline) ExecuteWithTimeout(timeout time.Duration) (*Pipeline, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := p.execute(ctx)
	return p, err
}

// Convenience one-shot color adjust
func AdjustColor(src image.Image, brightnessDelta, contrast, saturation, hueShift float64) *image.NRGBA {
	o := src
	if brightnessDelta != 0 {
		o = AdjustBrightness(o, brightnessDelta)
	}
	if contrast != 1 {
		o = AdjustContrast(o, contrast)
	}
	if saturation != 1 {
		o = AdjustSaturation(o, saturation)
	}
	if hueShift != 0 {
		o = AdjustHue(o, hueShift)
	}
	return ToNRGBA(o)
}
