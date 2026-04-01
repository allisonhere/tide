package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tide/internal/config"
	"tide/internal/db"
	"tide/internal/update"
)

func TestFeedsLoadedPopulatesPane(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")

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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")

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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")

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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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

func TestStatusBarShowsUpdateCheckActivity(t *testing.T) {
	m := Model{
		width:       80,
		styles:      BuildStyles(CatppuccinMocha),
		updateState: updateStateChecking,
		spinner:     spinner.New(),
	}

	bar := m.renderStatusBar()
	if !containsString(bar, "checking updates") {
		t.Fatalf("expected status bar to show update check activity, got %q", bar)
	}
}

func TestStatusBarKeepsUpdateAvailableVisibleWithLongFeedTitle(t *testing.T) {
	m := Model{
		width:       48,
		styles:      BuildStyles(CatppuccinMocha),
		updateState: updateStateAvailable,
		updateInfo:  update.ReleaseInfo{Version: "v1.1.0"},
		feeds: []db.Feed{
			{ID: 1, Title: "A Very Long Feed Title That Would Normally Push Status Details Off Screen"},
		},
		sidebarRows:   []sidebarRow{{kind: rowKindFeed, feedID: 1}},
		sidebarCursor: 0,
	}

	bar := m.renderStatusBar()
	if !containsString(bar, "v1.1.0") {
		t.Fatalf("expected status bar to keep update version visible, got %q", bar)
	}
	if !containsString(bar, "U update") || !containsString(bar, "i ignore") {
		t.Fatalf("expected status bar to keep update actions visible, got %q", bar)
	}
	if !strings.HasSuffix(strings.TrimRight(ansi.Strip(bar), " "), "v1.1.0  U update  i ignore") {
		t.Fatalf("expected update prompt to be right-aligned at the end of the bar, got %q", ansi.Strip(bar))
	}
}

func TestStatusMessageStillIncludesAvailableUpdateHint(t *testing.T) {
	m := Model{
		width:       80,
		styles:      BuildStyles(CatppuccinMocha),
		updateState: updateStateAvailable,
		updateInfo:  update.ReleaseInfo{Version: "v1.1.0"},
		statusMsg:   "saved settings",
	}

	bar := m.renderStatusBar()
	if !containsString(bar, "U update") || !containsString(bar, "i ignore") {
		t.Fatalf("expected transient status bar to retain update actions, got %q", bar)
	}
	if !containsString(bar, "saved settings") {
		t.Fatalf("expected transient status message to remain visible, got %q", bar)
	}
}

func TestUppercaseUOpensAvailableUpdateConfirm(t *testing.T) {
	m := Model{
		updateState: updateStateAvailable,
		updateInfo:  update.ReleaseInfo{Version: "v1.1.0"},
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	got := next.(Model)

	if got.overlay != overlayUpdateConfirm {
		t.Fatalf("expected U to open update confirm, got overlay %v", got.overlay)
	}
}

func TestUpdateConfirmEscReturnsToApp(t *testing.T) {
	m := Model{
		overlay: overlayUpdateConfirm,
		keys:    DefaultKeys,
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := next.(Model)

	if got.overlay != overlayNone {
		t.Fatalf("expected esc from update confirm to return to app, got overlay %v", got.overlay)
	}
}

func TestUpdateConfirmEnterStartsDownloadAndReturnsToApp(t *testing.T) {
	m := Model{
		overlay:    overlayUpdateConfirm,
		keys:       DefaultKeys,
		updateInfo: update.ReleaseInfo{Version: "v1.1.0"},
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)

	if got.overlay != overlayNone {
		t.Fatalf("expected enter from update confirm to return to app, got overlay %v", got.overlay)
	}
	if got.updateState != updateStateDownloading {
		t.Fatalf("expected enter from update confirm to start downloading, got state %v", got.updateState)
	}
	if cmd == nil {
		t.Fatal("expected enter from update confirm to start a download command")
	}
}

func TestUpdateConfirmOverlayMentionsSettingsAvailability(t *testing.T) {
	m := Model{
		updateInfo: update.ReleaseInfo{
			Version:   "v1.1.0",
			AssetName: "tide-darwin-aarch64",
		},
	}

	view := m.renderUpdateConfirmOverlay(72, newManagerChrome(72, CatppuccinMocha))
	if !containsString(view, "Settings > Updates") {
		t.Fatalf("expected update confirm overlay to mention settings availability, got %q", view)
	}
}

func TestLowercaseUIgnoredForAppUpdateAndAppliesFeedURLUpdate(t *testing.T) {
	m := Model{
		pendingURLUpdate: &pendingURLUpdate{feedID: 7, newURL: "https://example.com/new.xml"},
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	got := next.(Model)

	if got.pendingURLUpdate != nil {
		t.Fatal("expected lowercase u to consume pending feed URL update")
	}
	if cmd == nil {
		t.Fatal("expected lowercase u to return a feed URL update command")
	}
}

func TestLowercaseIDismissesAvailableUpdate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := Model{
		cfg:         config.DefaultConfig(),
		updateState: updateStateAvailable,
		updateInfo:  update.ReleaseInfo{Version: "v1.1.0"},
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	got := next.(Model)

	if !got.updateDismissed {
		t.Fatal("expected i to dismiss the available update")
	}
	if got.cfg.Updates.DismissedVersion != "v1.1.0" {
		t.Fatalf("expected dismissed version to be persisted, got %q", got.cfg.Updates.DismissedVersion)
	}
	if got.statusMsg == "" {
		t.Fatal("expected dismiss action to set a status message")
	}
}

func TestFeedPaneDoesNotStartWithBlankLines(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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
	m := NewModel(database, cfg, "v1.0.0")
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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssFeeds)

	view := s.View(100, 30, newManagerChrome(100, CatppuccinMocha))

	if !containsString(view, "Feed max size (MiB)") {
		t.Fatalf("expected feed max size field in settings view: %q", view)
	}
}

func TestSettingsAboutActionReturnsBrowserCommand(t *testing.T) {
	m := Model{
		cfg:      config.DefaultConfig(),
		keys:     DefaultKeys,
		settings: newSettings(config.DefaultConfig(), settingsUpdateState{}),
	}
	m.settings.setActiveSection(ssAbout)
	m.settings.setFocusedField(sfAboutRepo)

	next, cmd := m.handleSettings(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)

	if cmd == nil {
		t.Fatal("expected ABOUT repository action to return a browser command")
	}
	if got.settings.action != settingsActionNone {
		t.Fatalf("expected ABOUT repository action to be consumed, got %v", got.settings.action)
	}
}

func TestRightKeyReachesContentPane(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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

func TestArticleReadUpdatedAdvancesToNextArticleInArticlesPane(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	feed := db.Feed{ID: 1, Title: "Feed One", URL: "https://example.com/feed"}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: []db.Feed{feed}})
	m = m2.(Model)
	m.sidebarCursor = 1

	articles := []db.Article{
		{ID: 1, FeedID: 1, Title: "Article One", Link: "https://example.com/a", Content: "one", PublishedAt: unixTestTime(1710000100), Read: false},
		{ID: 2, FeedID: 1, Title: "Article Two", Link: "https://example.com/b", Content: "two", PublishedAt: unixTestTime(1710000000), Read: false},
	}
	m2, _ = m.Update(ArticlesLoadedMsg{FeedID: 1, Articles: articles})
	m = m2.(Model)
	m.focused = paneArticles

	m2, cmd := m.Update(ArticleReadUpdatedMsg{ArticleID: 1, Read: true, Advance: true})
	m = m2.(Model)

	if !m.articles[0].Read {
		t.Fatal("expected first article to be marked read")
	}
	if m.articleCursor != 1 {
		t.Fatalf("expected article cursor to advance to next article, got %d", m.articleCursor)
	}
	if m.filteredArticles[m.articleCursor].ID != 2 {
		t.Fatalf("expected second article to become selected, got article %d", m.filteredArticles[m.articleCursor].ID)
	}
	if cmd == nil {
		t.Fatal("expected mark-read update to trigger follow-up commands")
	}
}

func TestArticleReadUpdatedDoesNotAdvanceOutsideArticlesPane(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m2.(Model)

	feed := db.Feed{ID: 1, Title: "Feed One", URL: "https://example.com/feed"}
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: []db.Feed{feed}})
	m = m2.(Model)
	m.sidebarCursor = 1

	articles := []db.Article{
		{ID: 1, FeedID: 1, Title: "Article One", Link: "https://example.com/a", Content: "one", PublishedAt: unixTestTime(1710000100), Read: false},
		{ID: 2, FeedID: 1, Title: "Article Two", Link: "https://example.com/b", Content: "two", PublishedAt: unixTestTime(1710000000), Read: false},
	}
	m2, _ = m.Update(ArticlesLoadedMsg{FeedID: 1, Articles: articles})
	m = m2.(Model)
	m.focused = paneContent

	m2, _ = m.Update(ArticleReadUpdatedMsg{ArticleID: 1, Read: true, Advance: false})
	m = m2.(Model)

	if !m.articles[0].Read {
		t.Fatal("expected first article to be marked read")
	}
	if m.articleCursor != 0 {
		t.Fatalf("expected article cursor to stay put outside articles pane, got %d", m.articleCursor)
	}
	if m.filteredArticles[m.articleCursor].ID != 1 {
		t.Fatalf("expected first article to remain selected, got article %d", m.filteredArticles[m.articleCursor].ID)
	}
}

