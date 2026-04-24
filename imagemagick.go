package vango

// ImageMagick-inspired image processing functions.
// This file adds operations that mirror or are inspired by ImageMagick's
// command-line tools such as -normalize, -charcoal, -sketch, -roll, -spread,
// -transpose, -transverse, -sigmoidal-contrast, -extent, -ordered-dither,
// -selective-blur, -auto-threshold, -adaptive-blur, -adaptive-sharpen,
// -morphology (dilate/erode/open/close), -statistic, -mean-shift, and -kuwahara.

import (
	"context"
	"image"
	"image/color"
	"math"
	"math/rand"
	"sort"
)

// --------------------------------------------------------------------------
// Normalize / AutoLevel — per-channel histogram stretch
// --------------------------------------------------------------------------

// Normalize stretches each colour channel independently so its darkest pixel
// maps to 0 and its brightest maps to 255 (equivalent to ImageMagick -normalize).
func Normalize(src image.Image) *image.NRGBA {
	return NormalizeCtx(context.Background(), src)
}

func NormalizeCtx(ctx context.Context, src image.Image) *image.NRGBA {
	n := ToNRGBA(src)
	b := n.Rect
	var minR, minG, minB uint8 = 255, 255, 255
	var maxR, maxG, maxB uint8

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := idx(n, x, y)
			if n.Pix[i] < minR {
				minR = n.Pix[i]
			}
			if n.Pix[i] > maxR {
				maxR = n.Pix[i]
			}
			if n.Pix[i+1] < minG {
				minG = n.Pix[i+1]
			}
			if n.Pix[i+1] > maxG {
				maxG = n.Pix[i+1]
			}
			if n.Pix[i+2] < minB {
				minB = n.Pix[i+2]
			}
			if n.Pix[i+2] > maxB {
				maxB = n.Pix[i+2]
			}
		}
	}

	scaleR, scaleG, scaleB := 1.0, 1.0, 1.0
	if maxR > minR {
		scaleR = 255.0 / float64(maxR-minR)
	}
	if maxG > minG {
		scaleG = 255.0 / float64(maxG-minG)
	}
	if maxB > minB {
		scaleB = 255.0 / float64(maxB-minB)
	}

	dst := CloneNRGBA(n)
	_ = parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := idx(dst, x, y)
			dst.Pix[i] = clamp8(int(float64(dst.Pix[i]-minR) * scaleR))
			dst.Pix[i+1] = clamp8(int(float64(dst.Pix[i+1]-minG) * scaleG))
			dst.Pix[i+2] = clamp8(int(float64(dst.Pix[i+2]-minB) * scaleB))
		}
	})
	return dst
}

// AutoLevel is an alias for Normalize (ImageMagick -auto-level behaviour).
func AutoLevel(src image.Image) *image.NRGBA { return Normalize(src) }

// --------------------------------------------------------------------------
// Charcoal — charcoal sketch effect (ImageMagick -charcoal)
// --------------------------------------------------------------------------

// Charcoal produces a charcoal-sketch look: grayscale, blur, edge-detect,
// negate and normalize.  radius is the Gaussian blur kernel radius (pixels),
// sigma is the blur standard deviation.
func Charcoal(src image.Image, radius int, sigma float64) *image.NRGBA {
	if sigma <= 0 {
		sigma = float64(radius) * 0.5
		if sigma < 0.5 {
			sigma = 0.5
		}
	}
	gray := GrayscaleProper(src)
	blurred := GaussianBlur(gray, sigma, radius)

	// Laplacian edge detection kernel (3×3)
	lap := [9]float64{0, 1, 0, 1, -4, 1, 0, 1, 0}
	b := blurred.Rect
	edge := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			var sum float64
			for ky := -1; ky <= 1; ky++ {
				for kx := -1; kx <= 1; kx++ {
					sx := x + kx
					if sx < b.Min.X {
						sx = b.Min.X
					} else if sx >= b.Max.X {
						sx = b.Max.X - 1
					}
					sy := y + ky
					if sy < b.Min.Y {
						sy = b.Min.Y
					} else if sy >= b.Max.Y {
						sy = b.Max.Y - 1
					}
					p := idx(blurred, sx, sy)
					lum := float64(blurred.Pix[p])
					sum += lum * lap[(ky+1)*3+(kx+1)]
				}
			}
			v := clamp8(int(math.Abs(sum)))
			i := idx(edge, x, y)
			edge.Pix[i] = v
			edge.Pix[i+1] = v
			edge.Pix[i+2] = v
			edge.Pix[i+3] = 255
		}
	}
	// Negate (so edges appear dark on white background) then normalize
	return Normalize(Invert(edge))
}

// --------------------------------------------------------------------------
// Sketch — pencil sketch effect (ImageMagick -sketch)
// --------------------------------------------------------------------------

