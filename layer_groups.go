package vango

import (
	"context"
	"image"
	"image/color"
	"sort"
)

// --------------------------------------------------------------------------
// Layer Groups
// --------------------------------------------------------------------------

// LayerGroup returns all layers in the specified group, sorted by ZOrder.
func (c *Canvas) LayerGroup(groupName string) []*Layer {
	c.mu.Lock()
	defer c.mu.Unlock()
	var result []*Layer
	for _, l := range c.Layers {
		if l.Group == groupName {
			result = append(result, l)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ZOrder < result[j].ZOrder
	})
	return result
}

// SetLayerGroup assigns a layer to a group.
func (c *Canvas) SetLayerGroup(layerName, groupName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.Layers {
		if l.Name == layerName {
			l.Group = groupName
			return true
		}
	}
	return false
}

// FlattenGroup merges all layers in a group into one layer.
func (c *Canvas) FlattenGroup(groupName string) *Layer {
	c.mu.Lock()
	defer c.mu.Unlock()

	var group []*Layer
	var rest []*Layer
	for _, l := range c.Layers {
		if l.Group == groupName {
			group = append(group, l)
		} else {
			rest = append(rest, l)
		}
	}
	if len(group) == 0 {
		return nil
	}

	sort.Slice(group, func(i, j int) bool {
		return group[i].ZOrder < group[j].ZOrder
	})

	result := image.NewNRGBA(image.Rect(0, 0, c.Width, c.Height))
	for _, l := range group {
		if !l.Visible {
			continue
		}
		layerImg := l.ApplyEffects(context.Background())
		compositeLayerOnto(context.Background(), result, layerImg, l)
	}

	merged := &Layer{
		Name:    groupName,
		Image:   result,
		Blend:   BlendNormal,
		Opacity: 1,
		Visible: true,
		ZOrder:  group[0].ZOrder,
	}

	rest = append(rest, merged)
	c.Layers = rest
	return merged
}

// Groups returns a list of unique group names in the canvas.
func (c *Canvas) Groups() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := make(map[string]bool)
	var groups []string
	for _, l := range c.Layers {
		if l.Group != "" && !seen[l.Group] {
			seen[l.Group] = true
			groups = append(groups, l.Group)
		}
	}
	return groups
}

// --------------------------------------------------------------------------
// Adjustment Layers (non-destructive)
// --------------------------------------------------------------------------

// AdjustmentType identifies the type of adjustment.
type AdjustmentType int

const (
	AdjBrightnessContrast AdjustmentType = iota
	AdjHueSaturation
	AdjLevels
	AdjCurves
	AdjColorBalance
	AdjVibrance
	AdjThreshold
	AdjPosterize
	AdjInvert
	AdjGradientMap
	AdjSolidColor
)

// AdjustmentParams holds parameters for adjustment layers.
type AdjustmentParams struct {
	Type AdjustmentType

	// Brightness/Contrast
	Brightness float64
	Contrast   float64

	// Hue/Saturation
	Hue        float64
	Saturation float64

	// Levels
	InBlack  float64
	InWhite  float64
	OutBlack float64
	OutWhite float64
	MidGamma float64

	// Color Balance
	ShadowRGB [3]float64
	MidRGB    [3]float64
	HighRGB   [3]float64

	// Vibrance
	VibranceAmount float64

	// Threshold / Posterize
	ThresholdVal float64
	PosterizeN   int

	// Curves
	CurveR []CurvePoint
	CurveG []CurvePoint
	CurveB []CurvePoint

	// Gradient Map
	GradientStops []GradientStop

	// Solid Color
	FillColor color.NRGBA
}

// NewAdjustmentLayer creates a non-destructive adjustment layer.
// It applies the adjustment to all visible layers below it during flattening.
func NewAdjustmentLayer(name string, w, h int, params AdjustmentParams) *Layer {
	// Adjustment layer uses a transparent image
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	l := &Layer{
		Name:    name,
		Image:   img,
		Blend:   BlendNormal,
		Opacity: 1,
		Visible: true,
	}
	// Store adjustment as an effect step
	l.Effects = []step{{
		apply: func(ctx context.Context, input *image.NRGBA) *image.NRGBA {
			return applyAdjustment(ctx, input, params)
		},
	}}
	return l
}

// applyAdjustment applies the adjustment to an image.
func applyAdjustment(ctx context.Context, img *image.NRGBA, p AdjustmentParams) *image.NRGBA {
	switch p.Type {
	case AdjBrightnessContrast:
		out := AdjustBrightness(img, p.Brightness)
		return AdjustContrast(out, p.Contrast)
	case AdjHueSaturation:
		out := AdjustHue(img, p.Hue)
		return AdjustSaturation(out, p.Saturation)
	case AdjLevels:
		return Levels(img, p.InBlack, p.InWhite, p.MidGamma, p.OutBlack, p.OutWhite)
	case AdjCurves:
		return ChannelCurvesCtx(ctx, img, p.CurveR, p.CurveG, p.CurveB)
	case AdjColorBalance:
		return ColorBalance(img, p.ShadowRGB[0], p.ShadowRGB[1], p.ShadowRGB[2], p.MidRGB[0], p.MidRGB[1], p.MidRGB[2], p.HighRGB[0], p.HighRGB[1], p.HighRGB[2])
	case AdjVibrance:
		return VibranceCtx(ctx, img, p.VibranceAmount)
	case AdjThreshold:
		return ToNRGBA(Threshold(img, uint8(p.ThresholdVal)))
	case AdjPosterize:
		return Posterize(img, p.PosterizeN)
	case AdjInvert:
		return Invert(img)
	case AdjGradientMap:
		return GradientMap(img, p.GradientStops)
	case AdjSolidColor:
		out := CloneNRGBA(img)
		b := out.Rect
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				i := idx(out, x, y)
				a := float64(out.Pix[i+3]) / 255.0
				out.Pix[i+0] = clamp8(int(lerp(float64(out.Pix[i+0]), float64(p.FillColor.R), a)))
				out.Pix[i+1] = clamp8(int(lerp(float64(out.Pix[i+1]), float64(p.FillColor.G), a)))
				out.Pix[i+2] = clamp8(int(lerp(float64(out.Pix[i+2]), float64(p.FillColor.B), a)))
			}
		}
		return out
	}
	return img
}
