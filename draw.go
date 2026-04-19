package vango

import (
	"image"
	"image/color"
	"math"
)

// --------------------------------------------------------------------------
// Line drawing (Bresenham + anti-aliased Wu's)
// --------------------------------------------------------------------------

// DrawLine draws a 1px anti-aliased line using Wu's algorithm.
func DrawLine(dst *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA, thickness int) {
	if thickness <= 1 {
		drawLineWu(dst, float64(x0), float64(y0), float64(x1), float64(y1), c)
		return
	}
	// Thick line: draw multiple parallel lines
	dx := float64(x1 - x0)
	dy := float64(y1 - y0)
	length := math.Hypot(dx, dy)
	if length == 0 {
		return
	}
	// perpendicular normal
	nx := -dy / length
	ny := dx / length
	half := float64(thickness-1) / 2.0
	for t := -half; t <= half; t += 0.5 {
		drawLineWu(dst,
			float64(x0)+nx*t, float64(y0)+ny*t,
			float64(x1)+nx*t, float64(y1)+ny*t, c)
	}
}

func drawLineWu(dst *image.NRGBA, x0, y0, x1, y1 float64, c color.NRGBA) {
	steep := math.Abs(y1-y0) > math.Abs(x1-x0)
	if steep {
		x0, y0 = y0, x0
		x1, y1 = y1, x1
	}
	if x0 > x1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
	}
	dx := x1 - x0
	dy := y1 - y0
	gradient := 0.0
	if dx != 0 {
		gradient = dy / dx
	}

	// first endpoint
	xend := math.Round(x0)
	yend := y0 + gradient*(xend-x0)
	xpxl1 := int(xend)
	ypxl1 := int(math.Floor(yend))
	blendPixelAA(dst, xpxl1, ypxl1, c, 1.0, steep)

	intery := yend + gradient

	// second endpoint
	xend = math.Round(x1)
	xpxl2 := int(xend)

	for x := xpxl1 + 1; x < xpxl2; x++ {
		iy := int(math.Floor(intery))
		frac := intery - math.Floor(intery)
		blendPixelAA(dst, x, iy, c, 1-frac, steep)
		blendPixelAA(dst, x, iy+1, c, frac, steep)
		intery += gradient
	}
}

func blendPixelAA(dst *image.NRGBA, x, y int, c color.NRGBA, alpha float64, steep bool) {
	if steep {
		x, y = y, x
	}
	if x < dst.Rect.Min.X || x >= dst.Rect.Max.X || y < dst.Rect.Min.Y || y >= dst.Rect.Max.Y {
		return
	}
	i := idx(dst, x, y)
	a := alpha * float64(c.A) / 255.0
	dst.Pix[i+0] = clamp8(int(lerp(float64(dst.Pix[i+0]), float64(c.R), a)))
	dst.Pix[i+1] = clamp8(int(lerp(float64(dst.Pix[i+1]), float64(c.G), a)))
	dst.Pix[i+2] = clamp8(int(lerp(float64(dst.Pix[i+2]), float64(c.B), a)))
	if a > float64(dst.Pix[i+3])/255.0 {
		dst.Pix[i+3] = clamp8(int(a * 255))
	}
}

// --------------------------------------------------------------------------
// Rectangle drawing
// --------------------------------------------------------------------------

// DrawRect draws a rectangle outline.
func DrawRect(dst *image.NRGBA, r image.Rectangle, c color.NRGBA, thickness int) {
	DrawLine(dst, r.Min.X, r.Min.Y, r.Max.X-1, r.Min.Y, c, thickness)
	DrawLine(dst, r.Max.X-1, r.Min.Y, r.Max.X-1, r.Max.Y-1, c, thickness)
	DrawLine(dst, r.Max.X-1, r.Max.Y-1, r.Min.X, r.Max.Y-1, c, thickness)
	DrawLine(dst, r.Min.X, r.Max.Y-1, r.Min.X, r.Min.Y, c, thickness)
}

