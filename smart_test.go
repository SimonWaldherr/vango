package vango

import (
	"image"
	"image/color"
	"testing"
)

func TestSmartCropRectTracksSalientRegion(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.NRGBA{R: 30, G: 30, B: 30, A: 255})
		}
	}
	for y := 8; y < 32; y++ {
		for x := 52; x < 76; x++ {
			img.Set(x, y, color.NRGBA{R: 230, G: 70, B: 40, A: 255})
		}
	}

	rect := SmartCropRect(img, 40, 40)
	if rect.Dx() != 40 || rect.Dy() != 40 {
		t.Fatalf("unexpected crop size: %v", rect)
	}
	if rect.Min.X <= 20 {
		t.Fatalf("expected salient right-side object to influence crop, got %v", rect)
	}
}

func TestAutoExposureExpandsLowContrastImage(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 1))
	for x := 0; x < 32; x++ {
		v := uint8(90 + x%8)
		img.Set(x, 0, color.NRGBA{R: v, G: v, B: v, A: 255})
	}

	out := AutoExposure(img)
	minV, maxV := uint8(255), uint8(0)
	for i := 0; i < len(out.Pix); i += 4 {
		if out.Pix[i] < minV {
			minV = out.Pix[i]
		}
		if out.Pix[i] > maxV {
			maxV = out.Pix[i]
		}
	}
	if maxV-minV < 100 {
		t.Fatalf("expected auto exposure to expand tonal range, got %d..%d", minV, maxV)
	}
}

func TestAnalyzeImageAndAutoScene(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(35 + x), G: uint8(45 + y), B: 70, A: 255})
		}
	}

	stats := AnalyzeImage(img)
	if stats.Pixels != 16*16 {
		t.Fatalf("unexpected stats pixel count: %d", stats.Pixels)
	}
	if stats.AvgLuma <= 0 || stats.DynamicRange <= 0 {
		t.Fatalf("expected useful image stats, got %+v", stats)
	}
	out := AutoScene(img)
	if got := out.Bounds().Size(); got.X != 16 || got.Y != 16 {
		t.Fatalf("auto scene should preserve size, got %v", got)
	}
}

func TestModernFilterChangesPixelsAndPreservesAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.NRGBA{R: 80, G: 120, B: 160, A: 200})
	img.Set(1, 0, color.NRGBA{R: 220, G: 180, B: 120, A: 128})

	out := ModernFilter(img, "cinematic", 1)
	if out.Pix[3] != 200 || out.Pix[7] != 128 {
		t.Fatalf("modern filter should preserve alpha, got %d %d", out.Pix[3], out.Pix[7])
	}
	if out.Pix[0] == img.Pix[0] && out.Pix[1] == img.Pix[1] && out.Pix[2] == img.Pix[2] {
		t.Fatalf("modern filter did not alter first pixel")
	}
}

func TestModernFilterNamesIncludesNewLooks(t *testing.T) {
	names := ModernFilterNames()
	want := map[string]bool{
		"golden_hour": false,
		"moody":       false,
		"clean":       false,
		"portrait":    false,
		"cyberpunk":   false,
		"dreamscape":  false,
		"sunset":      false,
		"forest":      false,
		"infrared":    false,
	}
	for _, name := range names {
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, ok := range want {
		if !ok {
			t.Fatalf("ModernFilterNames missing %q in %v", name, names)
		}
	}
}

func TestNewCreativeModernFiltersChangePixels(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 90, G: 140, B: 180, A: 177})

	for _, name := range []string{"dreamscape", "sunset", "forest", "infrared"} {
		t.Run(name, func(t *testing.T) {
			out := ModernFilter(img, name, 1)
			if out.Pix[3] != 177 {
				t.Fatalf("%s should preserve alpha, got %d", name, out.Pix[3])
			}
			if out.Pix[0] == img.Pix[0] && out.Pix[1] == img.Pix[1] && out.Pix[2] == img.Pix[2] {
				t.Fatalf("%s did not alter pixel", name)
			}
		})
	}
}
