//go:build unix

package image

import "testing"

func TestQueryCellSize_NoActiveProbeIsIOFree(t *testing.T) {
	// With activeProbe=false, queryCellSize must not touch /dev/tty; it can
	// only return a real size (if this test host's terminal reports pixels) or
	// 0,0 — never negative, never blocking.
	w, h := queryCellSize(false)
	if w < 0 || h < 0 {
		t.Fatalf("queryCellSize returned negative %dx%d", w, h)
	}
}

func TestParseCellSizeReport(t *testing.T) {
	cases := []struct {
		in   string
		w, h int
	}{
		{"\x1b[6;20;9t", 9, 20},
		{"prefix junk \x1b[6;32;14t trailing", 14, 32},
		{"no report here", 0, 0},
		{"\x1b[6;bad;9t", 0, 0},
	}
	for _, c := range cases {
		w, h := parseCellSizeReport(c.in)
		if w != c.w || h != c.h {
			t.Errorf("parseCellSizeReport(%q) = %dx%d, want %dx%d", c.in, w, h, c.w, c.h)
		}
	}
}
