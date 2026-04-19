package vango

import (
	"context"
	"image"
	"math"
)

// --------------------------------------------------------------------------
// Pro Adjustments
// --------------------------------------------------------------------------

// Vibrance applies a smart saturation that protects already-saturated colors and skin tones.
func Vibrance(src *image.NRGBA, amount float64) *image.NRGBA {
	return VibranceCtx(context.Background(), src, amount)
}

func VibranceCtx(ctx context.Context, src *image.NRGBA, amount float64) *image.NRGBA {
	dst := CloneNRGBA(src)
	b := dst.Rect
	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := idx(dst, x, y)
			r, g, bl := float64(dst.Pix[i])/255.0, float64(dst.Pix[i+1])/255.0, float64(dst.Pix[i+2])/255.0

			maxC := math.Max(r, math.Max(g, bl))
			minC := math.Min(r, math.Min(g, bl))
			sat := 0.0
			if maxC > 0 {
				sat = (maxC - minC) / maxC
			}

			// Less saturation = more effect (inverse relationship)
			factor := amount * (1 - sat) * 0.5
			avg := (r + g + bl) / 3.0
			dst.Pix[i+0] = clamp8(int((r + (r-avg)*factor) * 255))
			dst.Pix[i+1] = clamp8(int((g + (g-avg)*factor) * 255))
			dst.Pix[i+2] = clamp8(int((bl + (bl-avg)*factor) * 255))
		}
	})
	return dst
}

// Dehaze removes haze by estimating atmospheric light and transmission.
func Dehaze(src *image.NRGBA, strength float64) *image.NRGBA {
	return DehazeCtx(context.Background(), src, strength)
}

func DehazeCtx(ctx context.Context, src *image.NRGBA, strength float64) *image.NRGBA {
	b := src.Rect
	w, h := b.Dx(), b.Dy()

	// Estimate dark channel (minimum of RGB in a patch)
	patchSize := 7
	half := patchSize / 2
	dark := make([]float64, w*h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			minVal := 255.0
			for py := -half; py <= half; py++ {
				for px := -half; px <= half; px++ {
					sx := x + px + b.Min.X
					sy := y + py + b.Min.Y
					if sx < b.Min.X {
						sx = b.Min.X
					}
					if sx >= b.Max.X {
						sx = b.Max.X - 1
					}
					if sy < b.Min.Y {
						sy = b.Min.Y
					}
					if sy >= b.Max.Y {
						sy = b.Max.Y - 1
					}
					si := idx(src, sx, sy)
					v := math.Min(float64(src.Pix[si]), math.Min(float64(src.Pix[si+1]), float64(src.Pix[si+2])))
					if v < minVal {
						minVal = v
					}
				}
			}
			dark[y*w+x] = minVal / 255.0
		}
	}

	// Estimate atmospheric light (brightest pixel in dark channel top 0.1%)
	atmoR, atmoG, atmoB := 220.0, 220.0, 220.0 // default
	type dp struct {
		val  float64
		x, y int
	}
	n := w * h / 1000
	if n < 1 {
		n = 1
	}
	var best dp
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := dark[y*w+x]
			if v > best.val {
				best = dp{v, x, y}
			}
		}
	}
	si := idx(src, best.x+b.Min.X, best.y+b.Min.Y)
	atmoR = float64(src.Pix[si])
	atmoG = float64(src.Pix[si+1])
	atmoB = float64(src.Pix[si+2])

	// Dehaze
	dst := CloneNRGBA(src)
	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			ly, lx := y-b.Min.Y, x-b.Min.X
			t := 1.0 - strength*dark[ly*w+lx]
			if t < 0.1 {
				t = 0.1
			}
			i := idx(dst, x, y)
			r := (float64(dst.Pix[i]) - atmoR*(1-t)) / t
			g := (float64(dst.Pix[i+1]) - atmoG*(1-t)) / t
			bl := (float64(dst.Pix[i+2]) - atmoB*(1-t)) / t
			dst.Pix[i+0] = clamp8(int(r))
			dst.Pix[i+1] = clamp8(int(g))
			dst.Pix[i+2] = clamp8(int(bl))
		}
	})
	return dst
}

