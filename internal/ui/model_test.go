package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tide/internal/config"
	"tide/internal/db"
)

func TestFeedsLoadedPopulatesPane(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())

	// Simulate WindowSizeMsg so width/height are set
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = m2.(Model)

	// Simulate FeedsLoadedMsg with one feed
	feed := db.Feed{ID: 1, Title: "XDA", URL: "https://example.com/feed"}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: []db.Feed{feed}})
	m = m2.(Model)

	if len(m.feeds) != 1 {
		t.Errorf("expected 1 feed after FeedsLoadedMsg, got %d", len(m.feeds))
	}
	if m.overlay != overlayNone {
		t.Errorf("expected overlay=None, got %v", m.overlay)
	}

	// Verify the feed pane renders with the feed name
	view := m.View()
	if view == "" {
		t.Fatal("View() returned empty string")
	}
	if !containsString(view, "XDA") {
		t.Errorf("feeds pane does not contain 'XDA'")
		// Print a truncated view for debugging
		if len(view) > 500 {
			t.Logf("view (first 500 chars): %q", view[:500])
		} else {
			t.Logf("view: %q", view)
		}
	}
	if m.firstLoad {
		t.Error("firstLoad should be false after FeedsLoadedMsg")
	}
}

func TestFirstLoadEmptyDoesNotOpenOverlay(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())

	m2, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = m2.(Model)

	// Simulate empty FeedsLoadedMsg on first load
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: nil})
	m = m2.(Model)

	if m.overlay != overlayNone {
		t.Errorf("expected overlay=None for empty first load, got %v", m.overlay)
	}
}

func TestFeedPaneKeepsCurrentFeedVisibleWhenUnfocused(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())

	m2, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = m2.(Model)

	feed := db.Feed{ID: 1, Title: "Visible Feed", URL: "https://example.com/feed"}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: []db.Feed{feed}})
	m = m2.(Model)

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = m2.(Model)

	view := m.View()
	if !containsString(view, "Visible Feed") {
		t.Fatalf("feed pane lost current feed when unfocused: %q", view)
	}
}

func TestLongStatusMessageDoesNotChangeViewHeight(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = m2.(Model)

	feed := db.Feed{ID: 1, Title: "Stable Feed", URL: "https://example.com/feed"}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: []db.Feed{feed}})
	m = m2.(Model)

	baseHeight := strings.Count(m.View(), "\n")

	m.setStatus("this is a very long error message that should stay on one line and not push the whole layout up or make the feed pane disappear when it renders at the bottom of the screen", true)
	view := m.View()

	if strings.Count(view, "\n") != baseHeight {
		t.Fatalf("view height changed when status message was shown: before=%d after=%d", baseHeight, strings.Count(view, "\n"))
	}
	if !containsString(view, "Stable Feed") {
		t.Fatalf("feed pane lost feed title when status message was shown: %q", view)
	}
}

func TestFeedPaneDoesNotStartWithBlankLines(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = m2.(Model)

	feed := db.Feed{ID: 1, Title: "Top Feed", URL: "https://example.com/feed"}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: []db.Feed{feed}})
	m = m2.(Model)

	pane := m.renderFeedsPane()
	if strings.HasPrefix(pane, "\n\n") {
		t.Fatalf("feed pane starts with unexpected blank lines: %q", pane[:min(40, len(pane))])
	}
	if !containsString(pane, "Top Feed") {
		t.Fatalf("feed pane missing feed title: %q", pane)
	}
}

func TestCursorMoveDoesNotChangeRenderedLineCount(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	feeds := []db.Feed{
		{ID: 1, Title: "Linux Magazine News (path: lmi_news)", URL: "https://example.com/1"},
		{ID: 2, Title: "XDA", URL: "https://example.com/2"},
		{ID: 3, Title: "XDA Dev", URL: "https://example.com/3"},
	}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: feeds})
	m = m2.(Model)

	before := m.View()
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = m2.(Model)
	after := m.View()

	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	if len(beforeLines) != len(afterLines) {
		t.Fatalf("line count changed on cursor move: before=%d after=%d", len(beforeLines), len(afterLines))
	}

	for i := range beforeLines {
		beforeWidth := lipgloss.Width(beforeLines[i])
		afterWidth := lipgloss.Width(afterLines[i])
		if beforeWidth != afterWidth {
			t.Fatalf("line %d width changed on cursor move: before=%d after=%d\nbefore=%q\nafter=%q", i+1, beforeWidth, afterWidth, beforeLines[i], afterLines[i])
		}
	}
}

