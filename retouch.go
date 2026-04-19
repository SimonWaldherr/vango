package vango

import (
	"context"
	"image"
	"math"
)

// --------------------------------------------------------------------------
// Retouching Tools
// --------------------------------------------------------------------------

// Dodge lightens areas of the image, simulating the darkroom technique.
// amount 0..1 controls strength, midtone range controls tonal targeting.
func Dodge(src *image.NRGBA, amount float64, rangeType string) *image.NRGBA {
	return DodgeCtx(context.Background(), src, amount, rangeType)
}

func DodgeCtx(ctx context.Context, src *image.NRGBA, amount float64, rangeType string) *image.NRGBA {
	dst := CloneNRGBA(src)
	b := dst.Rect
	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := idx(dst, x, y)
			r, g, bb := float64(dst.Pix[i]), float64(dst.Pix[i+1]), float64(dst.Pix[i+2])
			lum := (r*0.299 + g*0.587 + bb*0.114) / 255.0
			factor := toneRangeFactor(lum, rangeType) * amount
			dst.Pix[i+0] = clamp8(int(r + (255-r)*factor))
			dst.Pix[i+1] = clamp8(int(g + (255-g)*factor))
			dst.Pix[i+2] = clamp8(int(bb + (255-bb)*factor))
		}
	})
	return dst
}

// Burn darkens areas of the image.
func Burn(src *image.NRGBA, amount float64, rangeType string) *image.NRGBA {
	return BurnCtx(context.Background(), src, amount, rangeType)
}

func BurnCtx(ctx context.Context, src *image.NRGBA, amount float64, rangeType string) *image.NRGBA {
	dst := CloneNRGBA(src)
	b := dst.Rect
	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := idx(dst, x, y)
			r, g, bb := float64(dst.Pix[i]), float64(dst.Pix[i+1]), float64(dst.Pix[i+2])
			lum := (r*0.299 + g*0.587 + bb*0.114) / 255.0
			factor := toneRangeFactor(lum, rangeType) * amount
			dst.Pix[i+0] = clamp8(int(r * (1 - factor)))
			dst.Pix[i+1] = clamp8(int(g * (1 - factor)))
			dst.Pix[i+2] = clamp8(int(bb * (1 - factor)))
		}
	})
	return dst
}

// toneRangeFactor returns how much the effect applies based on luminance and range.
func toneRangeFactor(lum float64, rangeType string) float64 {
	switch rangeType {
	case "shadows":
		if lum < 0.33 {
			return 1 - lum/0.33
		}
		return 0
	case "highlights":
		if lum > 0.67 {
			return (lum - 0.67) / 0.33
		}
		return 0
	default: // midtones
		if lum < 0.5 {
			return lum * 2
		}
		return (1 - lum) * 2
	}
}

// CloneStamp copies pixels from a source region to a destination region.
// srcX, srcY is the center of the source; dstX, dstY is where to paint.
// radius is the brush radius. feather softens edges (0=hard, 1=fully feathered).
func CloneStamp(img *image.NRGBA, srcX, srcY, dstX, dstY, radius int, feather float64) {
	b := img.Rect
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			dist := math.Sqrt(float64(dx*dx + dy*dy))
			if dist > float64(radius) {
				continue
			}

			sx, sy := srcX+dx, srcY+dy
			tx, ty := dstX+dx, dstY+dy

			if sx < b.Min.X || sx >= b.Max.X || sy < b.Min.Y || sy >= b.Max.Y {
				continue
			}
			if tx < b.Min.X || tx >= b.Max.X || ty < b.Min.Y || ty >= b.Max.Y {
				continue
			}

			si := idx(img, sx, sy)
			di := idx(img, tx, ty)

			alpha := 1.0
			if feather > 0 {
				edge := 1 - dist/float64(radius)
				if edge < feather {
					alpha = edge / feather
				}
			}

			img.Pix[di+0] = clamp8(int(lerp(float64(img.Pix[di+0]), float64(img.Pix[si+0]), alpha)))
			img.Pix[di+1] = clamp8(int(lerp(float64(img.Pix[di+1]), float64(img.Pix[si+1]), alpha)))
			img.Pix[di+2] = clamp8(int(lerp(float64(img.Pix[di+2]), float64(img.Pix[si+2]), alpha)))
			img.Pix[di+3] = clamp8(int(lerp(float64(img.Pix[di+3]), float64(img.Pix[si+3]), alpha)))
		}
	}
}

