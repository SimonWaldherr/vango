package vango

import (
	"image"
	"image/color"
	"math"
)

// Selection represents a pixel-level selection mask (0=unselected, 255=fully selected).
// It wraps an *image.Gray for interoperability with layer masks.
type Selection struct {
	Mask   *image.Gray
	Bounds image.Rectangle
}

// NewSelection creates an empty (all-zero) selection for the given bounds.
func NewSelection(r image.Rectangle) *Selection {
	return &Selection{
		Mask:   image.NewGray(r),
		Bounds: r,
	}
}

// SelectAll sets the entire selection to fully selected.
func (s *Selection) SelectAll() {
	for i := range s.Mask.Pix {
		s.Mask.Pix[i] = 255
	}
}

// SelectNone clears the selection.
func (s *Selection) SelectNone() {
	for i := range s.Mask.Pix {
		s.Mask.Pix[i] = 0
	}
}

// Invert flips selected/unselected.
func (s *Selection) Invert() {
	for i := range s.Mask.Pix {
		s.Mask.Pix[i] = 255 - s.Mask.Pix[i]
	}
}

// SelectRect selects a rectangular region.
func (s *Selection) SelectRect(r image.Rectangle) {
	r = r.Intersect(s.Bounds)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			s.Mask.SetGray(x, y, color.Gray{Y: 255})
		}
	}
}

// SelectEllipse selects an elliptical region with anti-aliased edges.
func (s *Selection) SelectEllipse(cx, cy, rx, ry float64) {
	minX := int(math.Floor(cx - rx - 1))
	maxX := int(math.Ceil(cx + rx + 1))
	minY := int(math.Floor(cy - ry - 1))
	maxY := int(math.Ceil(cy + ry + 1))
	for y := minY; y <= maxY; y++ {
		if y < s.Bounds.Min.Y || y >= s.Bounds.Max.Y {
			continue
		}
		for x := minX; x <= maxX; x++ {
			if x < s.Bounds.Min.X || x >= s.Bounds.Max.X {
				continue
			}
			dx := (float64(x) + 0.5 - cx) / rx
			dy := (float64(y) + 0.5 - cy) / ry
			d := dx*dx + dy*dy
			if d <= 1.0 {
				s.Mask.SetGray(x, y, color.Gray{Y: 255})
			} else if d < 1.02 { // anti-alias fringe
				aa := uint8(clampF01(1.0-(d-1.0)/0.02) * 255)
				s.Mask.SetGray(x, y, color.Gray{Y: aa})
			}
		}
	}
}

// SelectByColor (magic wand) selects contiguous pixels similar to the seed color.
// tolerance is 0..255, contiguous determines flood-fill vs global select.
func (s *Selection) SelectByColor(src *image.NRGBA, seedX, seedY int, tolerance uint8, contiguous bool) {
	if seedX < src.Rect.Min.X || seedX >= src.Rect.Max.X || seedY < src.Rect.Min.Y || seedY >= src.Rect.Max.Y {
		return
	}
	si := idx(src, seedX, seedY)
	seedR, seedG, seedB := src.Pix[si+0], src.Pix[si+1], src.Pix[si+2]
	tol := int(tolerance)

	colorMatch := func(x, y int) bool {
		i := idx(src, x, y)
		dr := absInt(int(src.Pix[i+0]) - int(seedR))
		dg := absInt(int(src.Pix[i+1]) - int(seedG))
		db := absInt(int(src.Pix[i+2]) - int(seedB))
		return dr <= tol && dg <= tol && db <= tol
	}

	if !contiguous {
		// Global: select all matching pixels
		for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
			for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
				if colorMatch(x, y) {
					s.Mask.SetGray(x, y, color.Gray{Y: 255})
				}
			}
		}
		return
	}

	// Flood fill from seed
	visited := make([]bool, src.Rect.Dx()*src.Rect.Dy())
	w := src.Rect.Dx()
	queue := []image.Point{{seedX, seedY}}
	visited[(seedY-src.Rect.Min.Y)*w+(seedX-src.Rect.Min.X)] = true

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if colorMatch(p.X, p.Y) {
			s.Mask.SetGray(p.X, p.Y, color.Gray{Y: 255})
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := p.X+d[0], p.Y+d[1]
				if nx >= src.Rect.Min.X && nx < src.Rect.Max.X && ny >= src.Rect.Min.Y && ny < src.Rect.Max.Y {
					vi := (ny-src.Rect.Min.Y)*w + (nx - src.Rect.Min.X)
					if !visited[vi] {
						visited[vi] = true
						queue = append(queue, image.Point{nx, ny})
					}
				}
			}
		}
	}
}

