package vango

import (
	"context"
	"image"
	"math"
)

// --------------------------------------------------------------------------
// Distortion Filters
// --------------------------------------------------------------------------

// Twirl rotates pixels around the center with decreasing angle toward the edge.
func Twirl(src *image.NRGBA, angle float64, radius float64) *image.NRGBA {
	return TwirlCtx(context.Background(), src, angle, radius)
}

func TwirlCtx(ctx context.Context, src *image.NRGBA, angle float64, radius float64) *image.NRGBA {
	b := src.Rect
	dst := image.NewNRGBA(b)
	cx := float64(b.Min.X+b.Max.X) / 2.0
	cy := float64(b.Min.Y+b.Max.Y) / 2.0
	if radius <= 0 {
		radius = math.Min(float64(b.Dx()), float64(b.Dy())) / 2.0
	}

	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Hypot(dx, dy)
			if dist < radius {
				t := 1 - dist/radius
				a := angle * t * t
				cosA := math.Cos(a)
				sinA := math.Sin(a)
				sx := cx + dx*cosA - dy*sinA
				sy := cy + dx*sinA + dy*cosA
				bilinearSample(src, dst, x, y, sx, sy)
			} else {
				di := idx(dst, x, y)
				si := idx(src, x, y)
				copy(dst.Pix[di:di+4], src.Pix[si:si+4])
			}
		}
	})
	return dst
}

// Spherize applies a spherical distortion (bulge/pinch).
// amount > 0 = bulge, < 0 = pinch
func Spherize(src *image.NRGBA, amount float64) *image.NRGBA {
	return SpherizeCtx(context.Background(), src, amount)
}

func SpherizeCtx(ctx context.Context, src *image.NRGBA, amount float64) *image.NRGBA {
	b := src.Rect
	dst := image.NewNRGBA(b)
	cx := float64(b.Min.X+b.Max.X) / 2.0
	cy := float64(b.Min.Y+b.Max.Y) / 2.0
	radius := math.Min(float64(b.Dx()), float64(b.Dy())) / 2.0

	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := (float64(x) - cx) / radius
			dy := (float64(y) - cy) / radius
			dist := math.Hypot(dx, dy)
			if dist < 1.0 && dist > 0 {
				var r float64
				if amount > 0 {
					r = math.Pow(dist, 1.0/(1.0+amount)) / dist
				} else {
					r = math.Pow(dist, 1.0-amount) / dist
				}
				sx := cx + dx*r*radius
				sy := cy + dy*r*radius
				bilinearSample(src, dst, x, y, sx, sy)
			} else {
				di := idx(dst, x, y)
				si := idx(src, x, y)
				copy(dst.Pix[di:di+4], src.Pix[si:si+4])
			}
		}
	})
	return dst
}

// Wave applies a sinusoidal wave distortion.
func Wave(src *image.NRGBA, amplitudeX, amplitudeY, wavelengthX, wavelengthY float64) *image.NRGBA {
	return WaveCtx(context.Background(), src, amplitudeX, amplitudeY, wavelengthX, wavelengthY)
}

func WaveCtx(ctx context.Context, src *image.NRGBA, amplitudeX, amplitudeY, wavelengthX, wavelengthY float64) *image.NRGBA {
	b := src.Rect
	dst := image.NewNRGBA(b)
	if wavelengthX <= 0 {
		wavelengthX = 1
	}
	if wavelengthY <= 0 {
		wavelengthY = 1
	}

	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			sx := float64(x) + amplitudeX*math.Sin(2*math.Pi*float64(y)/wavelengthX)
			sy := float64(y) + amplitudeY*math.Sin(2*math.Pi*float64(x)/wavelengthY)
			bilinearSample(src, dst, x, y, sx, sy)
		}
	})
	return dst
}

// Ripple applies a radial ripple effect from the center.
func Ripple(src *image.NRGBA, amplitude, wavelength float64) *image.NRGBA {
	return RippleCtx(context.Background(), src, amplitude, wavelength)
}

func RippleCtx(ctx context.Context, src *image.NRGBA, amplitude, wavelength float64) *image.NRGBA {
	b := src.Rect
	dst := image.NewNRGBA(b)
	cx := float64(b.Min.X+b.Max.X) / 2.0
	cy := float64(b.Min.Y+b.Max.Y) / 2.0
	if wavelength <= 0 {
		wavelength = 1
	}

	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Hypot(dx, dy)
			if dist > 0 {
				offset := amplitude * math.Sin(2*math.Pi*dist/wavelength)
				sx := float64(x) + dx/dist*offset
				sy := float64(y) + dy/dist*offset
				bilinearSample(src, dst, x, y, sx, sy)
			} else {
				di := idx(dst, x, y)
				si := idx(src, x, y)
				copy(dst.Pix[di:di+4], src.Pix[si:si+4])
			}
		}
	})
	return dst
}

// PolarCoordinates converts between rectangular and polar coordinate systems.
// If toPolar is true, converts rect->polar; otherwise polar->rect.
func PolarCoordinates(src *image.NRGBA, toPolar bool) *image.NRGBA {
	return PolarCoordinatesCtx(context.Background(), src, toPolar)
}

