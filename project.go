package vango

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
)

// projectVersion is the current project file format version.
const projectVersion = "1.0"

// ProjectFile is the JSON representation of a saved vango project.
type ProjectFile struct {
	Version string        `json:"version"`
	Width   int           `json:"width"`
	Height  int           `json:"height"`
	Layers  []layerRecord `json:"layers"`
}

// layerRecord is the serialised form of a single Layer.
type layerRecord struct {
	Name      string  `json:"name"`
	Blend     string  `json:"blend"`
	Opacity   float64 `json:"opacity"`
	Visible   bool    `json:"visible"`
	OffsetX   int     `json:"offsetX"`
	OffsetY   int     `json:"offsetY"`
	ZOrder    int     `json:"zOrder"`
	Locked    bool    `json:"locked"`
	ClipGroup bool    `json:"clipGroup"`
	Group     string  `json:"group,omitempty"`
	// ImageData is the layer image encoded as a base64 PNG.
	ImageData string `json:"imageData"`
	// MaskData is the layer mask encoded as a base64 PNG (empty if no mask).
	MaskData string `json:"maskData,omitempty"`
}

// SaveProject serialises canvas c to w as a JSON project file.
// Each layer's pixel data is stored as a base64-encoded PNG.
// Layer effect closures and style structs are not serialised; only the
// rasterised (post-ApplyEffects) pixel data is saved.
func SaveProject(c *Canvas, w io.Writer) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	pf := ProjectFile{
		Version: projectVersion,
		Width:   c.Width,
		Height:  c.Height,
	}

	for _, l := range c.Layers {
		rec := layerRecord{
			Name:      l.Name,
			Blend:     l.Blend.String(),
			Opacity:   l.Opacity,
			Visible:   l.Visible,
			OffsetX:   l.OffsetX,
			OffsetY:   l.OffsetY,
			ZOrder:    l.ZOrder,
			Locked:    l.Locked,
			ClipGroup: l.ClipGroup,
			Group:     l.Group,
		}

		imgData, err := encodePNGBase64(l.Image)
		if err != nil {
			return err
		}
		rec.ImageData = imgData

		if l.Mask != nil {
			maskData, err := encodePNGBase64(l.Mask)
			if err != nil {
				return err
			}
			rec.MaskData = maskData
		}

		pf.Layers = append(pf.Layers, rec)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(pf)
}

// LoadProject deserialises a JSON project file from r into a new Canvas.
func LoadProject(r io.Reader) (*Canvas, error) {
	var pf ProjectFile
	if err := json.NewDecoder(r).Decode(&pf); err != nil {
		return nil, err
	}
	if pf.Version != projectVersion {
		return nil, errors.New("vango: unsupported project version " + pf.Version)
	}
	if pf.Width <= 0 || pf.Height <= 0 {
		return nil, errors.New("vango: invalid canvas dimensions in project file")
	}

	c := NewCanvas(pf.Width, pf.Height)

	for _, rec := range pf.Layers {
		img, err := decodePNGBase64NRGBA(rec.ImageData)
		if err != nil {
			return nil, err
		}

		l := &Layer{
			Name:      rec.Name,
			Image:     img,
			Blend:     BlendModeFromString(rec.Blend),
			Opacity:   rec.Opacity,
			Visible:   rec.Visible,
			OffsetX:   rec.OffsetX,
			OffsetY:   rec.OffsetY,
			ZOrder:    rec.ZOrder,
			Locked:    rec.Locked,
			ClipGroup: rec.ClipGroup,
			Group:     rec.Group,
		}

		if rec.MaskData != "" {
			mask, err := decodePNGBase64Gray(rec.MaskData)
			if err != nil {
				return nil, err
			}
			l.Mask = mask
		}

		c.Layers = append(c.Layers, l)
	}

	return c, nil
}

// encodePNGBase64 encodes an image as a base64-encoded PNG string.
func encodePNGBase64(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// decodePNGBase64NRGBA decodes a base64 PNG string to *image.NRGBA.
func decodePNGBase64NRGBA(data string) (*image.NRGBA, error) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return ToNRGBA(img), nil
}

// decodePNGBase64Gray decodes a base64 PNG string to *image.Gray.
func decodePNGBase64Gray(data string) (*image.Gray, error) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	// Fast path: already Gray
	if g, ok := img.(*image.Gray); ok {
		return g, nil
	}
	b := img.Bounds()
	g := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			// RGBA returns pre-multiplied 16-bit values; take high byte of red channel.
			g.Pix[(y-b.Min.Y)*g.Stride+(x-b.Min.X)] = uint8(r >> 8)
		}
	}
	return g, nil
}
