package vango

import (
	"context"
	"image"
	"image/color"
	"math"
	"sort"
	"sync"
)

// BlendMode determines how a layer composites onto those below.
type BlendMode int

const (
	BlendNormal BlendMode = iota
	BlendMultiply
	BlendScreen
	BlendOverlay
	BlendSoftLight
	BlendHardLight
	BlendColorDodge
	BlendColorBurn
	BlendDarken
	BlendLighten
	BlendDifference
	BlendExclusion
	BlendHue
	BlendSaturation
	BlendColor
	BlendLuminosity
	BlendDissolve
	BlendLinearBurn
	BlendLinearDodge
	BlendVividLight
	BlendLinearLight
	BlendPinLight
)

// BlendModeFromString parses a blend mode name (case-insensitive fallback: Normal).
func BlendModeFromString(s string) BlendMode {
	switch s {
	case "normal":
		return BlendNormal
	case "multiply":
		return BlendMultiply
	case "screen":
		return BlendScreen
	case "overlay":
		return BlendOverlay
	case "soft_light", "softlight":
		return BlendSoftLight
	case "hard_light", "hardlight":
		return BlendHardLight
	case "color_dodge", "colordodge":
		return BlendColorDodge
	case "color_burn", "colorburn":
		return BlendColorBurn
	case "darken":
		return BlendDarken
	case "lighten":
		return BlendLighten
	case "difference":
		return BlendDifference
	case "exclusion":
		return BlendExclusion
	case "hue":
		return BlendHue
	case "saturation":
		return BlendSaturation
	case "color":
		return BlendColor
	case "luminosity":
		return BlendLuminosity
	case "dissolve":
		return BlendDissolve
	case "linear_burn", "linearburn":
		return BlendLinearBurn
	case "linear_dodge", "lineardodge":
		return BlendLinearDodge
	case "vivid_light", "vividlight":
		return BlendVividLight
	case "linear_light", "linearlight":
		return BlendLinearLight
	case "pin_light", "pinlight":
		return BlendPinLight
	default:
		return BlendNormal
	}
}

func (m BlendMode) String() string {
	switch m {
	case BlendNormal:
		return "normal"
	case BlendMultiply:
		return "multiply"
	case BlendScreen:
		return "screen"
	case BlendOverlay:
		return "overlay"
	case BlendSoftLight:
		return "soft_light"
	case BlendHardLight:
		return "hard_light"
	case BlendColorDodge:
		return "color_dodge"
	case BlendColorBurn:
		return "color_burn"
	case BlendDarken:
		return "darken"
	case BlendLighten:
		return "lighten"
	case BlendDifference:
		return "difference"
	case BlendExclusion:
		return "exclusion"
	case BlendHue:
		return "hue"
	case BlendSaturation:
		return "saturation"
	case BlendColor:
		return "color"
	case BlendLuminosity:
		return "luminosity"
	case BlendDissolve:
		return "dissolve"
	case BlendLinearBurn:
		return "linear_burn"
	case BlendLinearDodge:
		return "linear_dodge"
	case BlendVividLight:
		return "vivid_light"
	case BlendLinearLight:
		return "linear_light"
	case BlendPinLight:
		return "pin_light"
	default:
		return "normal"
	}
}

// Layer represents a single compositing layer.
type Layer struct {
	Name      string
	Image     *image.NRGBA
	Blend     BlendMode
	Opacity   float64 // 0..1
	Visible   bool
	OffsetX   int
	OffsetY   int
	Mask      *image.Gray // optional alpha mask (same size as Image or nil)
	Effects   []step      // per-layer effect pipeline
	ZOrder    int         // lower = further back
	Locked    bool
	ClipGroup bool        // if true, clips to the alpha of the layer below
	Style     *LayerStyle // optional Photoshop-like layer styles
	Group     string      // layer group name (empty = ungrouped)
}

// NewLayer creates a visible layer at full opacity with Normal blending.
func NewLayer(name string, img image.Image) *Layer {
	return &Layer{
		Name:    name,
		Image:   ToNRGBA(img),
		Blend:   BlendNormal,
		Opacity: 1,
		Visible: true,
	}
}

