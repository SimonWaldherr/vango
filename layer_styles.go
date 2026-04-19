package vango

import (
	"context"
	"image"
	"image/color"
	"math"
)

// LayerStyle defines visual effects applied to a layer (like Photoshop layer styles).
type LayerStyle struct {
	DropShadow  *ShadowStyle
	InnerShadow *ShadowStyle
	OuterGlow   *GlowStyle
	InnerGlow   *GlowStyle
	Stroke      *StrokeStyle
	BevelEmboss *BevelStyle
}

// ShadowStyle configures drop shadow or inner shadow.
type ShadowStyle struct {
	Color    color.NRGBA
	Opacity  float64 // 0..1
	Angle    float64 // degrees
	Distance float64 // pixels
	Spread   float64 // 0..1 (choke)
	Size     float64 // blur radius
}

// GlowStyle configures outer or inner glow.
type GlowStyle struct {
	Color   color.NRGBA
	Opacity float64
	Size    float64 // blur radius
	Spread  float64 // 0..1
}

// StrokeStyle configures a layer stroke.
type StrokeStyle struct {
	Color    color.NRGBA
	Size     int    // pixels
	Position string // "outside", "inside", "center"
	Opacity  float64
}

// BevelStyle configures bevel and emboss.
type BevelStyle struct {
	Style    string  // "outer_bevel", "inner_bevel", "emboss", "pillow_emboss"
	Depth    float64 // 1..100
	Size     float64 // blur radius
	Angle    float64 // light angle in degrees
	Altitude float64 // light elevation 0..90
}

// ApplyLayerStyle renders all layer styles for a layer image,
// returning a new image that includes the styled effects.
func ApplyLayerStyle(src *image.NRGBA, style LayerStyle) *image.NRGBA {
	// Calculate expanded bounds for external effects
	expand := 0
	if style.DropShadow != nil {
		e := int(math.Ceil(style.DropShadow.Distance + style.DropShadow.Size*2))
		if e > expand {
			expand = e
		}
	}
	if style.OuterGlow != nil {
		e := int(math.Ceil(style.OuterGlow.Size * 2))
		if e > expand {
			expand = e
		}
	}
	if style.Stroke != nil && style.Stroke.Position != "inside" {
		e := style.Stroke.Size * 2
		if e > expand {
			expand = e
		}
	}

	outRect := image.Rect(
		src.Rect.Min.X-expand,
		src.Rect.Min.Y-expand,
		src.Rect.Max.X+expand,
		src.Rect.Max.Y+expand,
	)
	out := image.NewNRGBA(outRect)

	// 1. Drop Shadow (rendered behind content)
	if style.DropShadow != nil {
		applyDropShadow(out, src, style.DropShadow, expand)
	}

	// 2. Outer Glow (behind content)
	if style.OuterGlow != nil {
		applyOuterGlow(out, src, style.OuterGlow, expand)
	}

	// 3. Stroke (around content)
	if style.Stroke != nil {
		applyStroke(out, src, style.Stroke, expand)
	}

	// 4. Copy original content
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			si := idx(src, x, y)
			di := idx(out, x, y)
			sa := float64(src.Pix[si+3]) / 255.0
			out.Pix[di+0] = clamp8(int(lerp(float64(out.Pix[di+0]), float64(src.Pix[si+0]), sa)))
			out.Pix[di+1] = clamp8(int(lerp(float64(out.Pix[di+1]), float64(src.Pix[si+1]), sa)))
			out.Pix[di+2] = clamp8(int(lerp(float64(out.Pix[di+2]), float64(src.Pix[si+2]), sa)))
			out.Pix[di+3] = clamp8(int(math.Max(float64(out.Pix[di+3]), float64(src.Pix[si+3]))))
		}
	}

	// 5. Inner Shadow (on top of content, inside alpha boundary)
	if style.InnerShadow != nil {
		applyInnerShadow(out, src, style.InnerShadow, expand)
	}

	// 6. Inner Glow
	if style.InnerGlow != nil {
		applyInnerGlow(out, src, style.InnerGlow, expand)
	}

	// 7. Bevel/Emboss
	if style.BevelEmboss != nil {
		applyBevel(out, src, style.BevelEmboss, expand)
	}

	return out
}

// extractAlpha creates a grayscale image from the alpha channel.
func extractAlpha(src *image.NRGBA) *image.Gray {
	r := src.Rect
	g := image.NewGray(r)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			si := idx(src, x, y)
			g.SetGray(x, y, color.Gray{Y: src.Pix[si+3]})
		}
	}
	return g
}

