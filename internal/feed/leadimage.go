package feed

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

// leadImageURL picks the best "lead" image for an item from its feed metadata,
// falling back to the first meaningful <img> in the item HTML. It returns "" when
// nothing usable is found. It never performs any network I/O and is deterministic.
//
// Priority:
//  1. Media RSS media:content / media:group>media:content with medium="image"
//     (largest declared width wins)
//  2. Media RSS media:thumbnail
//  3. an <enclosure> with an image/* type
//  4. item.Image.URL (RSS <image> on the item, rare)
//  5. first non-junk <img src> in the item content HTML
func leadImageURL(item *gofeed.Item) string {
	if item == nil {
		return ""
	}

	if u := mediaRSSImageURL(item); u != "" {
		return u
	}
	for _, enc := range item.Enclosures {
		if enc == nil {
			continue
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(enc.Type)), "image/") {
			if u := cleanImageURL(enc.URL); u != "" && !isJunkImageURL(u) {
				return u
			}
		}
	}
	if item.Image != nil {
		if u := cleanImageURL(item.Image.URL); u != "" && !isJunkImageURL(u) {
			return u
		}
	}

	content := item.Content
	if content == "" {
		content = item.Description
	}
	if u := firstContentImageURL(content); u != "" {
		return u
	}
	return ""
}

// mediaRSSImageURL inspects the "media" extension namespace for media:content and
// media:thumbnail elements, including those nested inside a media:group.
func mediaRSSImageURL(item *gofeed.Item) string {
	media, ok := item.Extensions["media"]
	if !ok {
		return ""
	}

	type cand struct {
		url   string
		width int
	}
	var best cand

	consider := func(attrs map[string]string) {
		u := cleanImageURL(attrs["url"])
		if u == "" || isJunkImageURL(u) {
			return
		}
		medium := strings.ToLower(strings.TrimSpace(attrs["medium"]))
		typ := strings.ToLower(strings.TrimSpace(attrs["type"]))
		isImage := medium == "image" ||
			strings.HasPrefix(typ, "image/") ||
			(medium == "" && typ == "" && looksLikeImagePath(u))
		if !isImage {
			return
		}
		w, _ := strconv.Atoi(strings.TrimSpace(attrs["width"]))
		if best.url == "" || w > best.width {
			best = cand{url: u, width: w}
		}
	}

	for _, ext := range media["content"] {
		consider(ext.Attrs)
	}
	for _, grp := range media["group"] {
		for _, ext := range grp.Children["content"] {
			consider(ext.Attrs)
		}
	}
	if best.url != "" {
		return best.url
	}

	for _, ext := range media["thumbnail"] {
		if u := cleanImageURL(ext.Attrs["url"]); u != "" && !isJunkImageURL(u) {
			return u
		}
	}
	for _, grp := range media["group"] {
		for _, ext := range grp.Children["thumbnail"] {
			if u := cleanImageURL(ext.Attrs["url"]); u != "" && !isJunkImageURL(u) {
				return u
			}
		}
	}
	return ""
}

// LeadImageFromHTML returns the first non-junk <img> src in an HTML fragment, or
// "" when none is found. Used for sources (e.g. Google Reader) that only expose
// article HTML and no structured media metadata.
func LeadImageFromHTML(htmlStr string) string {
	return firstContentImageURL(htmlStr)
}

// firstContentImageURL returns the src of the first <img> in html that is not
// obvious junk (tracking pixel, spacer, tiny icon, avatar, likely logo).
func firstContentImageURL(htmlStr string) string {
	htmlStr = strings.TrimSpace(htmlStr)
	if htmlStr == "" || !strings.Contains(strings.ToLower(htmlStr), "<img") {
		return ""
	}
	doc, err := html.Parse(bytes.NewReader([]byte(htmlStr)))
	if err != nil {
		return ""
	}

	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" || n == nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "img" {
			attrs := attrMap(n.Attr)
			src := attrs["src"]
			if src == "" {
				src = attrs["data-src"]
			}
			if src == "" {
				src = firstSrcFromSrcset(attrs["srcset"])
			}
			u := cleanImageURL(src)
			if u != "" && !isJunkImageURL(u) && !hasTinyDimension(attrs) {
				found = u
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func firstSrcFromSrcset(srcset string) string {
	srcset = strings.TrimSpace(srcset)
	if srcset == "" {
		return ""
	}
	first := strings.SplitN(srcset, ",", 2)[0]
	return strings.TrimSpace(strings.SplitN(strings.TrimSpace(first), " ", 2)[0])
}

// hasTinyDimension reports whether width/height attributes mark the image as a
// 1x1 (or otherwise sub-16px) pixel.
func hasTinyDimension(attrs map[string]string) bool {
	tiny := func(key string) bool {
		v := strings.TrimSpace(attrs[key])
		if v == "" {
			return false
		}
		v = strings.TrimSuffix(v, "px")
		n, err := strconv.Atoi(v)
		return err == nil && n > 0 && n < 16
	}
	return tiny("width") || tiny("height")
}

func cleanImageURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Protocol-relative -> https.
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}

var (
	// junkImageSubstrings are strong enough signals that a bare substring match
	// anywhere in the URL is enough to reject it.
	junkImageSubstrings = []string{
		"tracking", "beacon", "doubleclick", "feedburner", "feedblitz",
		"gravatar", "spacer", "transparent", "pixel.gif", "/pixel", "analytics",
	}
	// junkImageTokens need a delimiter boundary so we don't reject e.g. a photo
	// whose slug merely contains "button".
	junkImagePattern = regexp.MustCompile(`(?i)(^|[/_.\-])(1x1|1px|blank|avatar|emoji|sprite|badge|button|share[_-]?icon|social[_-]?icon)([/_.\-?#]|$)`)
	iconPathPattern  = regexp.MustCompile(`(?i)/(icons?|logos?|favicons?)/`)
	imagePathPattern = regexp.MustCompile(`(?i)\.(jpe?g|png|gif|webp|bmp|avif)(\?|#|$)`)
)

// isJunkImageURL screens out images that are almost never a real lead image:
// data URIs, SVGs, tracking pixels, spacers, tiny icons, avatars and likely logos.
func isJunkImageURL(u string) bool {
	lu := strings.ToLower(strings.TrimSpace(u))
	if lu == "" {
		return true
	}
	if strings.HasPrefix(lu, "data:") {
		return true
	}
	if strings.Contains(lu, ".svg") {
		return true
	}
	for _, s := range junkImageSubstrings {
		if strings.Contains(lu, s) {
			return true
		}
	}
	if junkImagePattern.MatchString(lu) {
		return true
	}
	if iconPathPattern.MatchString(lu) {
		return true
	}
	return false
}

func looksLikeImagePath(u string) bool {
	return imagePathPattern.MatchString(strings.ToLower(u))
}
