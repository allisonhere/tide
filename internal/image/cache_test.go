package image

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKeyDeterministic(t *testing.T) {
	a := Key("https://example.com/a.jpg")
	b := Key("https://example.com/a.jpg")
	c := Key("https://example.com/b.jpg")
	if a != b {
		t.Fatal("same URL must produce the same key")
	}
	if a == c {
		t.Fatal("different URLs must produce different keys")
	}
	if len(a) != 64 {
		t.Fatalf("key length = %d, want 64 hex chars", len(a))
	}
}

func TestCachePutGetPathUnderDir(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir)

	url := "https://example.com/hero.png"
	c.Put(url, []byte("PNGDATA"))

	want := filepath.Join(dir, Key(url))
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("cache file not at %s: %v", want, err)
	}

	got, ok := c.Get(url)
	if !ok || string(got) != "PNGDATA" {
		t.Fatalf("Get = %q, %v", got, ok)
	}
}

func TestCacheMissWhenDisabled(t *testing.T) {
	c := NewCache("")
	c.Put("x", []byte("y"))
	if _, ok := c.Get("x"); ok {
		t.Fatal("disabled cache must always miss")
	}
}

func TestCacheRejectsOversizeEntry(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir)
	c.maxEntrySize = 4
	c.Put("u", []byte("too-long"))
	if _, ok := c.Get("u"); ok {
		t.Fatal("entry over maxEntrySize must not be stored")
	}
}

func TestCacheEvictsOldest(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir)
	c.maxTotalSize = 30 // room for ~3 x 10-byte entries

	blob := []byte("0123456789") // 10 bytes
	for i, u := range []string{"a", "b", "c", "d", "e"} {
		c.Put(u, blob)
		// Force strictly increasing mtimes so eviction order is deterministic.
		p := filepath.Join(dir, Key(u))
		ts := time.Unix(1_700_000_000+int64(i), 0)
		_ = os.Chtimes(p, ts, ts)
		c.evict()
	}

	if _, ok := c.Get("a"); ok {
		t.Error("oldest entry 'a' should have been evicted")
	}
	if _, ok := c.Get("e"); !ok {
		t.Error("newest entry 'e' should survive")
	}
}

func TestCacheDeleteCorruptedEntry(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir)
	c.Put("u", []byte("bytes"))
	c.Delete("u")
	if _, ok := c.Get("u"); ok {
		t.Fatal("deleted entry must miss")
	}
}
