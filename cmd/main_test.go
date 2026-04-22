package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/SimonWaldherr/vango"
)

func TestSplitCommandsSupportsMultipleSeparators(t *testing.T) {
	cmds := splitCommands("blur 1.2, contrast 1.1; sepia 0.2\ninvert")
	if len(cmds) != 4 {
		t.Fatalf("expected 4 commands, got %d (%v)", len(cmds), cmds)
	}
}

func TestApplyCommandResizeNearestHasExpectedSize(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 20, 10))
	out := applyCommand(vango.From(img), "resize_nearest 8 6").Image()
	if got := out.Bounds().Size(); got.X != 8 || got.Y != 6 {
		t.Fatalf("unexpected output size after resize_nearest: %v", got)
	}
}

func TestApplyCommandAdditionalEffects(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 20, 10))
	p := vango.From(img)

	for _, cmd := range []string{
		"resize_nearest 8 6",
		"skew 0.1 0.0",
		"whitebalance",
		"wb 0 0 4 4",
	} {
		p = applyCommand(p, cmd)
	}

	out := p.Image()
	if got := out.Bounds().Size(); got.X == 0 || got.Y == 0 {
		t.Fatalf("unexpected empty output size after additional effects pipeline: %v", got)
	}
}

func TestApplyCommandWhiteBalanceAutoMatchesDefault(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(10 + x*5), G: uint8(20 + y*5), B: 30, A: 255})
		}
	}

	a := applyCommand(vango.From(img), "whitebalance").Image()
	b := applyCommand(vango.From(img), "whitebalance auto").Image()
	c := applyCommand(vango.From(img), "whitebalance 0 0 4 4").Image()
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatalf("whitebalance default and auto should match")
	}
	if !bytes.Equal(a.Pix, c.Pix) {
		t.Fatalf("whitebalance rectangle mode should remain supported")
	}
}

func TestApplyCommandAutoModes(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(30 + x), G: uint8(40 + y), B: uint8(50 + x/2), A: 255})
		}
	}
	p := vango.From(img)
	for _, cmd := range []string{
		"autocontrast",
		"auto_brightness",
		"autovibrance",
		"auto_color",
	} {
		p = applyCommand(p, cmd)
	}
	out := p.Image()
	if got := out.Bounds().Size(); got.X != 12 || got.Y != 8 {
		t.Fatalf("auto modes must preserve image size: %v", got)
	}
}

func TestApplyCommandEdgeAndSmartCrop(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 20, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(20 + x), G: uint8(30 + y), B: uint8(40 + x + y), A: 255})
		}
	}
	p := vango.From(img)
	p = applyCommand(p, "edge")
	p = applyCommand(p, "smartcrop 6 6")
	out := p.Image()
	if got := out.Bounds().Size(); got.X != 6 || got.Y != 6 {
		t.Fatalf("edge+smartcrop should produce requested size: %v", got)
	}
}

func TestApplyCommandNewDistortionEffects(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x * 8), G: uint8(y * 8), B: 128, A: 255})
		}
	}

	cmds := []string{
		"twirl 1.0",
		"twirl 0.5 10",
		"spherize 0.5",
		"wave 5 5 20 20",
		"ripple 5 20",
		"polar_coordinates to",
		"polar_coordinates from",
		"pinch 0.4",
		"pinch 0.4 12",
	}
	for _, cmd := range cmds {
		out := applyCommand(vango.From(img), cmd).Image()
		if out.Bounds().Empty() {
			t.Errorf("command %q produced empty image", cmd)
		}
	}
}

func TestApplyCommandProAdjustments(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(50 + x*5), G: uint8(60 + y*5), B: 100, A: 255})
		}
	}

	cmds := []string{
		"vibrance 0.8",
		"dehaze 0.5",
		"shadow_highlight 0.3 0.3",
		"dodge 0.3 midtones",
		"burn 0.3 shadows",
		"curves 0 0 0.5 0.6 1 1",
		"gradient_map 000000,0 FFFFFF,1",
		"tint FF8800 0.3",
		"channel_curves r:0,0,1,1 g:0,0,1,1 b:0,0,1,1",
	}
	for _, cmd := range cmds {
		out := applyCommand(vango.From(img), cmd).Image()
		if out.Bounds().Empty() {
			t.Errorf("command %q produced empty image", cmd)
		}
	}
}

func TestApplyCommandSeamCarve(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x * 12), G: uint8(y * 12), B: 100, A: 255})
		}
	}
	out := applyCommand(vango.From(img), "seam_carve 16 16").Image()
	if got := out.Bounds().Size(); got.X != 16 || got.Y != 16 {
		t.Fatalf("seam_carve produced unexpected size: %v", got)
	}
}

func TestApplyCommandPerspectiveTransform(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x * 6), G: uint8(y * 6), B: 80, A: 255})
		}
	}
	// corners: TL(2,2) TR(38,2) BR(38,38) BL(2,38), output 30x30
	out := applyCommand(vango.From(img), "perspective 2 2 38 2 38 38 2 38 30 30").Image()
	if got := out.Bounds().Size(); got.X != 30 || got.Y != 30 {
		t.Fatalf("perspective produced unexpected size: %v", got)
	}
}

func TestParseHexColor(t *testing.T) {
	tests := []struct {
		input string
		want  color.NRGBA
	}{
		{"FF0000", color.NRGBA{255, 0, 0, 255}},
		{"00FF00", color.NRGBA{0, 255, 0, 255}},
		{"0000FF", color.NRGBA{0, 0, 255, 255}},
		{"FF000080", color.NRGBA{255, 0, 0, 128}},
		{"#ABCDEF", color.NRGBA{0xAB, 0xCD, 0xEF, 255}},
	}
	for _, tc := range tests {
		got := parseHexColor(tc.input)
		if got != tc.want {
			t.Errorf("parseHexColor(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestParseGradientStops(t *testing.T) {
	args := []string{"000000,0", "FF8800,0.5", "FFFFFF,1"}
	stops := parseGradientStops(args)
	if len(stops) != 3 {
		t.Fatalf("expected 3 stops, got %d", len(stops))
	}
	if stops[0].Color.R != 0 || stops[2].Color.R != 255 {
		t.Error("unexpected stop colors")
	}
	if stops[1].Pos != 0.5 {
		t.Errorf("expected pos 0.5, got %f", stops[1].Pos)
	}
}

func TestApplyCommandWatermark(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 30, 20))
	mark := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			mark.Set(x, y, color.NRGBA{R: 255, A: 255})
		}
	}

	tmp, err := os.CreateTemp(t.TempDir(), "mark-*.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(tmp, mark); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}

	out := applyCommand(vango.From(img), "watermark "+tmp.Name()+" 2 2 0.7").Image()
	if got := out.Bounds().Size(); got.X != 30 || got.Y != 20 {
		t.Fatalf("watermark should preserve output size: %v", got)
	}
}
