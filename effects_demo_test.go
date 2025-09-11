package vango_test

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/SimonWaldherr/vango"
)

// TestGenerateDemos generates demo.<effect>.jpg files for every supported effect.
// Run: go test -run TestGenerateDemos
func TestGenerateDemos(t *testing.T) {
	// Try to open demo.jpg if present, otherwise use a synthetic image.
	var img image.Image
	f, err := os.Open("demo.jpg")
	if err == nil {
		defer f.Close()
		img, _, _ = vango.Decode(f)
	} else {
		// create synthetic image
		n := image.NewNRGBA(image.Rect(0, 0, 800, 600))
		for y := 0; y < 600; y++ {
			for x := 0; x < 800; x++ {
				i := y*n.Stride + x*4
				n.Pix[i+0] = uint8((x * 3) % 256)
				n.Pix[i+1] = uint8((y * 5) % 256)
				n.Pix[i+2] = uint8((x+y) % 256)
				n.Pix[i+3] = 255
			}
		}
		img = n
	}

	outDir := "."
	save := func(name string, im image.Image) {
		path := filepath.Join(outDir, "demo."+name+".jpg")
		of, err := os.Create(path)
		if err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
		if err := vango.EncodeJPEG(of, im, 90); err != nil {
			of.Close()
			t.Fatalf("encode %s: %v", path, err)
		}
		of.Close()
	}

	// list of effects to generate
	// use reasonable default args
	effects := []struct{
		name string
		fn func(image.Image) image.Image
	}{
		{"blur", func(i image.Image) image.Image { return vango.GaussianBlur(i, 1.2, 0) }},
		{"unsharp", func(i image.Image) image.Image { return vango.UnsharpMask(i, 0.6, 1.0, 0) }},
		{"contrast", func(i image.Image) image.Image { return vango.AdjustContrast(i, 1.1) }},
		{"saturation", func(i image.Image) image.Image { return vango.AdjustSaturation(i, 1.15) }},
		{"brightness", func(i image.Image) image.Image { return vango.AdjustBrightness(i, 0.05) }},
		{"hue", func(i image.Image) image.Image { return vango.AdjustHue(i, 15) }},
		{"sepia", func(i image.Image) image.Image { return vango.Sepia(i, 0.25) }},
		{"invert", func(i image.Image) image.Image { return vango.Invert(i) }},
		{"gamma", func(i image.Image) image.Image { return vango.Gamma(i, 1.2) }},
		{"rotate", func(i image.Image) image.Image { return vango.Rotate(i, 15, "bilinear", color.NRGBA{255,255,255,255}) }},
		{"resize", func(i image.Image) image.Image { return vango.ResizeBilinear(i, 400, 300) }},
		{"crop", func(i image.Image) image.Image { b := i.Bounds(); return vango.Crop(i, image.Rect(b.Min.X+10, b.Min.Y+10, b.Min.X+410, b.Min.Y+310)) }},
		{"pixelate", func(i image.Image) image.Image { return vango.Pixelate(i, 8) }},
		{"posterize", func(i image.Image) image.Image { return vango.Posterize(i, 6) }},
		{"threshold", func(i image.Image) image.Image { return vango.Threshold(i, 128) }},
		{"equalize", func(i image.Image) image.Image { return vango.EqualizeLuma(i) }},
		{"tonemap", func(i image.Image) image.Image { return vango.TonemapReinhard(i, 1.0) }},
		{"dither", func(i image.Image) image.Image { return vango.DitherFS(i, 4) }},
		{"text", func(i image.Image) image.Image { return vango.From(i).DrawText("Demo", image.Pt(10,10), color.NRGBA{255,0,0,255}, 2).Image() }},
		{"grayscale", func(i image.Image) image.Image { return vango.From(i).Grayscale().Image() }},
		{"solarize", func(i image.Image) image.Image { return vango.Solarize(i, 128) }},
		{"emboss", func(i image.Image) image.Image { return vango.Emboss(i, 0.6) }},
		{"vignette", func(i image.Image) image.Image { return vango.Vignette(i, 0.6) }},
		{"whitebalance", func(i image.Image) image.Image { b := i.Bounds(); return vango.WhiteBalanceByRect(vango.ToNRGBA(i), image.Rect(b.Min.X, b.Min.Y, b.Min.X+50, b.Min.Y+50)) }},
	}

	for _, e := range effects {
		im := e.fn(img)
		save(e.name, im)
	}
}
