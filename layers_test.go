package vango

import (
	"context"
	"image"
	"image/color"
	"testing"
)

func testImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			img.Pix[i+0] = uint8((x * 255) / w)
			img.Pix[i+1] = uint8((y * 255) / h)
			img.Pix[i+2] = 128
			img.Pix[i+3] = 255
		}
	}
	return img
}

// ---------- Layer tests ----------

func TestNewLayer(t *testing.T) {
	img := testImage(100, 100)
	l := NewLayer("test", img)
	if l.Name != "test" {
		t.Errorf("expected name 'test', got %q", l.Name)
	}
	if l.Opacity != 1.0 {
		t.Errorf("expected opacity 1.0, got %f", l.Opacity)
	}
	if !l.Visible {
		t.Error("expected visible")
	}
	if l.Blend != BlendNormal {
		t.Errorf("expected BlendNormal, got %v", l.Blend)
	}
}

func TestCanvasAddRemoveLayer(t *testing.T) {
	c := NewCanvas(100, 100)
	c.AddLayer(NewLayer("A", testImage(100, 100)))
	c.AddLayer(NewLayer("B", testImage(100, 100)))
	if len(c.Layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(c.Layers))
	}
	c.RemoveLayer("A")
	if len(c.Layers) != 1 {
		t.Fatalf("expected 1 layer after removal, got %d", len(c.Layers))
	}
	if c.Layers[0].Name != "B" {
		t.Errorf("expected remaining layer 'B', got %q", c.Layers[0].Name)
	}
}

func TestCanvasFlatten(t *testing.T) {
	c := NewCanvas(50, 50)
	bg := NewSolidLayer("bg", 50, 50, color.NRGBA{255, 0, 0, 255})
	c.AddLayer(bg)
	overlay := NewSolidLayer("overlay", 50, 50, color.NRGBA{0, 0, 255, 128})
	overlay.ZOrder = 1
	c.AddLayer(overlay)
	out := c.FlattenAll()
	if out.Rect.Dx() != 50 || out.Rect.Dy() != 50 {
		t.Errorf("expected 50x50, got %dx%d", out.Rect.Dx(), out.Rect.Dy())
	}
	// Check that compositing happened (pixel shouldn't be pure red or pure blue)
	p := idx(out, 25, 25)
	if out.Pix[p+0] == 255 && out.Pix[p+2] == 0 {
		t.Error("expected blended pixel, got pure red")
	}
}

func TestCanvasDuplicateLayer(t *testing.T) {
	c := NewCanvas(50, 50)
	c.AddLayer(NewSolidLayer("A", 50, 50, color.NRGBA{100, 100, 100, 255}))
	dup := c.DuplicateLayer("A", "A_copy")
	if dup == nil {
		t.Fatal("expected non-nil duplicate")
	}
	if len(c.Layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(c.Layers))
	}
	if dup.Name != "A_copy" {
		t.Errorf("expected name 'A_copy', got %q", dup.Name)
	}
}

func TestCanvasMergeDown(t *testing.T) {
	c := NewCanvas(50, 50)
	a := NewSolidLayer("A", 50, 50, color.NRGBA{255, 0, 0, 255})
	a.ZOrder = 0
	c.AddLayer(a)
	b := NewSolidLayer("B", 50, 50, color.NRGBA{0, 0, 255, 128})
	b.ZOrder = 1
	c.AddLayer(b)
	c.MergeDown("B")
	if len(c.Layers) != 1 {
		t.Fatalf("expected 1 layer after merge, got %d", len(c.Layers))
	}
}

func TestBlendModes(t *testing.T) {
	modes := []BlendMode{BlendMultiply, BlendScreen, BlendOverlay, BlendSoftLight,
		BlendHardLight, BlendColorDodge, BlendColorBurn, BlendDarken, BlendLighten,
		BlendDifference, BlendExclusion}
	for _, m := range modes {
		r, g, b := blendPixel(m, 0.5, 0.3, 0.7, 0.6, 0.4, 0.2)
		if r < 0 || r > 1 || g < 0 || g > 1 || b < 0 || b > 1 {
			t.Errorf("blend mode %v produced out-of-range values: %f %f %f", m, r, g, b)
		}
	}
}

