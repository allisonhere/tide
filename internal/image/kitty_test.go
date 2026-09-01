package image

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBuildTransmitPNG_SingleChunk(t *testing.T) {
	payload := []byte("hello-png-bytes")
	packets := buildTransmitPNG(7, payload, 4096)

	if len(packets) != 2 {
		t.Fatalf("want [delete, transmit], got %d packets: %q", len(packets), packets)
	}
	if !strings.HasPrefix(packets[0], "\x1b_Ga=d,d=I,i=7") {
		t.Fatalf("first packet should delete image 7, got %q", packets[0])
	}
	p := packets[1]
	if !strings.HasPrefix(p, "\x1b_Gf=100,i=7,a=t,q=2,m=0;") || !strings.HasSuffix(p, "\x1b\\") {
		t.Fatalf("unexpected transmit packet framing: %q", p)
	}
	b64 := p[strings.IndexByte(p, ';')+1 : len(p)-len("\x1b\\")]
	got, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("payload round-trip failed: %v / %q", err, got)
	}
}

func TestBuildTransmitPNG_Chunking(t *testing.T) {
	// 10000 raw bytes -> base64 length is 13336, so 4 chunks at chunk=4096.
	raw := make([]byte, 10000)
	for i := range raw {
		raw[i] = byte(i)
	}
	packets := buildTransmitPNG(1, raw, 4096)
	transmit := packets[1:]
	if len(transmit) != 4 {
		t.Fatalf("want 4 transmit chunks, got %d", len(transmit))
	}

	// First chunk carries the control keys and m=1.
	if !strings.Contains(transmit[0], "f=100,i=1,a=t,q=2,m=1;") {
		t.Fatalf("first chunk control keys wrong: %q", transmit[0])
	}
	// Middle chunks are bare m=1.
	for _, mid := range transmit[1:3] {
		if !strings.HasPrefix(mid, "\x1b_Gm=1;") {
			t.Fatalf("middle chunk should be \\x1b_Gm=1;..., got %q", mid[:16])
		}
	}
	// Last chunk is m=0.
	if !strings.HasPrefix(transmit[3], "\x1b_Gm=0;") {
		t.Fatalf("last chunk should be m=0, got %q", transmit[3][:16])
	}

	// Reassembled base64 decodes to the original bytes.
	var b64 strings.Builder
	for _, c := range transmit {
		s := c[strings.IndexByte(c, ';')+1 : len(c)-len("\x1b\\")]
		b64.WriteString(s)
	}
	got, err := base64.StdEncoding.DecodeString(b64.String())
	if err != nil || len(got) != len(raw) {
		t.Fatalf("reassembly failed: err=%v len=%d", err, len(got))
	}
}

func TestBuildTransmitRGBA_Header(t *testing.T) {
	packets := buildTransmitRGBA(3, 20, 10, make([]byte, 20*10*4), 4096)
	if !strings.Contains(packets[1], "f=32,s=20,v=10,i=3,a=t,q=2,m=0;") {
		t.Fatalf("rgba header wrong: %q", packets[1])
	}
}

func TestBuildPlace(t *testing.T) {
	got := buildPlace(9, 5, 3, 40, 10)
	want := "\x1b7" + "\x1b[3;5H" + "\x1b_Ga=p,i=9,c=40,r=10,C=1,q=2\x1b\\" + "\x1b8"
	if got != want {
		t.Fatalf("buildPlace:\n got %q\nwant %q", got, want)
	}
}

func TestBuildPlace_ClampsCoords(t *testing.T) {
	got := buildPlace(1, 0, -4, 10, 2)
	if !strings.Contains(got, "\x1b[1;1H") {
		t.Fatalf("coords should clamp to 1;1, got %q", got)
	}
}

func TestBuildDelete(t *testing.T) {
	if got := buildDelete(1974); got != "\x1b_Ga=d,d=I,i=1974,q=2\x1b\\" {
		t.Fatalf("buildDelete = %q", got)
	}
}

func TestBuildDeleteAll(t *testing.T) {
	if got := buildDeleteAll(); got != "\x1b_Ga=d,d=A\x1b\\" {
		t.Fatalf("buildDeleteAll = %q", got)
	}
}

func TestWrapAtCursor(t *testing.T) {
	got := wrapAtCursor(12, 7, "PAYLOAD")
	want := "\x1b7\x1b[7;12HPAYLOAD\x1b8"
	if got != want {
		t.Fatalf("wrapAtCursor = %q, want %q", got, want)
	}
}

func TestProbeSequence(t *testing.T) {
	got := probeSequence(31)
	if !strings.HasPrefix(got, "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA") || !strings.HasSuffix(got, "\x1b\\") {
		t.Fatalf("probeSequence = %q", got)
	}
}
