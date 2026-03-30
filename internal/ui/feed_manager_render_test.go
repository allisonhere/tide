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

	view := fm.View(96, 24, BuildStyles(CatppuccinMocha), true)
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

	view := ansi.Strip(fm.View(80, 20, BuildStyles(CatppuccinMocha), true))

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
		focusedField:  4,
		colorCursor:   1,
	}
	fm.newFolderInput = textinput.New()
	fm.newFolderInput.SetValue("Infra")

	view := ansi.Strip(fm.View(80, 20, BuildStyles(CatppuccinMocha), true))

	if !strings.Contains(view, "Folder") {
		t.Fatalf("expected folder picker label, got %q", view)
	}
	if !strings.Contains(view, "+ New folder") {
		t.Fatalf("expected new folder option, got %q", view)
	}
	if !strings.Contains(view, "New") || !strings.Contains(view, "Infra") {
		t.Fatalf("expected new folder input row, got %q", view)
	}
	if !strings.Contains(view, "Color") {
		t.Fatalf("expected color picker row, got %q", view)
	}
}

func TestFeedManagerEditUsesPickedFolderColorInsteadOfSelectedRow(t *testing.T) {
	fm := FeedManager{
		mode:         fmEdit,
		folders:      []db.Folder{{ID: 1, Name: "One", Color: "#7aa2f7"}, {ID: 2, Name: "Two", Color: "#f7768e"}},
		rows:         []fmRow{{kind: fmRowFolder, folderID: 1}, {kind: fmRowFolder, folderID: 2}},
		cursor:       0,
		folderCursor: 2, // picks folder ID 2
	}

	fm.syncFolderPicker()

	if got := string(fm.currentColorOption().Color); got != "#f7768e" {
		t.Fatalf("expected picked folder color #f7768e, got %q", got)
	}
}

func TestFeedManagerDisplayedColorTracksPickedFolderWhileBrowsing(t *testing.T) {
	fm := FeedManager{
		mode:         fmEdit,
		folders:      []db.Folder{{ID: 1, Name: "One", Color: "#7aa2f7"}, {ID: 2, Name: "Two", Color: "#f7768e"}},
		folderCursor: 1,
		focusedField: 2,
		colorCursor:  0,
	}

	fm.syncFolderPicker()
	if got := string(fm.displayedColorOption().Color); got != "#7aa2f7" {
		t.Fatalf("expected first picked folder color #7aa2f7, got %q", got)
	}

	fm.folderCursor = 2
	fm.syncFolderPicker()
	if got := string(fm.displayedColorOption().Color); got != "#f7768e" {
		t.Fatalf("expected second picked folder color #f7768e, got %q", got)
	}
}

func TestFeedManagerExistingFolderColorIsReadOnlyInFeedEdit(t *testing.T) {
	fm := FeedManager{
		mode:         fmEdit,
		folders:      []db.Folder{{ID: 1, Name: "One", Color: "#7aa2f7"}, {ID: 2, Name: "Two", Color: "#f7768e"}},
		folderCursor: 1,
		focusedField: 4,
		colorCursor:  0,
	}

	fm.syncFolderPicker()
	before := string(fm.displayedColorOption().Color)
	next, _ := fm.updateEdit(tea.KeyMsg{Type: tea.KeyRight}, DefaultKeys)
	after := string(next.displayedColorOption().Color)

	if before != "#7aa2f7" || after != "#7aa2f7" {
		t.Fatalf("expected existing folder color to stay #7aa2f7, got before=%q after=%q", before, after)
	}
}

func TestFeedManagerListShowsFoldersBeforeFeeds(t *testing.T) {
	fm := FeedManager{
		folders: []db.Folder{{ID: 1, Name: "Tech", Color: "#7aa2f7"}},
		feeds:   []db.Feed{{ID: 2, Title: "Feed One", URL: "https://example.com/feed"}},
		cursor:  0,
		mode:    fmList,
	}

	view := ansi.Strip(fm.View(96, 24, BuildStyles(CatppuccinMocha), true))
	if !strings.Contains(view, "FOLDERS + FEEDS") {
		t.Fatalf("expected combined manager section, got %q", view)
	}
	if !strings.Contains(view, "󰉋 TECH") {
		t.Fatalf("expected folder row in manager list, got %q", view)
	}
	if !strings.Contains(view, "\U000f046b FEED ONE") {
		t.Fatalf("expected feed icon in manager list, got %q", view)
	}
	if !strings.Contains(view, "EDIT") || !strings.Contains(view, "DELETE") {
		t.Fatalf("expected generic actions in footer, got %q", view)
	}
}

func TestFeedManagerListShowsCollapsedFolderIcon(t *testing.T) {
	fm := FeedManager{
		folders:          []db.Folder{{ID: 1, Name: "Tech", Color: "#7aa2f7"}},
		collapsedFolders: map[int64]bool{1: true},
		cursor:           0,
		mode:             fmList,
	}

	view := ansi.Strip(fm.View(96, 24, BuildStyles(CatppuccinMocha), true))
	if !strings.Contains(view, "󰉖 TECH") {
		t.Fatalf("expected collapsed folder icon in manager list, got %q", view)
	}
}

func TestFeedManagerListOmitsIconsWhenDisabled(t *testing.T) {
	fm := FeedManager{
		folders: []db.Folder{{ID: 1, Name: "Tech", Color: "#7aa2f7"}},
		feeds:   []db.Feed{{ID: 2, Title: "Feed One", URL: "https://example.com/feed"}},
		cursor:  0,
		mode:    fmList,
	}

	view := ansi.Strip(fm.View(96, 24, BuildStyles(CatppuccinMocha), false))
	if strings.Contains(view, "󰉋 TECH") || strings.Contains(view, "󰉖 TECH") {
		t.Fatalf("expected no folder glyphs when icons disabled, got %q", view)
	}
	if strings.Contains(view, "\U000f046b FEED ONE") {
		t.Fatalf("expected no feed glyph when icons disabled, got %q", view)
	}
}

func TestFeedManagerFolderEditViewShowsNameAndColor(t *testing.T) {
	fm := FeedManager{
		mode:             fmFolderEdit,
		folderEditTarget: 1,
		focusedField:     4,
		colorCursor:      2,
	}
	fm.titleInput = textinput.New()
	fm.titleInput.SetValue("Tech")

	view := ansi.Strip(fm.View(80, 20, BuildStyles(CatppuccinMocha), true))
	if !strings.Contains(view, "EDIT FOLDER") {
		t.Fatalf("expected folder edit header, got %q", view)
	}
	if !strings.Contains(view, "Name") || !strings.Contains(view, "Color") {
		t.Fatalf("expected folder edit fields, got %q", view)
	}
}

func TestFeedManagerAddFolderCanFocusColorField(t *testing.T) {
	fm := FeedManager{}
	fm.titleInput = textinput.New()

	fm.focusAddFolder()
	if fm.focusedField != 0 {
		t.Fatalf("expected add-folder flow to start on name field, got %d", fm.focusedField)
	}

	next, _ := fm.updateFolderEdit(tea.KeyMsg{Type: tea.KeyTab}, DefaultKeys)
	if next.focusedField != 4 {
		t.Fatalf("expected tab to move focus to color field, got %d", next.focusedField)
	}

	next, _ = next.updateFolderEdit(tea.KeyMsg{Type: tea.KeyDown}, DefaultKeys)
	if next.focusedField != 0 {
		t.Fatalf("expected down from color field to wrap to name field, got %d", next.focusedField)
	}
}
