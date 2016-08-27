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
	commands := strings.Split(*cmds, ";")
	for _, raw := range commands {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		toks := strings.Fields(raw)
		name := strings.ToLower(toks[0])
		args := toks[1:]

		switch name {
		case "blur":
			if len(args) >= 1 {
				sigma, _ := strconv.ParseFloat(args[0], 64)
				p = p.GaussianBlur(sigma, 0)
			}
		case "unsharp":
			if len(args) >= 2 {
				amount, _ := strconv.ParseFloat(args[0], 64)
				sigma, _ := strconv.ParseFloat(args[1], 64)
				p = p.Unsharp(amount, sigma, 0)
			}
		case "contrast":
			if len(args) >= 1 {
				fac, _ := strconv.ParseFloat(args[0], 64)
				p = p.Contrast(fac)
			}
		case "saturation":
			if len(args) >= 1 {
				fac, _ := strconv.ParseFloat(args[0], 64)
				p = p.Saturation(fac)
			}
		case "brightness":
			if len(args) >= 1 {
				delta, _ := strconv.ParseFloat(args[0], 64)
				p = p.Brightness(delta)
			}
		case "hue":
			if len(args) >= 1 {
				deg, _ := strconv.ParseFloat(args[0], 64)
				p = p.Hue(deg)
			}
		case "sepia":
			if len(args) >= 1 {
				amt, _ := strconv.ParseFloat(args[0], 64)
				p = p.Sepia(amt)
			}
		case "invert":
			p = p.Invert()
		case "gamma":
			if len(args) >= 1 {
				g, _ := strconv.ParseFloat(args[0], 64)
				p = p.Gamma(g)
			}
		case "rotate":
			if len(args) >= 1 {
				deg, _ := strconv.ParseFloat(args[0], 64)
				p = p.Rotate(deg, "bilinear", color.NRGBA{255, 255, 255, 255})
			}
		case "resize":
			if len(args) >= 2 {
				w, _ := strconv.Atoi(args[0])
				h, _ := strconv.Atoi(args[1])
				p = p.ResizeBilinear(w, h)
			}
		case "crop":
			if len(args) >= 4 {
				x0, _ := strconv.Atoi(args[0])
				y0, _ := strconv.Atoi(args[1])
				x1, _ := strconv.Atoi(args[2])
				y1, _ := strconv.Atoi(args[3])
				p = p.Crop(image.Rect(x0, y0, x1, y1))
			}
		case "trim":
			p = p.Trim(color.NRGBA{255, 255, 255, 255}, 8)
		case "pixelate":
			if len(args) >= 1 {
				b, _ := strconv.Atoi(args[0])
				p = p.Pixelate(b)
			}
		case "posterize":
			if len(args) >= 1 {
				lv, _ := strconv.Atoi(args[0])
				p = p.Posterize(lv)
			}
		case "threshold":
			if len(args) >= 1 {
				cut, _ := strconv.Atoi(args[0])
				p = p.Threshold(uint8(cut))
			}
		case "equalize":
			p = p.Equalize()
		case "tonemap":
			if len(args) >= 1 {
				exp, _ := strconv.ParseFloat(args[0], 64)
				p = p.Tonemap(exp)
			}
		case "dither":
			if len(args) >= 1 {
				lv, _ := strconv.Atoi(args[0])
				p = p.Dither(lv)
			}
		case "text":
			if len(args) >= 3 {
				txt := args[0]
				x, _ := strconv.Atoi(args[1])
				y, _ := strconv.Atoi(args[2])
				p = p.DrawText(txt, image.Pt(x, y), color.NRGBA{0, 0, 0, 255}, 2)
			}
		default:
			fmt.Fprintln(os.Stderr, "unknown command:", name)
		}
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