// FillRect fills a rectangle with a color.
func FillRect(dst *image.NRGBA, r image.Rectangle, c color.NRGBA) {
	r = r.Intersect(dst.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := idx(dst, x, y)
			a := float64(c.A) / 255.0
			dst.Pix[i+0] = clamp8(int(lerp(float64(dst.Pix[i+0]), float64(c.R), a)))
			dst.Pix[i+1] = clamp8(int(lerp(float64(dst.Pix[i+1]), float64(c.G), a)))
			dst.Pix[i+2] = clamp8(int(lerp(float64(dst.Pix[i+2]), float64(c.B), a)))
			dst.Pix[i+3] = clamp8(int(math.Max(float64(dst.Pix[i+3]), float64(c.A))))
		}
	}
}

// --------------------------------------------------------------------------
// Ellipse drawing
// --------------------------------------------------------------------------

// DrawEllipse draws an anti-aliased ellipse outline.
func DrawEllipse(dst *image.NRGBA, cx, cy, rx, ry float64, c color.NRGBA, thickness int) {
	steps := int(math.Max(float64(rx), float64(ry)) * 4)
	if steps < 32 {
		steps = 32
	}
	for i := 0; i < steps; i++ {
		a0 := 2 * math.Pi * float64(i) / float64(steps)
		a1 := 2 * math.Pi * float64(i+1) / float64(steps)
		x0 := cx + rx*math.Cos(a0)
		y0 := cy + ry*math.Sin(a0)
		x1 := cx + rx*math.Cos(a1)
		y1 := cy + ry*math.Sin(a1)
		DrawLine(dst, int(x0), int(y0), int(x1), int(y1), c, thickness)
	}
}

// FillEllipse fills an anti-aliased ellipse.
func FillEllipse(dst *image.NRGBA, cx, cy, rx, ry float64, c color.NRGBA) {
	minY := int(math.Floor(cy - ry))
	maxY := int(math.Ceil(cy + ry))
	for y := minY; y <= maxY; y++ {
		if y < dst.Rect.Min.Y || y >= dst.Rect.Max.Y {
			continue
		}
		dy := (float64(y) + 0.5 - cy) / ry
		if math.Abs(dy) > 1 {
			continue
		}
		halfW := rx * math.Sqrt(1-dy*dy)
		minX := int(math.Floor(cx - halfW))
		maxX := int(math.Ceil(cx + halfW))
		for x := minX; x <= maxX; x++ {
			if x < dst.Rect.Min.X || x >= dst.Rect.Max.X {
				continue
			}
			dx := (float64(x) + 0.5 - cx) / rx
			d := dx*dx + dy*dy
			if d > 1.02 {
				continue
			}
			a := float64(c.A) / 255.0
			if d > 1.0 {
				a *= clampF01(1 - (d-1.0)/0.02)
			}
			i := idx(dst, x, y)
			dst.Pix[i+0] = clamp8(int(lerp(float64(dst.Pix[i+0]), float64(c.R), a)))
			dst.Pix[i+1] = clamp8(int(lerp(float64(dst.Pix[i+1]), float64(c.G), a)))
			dst.Pix[i+2] = clamp8(int(lerp(float64(dst.Pix[i+2]), float64(c.B), a)))
			dst.Pix[i+3] = clamp8(int(math.Max(float64(dst.Pix[i+3]), a*255)))
		}
	}
}

// --------------------------------------------------------------------------
// Polygon drawing
// --------------------------------------------------------------------------

// DrawPolygon draws a polygon outline from a list of points.
func DrawPolygon(dst *image.NRGBA, points []image.Point, c color.NRGBA, thickness int, closed bool) {
	if len(points) < 2 {
		return
	}
	for i := 0; i < len(points)-1; i++ {
		DrawLine(dst, points[i].X, points[i].Y, points[i+1].X, points[i+1].Y, c, thickness)
	}
	if closed && len(points) > 2 {
		last := points[len(points)-1]
		DrawLine(dst, last.X, last.Y, points[0].X, points[0].Y, c, thickness)
	}
}