func TestBlendModeFromString(t *testing.T) {
	if BlendModeFromString("multiply") != BlendMultiply {
		t.Error("expected BlendMultiply")
	}
	if BlendModeFromString("unknown") != BlendNormal {
		t.Error("expected BlendNormal for unknown")
	}
}

func TestLayerVisibility(t *testing.T) {
	c := NewCanvas(50, 50)
	a := NewSolidLayer("visible", 50, 50, color.NRGBA{255, 0, 0, 255})
	a.ZOrder = 0
	c.AddLayer(a)
	b := NewSolidLayer("hidden", 50, 50, color.NRGBA{0, 0, 255, 255})
	b.Visible = false
	b.ZOrder = 1
	c.AddLayer(b)
	out := c.FlattenAll()
	p := idx(out, 25, 25)
	if out.Pix[p+0] != 255 || out.Pix[p+2] != 0 {
		t.Errorf("hidden layer affected output: R=%d B=%d", out.Pix[p+0], out.Pix[p+2])
	}
}

func TestLayerOffset(t *testing.T) {
	c := NewCanvas(100, 100)
	bg := NewSolidLayer("bg", 100, 100, color.NRGBA{0, 0, 0, 255})
	bg.ZOrder = 0
	c.AddLayer(bg)
	small := NewSolidLayer("small", 20, 20, color.NRGBA{255, 255, 255, 255})
	small.OffsetX = 40
	small.OffsetY = 40
	small.ZOrder = 1
	c.AddLayer(small)
	out := c.FlattenAll()
	// pixel at (0,0) should be black
	p := idx(out, 0, 0)
	if out.Pix[p+0] != 0 {
		t.Errorf("expected black at (0,0), got R=%d", out.Pix[p+0])
	}
	// pixel at (50,50) should be white
	p2 := idx(out, 50, 50)
	if out.Pix[p2+0] != 255 {
		t.Errorf("expected white at (50,50), got R=%d", out.Pix[p2+0])
	}
}

func TestLayerMask(t *testing.T) {
	c := NewCanvas(50, 50)
	bg := NewSolidLayer("bg", 50, 50, color.NRGBA{255, 0, 0, 255})
	bg.ZOrder = 0
	c.AddLayer(bg)
	overlay := NewSolidLayer("masked", 50, 50, color.NRGBA{0, 255, 0, 255})
	overlay.ZOrder = 1
	mask := image.NewGray(image.Rect(0, 0, 50, 50))
	// left half transparent, right half opaque
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			if x >= 25 {
				mask.Pix[y*50+x] = 255
			}
		}
	}
	overlay.Mask = mask
	c.AddLayer(overlay)
	out := c.FlattenAll()
	// Left side should be red
	p := idx(out, 5, 25)
	if out.Pix[p+1] != 0 {
		t.Errorf("expected no green at (5,25), got G=%d", out.Pix[p+1])
	}
	// Right side should be green
	p2 := idx(out, 45, 25)
	if out.Pix[p2+1] != 255 {
		t.Errorf("expected green at (45,25), got G=%d", out.Pix[p2+1])
	}
}

func TestLayerPerLayerEffects(t *testing.T) {
	c := NewCanvas(50, 50)
	l := NewSolidLayer("fx", 50, 50, color.NRGBA{128, 128, 128, 255})
	l.Effects = []step{{name: "invert", pf: pfInvert()}}
	c.AddLayer(l)
	out := c.Flatten(context.Background())
	p := idx(out, 25, 25)
	// inverted 128 → 127
	if out.Pix[p+0] != 127 {
		t.Errorf("expected inverted 127, got %d", out.Pix[p+0])
	}
}

// ---------- Sharpening tests ----------

