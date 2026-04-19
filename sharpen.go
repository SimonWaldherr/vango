package vango

import (
	"context"
	"image"
	"math"
)

// HighPassSharpen sharpens by blending the high-frequency detail with the
// original. amount controls intensity (1.0 = normal), radius sets the blur
// used to extract the low-frequency component.
func HighPassSharpen(src image.Image, amount float64, radius int) *image.NRGBA {
	n := ToNRGBA(src)
	sigma := float64(radius)
	if sigma < 0.5 {
		sigma = 0.5
	}
	low := GaussianBlur(n, sigma, radius)
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), 0, r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			j := idx(low, x, y)
			for c := 0; c < 3; c++ {
				highPass := float64(n.Pix[i+c]) - float64(low.Pix[j+c])
				v := float64(n.Pix[i+c]) + highPass*amount
				out.Pix[i+c] = clamp8(int(math.Round(v)))
			}
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// Clarity enhances midtone contrast, giving images a punchy look.
// strength 0..1 (typical: 0.3-0.7).
func Clarity(src image.Image, strength float64) *image.NRGBA {
	n := ToNRGBA(src)
	blurred := GaussianBlur(n, 10, 0) // large radius for midtone extraction
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), 0, r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			j := idx(blurred, x, y)
			for c := 0; c < 3; c++ {
				orig := float64(n.Pix[i+c]) / 255.0
				blur := float64(blurred.Pix[j+c]) / 255.0
				// midtone mask: peaks at 0.5, drops at shadows/highlights
				midMask := 1.0 - math.Abs(orig-0.5)*2
				midMask = midMask * midMask // emphasize midtones
				detail := orig - blur
				v := orig + detail*strength*midMask
				out.Pix[i+c] = clamp8(int(clampF01(v) * 255))
			}
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// SharpenConvolution applies a classic 3×3 sharpen kernel.
// amount controls the sharpening intensity (1.0 = standard).
func SharpenConvolution(src image.Image, amount float64) *image.NRGBA {
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	center := 1 + 4*amount
	edge := -amount
	_ = parallelRows(context.Background(), 0, r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			var rr, gg, bb float64
			for _, off := range [][2]int{{0, -1}, {-1, 0}, {0, 0}, {1, 0}, {0, 1}} {
				xx := x + off[0]
				yy2 := y + off[1]
				if xx < r.Min.X {
					xx = r.Min.X
				} else if xx >= r.Max.X {
					xx = r.Max.X - 1
				}
				if yy2 < r.Min.Y {
					yy2 = r.Min.Y
				} else if yy2 >= r.Max.Y {
					yy2 = r.Max.Y - 1
				}
				p := idx(n, xx, yy2)
				w := edge
				if off[0] == 0 && off[1] == 0 {
					w = center
				}
				rr += float64(n.Pix[p+0]) * w
				gg += float64(n.Pix[p+1]) * w
				bb += float64(n.Pix[p+2]) * w
			}
			i := idx(out, x, y)
			out.Pix[i+0] = clamp8(int(math.Round(rr)))
			out.Pix[i+1] = clamp8(int(math.Round(gg)))
			out.Pix[i+2] = clamp8(int(math.Round(bb)))
			out.Pix[i+3] = n.Pix[idx(n, x, y)+3]
		}
	})
	return out
}

// Pipeline methods for sharpening

func (p *Pipeline) HighPassSharpen(amount float64, radius int) *Pipeline {
	p.steps = append(p.steps, step{name: "highPassSharpen", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return HighPassSharpen(in, amount, radius)
	}})
	return p
}

func (p *Pipeline) Clarity(strength float64) *Pipeline {
	p.steps = append(p.steps, step{name: "clarity", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return Clarity(in, strength)
	}})
	return p
}

func (p *Pipeline) SharpenConvolution(amount float64) *Pipeline {
	p.steps = append(p.steps, step{name: "sharpen", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return SharpenConvolution(in, amount)
	}})
	return p
}
