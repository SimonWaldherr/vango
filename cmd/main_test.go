package main

import (
	"image"
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