// SelectColorRange selects pixels within a color range across the whole image.
// fuzziness is 0..1 controlling how much the selection feathers at boundaries.
func (s *Selection) SelectColorRange(src *image.NRGBA, targetR, targetG, targetB uint8, fuzziness float64) {
	maxDist := fuzziness * 441.67 // sqrt(255^2 * 3)
	if maxDist < 1 {
		maxDist = 1
	}
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			i := idx(src, x, y)
			dr := float64(src.Pix[i+0]) - float64(targetR)
			dg := float64(src.Pix[i+1]) - float64(targetG)
			db := float64(src.Pix[i+2]) - float64(targetB)
			dist := math.Sqrt(dr*dr + dg*dg + db*db)
			v := clampF01(1 - dist/maxDist)
			s.Mask.SetGray(x, y, color.Gray{Y: uint8(v * 255)})
		}
	}
}

// SelectByLuminosity creates a luminosity mask.
// mode: "highlights" (bright), "midtones", "shadows" (dark).
func (s *Selection) SelectByLuminosity(src *image.NRGBA, mode string) {
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			i := idx(src, x, y)
			lum := 0.2126*float64(src.Pix[i+0]) + 0.7152*float64(src.Pix[i+1]) + 0.0722*float64(src.Pix[i+2])
			l := lum / 255.0
			var v float64
			switch mode {
			case "highlights":
				v = clampF01(l*2 - 1) // 0 below 50%, ramps up
			case "shadows":
				v = clampF01(1 - l*2) // 1 at black, drops off
			case "midtones":
				v = 1 - math.Abs(l-0.5)*4
				if v < 0 {
					v = 0
				}
			default:
				v = l
			}
			s.Mask.SetGray(x, y, color.Gray{Y: uint8(v * 255)})
		}
	}
}

// Feather applies a Gaussian blur to the selection mask edges.
func (s *Selection) Feather(radius float64) {
	if radius <= 0 {
		return
	}
	// Treat the gray mask as a single-channel image, blur it
	r := s.Mask.Rect
	w, h := r.Dx(), r.Dy()
	k := gaussianKernel(radius, 0)
	halfK := (len(k) - 1) / 2

	// Horizontal pass
	tmp := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum float64
			for i := -halfK; i <= halfK; i++ {
				xx := x + i
				if xx < 0 {
					xx = 0
				} else if xx >= w {
					xx = w - 1
				}
				sum += float64(s.Mask.Pix[y*s.Mask.Stride+xx]) * k[i+halfK]
			}
			tmp[y*w+x] = clamp8(int(sum + 0.5))
		}
	}
	// Vertical pass
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum float64
			for i := -halfK; i <= halfK; i++ {
				yy := y + i
				if yy < 0 {
					yy = 0
				} else if yy >= h {
					yy = h - 1
				}
				sum += float64(tmp[yy*w+x]) * k[i+halfK]
			}
			s.Mask.Pix[y*s.Mask.Stride+x] = clamp8(int(sum + 0.5))
		}
	}
}