func TestFeedSelectionChangeWithArticlesKeepsFrameStable(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	feeds := []db.Feed{
		{ID: 1, Title: "Feed One", URL: "https://example.com/1"},
		{ID: 2, Title: "Feed Two", URL: "https://example.com/2"},
	}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: feeds})
	m = m2.(Model)

	articlesOne := []db.Article{
		{ID: 1, FeedID: 1, Title: "Short", Link: "https://example.com/a", Content: "one", PublishedAt: unixTestTime(1710000000)},
	}
	m2, _ = m.Update(ArticlesLoadedMsg{FeedID: 1, Articles: articlesOne})
	m = m2.(Model)
	before := m.View()

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = m2.(Model)
	articlesTwo := []db.Article{
		{ID: 2, FeedID: 2, Title: "A much longer article title for wrapping checks", Link: "https://example.com/b", Content: strings.Repeat("content line\n", 40), PublishedAt: unixTestTime(1710000100)},
	}
	m2, _ = m.Update(ArticlesLoadedMsg{FeedID: 2, Articles: articlesTwo})
	m = m2.(Model)
	after := m.View()

	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	if len(beforeLines) != len(afterLines) {
		t.Fatalf("frame height changed after feed selection update: before=%d after=%d", len(beforeLines), len(afterLines))
	}
	for i := range beforeLines {
		if lipgloss.Width(beforeLines[i]) != lipgloss.Width(afterLines[i]) {
			t.Fatalf("frame width changed at line %d after feed selection update", i+1)
		}
	}
}

func TestBuildStylesUsesThemeOverlayColors(t *testing.T) {
	styles := BuildStyles(CatppuccinMocha)
	wantBg := adjustLightness(CatppuccinMocha.Bg, 0.06)

	if got := styles.Overlay.GetBackground(); got != wantBg {
		t.Fatalf("expected overlay background %q, got %q", wantBg, got)
	}
	if got := styles.Overlay.GetBorderTopForeground(); got != CatppuccinMocha.OverlayBorder {
		t.Fatalf("expected overlay border color %q, got %q", CatppuccinMocha.OverlayBorder, got)
	}
	if got := styles.OverlayTitle.GetBackground(); got != CatppuccinMocha.BorderFocus {
		t.Fatalf("expected overlay title accent %q, got %q", CatppuccinMocha.BorderFocus, got)
	}
}

func TestIconsToggleChangesRenderedPaneMarkers(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	cfg := config.DefaultConfig()
	cfg.Display.Icons = true
	m := NewModel(database, cfg)
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	feeds := []db.Feed{{ID: 1, Title: "Feed One", URL: "https://example.com/feed", UnreadCount: 2}}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: feeds})
	m = m2.(Model)
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = m2.(Model)
	m2, _ = m.Update(ArticlesLoadedMsg{FeedID: 1, Articles: []db.Article{
		{ID: 1, FeedID: 1, Title: "Unread Article", PublishedAt: unixTestTime(1710000000), Read: false},
		{ID: 2, FeedID: 1, Title: "Read Article", PublishedAt: unixTestTime(1710000100), Read: true},
	}})
	m = m2.(Model)

	view := m.View()
	if !containsString(view, "◉ Feeds") {
		t.Fatalf("expected feeds header icon when icons are enabled: %q", view)
	}
	if !containsString(view, "≣ Articles") {
		t.Fatalf("expected articles header icon when icons are enabled: %q", view)
	}
	if !containsString(view, "● Unread Article") {
		t.Fatalf("expected unread article icon when icons are enabled: %q", view)
	}
	if !containsString(view, "· Read Article") {
		t.Fatalf("expected read article marker when icons are enabled: %q", view)
	}
}

func TestSettingsCanReopenAfterSave(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = m2.(Model)
	if m.overlay != overlaySettings {
		t.Fatalf("expected settings overlay to open, got %v", m.overlay)
	}

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = m2.(Model)
	if m.overlay != overlayNone {
		t.Fatalf("expected settings overlay to close on save, got %v", m.overlay)
	}

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = m2.(Model)
	if m.overlay != overlaySettings {
		t.Fatalf("expected settings overlay to reopen after save, got %v", m.overlay)
	}
}