// NewSolidLayer creates a layer filled with a solid color.
func NewSolidLayer(name string, w, h int, c color.NRGBA) *Layer {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+0] = c.R
		img.Pix[i+1] = c.G
		img.Pix[i+2] = c.B
		img.Pix[i+3] = c.A
	}
	return &Layer{
		Name:    name,
		Image:   img,
		Blend:   BlendNormal,
		Opacity: 1,
		Visible: true,
	}
}

// NewEmptyLayer creates a transparent layer.
func NewEmptyLayer(name string, w, h int) *Layer {
	return &Layer{
		Name:    name,
		Image:   image.NewNRGBA(image.Rect(0, 0, w, h)),
		Blend:   BlendNormal,
		Opacity: 1,
		Visible: true,
	}
}

// ApplyEffects runs the layer's per-layer effect pipeline on its image,
// then applies any layer styles, and returns the processed result (does not mutate the original).
func (l *Layer) ApplyEffects(ctx context.Context) *image.NRGBA {
	img := l.Image
	if len(l.Effects) > 0 {
		p := &Pipeline{img: CloneNRGBA(l.Image), steps: l.Effects}
		out, err := p.execute(ctx)
		if err == nil {
			img = out
		}
	}
	if l.Style != nil {
		img = ApplyLayerStyle(img, *l.Style)
	}
	return img
}

// Canvas holds an ordered stack of layers and composites them.
type Canvas struct {
	Width  int
	Height int
	Layers []*Layer
	mu     sync.RWMutex
}

// NewCanvas creates an empty canvas of the given dimensions.
func NewCanvas(w, h int) *Canvas {
	return &Canvas{Width: w, Height: h}
}

// NewCanvasFrom creates a canvas sized to the given image, with it as the background layer.
func NewCanvasFrom(img image.Image) *Canvas {
	b := img.Bounds()
	c := &Canvas{Width: b.Dx(), Height: b.Dy()}
	c.Layers = append(c.Layers, NewLayer("Background", img))
	return c
}

// AddLayer appends a layer. ZOrder is set to current count if zero.
func (c *Canvas) AddLayer(l *Layer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if l.ZOrder == 0 {
		l.ZOrder = len(c.Layers)
	}
	c.Layers = append(c.Layers, l)
}

// RemoveLayer removes a layer by name.
func (c *Canvas) RemoveLayer(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, l := range c.Layers {
		if l.Name == name {
			c.Layers = append(c.Layers[:i], c.Layers[i+1:]...)
			return
		}
	}
}

// FindLayer returns the layer with the given name, or nil.
func (c *Canvas) FindLayer(name string) *Layer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, l := range c.Layers {
		if l.Name == name {
			return l
		}
	}
	return nil
}

// MoveLayer changes a layer's ZOrder.
func (c *Canvas) MoveLayer(name string, newZ int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.Layers {
		if l.Name == name {
			l.ZOrder = newZ
			return
		}
	}
}

// DuplicateLayer clones a layer with a new name.
func (c *Canvas) DuplicateLayer(name, newName string) *Layer {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.Layers {
		if l.Name == name {
			dup := &Layer{
				Name:    newName,
				Image:   CloneNRGBA(l.Image),
				Blend:   l.Blend,
				Opacity: l.Opacity,
				Visible: l.Visible,
				OffsetX: l.OffsetX,
				OffsetY: l.OffsetY,
				ZOrder:  l.ZOrder + 1,
				Locked:  false,
			}
			if l.Mask != nil {
				dup.Mask = image.NewGray(l.Mask.Rect)
				copy(dup.Mask.Pix, l.Mask.Pix)
			}
			dup.Effects = make([]step, len(l.Effects))
			copy(dup.Effects, l.Effects)
			c.Layers = append(c.Layers, dup)
			return dup
		}
	}
	return nil
}

// MergeDown merges a layer into the one directly below it (by ZOrder).
func (c *Canvas) MergeDown(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sorted := make([]*Layer, len(c.Layers))
	copy(sorted, c.Layers)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ZOrder < sorted[j].ZOrder })

	for i, l := range sorted {
		if l.Name == name && i > 0 {
			below := sorted[i-1]
			merged := compositeTwo(below, l, c.Width, c.Height)
			below.Image = merged
			// remove the top layer
			for j, ll := range c.Layers {
				if ll.Name == name {
					c.Layers = append(c.Layers[:j], c.Layers[j+1:]...)
					break
				}
			}
			return
		}
	}
}