// Sketch simulates a pencil-sketch look.  sigma controls the softness of the
// sketch lines; angle is unused for the colour-dodge technique but kept for
// API parity with ImageMagick.
func Sketch(src image.Image, sigma float64, angle float64) *image.NRGBA {
	_ = angle // reserved for future motion-blur based variant
	if sigma <= 0 {
		sigma = 1.0
	}
	radius := int(math.Ceil(sigma * 3))
	if radius < 1 {
		radius = 1
	}
	gray := GrayscaleProper(src)
	inverted := Invert(gray)
	blurred := GaussianBlur(inverted, sigma, radius)

	// Colour dodge: result = clamp(base / (1 - blend))
	dst := CloneNRGBA(gray)
	b := gray.Rect
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			gi := idx(gray, x, y)
			bi := idx(blurred, x, y)
			base := float64(gray.Pix[gi]) / 255.0
			blend := float64(blurred.Pix[bi]) / 255.0
			var v float64
			if blend >= 1.0 {
				v = 1.0
			} else {
				v = base / (1.0 - blend)
				if v > 1.0 {
					v = 1.0
				}
			}
			val := uint8(v * 255)
			di := idx(dst, x, y)
			dst.Pix[di] = val
			dst.Pix[di+1] = val
			dst.Pix[di+2] = val
			dst.Pix[di+3] = 255
		}
	}
	return dst
}

// --------------------------------------------------------------------------
// SigmoidalContrast — S-curve contrast (ImageMagick -sigmoidal-contrast)
// --------------------------------------------------------------------------

// SigmoidalContrast applies a sigmoidal (S-curve) contrast adjustment to all
// channels.  strength controls how steep the S-curve is (typical 3–10);
// midpoint is the pivot point in the 0..1 range (0.5 = mid-gray).
// If sharpen is true the contrast is increased; if false it is decreased.
func SigmoidalContrast(src image.Image, sharpen bool, strength, midpoint float64) *image.NRGBA {
	return SigmoidalContrastCtx(context.Background(), src, sharpen, strength, midpoint)
}

func SigmoidalContrastCtx(ctx context.Context, src image.Image, sharpen bool, strength, midpoint float64) *image.NRGBA {
	if midpoint <= 0 {
		midpoint = 0.5
	}
	if strength <= 0 {
		strength = 5.0
	}

	// Pre-compute a 256-entry LUT.
	lut := [256]uint8{}
	sig := func(u float64) float64 {
		return 1.0 / (1.0 + math.Exp(-strength*(u-midpoint)))
	}
	sigMin := sig(0)
	sigMax := sig(1)
	span := sigMax - sigMin
	for i := 0; i < 256; i++ {
		u := float64(i) / 255.0
		var out float64
		if sharpen {
			out = (sig(u) - sigMin) / span
		} else {
			// Inverse: map back through inverse sigmoid.
			// Normalise the input value into the [sigMin, sigMax] range, then
			// apply the inverse of the logistic function.
			normalised := clampF01(float64(i)/255.0*(sigMax-sigMin) + sigMin)
			out = midpoint - math.Log(1.0/normalised-1.0)/strength
		}
		lut[i] = clamp8(int(out * 255))
	}

	dst := CloneNRGBA(ToNRGBA(src))
	b := dst.Rect
	_ = parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := idx(dst, x, y)
			dst.Pix[i] = lut[dst.Pix[i]]
			dst.Pix[i+1] = lut[dst.Pix[i+1]]
			dst.Pix[i+2] = lut[dst.Pix[i+2]]
		}
	})
	return dst
}

// --------------------------------------------------------------------------
// Extent — expand canvas with fill (ImageMagick -extent)
// --------------------------------------------------------------------------

// Extent creates a new canvas of size w×h, fills it with bg, then overlays
// the source image at the position determined by gravity.
// Supported gravity values: "northwest", "north", "northeast", "west",
// "center" (default), "east", "southwest", "south", "southeast".
func Extent(src image.Image, w, h int, gravity string, bg color.NRGBA) *image.NRGBA {
	srcB := src.Bounds()
	sw, sh := srcB.Dx(), srcB.Dy()

	var ox, oy int
	switch gravity {
	case "northwest", "nw":
		ox, oy = 0, 0
	case "north", "n":
		ox, oy = (w-sw)/2, 0
	case "northeast", "ne":
		ox, oy = w-sw, 0
	case "west", "w":
		ox, oy = 0, (h-sh)/2
	case "east", "e":
		ox, oy = w-sw, (h-sh)/2
	case "southwest", "sw":
		ox, oy = 0, h-sh
	case "south", "s":
		ox, oy = (w-sw)/2, h-sh
	case "southeast", "se":
		ox, oy = w-sw, h-sh
	default: // center
		ox, oy = (w-sw)/2, (h-sh)/2
	}

	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	// Fill background
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*dst.Stride + x*4)
			dst.Pix[i] = bg.R
			dst.Pix[i+1] = bg.G
			dst.Pix[i+2] = bg.B
			dst.Pix[i+3] = bg.A
		}
	}
	// Composite source over destination
	srcNRGBA := ToNRGBA(src)
	sb := srcNRGBA.Rect
	for y := sb.Min.Y; y < sb.Max.Y; y++ {
		dy := y - sb.Min.Y + oy
		if dy < 0 || dy >= h {
			continue
		}
		for x := sb.Min.X; x < sb.Max.X; x++ {
			dx := x - sb.Min.X + ox
			if dx < 0 || dx >= w {
				continue
			}
			si := idx(srcNRGBA, x, y)
			di := dy*dst.Stride + dx*4
			sa := float64(srcNRGBA.Pix[si+3]) / 255.0
			da := float64(dst.Pix[di+3]) / 255.0
			outA := sa + da*(1-sa)
			if outA == 0 {
				dst.Pix[di] = 0
				dst.Pix[di+1] = 0
				dst.Pix[di+2] = 0
				dst.Pix[di+3] = 0
				continue
			}
			for c := 0; c < 3; c++ {
				sc := float64(srcNRGBA.Pix[si+c]) / 255.0
				dc := float64(dst.Pix[di+c]) / 255.0
				dst.Pix[di+c] = clamp8(int((sc*sa + dc*da*(1-sa)) / outA * 255))
			}
			dst.Pix[di+3] = clamp8(int(outA * 255))
		}
	}
	return dst
}