func TestSettingsViewShowsFeedMaxSizeField(t *testing.T) {
	s := newSettings(config.DefaultConfig())

	view := s.View(100, 30, newManagerChrome(100, CatppuccinMocha))

	if !containsString(view, "Feed max size (MiB)") {
		t.Fatalf("expected feed max size field in settings view: %q", view)
	}
}

func TestRightKeyReachesContentPane(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = m2.(Model)
	if m.focused != paneArticles {
		t.Fatalf("expected first l to focus articles, got %v", m.focused)
	}

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = m2.(Model)
	if m.focused != paneContent {
		t.Fatalf("expected second l to focus content, got %v", m.focused)
	}
}

func TestQuitOverlayDoesNotCloseOnQ(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = m2.(Model)
	if m.overlay != overlayQuitConfirm {
		t.Fatalf("expected quit confirm overlay, got %v", m.overlay)
	}

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = m2.(Model)
	if m.overlay != overlayQuitConfirm {
		t.Fatalf("expected q to leave quit confirm open, got %v", m.overlay)
	}
}

func TestSummaryUnavailableFromFeedsPane(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	feeds := []db.Feed{{ID: 1, Title: "Feed One", URL: "https://example.com/feed"}}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: feeds})
	m = m2.(Model)

	articles := []db.Article{
		{ID: 1, FeedID: 1, Title: "Article One", Content: "Body", PublishedAt: unixTestTime(1710000000)},
	}
	m2, _ = m.Update(ArticlesLoadedMsg{FeedID: 1, Articles: articles})
	m = m2.(Model)

	m.focused = paneFeeds
	m2, _ = m.handleMainKey(tea.KeyMsg{Runes: []rune{'s'}})
	m = m2.(Model)

	if m.overlay == overlaySummary {
		t.Fatal("summary overlay should not open from feeds pane")
	}
}

func TestFormatSummaryBodyPreservesParagraphsAndLists(t *testing.T) {
	body := "First paragraph with extra   spacing.\n\n- first bullet item\n- second bullet item\n\n1. first numbered item\n2. second numbered item"

	got := formatSummaryBody(body, 24)

	if !strings.Contains(got, "First paragraph with") {
		t.Fatalf("expected formatted paragraph, got %q", got)
	}
	if !strings.Contains(got, "• first bullet item") {
		t.Fatalf("expected bullet formatting, got %q", got)
	}
	if !strings.Contains(got, "1. first numbered item") {
		t.Fatalf("expected numbered list formatting, got %q", got)
	}
	if !strings.Contains(got, "\n\n• first bullet item") {
		t.Fatalf("expected paragraph break before list, got %q", got)
	}
}

func TestFormatSummaryBodySplitsDenseSingleParagraph(t *testing.T) {
	body := "Sentence one explains the setup. Sentence two adds context. Sentence three gives the key point. Sentence four closes it out."

	got := formatSummaryBody(body, 48)

	if !strings.Contains(got, "\n\nSentence three gives the key point.") {
		t.Fatalf("expected dense summary to split into short paragraphs, got %q", got)
	}
}

func TestArticleCursorMoveKeepsFrameStable(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	feeds := []db.Feed{{ID: 1, Title: "Feed One", URL: "https://example.com/1"}}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: feeds})
	m = m2.(Model)
	m.focused = paneArticles

	articles := []db.Article{
		{ID: 1, FeedID: 1, Title: "Short title", Link: "https://example.com/a", Content: "one", PublishedAt: unixTestTime(1710000000), Read: false},
		{ID: 2, FeedID: 1, Title: "A much longer article title for width stability testing", Link: "https://example.com/b", Content: "two", PublishedAt: unixTestTime(1710000100), Read: true},
	}
	m2, _ = m.Update(ArticlesLoadedMsg{FeedID: 1, Articles: articles})
	m = m2.(Model)

	before := m.View()
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = m2.(Model)
	after := m.View()

	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	if len(beforeLines) != len(afterLines) {
		t.Fatalf("frame height changed after article cursor move: before=%d after=%d", len(beforeLines), len(afterLines))
	}
	for i := range beforeLines {
		if lipgloss.Width(beforeLines[i]) != lipgloss.Width(afterLines[i]) {
			t.Fatalf("frame width changed at line %d after article cursor move", i+1)
		}
	}
}

