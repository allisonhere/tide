package feed

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

type ParsedFeed struct {
	Title       string
	Description string
	Items       []ParsedItem
}

type ParsedItem struct {
	GUID        string
	Title       string
	Link        string
	Content     string // raw HTML
	ImageURL    string // lead image URL
	PublishedAt time.Time
}

// Parse reads an RSS/Atom/JSON feed from r.
// If the content looks like HTML it attempts feed auto-discovery,
// returning the discovered feed URL as a sentinel error so the caller
// can retry with that URL.
func Parse(r io.Reader) (*ParsedFeed, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	fp := gofeed.NewParser()
	f, err := fp.Parse(bytes.NewReader(data))
	if err != nil {
		// If content looks like HTML, try to find the real feed URL
		if looksLikeHTML(data) {
			if feedURL := discoverFeedURL(data); feedURL != "" {
				return nil, &ErrNeedRedirect{URL: feedURL}
			}
			return nil, fmt.Errorf("URL points to an HTML page — enter the direct feed URL (e.g. /feed, /rss, /atom.xml)")
		}
		// Return the raw gofeed error so the user sees exactly what's wrong
		return nil, err
	}

	pf := &ParsedFeed{
		Title:       f.Title,
		Description: f.Description,
	}
	for _, item := range f.Items {
		pf.Items = append(pf.Items, parseItem(item))
	}
	return pf, nil
}

// ErrNeedRedirect signals that feed auto-discovery found a better URL.
type ErrNeedRedirect struct{ URL string }

func (e *ErrNeedRedirect) Error() string {
	return "redirect to " + e.URL
}

func looksLikeHTML(data []byte) bool {
	prefix := strings.ToLower(strings.TrimSpace(string(data[:min(512, len(data))])))
	return strings.HasPrefix(prefix, "<!doctype html") ||
		strings.HasPrefix(prefix, "<html") ||
		strings.Contains(prefix[:min(200, len(prefix))], "<head")
}

// discoverFeedURL parses HTML and looks for
// <link rel="alternate" type="application/rss+xml" href="...">
func discoverFeedURL(data []byte) string {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "link" {
			attrs := attrMap(n.Attr)
			rel := strings.ToLower(attrs["rel"])
			t := strings.ToLower(attrs["type"])
			if rel == "alternate" && (strings.Contains(t, "rss") ||
				strings.Contains(t, "atom") ||
				strings.Contains(t, "feed")) {
				found = attrs["href"]
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func attrMap(attrs []html.Attribute) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[a.Key] = a.Val
	}
	return m
}

func parseItem(item *gofeed.Item) ParsedItem {
	guid := item.GUID
	if guid == "" {
		guid = item.Link
	}
	if guid == "" {
		guid = fmt.Sprintf("%s-%d", item.Title, time.Now().UnixNano())
	}

	content := item.Content
	if content == "" {
		content = item.Description
	}

	pub := time.Now()
	if item.PublishedParsed != nil {
		pub = *item.PublishedParsed
	} else if item.UpdatedParsed != nil {
		pub = *item.UpdatedParsed
	}

	imageURL := extractLeadImage(item, content)

	return ParsedItem{
		GUID:        guid,
		Title:       item.Title,
		Link:        item.Link,
		Content:     content,
		ImageURL:    imageURL,
		PublishedAt: pub,
	}
}

// extractLeadImage returns the best lead image URL for an item.
// Priority: gofeed Image > first <img> in content > first image enclosure.
func extractLeadImage(item *gofeed.Item, content string) string {
	if item.Image != nil && item.Image.URL != "" {
		return item.Image.URL
	}
	if src := firstImgSrc(content); src != "" {
		return src
	}
	for _, enc := range item.Enclosures {
		if strings.HasPrefix(enc.Type, "image/") && enc.URL != "" {
			return enc.URL
		}
	}
	return ""
}

// firstImgSrc returns the src attribute of the first <img> tag in HTML.
func firstImgSrc(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return ""
	}
	var src string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if src != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "img" {
			attrs := attrMap(n.Attr)
			if attrs["src"] != "" {
				src = attrs["src"]
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return src
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