func applyDropShadow(dst *image.NRGBA, src *image.NRGBA, s *ShadowStyle, expand int) {
	// Create shadow: alpha of source, shifted, blurred
	rad := s.Angle * math.Pi / 180
	offX := int(math.Round(math.Cos(rad) * s.Distance))
	offY := int(math.Round(math.Sin(rad) * s.Distance))

	// Create shadow alpha on dst-sized canvas
	shadowAlpha := image.NewNRGBA(dst.Rect)
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			sx, sy := x+offX, y+offY
			if sx >= dst.Rect.Min.X && sx < dst.Rect.Max.X && sy >= dst.Rect.Min.Y && sy < dst.Rect.Max.Y {
				si := idx(src, x, y)
				di := idx(shadowAlpha, sx, sy)
				a := float64(src.Pix[si+3]) / 255.0 * s.Opacity
				shadowAlpha.Pix[di+0] = s.Color.R
				shadowAlpha.Pix[di+1] = s.Color.G
				shadowAlpha.Pix[di+2] = s.Color.B
				shadowAlpha.Pix[di+3] = clamp8(int(a * 255))
			}
		}
	}

	// Blur the shadow
	if s.Size > 0 {
		shadowAlpha = GaussianBlurCtx(context.Background(), shadowAlpha, s.Size, 0)
	}

	// Composite shadow under content
	for y := dst.Rect.Min.Y; y < dst.Rect.Max.Y; y++ {
		for x := dst.Rect.Min.X; x < dst.Rect.Max.X; x++ {
			si := idx(shadowAlpha, x, y)
			sa := float64(shadowAlpha.Pix[si+3]) / 255.0
			if sa <= 0 {
				continue
			}
			di := idx(dst, x, y)
			da := float64(dst.Pix[di+3]) / 255.0
			outA := sa + da*(1-sa)
			if outA > 0 {
				dst.Pix[di+0] = clamp8(int((float64(shadowAlpha.Pix[si+0])*sa + float64(dst.Pix[di+0])*da*(1-sa)) / outA))
				dst.Pix[di+1] = clamp8(int((float64(shadowAlpha.Pix[si+1])*sa + float64(dst.Pix[di+1])*da*(1-sa)) / outA))
				dst.Pix[di+2] = clamp8(int((float64(shadowAlpha.Pix[si+2])*sa + float64(dst.Pix[di+2])*da*(1-sa)) / outA))
				dst.Pix[di+3] = clamp8(int(outA * 255))
			}
		}
	}
}

func applyOuterGlow(dst *image.NRGBA, src *image.NRGBA, g *GlowStyle, expand int) {
	// Create glow from alpha, blur it, composite behind content
	glow := image.NewNRGBA(dst.Rect)
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			si := idx(src, x, y)
			di := idx(glow, x, y)
			a := float64(src.Pix[si+3]) / 255.0 * g.Opacity
			glow.Pix[di+0] = g.Color.R
			glow.Pix[di+1] = g.Color.G
			glow.Pix[di+2] = g.Color.B
			glow.Pix[di+3] = clamp8(int(a * 255))
		}
	}
	if g.Size > 0 {
		glow = GaussianBlurCtx(context.Background(), glow, g.Size, 0)
	}

	for y := dst.Rect.Min.Y; y < dst.Rect.Max.Y; y++ {
		for x := dst.Rect.Min.X; x < dst.Rect.Max.X; x++ {
			gi := idx(glow, x, y)
			ga := float64(glow.Pix[gi+3]) / 255.0
			if ga <= 0 {
				continue
			}
			di := idx(dst, x, y)
			da := float64(dst.Pix[di+3]) / 255.0
			outA := ga + da*(1-ga)
			if outA > 0 {
				dst.Pix[di+0] = clamp8(int((float64(glow.Pix[gi+0])*ga + float64(dst.Pix[di+0])*da*(1-ga)) / outA))
				dst.Pix[di+1] = clamp8(int((float64(glow.Pix[gi+1])*ga + float64(dst.Pix[di+1])*da*(1-ga)) / outA))
				dst.Pix[di+2] = clamp8(int((float64(glow.Pix[gi+2])*ga + float64(dst.Pix[di+2])*da*(1-ga)) / outA))
				dst.Pix[di+3] = clamp8(int(outA * 255))
			}
		}
	}
}