// --------------------------------------------------------------------------
// Roll — circular shift (ImageMagick -roll)
// --------------------------------------------------------------------------

// Roll shifts the image by (dx, dy) pixels with wrap-around, equivalent to
// ImageMagick's -roll +dx+dy.
func Roll(src image.Image, dx, dy int) *image.NRGBA {
	n := ToNRGBA(src)
	b := n.Rect
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return CloneNRGBA(n)
	}
	// Normalise to positive offsets
	dx = ((dx % w) + w) % w
	dy = ((dy % h) + h) % h

	dst := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		sy := b.Min.Y + (y-b.Min.Y+dy)%h
		for x := b.Min.X; x < b.Max.X; x++ {
			sx := b.Min.X + (x-b.Min.X+dx)%w
			si := idx(n, sx, sy)
			di := idx(dst, x, y)
			copy(dst.Pix[di:di+4], n.Pix[si:si+4])
		}
	}
	return dst
}

// --------------------------------------------------------------------------
// Spread — random pixel displacement (ImageMagick -spread)
// --------------------------------------------------------------------------

// Spread displaces each pixel randomly within ±radius pixels (ImageMagick
// -spread radius).
func Spread(src image.Image, radius int) *image.NRGBA {
	return SpreadCtx(context.Background(), src, radius)
}

func SpreadCtx(ctx context.Context, src image.Image, radius int) *image.NRGBA {
	if radius < 1 {
		radius = 1
	}
	n := ToNRGBA(src)
	b := n.Rect
	dst := image.NewNRGBA(b)
	diameter := 2*radius + 1
	_ = parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		// Each goroutine gets its own RNG seeded by the row index.
		rng := rand.New(rand.NewSource(int64(y + 1))) //nolint:gosec
		for x := b.Min.X; x < b.Max.X; x++ {
			ox := x + rng.Intn(diameter) - radius
			oy := y + rng.Intn(diameter) - radius
			if ox < b.Min.X {
				ox = b.Min.X
			} else if ox >= b.Max.X {
				ox = b.Max.X - 1
			}
			if oy < b.Min.Y {
				oy = b.Min.Y
			} else if oy >= b.Max.Y {
				oy = b.Max.Y - 1
			}
			si := idx(n, ox, oy)
			di := idx(dst, x, y)
			copy(dst.Pix[di:di+4], n.Pix[si:si+4])
		}
	})
	return dst
}

// --------------------------------------------------------------------------
// Transpose / Transverse — diagonal flips (ImageMagick -transpose/-transverse)
// --------------------------------------------------------------------------

// Transpose reflects the image along the main diagonal (top-left to
// bottom-right), i.e. Output(x,y) = Input(y,x).  The output dimensions are
// the source's height × width.
func Transpose(src image.Image) *image.NRGBA {
	n := ToNRGBA(src)
	b := n.Rect
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, h, w))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			si := idx(n, x, y)
			dx := y - b.Min.Y
			dy := x - b.Min.X
			di := dy*dst.Stride + dx*4
			copy(dst.Pix[di:di+4], n.Pix[si:si+4])
		}
	}
	return dst
}

// Transverse reflects the image along the anti-diagonal (top-right to
// bottom-left), i.e. Output(x,y) = Input(h-1-y, w-1-x).  The output
// dimensions are the source's height × width.
func Transverse(src image.Image) *image.NRGBA {
	n := ToNRGBA(src)
	b := n.Rect
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, h, w))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			si := idx(n, x, y)
			dx := h - 1 - (y - b.Min.Y)
			dy := w - 1 - (x - b.Min.X)
			di := dy*dst.Stride + dx*4
			copy(dst.Pix[di:di+4], n.Pix[si:si+4])
		}
	}
	return dst
}

// --------------------------------------------------------------------------
// Shave — remove border pixels (ImageMagick -shave)
// --------------------------------------------------------------------------

// Shave removes shaveX pixels from the left and right edges and shaveY pixels
// from the top and bottom edges.  Equivalent to ImageMagick -shave WxH.
func Shave(src image.Image, shaveX, shaveY int) *image.NRGBA {
	b := src.Bounds()
	x0 := b.Min.X + shaveX
	y0 := b.Min.Y + shaveY
	x1 := b.Max.X - shaveX
	y1 := b.Max.Y - shaveY
	if x1 <= x0 || y1 <= y0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	return Crop(src, image.Rect(x0, y0, x1, y1))
}

// --------------------------------------------------------------------------
// OrderedDither — Bayer-matrix ordered dithering (ImageMagick -ordered-dither)
// --------------------------------------------------------------------------