// FillPolygon fills a polygon using scanline rasterization.
func FillPolygon(dst *image.NRGBA, points []image.Point, c color.NRGBA) {
	if len(points) < 3 {
		return
	}
	// Find bounding box
	minY, maxY := points[0].Y, points[0].Y
	for _, p := range points {
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}

	n := len(points)
	for y := minY; y <= maxY; y++ {
		if y < dst.Rect.Min.Y || y >= dst.Rect.Max.Y {
			continue
		}
		var nodeX []int
		j := n - 1
		for i := 0; i < n; i++ {
			yi, yj := points[i].Y, points[j].Y
			if (yi < y && yj >= y) || (yj < y && yi >= y) {
				xi := points[i].X + (y-yi)*(points[j].X-points[i].X)/(yj-yi)
				nodeX = append(nodeX, xi)
			}
			j = i
		}
		// sort
		for i := 0; i < len(nodeX)-1; i++ {
			for k := i + 1; k < len(nodeX); k++ {
				if nodeX[k] < nodeX[i] {
					nodeX[i], nodeX[k] = nodeX[k], nodeX[i]
				}
			}
		}
		// fill between pairs
		for i := 0; i+1 < len(nodeX); i += 2 {
			x0 := nodeX[i]
			x1 := nodeX[i+1]
			if x0 < dst.Rect.Min.X {
				x0 = dst.Rect.Min.X
			}
			if x1 >= dst.Rect.Max.X {
				x1 = dst.Rect.Max.X - 1
			}
			for x := x0; x <= x1; x++ {
				pi := idx(dst, x, y)
				a := float64(c.A) / 255.0
				dst.Pix[pi+0] = clamp8(int(lerp(float64(dst.Pix[pi+0]), float64(c.R), a)))
				dst.Pix[pi+1] = clamp8(int(lerp(float64(dst.Pix[pi+1]), float64(c.G), a)))
				dst.Pix[pi+2] = clamp8(int(lerp(float64(dst.Pix[pi+2]), float64(c.B), a)))
				dst.Pix[pi+3] = clamp8(int(math.Max(float64(dst.Pix[pi+3]), float64(c.A))))
			}
		}
	}
}

// --------------------------------------------------------------------------
// Rounded Rectangle
// --------------------------------------------------------------------------

// FillRoundedRect fills a rectangle with rounded corners.
func FillRoundedRect(dst *image.NRGBA, r image.Rectangle, radius float64, c color.NRGBA) {
	if radius <= 0 {
		FillRect(dst, r, c)
		return
	}
	maxR := math.Min(float64(r.Dx())/2, float64(r.Dy())/2)
	if radius > maxR {
		radius = maxR
	}

	for y := r.Min.Y; y < r.Max.Y; y++ {
		if y < dst.Rect.Min.Y || y >= dst.Rect.Max.Y {
			continue
		}
		for x := r.Min.X; x < r.Max.X; x++ {
			if x < dst.Rect.Min.X || x >= dst.Rect.Max.X {
				continue
			}
			// Check if we're in a corner region
			inside := true
			corners := [4][2]float64{
				{float64(r.Min.X) + radius, float64(r.Min.Y) + radius},
				{float64(r.Max.X) - radius, float64(r.Min.Y) + radius},
				{float64(r.Max.X) - radius, float64(r.Max.Y) - radius},
				{float64(r.Min.X) + radius, float64(r.Max.Y) - radius},
			}
			fx := float64(x) + 0.5
			fy := float64(y) + 0.5
			for ci, corner := range corners {
				inCorner := false
				switch ci {
				case 0:
					inCorner = fx < corner[0] && fy < corner[1]
				case 1:
					inCorner = fx > corner[0] && fy < corner[1]
				case 2:
					inCorner = fx > corner[0] && fy > corner[1]
				case 3:
					inCorner = fx < corner[0] && fy > corner[1]
				}
				if inCorner {
					dx := fx - corner[0]
					dy := fy - corner[1]
					if dx*dx+dy*dy > radius*radius {
						inside = false
					}
					break
				}
			}
			if inside {
				i := idx(dst, x, y)
				a := float64(c.A) / 255.0
				dst.Pix[i+0] = clamp8(int(lerp(float64(dst.Pix[i+0]), float64(c.R), a)))
				dst.Pix[i+1] = clamp8(int(lerp(float64(dst.Pix[i+1]), float64(c.G), a)))
				dst.Pix[i+2] = clamp8(int(lerp(float64(dst.Pix[i+2]), float64(c.B), a)))
				dst.Pix[i+3] = clamp8(int(math.Max(float64(dst.Pix[i+3]), float64(c.A))))
			}
		}
	}
}