// FlattenAll merges all visible layers into one, returns the result.
func (c *Canvas) FlattenAll() *image.NRGBA {
	return c.Flatten(context.Background())
}

// Flatten composites all visible layers in ZOrder onto a transparent canvas.
func (c *Canvas) Flatten(ctx context.Context) *image.NRGBA {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := image.NewNRGBA(image.Rect(0, 0, c.Width, c.Height))

	sorted := make([]*Layer, 0, len(c.Layers))
	for _, l := range c.Layers {
		if l.Visible {
			sorted = append(sorted, l)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ZOrder < sorted[j].ZOrder })

	for _, l := range sorted {
		processed := l.ApplyEffects(ctx)
		compositeLayerOnto(ctx, out, processed, l)
	}
	return out
}

// compositeTwo merges layer top onto base for MergeDown.
func compositeTwo(base, top *Layer, w, h int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	ctx := context.Background()
	compositeLayerOnto(ctx, out, base.Image, base)
	processed := top.ApplyEffects(ctx)
	compositeLayerOnto(ctx, out, processed, top)
	return out
}

// compositeLayerOnto blends a processed layer image onto dst.
func compositeLayerOnto(ctx context.Context, dst *image.NRGBA, src *image.NRGBA, l *Layer) {
	r := dst.Rect
	_ = parallelRows(ctx, 0, r.Dy(), func(yy int) {
		y := r.Min.Y + yy
		for x := r.Min.X; x < r.Max.X; x++ {
			// source coordinates
			sx := x - l.OffsetX
			sy := y - l.OffsetY
			if sx < src.Rect.Min.X || sx >= src.Rect.Max.X || sy < src.Rect.Min.Y || sy >= src.Rect.Max.Y {
				continue
			}

			si := idx(src, sx, sy)
			di := idx(dst, x, y)

			sa := float64(src.Pix[si+3]) / 255.0 * l.Opacity

			// Apply mask
			if l.Mask != nil {
				mx := sx - l.Mask.Rect.Min.X
				my := sy - l.Mask.Rect.Min.Y
				if mx >= 0 && mx < l.Mask.Rect.Dx() && my >= 0 && my < l.Mask.Rect.Dy() {
					sa *= float64(l.Mask.Pix[my*l.Mask.Stride+mx]) / 255.0
				} else {
					sa = 0
				}
			}

			if sa <= 0 {
				continue
			}

			sr := float64(src.Pix[si+0]) / 255.0
			sg := float64(src.Pix[si+1]) / 255.0
			sb := float64(src.Pix[si+2]) / 255.0
			dr := float64(dst.Pix[di+0]) / 255.0
			dg := float64(dst.Pix[di+1]) / 255.0
			db := float64(dst.Pix[di+2]) / 255.0
			da := float64(dst.Pix[di+3]) / 255.0

			// Apply blend mode
			br, bg, bb := blendPixel(l.Blend, sr, sg, sb, dr, dg, db)

			// Porter-Duff over with blended color
			outA := sa + da*(1-sa)
			if outA > 0 {
				outR := (br*sa + dr*da*(1-sa)) / outA
				outG := (bg*sa + dg*da*(1-sa)) / outA
				outB := (bb*sa + db*da*(1-sa)) / outA
				dst.Pix[di+0] = clamp8(int(outR*255 + 0.5))
				dst.Pix[di+1] = clamp8(int(outG*255 + 0.5))
				dst.Pix[di+2] = clamp8(int(outB*255 + 0.5))
				dst.Pix[di+3] = clamp8(int(outA*255 + 0.5))
			}
		}
	})
}

