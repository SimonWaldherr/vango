package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SimonWaldherr/vango"
)

// emptyRect returns a zero rectangle used by whitebalance to select full-image auto mode.
func emptyRect() image.Rectangle { return image.Rectangle{} }

func splitCommands(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' || r == '\n' })
}

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

func smartCropRect(b image.Rectangle, outW, outH int) image.Rectangle {
	if outW <= 0 || outH <= 0 || b.Empty() {
		return b
	}
	srcW := b.Dx()
	srcH := b.Dy()
	target := float64(outW) / float64(outH)
	srcAspect := float64(srcW) / float64(srcH)

	cropW, cropH := srcW, srcH
	if srcAspect > target {
		cropW = int(math.Round(float64(srcH) * target))
	} else if srcAspect < target {
		cropH = int(math.Round(float64(srcW) / target))
	}
	if cropW < 1 {
		cropW = 1
	}
	if cropH < 1 {
		cropH = 1
	}
	x0 := b.Min.X + (srcW-cropW)/2
	y0 := b.Min.Y + (srcH-cropH)/2
	return image.Rect(x0, y0, x0+cropW, y0+cropH)
}

// autoBrightnessDelta targets mid-gray average luma (0.5), clamped to +/-0.3.
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

// autoVibranceFactor raises average saturation toward ~0.55, capped at 1.8x.
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
			p = p.Rotate(parseFloatArg(args[0], 0), "bilinear", color.NRGBA{255, 255, 255, 255})
		}
	case "skew":
		if len(args) >= 2 {
			p = p.Skew(parseFloatArg(args[0], 0), parseFloatArg(args[1], 0), "bilinear", color.NRGBA{255, 255, 255, 255})
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
			x0 := parseIntArg(args[0], 0)
			y0 := parseIntArg(args[1], 0)
			x1 := parseIntArg(args[2], 0)
			y1 := parseIntArg(args[3], 0)
			p = p.Crop(image.Rect(x0, y0, x1, y1))
		}
	case "smartcrop", "smart_crop":
		if len(args) >= 2 {
			w := parseIntArg(args[0], 1)
			h := parseIntArg(args[1], 1)
			b := p.Image().Bounds()
			rect := smartCropRect(b, w, h)
			p = p.Crop(rect).ResizeBilinear(w, h)
		}
	case "trim":
		p = p.Trim(color.NRGBA{255, 255, 255, 255}, 8)
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
	case "text":
		if len(args) >= 3 {
			p = p.DrawText(args[0], image.Pt(parseIntArg(args[1], 0), parseIntArg(args[2], 0)), color.NRGBA{0, 0, 0, 255}, 2)
		}
	case "whitebalance", "wb":
		rect := emptyRect()
		if len(args) >= 4 {
			rect = image.Rect(parseIntArg(args[0], 0), parseIntArg(args[1], 0), parseIntArg(args[2], 50), parseIntArg(args[3], 50))
		}
		p = p.WhiteBalance(rect)
	case "autocontrast", "auto_contrast":
		p = p.Equalize()
	case "autobrightness", "auto_brightness":
		n := vango.ToNRGBA(p.Image())
		p = vango.From(n).Brightness(autoBrightnessDelta(n))
	case "autovibrance", "auto_vibrance":
		n := vango.ToNRGBA(p.Image())
		p = vango.From(n).Saturation(autoVibranceFactor(n))
	case "autocolor", "auto_color":
		p = p.WhiteBalance(emptyRect())
		p = p.Equalize()
		n := vango.ToNRGBA(p.Image())
		p = vango.From(n).Brightness(autoBrightnessDelta(n))
		brightnessAdjusted := p.Image()
		p = vango.From(brightnessAdjusted).Saturation(autoVibranceFactor(brightnessAdjusted))
	case "autofull", "auto_full", "autoenhance", "auto_enhance":
		// Full auto: white balance + noise reduction + auto contrast + auto brightness + auto vibrance
		p = p.WhiteBalance(emptyRect())
		p = p.NoiseReduction(1)
		p = p.Equalize()
		n := vango.ToNRGBA(p.Image())
		p = vango.From(n).Brightness(autoBrightnessDelta(n))
		brightnessAdjusted := p.Image()
		p = vango.From(brightnessAdjusted).Saturation(autoVibranceFactor(brightnessAdjusted))
	case "noisereduction", "noise_reduction", "denoise":
		radius := 1
		if len(args) >= 1 {
			radius = parseIntArg(args[0], 1)
		}
		p = p.NoiseReduction(radius)
	case "collage":
		// collage <file> [direction]
		// direction: horizontal (default) or vertical
		if len(args) >= 1 {
			collagePath := filepath.Clean(args[0])
			if collagePath == ".." || strings.HasPrefix(collagePath, ".."+string(filepath.Separator)) {
				fmt.Fprintln(os.Stderr, "warning: collage path must not traverse parent directories")
				break
			}
			cf, err := os.Open(collagePath)
			if err == nil {
				defer func() {
					if cerr := cf.Close(); cerr != nil {
						fmt.Fprintf(os.Stderr, "warning: closing collage file %s: %v\n", collagePath, cerr)
					}
				}()
				other, _, derr := vango.Decode(cf)
				if derr == nil {
					dir := "horizontal"
					if len(args) >= 2 {
						dir = args[1]
					}
					p = vango.From(vango.AlignImages([]image.Image{p.Image(), other}, dir, color.NRGBA{255, 255, 255, 255}))
				} else {
					fmt.Fprintf(os.Stderr, "warning: decode collage image %s: %v\n", collagePath, derr)
				}
			} else {
				fmt.Fprintf(os.Stderr, "warning: open collage image %s: %v\n", collagePath, err)
			}
		}
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
	case "watermark":
		if len(args) >= 4 {
			if strings.Contains(args[0], "..") {
				fmt.Fprintln(os.Stderr, "warning: watermark path must not contain '..'")
				break
			}
			markPath := filepath.Clean(args[0])
			mf, err := os.Open(markPath)
			if err == nil {
				defer func() {
					if cerr := mf.Close(); cerr != nil {
						fmt.Fprintf(os.Stderr, "warning: closing watermark file %s: %v\n", markPath, cerr)
					}
				}()
				mark, _, derr := vango.Decode(mf)
				if derr == nil {
					x := parseIntArg(args[1], 0)
					y := parseIntArg(args[2], 0)
					opacity := parseFloatArg(args[3], 0.5)
					p = p.Watermark(mark, image.Pt(x, y), opacity)
				} else {
					fmt.Fprintf(os.Stderr, "warning: decode watermark image %s: %v\n", markPath, derr)
				}
			} else {
				fmt.Fprintf(os.Stderr, "warning: open watermark image %s: %v\n", markPath, err)
			}
		}
	case "apply":
		if len(args) >= 1 {
			p = p.Apply(args[0])
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", name)
	}
	return p
}

