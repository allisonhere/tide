package image

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFetch_HappyPathAndCache(t *testing.T) {
	body := pngBytes(t, 32, 32)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/png")
		w.Write(body)
	}))
	defer srv.Close()

	f := NewFetcher(NewCache(t.TempDir()))
	got, ct, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if len(got) != len(body) || ct != "image/png" {
		t.Fatalf("unexpected result: len=%d ct=%q", len(got), ct)
	}

	// Second call served from cache, no new HTTP hit.
	if _, _, err := f.Fetch(context.Background(), srv.URL); err != nil {
		t.Fatalf("cached Fetch err: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected 1 HTTP hit, got %d", hits)
	}
}

func TestFetch_RejectsNonImageContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>nope</html>"))
	}))
	defer srv.Close()

	f := NewFetcher(nil)
	if _, _, err := f.Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("expected content-type rejection")
	}
}

func TestFetch_RejectsSniffedNonImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Lies about being an image; body sniffs as text.
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("this is definitely not a PNG file at all"))
	}))
	defer srv.Close()

	f := NewFetcher(nil)
	if _, _, err := f.Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("expected sniff rejection")
	}
}

func TestFetch_EnforcesMaxBytes(t *testing.T) {
	big := pngBytes(t, 200, 200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(big)
	}))
	defer srv.Close()

	f := NewFetcher(nil)
	f.MaxBytes = 32
	if _, _, err := f.Fetch(context.Background(), srv.URL); err != ErrTooLarge {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestFetch_HTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	f := NewFetcher(nil)
	if _, _, err := f.Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error on 404")
	}
}

func TestFetch_EmptyURL(t *testing.T) {
	f := NewFetcher(nil)
	if _, _, err := f.Fetch(context.Background(), "   "); err == nil {
		t.Fatal("expected error for empty url")
	}
}