// blendPixel applies a blend mode to (src, dst) color channels (0..1).
func blendPixel(mode BlendMode, sr, sg, sb, dr, dg, db float64) (float64, float64, float64) {
	switch mode {
	case BlendMultiply:
		return sr * dr, sg * dg, sb * db
	case BlendScreen:
		return 1 - (1-sr)*(1-dr), 1 - (1-sg)*(1-dg), 1 - (1-sb)*(1-db)
	case BlendOverlay:
		return blendOverlayCh(sr, dr), blendOverlayCh(sg, dg), blendOverlayCh(sb, db)
	case BlendSoftLight:
		return blendSoftLightCh(sr, dr), blendSoftLightCh(sg, dg), blendSoftLightCh(sb, db)
	case BlendHardLight:
		return blendOverlayCh(dr, sr), blendOverlayCh(dg, sg), blendOverlayCh(db, sb)
	case BlendColorDodge:
		return blendDodgeCh(sr, dr), blendDodgeCh(sg, dg), blendDodgeCh(sb, db)
	case BlendColorBurn:
		return blendBurnCh(sr, dr), blendBurnCh(sg, dg), blendBurnCh(sb, db)
	case BlendDarken:
		return math.Min(sr, dr), math.Min(sg, dg), math.Min(sb, db)
	case BlendLighten:
		return math.Max(sr, dr), math.Max(sg, dg), math.Max(sb, db)
	case BlendDifference:
		return math.Abs(sr - dr), math.Abs(sg - dg), math.Abs(sb - db)
	case BlendExclusion:
		return sr + dr - 2*sr*dr, sg + dg - 2*sg*dg, sb + db - 2*sb*db
	case BlendLinearBurn:
		return clampF01(sr + dr - 1), clampF01(sg + dg - 1), clampF01(sb + db - 1)
	case BlendLinearDodge:
		return clampF01(sr + dr), clampF01(sg + dg), clampF01(sb + db)
	case BlendVividLight:
		return blendVividCh(sr, dr), blendVividCh(sg, dg), blendVividCh(sb, db)
	case BlendLinearLight:
		return clampF01(dr + 2*sr - 1), clampF01(dg + 2*sg - 1), clampF01(db + 2*sb - 1)
	case BlendPinLight:
		return blendPinCh(sr, dr), blendPinCh(sg, dg), blendPinCh(sb, db)
	case BlendHue:
		return blendHSLMode(sr, sg, sb, dr, dg, db, true, false, false)
	case BlendSaturation:
		return blendHSLMode(sr, sg, sb, dr, dg, db, false, true, false)
	case BlendColor:
		return blendHSLMode(sr, sg, sb, dr, dg, db, true, true, false)
	case BlendLuminosity:
		return blendHSLMode(sr, sg, sb, dr, dg, db, false, false, true)
	default: // Normal
		return sr, sg, sb
	}
}

func blendOverlayCh(a, b float64) float64 {
	if b < 0.5 {
		return 2 * a * b
	}
	return 1 - 2*(1-a)*(1-b)
}

func blendSoftLightCh(a, b float64) float64 {
	if a <= 0.5 {
		return b - (1-2*a)*b*(1-b)
	}
	var d float64
	if b <= 0.25 {
		d = ((16*b-12)*b + 4) * b
	} else {
		d = math.Sqrt(b)
	}
	return b + (2*a-1)*(d-b)
}

func blendDodgeCh(s, d float64) float64 {
	if s >= 1 {
		return 1
	}
	v := d / (1 - s)
	if v > 1 {
		return 1
	}
	return v
}

func blendBurnCh(s, d float64) float64 {
	if s <= 0 {
		return 0
	}
	v := 1 - (1-d)/s
	if v < 0 {
		return 0
	}
	return v
}

func blendVividCh(s, d float64) float64 {
	if s <= 0.5 {
		return blendBurnCh(2*s, d)
	}
	return blendDodgeCh(2*s-1, d)
}

func blendPinCh(s, d float64) float64 {
	if s < 0.5 {
		return math.Min(d, 2*s)
	}
	return math.Max(d, 2*s-1)
}

// blendHSLMode applies hue/saturation/luminosity blend modes.
func blendHSLMode(sr, sg, sb, dr, dg, db float64, useHue, useSat, useLum bool) (float64, float64, float64) {
	sh, ss, sl := rgbToHSL(sr, sg, sb)
	dh, ds, dl := rgbToHSL(dr, dg, db)
	h, s, l := dh, ds, dl
	if useHue {
		h = sh
	}
	if useSat {
		s = ss
	}
	if useLum {
		l = sl
	}
	return hslToRGB(h, s, l)
}