// bayer4 is the 4×4 Bayer threshold matrix (values 0..15, normalised to
// 0..1 by dividing by 16).
var bayer4 = [4][4]float64{
	{0, 8, 2, 10},
	{12, 4, 14, 6},
	{3, 11, 1, 9},
	{15, 7, 13, 5},
}

// OrderedDither applies Bayer 4×4 ordered dithering, reducing each channel
// to levels distinct values.  levels=2 gives 1-bit per channel, levels=4
// gives 2-bit, etc.  Equivalent to ImageMagick -ordered-dither o4x4,N.
func OrderedDither(src image.Image, levels int) *image.NRGBA {
	if levels < 2 {
		levels = 2
	}
	n := ToNRGBA(src)
	b := n.Rect
	dst := image.NewNRGBA(b)
	step := 1.0 / float64(levels-1)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			threshold := (bayer4[(y-b.Min.Y)%4][(x-b.Min.X)%4]/16.0 - 0.5) * step
			i := idx(n, x, y)
			di := idx(dst, x, y)
			for c := 0; c < 3; c++ {
				v := float64(n.Pix[i+c])/255.0 + threshold
				v = math.Round(v/step) * step
				dst.Pix[di+c] = clamp8(int(v * 255))
			}
			dst.Pix[di+3] = n.Pix[i+3]
		}
	}
	return dst
}

// --------------------------------------------------------------------------
// SelectiveBlur — blur only smooth regions (ImageMagick -selective-blur)
// --------------------------------------------------------------------------

// SelectiveBlur blurs only pixels where the local gradient magnitude is below
// threshold (0..1).  sigma and radius control the Gaussian blur applied to
// smooth areas.  Equivalent to ImageMagick -selective-blur geometry,threshold.
func SelectiveBlur(src image.Image, sigma float64, threshold float64) *image.NRGBA {
	n := ToNRGBA(src)
	radius := int(math.Ceil(sigma * 3))
	if radius < 1 {
		radius = 1
	}
	blurred := GaussianBlur(n, sigma, radius)
	b := n.Rect
	dst := CloneNRGBA(n)

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			// Compute Sobel gradient magnitude
			var gx, gy float64
			sobelX := [3][3]float64{{-1, 0, 1}, {-2, 0, 2}, {-1, 0, 1}}
			sobelY := [3][3]float64{{-1, -2, -1}, {0, 0, 0}, {1, 2, 1}}
			for ky := -1; ky <= 1; ky++ {
				sy := y + ky
				if sy < b.Min.Y {
					sy = b.Min.Y
				} else if sy >= b.Max.Y {
					sy = b.Max.Y - 1
				}
				for kx := -1; kx <= 1; kx++ {
					sx := x + kx
					if sx < b.Min.X {
						sx = b.Min.X
					} else if sx >= b.Max.X {
						sx = b.Max.X - 1
					}
					p := idx(n, sx, sy)
					lum := (0.2126*float64(n.Pix[p]) + 0.7152*float64(n.Pix[p+1]) + 0.0722*float64(n.Pix[p+2])) / 255.0
					gx += lum * sobelX[ky+1][kx+1]
					gy += lum * sobelY[ky+1][kx+1]
				}
			}
			grad := math.Hypot(gx, gy) // ~0..1 for typical images
			if grad < threshold {
				// Replace with blurred value
				bi := idx(blurred, x, y)
				di := idx(dst, x, y)
				copy(dst.Pix[di:di+4], blurred.Pix[bi:bi+4])
			}
		}
	}
	return dst
}

// --------------------------------------------------------------------------
// AutoThreshold — Otsu automatic thresholding (ImageMagick -auto-threshold Otsu)
// --------------------------------------------------------------------------

// AutoThreshold converts the image to binary (black/white) using Otsu's
// method to automatically find the optimal threshold.  Equivalent to
// ImageMagick -auto-threshold Otsu.
func AutoThreshold(src image.Image) *image.NRGBA {
	n := ToNRGBA(src)
	b := n.Rect
	total := b.Dx() * b.Dy()
	if total == 0 {
		return CloneNRGBA(n)
	}

	// Build grayscale histogram
	var hist [256]int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := idx(n, x, y)
			lum := int(0.2126*float64(n.Pix[i]) + 0.7152*float64(n.Pix[i+1]) + 0.0722*float64(n.Pix[i+2]))
			if lum > 255 {
				lum = 255
			}
			hist[lum]++
		}
	}

	// Otsu's method
	var sumAll float64
	for i := 0; i < 256; i++ {
		sumAll += float64(i) * float64(hist[i])
	}
	var wB, sumB float64
	var maxVar float64
	threshold := 128
	for t := 0; t < 256; t++ {
		wB += float64(hist[t])
		if wB == 0 {
			continue
		}
		wF := float64(total) - wB
		if wF == 0 {
			break
		}
		sumB += float64(t) * float64(hist[t])
		mB := sumB / wB
		mF := (sumAll - sumB) / wF
		v := wB * wF * (mB - mF) * (mB - mF)
		if v > maxVar {
			maxVar = v
			threshold = t
		}
	}

	dst := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := idx(n, x, y)
			lum := int(0.2126*float64(n.Pix[i]) + 0.7152*float64(n.Pix[i+1]) + 0.0722*float64(n.Pix[i+2]))
			di := idx(dst, x, y)
			var v uint8
			if lum > threshold {
				v = 255
			}
			dst.Pix[di] = v
			dst.Pix[di+1] = v
			dst.Pix[di+2] = v
			dst.Pix[di+3] = n.Pix[i+3]
		}
	}
	return dst
}