// --------------------------------------------------------------------------
// Gradient generation
// --------------------------------------------------------------------------

// GradientType specifies the gradient shape.
type GradientType int

const (
	GradientLinear GradientType = iota
	GradientRadial
	GradientAngular
	GradientDiamond
)

// GenerateGradient creates a gradient image between two colors.
// For linear: angle in degrees. For radial: cx,cy normalized (0..1).
func GenerateGradient(w, h int, gtype GradientType, c1, c2 color.NRGBA, angle float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy := float64(w)/2, float64(h)/2

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var t float64
			switch gtype {
			case GradientLinear:
				rad := angle * math.Pi / 180
				dx := math.Cos(rad)
				dy := math.Sin(rad)
				t = (float64(x)*dx+float64(y)*dy)/(float64(w)*math.Abs(dx)+float64(h)*math.Abs(dy)) + 0.5
			case GradientRadial:
				dx := (float64(x) - cx) / cx
				dy := (float64(y) - cy) / cy
				t = math.Hypot(dx, dy)
			case GradientAngular:
				dx := float64(x) - cx
				dy := float64(y) - cy
				t = (math.Atan2(dy, dx) + math.Pi) / (2 * math.Pi)
			case GradientDiamond:
				dx := math.Abs(float64(x)-cx) / cx
				dy := math.Abs(float64(y)-cy) / cy
				t = math.Max(dx, dy)
			}
			t = clampF01(t)
			i := (y*w + x) * 4
			img.Pix[i+0] = clamp8(int(lerp(float64(c1.R), float64(c2.R), t)))
			img.Pix[i+1] = clamp8(int(lerp(float64(c1.G), float64(c2.G), t)))
			img.Pix[i+2] = clamp8(int(lerp(float64(c1.B), float64(c2.B), t)))
			img.Pix[i+3] = clamp8(int(lerp(float64(c1.A), float64(c2.A), t)))
		}
	}
	return img
}

// GenerateGradientMulti creates a gradient with multiple color stops.
func GenerateGradientMulti(w, h int, gtype GradientType, stops []GradientStop, angle float64) *image.NRGBA {
	if len(stops) < 2 {
		if len(stops) == 1 {
			return GenerateGradient(w, h, gtype, stops[0].Color, stops[0].Color, angle)
		}
		return image.NewNRGBA(image.Rect(0, 0, w, h))
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy := float64(w)/2, float64(h)/2

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var t float64
			switch gtype {
			case GradientLinear:
				rad := angle * math.Pi / 180
				dx := math.Cos(rad)
				dy := math.Sin(rad)
				t = (float64(x)*dx+float64(y)*dy)/(float64(w)*math.Abs(dx)+float64(h)*math.Abs(dy)) + 0.5
			case GradientRadial:
				dx := (float64(x) - cx) / cx
				dy := (float64(y) - cy) / cy
				t = math.Hypot(dx, dy)
			case GradientAngular:
				dx := float64(x) - cx
				dy := float64(y) - cy
				t = (math.Atan2(dy, dx) + math.Pi) / (2 * math.Pi)
			case GradientDiamond:
				dx := math.Abs(float64(x)-cx) / cx
				dy := math.Abs(float64(y)-cy) / cy
				t = math.Max(dx, dy)
			}
			t = clampF01(t)

			// Find color from stops
			var r, g, b, a float64
			if t <= stops[0].Pos {
				r, g, b, a = float64(stops[0].Color.R), float64(stops[0].Color.G), float64(stops[0].Color.B), float64(stops[0].Color.A)
			} else if t >= stops[len(stops)-1].Pos {
				last := stops[len(stops)-1].Color
				r, g, b, a = float64(last.R), float64(last.G), float64(last.B), float64(last.A)
			} else {
				for j := 0; j < len(stops)-1; j++ {
					if t >= stops[j].Pos && t <= stops[j+1].Pos {
						seg := stops[j+1].Pos - stops[j].Pos
						f := 0.0
						if seg > 0 {
							f = (t - stops[j].Pos) / seg
						}
						r = lerp(float64(stops[j].Color.R), float64(stops[j+1].Color.R), f)
						g = lerp(float64(stops[j].Color.G), float64(stops[j+1].Color.G), f)
						b = lerp(float64(stops[j].Color.B), float64(stops[j+1].Color.B), f)
						a = lerp(float64(stops[j].Color.A), float64(stops[j+1].Color.A), f)
						break
					}
				}
			}
			i := (y*w + x) * 4
			img.Pix[i+0] = clamp8(int(r + 0.5))
			img.Pix[i+1] = clamp8(int(g + 0.5))
			img.Pix[i+2] = clamp8(int(b + 0.5))
			img.Pix[i+3] = clamp8(int(a + 0.5))
		}
	}
	return img
}