// Grow expands the selection by the given number of pixels.
func (s *Selection) Grow(pixels int) {
	if pixels <= 0 {
		return
	}
	r := s.Mask.Rect
	w, h := r.Dx(), r.Dy()
	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var maxV uint8
			for dy := -pixels; dy <= pixels; dy++ {
				for dx := -pixels; dx <= pixels; dx++ {
					if dx*dx+dy*dy > pixels*pixels {
						continue
					}
					xx, yy := x+dx, y+dy
					if xx >= 0 && xx < w && yy >= 0 && yy < h {
						v := s.Mask.Pix[yy*s.Mask.Stride+xx]
						if v > maxV {
							maxV = v
						}
					}
				}
			}
			out[y*w+x] = maxV
		}
	}
	copy(s.Mask.Pix, out)
}

// Shrink contracts the selection by the given number of pixels.
func (s *Selection) Shrink(pixels int) {
	if pixels <= 0 {
		return
	}
	r := s.Mask.Rect
	w, h := r.Dx(), r.Dy()
	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			minV := uint8(255)
			for dy := -pixels; dy <= pixels; dy++ {
				for dx := -pixels; dx <= pixels; dx++ {
					if dx*dx+dy*dy > pixels*pixels {
						continue
					}
					xx, yy := x+dx, y+dy
					if xx >= 0 && xx < w && yy >= 0 && yy < h {
						v := s.Mask.Pix[yy*s.Mask.Stride+xx]
						if v < minV {
							minV = v
						}
					} else {
						minV = 0
					}
				}
			}
			out[y*w+x] = minV
		}
	}
	copy(s.Mask.Pix, out)
}

// Union combines two selections (max).
func (s *Selection) Union(other *Selection) {
	for i := range s.Mask.Pix {
		if i < len(other.Mask.Pix) && other.Mask.Pix[i] > s.Mask.Pix[i] {
			s.Mask.Pix[i] = other.Mask.Pix[i]
		}
	}
}

// Intersect keeps only where both selections overlap (min).
func (s *Selection) Intersect(other *Selection) {
	for i := range s.Mask.Pix {
		if i < len(other.Mask.Pix) && other.Mask.Pix[i] < s.Mask.Pix[i] {
			s.Mask.Pix[i] = other.Mask.Pix[i]
		}
	}
}

// Subtract removes other from this selection.
func (s *Selection) Subtract(other *Selection) {
	for i := range s.Mask.Pix {
		if i < len(other.Mask.Pix) {
			v := int(s.Mask.Pix[i]) - int(other.Mask.Pix[i])
			if v < 0 {
				v = 0
			}
			s.Mask.Pix[i] = uint8(v)
		}
	}
}

// MarchingAnts returns the outline pixels of the selection (for visualization).
func (s *Selection) MarchingAnts() []image.Point {
	r := s.Mask.Rect
	w, h := r.Dx(), r.Dy()
	var pts []image.Point
	threshold := uint8(128)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if s.Mask.Pix[y*s.Mask.Stride+x] < threshold {
				continue
			}
			// is it an edge pixel?
			edge := false
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || nx >= w || ny < 0 || ny >= h {
					edge = true
					break
				}
				if s.Mask.Pix[ny*s.Mask.Stride+nx] < threshold {
					edge = true
					break
				}
			}
			if edge {
				pts = append(pts, image.Point{x + r.Min.X, y + r.Min.Y})
			}
		}
	}
	return pts
}

// ApplyToImage applies an effect only within the selection, blending with the
// original based on the mask value.
func (s *Selection) ApplyToImage(original, processed *image.NRGBA) *image.NRGBA {
	out := CloneNRGBA(original)
	r := original.Rect.Intersect(s.Bounds)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			mi := (y-s.Mask.Rect.Min.Y)*s.Mask.Stride + (x - s.Mask.Rect.Min.X)
			alpha := float64(s.Mask.Pix[mi]) / 255.0
			if alpha <= 0 {
				continue
			}
			oi := idx(original, x, y)
			pi := idx(processed, x, y)
			di := idx(out, x, y)
			for c := 0; c < 4; c++ {
				out.Pix[di+c] = clamp8(int(lerp(float64(original.Pix[oi+c]), float64(processed.Pix[pi+c]), alpha)))
			}
		}
	}
	return out
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