func TestHighPassSharpen(t *testing.T) {
	img := testImage(100, 100)
	out := HighPassSharpen(img, 1.0, 3)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestClarity(t *testing.T) {
	img := testImage(100, 100)
	out := Clarity(img, 0.5)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestSharpenConvolution(t *testing.T) {
	img := testImage(100, 100)
	out := SharpenConvolution(img, 1.0)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

// ---------- Advanced effects tests ----------

func TestLevels(t *testing.T) {
	img := testImage(50, 50)
	out := Levels(img, 10, 240, 1.0, 0, 255)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestApplyCurve(t *testing.T) {
	img := testImage(50, 50)
	points := []CurvePoint{{0, 0}, {0.5, 0.7}, {1, 1}}
	out := ApplyCurve(img, points)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestChannelMix(t *testing.T) {
	img := testImage(50, 50)
	out := ChannelMix(img, 1, 0, 0, 0, 1, 0, 0, 0, 1) // identity
	p := idx(img, 25, 25)
	q := idx(out, 25, 25)
	for c := 0; c < 3; c++ {
		if img.Pix[p+c] != out.Pix[q+c] {
			t.Errorf("identity channel mix changed pixel at channel %d: %d → %d", c, img.Pix[p+c], out.Pix[q+c])
		}
	}
}

func TestColorBalance(t *testing.T) {
	img := testImage(50, 50)
	out := ColorBalance(img, 0, 0, 0, 0, 0, 0, 0, 0, 0) // neutral
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestMotionBlur(t *testing.T) {
	img := testImage(50, 50)
	out := MotionBlur(img, 0, 5)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestGlow(t *testing.T) {
	img := testImage(50, 50)
	out := Glow(img, 3, 0.5)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestHalftone(t *testing.T) {
	img := testImage(50, 50)
	out := Halftone(img, 4)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestOilPainting(t *testing.T) {
	img := testImage(50, 50)
	out := OilPainting(img, 2, 10)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestChromaticAberration(t *testing.T) {
	img := testImage(50, 50)
	out := ChromaticAberration(img, 3)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestAddNoise(t *testing.T) {
	img := testImage(50, 50)
	out := AddNoise(img, 0.1, false)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestTiltShift(t *testing.T) {
	img := testImage(50, 50)
	out := TiltShift(img, 0.5, 0.3, 3)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestColorTemperature(t *testing.T) {
	img := testImage(50, 50)
	warm := ColorTemperature(img, 0.5)
	cool := ColorTemperature(img, -0.5)
	// Warm should have more red, cool more blue
	p := 25*50*4 + 25*4
	if warm.Pix[p+0] <= cool.Pix[p+0] {
		t.Error("warm should have more red than cool")
	}
}

func TestBilateralFilter(t *testing.T) {
	img := testImage(50, 50)
	out := BilateralFilter(img, 2, 20)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestHSLSelective(t *testing.T) {
	img := testImage(50, 50)
	out := HSLSelective(img, 0, 30, 1.5, 0.1)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestGradientMap(t *testing.T) {
	img := testImage(50, 50)
	stops := []GradientStop{
		{0, color.NRGBA{0, 0, 128, 255}},
		{1, color.NRGBA{255, 200, 0, 255}},
	}
	out := GradientMap(img, stops)
	if out.Rect != img.Rect {
		t.Error("size changed")
	}
}

func TestPerspectiveTransform(t *testing.T) {
	img := testImage(100, 100)
	corners := [4][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}} // identity
	out := PerspectiveTransform(img, corners, 100, 100)
	if out.Rect.Dx() != 100 || out.Rect.Dy() != 100 {
		t.Error("size mismatch")
	}
}

// ---------- Pipeline chainable tests ----------

func TestPipelineNewEffects(t *testing.T) {
	img := testImage(50, 50)

	// Test that all new pipeline methods compile and execute
	out := From(img).
		SharpenConvolution(0.5).
		Clarity(0.3).
		Levels(0, 255, 1, 0, 255).
		ColorTemperature(0.2).
		Glow(3, 0.3).
		FlipX().
		FlipY().
		Image()

	if out.Rect.Dx() != 50 || out.Rect.Dy() != 50 {
		t.Errorf("expected 50x50, got %dx%d", out.Rect.Dx(), out.Rect.Dy())
	}
}

func TestEffectNamesContainsNew(t *testing.T) {
	names := EffectNames()
	want := []string{"sharpen", "clarity", "levels", "motion_blur", "glow", "halftone",
		"oil_painting", "chromatic_aberration", "bilateral", "tilt_shift", "color_temperature",
		"flip_x", "flip_y"}
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, w := range want {
		if !nameSet[w] {
			t.Errorf("EffectNames() missing %q", w)
		}
	}
}

func TestBlendModeNames(t *testing.T) {
	modes := BlendModeNames()
	if len(modes) < 10 {
		t.Errorf("expected at least 10 blend modes, got %d", len(modes))
	}
}
