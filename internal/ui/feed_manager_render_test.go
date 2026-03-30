package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
	wantBase := adjustLightness(CatppuccinMocha.Bg, 0.06)

	if chrome.baseBg != wantBase {
		t.Fatalf("expected modal base to be raised from theme bg, got %q want %q", chrome.baseBg, wantBase)
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
	wantBase := adjustLightness(CatppuccinLatte.Bg, -0.06)

	if chrome.baseBg != wantBase {
		t.Fatalf("expected light modal base to be lowered from theme bg, got %q want %q", chrome.baseBg, wantBase)
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

func TestRenderTextInputCompactsUnfocusedSecretPreview(t *testing.T) {
	input := textinput.New()
	input.EchoMode = textinput.EchoPassword
	input.EchoCharacter = '●'
	input.SetValue("sk-abcdefghijklmnopqrstuvwxyz123456")

	rendered := ansi.Strip(renderTextInput(input, 30, false, true, newManagerChrome(60, CatppuccinMocha)))

	if strings.Contains(rendered, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("secret preview leaked raw value: %q", rendered)
	}
	if !strings.Contains(rendered, "●●●●●●●●●●●●●●●●●●●●…") {
		t.Fatalf("expected compact masked preview, got %q", rendered)
	}
}

func TestFeedManagerEditViewShowsBusyStatus(t *testing.T) {
	fm := FeedManager{
		mode:      fmEdit,
		statusMsg: "ADDING FEED...",
		busy:      true,
		busyMsg:   "ADDING FEED...",
	}

	view := ansi.Strip(fm.View(80, 20, BuildStyles(CatppuccinMocha)))

	if !strings.Contains(view, "ADDING FEED...") {
		t.Fatalf("expected busy status in edit view, got %q", view)
	}
	if !strings.Contains(view, "WORKING") {
		t.Fatalf("expected busy action hint in edit view, got %q", view)
	}
}

func TestFeedManagerDeleteRequiresY(t *testing.T) {
	fm := FeedManager{
		feeds:  []db.Feed{{ID: 1, Title: "Test Feed", URL: "https://example.com/feed"}},
		mode:   fmConfirmDelete,
		cursor: 0,
	}

	next, cmd := fm.updateConfirmDelete(tea.KeyMsg{Type: tea.KeyEnter}, DefaultKeys)
	if cmd != nil {
		t.Fatal("expected enter not to trigger delete command")
	}
	if next.mode != fmConfirmDelete {
		t.Fatalf("expected delete confirm to remain open on enter, got %v", next.mode)
	}

	next, cmd = fm.updateConfirmDelete(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}, DefaultKeys)
	if cmd == nil {
		t.Fatal("expected y to trigger delete command")
	}
	if next.mode != fmList {
		t.Fatalf("expected delete confirm to close on y, got %v", next.mode)
	}
}

func TestFeedManagerEditViewShowsFolderPickerAndNewField(t *testing.T) {
	fm := FeedManager{
		mode:          fmEdit,
		folders:       []db.Folder{{ID: 1, Name: "Tech"}},
		folderCursor:  2,
		showNewFolder: true,
		focusedField:  3,
	}
	fm.newFolderInput = textinput.New()
	fm.newFolderInput.SetValue("Infra")

	view := ansi.Strip(fm.View(80, 20, BuildStyles(CatppuccinMocha)))

	if !strings.Contains(view, "Folder") {
		t.Fatalf("expected folder picker label, got %q", view)
	}
	if !strings.Contains(view, "+ New folder") {
		t.Fatalf("expected new folder option, got %q", view)
	}
	if !strings.Contains(view, "New") || !strings.Contains(view, "Infra") {
		t.Fatalf("expected new folder input row, got %q", view)
	}
}
