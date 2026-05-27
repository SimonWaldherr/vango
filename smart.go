package vango

import (
	"context"
	"image"
	"math"
	"sort"
	"strings"
)

// ImageStats summarizes global image characteristics used by automatic modes.
type ImageStats struct {
	AvgLuma       float64
	AvgSaturation float64
	LowLuma       int
	HighLuma      int
	DynamicRange  int
	Pixels        int
}

// AnalyzeImage computes luminance and saturation statistics for automatic modes.
func AnalyzeImage(src image.Image) ImageStats {
	n := ToNRGBA(src)
	var hist [256]int
	var lumaSum, satSum float64
	var cnt int
	for i := 0; i+3 < len(n.Pix); i += 4 {
		if n.Pix[i+3] == 0 {
			continue
		}
		l := luma8(n.Pix[i+0], n.Pix[i+1], n.Pix[i+2])
		hist[int(l+0.5)]++
		lumaSum += l / 255.0
		satSum += saturationApprox(n.Pix[i+0], n.Pix[i+1], n.Pix[i+2])
		cnt++
	}
	if cnt == 0 {
		return ImageStats{}
	}
	lo := percentileFromHist(hist, cnt, 0.01)
	hi := percentileFromHist(hist, cnt, 0.99)
	return ImageStats{
		AvgLuma:       lumaSum / float64(cnt),
		AvgSaturation: satSum / float64(cnt),
		LowLuma:       lo,
		HighLuma:      hi,
		DynamicRange:  hi - lo,
		Pixels:        cnt,
	}
}

// AutoBrightnessDelta targets mid-gray average luma (0.5), clamped to +/-0.3.
func AutoBrightnessDelta(src image.Image) float64 {
	stats := AnalyzeImage(src)
	if stats.Pixels == 0 {
		return 0
	}
	delta := 0.5 - stats.AvgLuma
	if delta > 0.3 {
		return 0.3
	}
	if delta < -0.3 {
		return -0.3
	}
	return delta
}

// AutoVibranceFactor raises average saturation toward about 0.55, capped at 1.8x.
func AutoVibranceFactor(src image.Image) float64 {
	stats := AnalyzeImage(src)
	if stats.Pixels == 0 {
		return 1
	}
	avgSat := stats.AvgSaturation
	if avgSat >= 0.55 {
		return 1
	}
	factor := 1 + (0.55-avgSat)*1.2
	if factor > 1.8 {
		return 1.8
	}
	return factor
}

// AutoExposure stretches luminance between low/high percentiles while preserving hue.
func AutoExposure(src image.Image) *image.NRGBA {
	n := ToNRGBA(src)
	stats := AnalyzeImage(n)
	if stats.Pixels == 0 {
		return CloneNRGBA(n)
	}

	lo := stats.LowLuma
	hi := stats.HighLuma
	if hi <= lo+2 {
		return CloneNRGBA(n)
	}

	dst := image.NewNRGBA(n.Rect)
	scaleRange := float64(hi - lo)
	_ = parallelRows(context.Background(), 0, n.Rect.Dy(), func(yy int) {
		y := n.Rect.Min.Y + yy
		for x := n.Rect.Min.X; x < n.Rect.Max.X; x++ {
			i := idx(n, x, y)
			l := luma8(n.Pix[i+0], n.Pix[i+1], n.Pix[i+2])
			target := clampF01((l - float64(lo)) / scaleRange)
			target = math.Pow(target, 0.92)
			if l < 1 {
				dst.Pix[i+0], dst.Pix[i+1], dst.Pix[i+2] = 0, 0, 0
			} else {
				ratio := target * 255.0 / l
				dst.Pix[i+0] = clamp8(int(float64(n.Pix[i+0])*ratio + 0.5))
				dst.Pix[i+1] = clamp8(int(float64(n.Pix[i+1])*ratio + 0.5))
				dst.Pix[i+2] = clamp8(int(float64(n.Pix[i+2])*ratio + 0.5))
			}
			dst.Pix[i+3] = n.Pix[i+3]
		}
	})
	return dst
}

// AutoTone balances color, stretches exposure, and gently protects shadows/highlights.
func AutoTone(src image.Image) *image.NRGBA {
	out := WhiteBalanceByRect(src, image.Rectangle{})
	out = AutoExposure(out)
	return ShadowHighlight(out, 0.18, 0.12)
}

