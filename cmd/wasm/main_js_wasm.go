//go:build js && wasm

package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"github.com/SimonWaldherr/vango"
)

var wasmVersion = "dev"

// --------------------------------------------------------------------------
// Helpers shared between processImage and processImagePNG
// --------------------------------------------------------------------------

func parseFloatArg(s string, fallback float64) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fallback
	}
	return v
}

func parseIntArg(s string, fallback int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}

func splitCommands(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' || r == '\n' })
}

// autoBrightnessDelta targets mid-grey average luma (0.5), clamped to ±0.3.
func autoBrightnessDelta(n *image.NRGBA) float64 {
	var sum float64
	var cnt int
	for i := 0; i+3 < len(n.Pix); i += 4 {
		if n.Pix[i+3] == 0 {
			continue
		}
		l := (0.2126*float64(n.Pix[i+0]) + 0.7152*float64(n.Pix[i+1]) + 0.0722*float64(n.Pix[i+2])) / 255.0
		sum += l
		cnt++
	}
	if cnt == 0 {
		return 0
	}
	delta := 0.5 - (sum / float64(cnt))
	if delta > 0.3 {
		return 0.3
	}
	if delta < -0.3 {
		return -0.3
	}
	return delta
}

// autoVibranceFactor raises average saturation toward ~0.55, capped at 1.8×.
func autoVibranceFactor(n *image.NRGBA) float64 {
	var satSum float64
	var cnt int
	for i := 0; i+3 < len(n.Pix); i += 4 {
		if n.Pix[i+3] == 0 {
			continue
		}
		rf := float64(n.Pix[i+0]) / 255.0
		gf := float64(n.Pix[i+1]) / 255.0
		bf := float64(n.Pix[i+2]) / 255.0
		mx := math.Max(rf, math.Max(gf, bf))
		mn := math.Min(rf, math.Min(gf, bf))
		s := 0.0
		if mx > 0 {
			s = (mx - mn) / mx
		}
		satSum += s
		cnt++
	}
	if cnt == 0 {
		return 1
	}
	avgSat := satSum / float64(cnt)
	if avgSat >= 0.55 {
		return 1
	}
	factor := 1 + (0.55-avgSat)*1.2
	if factor > 1.8 {
		return 1.8
	}
	return factor
}

