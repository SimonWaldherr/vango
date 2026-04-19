package vango

import (
	"context"
	"image"
	"math"
)

// SeamCarve performs content-aware resizing by removing low-energy seams.
// newWidth and newHeight specify the target dimensions.
// Currently supports horizontal seam removal (height reduction) and
// vertical seam removal (width reduction).
func SeamCarve(src *image.NRGBA, newWidth, newHeight int) *image.NRGBA {
	return SeamCarveCtx(context.Background(), src, newWidth, newHeight)
}

func SeamCarveCtx(ctx context.Context, src *image.NRGBA, newWidth, newHeight int) *image.NRGBA {
	img := CloneNRGBA(src)

	// Remove vertical seams (reduce width)
	for img.Rect.Dx() > newWidth {
		select {
		case <-ctx.Done():
			return img
		default:
		}
		energy := computeEnergy(img)
		seam := findVerticalSeam(energy, img.Rect.Dx(), img.Rect.Dy())
		img = removeVerticalSeam(img, seam)
	}

	// Remove horizontal seams (reduce height) by transposing
	for img.Rect.Dy() > newHeight {
		select {
		case <-ctx.Done():
			return img
		default:
		}
		transposed := transposeNRGBA(img)
		energy := computeEnergy(transposed)
		seam := findVerticalSeam(energy, transposed.Rect.Dx(), transposed.Rect.Dy())
		transposed = removeVerticalSeam(transposed, seam)
		img = transposeNRGBA(transposed)
	}

	return img
}

// computeEnergy computes the gradient magnitude energy of each pixel.
func computeEnergy(img *image.NRGBA) []float64 {
	w := img.Rect.Dx()
	h := img.Rect.Dy()
	energy := make([]float64, w*h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Dual gradient energy
			x0 := x - 1
			if x0 < 0 {
				x0 = 0
			}
			x1 := x + 1
			if x1 >= w {
				x1 = w - 1
			}
			y0 := y - 1
			if y0 < 0 {
				y0 = 0
			}
			y1 := y + 1
			if y1 >= h {
				y1 = h - 1
			}

			li := idx(img, img.Rect.Min.X+x0, img.Rect.Min.Y+y)
			ri := idx(img, img.Rect.Min.X+x1, img.Rect.Min.Y+y)
			ti := idx(img, img.Rect.Min.X+x, img.Rect.Min.Y+y0)
			bi := idx(img, img.Rect.Min.X+x, img.Rect.Min.Y+y1)

			dxR := float64(img.Pix[ri]) - float64(img.Pix[li])
			dxG := float64(img.Pix[ri+1]) - float64(img.Pix[li+1])
			dxB := float64(img.Pix[ri+2]) - float64(img.Pix[li+2])

			dyR := float64(img.Pix[bi]) - float64(img.Pix[ti])
			dyG := float64(img.Pix[bi+1]) - float64(img.Pix[ti+1])
			dyB := float64(img.Pix[bi+2]) - float64(img.Pix[ti+2])

			energy[y*w+x] = math.Sqrt(dxR*dxR + dxG*dxG + dxB*dxB + dyR*dyR + dyG*dyG + dyB*dyB)
		}
	}
	return energy
}

// findVerticalSeam finds the minimum-energy vertical seam using dynamic programming.
func findVerticalSeam(energy []float64, w, h int) []int {
	// dp stores cumulative minimum energy
	dp := make([]float64, w*h)
	copy(dp[:w], energy[:w])

	for y := 1; y < h; y++ {
		for x := 0; x < w; x++ {
			minE := dp[(y-1)*w+x]
			if x > 0 {
				if v := dp[(y-1)*w+x-1]; v < minE {
					minE = v
				}
			}
			if x < w-1 {
				if v := dp[(y-1)*w+x+1]; v < minE {
					minE = v
				}
			}
			dp[y*w+x] = energy[y*w+x] + minE
		}
	}

	// Backtrack to find seam
	seam := make([]int, h)
	// Find minimum in last row
	minX := 0
	minE := dp[(h-1)*w]
	for x := 1; x < w; x++ {
		if dp[(h-1)*w+x] < minE {
			minE = dp[(h-1)*w+x]
			minX = x
		}
	}
	seam[h-1] = minX

	for y := h - 2; y >= 0; y-- {
		x := seam[y+1]
		best := x
		bestE := dp[y*w+x]
		if x > 0 && dp[y*w+x-1] < bestE {
			bestE = dp[y*w+x-1]
			best = x - 1
		}
		if x < w-1 && dp[y*w+x+1] < bestE {
			best = x + 1
		}
		seam[y] = best
	}
	return seam
}

// removeVerticalSeam removes one vertical seam from the image.
func removeVerticalSeam(src *image.NRGBA, seam []int) *image.NRGBA {
	w := src.Rect.Dx()
	h := src.Rect.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w-1, h))

	for y := 0; y < h; y++ {
		sx := seam[y]
		for x := 0; x < w-1; x++ {
			srcX := x
			if x >= sx {
				srcX = x + 1
			}
			si := idx(src, src.Rect.Min.X+srcX, src.Rect.Min.Y+y)
			di := (y*(w-1) + x) * 4
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}

// transposeNRGBA rotates image 90 degrees (swap x/y).
func transposeNRGBA(src *image.NRGBA) *image.NRGBA {
	w := src.Rect.Dx()
	h := src.Rect.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := idx(src, src.Rect.Min.X+x, src.Rect.Min.Y+y)
			di := (x*h + y) * 4
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}

// Pipeline method
func (p *Pipeline) SeamCarve(newWidth, newHeight int) *Pipeline {
	p.steps = append(p.steps, step{apply: func(ctx context.Context, img *image.NRGBA) *image.NRGBA {
		return SeamCarveCtx(ctx, img, newWidth, newHeight)
	}})
	return p
}
