package image

import (
	"image"

	xdraw "golang.org/x/image/draw"
)

// DefaultMaxRows is the ceiling on how many terminal rows a lead image may
// occupy so a large image never dominates the reader.
const DefaultMaxRows = 12

// Fit returns src scaled to fit within maxPxW x maxPxH pixels while preserving
// aspect ratio. It never upscales past the source's native size. The result is
// always a fresh *image.RGBA (suitable for direct Kitty RGBA transmission or PNG
// re-encoding).
func Fit(src image.Image, maxPxW, maxPxH int) *image.RGBA {
	if src == nil {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw <= 0 || sh <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	if maxPxW < 1 {
		maxPxW = 1
	}
	if maxPxH < 1 {
		maxPxH = 1
	}

	dw, dh := scaledDimensions(sw, sh, maxPxW, maxPxH)

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	if dw == sw && dh == sh {
		xdraw.Copy(dst, image.Point{}, src, sb, xdraw.Src, nil)
		return dst
	}
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, sb, xdraw.Over, nil)
	return dst
}

// scaledDimensions computes the largest w x h that fits inside (maxW, maxH),
// keeps the sw:sh ratio, and does not exceed the source size.
func scaledDimensions(sw, sh, maxW, maxH int) (int, int) {
	// Ratio to shrink by (>= 1 means "fits already").
	rw := float64(sw) / float64(maxW)
	rh := float64(sh) / float64(maxH)
	r := rw
	if rh > r {
		r = rh
	}
	if r <= 1 {
		return sw, sh // fits without scaling; never upscale
	}
	w := int(float64(sw)/r + 0.5)
	h := int(float64(sh)/r + 0.5)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// RowsFor returns how many terminal rows an image of imgPxH pixels needs at a
// cell height of cellPxH, clamped to [1, maxRows].
func RowsFor(imgPxH, cellPxH, maxRows int) int {
	if cellPxH < 1 {
		cellPxH = 16
	}
	if maxRows < 1 {
		maxRows = 1
	}
	rows := (imgPxH + cellPxH - 1) / cellPxH // ceil
	if rows < 1 {
		rows = 1
	}
	if rows > maxRows {
		rows = maxRows
	}
	return rows
}

// ColsFor returns how many terminal columns an image of imgPxW pixels needs at a
// cell width of cellPxW, clamped to [1, maxCols].
func ColsFor(imgPxW, cellPxW, maxCols int) int {
	if cellPxW < 1 {
		cellPxW = 8
	}
	if maxCols < 1 {
		maxCols = 1
	}
	cols := (imgPxW + cellPxW - 1) / cellPxW
	if cols < 1 {
		cols = 1
	}
	if cols > maxCols {
		cols = maxCols
	}
	return cols
}