// ShadowHighlight recovers detail in shadows and highlights.
// shadowAmount > 0 brightens shadows; highlightAmount > 0 darkens highlights.
func ShadowHighlight(src *image.NRGBA, shadowAmount, highlightAmount float64) *image.NRGBA {
	return ShadowHighlightCtx(context.Background(), src, shadowAmount, highlightAmount)
}

func ShadowHighlightCtx(ctx context.Context, src *image.NRGBA, shadowAmount, highlightAmount float64) *image.NRGBA {
	dst := CloneNRGBA(src)
	b := dst.Rect
	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := idx(dst, x, y)
			r, g, bl := float64(dst.Pix[i])/255.0, float64(dst.Pix[i+1])/255.0, float64(dst.Pix[i+2])/255.0
			lum := r*0.299 + g*0.587 + bl*0.114

			adj := 0.0
			if lum < 0.5 {
				// Shadow region
				w := (0.5 - lum) / 0.5
				adj = shadowAmount * w * 0.5
			} else {
				// Highlight region
				w := (lum - 0.5) / 0.5
				adj = -highlightAmount * w * 0.5
			}

			dst.Pix[i+0] = clamp8(int((r + adj) * 255))
			dst.Pix[i+1] = clamp8(int((g + adj) * 255))
			dst.Pix[i+2] = clamp8(int((bl + adj) * 255))
		}
	})
	return dst
}

// FrequencySeparation splits an image into low-frequency (color) and high-frequency (detail) layers.
// Returns (lowFreq, highFreq).
func FrequencySeparation(src *image.NRGBA, blurRadius float64) (*image.NRGBA, *image.NRGBA) {
	return FrequencySeparationCtx(context.Background(), src, blurRadius)
}

func FrequencySeparationCtx(ctx context.Context, src *image.NRGBA, blurRadius float64) (*image.NRGBA, *image.NRGBA) {
	low := GaussianBlurCtx(ctx, src, blurRadius, 0)
	high := image.NewNRGBA(src.Rect)
	b := src.Rect

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			si := idx(src, x, y)
			li := idx(low, x, y)
			hi := idx(high, x, y)
			// High = source - low + 128 (linear light blend math)
			high.Pix[hi+0] = clamp8(int(src.Pix[si+0]) - int(low.Pix[li+0]) + 128)
			high.Pix[hi+1] = clamp8(int(src.Pix[si+1]) - int(low.Pix[li+1]) + 128)
			high.Pix[hi+2] = clamp8(int(src.Pix[si+2]) - int(low.Pix[li+2]) + 128)
			high.Pix[hi+3] = src.Pix[si+3]
		}
	}
	return low, high
}

// --------------------------------------------------------------------------
// Per-Channel Curves
// --------------------------------------------------------------------------

// ChannelCurves applies separate curves to R, G, B channels.
// Each curve is a slice of CurvePoints (defined in effects_advanced.go).
// Pass nil for a channel to leave it unchanged.
func ChannelCurves(src *image.NRGBA, curveR, curveG, curveB []CurvePoint) *image.NRGBA {
	return ChannelCurvesCtx(context.Background(), src, curveR, curveG, curveB)
}

func ChannelCurvesCtx(ctx context.Context, src *image.NRGBA, curveR, curveG, curveB []CurvePoint) *image.NRGBA {
	// Build LUTs
	lutR := buildCurveLUT(curveR)
	lutG := buildCurveLUT(curveG)
	lutB := buildCurveLUT(curveB)

	dst := CloneNRGBA(src)
	b := dst.Rect
	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := idx(dst, x, y)
			if lutR != nil {
				dst.Pix[i+0] = lutR[dst.Pix[i+0]]
			}
			if lutG != nil {
				dst.Pix[i+1] = lutG[dst.Pix[i+1]]
			}
			if lutB != nil {
				dst.Pix[i+2] = lutB[dst.Pix[i+2]]
			}
		}
	})
	return dst
}

