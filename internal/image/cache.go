package image

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// nowFunc is indirected for tests that need deterministic mtimes.
var nowFunc = time.Now

// Cache is a small content-addressed disk cache for downloaded image bytes.
// Every operation degrades gracefully: a cache failure must never break article
// reading, so all methods swallow I/O errors and behave as a miss.
type Cache struct {
	dir          string
	maxTotalSize int64 // whole-cache byte budget; oldest entries evicted past it
	maxEntrySize int64 // per-file byte ceiling; larger payloads are not cached
}

const (
	defaultCacheMaxTotal int64 = 64 << 20 // 64 MiB
	defaultCacheMaxEntry int64 = 8 << 20  // 8 MiB
)

// NewCache returns a Cache rooted at dir (created lazily on first Put). A dir of
// "" disables persistence – every Get misses and Put is a no-op.
func NewCache(dir string) *Cache {
	return &Cache{
		dir:          dir,
		maxTotalSize: defaultCacheMaxTotal,
		maxEntrySize: defaultCacheMaxEntry,
	}
}

// CacheDir returns Tide's on-disk image cache directory
// ($XDG_CACHE_HOME|~/.cache)/rss/images, mirroring the layout of the data and
// config dirs. It does not create the directory.
func CacheDir() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdg != "" {
		return filepath.Join(xdg, "rss", "images"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "rss", "images"), nil
}

// Key is the deterministic cache key for a URL: the hex SHA-256 of the URL
// string. Exposed so tests and callers can reason about cache paths.
func Key(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:])
}

func (c *Cache) path(url string) string {
	if c == nil || c.dir == "" {
		return ""
	}
	return filepath.Join(c.dir, Key(url))
}

// Get returns the cached bytes for url and true, or nil and false on any miss.
func (c *Cache) Get(url string) ([]byte, bool) {
	p := c.path(url)
	if p == "" {
		return nil, false
	}
	data, err := os.ReadFile(p)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	// Touch so LRU eviction keeps recently-used entries.
	_ = touch(p)
	return data, true
}

// Put stores data for url. Payloads over maxEntrySize are not written. After a
// successful write the cache is trimmed back under maxTotalSize by deleting the
// least-recently-modified entries.
func (c *Cache) Put(url string, data []byte) {
	p := c.path(url)
	if p == "" || len(data) == 0 || int64(len(data)) > c.maxEntrySize {
		return
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return
	}
	c.evict()
}

// Delete removes the entry for url (used when its bytes fail to decode).
func (c *Cache) Delete(url string) {
	if p := c.path(url); p != "" {
		_ = os.Remove(p)
	}
}

// evict deletes oldest-by-mtime entries until the cache fits maxTotalSize.
func (c *Cache) evict() {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type item struct {
		path string
		size int64
		mod  int64
	}
	var items []item
	var total int64
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, item{
			path: filepath.Join(c.dir, e.Name()),
			size: info.Size(),
			mod:  info.ModTime().UnixNano(),
		})
		total += info.Size()
	}
	if total <= c.maxTotalSize {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod < items[j].mod })
	for _, it := range items {
		if total <= c.maxTotalSize {
			break
		}
		if err := os.Remove(it.path); err == nil {
			total -= it.size
		}
	}
}

func touch(path string) error {
	now := nowFunc()
	return os.Chtimes(path, now, now)
}