func applyStroke(dst *image.NRGBA, src *image.NRGBA, s *StrokeStyle, expand int) {
	// Dilate alpha to create stroke region, then draw only where original alpha was 0
	alpha := extractAlpha(src)
	dilated := MorphologyGray(alpha, s.Size, "dilate")

	for y := dst.Rect.Min.Y; y < dst.Rect.Max.Y; y++ {
		for x := dst.Rect.Min.X; x < dst.Rect.Max.X; x++ {
			// Check if within dilated region but outside original (for "outside" position)
			origA := uint8(0)
			if x >= src.Rect.Min.X && x < src.Rect.Max.X && y >= src.Rect.Min.Y && y < src.Rect.Max.Y {
				origA = src.Pix[idx(src, x, y)+3]
			}
			dilA := uint8(0)
			if x >= dilated.Rect.Min.X && x < dilated.Rect.Max.X && y >= dilated.Rect.Min.Y && y < dilated.Rect.Max.Y {
				dilA = dilated.GrayAt(x, y).Y
			}

			var strokeA float64
			switch s.Position {
			case "outside":
				if dilA > 0 && origA == 0 {
					strokeA = float64(dilA) / 255.0
				}
			case "inside":
				if origA > 0 {
					// Check if near the edge
					eroded := uint8(0)
					for dy := -s.Size; dy <= s.Size; dy++ {
						for dx := -s.Size; dx <= s.Size; dx++ {
							nx, ny := x+dx, y+dy
							if nx >= src.Rect.Min.X && nx < src.Rect.Max.X && ny >= src.Rect.Min.Y && ny < src.Rect.Max.Y {
								continue
							}
							eroded = 255
						}
					}
					if eroded > 0 {
						strokeA = float64(origA) / 255.0
					}
				}
			default: // center
				if dilA > origA {
					strokeA = float64(dilA-origA) / 255.0
				}
				if origA > 0 {
					strokeA = math.Max(strokeA, float64(origA)/255.0*0.5)
				}
			}

			strokeA *= s.Opacity
			if strokeA <= 0 {
				continue
			}

			di := idx(dst, x, y)
			da := float64(dst.Pix[di+3]) / 255.0
			outA := strokeA + da*(1-strokeA)
			if outA > 0 {
				dst.Pix[di+0] = clamp8(int((float64(s.Color.R)*strokeA + float64(dst.Pix[di+0])*da*(1-strokeA)) / outA))
				dst.Pix[di+1] = clamp8(int((float64(s.Color.G)*strokeA + float64(dst.Pix[di+1])*da*(1-strokeA)) / outA))
				dst.Pix[di+2] = clamp8(int((float64(s.Color.B)*strokeA + float64(dst.Pix[di+2])*da*(1-strokeA)) / outA))
				dst.Pix[di+3] = clamp8(int(outA * 255))
			}
		}
	}
}

func applyInnerShadow(dst *image.NRGBA, src *image.NRGBA, s *ShadowStyle, expand int) {
	// Inner shadow: invert alpha, shift, blur, mask to original alpha
	rad := s.Angle * math.Pi / 180
	offX := int(math.Round(math.Cos(rad) * s.Distance))
	offY := int(math.Round(math.Sin(rad) * s.Distance))

	invAlpha := image.NewNRGBA(src.Rect)
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			si := idx(src, x, y)
			di := idx(invAlpha, x, y)
			invAlpha.Pix[di+3] = 255 - src.Pix[si+3]
		}
	}

	// shift
	shifted := image.NewNRGBA(src.Rect)
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			sx, sy := x-offX, y-offY
			di := idx(shifted, x, y)
			if sx >= invAlpha.Rect.Min.X && sx < invAlpha.Rect.Max.X && sy >= invAlpha.Rect.Min.Y && sy < invAlpha.Rect.Max.Y {
				si := idx(invAlpha, sx, sy)
				shifted.Pix[di+3] = invAlpha.Pix[si+3]
			}
		}
	}

	if s.Size > 0 {
		shifted = GaussianBlurCtx(context.Background(), shifted, s.Size, 0)
	}

	// Apply inner shadow only where original has alpha
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			si := idx(src, x, y)
			if src.Pix[si+3] == 0 {
				continue
			}
			shi := idx(shifted, x, y)
			shadowA := float64(shifted.Pix[shi+3]) / 255.0 * s.Opacity
			if shadowA <= 0 {
				continue
			}
			di := idx(dst, x, y)
			// Darken based on shadow
			dst.Pix[di+0] = clamp8(int(lerp(float64(dst.Pix[di+0]), float64(s.Color.R), shadowA)))
			dst.Pix[di+1] = clamp8(int(lerp(float64(dst.Pix[di+1]), float64(s.Color.G), shadowA)))
			dst.Pix[di+2] = clamp8(int(lerp(float64(dst.Pix[di+2]), float64(s.Color.B), shadowA)))
		}
	}
}