func TestContentPaneClampsViewportOutputToPaneSize(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	feeds := []db.Feed{{ID: 1, Title: "Feed One", URL: "https://example.com/1"}}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: feeds})
	m = m2.(Model)

	articles := []db.Article{
		{ID: 1, FeedID: 1, Title: "Short title", Link: "https://example.com/a", Content: "one short line", PublishedAt: unixTestTime(1710000000)},
	}
	m2, _ = m.Update(ArticlesLoadedMsg{FeedID: 1, Articles: articles})
	m = m2.(Model)

	w := m.articlesPaneWidth()
	bodyH := m.contentBodyHeight()
	bg := m.styles.Theme.Bg

	vp := m.viewport
	vp.Width = w
	vp.Height = bodyH
	vp.Style = lipgloss.NewStyle().Background(bg)
	wantBody := clampView(vp.View(), w, bodyH, bg)

	got := m.renderContentPane()
	if !strings.Contains(got, wantBody) {
		t.Fatalf("expected content pane to include clamped viewport body")
	}
}

func TestRenderArticleContentFillsPaneWidth(t *testing.T) {
	m := Model{
		width:  120,
		height: 30,
		styles: BuildStyles(GruvboxLight),
	}

	got := m.renderArticleContent(db.Article{
		Title:       "Short title",
		Link:        "https://example.com/a",
		Content:     "one short line",
		PublishedAt: unixTestTime(1710000000),
	})

	for i, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) != m.articlesPaneWidth() {
			t.Fatalf("expected article content line %d to fill pane width %d, got %d", i+1, m.articlesPaneWidth(), lipgloss.Width(line))
		}
	}
}

func TestThemePickerUsesFullWidthChromeRows(t *testing.T) {
	m := Model{
		width:       120,
		height:      30,
		themeCursor: 7,
		styles:      BuildStyles(GruvboxLight),
	}

	chrome := newManagerChrome(40, m.styles.Theme)
	got := m.renderThemePicker(40, chrome)
	lines := strings.Split(got, "\n")

	if !containsString(got, "THEME") {
		t.Fatalf("expected theme picker header, got %q", got)
	}
	if !containsString(got, "gruvbox-light") {
		t.Fatalf("expected theme picker to include current selection row, got %q", got)
	}
	for i, line := range lines {
		if lipgloss.Width(line) != 40 {
			t.Fatalf("expected theme picker line %d to fill width 40, got %d", i+1, lipgloss.Width(line))
		}
	}
}

func TestFeedsPaneRendersFoldersAndUncategorized(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	feeds := []db.Feed{
		{ID: 1, Title: "Alpha", URL: "https://example.com/1", FolderID: 10, UnreadCount: 2},
		{ID: 2, Title: "Beta", URL: "https://example.com/2"},
	}
	folders := []db.Folder{{ID: 10, Name: "Tech", Position: 0}}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: feeds, Folders: folders})
	m = m2.(Model)

	pane := m.renderFeedsPane()
	if !containsString(pane, "Tech") {
		t.Fatalf("expected folder name in feed pane: %q", pane)
	}
	if !containsString(pane, "Uncategorized") {
		t.Fatalf("expected uncategorized group in feed pane: %q", pane)
	}
	if !containsString(pane, "1 folders · 2 feeds") {
		t.Fatalf("expected folder/feed footer in pane: %q", pane)
	}
}

func TestFolderSelectionClearsArticlesAndToggleCollapses(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	feeds := []db.Feed{
		{ID: 1, Title: "Alpha", URL: "https://example.com/1", FolderID: 10},
		{ID: 2, Title: "Beta", URL: "https://example.com/2", FolderID: 10},
	}
	folders := []db.Folder{{ID: 10, Name: "Tech", Position: 0}}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: feeds, Folders: folders})
	m = m2.(Model)

	m2, _ = m.Update(ArticlesLoadedMsg{FeedID: 1, Articles: []db.Article{
		{ID: 1, FeedID: 1, Title: "Article", PublishedAt: unixTestTime(1710000000)},
	}})
	m = m2.(Model)

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = m2.(Model)
	if len(m.filteredArticles) != 0 {
		t.Fatalf("expected folder selection to clear articles, got %d", len(m.filteredArticles))
	}

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)
	if len(m.sidebarRows) != 1 {
		t.Fatalf("expected collapsed folder to hide feed rows, got %d sidebar rows", len(m.sidebarRows))
	}
}