// applyCommand dispatches a single text command to the pipeline.
// File-based commands (collage, watermark) are not supported in WASM.
func applyCommand(p *vango.Pipeline, raw string) *vango.Pipeline {
	toks := strings.Fields(strings.TrimSpace(raw))
	if len(toks) == 0 {
		return p
	}
	name := strings.ToLower(toks[0])
	args := toks[1:]

	switch name {
	case "blur":
		if len(args) >= 1 {
			p = p.GaussianBlur(parseFloatArg(args[0], 0), 0)
		}
	case "unsharp":
		if len(args) >= 2 {
			p = p.Unsharp(parseFloatArg(args[0], 0), parseFloatArg(args[1], 0), 0)
		}
	case "contrast":
		if len(args) >= 1 {
			p = p.Contrast(parseFloatArg(args[0], 1))
		}
	case "saturation":
		if len(args) >= 1 {
			p = p.Saturation(parseFloatArg(args[0], 1))
		}
	case "brightness":
		if len(args) >= 1 {
			p = p.Brightness(parseFloatArg(args[0], 0))
		}
	case "hue":
		if len(args) >= 1 {
			p = p.Hue(parseFloatArg(args[0], 0))
		}
	case "sepia":
		if len(args) >= 1 {
			p = p.Sepia(parseFloatArg(args[0], 0))
		}
	case "invert":
		p = p.Invert()
	case "grayscale":
		p = p.Grayscale()
	case "solarize":
		if len(args) >= 1 {
			p = p.Solarize(uint8(parseIntArg(args[0], 128)))
		} else {
			p = p.Solarize(128)
		}
	case "emboss":
		if len(args) >= 1 {
			p = p.Emboss(parseFloatArg(args[0], 0.5))
		} else {
			p = p.Emboss(0.5)
		}
	case "vignette":
		if len(args) >= 1 {
			p = p.Vignette(parseFloatArg(args[0], 0.5))
		} else {
			p = p.Vignette(0.5)
		}
	case "gamma":
		if len(args) >= 1 {
			p = p.Gamma(parseFloatArg(args[0], 1))
		}
	case "rotate":
		if len(args) >= 1 {
			p = p.Rotate(parseFloatArg(args[0], 0), "bilinear", color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	case "skew":
		if len(args) >= 2 {
			p = p.Skew(parseFloatArg(args[0], 0), parseFloatArg(args[1], 0), "bilinear", color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	case "resize":
		if len(args) >= 2 {
			p = p.ResizeBilinear(parseIntArg(args[0], 1), parseIntArg(args[1], 1))
		}
	case "resizenearest", "resize_nearest":
		if len(args) >= 2 {
			p = p.ResizeNearest(parseIntArg(args[0], 1), parseIntArg(args[1], 1))
		}
	case "crop":
		if len(args) >= 4 {
			p = p.Crop(image.Rect(
				parseIntArg(args[0], 0), parseIntArg(args[1], 0),
				parseIntArg(args[2], 0), parseIntArg(args[3], 0),
			))
		}
	case "pixelate":
		if len(args) >= 1 {
			p = p.Pixelate(parseIntArg(args[0], 1))
		}
	case "posterize":
		if len(args) >= 1 {
			p = p.Posterize(parseIntArg(args[0], 2))
		}
	case "threshold":
		if len(args) >= 1 {
			p = p.Threshold(uint8(parseIntArg(args[0], 128)))
		}
	case "equalize":
		p = p.Equalize()
	case "tonemap":
		if len(args) >= 1 {
			p = p.Tonemap(parseFloatArg(args[0], 1))
		}
	case "dither":
		if len(args) >= 1 {
			p = p.Dither(parseIntArg(args[0], 4))
		}
	case "whitebalance", "wb":
		p = p.WhiteBalance(image.Rectangle{})
	case "autocontrast", "auto_contrast":
		p = p.Equalize()
	case "autobrightness", "auto_brightness":
		n := vango.ToNRGBA(p.Image())
		p = vango.From(n).Brightness(autoBrightnessDelta(n))
	case "autovibrance", "auto_vibrance":
		n := vango.ToNRGBA(p.Image())
		p = vango.From(n).Saturation(autoVibranceFactor(n))
	case "autocolor", "auto_color":
		p = p.WhiteBalance(image.Rectangle{})
		p = p.Equalize()
		n := vango.ToNRGBA(p.Image())
		p = vango.From(n).Brightness(autoBrightnessDelta(n))
		brightened := p.Image()
		p = vango.From(brightened).Saturation(autoVibranceFactor(brightened))
	case "autofull", "auto_full":
		p = p.WhiteBalance(image.Rectangle{})
		p = p.NoiseReduction(1)
		p = p.Equalize()
		n := vango.ToNRGBA(p.Image())
		p = vango.From(n).Brightness(autoBrightnessDelta(n))
		brightened := p.Image()
		p = vango.From(brightened).Saturation(autoVibranceFactor(brightened))
	case "noisereduction", "noise_reduction", "denoise":
		radius := 1
		if len(args) >= 1 {
			radius = parseIntArg(args[0], 1)
		}
		p = p.NoiseReduction(radius)
	case "edge", "edgedetect", "edge_detect":
		p = vango.From(vango.SobelEdges(p.Image()))
	case "sharpen":
		amount := 1.0
		if len(args) >= 1 {
			amount = parseFloatArg(args[0], 1)
		}
		p = p.SharpenConvolution(amount)
	case "highpasssharpen", "high_pass_sharpen":
		amount := 1.0
		radius := 3
		if len(args) >= 1 {
			amount = parseFloatArg(args[0], 1)
		}
		if len(args) >= 2 {
			radius = parseIntArg(args[1], 3)
		}
		p = p.HighPassSharpen(amount, radius)
	case "clarity":
		strength := 0.5
		if len(args) >= 1 {
			strength = parseFloatArg(args[0], 0.5)
		}
		p = p.Clarity(strength)
	case "levels":
		if len(args) >= 5 {
			p = p.Levels(parseFloatArg(args[0], 0), parseFloatArg(args[1], 255), parseFloatArg(args[2], 1), parseFloatArg(args[3], 0), parseFloatArg(args[4], 255))
		}
	case "channelmix", "channel_mix":
		if len(args) >= 9 {
			p = p.ChannelMix(
				parseFloatArg(args[0], 1), parseFloatArg(args[1], 0), parseFloatArg(args[2], 0),
				parseFloatArg(args[3], 0), parseFloatArg(args[4], 1), parseFloatArg(args[5], 0),
				parseFloatArg(args[6], 0), parseFloatArg(args[7], 0), parseFloatArg(args[8], 1),
			)
		}
	case "colorbalance", "color_balance":
		if len(args) >= 9 {
			p = p.ColorBalance(
				parseFloatArg(args[0], 0), parseFloatArg(args[1], 0), parseFloatArg(args[2], 0),
				parseFloatArg(args[3], 0), parseFloatArg(args[4], 0), parseFloatArg(args[5], 0),
				parseFloatArg(args[6], 0), parseFloatArg(args[7], 0), parseFloatArg(args[8], 0),
			)
		}
	case "hslselective", "hsl_selective":
		if len(args) >= 4 {
			p = p.HSLSelective(parseFloatArg(args[0], 0), parseFloatArg(args[1], 30), parseFloatArg(args[2], 1), parseFloatArg(args[3], 0))
		}
	case "motionblur", "motion_blur":
		if len(args) >= 2 {
			p = p.MotionBlur(parseFloatArg(args[0], 0), parseIntArg(args[1], 10))
		}
	case "radialblur", "radial_blur":
		if len(args) >= 3 {
			p = p.RadialBlur(parseFloatArg(args[0], 0), parseFloatArg(args[1], 0), parseFloatArg(args[2], 0.05), 10)
		}
	case "glow":
		sigma := 5.0
		intensity := 0.5
		if len(args) >= 1 {
			sigma = parseFloatArg(args[0], 5)
		}
		if len(args) >= 2 {
			intensity = parseFloatArg(args[1], 0.5)
		}
		p = p.Glow(sigma, intensity)
	case "halftone":
		dotSize := 4
		if len(args) >= 1 {
			dotSize = parseIntArg(args[0], 4)
		}
		p = p.Halftone(dotSize)
	case "oilpainting", "oil_painting":
		radius := 3
		levels := 20
		if len(args) >= 1 {
			radius = parseIntArg(args[0], 3)
		}
		if len(args) >= 2 {
			levels = parseIntArg(args[1], 20)
		}
		p = p.OilPainting(radius, levels)
	case "chromaticaberration", "chromatic_aberration":
		shift := 3.0
		if len(args) >= 1 {
			shift = parseFloatArg(args[0], 3)
		}
		p = p.ChromaticAberration(shift)
	case "addnoise", "add_noise":
		amount := 0.1
		if len(args) >= 1 {
			amount = parseFloatArg(args[0], 0.1)
		}
		p = p.AddNoise(amount, false)
	case "tiltshift", "tilt_shift":
		focusY := 0.5
		bandW := 0.3
		sigma := 5.0
		if len(args) >= 1 {
			focusY = parseFloatArg(args[0], 0.5)
		}
		if len(args) >= 2 {
			bandW = parseFloatArg(args[1], 0.3)
		}
		if len(args) >= 3 {
			sigma = parseFloatArg(args[2], 5)
		}
		p = p.TiltShift(focusY, bandW, sigma)
	case "colortemperature", "color_temperature":
		if len(args) >= 1 {
			p = p.ColorTemperature(parseFloatArg(args[0], 0))
		}
	case "bilateral":
		ss := 3.0
		sr := 30.0
		if len(args) >= 1 {
			ss = parseFloatArg(args[0], 3)
		}
		if len(args) >= 2 {
			sr = parseFloatArg(args[1], 30)
		}
		p = p.BilateralFilter(ss, sr)
	case "flipx", "flip_x":
		p = p.FlipX()
	case "flipy", "flip_y":
		p = p.FlipY()

	// ── Distortion filters ──
	case "twirl":
		angle := math.Pi / 2
		radius := 0.0
		if len(args) >= 1 {
			angle = parseFloatArg(args[0], math.Pi/2)
		}
		if len(args) >= 2 {
			radius = parseFloatArg(args[1], 0)
		}
		p = p.Twirl(angle, radius)
	case "spherize":
		amount := 1.0
		if len(args) >= 1 {
			amount = parseFloatArg(args[0], 1)
		}
		p = p.Spherize(amount)
	case "wave":
		ax, ay := 10.0, 10.0
		wx, wy := 60.0, 60.0
		if len(args) >= 2 {
			ax = parseFloatArg(args[0], 10)
			ay = parseFloatArg(args[1], 10)
		}
		if len(args) >= 4 {
			wx = parseFloatArg(args[2], 60)
			wy = parseFloatArg(args[3], 60)
		}
		p = p.Wave(ax, ay, wx, wy)
	case "ripple":
		amp := 5.0
		wl := 30.0
		if len(args) >= 1 {
			amp = parseFloatArg(args[0], 5)
		}
		if len(args) >= 2 {
			wl = parseFloatArg(args[1], 30)
		}
		p = p.Ripple(amp, wl)
	case "polarcoordinates", "polar_coordinates":
		toPolar := true
		if len(args) >= 1 && args[0] == "false" {
			toPolar = false
		}
		p = p.PolarCoordinates(toPolar)
	case "pinch":
		amount := 2.0
		radius := 0.0
		if len(args) >= 1 {
			amount = parseFloatArg(args[0], 2)
		}
		if len(args) >= 2 {
			radius = parseFloatArg(args[1], 0)
		}
		p = p.Pinch(amount, radius)

	// ── Retouching ──
	case "dodge":
		amount := 0.3
		rangeType := "midtones"
		if len(args) >= 1 {
			amount = parseFloatArg(args[0], 0.3)
		}
		if len(args) >= 2 {
			rangeType = strings.ToLower(args[1])
		}
		p = p.Dodge(amount, rangeType)
	case "burn":
		amount := 0.3
		rangeType := "midtones"
		if len(args) >= 1 {
			amount = parseFloatArg(args[0], 0.3)
		}
		if len(args) >= 2 {
			rangeType = strings.ToLower(args[1])
		}
		p = p.Burn(amount, rangeType)

	// ── Pro Adjustments ──
	case "vibrance":
		amount := 0.5
		if len(args) >= 1 {
			amount = parseFloatArg(args[0], 0.5)
		}
		p = p.Vibrance(amount)
	case "dehaze":
		strength := 0.7
		if len(args) >= 1 {
			strength = parseFloatArg(args[0], 0.7)
		}
		p = p.Dehaze(strength)
	case "shadowhighlight", "shadow_highlight":
		shadowAmt := 0.3
		highAmt := 0.3
		if len(args) >= 1 {
			shadowAmt = parseFloatArg(args[0], 0.3)
		}
		if len(args) >= 2 {
			highAmt = parseFloatArg(args[1], 0.3)
		}
		p = p.ShadowHighlight(shadowAmt, highAmt)

	// ── Seam Carve ──
	case "seamcarve", "seam_carve":
		if len(args) >= 2 {
			p = p.SeamCarve(parseIntArg(args[0], 100), parseIntArg(args[1], 100))
		}
	}
	return p
}

// pixelDataFromJS copies a JS Uint8Array / Uint8ClampedArray into a Go
// image.NRGBA without allocating a second buffer.
func pixelDataFromJS(jsArr js.Value, width, height int) *image.NRGBA {
	pix := make([]byte, width*height*4)
	js.CopyBytesToGo(pix, jsArr)
	return &image.NRGBA{
		Pix:    pix,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}
}

// --------------------------------------------------------------------------
// Exported JS functions
// --------------------------------------------------------------------------

// jsProcessImage: (pixelData Uint8Array, width int, height int, commands string)
// → {pixels: Uint8ClampedArray, width: int, height: int, durationMs: int}
func jsProcessImage(_ js.Value, args []js.Value) any {
	if len(args) < 4 {
		ret := js.Global().Get("Object").New()
		ret.Set("error", "processImage requires 4 args: pixelData, width, height, commands")
		return ret
	}
	width := args[1].Int()
	height := args[2].Int()
	commands := args[3].String()

	img := pixelDataFromJS(args[0], width, height)

	start := time.Now()
	p := vango.From(img)
	for _, cmd := range splitCommands(commands) {
		p = applyCommand(p, cmd)
	}
	result := p.Image()
	elapsed := time.Since(start)

	jsBuf := js.Global().Get("Uint8ClampedArray").New(len(result.Pix))
	js.CopyBytesToJS(jsBuf, result.Pix)

	ret := js.Global().Get("Object").New()
	ret.Set("pixels", jsBuf)
	ret.Set("width", result.Rect.Dx())
	ret.Set("height", result.Rect.Dy())
	ret.Set("durationMs", elapsed.Milliseconds())
	return ret
}

// jsProcessImagePNG: (pixelData Uint8Array, width int, height int, commands string)
// → {dataURL: string, width: int, height: int, durationMs: int}
func jsProcessImagePNG(_ js.Value, args []js.Value) any {
	if len(args) < 4 {
		ret := js.Global().Get("Object").New()
		ret.Set("error", "processImagePNG requires 4 args: pixelData, width, height, commands")
		return ret
	}
	width := args[1].Int()
	height := args[2].Int()
	commands := args[3].String()

	img := pixelDataFromJS(args[0], width, height)

	start := time.Now()
	p := vango.From(img)
	for _, cmd := range splitCommands(commands) {
		p = applyCommand(p, cmd)
	}
	result := p.Image()
	elapsed := time.Since(start)

	var buf bytes.Buffer
	if err := png.Encode(&buf, result); err != nil {
		errRet := js.Global().Get("Object").New()
		errRet.Set("error", err.Error())
		return errRet
	}
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	ret := js.Global().Get("Object").New()
	ret.Set("dataURL", "data:image/png;base64,"+b64)
	ret.Set("width", result.Rect.Dx())
	ret.Set("height", result.Rect.Dy())
	ret.Set("durationMs", elapsed.Milliseconds())
	return ret
}

// --------------------------------------------------------------------------
// Registration
// --------------------------------------------------------------------------

// --------------------------------------------------------------------------
// Layer-based Canvas API for WASM
// --------------------------------------------------------------------------

var activeCanvas *vango.Canvas
var activeHistory *vango.History

// jsCanvasCreate: (width int, height int) → void
func jsCanvasCreate(_ js.Value, args []js.Value) any {
	if len(args) < 2 {
		return js.ValueOf("canvasCreate requires 2 args: width, height")
	}
	activeCanvas = vango.NewCanvas(args[0].Int(), args[1].Int())
	return js.Undefined()
}

// jsCanvasAddLayer: (name string, pixelData Uint8Array, width int, height int) → void
func jsCanvasAddLayer(_ js.Value, args []js.Value) any {
	if activeCanvas == nil || len(args) < 4 {
		return js.ValueOf("canvasAddLayer requires canvas + 4 args")
	}
	name := args[0].String()
	width := args[2].Int()
	height := args[3].Int()
	img := pixelDataFromJS(args[1], width, height)
	activeCanvas.AddLayer(vango.NewLayer(name, img))
	return js.Undefined()
}

// jsCanvasSetLayerProps: (name string, propsJSON string)
// props: blend, opacity, visible, offsetX, offsetY, zorder
func jsCanvasSetLayerProps(_ js.Value, args []js.Value) any {
	if activeCanvas == nil || len(args) < 2 {
		return js.ValueOf("need canvas + name + props")
	}
	name := args[0].String()
	l := activeCanvas.FindLayer(name)
	if l == nil {
		return js.ValueOf("layer not found: " + name)
	}

	props := args[1]
	if props.Type() != js.TypeObject {
		return js.ValueOf("props must be object")
	}
	if v := props.Get("blend"); !v.IsUndefined() {
		l.Blend = vango.BlendModeFromString(strings.ToLower(v.String()))
	}
	if v := props.Get("opacity"); !v.IsUndefined() {
		l.Opacity = v.Float()
	}
	if v := props.Get("visible"); !v.IsUndefined() {
		l.Visible = v.Bool()
	}
	if v := props.Get("offsetX"); !v.IsUndefined() {
		l.OffsetX = v.Int()
	}
	if v := props.Get("offsetY"); !v.IsUndefined() {
		l.OffsetY = v.Int()
	}
	if v := props.Get("zorder"); !v.IsUndefined() {
		l.ZOrder = v.Int()
	}
	if v := props.Get("locked"); !v.IsUndefined() {
		l.Locked = v.Bool()
	}
	return js.Undefined()
}

// jsCanvasApplyLayerEffects: (name string, commands string) → void
func jsCanvasApplyLayerEffects(_ js.Value, args []js.Value) any {
	if activeCanvas == nil || len(args) < 2 {
		return js.ValueOf("need canvas + name + commands")
	}
	name := args[0].String()
	commands := args[1].String()
	l := activeCanvas.FindLayer(name)
	if l == nil {
		return js.ValueOf("layer not found: " + name)
	}
	// apply commands to the layer's image
	p := vango.From(l.Image)
	for _, cmd := range splitCommands(commands) {
		p = applyCommand(p, cmd)
	}
	l.Image = p.Image()
	return js.Undefined()
}

// jsCanvasRemoveLayer: (name string) → void
func jsCanvasRemoveLayer(_ js.Value, args []js.Value) any {
	if activeCanvas == nil || len(args) < 1 {
		return js.ValueOf("need canvas + name")
	}
	activeCanvas.RemoveLayer(args[0].String())
	return js.Undefined()
}

// jsCanvasDuplicateLayer: (name string, newName string) → void
func jsCanvasDuplicateLayer(_ js.Value, args []js.Value) any {
	if activeCanvas == nil || len(args) < 2 {
		return js.ValueOf("need canvas + name + newName")
	}
	activeCanvas.DuplicateLayer(args[0].String(), args[1].String())
	return js.Undefined()
}

// jsCanvasMergeDown: (name string) → void
func jsCanvasMergeDown(_ js.Value, args []js.Value) any {
	if activeCanvas == nil || len(args) < 1 {
		return js.ValueOf("need canvas + name")
	}
	activeCanvas.MergeDown(args[0].String())
	return js.Undefined()
}

// jsCanvasFlatten: () → {pixels Uint8ClampedArray, width int, height int, durationMs int}
func jsCanvasFlatten(_ js.Value, _ []js.Value) any {
	if activeCanvas == nil {
		ret := js.Global().Get("Object").New()
		ret.Set("error", "no canvas")
		return ret
	}
	start := time.Now()
	result := activeCanvas.FlattenAll()
	elapsed := time.Since(start)

	jsBuf := js.Global().Get("Uint8ClampedArray").New(len(result.Pix))
	js.CopyBytesToJS(jsBuf, result.Pix)

	ret := js.Global().Get("Object").New()
	ret.Set("pixels", jsBuf)
	ret.Set("width", result.Rect.Dx())
	ret.Set("height", result.Rect.Dy())
	ret.Set("durationMs", elapsed.Milliseconds())
	return ret
}

// jsCanvasGetLayers: () → [{name, blend, opacity, visible, zorder, locked, width, height, offsetX, offsetY}]
func jsCanvasGetLayers(_ js.Value, _ []js.Value) any {
	if activeCanvas == nil {
		return js.ValueOf([]any{})
	}
	arr := make([]any, len(activeCanvas.Layers))
	for i, l := range activeCanvas.Layers {
		obj := js.Global().Get("Object").New()
		obj.Set("name", l.Name)
		obj.Set("blend", l.Blend.String())
		obj.Set("opacity", l.Opacity)
		obj.Set("visible", l.Visible)
		obj.Set("zorder", l.ZOrder)
		obj.Set("locked", l.Locked)
		obj.Set("width", l.Image.Rect.Dx())
		obj.Set("height", l.Image.Rect.Dy())
		obj.Set("offsetX", l.OffsetX)
		obj.Set("offsetY", l.OffsetY)
		arr[i] = obj
	}
	return js.ValueOf(arr)
}

// --------------------------------------------------------------------------
// History (Undo/Redo) API
// --------------------------------------------------------------------------

// jsHistoryInit: (maxUndo int) → void
func jsHistoryInit(_ js.Value, args []js.Value) any {
	maxUndo := 50
	if len(args) >= 1 {
		maxUndo = args[0].Int()
	}
	activeHistory = vango.NewHistory(maxUndo)
	if activeCanvas != nil {
		activeHistory.SaveState(activeCanvas, "initial")
	}
	return js.Undefined()
}

// jsHistorySave: (description string) → void
func jsHistorySave(_ js.Value, args []js.Value) any {
	if activeHistory == nil || activeCanvas == nil {
		return js.ValueOf("no history or canvas")
	}
	desc := "action"
	if len(args) >= 1 {
		desc = args[0].String()
	}
	activeHistory.SaveState(activeCanvas, desc)
	return js.Undefined()
}

// jsHistoryUndo: () → bool
func jsHistoryUndo(_ js.Value, _ []js.Value) any {
	if activeHistory == nil || activeCanvas == nil {
		return js.ValueOf(false)
	}
	return js.ValueOf(activeHistory.Undo(activeCanvas))
}

// jsHistoryRedo: () → bool
func jsHistoryRedo(_ js.Value, _ []js.Value) any {
	if activeHistory == nil || activeCanvas == nil {
		return js.ValueOf(false)
	}
	return js.ValueOf(activeHistory.Redo(activeCanvas))
}

// jsHistoryInfo: () → {canUndo, canRedo, undoCount, redoCount}
func jsHistoryInfo(_ js.Value, _ []js.Value) any {
	ret := js.Global().Get("Object").New()
	if activeHistory == nil {
		ret.Set("canUndo", false)
		ret.Set("canRedo", false)
		ret.Set("undoCount", 0)
		ret.Set("redoCount", 0)
		return ret
	}
	ret.Set("canUndo", activeHistory.CanUndo())
	ret.Set("canRedo", activeHistory.CanRedo())
	ret.Set("undoCount", activeHistory.UndoCount())
	ret.Set("redoCount", activeHistory.RedoCount())
	return ret
}

// --------------------------------------------------------------------------
// Layer Groups API
// --------------------------------------------------------------------------

// jsCanvasSetLayerGroup: (layerName, groupName) → bool
func jsCanvasSetLayerGroup(_ js.Value, args []js.Value) any {
	if activeCanvas == nil || len(args) < 2 {
		return js.ValueOf(false)
	}
	return js.ValueOf(activeCanvas.SetLayerGroup(args[0].String(), args[1].String()))
}

// jsCanvasGetGroups: () → [string]
func jsCanvasGetGroups(_ js.Value, _ []js.Value) any {
	if activeCanvas == nil {
		return js.ValueOf([]any{})
	}
	groups := activeCanvas.Groups()
	arr := make([]any, len(groups))
	for i, g := range groups {
		arr[i] = g
	}
	return js.ValueOf(arr)
}

// jsCanvasFlattenGroup: (groupName) → void
func jsCanvasFlattenGroup(_ js.Value, args []js.Value) any {
	if activeCanvas == nil || len(args) < 1 {
		return js.ValueOf("need canvas + groupName")
	}
	activeCanvas.FlattenGroup(args[0].String())
	return js.Undefined()
}

// --------------------------------------------------------------------------
// Layer Styles API
// --------------------------------------------------------------------------

// jsCanvasSetLayerStyle: (layerName string, styleObj object) → void
func jsCanvasSetLayerStyle(_ js.Value, args []js.Value) any {
	if activeCanvas == nil || len(args) < 2 {
		return js.ValueOf("need canvas + layerName + style")
	}
	l := activeCanvas.FindLayer(args[0].String())
	if l == nil {
		return js.ValueOf("layer not found")
	}
	styleJS := args[1]
	if styleJS.Type() != js.TypeObject {
		return js.ValueOf("style must be object")
	}

	style := vango.LayerStyle{}

	// Drop Shadow
	if ds := styleJS.Get("dropShadow"); !ds.IsUndefined() && ds.Type() == js.TypeObject {
		style.DropShadow = parseShadowStyle(ds)
	}
	// Inner Shadow
	if is := styleJS.Get("innerShadow"); !is.IsUndefined() && is.Type() == js.TypeObject {
		style.InnerShadow = parseShadowStyle(is)
	}
	// Outer Glow
	if og := styleJS.Get("outerGlow"); !og.IsUndefined() && og.Type() == js.TypeObject {
		style.OuterGlow = parseGlowStyle(og)
	}
	// Inner Glow
	if ig := styleJS.Get("innerGlow"); !ig.IsUndefined() && ig.Type() == js.TypeObject {
		style.InnerGlow = parseGlowStyle(ig)
	}
	// Stroke
	if sk := styleJS.Get("stroke"); !sk.IsUndefined() && sk.Type() == js.TypeObject {
		style.Stroke = &vango.StrokeStyle{
			Color:    parseColorJS(sk, 0, 0, 0, 255),
			Size:     getIntJS(sk, "size", 2),
			Position: getStringJS(sk, "position", "outside"),
			Opacity:  getFloatJS(sk, "opacity", 1.0),
		}
	}
	// Bevel/Emboss
	if bv := styleJS.Get("bevelEmboss"); !bv.IsUndefined() && bv.Type() == js.TypeObject {
		style.BevelEmboss = &vango.BevelStyle{
			Style:    getStringJS(bv, "style", "inner_bevel"),
			Depth:    getFloatJS(bv, "depth", 50),
			Size:     getFloatJS(bv, "size", 5),
			Angle:    getFloatJS(bv, "angle", 135),
			Altitude: getFloatJS(bv, "altitude", 30),
		}
	}

	l.SetStyle(style)
	return js.Undefined()
}

func parseShadowStyle(obj js.Value) *vango.ShadowStyle {
	return &vango.ShadowStyle{
		Color:    parseColorJS(obj, 0, 0, 0, 255),
		Opacity:  getFloatJS(obj, "opacity", 0.75),
		Angle:    getFloatJS(obj, "angle", 135),
		Distance: getFloatJS(obj, "distance", 5),
		Spread:   getFloatJS(obj, "spread", 0),
		Size:     getFloatJS(obj, "size", 5),
	}
}

func parseGlowStyle(obj js.Value) *vango.GlowStyle {
	return &vango.GlowStyle{
		Color:   parseColorJS(obj, 255, 255, 0, 255),
		Opacity: getFloatJS(obj, "opacity", 0.75),
		Size:    getFloatJS(obj, "size", 10),
		Spread:  getFloatJS(obj, "spread", 0),
	}
}

func parseColorJS(obj js.Value, dr, dg, db, da uint8) color.NRGBA {
	r := getIntJS(obj, "r", int(dr))
	g := getIntJS(obj, "g", int(dg))
	b := getIntJS(obj, "b", int(db))
	a := getIntJS(obj, "a", int(da))
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
}

func getFloatJS(obj js.Value, key string, fallback float64) float64 {
	v := obj.Get(key)
	if v.IsUndefined() || v.IsNull() {
		return fallback
	}
	return v.Float()
}

func getIntJS(obj js.Value, key string, fallback int) int {
	v := obj.Get(key)
	if v.IsUndefined() || v.IsNull() {
		return fallback
	}
	return v.Int()
}

func getStringJS(obj js.Value, key, fallback string) string {
	v := obj.Get(key)
	if v.IsUndefined() || v.IsNull() {
		return fallback
	}
	return v.String()
}

func registerVangoAPI() {
	api := js.Global().Get("Object").New()
	api.Set("ready", js.ValueOf(true))
	api.Set("version", js.ValueOf(wasmVersion))

	names := vango.EffectNames()
	// js.ValueOf requires []any for JS arrays; []string is not accepted directly.
	effects := make([]any, len(names))
	for i, e := range names {
		effects[i] = e
	}
	api.Set("effects", js.ValueOf(effects))

	blendModes := vango.BlendModeNames()
	blends := make([]any, len(blendModes))
	for i, b := range blendModes {
		blends[i] = b
	}
	api.Set("blendModes", js.ValueOf(blends))

	api.Set("processImage", js.FuncOf(jsProcessImage))
	api.Set("processImagePNG", js.FuncOf(jsProcessImagePNG))

	// Layer/Canvas API
	api.Set("canvasCreate", js.FuncOf(jsCanvasCreate))
	api.Set("canvasAddLayer", js.FuncOf(jsCanvasAddLayer))
	api.Set("canvasSetLayerProps", js.FuncOf(jsCanvasSetLayerProps))
	api.Set("canvasApplyLayerEffects", js.FuncOf(jsCanvasApplyLayerEffects))
	api.Set("canvasRemoveLayer", js.FuncOf(jsCanvasRemoveLayer))
	api.Set("canvasDuplicateLayer", js.FuncOf(jsCanvasDuplicateLayer))
	api.Set("canvasMergeDown", js.FuncOf(jsCanvasMergeDown))
	api.Set("canvasFlatten", js.FuncOf(jsCanvasFlatten))
	api.Set("canvasGetLayers", js.FuncOf(jsCanvasGetLayers))

	// History API
	api.Set("historyInit", js.FuncOf(jsHistoryInit))
	api.Set("historySave", js.FuncOf(jsHistorySave))
	api.Set("historyUndo", js.FuncOf(jsHistoryUndo))
	api.Set("historyRedo", js.FuncOf(jsHistoryRedo))
	api.Set("historyInfo", js.FuncOf(jsHistoryInfo))

	// Layer Groups API
	api.Set("canvasSetLayerGroup", js.FuncOf(jsCanvasSetLayerGroup))
	api.Set("canvasGetGroups", js.FuncOf(jsCanvasGetGroups))
	api.Set("canvasFlattenGroup", js.FuncOf(jsCanvasFlattenGroup))

	// Layer Styles API
	api.Set("canvasSetLayerStyle", js.FuncOf(jsCanvasSetLayerStyle))

	js.Global().Set("vango", api)
}

func main() {
	registerVangoAPI()
	select {}
}