func PolarCoordinatesCtx(ctx context.Context, src *image.NRGBA, toPolar bool) *image.NRGBA {
	b := src.Rect
	dst := image.NewNRGBA(b)
	w := float64(b.Dx())
	h := float64(b.Dy())
	cx := float64(b.Min.X) + w/2
	cy := float64(b.Min.Y) + h/2
	maxR := math.Hypot(w/2, h/2)

	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			var sx, sy float64
			if toPolar {
				// Map x -> angle, y -> radius
				angle := (float64(x) - float64(b.Min.X)) / w * 2 * math.Pi
				radius := (float64(y) - float64(b.Min.Y)) / h * maxR
				sx = cx + radius*math.Cos(angle)
				sy = cy + radius*math.Sin(angle)
			} else {
				// Map angle -> x, radius -> y
				dx := float64(x) - cx
				dy := float64(y) - cy
				angle := math.Atan2(dy, dx)
				if angle < 0 {
					angle += 2 * math.Pi
				}
				radius := math.Hypot(dx, dy)
				sx = float64(b.Min.X) + angle/(2*math.Pi)*w
				sy = float64(b.Min.Y) + radius/maxR*h
			}
			bilinearSample(src, dst, x, y, sx, sy)
		}
	})
	return dst
}

// Pinch squeezes pixels toward the center.
func Pinch(src *image.NRGBA, amount, radius float64) *image.NRGBA {
	return PinchCtx(context.Background(), src, amount, radius)
}

func PinchCtx(ctx context.Context, src *image.NRGBA, amount, radius float64) *image.NRGBA {
	b := src.Rect
	dst := image.NewNRGBA(b)
	cx := float64(b.Min.X+b.Max.X) / 2.0
	cy := float64(b.Min.Y+b.Max.Y) / 2.0
	if radius <= 0 {
		radius = math.Min(float64(b.Dx()), float64(b.Dy())) / 2.0
	}

	parallelRows(ctx, b.Min.Y, b.Max.Y, func(y int) {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Hypot(dx, dy)
			if dist < radius && dist > 0 {
				t := dist / radius
				factor := math.Pow(t, amount)
				sx := cx + dx*factor/t
				sy := cy + dy*factor/t
				bilinearSample(src, dst, x, y, sx, sy)
			} else {
				di := idx(dst, x, y)
				si := idx(src, x, y)
				copy(dst.Pix[di:di+4], src.Pix[si:si+4])
			}
		}
	})
	return dst
}

// bilinearSample samples src at floating-point coordinates and writes to dst at (dx,dy).
func bilinearSample(src, dst *image.NRGBA, dx, dy int, sx, sy float64) {
	b := src.Rect
	x0 := int(math.Floor(sx))
	y0 := int(math.Floor(sy))
	x1, y1 := x0+1, y0+1
	fx := sx - float64(x0)
	fy := sy - float64(y0)

	di := idx(dst, dx, dy)

	// Clamp to bounds
	c00 := samplePixel(src, b, x0, y0)
	c10 := samplePixel(src, b, x1, y0)
	c01 := samplePixel(src, b, x0, y1)
	c11 := samplePixel(src, b, x1, y1)

	for ch := 0; ch < 4; ch++ {
		top := lerp(float64(c00[ch]), float64(c10[ch]), fx)
		bot := lerp(float64(c01[ch]), float64(c11[ch]), fx)
		dst.Pix[di+ch] = clamp8(int(lerp(top, bot, fy) + 0.5))
	}
}

func samplePixel(src *image.NRGBA, b image.Rectangle, x, y int) [4]uint8 {
	if x < b.Min.X {
		x = b.Min.X
	} else if x >= b.Max.X {
		x = b.Max.X - 1
	}
	if y < b.Min.Y {
		y = b.Min.Y
	} else if y >= b.Max.Y {
		y = b.Max.Y - 1
	}
	i := idx(src, x, y)
	return [4]uint8{src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3]}
}

// --------------------------------------------------------------------------
// Pipeline methods for distortion filters
// --------------------------------------------------------------------------

func (p *Pipeline) Twirl(angle, radius float64) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return TwirlCtx(ctx, img, angle, radius)
	}})
	return p
}

func (p *Pipeline) Spherize(amount float64) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return SpherizeCtx(ctx, img, amount)
	}})
	return p
}

func (p *Pipeline) Wave(ampX, ampY, wlX, wlY float64) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return WaveCtx(ctx, img, ampX, ampY, wlX, wlY)
	}})
	return p
}

func (p *Pipeline) Ripple(amplitude, wavelength float64) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return RippleCtx(ctx, img, amplitude, wavelength)
	}})
	return p
}

func (p *Pipeline) PolarCoordinates(toPolar bool) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return PolarCoordinatesCtx(ctx, img, toPolar)
	}})
	return p
}

func (p *Pipeline) Pinch(amount, radius float64) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return PinchCtx(ctx, img, amount, radius)
	}})
	return p
}
