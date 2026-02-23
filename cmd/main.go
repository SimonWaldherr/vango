package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"os"
	"strconv"
	"strings"

	"github.com/SimonWaldherr/vango"
)

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
		rect := image.Rect(0, 0, 50, 50)
		if len(args) >= 4 {
			rect = image.Rect(parseIntArg(args[0], rect.Min.X), parseIntArg(args[1], rect.Min.Y), parseIntArg(args[2], rect.Max.X), parseIntArg(args[3], rect.Max.Y))
		}
		p = p.WhiteBalance(rect)
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