func applyInnerGlow(dst *image.NRGBA, src *image.NRGBA, g *GlowStyle, expand int) {
	// Create glow from inverted alpha edge, mask to original alpha
	invAlpha := image.NewNRGBA(src.Rect)
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			si := idx(src, x, y)
			di := idx(invAlpha, x, y)
			invAlpha.Pix[di+0] = g.Color.R
			invAlpha.Pix[di+1] = g.Color.G
			invAlpha.Pix[di+2] = g.Color.B
			invAlpha.Pix[di+3] = 255 - src.Pix[si+3]
		}
	}
	if g.Size > 0 {
		invAlpha = GaussianBlurCtx(context.Background(), invAlpha, g.Size, 0)
	}

	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			si := idx(src, x, y)
			if src.Pix[si+3] == 0 {
				continue
			}
			gi := idx(invAlpha, x, y)
			ga := float64(invAlpha.Pix[gi+3]) / 255.0 * g.Opacity
			if ga <= 0 {
				continue
			}
			di := idx(dst, x, y)
			dst.Pix[di+0] = clamp8(int(lerp(float64(dst.Pix[di+0]), float64(g.Color.R), ga)))
			dst.Pix[di+1] = clamp8(int(lerp(float64(dst.Pix[di+1]), float64(g.Color.G), ga)))
			dst.Pix[di+2] = clamp8(int(lerp(float64(dst.Pix[di+2]), float64(g.Color.B), ga)))
		}
	}
}

func applyBevel(dst *image.NRGBA, src *image.NRGBA, b *BevelStyle, expand int) {
	// Simple bevel: use alpha gradient to create highlights/shadows
	alpha := extractAlpha(src)
	blurred := image.NewGray(alpha.Rect)

	// Blur the alpha for edge detection
	if b.Size > 0 {
		blurredNRGBA := GaussianBlurCtx(context.Background(), ToNRGBA(alpha), b.Size, 0)
		for y := alpha.Rect.Min.Y; y < alpha.Rect.Max.Y; y++ {
			for x := alpha.Rect.Min.X; x < alpha.Rect.Max.X; x++ {
				bi := idx(blurredNRGBA, x, y)
				blurred.SetGray(x, y, color.Gray{Y: blurredNRGBA.Pix[bi+0]})
			}
		}
	} else {
		copy(blurred.Pix, alpha.Pix)
	}

	// Compute gradient direction for lighting
	rad := b.Angle * math.Pi / 180
	lightX := math.Cos(rad)
	lightY := math.Sin(rad)
	depth := b.Depth / 100.0

	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			si := idx(src, x, y)
			if src.Pix[si+3] == 0 {
				continue
			}

			// Compute gradient at this pixel
			gx, gy := 0.0, 0.0
			for _, off := range [][3]int{{-1, 0, -1}, {1, 0, 1}, {0, -1, -1}, {0, 1, 1}} {
				nx, ny := x+off[0], y+off[1]
				if nx >= blurred.Rect.Min.X && nx < blurred.Rect.Max.X && ny >= blurred.Rect.Min.Y && ny < blurred.Rect.Max.Y {
					v := float64(blurred.GrayAt(nx, ny).Y) / 255.0
					if off[0] != 0 {
						gx += v * float64(off[2])
					}
					if off[1] != 0 {
						gy += v * float64(off[2])
					}
				}
			}

			// Dot product with light direction
			dot := (gx*lightX + gy*lightY) * depth
			dot = clampF01(dot*0.5 + 0.5)

			di := idx(dst, x, y)
			if dot > 0.5 {
				// Highlight
				bright := (dot - 0.5) * 2
				dst.Pix[di+0] = clamp8(int(float64(dst.Pix[di+0]) + bright*80))
				dst.Pix[di+1] = clamp8(int(float64(dst.Pix[di+1]) + bright*80))
				dst.Pix[di+2] = clamp8(int(float64(dst.Pix[di+2]) + bright*80))
			} else {
				// Shadow
				dark := (0.5 - dot) * 2
				dst.Pix[di+0] = clamp8(int(float64(dst.Pix[di+0]) * (1 - dark*0.5)))
				dst.Pix[di+1] = clamp8(int(float64(dst.Pix[di+1]) * (1 - dark*0.5)))
				dst.Pix[di+2] = clamp8(int(float64(dst.Pix[di+2]) * (1 - dark*0.5)))
			}
		}
	}
}

// Layer method to set styles
func (l *Layer) SetStyle(style LayerStyle) {
	l.Style = &style
}
