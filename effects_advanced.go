package vango

import (
	"context"
	"image"
	"image/color"
	"math"
	"math/rand"
)

// --------------------------------------------------------------------------
// Levels: remap black/white points and midtone gamma per-channel or master
// --------------------------------------------------------------------------

// Levels adjusts input levels. inBlack/inWhite (0..255), gamma (midtone, 1=linear),
// outBlack/outWhite (0..255).
func Levels(src image.Image, inBlack, inWhite float64, gamma float64, outBlack, outWhite float64) *image.NRGBA {
	n := ToNRGBA(src)
	if gamma <= 0 {
		gamma = 1
	}
	inRange := inWhite - inBlack
	if inRange <= 0 {
		inRange = 1
	}
	outRange := outWhite - outBlack
	var table [256]uint8
	for i := 0; i < 256; i++ {
		v := (float64(i) - inBlack) / inRange
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		v = math.Pow(v, 1.0/gamma)
		v = outBlack + v*outRange
		table[i] = clamp8(int(v + 0.5))
	}
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), 0, r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			out.Pix[i+0] = table[n.Pix[i+0]]
			out.Pix[i+1] = table[n.Pix[i+1]]
			out.Pix[i+2] = table[n.Pix[i+2]]
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Curves: apply a tone curve defined by control points (simple piecewise linear)
// --------------------------------------------------------------------------

// CurvePoint defines a control point for a tone curve.
type CurvePoint struct {
	In  float64 // 0..1
	Out float64 // 0..1
}

