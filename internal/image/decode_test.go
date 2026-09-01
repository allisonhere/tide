package image

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestDecode_PNG(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, solid(40, 30))
	img, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("Decode png: %v", err)
	}
	if img.Bounds().Dx() != 40 || img.Bounds().Dy() != 30 {
		t.Fatalf("bounds %v", img.Bounds())
	}
}

func TestDecode_JPEG(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solid(64, 48), &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(buf.Bytes()); err != nil {
		t.Fatalf("Decode jpeg: %v", err)
	}
}

func TestDecode_GIFFirstFrame(t *testing.T) {
	var buf bytes.Buffer
	g := &gif.GIF{}
	pal := image.NewPaletted(image.Rect(0, 0, 32, 32), color.Palette{color.Black, color.White})
	g.Image = append(g.Image, pal)
	g.Delay = append(g.Delay, 0)
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(buf.Bytes()); err != nil {
		t.Fatalf("Decode gif: %v", err)
	}
}

func TestDecode_RejectsTiny(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, solid(8, 8))
	if _, err := Decode(buf.Bytes()); err == nil {
		t.Fatal("expected ErrImageTooSmall for an 8x8 image")
	}
}

func TestDecode_RejectsGarbage(t *testing.T) {
	if _, err := Decode([]byte("not an image")); err == nil {
		t.Fatal("expected decode error for garbage bytes")
	}
	if _, err := Decode(nil); err == nil {
		t.Fatal("expected error for nil bytes")
	}
}
