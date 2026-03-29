package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"tide/internal/db"
)

func TestFeedManagerListViewGeometry(t *testing.T) {
	fm := FeedManager{
		feeds: []db.Feed{
			{ID: 1, Title: "How-To Geek - Linux", URL: "https://www.howtogeek.com/feed/category/linux/"},
			{ID: 2, Title: "The Verge - Tech Policy (path: verge_policy)", URL: "https://www.theverge.com/rss/index.xml"},
			{ID: 3, Title: "Hacker News - Frontpage (path: hn_top)", URL: "https://news.ycombinator.com/rss"},
			{ID: 4, Title: "XDA Dev", URL: "https://www.xda-developers.com/feed/"},
		},
		cursor: 0,
		mode:   fmList,
	}

	view := fm.View(96, 24, BuildStyles(CatppuccinMocha))
	lines := strings.Split(ansi.Strip(view), "\n")

	if got := len(lines); got > 24 {
		t.Fatalf("expected at most 24 lines, got %d", got)
	}
	if !strings.Contains(lines[len(lines)-1], "BACK") {
		t.Fatalf("expected bottom line to contain action bar, got %q", lines[len(lines)-1])
	}
	if strings.Contains(lines[4], "Linux\n") {
		t.Fatalf("selected row wrapped unexpectedly")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "HTTPS://WWW.HOWTOGEEK.COM/FEED/CATEGORY/LINUX/") {
		t.Fatalf("source URL missing from rendered view")
	}
}
