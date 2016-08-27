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
