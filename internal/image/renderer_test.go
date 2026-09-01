package image

import (
	"image"
	"strings"
	"testing"
)

func TestNoopRenderer(t *testing.T) {
	r := NewNoopRenderer()
	if r.Supported() {
		t.Fatal("noop renderer must report unsupported")
	}
	if r.Frame(solid(10, 10), 1, 1, 4, 2) != "" || r.Hide() != "" || r.ClearAll() != "" {
		t.Fatal("noop renderer must produce empty strings")
	}
}

func TestKittyRenderer_FrameTransmitsThenPlaces(t *testing.T) {
	r := NewKittyRenderer()
	if !r.Supported() {
		t.Fatal("kitty renderer should be supported")
	}

	out := r.Frame(solid(64, 32), 10, 4, 8, 2)
	if !strings.Contains(out, "\x1b_Ga=d,d=I,i=1974") {
		t.Fatal("first Frame should begin by freeing the previous image")
	}
	if !strings.Contains(out, "f=100,i=1974,a=t") {
		t.Fatal("first Frame should transmit a PNG payload")
	}
	if !strings.Contains(out, "\x1b7\x1b[4;10H\x1b_Ga=p,i=1974,c=8,r=2,C=1,q=2\x1b\\\x1b8") {
		t.Fatalf("Frame should place at 4;10 without moving the cursor, got:\n%q", out)
	}
}

func TestKittyRenderer_SkipsRetransmitForSameImage(t *testing.T) {
	r := NewKittyRenderer()
	im := solid(48, 48)

	_ = r.Frame(im, 1, 1, 6, 3)
	out := r.Frame(im, 5, 9, 6, 3) // same image, new position

	if strings.Contains(out, "a=t,q=2") || strings.Contains(out, "f=100") {
		t.Fatalf("second Frame of an identical image must not retransmit:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[9;5H") {
		t.Fatalf("second Frame should re-place at the new coords:\n%q", out)
	}
}

func TestKittyRenderer_RetransmitsForDifferentImage(t *testing.T) {
	r := NewKittyRenderer()
	_ = r.Frame(solid(40, 40), 1, 1, 5, 2)
	out := r.Frame(solid(80, 20), 1, 1, 5, 2)
	if !strings.Contains(out, "f=100,i=1974,a=t") {
		t.Fatal("a different image must be retransmitted")
	}
}

func TestKittyRenderer_HideAndClearAll(t *testing.T) {
	r := NewKittyRenderer()
	_ = r.Frame(solid(20, 20), 1, 1, 3, 2)

	if got := r.Hide(); got != "\x1b_Ga=d,d=i,i=1974,q=2\x1b\\" {
		t.Fatalf("Hide = %q", got)
	}
	if got := r.ClearAll(); got != "\x1b_Ga=d,d=A\x1b\\" {
		t.Fatalf("ClearAll = %q", got)
	}
	// After ClearAll the next Frame must retransmit.
	out := r.Frame(solid(20, 20), 1, 1, 3, 2)
	if !strings.Contains(out, "f=100,i=1974,a=t") {
		t.Fatal("Frame after ClearAll should retransmit")
	}
}

func TestKittyRenderer_NilImage(t *testing.T) {
	if NewKittyRenderer().Frame(nil, 1, 1, 2, 2) != "" {
		t.Fatal("Frame(nil) must be empty")
	}
}

func TestImageIdentityStable(t *testing.T) {
	a := imageIdentity(solid(30, 30))
	if a != imageIdentity(solid(30, 30)) {
		t.Fatal("identical images should hash the same")
	}
	if imageIdentity(solid(30, 31)) == a {
		t.Fatal("different-sized images should hash differently")
	}
}

func TestDeleteAllSequence(t *testing.T) {
	if DeleteAllSequence() != "\x1b_Ga=d,d=A\x1b\\" {
		t.Fatalf("DeleteAllSequence = %q", DeleteAllSequence())
	}
}

var _ image.Image = (*image.RGBA)(nil)