func main() {
	inPath := flag.String("in", "", "input image path (required)")
	outPath := flag.String("out", "out.png", "output image path")
	cmds := flag.String("cmds", "", "comma-separated commands, e.g. \"blur 1.2; contrast 1.1; sepia 0.2\"")
	flag.Parse()

	if *inPath == "" {
		fmt.Fprintln(os.Stderr, "missing -in file")
		flag.Usage()
		os.Exit(2)
	}

	// Load image
	f, err := os.Open(*inPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	img, _, err := vango.Decode(f)
	if err != nil {
		panic(err)
	}

	p := vango.From(img)

	// Parse commands
	commands := splitCommands(*cmds)
	for _, raw := range commands {
		p = applyCommand(p, raw)
	}

	// Save output based on extension
	outFile, err := os.Create(*outPath)
	if err != nil {
		panic(err)
	}
	defer outFile.Close()

	ext := strings.ToLower(*outPath)
	switch {
	case strings.HasSuffix(ext, ".jpg"), strings.HasSuffix(ext, ".jpeg"):
		err = p.EncodeJPEG(outFile, 90)
	case strings.HasSuffix(ext, ".gif"):
		err = p.EncodeGIF(outFile)
	default:
		err = p.EncodePNG(outFile)
	}
	if err != nil {
		panic(err)
	}

	fmt.Println("Saved", *outPath)
}
