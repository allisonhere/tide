package feed

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

func Fetch(feedURL string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "rss-reader/1.0")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/json, text/xml, */*")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", feedURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, feedURL)
	}
	return resp.Body, nil
}

// FetchAndParse fetches feedURL, parses it, and follows one level of
// feed auto-discovery if the URL points to an HTML page.
// Returns the parsed feed and the final URL used.
func FetchAndParse(feedURL string) (*ParsedFeed, string, error) {
	body, err := Fetch(feedURL)
	if err != nil {
		return nil, feedURL, err
	}
	defer body.Close()

	parsed, err := Parse(body)
	if err != nil {
		var redirect *ErrNeedRedirect
		if isRedirect(err, &redirect) {
			// Resolve relative URLs against the original
			resolved, resolveErr := resolveURL(feedURL, redirect.URL)
			if resolveErr != nil {
				return nil, feedURL, fmt.Errorf("discovered feed URL %q is invalid: %w", redirect.URL, resolveErr)
			}
			// Retry once with the discovered feed URL
			body2, err2 := Fetch(resolved)
			if err2 != nil {
				return nil, resolved, err2
			}
			defer body2.Close()
			parsed2, err2 := Parse(body2)
			return parsed2, resolved, err2
		}
		return nil, feedURL, err
	}
	return parsed, feedURL, nil
}

func isRedirect(err error, out **ErrNeedRedirect) bool {
	if r, ok := err.(*ErrNeedRedirect); ok {
		*out = r
		return true
	}
	return false
}

func resolveURL(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return b.ResolveReference(r).String(), nil
}