func TestFolderAccentStylesPropagateToFeedsAndArticles(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig())
	m.folders = []db.Folder{{ID: 10, Name: "Tech", Color: "#f7768e"}}
	m.feeds = []db.Feed{{ID: 1, Title: "Feed One", URL: "https://example.com/1", FolderID: 10, UnreadCount: 3}}
	m.sidebarRows = []sidebarRow{{kind: rowKindFolder, folderID: 10}, {kind: rowKindFeed, feedID: 1}}
	m.sidebarCursor = 1

	feedStyle := m.feedAccentStyle(m.feeds[0], false)
	if got := feedStyle.GetForeground(); got != lipgloss.Color("#f7768e") {
		t.Fatalf("expected feed accent foreground, got %q", got)
	}
	badgeStyle := m.feedBadgeStyle(m.feeds[0], false)
	if got := badgeStyle.GetForeground(); got != lipgloss.Color("#f7768e") {
		t.Fatalf("expected feed badge accent, got %q", got)
	}

	articleUnread, _, articleSelected, headerActive, _, borderFocus := m.articleRowStyles()
	if got := articleUnread.GetForeground(); got != lipgloss.Color("#f7768e") {
		t.Fatalf("expected article unread accent, got %q", got)
	}
	if got := articleSelected.GetForeground(); got != lipgloss.Color("#f7768e") {
		t.Fatalf("expected article selected accent, got %q", got)
	}
	if got := headerActive.GetBackground(); got != lipgloss.Color("#f7768e") {
		t.Fatalf("expected article header accent background, got %q", got)
	}
	if borderFocus != lipgloss.Color("#f7768e") {
		t.Fatalf("expected article border focus accent, got %q", borderFocus)
	}
}

func TestSidebarSelectedStyleUsesFilledBackground(t *testing.T) {
	m := Model{styles: BuildStyles(CatppuccinMocha), focused: paneFeeds}

	selected := m.sidebarSelectedStyle("")
	if got := selected.GetBackground(); got == CatppuccinMocha.Bg {
		t.Fatalf("expected selected feed background to differ from theme bg, got %q", got)
	}
	if got := selected.GetBackground(); got != m.styles.FeedItemSelectedFocused.GetBackground() {
		t.Fatalf("expected focused sidebar selection background, got %q", got)
	}
}

func TestSidebarSelectedStyleSoftensWhenFeedsPaneUnfocused(t *testing.T) {
	m := Model{styles: BuildStyles(CatppuccinMocha), focused: paneArticles}

	selected := m.sidebarSelectedStyle("")
	if got := selected.GetBackground(); got != m.styles.FeedItemSelectedUnfocused.GetBackground() {
		t.Fatalf("expected unfocused sidebar selection background, got %q", got)
	}
	if got := selected.GetBackground(); got == m.styles.FeedItemSelectedFocused.GetBackground() {
		t.Fatalf("expected unfocused selection to differ from focused selection background, got %q", got)
	}
}

func TestSidebarSelectedStyleUsesFolderAccentAsFocusedBackground(t *testing.T) {
	m := Model{styles: BuildStyles(CatppuccinMocha), focused: paneFeeds}

	selected := m.sidebarSelectedStyle(lipgloss.Color("#f7768e"))
	if got := selected.GetBackground(); got != lipgloss.Color("#f7768e") {
		t.Fatalf("expected focused folder accent background, got %q", got)
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func unixTestTime(ts int64) time.Time {
	return time.Unix(ts, 0)
}

func TestFormatArticleBodyWrapsParagraphsAndBullets(t *testing.T) {
	body := "This is a long paragraph that should wrap cleanly across multiple lines without turning into one unreadable run-on sentence.\n\n- first bullet point with a bit more detail\n- second bullet point\n\n> quoted line here"
	got := formatArticleBody(body, 24)

	if !containsString(got, "\n") {
		t.Fatalf("expected wrapped output, got %q", got)
	}
	if !containsString(got, "• first bullet point") {
		t.Fatalf("expected bullet formatting, got %q", got)
	}
	if !containsString(got, "│ quoted line here") {
		t.Fatalf("expected quote formatting, got %q", got)
	}
}