// HealingBrush copies pixels from source but blends the color to match the destination luminance.
func HealingBrush(img *image.NRGBA, srcX, srcY, dstX, dstY, radius int) {
	b := img.Rect
	// First pass: compute average luminance difference
	var srcLumSum, dstLumSum float64
	var count float64
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy > radius*radius {
				continue
			}
			sx, sy := srcX+dx, srcY+dy
			tx, ty := dstX+dx, dstY+dy
			if sx < b.Min.X || sx >= b.Max.X || sy < b.Min.Y || sy >= b.Max.Y {
				continue
			}
			if tx < b.Min.X || tx >= b.Max.X || ty < b.Min.Y || ty >= b.Max.Y {
				continue
			}
			si := idx(img, sx, sy)
			di := idx(img, tx, ty)
			srcLumSum += float64(img.Pix[si])*0.299 + float64(img.Pix[si+1])*0.587 + float64(img.Pix[si+2])*0.114
			dstLumSum += float64(img.Pix[di])*0.299 + float64(img.Pix[di+1])*0.587 + float64(img.Pix[di+2])*0.114
			count++
		}
	}
	if count == 0 {
		return
	}
	lumDiff := (dstLumSum - srcLumSum) / count

	// Second pass: copy with luminance adjustment
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			dist := math.Sqrt(float64(dx*dx + dy*dy))
			if dist > float64(radius) {
				continue
			}
			sx, sy := srcX+dx, srcY+dy
			tx, ty := dstX+dx, dstY+dy
			if sx < b.Min.X || sx >= b.Max.X || sy < b.Min.Y || sy >= b.Max.Y {
				continue
			}
			if tx < b.Min.X || tx >= b.Max.X || ty < b.Min.Y || ty >= b.Max.Y {
				continue
			}
			si := idx(img, sx, sy)
			di := idx(img, tx, ty)

			alpha := 1 - dist/float64(radius)
			img.Pix[di+0] = clamp8(int(lerp(float64(img.Pix[di+0]), float64(img.Pix[si+0])+lumDiff, alpha)))
			img.Pix[di+1] = clamp8(int(lerp(float64(img.Pix[di+1]), float64(img.Pix[si+1])+lumDiff, alpha)))
			img.Pix[di+2] = clamp8(int(lerp(float64(img.Pix[di+2]), float64(img.Pix[si+2])+lumDiff, alpha)))
		}
	}
}

// RedEyeRemoval removes red-eye by desaturating red-dominant pixels in a circular region.
func RedEyeRemoval(img *image.NRGBA, cx, cy, radius int) {
	b := img.Rect
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy > radius*radius {
				continue
			}
			x, y := cx+dx, cy+dy
			if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
				continue
			}
			i := idx(img, x, y)
			r, g, bb := float64(img.Pix[i]), float64(img.Pix[i+1]), float64(img.Pix[i+2])
			// Check if pixel is "red"
			avg := (g + bb) / 2
			if r > avg*1.5 && r > 80 {
				// Replace red with average of green/blue
				img.Pix[i+0] = clamp8(int(avg))
			}
		}
	}
}

// --------------------------------------------------------------------------
// Pipeline methods
// --------------------------------------------------------------------------

func (p *Pipeline) Dodge(amount float64, rangeType string) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return DodgeCtx(ctx, img, amount, rangeType)
	}})
	return p
}

func (p *Pipeline) Burn(amount float64, rangeType string) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return BurnCtx(ctx, img, amount, rangeType)
	}})
	return p
}