// --------------------------------------------------------------------------
// AdaptiveBlur — edge-adaptive Gaussian blur (ImageMagick -adaptive-blur)
// --------------------------------------------------------------------------

// AdaptiveBlur blurs the image with a Gaussian kernel whose effective radius
// is proportional to the inverse local gradient magnitude.  Flat regions
// receive the full sigma blur; strong edges are preserved.  Equivalent to
// ImageMagick -adaptive-blur geometry.
func AdaptiveBlur(src image.Image, sigma float64) *image.NRGBA {
	return AdaptiveBlurCtx(context.Background(), src, sigma)
}

func AdaptiveBlurCtx(ctx context.Context, src image.Image, sigma float64) *image.NRGBA {
	if sigma <= 0 {
		sigma = 1.0
	}
	n := ToNRGBA(src)
	b := n.Rect
	radius := int(math.Ceil(sigma * 3))
	if radius < 1 {
		radius = 1
	}
	blurred := GaussianBlur(n, sigma, radius)
	dst := image.NewNRGBA(b)

	_ = parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			// Sobel gradient
			var gx, gy float64
			for ky := -1; ky <= 1; ky++ {
				sy := y + ky
				if sy < b.Min.Y {
					sy = b.Min.Y
				} else if sy >= b.Max.Y {
					sy = b.Max.Y - 1
				}
				for kx := -1; kx <= 1; kx++ {
					sx := x + kx
					if sx < b.Min.X {
						sx = b.Min.X
					} else if sx >= b.Max.X {
						sx = b.Max.X - 1
					}
					p := idx(n, sx, sy)
					lum := (0.2126*float64(n.Pix[p]) + 0.7152*float64(n.Pix[p+1]) + 0.0722*float64(n.Pix[p+2])) / 255.0
					kw := [3][3]float64{{-1, 0, 1}, {-2, 0, 2}, {-1, 0, 1}}
					kh := [3][3]float64{{-1, -2, -1}, {0, 0, 0}, {1, 2, 1}}
					gx += lum * kw[ky+1][kx+1]
					gy += lum * kh[ky+1][kx+1]
				}
			}
			grad := math.Hypot(gx, gy) // 0..~1
			// Blend original and blurred based on edge strength
			t := clampF01(grad * 4) // ramp: 0 → fully blurred, 0.25 → original
			ni := idx(n, x, y)
			bi := idx(blurred, x, y)
			di := idx(dst, x, y)
			for c := 0; c < 3; c++ {
				orig := float64(n.Pix[ni+c])
				blur := float64(blurred.Pix[bi+c])
				dst.Pix[di+c] = clamp8(int(blur*(1-t) + orig*t))
			}
			dst.Pix[di+3] = n.Pix[ni+3]
		}
	})
	return dst
}

// --------------------------------------------------------------------------
// AdaptiveSharpen — edge-adaptive sharpening (ImageMagick -adaptive-sharpen)
// --------------------------------------------------------------------------

// AdaptiveSharpen applies more sharpening to edge pixels and leaves flat
// regions largely untouched.  sigma controls the unsharp-mask kernel used for
// sharpening.  Equivalent to ImageMagick -adaptive-sharpen geometry.
func AdaptiveSharpen(src image.Image, sigma float64) *image.NRGBA {
	return AdaptiveSharpenCtx(context.Background(), src, sigma)
}

func AdaptiveSharpenCtx(ctx context.Context, src image.Image, sigma float64) *image.NRGBA {
	if sigma <= 0 {
		sigma = 1.0
	}
	n := ToNRGBA(src)
	b := n.Rect
	radius := int(math.Ceil(sigma * 3))
	if radius < 1 {
		radius = 1
	}
	sharpened := UnsharpMask(n, 1.5, sigma, radius)
	dst := image.NewNRGBA(b)

	_ = parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			var gx, gy float64
			for ky := -1; ky <= 1; ky++ {
				sy := y + ky
				if sy < b.Min.Y {
					sy = b.Min.Y
				} else if sy >= b.Max.Y {
					sy = b.Max.Y - 1
				}
				for kx := -1; kx <= 1; kx++ {
					sx := x + kx
					if sx < b.Min.X {
						sx = b.Min.X
					} else if sx >= b.Max.X {
						sx = b.Max.X - 1
					}
					p := idx(n, sx, sy)
					lum := (0.2126*float64(n.Pix[p]) + 0.7152*float64(n.Pix[p+1]) + 0.0722*float64(n.Pix[p+2])) / 255.0
					kw := [3][3]float64{{-1, 0, 1}, {-2, 0, 2}, {-1, 0, 1}}
					kh := [3][3]float64{{-1, -2, -1}, {0, 0, 0}, {1, 2, 1}}
					gx += lum * kw[ky+1][kx+1]
					gy += lum * kh[ky+1][kx+1]
				}
			}
			grad := math.Hypot(gx, gy)
			t := clampF01(grad * 4)
			ni := idx(n, x, y)
			si := idx(sharpened, x, y)
			di := idx(dst, x, y)
			for c := 0; c < 3; c++ {
				orig := float64(n.Pix[ni+c])
				sharp := float64(sharpened.Pix[si+c])
				dst.Pix[di+c] = clamp8(int(orig*(1-t) + sharp*t))
			}
			dst.Pix[di+3] = n.Pix[ni+3]
		}
	})
	return dst
}

