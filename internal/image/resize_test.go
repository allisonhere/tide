package image

import (
	"image"
	"image/color"
	"testing"
)

func solid(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	return img
}

func TestFit_PreservesAspectLandscape(t *testing.T) {
	src := solid(1600, 900) // 16:9
	dst := Fit(src, 800, 600)
	if dst.Bounds().Dx() != 800 {
		t.Fatalf("width = %d, want 800", dst.Bounds().Dx())
	}
	// 900 * 800/1600 = 450
	if dst.Bounds().Dy() != 450 {
		t.Fatalf("height = %d, want 450", dst.Bounds().Dy())
	}
}

func TestFit_PreservesAspectPortrait(t *testing.T) {
	src := solid(600, 1200) // 1:2
	dst := Fit(src, 800, 400)
	// height-bound: 1200 -> 400, so scale 1/3, width 200
	if dst.Bounds().Dy() != 400 || dst.Bounds().Dx() != 200 {
		t.Fatalf("got %dx%d, want 200x400", dst.Bounds().Dx(), dst.Bounds().Dy())
	}
}

func TestFit_NeverUpscales(t *testing.T) {
	src := solid(100, 80)
	dst := Fit(src, 800, 600)
	if dst.Bounds().Dx() != 100 || dst.Bounds().Dy() != 80 {
		t.Fatalf("upscaled to %dx%d; must stay 100x80", dst.Bounds().Dx(), dst.Bounds().Dy())
	}
}

func TestFit_WithinBounds(t *testing.T) {
	src := solid(3000, 3000)
	dst := Fit(src, 640, 192)
	if dst.Bounds().Dx() > 640 || dst.Bounds().Dy() > 192 {
		t.Fatalf("result %dx%d exceeds 640x192", dst.Bounds().Dx(), dst.Bounds().Dy())
	}
}

func TestFit_NilAndDegenerate(t *testing.T) {
	if got := Fit(nil, 10, 10); got == nil || got.Bounds().Empty() {
		t.Fatalf("Fit(nil) must return a non-empty image")
	}
	if got := Fit(solid(0, 0), 10, 10); got == nil {
		t.Fatalf("Fit(empty) must not return nil")
	}
}

func TestRowsFor(t *testing.T) {
	cases := []struct {
		imgH, cellH, maxRows, want int
	}{
		{200, 16, 12, 12}, // ceil(12.5)=13 -> clamp 12
		{100, 16, 12, 7},  // ceil(6.25)=7
		{8, 16, 12, 1},    // clamp to 1
		{0, 16, 12, 1},
		{160, 0, 12, 10}, // cellH fallback 16
	}
	for _, c := range cases {
		if got := RowsFor(c.imgH, c.cellH, c.maxRows); got != c.want {
			t.Errorf("RowsFor(%d,%d,%d) = %d, want %d", c.imgH, c.cellH, c.maxRows, got, c.want)
		}
	}
}

func TestColsFor(t *testing.T) {
	if got := ColsFor(640, 8, 80); got != 80 {
		t.Fatalf("ColsFor(640,8,80) = %d, want 80", got)
	}
	if got := ColsFor(300, 8, 80); got != 38 { // ceil(37.5)
		t.Fatalf("ColsFor(300,8,80) = %d, want 38", got)
	}
}