// SmartEnhance applies a balanced one-shot enhancement for general photos.
func SmartEnhance(src image.Image) *image.NRGBA {
	out := AutoTone(src)
	out = Vibrance(out, 0.42)
	out = Clarity(out, 0.18)
	return out
}

// AutoScene adapts enhancement strength from the image's exposure, saturation,
// and dynamic range. It is a stronger automatic mode than SmartEnhance.
func AutoScene(src image.Image) *image.NRGBA {
	stats := AnalyzeImage(src)
	if stats.Pixels == 0 {
		return ToNRGBA(src)
	}
	out := WhiteBalanceByRect(src, image.Rectangle{})
	if stats.DynamicRange < 150 || stats.AvgLuma < 0.42 || stats.AvgLuma > 0.62 {
		out = AutoExposure(out)
	}
	shadow := clampF01((0.52 - stats.AvgLuma) * 0.7)
	highlight := clampF01((stats.AvgLuma - 0.48) * 0.45)
	if shadow > 0 || highlight > 0 {
		out = ShadowHighlight(out, shadow, highlight)
	}
	vibrance := clampF01((0.58 - stats.AvgSaturation) * 0.9)
	if vibrance > 0.05 {
		out = Vibrance(out, vibrance)
	}
	clarity := 0.12
	if stats.DynamicRange < 120 {
		clarity = 0.22
	}
	return Clarity(out, clarity)
}

// SmartCropRect chooses a crop rectangle with the requested aspect ratio using edge,
// saturation, skin-tone, and center-bias saliency.
func SmartCropRect(src image.Image, outW, outH int) image.Rectangle {
	n := ToNRGBA(src)
	b := n.Rect
	if outW <= 0 || outH <= 0 || b.Empty() {
		return b
	}
	srcW, srcH := b.Dx(), b.Dy()
	target := float64(outW) / float64(outH)
	srcAspect := float64(srcW) / float64(srcH)
	cropW, cropH := srcW, srcH
	if srcAspect > target {
		cropW = int(math.Round(float64(srcH) * target))
	} else if srcAspect < target {
		cropH = int(math.Round(float64(srcW) / target))
	}
	if cropW >= srcW && cropH >= srcH {
		return b
	}
	if cropW < 1 {
		cropW = 1
	}
	if cropH < 1 {
		cropH = 1
	}

	w, h := srcW, srcH
	saliency := buildSaliencyMap(n)
	integral := make([]float64, (w+1)*(h+1))
	centerX, centerY := float64(w-1)/2, float64(h-1)/2
	maxDist := math.Hypot(centerX, centerY)
	for y := 0; y < h; y++ {
		rowSum := 0.0
		for x := 0; x < w; x++ {
			score := saliency[y*w+x]
			if maxDist > 0 {
				dist := math.Hypot(float64(x)-centerX, float64(y)-centerY) / maxDist
				score *= 1.0 + 0.15*(1.0-dist)
			}
			rowSum += score
			integral[(y+1)*(w+1)+x+1] = integral[y*(w+1)+x+1] + rowSum
		}
	}

	step := minInt(cropW, cropH) / 80
	if step < 1 {
		step = 1
	}
	bestX := (w - cropW) / 2
	bestY := (h - cropH) / 2
	bestScore := math.Inf(-1)
	xs := scanPositions(w-cropW, step)
	ys := scanPositions(h-cropH, step)
	for _, y := range ys {
		for _, x := range xs {
			score := integralSum(integral, w+1, x, y, x+cropW, y+cropH)
			cx := float64(x+cropW/2) - centerX
			cy := float64(y+cropH/2) - centerY
			score -= 0.001 * math.Hypot(cx, cy)
			if score > bestScore {
				bestScore = score
				bestX, bestY = x, y
			}
		}
	}

	return image.Rect(b.Min.X+bestX, b.Min.Y+bestY, b.Min.X+bestX+cropW, b.Min.Y+bestY+cropH)
}

// SmartCrop crops to a saliency-selected rectangle and resizes to the requested size.
func SmartCrop(src image.Image, outW, outH int) *image.NRGBA {
	if outW <= 0 || outH <= 0 {
		return ToNRGBA(src)
	}
	rect := SmartCropRect(src, outW, outH)
	return ResizeBilinear(Crop(src, rect), outW, outH)
}