// --------------------------------------------------------------------------
// MorphologyColor — dilate / erode / open / close on RGB images
// --------------------------------------------------------------------------

// MorphologyColor applies morphological operations (dilate, erode, open,
// close) to all colour channels.  mode is one of "dilate", "erode", "open",
// "close".  radius is the half-window size (1 = 3×3, 2 = 5×5, etc.).
// Equivalent to ImageMagick -morphology Dilate/Erode/Open/Close.
func MorphologyColor(src image.Image, radius int, mode string) *image.NRGBA {
	if radius < 1 {
		radius = 1
	}
	mode = toLowerASCII(mode)
	switch mode {
	case "open":
		return morphColorOnce(morphColorOnce(ToNRGBA(src), radius, "erode"), radius, "dilate")
	case "close":
		return morphColorOnce(morphColorOnce(ToNRGBA(src), radius, "dilate"), radius, "erode")
	default:
		return morphColorOnce(ToNRGBA(src), radius, mode)
	}
}

func morphColorOnce(n *image.NRGBA, radius int, mode string) *image.NRGBA {
	b := n.Rect
	dst := image.NewNRGBA(b)
	dilate := mode != "erode"
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			var bestR, bestG, bestB uint8
			if !dilate {
				bestR, bestG, bestB = 255, 255, 255
			}
			for dy := -radius; dy <= radius; dy++ {
				sy := y + dy
				if sy < b.Min.Y {
					sy = b.Min.Y
				} else if sy >= b.Max.Y {
					sy = b.Max.Y - 1
				}
				for dx := -radius; dx <= radius; dx++ {
					sx := x + dx
					if sx < b.Min.X {
						sx = b.Min.X
					} else if sx >= b.Max.X {
						sx = b.Max.X - 1
					}
					p := idx(n, sx, sy)
					if dilate {
						if n.Pix[p] > bestR {
							bestR = n.Pix[p]
						}
						if n.Pix[p+1] > bestG {
							bestG = n.Pix[p+1]
						}
						if n.Pix[p+2] > bestB {
							bestB = n.Pix[p+2]
						}
					} else {
						if n.Pix[p] < bestR {
							bestR = n.Pix[p]
						}
						if n.Pix[p+1] < bestG {
							bestG = n.Pix[p+1]
						}
						if n.Pix[p+2] < bestB {
							bestB = n.Pix[p+2]
						}
					}
				}
			}
			di := idx(dst, x, y)
			dst.Pix[di] = bestR
			dst.Pix[di+1] = bestG
			dst.Pix[di+2] = bestB
			ni := idx(n, x, y)
			dst.Pix[di+3] = n.Pix[ni+3]
		}
	}
	return dst
}

// toLowerASCII is a small helper to avoid importing strings in this file.
func toLowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// --------------------------------------------------------------------------
// Statistic — neighbourhood statistics (ImageMagick -statistic)
// --------------------------------------------------------------------------

// Statistic replaces each pixel with a neighbourhood statistic computed over a
// (2*radius+1)×(2*radius+1) window.  mode is one of "minimum", "maximum",
// "mean", "median".  Equivalent to ImageMagick -statistic Minimum/Maximum/Mean/Median.
func Statistic(src image.Image, radius int, mode string) *image.NRGBA {
	return StatisticCtx(context.Background(), src, radius, mode)
}

func StatisticCtx(ctx context.Context, src image.Image, radius int, mode string) *image.NRGBA {
	if radius < 1 {
		radius = 1
	}
	n := ToNRGBA(src)
	b := n.Rect
	dst := image.NewNRGBA(b)
	mode = toLowerASCII(mode)

	size := (2*radius + 1) * (2*radius + 1)
	_ = parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		rVals := make([]int, 0, size)
		gVals := make([]int, 0, size)
		bVals := make([]int, 0, size)
		for x := b.Min.X; x < b.Max.X; x++ {
			rVals = rVals[:0]
			gVals = gVals[:0]
			bVals = bVals[:0]
			for dy := -radius; dy <= radius; dy++ {
				sy := y + dy
				if sy < b.Min.Y {
					sy = b.Min.Y
				} else if sy >= b.Max.Y {
					sy = b.Max.Y - 1
				}
				for dx := -radius; dx <= radius; dx++ {
					sx := x + dx
					if sx < b.Min.X {
						sx = b.Min.X
					} else if sx >= b.Max.X {
						sx = b.Max.X - 1
					}
					p := idx(n, sx, sy)
					rVals = append(rVals, int(n.Pix[p]))
					gVals = append(gVals, int(n.Pix[p+1]))
					bVals = append(bVals, int(n.Pix[p+2]))
				}
			}
			var rv, gv, bv int
			switch mode {
			case "minimum", "min":
				rv, gv, bv = 255, 255, 255
				for _, v := range rVals {
					if v < rv {
						rv = v
					}
				}
				for _, v := range gVals {
					if v < gv {
						gv = v
					}
				}
				for _, v := range bVals {
					if v < bv {
						bv = v
					}
				}
			case "maximum", "max":
				rv, gv, bv = 0, 0, 0
				for _, v := range rVals {
					if v > rv {
						rv = v
					}
				}
				for _, v := range gVals {
					if v > gv {
						gv = v
					}
				}
				for _, v := range bVals {
					if v > bv {
						bv = v
					}
				}
			case "median":
				sort.Ints(rVals)
				sort.Ints(gVals)
				sort.Ints(bVals)
				mid := len(rVals) / 2
				rv, gv, bv = rVals[mid], gVals[mid], bVals[mid]
			default: // mean
				var sr, sg, sb int
				for _, v := range rVals {
					sr += v
				}
				for _, v := range gVals {
					sg += v
				}
				for _, v := range bVals {
					sb += v
				}
				cnt := len(rVals)
				rv = sr / cnt
				gv = sg / cnt
				bv = sb / cnt
			}
			di := idx(dst, x, y)
			ni := idx(n, x, y)
			dst.Pix[di] = clamp8(rv)
			dst.Pix[di+1] = clamp8(gv)
			dst.Pix[di+2] = clamp8(bv)
			dst.Pix[di+3] = n.Pix[ni+3]
		}
	})
	return dst
}

