package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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

func TestManagerChromeUsesThemeOverlaySurface(t *testing.T) {
	chrome := newManagerChrome(80, CatppuccinMocha)

	if chrome.baseBg != CatppuccinMocha.Overlay {
		t.Fatalf("expected modal base to use theme overlay, got %q want %q", chrome.baseBg, CatppuccinMocha.Overlay)
	}
	if chrome.border != CatppuccinMocha.OverlayBorder {
		t.Fatalf("expected modal border to use theme overlay border, got %q want %q", chrome.border, CatppuccinMocha.OverlayBorder)
	}
	if chrome.accent != CatppuccinMocha.BorderFocus {
		t.Fatalf("expected modal accent to use theme focus border, got %q want %q", chrome.accent, CatppuccinMocha.BorderFocus)
	}
	if chrome.baseBg == lipgloss.Color("#0c0e14") || chrome.surfaceBg == lipgloss.Color("#111319") || chrome.fieldBg == lipgloss.Color("#1e2235") {
		t.Fatal("manager chrome still uses hard-coded dark modal colors")
	}
	if contrastRatio(chrome.text, chrome.baseBg) < 4.5 {
		t.Fatalf("expected accessible text contrast on modal base, got %.2f", contrastRatio(chrome.text, chrome.baseBg))
	}
}

func TestManagerChromeKeepsReadableLightThemeContrast(t *testing.T) {
	chrome := newManagerChrome(80, CatppuccinLatte)

	if chrome.baseBg != CatppuccinLatte.Overlay {
		t.Fatalf("expected light modal base to use theme overlay, got %q want %q", chrome.baseBg, CatppuccinLatte.Overlay)
	}
	if chrome.baseBg == CatppuccinLatte.Bg {
		t.Fatal("expected modal base to remain distinct from main background")
	}
	if contrastRatio(chrome.text, chrome.baseBg) < 4.5 {
		t.Fatalf("expected readable text on light modal base, got %.2f", contrastRatio(chrome.text, chrome.baseBg))
	}
	if contrastRatio(chrome.muted, chrome.baseBg) < 3 {
		t.Fatalf("expected readable muted text on light modal base, got %.2f", contrastRatio(chrome.muted, chrome.baseBg))
	}
}