// ApplyCurve maps pixel values through a curve defined by sorted control points.
func ApplyCurve(src image.Image, points []CurvePoint) *image.NRGBA {
	if len(points) < 2 {
		return ToNRGBA(src)
	}
	var table [256]uint8
	for i := 0; i < 256; i++ {
		t := float64(i) / 255.0
		// find segment
		v := points[len(points)-1].Out
		for j := 0; j < len(points)-1; j++ {
			if t >= points[j].In && t <= points[j+1].In {
				seg := points[j+1].In - points[j].In
				if seg > 0 {
					f := (t - points[j].In) / seg
					v = points[j].Out + f*(points[j+1].Out-points[j].Out)
				} else {
					v = points[j].Out
				}
				break
			}
		}
		table[i] = clamp8(int(clampF01(v)*255 + 0.5))
	}
	n := ToNRGBA(src)
	r := n.Rect
	out := image.NewNRGBA(r)
	_ = parallelRows(context.Background(), 0, r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(n, x, y)
			out.Pix[i+0] = table[n.Pix[i+0]]
			out.Pix[i+1] = table[n.Pix[i+1]]
			out.Pix[i+2] = table[n.Pix[i+2]]
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Channel Mixer
// --------------------------------------------------------------------------

// ChannelMix remaps output channels: outR = rr*R + rg*G + rb*B, etc.
func ChannelMix(src image.Image, rr, rg, rb, gr, gg, gb, br, bg, bb float64) *image.NRGBA {
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := idx(n, x, y)
			rf, gf, bf := float64(n.Pix[i+0]), float64(n.Pix[i+1]), float64(n.Pix[i+2])
			out.Pix[i+0] = clamp8(int(rr*rf + rg*gf + rb*bf + 0.5))
			out.Pix[i+1] = clamp8(int(gr*rf + gg*gf + gb*bf + 0.5))
			out.Pix[i+2] = clamp8(int(br*rf + bg*gf + bb*bf + 0.5))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Color Balance: shift shadows/midtones/highlights
// --------------------------------------------------------------------------

// ColorBalance adjusts color balance for shadows, midtones, highlights.
// Each parameter is in range -1..1 (cyan/red, magenta/green, yellow/blue).
func ColorBalance(src image.Image, shadowR, shadowG, shadowB, midR, midG, midB, highR, highG, highB float64) *image.NRGBA {
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := idx(n, x, y)
			rf := float64(n.Pix[i+0]) / 255.0
			gf := float64(n.Pix[i+1]) / 255.0
			bf := float64(n.Pix[i+2]) / 255.0

			lum := 0.2126*rf + 0.7152*gf + 0.0722*bf

			// tonal masks
			shadowW := clampF01(1.0 - lum*3)
			highW := clampF01(lum*3 - 2)
			midW := 1.0 - shadowW - highW
			if midW < 0 {
				midW = 0
			}

			rf += shadowR*shadowW*0.3 + midR*midW*0.3 + highR*highW*0.3
			gf += shadowG*shadowW*0.3 + midG*midW*0.3 + highG*highW*0.3
			bf += shadowB*shadowW*0.3 + midB*midW*0.3 + highB*highW*0.3

			out.Pix[i+0] = clamp8(int(clampF01(rf)*255 + 0.5))
			out.Pix[i+1] = clamp8(int(clampF01(gf)*255 + 0.5))
			out.Pix[i+2] = clamp8(int(clampF01(bf)*255 + 0.5))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// HSL Selective Color: adjust hue/saturation/lightness of a specific color range
// --------------------------------------------------------------------------

// HSLSelective adjusts saturation and lightness of pixels whose hue is near targetHue.
// targetHue in degrees (0..360), hueRange is ± tolerance, satFactor multiplies saturation,
// lightDelta shifts lightness.
func HSLSelective(src image.Image, targetHue, hueRange, satFactor, lightDelta float64) *image.NRGBA {
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	target := math.Mod(targetHue/360.0, 1.0)
	hRange := hueRange / 360.0
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := idx(n, x, y)
			rf := float64(n.Pix[i+0]) / 255.0
			gf := float64(n.Pix[i+1]) / 255.0
			bf := float64(n.Pix[i+2]) / 255.0
			h, s, l := rgbToHSL(rf, gf, bf)

			// distance in hue circle
			dist := math.Abs(h - target)
			if dist > 0.5 {
				dist = 1 - dist
			}
			if dist < hRange && s > 0.05 {
				// smooth falloff
				w := 1.0 - dist/hRange
				w = w * w
				s = clampF01(s * (1 + (satFactor-1)*w))
				l = clampF01(l + lightDelta*w)
			}
			rr, gg, bb := hslToRGB(h, s, l)
			out.Pix[i+0] = clamp8(int(rr*255 + 0.5))
			out.Pix[i+1] = clamp8(int(gg*255 + 0.5))
			out.Pix[i+2] = clamp8(int(bb*255 + 0.5))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Gradient Map: maps luminance to a gradient of colors
// --------------------------------------------------------------------------

// GradientStop defines a color at a position (0..1).
type GradientStop struct {
	Pos   float64
	Color color.NRGBA
}

// GradientMap maps pixel luminance to a gradient defined by sorted stops.
func GradientMap(src image.Image, stops []GradientStop) *image.NRGBA {
	if len(stops) < 2 {
		return ToNRGBA(src)
	}
	// build LUT
	var lut [256][3]uint8
	for i := 0; i < 256; i++ {
		t := float64(i) / 255.0
		// find segment
		var r, g, b float64
		if t <= stops[0].Pos {
			r, g, b = float64(stops[0].Color.R), float64(stops[0].Color.G), float64(stops[0].Color.B)
		} else if t >= stops[len(stops)-1].Pos {
			last := stops[len(stops)-1].Color
			r, g, b = float64(last.R), float64(last.G), float64(last.B)
		} else {
			for j := 0; j < len(stops)-1; j++ {
				if t >= stops[j].Pos && t <= stops[j+1].Pos {
					seg := stops[j+1].Pos - stops[j].Pos
					f := 0.0
					if seg > 0 {
						f = (t - stops[j].Pos) / seg
					}
					r = lerp(float64(stops[j].Color.R), float64(stops[j+1].Color.R), f)
					g = lerp(float64(stops[j].Color.G), float64(stops[j+1].Color.G), f)
					b = lerp(float64(stops[j].Color.B), float64(stops[j+1].Color.B), f)
					break
				}
			}
		}
		lut[i] = [3]uint8{clamp8(int(r + 0.5)), clamp8(int(g + 0.5)), clamp8(int(b + 0.5))}
	}
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := idx(n, x, y)
			lum := uint8(0.2126*float64(n.Pix[i+0]) + 0.7152*float64(n.Pix[i+1]) + 0.0722*float64(n.Pix[i+2]) + 0.5)
			c := lut[lum]
			out.Pix[i+0] = c[0]
			out.Pix[i+1] = c[1]
			out.Pix[i+2] = c[2]
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Motion Blur
// --------------------------------------------------------------------------

// MotionBlur applies linear motion blur at a given angle (degrees) and distance (pixels).
func MotionBlur(src image.Image, angle float64, distance int) *image.NRGBA {
	if distance < 1 {
		return ToNRGBA(src)
	}
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	rad := angle * math.Pi / 180.0
	dx := math.Cos(rad)
	dy := math.Sin(rad)
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			var rr, gg, bb, aa float64
			for k := 0; k < distance; k++ {
				sx := int(math.Round(float64(x) + dx*float64(k-distance/2)))
				sy := int(math.Round(float64(y) + dy*float64(k-distance/2)))
				if sx < rect.Min.X {
					sx = rect.Min.X
				} else if sx >= rect.Max.X {
					sx = rect.Max.X - 1
				}
				if sy < rect.Min.Y {
					sy = rect.Min.Y
				} else if sy >= rect.Max.Y {
					sy = rect.Max.Y - 1
				}
				p := idx(n, sx, sy)
				rr += float64(n.Pix[p+0])
				gg += float64(n.Pix[p+1])
				bb += float64(n.Pix[p+2])
				aa += float64(n.Pix[p+3])
			}
			inv := 1.0 / float64(distance)
			i := idx(out, x, y)
			out.Pix[i+0] = clamp8(int(rr*inv + 0.5))
			out.Pix[i+1] = clamp8(int(gg*inv + 0.5))
			out.Pix[i+2] = clamp8(int(bb*inv + 0.5))
			out.Pix[i+3] = clamp8(int(aa*inv + 0.5))
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Radial Blur (zoom blur)
// --------------------------------------------------------------------------

// RadialBlur applies a zoom/radial blur centered at (cx,cy) with given strength.
func RadialBlur(src image.Image, cx, cy float64, strength float64, samples int) *image.NRGBA {
	if samples < 2 {
		samples = 10
	}
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			var rr, gg, bb, aa float64
			for s := 0; s < samples; s++ {
				t := (float64(s)/float64(samples-1) - 0.5) * strength
				sx := int(math.Round(cx + (float64(x)-cx)*(1+t)))
				sy := int(math.Round(cy + (float64(y)-cy)*(1+t)))
				if sx < rect.Min.X {
					sx = rect.Min.X
				} else if sx >= rect.Max.X {
					sx = rect.Max.X - 1
				}
				if sy < rect.Min.Y {
					sy = rect.Min.Y
				} else if sy >= rect.Max.Y {
					sy = rect.Max.Y - 1
				}
				p := idx(n, sx, sy)
				rr += float64(n.Pix[p+0])
				gg += float64(n.Pix[p+1])
				bb += float64(n.Pix[p+2])
				aa += float64(n.Pix[p+3])
			}
			inv := 1.0 / float64(samples)
			i := idx(out, x, y)
			out.Pix[i+0] = clamp8(int(rr*inv + 0.5))
			out.Pix[i+1] = clamp8(int(gg*inv + 0.5))
			out.Pix[i+2] = clamp8(int(bb*inv + 0.5))
			out.Pix[i+3] = clamp8(int(aa*inv + 0.5))
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Glow (bloom) effect
// --------------------------------------------------------------------------

// Glow adds a soft glow by blending a blurred version using Screen mode.
func Glow(src image.Image, sigma float64, intensity float64) *image.NRGBA {
	n := ToNRGBA(src)
	blurred := GaussianBlur(n, sigma, 0)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := idx(n, x, y)
			j := idx(blurred, x, y)
			for c := 0; c < 3; c++ {
				a := float64(n.Pix[i+c]) / 255.0
				b := float64(blurred.Pix[j+c]) / 255.0 * intensity
				// screen blend
				v := 1 - (1-a)*(1-b)
				out.Pix[i+c] = clamp8(int(clampF01(v)*255 + 0.5))
			}
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Halftone effect
// --------------------------------------------------------------------------

// Halftone renders the image as circular dots of varying size.
func Halftone(src image.Image, dotSize int) *image.NRGBA {
	if dotSize < 2 {
		dotSize = 4
	}
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	// fill white
	for i := range out.Pix {
		out.Pix[i] = 255
	}

	half := float64(dotSize) / 2.0
	for by := rect.Min.Y; by < rect.Max.Y; by += dotSize {
		for bx := rect.Min.X; bx < rect.Max.X; bx += dotSize {
			// average luminance of block
			var lum float64
			var cnt int
			for dy := 0; dy < dotSize && by+dy < rect.Max.Y; dy++ {
				for dx := 0; dx < dotSize && bx+dx < rect.Max.X; dx++ {
					p := idx(n, bx+dx, by+dy)
					l := 0.2126*float64(n.Pix[p+0]) + 0.7152*float64(n.Pix[p+1]) + 0.0722*float64(n.Pix[p+2])
					lum += l
					cnt++
				}
			}
			if cnt == 0 {
				continue
			}
			avgLum := lum / float64(cnt) / 255.0
			// dot radius proportional to darkness
			radius := half * (1 - avgLum)
			r2 := radius * radius

			cx := float64(bx) + half
			cy := float64(by) + half
			for dy := 0; dy < dotSize && by+dy < rect.Max.Y; dy++ {
				for dx := 0; dx < dotSize && bx+dx < rect.Max.X; dx++ {
					ddx := float64(bx+dx) + 0.5 - cx
					ddy := float64(by+dy) + 0.5 - cy
					if ddx*ddx+ddy*ddy <= r2 {
						i := idx(out, bx+dx, by+dy)
						out.Pix[i+0] = 0
						out.Pix[i+1] = 0
						out.Pix[i+2] = 0
						out.Pix[i+3] = 255
					}
				}
			}
		}
	}
	return out
}

// --------------------------------------------------------------------------
// Oil Painting effect (Kuwahara-like)
// --------------------------------------------------------------------------

// OilPainting simulates an oil painting look using quantized neighborhood averages.
func OilPainting(src image.Image, radius, levels int) *image.NRGBA {
	if radius < 1 {
		radius = 3
	}
	if levels < 2 {
		levels = 20
	}
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		counts := make([]int, levels)
		sumR := make([]float64, levels)
		sumG := make([]float64, levels)
		sumB := make([]float64, levels)
		for x := rect.Min.X; x < rect.Max.X; x++ {
			for i := range counts {
				counts[i] = 0
				sumR[i] = 0
				sumG[i] = 0
				sumB[i] = 0
			}
			for dy := -radius; dy <= radius; dy++ {
				sy := y + dy
				if sy < rect.Min.Y {
					sy = rect.Min.Y
				} else if sy >= rect.Max.Y {
					sy = rect.Max.Y - 1
				}
				for dx := -radius; dx <= radius; dx++ {
					sx := x + dx
					if sx < rect.Min.X {
						sx = rect.Min.X
					} else if sx >= rect.Max.X {
						sx = rect.Max.X - 1
					}
					p := idx(n, sx, sy)
					lum := (0.2126*float64(n.Pix[p+0]) + 0.7152*float64(n.Pix[p+1]) + 0.0722*float64(n.Pix[p+2])) / 255.0
					bin := int(lum * float64(levels-1))
					if bin >= levels {
						bin = levels - 1
					}
					counts[bin]++
					sumR[bin] += float64(n.Pix[p+0])
					sumG[bin] += float64(n.Pix[p+1])
					sumB[bin] += float64(n.Pix[p+2])
				}
			}
			// find most common bin
			maxBin := 0
			for b := 1; b < levels; b++ {
				if counts[b] > counts[maxBin] {
					maxBin = b
				}
			}
			i := idx(out, x, y)
			c := float64(counts[maxBin])
			if c > 0 {
				out.Pix[i+0] = clamp8(int(sumR[maxBin]/c + 0.5))
				out.Pix[i+1] = clamp8(int(sumG[maxBin]/c + 0.5))
				out.Pix[i+2] = clamp8(int(sumB[maxBin]/c + 0.5))
			}
			out.Pix[i+3] = n.Pix[idx(n, x, y)+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Chromatic Aberration
// --------------------------------------------------------------------------

// ChromaticAberration shifts red/blue channels outward from center.
func ChromaticAberration(src image.Image, shift float64) *image.NRGBA {
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	cx := float64(rect.Min.X+rect.Max.X) / 2.0
	cy := float64(rect.Min.Y+rect.Max.Y) / 2.0
	maxDist := math.Hypot(float64(rect.Dx())/2, float64(rect.Dy())/2)
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Hypot(dx, dy) / maxDist

			offset := shift * dist

			// Red channel: shift outward
			rx := int(math.Round(float64(x) + dx/math.Max(math.Hypot(dx, dy), 1)*offset))
			ry := int(math.Round(float64(y) + dy/math.Max(math.Hypot(dx, dy), 1)*offset))
			// Blue channel: shift inward
			bx := int(math.Round(float64(x) - dx/math.Max(math.Hypot(dx, dy), 1)*offset))
			by := int(math.Round(float64(y) - dy/math.Max(math.Hypot(dx, dy), 1)*offset))

			rx = clampInt(rx, rect.Min.X, rect.Max.X-1)
			ry = clampInt(ry, rect.Min.Y, rect.Max.Y-1)
			bx = clampInt(bx, rect.Min.X, rect.Max.X-1)
			by = clampInt(by, rect.Min.Y, rect.Max.Y-1)

			ri := idx(n, rx, ry)
			gi := idx(n, x, y)
			bi := idx(n, bx, by)

			oi := idx(out, x, y)
			out.Pix[oi+0] = n.Pix[ri+0] // red from shifted pos
			out.Pix[oi+1] = n.Pix[gi+1] // green stays
			out.Pix[oi+2] = n.Pix[bi+2] // blue from opposite shift
			out.Pix[oi+3] = n.Pix[gi+3]
		}
	})
	return out
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --------------------------------------------------------------------------
// Noise addition
// --------------------------------------------------------------------------

// AddNoise adds random noise. amount 0..1, monochrome controls color/gray noise.
func AddNoise(src image.Image, amount float64, monochrome bool) *image.NRGBA {
	n := ToNRGBA(src)
	rect := n.Rect
	out := CloneNRGBA(n)
	rng := rand.New(rand.NewSource(42))
	strength := amount * 128
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := idx(out, x, y)
			if monochrome {
				noise := (rng.Float64()*2 - 1) * strength
				out.Pix[i+0] = clamp8(int(float64(out.Pix[i+0]) + noise))
				out.Pix[i+1] = clamp8(int(float64(out.Pix[i+1]) + noise))
				out.Pix[i+2] = clamp8(int(float64(out.Pix[i+2]) + noise))
			} else {
				out.Pix[i+0] = clamp8(int(float64(out.Pix[i+0]) + (rng.Float64()*2-1)*strength))
				out.Pix[i+1] = clamp8(int(float64(out.Pix[i+1]) + (rng.Float64()*2-1)*strength))
				out.Pix[i+2] = clamp8(int(float64(out.Pix[i+2]) + (rng.Float64()*2-1)*strength))
			}
		}
	}
	return out
}

// --------------------------------------------------------------------------
// Tilt-Shift (selective blur with sharp band)
// --------------------------------------------------------------------------

// TiltShift blurs areas above and below a horizontal band.
// focusY: center of sharp band (0..1), bandWidth: sharp band size (0..1),
// blurSigma: blur strength.
func TiltShift(src image.Image, focusY, bandWidth, blurSigma float64) *image.NRGBA {
	n := ToNRGBA(src)
	blurred := GaussianBlur(n, blurSigma, 0)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	h := float64(rect.Dy())
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		// distance from focus band
		t := float64(yy) / h
		dist := math.Abs(t-focusY) - bandWidth/2
		if dist < 0 {
			dist = 0
		}
		// smooth transition
		blend := clampF01(dist / (bandWidth * 0.5))
		blend = blend * blend // smooth falloff

		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := idx(n, x, y)
			j := idx(blurred, x, y)
			for c := 0; c < 3; c++ {
				v := lerp(float64(n.Pix[i+c]), float64(blurred.Pix[j+c]), blend)
				out.Pix[i+c] = clamp8(int(v + 0.5))
			}
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Perspective Transform
// --------------------------------------------------------------------------

// PerspectiveTransform applies a simple 4-corner perspective warp.
// corners: [4][2]float64 as top-left, top-right, bottom-right, bottom-left
// in normalized coordinates (0..1).
func PerspectiveTransform(src image.Image, corners [4][2]float64, w, h int) *image.NRGBA {
	n := ToNRGBA(src)
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	srcW := float64(n.Rect.Dx())
	srcH := float64(n.Rect.Dy())

	_ = parallelRows(context.Background(), 0, h, func(yy int) {
		ty := float64(yy) / float64(h)
		for x := 0; x < w; x++ {
			tx := float64(x) / float64(w)
			// bilinear interpolation of corners
			topX := corners[0][0]*(1-tx) + corners[1][0]*tx
			topY := corners[0][1]*(1-tx) + corners[1][1]*tx
			botX := corners[3][0]*(1-tx) + corners[2][0]*tx
			botY := corners[3][1]*(1-tx) + corners[2][1]*tx
			srcX := (topX*(1-ty) + botX*ty) * srcW
			srcY := (topY*(1-ty) + botY*ty) * srcH

			sx := int(srcX)
			sy := int(srcY)
			if sx < n.Rect.Min.X || sx >= n.Rect.Max.X || sy < n.Rect.Min.Y || sy >= n.Rect.Max.Y {
				continue
			}
			si := idx(n, sx, sy)
			oi := idx(out, x, yy)
			out.Pix[oi+0] = n.Pix[si+0]
			out.Pix[oi+1] = n.Pix[si+1]
			out.Pix[oi+2] = n.Pix[si+2]
			out.Pix[oi+3] = n.Pix[si+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Color Temperature adjustment
// --------------------------------------------------------------------------

// ColorTemperature shifts the white point. temp > 0 warms (yellower), < 0 cools (bluer).
// Range roughly -1..1.
func ColorTemperature(src image.Image, temp float64) *image.NRGBA {
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	shift := temp * 30 // scale to usable range
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := idx(n, x, y)
			out.Pix[i+0] = clamp8(int(float64(n.Pix[i+0]) + shift))
			out.Pix[i+1] = n.Pix[i+1]
			out.Pix[i+2] = clamp8(int(float64(n.Pix[i+2]) - shift))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Tint (color overlay with opacity)
// --------------------------------------------------------------------------

// Tint overlays a color at the given opacity.
func Tint(src image.Image, c color.NRGBA, opacity float64) *image.NRGBA {
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	opacity = clampF01(opacity)
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := idx(n, x, y)
			out.Pix[i+0] = clamp8(int(lerp(float64(n.Pix[i+0]), float64(c.R), opacity)))
			out.Pix[i+1] = clamp8(int(lerp(float64(n.Pix[i+1]), float64(c.G), opacity)))
			out.Pix[i+2] = clamp8(int(lerp(float64(n.Pix[i+2]), float64(c.B), opacity)))
			out.Pix[i+3] = n.Pix[i+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Bilateral Filter (edge-preserving smoothing)
// --------------------------------------------------------------------------

// BilateralFilter smooths while preserving edges. sigmaSpatial controls
// spatial extent, sigmaRange controls color similarity threshold.
func BilateralFilter(src image.Image, sigmaSpatial, sigmaRange float64) *image.NRGBA {
	n := ToNRGBA(src)
	rect := n.Rect
	out := image.NewNRGBA(rect)
	radius := int(math.Ceil(2 * sigmaSpatial))
	ss2 := 2 * sigmaSpatial * sigmaSpatial
	sr2 := 2 * sigmaRange * sigmaRange
	_ = parallelRows(context.Background(), 0, rect.Dy(), func(yy int) {
		y := rect.Min.Y + yy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			ci := idx(n, x, y)
			cr, cg, cb := float64(n.Pix[ci+0]), float64(n.Pix[ci+1]), float64(n.Pix[ci+2])
			var sumR, sumG, sumB, sumW float64
			for dy := -radius; dy <= radius; dy++ {
				sy := y + dy
				if sy < rect.Min.Y || sy >= rect.Max.Y {
					continue
				}
				for dx := -radius; dx <= radius; dx++ {
					sx := x + dx
					if sx < rect.Min.X || sx >= rect.Max.X {
						continue
					}
					si := idx(n, sx, sy)
					pr, pg, pb := float64(n.Pix[si+0]), float64(n.Pix[si+1]), float64(n.Pix[si+2])
					spatialDist := float64(dx*dx + dy*dy)
					colorDist := (cr-pr)*(cr-pr) + (cg-pg)*(cg-pg) + (cb-pb)*(cb-pb)
					w := math.Exp(-spatialDist/ss2 - colorDist/sr2)
					sumR += pr * w
					sumG += pg * w
					sumB += pb * w
					sumW += w
				}
			}
			oi := idx(out, x, y)
			if sumW > 0 {
				out.Pix[oi+0] = clamp8(int(sumR/sumW + 0.5))
				out.Pix[oi+1] = clamp8(int(sumG/sumW + 0.5))
				out.Pix[oi+2] = clamp8(int(sumB/sumW + 0.5))
			} else {
				out.Pix[oi+0] = n.Pix[ci+0]
				out.Pix[oi+1] = n.Pix[ci+1]
				out.Pix[oi+2] = n.Pix[ci+2]
			}
			out.Pix[oi+3] = n.Pix[ci+3]
		}
	})
	return out
}

// --------------------------------------------------------------------------
// Pipeline methods for all new effects
// --------------------------------------------------------------------------

func (p *Pipeline) Levels(inBlack, inWhite float64, gamma float64, outBlack, outWhite float64) *Pipeline {
	p.steps = append(p.steps, step{name: "levels", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return Levels(in, inBlack, inWhite, gamma, outBlack, outWhite)
	}})
	return p
}

func (p *Pipeline) Curves(points []CurvePoint) *Pipeline {
	p.steps = append(p.steps, step{name: "curves", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return ApplyCurve(in, points)
	}})
	return p
}

func (p *Pipeline) ChannelMix(rr, rg, rb, gr, gg, gb, br, bg, bb float64) *Pipeline {
	p.steps = append(p.steps, step{name: "channelMix", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return ChannelMix(in, rr, rg, rb, gr, gg, gb, br, bg, bb)
	}})
	return p
}

func (p *Pipeline) ColorBalance(shadowR, shadowG, shadowB, midR, midG, midB, highR, highG, highB float64) *Pipeline {
	p.steps = append(p.steps, step{name: "colorBalance", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return ColorBalance(in, shadowR, shadowG, shadowB, midR, midG, midB, highR, highG, highB)
	}})
	return p
}

func (p *Pipeline) HSLSelective(targetHue, hueRange, satFactor, lightDelta float64) *Pipeline {
	p.steps = append(p.steps, step{name: "hslSelective", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return HSLSelective(in, targetHue, hueRange, satFactor, lightDelta)
	}})
	return p
}

func (p *Pipeline) GradientMap(stops []GradientStop) *Pipeline {
	p.steps = append(p.steps, step{name: "gradientMap", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return GradientMap(in, stops)
	}})
	return p
}

func (p *Pipeline) MotionBlur(angle float64, distance int) *Pipeline {
	p.steps = append(p.steps, step{name: "motionBlur", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return MotionBlur(in, angle, distance)
	}})
	return p
}

func (p *Pipeline) RadialBlur(cx, cy, strength float64, samples int) *Pipeline {
	p.steps = append(p.steps, step{name: "radialBlur", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return RadialBlur(in, cx, cy, strength, samples)
	}})
	return p
}

func (p *Pipeline) Glow(sigma, intensity float64) *Pipeline {
	p.steps = append(p.steps, step{name: "glow", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return Glow(in, sigma, intensity)
	}})
	return p
}

func (p *Pipeline) Halftone(dotSize int) *Pipeline {
	p.steps = append(p.steps, step{name: "halftone", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return Halftone(in, dotSize)
	}})
	return p
}

func (p *Pipeline) OilPainting(radius, levels int) *Pipeline {
	p.steps = append(p.steps, step{name: "oilPainting", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return OilPainting(in, radius, levels)
	}})
	return p
}

func (p *Pipeline) ChromaticAberration(shift float64) *Pipeline {
	p.steps = append(p.steps, step{name: "chromaticAberration", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return ChromaticAberration(in, shift)
	}})
	return p
}

func (p *Pipeline) AddNoise(amount float64, monochrome bool) *Pipeline {
	p.steps = append(p.steps, step{name: "addNoise", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return AddNoise(in, amount, monochrome)
	}})
	return p
}

func (p *Pipeline) TiltShift(focusY, bandWidth, blurSigma float64) *Pipeline {
	p.steps = append(p.steps, step{name: "tiltShift", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return TiltShift(in, focusY, bandWidth, blurSigma)
	}})
	return p
}

func (p *Pipeline) ColorTemperature(temp float64) *Pipeline {
	p.steps = append(p.steps, step{name: "colorTemperature", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return ColorTemperature(in, temp)
	}})
	return p
}

func (p *Pipeline) Tint(c color.NRGBA, opacity float64) *Pipeline {
	p.steps = append(p.steps, step{name: "tint", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return Tint(in, c, opacity)
	}})
	return p
}

func (p *Pipeline) BilateralFilter(sigmaSpatial, sigmaRange float64) *Pipeline {
	p.steps = append(p.steps, step{name: "bilateral", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return BilateralFilter(in, sigmaSpatial, sigmaRange)
	}})
	return p
}

func (p *Pipeline) FlipX() *Pipeline {
	p.steps = append(p.steps, step{name: "flipX", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return FlipX(in)
	}})
	return p
}

func (p *Pipeline) FlipY() *Pipeline {
	p.steps = append(p.steps, step{name: "flipY", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return FlipY(in)
	}})
	return p
}

func (p *Pipeline) PerspectiveTransform(corners [4][2]float64, w, h int) *Pipeline {
	p.steps = append(p.steps, step{name: "perspective", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return PerspectiveTransform(in, corners, w, h)
	}})
	return p
}