func TestContentPaneClampsViewportOutputToPaneSize(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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

func TestRenderArticleContentUsesOneCharacterLeftMargin(t *testing.T) {
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

	lines := strings.Split(ansi.Strip(got), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			t.Fatalf("expected article content line %d to start with a one-character left margin, got %q", i+1, line)
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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
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

func TestUpdateCheckedMsgMarksAvailableRelease(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
	m.settings = newSettings(m.cfg, m.settingsUpdateState())

	next, _ := m.Update(UpdateCheckedMsg{
		Result: update.CheckResult{
			CurrentVersion: "v1.0.0",
			Latest: update.ReleaseInfo{
				Version:     "v1.1.0",
				Summary:     "Faster update flow.",
				PublishedAt: unixTestTime(1710000000),
			},
			Available: true,
		},
		Manual: true,
	})
	m = next.(Model)

	if m.updateState != updateStateAvailable {
		t.Fatalf("expected updateStateAvailable, got %v", m.updateState)
	}
	if m.updateInfo.Version != "v1.1.0" {
		t.Fatalf("expected latest version v1.1.0, got %q", m.updateInfo.Version)
	}
	if !m.updateInfoFresh {
		t.Fatal("expected fresh update check to mark release info as fresh")
	}
	if m.settings.update.latestVersion != "v1.1.0" {
		t.Fatalf("expected settings update state to sync latest version, got %q", m.settings.update.latestVersion)
	}
	if !m.settings.update.latestIsFresh {
		t.Fatal("expected settings update state to mark latest version as fresh")
	}
	if m.cfg.Updates.AvailableVersion != "v1.1.0" {
		t.Fatalf("expected available update to persist in config, got %q", m.cfg.Updates.AvailableVersion)
	}
}

func TestNewModelRestoresCachedAvailableUpdate(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	cfg := config.DefaultConfig()
	cfg.Updates.AvailableVersion = "v1.1.0"
	cfg.Updates.AvailableSummary = "Faster update flow."
	cfg.Updates.AvailablePublished = 1710000000

	m := NewModel(database, cfg, "v1.0.0")

	if m.updateState != updateStateAvailable {
		t.Fatalf("expected cached update to restore available state, got %v", m.updateState)
	}
	if m.updateInfo.Version != "v1.1.0" {
		t.Fatalf("expected cached update version v1.1.0, got %q", m.updateInfo.Version)
	}
	if m.updateInfoFresh {
		t.Fatal("expected restored cached update to remain marked as stale")
	}
}

func TestSettingsViewRendersUpdateActions(t *testing.T) {
	cfg := config.DefaultConfig()
	s := newSettings(cfg, settingsUpdateState{
		currentVersion: "v1.0.0",
		state:          updateStateAvailable,
		latestVersion:  "v1.1.0",
		summary:        "Faster update flow.",
	})
	s.setActiveSection(ssUpdates)
	chrome := newManagerChrome(62, CatppuccinMocha)
	view := s.View(62, 40, chrome)

	if !containsString(view, "CATEGORIES") {
		t.Fatalf("expected categories pane in settings view: %q", view)
	}
	if !containsString(view, "Current version") {
		t.Fatalf("expected current version row in settings view: %q", view)
	}
	if !containsString(view, "Update now") {
		t.Fatalf("expected update action in settings view: %q", view)
	}
}

func TestSettingsViewShowsLatestVersionLabelForRestoredRelease(t *testing.T) {
	cfg := config.DefaultConfig()
	s := newSettings(cfg, settingsUpdateState{
		currentVersion: "v1.0.0",
		state:          updateStateAvailable,
		latestVersion:  "v1.1.0",
		latestIsFresh:  false,
		summary:        "Faster update flow.",
	})
	s.setActiveSection(ssUpdates)
	chrome := newManagerChrome(62, CatppuccinMocha)
	view := s.View(62, 40, chrome)

	if !containsString(view, "Latest version") {
		t.Fatalf("expected latest version label in settings view: %q", view)
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