func buildCurveLUT(points []CurvePoint) []uint8 {
	if len(points) < 2 {
		return nil
	}
	lut := make([]uint8, 256)
	for i := 0; i < 256; i++ {
		t := float64(i) / 255.0
		// Linear interpolation between curve points
		val := t
		for j := 0; j < len(points)-1; j++ {
			if t >= points[j].In && t <= points[j+1].In {
				seg := points[j+1].In - points[j].In
				if seg > 0 {
					f := (t - points[j].In) / seg
					val = lerp(points[j].Out, points[j+1].Out, f)
				} else {
					val = points[j].Out
				}
				break
			}
		}
		lut[i] = clamp8(int(val*255.0 + 0.5))
	}
	return lut
}

// --------------------------------------------------------------------------
// Blend-If (Conditional blending based on luminosity)
// --------------------------------------------------------------------------

// BlendIf composites top onto base, but only where luminosity of the specified
// layer falls within the given range.
// layer: "this" uses top layer's luminosity, "underlying" uses base layer's.
// blackPoint, whitePoint: 0..255 range of luminosity where blending is full.
// feather: pixels of transition at edges.
func BlendIf(base, top *image.NRGBA, layer string, blackPoint, whitePoint uint8, feather int) *image.NRGBA {
	dst := CloneNRGBA(base)
	b := dst.Rect
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			bi := idx(base, x, y)
			ti := idx(top, x, y)
			di := idx(dst, x, y)

			var lum float64
			switch layer {
			case "this":
				lum = float64(top.Pix[ti])*0.299 + float64(top.Pix[ti+1])*0.587 + float64(top.Pix[ti+2])*0.114
			default: // "underlying"
				lum = float64(base.Pix[bi])*0.299 + float64(base.Pix[bi+1])*0.587 + float64(base.Pix[bi+2])*0.114
			}

			// Calculate blend factor
			bp := float64(blackPoint)
			wp := float64(whitePoint)
			f := float64(feather)
			factor := 1.0

			if lum < bp {
				if f > 0 && lum >= bp-f {
					factor = (lum - (bp - f)) / f
				} else {
					factor = 0
				}
			} else if lum > wp {
				if f > 0 && lum <= wp+f {
					factor = ((wp + f) - lum) / f
				} else {
					factor = 0
				}
			}

			factor = clampF01(factor)
			topA := float64(top.Pix[ti+3]) / 255.0 * factor
			dst.Pix[di+0] = clamp8(int(lerp(float64(base.Pix[bi+0]), float64(top.Pix[ti+0]), topA)))
			dst.Pix[di+1] = clamp8(int(lerp(float64(base.Pix[bi+1]), float64(top.Pix[ti+1]), topA)))
			dst.Pix[di+2] = clamp8(int(lerp(float64(base.Pix[bi+2]), float64(top.Pix[ti+2]), topA)))
			dst.Pix[di+3] = clamp8(int(math.Max(float64(base.Pix[bi+3]), float64(top.Pix[ti+3])*factor)))
		}
	}
	return dst
}

// --------------------------------------------------------------------------
// Pipeline methods
// --------------------------------------------------------------------------

func (p *Pipeline) Vibrance(amount float64) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return VibranceCtx(ctx, img, amount)
	}})
	return p
}

func (p *Pipeline) Dehaze(strength float64) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return DehazeCtx(ctx, img, strength)
	}})
	return p
}

func (p *Pipeline) ShadowHighlight(shadowAmount, highlightAmount float64) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return ShadowHighlightCtx(ctx, img, shadowAmount, highlightAmount)
	}})
	return p
}

func (p *Pipeline) ChannelCurves(curveR, curveG, curveB []CurvePoint) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return ChannelCurvesCtx(ctx, img, curveR, curveG, curveB)
	}})
	return p
}
