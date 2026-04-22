package vango

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestSaveLoadProjectRoundtrip(t *testing.T) {
	// Build a two-layer canvas with a mask on the top layer.
	c := NewCanvas(64, 64)

	bg := NewSolidLayer("Background", 64, 64, color.NRGBA{100, 150, 200, 255})
	bg.ZOrder = 0
	c.AddLayer(bg)

	fg := NewLayer("Foreground", testImage(64, 64))
	fg.ZOrder = 1
	fg.Opacity = 0.75
	fg.Blend = BlendMultiply
	fg.OffsetX = 4
	fg.OffsetY = 8
	fg.Group = "grp1"

	// Add a mask
	mask := image.NewGray(image.Rect(0, 0, 64, 64))
	for i := range mask.Pix {
		mask.Pix[i] = uint8(i % 256)
	}
	fg.Mask = mask
	c.AddLayer(fg)

	// Save
	var buf bytes.Buffer
	if err := SaveProject(c, &buf); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("SaveProject wrote nothing")
	}

	// Load
	c2, err := LoadProject(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	if c2.Width != 64 || c2.Height != 64 {
		t.Errorf("canvas size: got %dx%d, want 64x64", c2.Width, c2.Height)
	}
	if len(c2.Layers) != 2 {
		t.Fatalf("layer count: got %d, want 2", len(c2.Layers))
	}

	// Check background layer
	l0 := c2.Layers[0]
	if l0.Name != "Background" {
		t.Errorf("layer[0].Name: got %q, want %q", l0.Name, "Background")
	}
	if l0.Blend != BlendNormal {
		t.Errorf("layer[0].Blend: got %v, want Normal", l0.Blend)
	}
	if l0.ZOrder != 0 {
		t.Errorf("layer[0].ZOrder: got %d, want 0", l0.ZOrder)
	}

	// Check foreground layer attributes
	l1 := c2.Layers[1]
	if l1.Name != "Foreground" {
		t.Errorf("layer[1].Name: got %q, want %q", l1.Name, "Foreground")
	}
	if l1.Blend != BlendMultiply {
		t.Errorf("layer[1].Blend: got %v, want Multiply", l1.Blend)
	}
	if l1.Opacity != 0.75 {
		t.Errorf("layer[1].Opacity: got %f, want 0.75", l1.Opacity)
	}
	if l1.OffsetX != 4 || l1.OffsetY != 8 {
		t.Errorf("layer[1].Offset: got (%d,%d), want (4,8)", l1.OffsetX, l1.OffsetY)
	}
	if l1.Group != "grp1" {
		t.Errorf("layer[1].Group: got %q, want %q", l1.Group, "grp1")
	}

	// Check mask was preserved
	if l1.Mask == nil {
		t.Fatal("layer[1].Mask is nil after round-trip")
	}
	if l1.Mask.Bounds().Dx() != 64 || l1.Mask.Bounds().Dy() != 64 {
		t.Errorf("mask size: got %v, want 64x64", l1.Mask.Bounds())
	}

	// Check pixel data is unchanged
	orig := c.Layers[1].Image
	loaded := l1.Image
	if orig.Bounds() != loaded.Bounds() {
		t.Errorf("pixel bounds: orig=%v loaded=%v", orig.Bounds(), loaded.Bounds())
	}
	for i := 0; i+3 < len(orig.Pix); i += 4 {
		if orig.Pix[i] != loaded.Pix[i] ||
			orig.Pix[i+1] != loaded.Pix[i+1] ||
			orig.Pix[i+2] != loaded.Pix[i+2] ||
			orig.Pix[i+3] != loaded.Pix[i+3] {
			t.Errorf("pixel[%d] mismatch: orig %v loaded %v",
				i, orig.Pix[i:i+4], loaded.Pix[i:i+4])
			break
		}
	}
}

func TestLoadProjectInvalidVersion(t *testing.T) {
	data := `{"version":"9.9","width":10,"height":10,"layers":[]}`
	_, err := LoadProject(bytes.NewBufferString(data))
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
}

func TestLoadProjectInvalidDimensions(t *testing.T) {
	data := `{"version":"1.0","width":0,"height":0,"layers":[]}`
	_, err := LoadProject(bytes.NewBufferString(data))
	if err == nil {
		t.Fatal("expected error for invalid dimensions, got nil")
	}
}
