package image

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultFetchTimeout  = 12 * time.Second
	defaultMaxImageBytes = 8 << 20 // 8 MiB
	userAgent            = "tide/1.0 (+https://github.com/allisonhere/tide)"
)

// Fetcher downloads image bytes with a bounded timeout and body size, validates
// the content type, and serves/records results through an optional disk Cache.
// It performs no decoding.
type Fetcher struct {
	Client   *http.Client
	MaxBytes int64
	Cache    *Cache
}

// NewFetcher returns a Fetcher with sane defaults. cache may be nil.
func NewFetcher(cache *Cache) *Fetcher {
	return &Fetcher{
		Client:   &http.Client{Timeout: defaultFetchTimeout},
		MaxBytes: defaultMaxImageBytes,
		Cache:    cache,
	}
}

// ErrNotImage is returned when a response is not an image by content type or
// content sniffing.
var ErrNotImage = errors.New("image: response is not an image")

// ErrTooLarge is returned when a response body exceeds MaxBytes.
var ErrTooLarge = errors.New("image: response body too large")

// Fetch returns the raw bytes for url. On a cache hit it returns immediately.
// On a miss it performs a GET bounded by ctx, validates the payload, writes it
// through the cache, and returns it. The returned string is the detected
// content type ("" on a cache hit).
func (f *Fetcher) Fetch(ctx context.Context, url string) ([]byte, string, error) {
	if strings.TrimSpace(url) == "" {
		return nil, "", errors.New("image: empty url")
	}
	if f.Cache != nil {
		if data, ok := f.Cache.Get(url); ok {
			return data, "", nil
		}
	}

	maxBytes := f.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxImageBytes
	}
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: defaultFetchTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif,image/*;q=0.8,*/*;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("image: http %d", resp.StatusCode)
	}

	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if ct != "" && !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "application/octet-stream") {
		return nil, "", fmt.Errorf("%w (content-type %q)", ErrNotImage, ct)
	}

	limited := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxBytes {
		return nil, "", ErrTooLarge
	}

	sniff := http.DetectContentType(data)
	if !strings.HasPrefix(sniff, "image/") {
		return nil, "", fmt.Errorf("%w (sniffed %q)", ErrNotImage, sniff)
	}

	if f.Cache != nil {
		f.Cache.Put(url, data)
	}
	return data, sniff, nil
}
