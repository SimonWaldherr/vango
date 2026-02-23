package main

import (
	"bytes"
	"image"
	"image/color"
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
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatalf("whitebalance default and auto should match")
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
