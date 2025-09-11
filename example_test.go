package vango_test

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/SimonWaldherr/vango"
)

// Example_basicPipeline shows how to use the fluent Pipeline API.
func Example_basicPipeline() {
	f, err := os.Open("demo.jpg")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	img, _, _ := vango.Decode(f)

	out := vango.From(img).
		GaussianBlur(0.2, 0).
		Contrast(1.10).
		Saturation(1.15).
		Sepia(0.15).
		//Unsharp(0.6, 1.0, 0).
		ResizeNearest(800, 600).
		Image()

	of, _ := os.Create("demo.out.jpg")
	defer of.Close()
	_ = png.Encode(of, out)

	fmt.Println("ok")
	// Output: ok
}

// Example_whiteBalance demonstrates white-balance correction
// using a reference rectangle (pretend the top-left 50x50 is a gray card).
func Example_whiteBalance() {
	f, _ := os.Open("demo.jpg")
	defer f.Close()
	img, _, _ := vango.Decode(f)

	n := vango.ToNRGBA(img)
	ref := image.Rect(0, 0, 50, 50)
	out := vango.WhiteBalanceByRect(n, ref)

	fmt.Println(out.Bounds().Size())
	// Output: (1500,1000)
}

// Example_analysis runs image analysis and prints some stats.
func Example_analysis() {
	f, _ := os.Open("demo.jpg")
	defer f.Close()
	img, _, _ := vango.Decode(f)

	stats := vango.Analyze(img, 3)

	fmt.Printf("Resolution %dx%d Aspect %.2f Avg R%d G%d B%d\n",
		stats.Width, stats.Height, stats.Aspect,
		stats.Average.R, stats.Average.G, stats.Average.B)

	// Output:
	// Resolution 1500x1000 Aspect 1.50 Avg R112 G132 B140
}

// Example_encodeOptions shows using DecodeWithOptions with auto-orient and size guard.
func Example_encodeOptions() {
	f, _ := os.Open("demo.jpg")
	defer f.Close()

	img, format, err := vango.DecodeWithOptions(f, vango.WithAutoOrient(), vango.WithMaxPixels(10_000_000))
	if err != nil {
		panic(err)
	}

	fmt.Printf("decoded as %s %dx%d\n", format, img.Bounds().Dx(), img.Bounds().Dy())
	// Output:
	// decoded as jpeg 1500x1000
}

// Example_textWatermark draws text directly onto an image.
func Example_textWatermark() {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 80))

	out := vango.From(img).
		DrawText("Hello Vango!", image.Pt(10, 20), color.NRGBA{255, 0, 0, 255}, 2).
		Image()

	fmt.Println(out.Bounds().Dx(), out.Bounds().Dy())
	// Output: 200 80
}

// Example_newEffects demonstrates grayscale + vignette on a synthetic image
func Example_newEffects_grayscaleVignette() {
	// create a simple gradient image
	img := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			i := y*img.Stride + x*4
			img.Pix[i+0] = uint8(x)
			img.Pix[i+1] = uint8(y)
			img.Pix[i+2] = uint8((x + y) / 2)
			img.Pix[i+3] = 255
		}
	}
	out := vango.From(img).
		Grayscale().
		Vignette(0.6).
		Image()

	fmt.Println(out.Bounds().Dx(), out.Bounds().Dy())
	// Output: 200 200
}

// Example_newEffects_embossSolarize demonstrates emboss then solarize
func Example_newEffects_embossSolarize() {
	img := image.NewNRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			i := y*img.Stride + x*4
			img.Pix[i+0] = uint8((x * y) % 256)
			img.Pix[i+1] = uint8((x*2 + y) % 256)
			img.Pix[i+2] = uint8((x + y*2) % 256)
			img.Pix[i+3] = 255
		}
	}
	out := vango.From(img).
		Emboss(0.5).
		Solarize(128).
		Image()
	fmt.Println(out.Bounds().Dx(), out.Bounds().Dy())
	// Output: 120 80
}