// ModernFilter applies named contemporary color looks. Supported names include:
// cinematic, teal_orange, matte, noir, lomo, chrome, fade, punch, golden_hour,
// moody, clean, portrait, cyberpunk, dreamscape, sunset, forest, and infrared.
func ModernFilter(src image.Image, name string, intensity float64) *image.NRGBA {
	if intensity <= 0 {
		return ToNRGBA(src)
	}
	if intensity > 1 {
		intensity = 1
	}
	name = strings.ToLower(strings.ReplaceAll(name, "-", "_"))
	n := ToNRGBA(src)
	out := image.NewNRGBA(n.Rect)
	_ = parallelRows(context.Background(), 0, n.Rect.Dy(), func(yy int) {
		y := n.Rect.Min.Y + yy
		for x := n.Rect.Min.X; x < n.Rect.Max.X; x++ {
			i := idx(n, x, y)
			r, g, b := applyModernLook(n.Pix[i+0], n.Pix[i+1], n.Pix[i+2], name, intensity)
			out.Pix[i+0], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = r, g, b, n.Pix[i+3]
		}
	})
	switch name {
	case "cinematic", "lomo", "dreamscape":
		out = Vignette(out, 0.25*intensity)
	case "noir":
		out = Vignette(out, 0.18*intensity)
	}
	return out
}

// ModernFilterNames returns the named color looks supported by ModernFilter.
func ModernFilterNames() []string {
	return []string{
		"cinematic", "teal_orange", "matte", "noir", "lomo", "chrome", "fade",
		"punch", "golden_hour", "moody", "clean", "portrait", "cyberpunk",
		"dreamscape", "sunset", "forest", "infrared",
	}
}

func (p *Pipeline) AutoBrightness() *Pipeline {
	p.steps = append(p.steps, step{name: "autoBrightness", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return AdjustBrightness(in, AutoBrightnessDelta(in))
	}})
	return p
}

func (p *Pipeline) AutoVibrance() *Pipeline {
	p.steps = append(p.steps, step{name: "autoVibrance", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return AdjustSaturation(in, AutoVibranceFactor(in))
	}})
	return p
}

func (p *Pipeline) AutoExposure() *Pipeline {
	p.steps = append(p.steps, step{name: "autoExposure", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return AutoExposure(in) }})
	return p
}

func (p *Pipeline) AutoTone() *Pipeline {
	p.steps = append(p.steps, step{name: "autoTone", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return AutoTone(in) }})
	return p
}

func (p *Pipeline) SmartEnhance() *Pipeline {
	p.steps = append(p.steps, step{name: "smartEnhance", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return SmartEnhance(in) }})
	return p
}

func (p *Pipeline) AutoScene() *Pipeline {
	p.steps = append(p.steps, step{name: "autoScene", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return AutoScene(in) }})
	return p
}

func (p *Pipeline) SmartCrop(outW, outH int) *Pipeline {
	p.steps = append(p.steps, step{name: "smartCrop", apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA { return SmartCrop(in, outW, outH) }})
	return p
}

func (p *Pipeline) ModernFilter(name string, intensity float64) *Pipeline {
	p.steps = append(p.steps, step{name: "modernFilter:" + name, apply: func(_ context.Context, in *image.NRGBA) *image.NRGBA {
		return ModernFilter(in, name, intensity)
	}})
	return p
}

func luma8(r, g, b uint8) float64 {
	return 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
}

func percentileFromHist(hist [256]int, total int, p float64) int {
	target := int(math.Round(float64(total-1) * p))
	if target < 0 {
		target = 0
	}
	var seen int
	for i, n := range hist {
		seen += n
		if seen > target {
			return i
		}
	}
	return 255
}

func buildSaliencyMap(n *image.NRGBA) []float64 {
	b := n.Rect
	w, h := b.Dx(), b.Dy()
	luma := make([]float64, w*h)
	saliency := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := idx(n, b.Min.X+x, b.Min.Y+y)
			r, g, bl := n.Pix[i+0], n.Pix[i+1], n.Pix[i+2]
			off := y*w + x
			luma[off] = luma8(r, g, bl)
			saliency[off] = saturationApprox(r, g, bl) * 90.0
			if isSkinTone(r, g, bl) {
				saliency[off] += 42.0
			}
		}
	}
	for y := 0; y < h; y++ {
		ym := clampInt(y-1, 0, h-1)
		yp := clampInt(y+1, 0, h-1)
		for x := 0; x < w; x++ {
			xm := clampInt(x-1, 0, w-1)
			xp := clampInt(x+1, 0, w-1)
			gx := -luma[ym*w+xm] - 2*luma[y*w+xm] - luma[yp*w+xm] +
				luma[ym*w+xp] + 2*luma[y*w+xp] + luma[yp*w+xp]
			gy := -luma[ym*w+xm] - 2*luma[ym*w+x] - luma[ym*w+xp] +
				luma[yp*w+xm] + 2*luma[yp*w+x] + luma[yp*w+xp]
			saliency[y*w+x] += math.Hypot(gx, gy) / 8.0
		}
	}
	return saliency
}