// --------------------------------------------------------------------------
// MeanShift — edge-preserving denoising (ImageMagick -mean-shift)
// --------------------------------------------------------------------------

// MeanShift denoises the image using a simplified mean-shift pass.  Each
// pixel is replaced by the mean of neighbours that are within colorDist (0..1)
// in colour distance and within radius pixels spatially.  Equivalent to
// ImageMagick -mean-shift WxH+colorDist%.
func MeanShift(src image.Image, radius int, colorDist float64) *image.NRGBA {
	return MeanShiftCtx(context.Background(), src, radius, colorDist)
}

func MeanShiftCtx(ctx context.Context, src image.Image, radius int, colorDist float64) *image.NRGBA {
	if radius < 1 {
		radius = 3
	}
	n := ToNRGBA(src)
	b := n.Rect
	dst := image.NewNRGBA(b)
	thresh := colorDist * 255.0 // convert to 0..255 space

	_ = parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			ci := idx(n, x, y)
			cr := float64(n.Pix[ci])
			cg := float64(n.Pix[ci+1])
			cb := float64(n.Pix[ci+2])

			var sumR, sumG, sumB float64
			var count float64
			for dy := -radius; dy <= radius; dy++ {
				sy := y + dy
				if sy < b.Min.Y || sy >= b.Max.Y {
					continue
				}
				for dx := -radius; dx <= radius; dx++ {
					sx := x + dx
					if sx < b.Min.X || sx >= b.Max.X {
						continue
					}
					ni := idx(n, sx, sy)
					nr := float64(n.Pix[ni])
					ng := float64(n.Pix[ni+1])
					nb := float64(n.Pix[ni+2])
					dist := math.Sqrt((nr-cr)*(nr-cr) + (ng-cg)*(ng-cg) + (nb-cb)*(nb-cb))
					if dist <= thresh {
						sumR += nr
						sumG += ng
						sumB += nb
						count++
					}
				}
			}
			di := idx(dst, x, y)
			if count == 0 {
				copy(dst.Pix[di:di+4], n.Pix[ci:ci+4])
			} else {
				dst.Pix[di] = clamp8(int(sumR / count))
				dst.Pix[di+1] = clamp8(int(sumG / count))
				dst.Pix[di+2] = clamp8(int(sumB / count))
				dst.Pix[di+3] = n.Pix[ci+3]
			}
		}
	})
	return dst
}

// --------------------------------------------------------------------------
// Kuwahara — true Kuwahara edge-preserving filter
// --------------------------------------------------------------------------

// Kuwahara applies the Kuwahara edge-preserving filter.  Each output pixel is
// the mean of the quadrant (of the four overlapping radius×radius regions
// centred on the pixel) that has the lowest variance.  Unlike OilPainting,
// this produces sharp edges with smooth inter-edge regions.
func Kuwahara(src image.Image, radius int) *image.NRGBA {
	return KuwaharaCtx(context.Background(), src, radius)
}

func KuwaharaCtx(ctx context.Context, src image.Image, radius int) *image.NRGBA {
	if radius < 1 {
		radius = 3
	}
	n := ToNRGBA(src)
	b := n.Rect
	dst := image.NewNRGBA(b)

	_ = parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			// Four quadrant windows (top-left, top-right, bottom-left, bottom-right)
			// Each is radius×radius pixels.
			type quad struct{ x0, y0, x1, y1 int }
			quads := [4]quad{
				{x - radius, y - radius, x, y},         // top-left
				{x, y - radius, x + radius, y},         // top-right
				{x - radius, y, x, y + radius},         // bottom-left
				{x, y, x + radius, y + radius},         // bottom-right
			}
			bestVar := math.MaxFloat64
			var bestR, bestG, bestB float64
			for _, q := range quads {
				var sumR, sumG, sumB float64
				var sumR2, sumG2, sumB2 float64
				var cnt float64
				for qy := q.y0; qy <= q.y1; qy++ {
					sy := qy
					if sy < b.Min.Y {
						sy = b.Min.Y
					} else if sy >= b.Max.Y {
						sy = b.Max.Y - 1
					}
					for qx := q.x0; qx <= q.x1; qx++ {
						sx := qx
						if sx < b.Min.X {
							sx = b.Min.X
						} else if sx >= b.Max.X {
							sx = b.Max.X - 1
						}
						p := idx(n, sx, sy)
						r := float64(n.Pix[p])
						g := float64(n.Pix[p+1])
						bv := float64(n.Pix[p+2])
						sumR += r
						sumG += g
						sumB += bv
						sumR2 += r * r
						sumG2 += g * g
						sumB2 += bv * bv
						cnt++
					}
				}
				if cnt == 0 {
					continue
				}
				mR := sumR / cnt
				mG := sumG / cnt
				mB := sumB / cnt
				vR := sumR2/cnt - mR*mR
				vG := sumG2/cnt - mG*mG
				vB := sumB2/cnt - mB*mB
				v := vR + vG + vB
				if v < bestVar {
					bestVar = v
					bestR, bestG, bestB = mR, mG, mB
				}
			}
			di := idx(dst, x, y)
			ni := idx(n, x, y)
			dst.Pix[di] = clamp8(int(bestR))
			dst.Pix[di+1] = clamp8(int(bestG))
			dst.Pix[di+2] = clamp8(int(bestB))
			dst.Pix[di+3] = n.Pix[ni+3]
		}
	})
	return dst
}

