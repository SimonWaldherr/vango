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
	api.Set("processImage", js.FuncOf(jsProcessImage))
	api.Set("processImagePNG", js.FuncOf(jsProcessImagePNG))
	js.Global().Set("vango", api)
}

func main() {
	registerVangoAPI()
	select {}
}