func integralSum(integral []float64, stride, x0, y0, x1, y1 int) float64 {
	return integral[y1*stride+x1] - integral[y0*stride+x1] - integral[y1*stride+x0] + integral[y0*stride+x0]
}

func saturationApprox(r, g, b uint8) float64 {
	rf, gf, bf := float64(r)/255.0, float64(g)/255.0, float64(b)/255.0
	mx := math.Max(rf, math.Max(gf, bf))
	mn := math.Min(rf, math.Min(gf, bf))
	if mx <= 0 {
		return 0
	}
	return (mx - mn) / mx
}

func isSkinTone(r, g, b uint8) bool {
	rf, gf, bf := float64(r), float64(g), float64(b)
	maxC := math.Max(rf, math.Max(gf, bf))
	minC := math.Min(rf, math.Min(gf, bf))
	return rf > 95 && gf > 40 && bf > 20 && maxC-minC > 15 && math.Abs(rf-gf) > 15 && rf > gf && rf > bf
}

func applyModernLook(r, g, b uint8, name string, intensity float64) (uint8, uint8, uint8) {
	rf, gf, bf := float64(r)/255.0, float64(g)/255.0, float64(b)/255.0
	h, s, l := rgbToHSL(rf, gf, bf)
	or, og, ob := rf, gf, bf

	switch name {
	case "cinematic":
		rf, gf, bf = splitTone(rf, gf, bf, [3]float64{0.00, 0.18, 0.22}, [3]float64{0.28, 0.14, 0.02}, 0.34*intensity)
		h, s, l = rgbToHSL(rf, gf, bf)
		s = clampF01(s * (1 + 0.18*intensity))
		l = contrastLightness(l, 1+0.22*intensity)
	case "teal_orange", "tealorange":
		rf, gf, bf = splitTone(rf, gf, bf, [3]float64{0.00, 0.22, 0.24}, [3]float64{0.34, 0.16, 0.01}, 0.42*intensity)
		h, s, l = rgbToHSL(rf, gf, bf)
		s = clampF01(s * (1 + 0.12*intensity))
	case "matte":
		l = 0.08*intensity + l*(1-0.14*intensity)
		l = contrastLightness(l, 1-0.22*intensity)
		s = clampF01(s * (1 - 0.18*intensity))
	case "noir":
		l = contrastLightness(l, 1+0.55*intensity)
		s = 0
	case "lomo":
		h = math.Mod(h-0.015*intensity+1, 1)
		s = clampF01(s * (1 + 0.36*intensity))
		l = contrastLightness(l, 1+0.28*intensity)
	case "chrome":
		h = math.Mod(h+0.01*intensity, 1)
		s = clampF01(s * (1 + 0.28*intensity))
		l = contrastLightness(l, 1+0.18*intensity)
	case "fade":
		l = 0.06*intensity + l*(1-0.10*intensity)
		s = clampF01(s * (1 - 0.26*intensity))
	case "punch":
		s = clampF01(s * (1 + 0.34*intensity))
		l = contrastLightness(l, 1+0.32*intensity)
	case "golden_hour", "goldenhour":
		h = math.Mod(h-0.025*intensity+1, 1)
		s = clampF01(s * (1 + 0.16*intensity))
		l = clampF01(l + 0.045*intensity)
		rf, gf, bf = splitTone(rf, gf, bf, [3]float64{0.08, 0.02, 0.00}, [3]float64{0.32, 0.18, 0.02}, 0.32*intensity)
		h, s, l = rgbToHSL(rf, gf, bf)
	case "moody":
		rf, gf, bf = splitTone(rf, gf, bf, [3]float64{0.00, 0.05, 0.10}, [3]float64{0.08, 0.05, 0.02}, 0.26*intensity)
		h, s, l = rgbToHSL(rf, gf, bf)
		l = contrastLightness(l, 1+0.26*intensity)
		s = clampF01(s * (1 - 0.14*intensity))
	case "clean":
		s = clampF01(s * (1 + 0.10*intensity))
		l = clampF01(l + 0.035*intensity)
		l = contrastLightness(l, 1+0.08*intensity)
	case "portrait":
		s = clampF01(s * (1 + 0.08*intensity))
		l = clampF01(l + 0.025*intensity)
		rf, gf, bf = splitTone(rf, gf, bf, [3]float64{0.03, 0.01, 0.00}, [3]float64{0.16, 0.06, 0.02}, 0.18*intensity)
		h, s, l = rgbToHSL(rf, gf, bf)
	case "cyberpunk":
		rf, gf, bf = splitTone(rf, gf, bf, [3]float64{0.02, 0.00, 0.28}, [3]float64{0.26, 0.00, 0.22}, 0.44*intensity)
		h, s, l = rgbToHSL(rf, gf, bf)
		s = clampF01(s * (1 + 0.38*intensity))
		l = contrastLightness(l, 1+0.22*intensity)
	case "dreamscape":
		rf, gf, bf = splitTone(rf, gf, bf, [3]float64{0.12, 0.02, 0.18}, [3]float64{0.06, 0.14, 0.22}, 0.38*intensity)
		h, s, l = rgbToHSL(rf, gf, bf)
		h = math.Mod(h+0.035*intensity, 1)
		s = clampF01(s * (1 + 0.20*intensity))
		l = clampF01(0.055*intensity + l*(1-0.08*intensity))
	case "sunset":
		rf, gf, bf = splitTone(rf, gf, bf, [3]float64{0.10, 0.03, 0.00}, [3]float64{0.38, 0.16, -0.04}, 0.36*intensity)
		h, s, l = rgbToHSL(rf, gf, bf)
		h = math.Mod(h-0.018*intensity+1, 1)
		s = clampF01(s * (1 + 0.18*intensity))
		l = clampF01(l + 0.025*intensity)
	case "forest":
		rf, gf, bf = splitTone(rf, gf, bf, [3]float64{-0.02, 0.12, 0.05}, [3]float64{0.10, 0.16, 0.02}, 0.34*intensity)
		h, s, l = rgbToHSL(rf, gf, bf)
		h = math.Mod(h+0.018*intensity, 1)
		s = clampF01(s * (1 + 0.10*intensity))
		l = contrastLightness(l, 1+0.14*intensity)
	case "infrared":
		lum := 0.38*rf + 0.50*gf + 0.12*bf
		rf = clampF01(0.18 + lum*1.05)
		gf = clampF01(0.08 + (1-lum)*0.22 + gf*0.12)
		bf = clampF01(0.12 + (1-lum)*0.42)
		h, s, l = rgbToHSL(rf, gf, bf)
		s = clampF01(s * (1 + 0.30*intensity))
		l = contrastLightness(l, 1+0.18*intensity)
	default:
		return r, g, b
	}

	rf, gf, bf = hslToRGB(h, s, l)
	rf = lerp(or, rf, intensity)
	gf = lerp(og, gf, intensity)
	bf = lerp(ob, bf, intensity)
	return clamp8(int(rf*255 + 0.5)), clamp8(int(gf*255 + 0.5)), clamp8(int(bf*255 + 0.5))
}

func splitTone(r, g, b float64, shadow, highlight [3]float64, amount float64) (float64, float64, float64) {
	l := 0.2126*r + 0.7152*g + 0.0722*b
	sw := clampF01((0.62 - l) / 0.62)
	hw := clampF01((l - 0.38) / 0.62)
	r = clampF01(r + amount*(shadow[0]*sw+highlight[0]*hw))
	g = clampF01(g + amount*(shadow[1]*sw+highlight[1]*hw))
	b = clampF01(b + amount*(shadow[2]*sw+highlight[2]*hw))
	return r, g, b
}

func contrastLightness(l, factor float64) float64 {
	return clampF01((l-0.5)*factor + 0.5)
}

func scanPositions(max, step int) []int {
	if max <= 0 {
		return []int{0}
	}
	var out []int
	for p := 0; p <= max; p += step {
		out = append(out, p)
	}
	if out[len(out)-1] != max {
		out = append(out, max)
	}
	sort.Ints(out)
	return out
}