// --------------------------------------------------------------------------
// Pipeline methods for all new functions
// --------------------------------------------------------------------------

func (p *Pipeline) Normalize() *Pipeline {
	p.steps = append(p.steps, step{name: "normalize", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return Normalize(in) }})
	return p
}

func (p *Pipeline) AutoLevel() *Pipeline {
	p.steps = append(p.steps, step{name: "auto_level", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return AutoLevel(in) }})
	return p
}

func (p *Pipeline) Charcoal(radius int, sigma float64) *Pipeline {
	p.steps = append(p.steps, step{name: "charcoal", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return Charcoal(in, radius, sigma) }})
	return p
}

func (p *Pipeline) Sketch(sigma float64, angle float64) *Pipeline {
	p.steps = append(p.steps, step{name: "sketch", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return Sketch(in, sigma, angle) }})
	return p
}

func (p *Pipeline) SigmoidalContrast(sharpen bool, strength, midpoint float64) *Pipeline {
	p.steps = append(p.steps, step{name: "sigmoidal_contrast", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return SigmoidalContrast(in, sharpen, strength, midpoint)
	}})
	return p
}

func (p *Pipeline) Extent(w, h int, gravity string, bg color.NRGBA) *Pipeline {
	p.steps = append(p.steps, step{name: "extent", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return Extent(in, w, h, gravity, bg) }})
	return p
}

func (p *Pipeline) Roll(dx, dy int) *Pipeline {
	p.steps = append(p.steps, step{name: "roll", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return Roll(in, dx, dy) }})
	return p
}

func (p *Pipeline) Spread(radius int) *Pipeline {
	p.steps = append(p.steps, step{name: "spread", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return Spread(in, radius) }})
	return p
}

func (p *Pipeline) Transpose() *Pipeline {
	p.steps = append(p.steps, step{name: "transpose", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return Transpose(in) }})
	return p
}

func (p *Pipeline) Transverse() *Pipeline {
	p.steps = append(p.steps, step{name: "transverse", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return Transverse(in) }})
	return p
}

func (p *Pipeline) Shave(shaveX, shaveY int) *Pipeline {
	p.steps = append(p.steps, step{name: "shave", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return Shave(in, shaveX, shaveY) }})
	return p
}

func (p *Pipeline) OrderedDither(levels int) *Pipeline {
	p.steps = append(p.steps, step{name: "ordered_dither", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return OrderedDither(in, levels) }})
	return p
}

func (p *Pipeline) SelectiveBlur(sigma, threshold float64) *Pipeline {
	p.steps = append(p.steps, step{name: "selective_blur", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return SelectiveBlur(in, sigma, threshold) }})
	return p
}

func (p *Pipeline) AutoThreshold() *Pipeline {
	p.steps = append(p.steps, step{name: "auto_threshold", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return AutoThreshold(in) }})
	return p
}

func (p *Pipeline) AdaptiveBlur(sigma float64) *Pipeline {
	p.steps = append(p.steps, step{name: "adaptive_blur", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return AdaptiveBlur(in, sigma) }})
	return p
}

func (p *Pipeline) AdaptiveSharpen(sigma float64) *Pipeline {
	p.steps = append(p.steps, step{name: "adaptive_sharpen", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return AdaptiveSharpen(in, sigma) }})
	return p
}

func (p *Pipeline) MorphologyColor(radius int, mode string) *Pipeline {
	p.steps = append(p.steps, step{name: "morphology", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return MorphologyColor(in, radius, mode) }})
	return p
}

func (p *Pipeline) Statistic(radius int, mode string) *Pipeline {
	p.steps = append(p.steps, step{name: "statistic", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return Statistic(in, radius, mode) }})
	return p
}

func (p *Pipeline) MeanShift(radius int, colorDist float64) *Pipeline {
	p.steps = append(p.steps, step{name: "mean_shift", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return MeanShift(in, radius, colorDist) }})
	return p
}

func (p *Pipeline) Kuwahara(radius int) *Pipeline {
	p.steps = append(p.steps, step{name: "kuwahara", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return Kuwahara(in, radius) }})
	return p
}