// --------------------------------------------------------------------------
// Pattern Fill
// --------------------------------------------------------------------------

// FillPattern tiles a pattern image across an area.
func FillPattern(dst *image.NRGBA, r image.Rectangle, pattern *image.NRGBA) {
	pw := pattern.Rect.Dx()
	ph := pattern.Rect.Dy()
	if pw == 0 || ph == 0 {
		return
	}
	r = r.Intersect(dst.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		py := ((y - r.Min.Y) % ph)
		for x := r.Min.X; x < r.Max.X; x++ {
			px := ((x - r.Min.X) % pw)
			si := idx(pattern, pattern.Rect.Min.X+px, pattern.Rect.Min.Y+py)
			di := idx(dst, x, y)
			sa := float64(pattern.Pix[si+3]) / 255.0
			dst.Pix[di+0] = clamp8(int(lerp(float64(dst.Pix[di+0]), float64(pattern.Pix[si+0]), sa)))
			dst.Pix[di+1] = clamp8(int(lerp(float64(dst.Pix[di+1]), float64(pattern.Pix[si+1]), sa)))
			dst.Pix[di+2] = clamp8(int(lerp(float64(dst.Pix[di+2]), float64(pattern.Pix[si+2]), sa)))
			dst.Pix[di+3] = clamp8(int(math.Max(float64(dst.Pix[di+3]), float64(pattern.Pix[si+3]))))
		}
	}
}

// --------------------------------------------------------------------------
// Flood Fill (paint bucket)
// --------------------------------------------------------------------------

// FloodFill fills contiguous same-color area with a new color.
func FloodFill(dst *image.NRGBA, seedX, seedY int, c color.NRGBA, tolerance uint8) {
	if seedX < dst.Rect.Min.X || seedX >= dst.Rect.Max.X || seedY < dst.Rect.Min.Y || seedY >= dst.Rect.Max.Y {
		return
	}
	si := idx(dst, seedX, seedY)
	seedR, seedG, seedB := dst.Pix[si+0], dst.Pix[si+1], dst.Pix[si+2]
	tol := int(tolerance)

	w := dst.Rect.Dx()
	visited := make([]bool, dst.Rect.Dx()*dst.Rect.Dy())
	queue := []image.Point{{seedX, seedY}}
	visited[(seedY-dst.Rect.Min.Y)*w+(seedX-dst.Rect.Min.X)] = true

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]

		pi := idx(dst, p.X, p.Y)
		dr := absInt(int(dst.Pix[pi+0]) - int(seedR))
		dg := absInt(int(dst.Pix[pi+1]) - int(seedG))
		db := absInt(int(dst.Pix[pi+2]) - int(seedB))
		if dr <= tol && dg <= tol && db <= tol {
			dst.Pix[pi+0] = c.R
			dst.Pix[pi+1] = c.G
			dst.Pix[pi+2] = c.B
			dst.Pix[pi+3] = c.A

			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := p.X+d[0], p.Y+d[1]
				if nx >= dst.Rect.Min.X && nx < dst.Rect.Max.X && ny >= dst.Rect.Min.Y && ny < dst.Rect.Max.Y {
					vi := (ny-dst.Rect.Min.Y)*w + (nx - dst.Rect.Min.X)
					if !visited[vi] {
						visited[vi] = true
						queue = append(queue, image.Point{nx, ny})
					}
				}
			}
		}
	}
}
