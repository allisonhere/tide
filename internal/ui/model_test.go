package ui

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tide/internal/config"
	"tide/internal/db"
	"tide/internal/feed"
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

func TestEnterOpensAddDialogWhenNoFeedsExist(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 96, Height: 24})
	m = m2.(Model)
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: nil})
	m = m2.(Model)

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.overlay != overlayFeedManager {
		t.Fatalf("expected enter on empty main screen to open feed manager overlay, got %v", m.overlay)
	}
	if m.feedManager.mode != fmEdit {
		t.Fatalf("expected enter on empty main screen to open add dialog, got mode %v", m.feedManager.mode)
	}
	if m.feedManager.listPaneFocused() {
		t.Fatal("expected enter on empty main screen to focus the add dialog form")
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
	if !containsString(bar, "checking Tide updates") {
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
	if !containsString(bar, "App update available") || !containsString(bar, "U install") || !containsString(bar, "i ignore") {
		t.Fatalf("expected status bar to keep update actions visible, got %q", bar)
	}
	if !strings.HasSuffix(strings.TrimRight(ansi.Strip(bar), " "), "App update available  U install  i ignore") {
		t.Fatalf("expected full update prompt to be right-aligned at the end of the bar, got %q", ansi.Strip(bar))
	}
}

func TestStatusMessageStillIncludesAvailableUpdateHint(t *testing.T) {
	m := Model{
		width:       80,
		styles:      BuildStyles(CatppuccinMocha),
		updateState: updateStateAvailable,
		updateInfo:  update.ReleaseInfo{Version: "v1.1.0", Summary: "Faster update flow."},
		statusMsg:   "saved settings",
	}

	bar := m.renderStatusBar()
	if !containsString(bar, "App update available") || !containsString(bar, "U install") || !containsString(bar, "i ignore") {
		t.Fatalf("expected transient status bar to retain update actions, got %q", bar)
	}
	if !containsString(bar, "saved settings") {
		t.Fatalf("expected transient status message to remain visible, got %q", bar)
	}
}

func TestStatusBarShowsUpdateSummaryWhenAvailable(t *testing.T) {
	m := Model{
		width:       96,
		styles:      BuildStyles(CatppuccinMocha),
		updateState: updateStateAvailable,
		updateInfo:  update.ReleaseInfo{Version: "v1.1.0", Summary: "Faster update flow."},
	}

	bar := m.renderStatusBar()
	if !containsString(bar, "App update available") || !containsString(bar, "U install") || !containsString(bar, "i ignore") {
		t.Fatalf("expected combined app update prompt in status bar, got %q", bar)
	}
	if containsString(ansi.Strip(bar), "Faster update flow.") {
		t.Fatalf("expected status bar to keep update summary out of the main prompt, got %q", ansi.Strip(bar))
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
			Summary:   "Faster update flow.",
		},
	}

	view := m.renderUpdateConfirmOverlay(72, newManagerChrome(72, CatppuccinMocha))
	if !containsString(view, "INSTALL TIDE UPDATE?") {
		t.Fatalf("expected Tide-specific update header, got %q", view)
	}
	if !containsString(view, "What's new: Faster update flow.") {
		t.Fatalf("expected update confirm overlay to include release summary, got %q", view)
	}
	if !containsString(view, "Settings > Updates") {
		t.Fatalf("expected update confirm overlay to mention settings availability, got %q", view)
	}
}

func TestLowercaseURefreshesAllFeeds(t *testing.T) {
	m := Model{
		keys:       DefaultKeys,
		refreshing: map[int64]bool{},
		feeds: []db.Feed{
			{ID: 7, URL: "https://example.com/feed.xml"},
		},
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	_ = next.(Model)
	if cmd == nil {
		t.Fatal("expected lowercase u to return a refresh-all command")
	}
}

func TestFeedRefreshAutoAppliesPermanentRedirectURL(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	feedID, err := database.AddFeed("https://example.com/old.xml", "Redirected Feed", "")
	if err != nil {
		t.Fatalf("AddFeed returned error: %v", err)
	}

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")

	next, cmd := m.Update(FeedRefreshedMsg{
		FeedID: feedID,
		Result: &feed.FetchResult{
			Kind:             feed.KindSuccess,
			SuggestURLUpdate: true,
			SuggestedURL:     "https://example.com/new.xml",
		},
	})
	m = next.(Model)

	if cmd == nil {
		t.Fatal("expected refresh handling to return follow-up commands")
	}
	gotFeed, err := database.GetFeed(feedID)
	if err != nil {
		t.Fatalf("GetFeed returned error: %v", err)
	}
	if gotFeed.URL != "https://example.com/new.xml" {
		t.Fatalf("expected feed URL to auto-update, got %q", gotFeed.URL)
	}
	if !strings.Contains(m.statusMsg, "feed URL updated to https://example.com/new.xml") {
		t.Fatalf("expected status message about auto-updated URL, got %q", m.statusMsg)
	}
}

func TestFetchErrorOverlayOmitsURLUpdateAction(t *testing.T) {
	m := Model{
		lastFetchError: &feed.FetchResult{
			Kind:             feed.KindHttpError,
			Err:              io.EOF,
			OriginalURL:      "https://example.com/old.xml",
			FinalURL:         "https://example.com/new.xml",
			RedirectChain:    []string{"https://example.com/old.xml", "https://example.com/new.xml"},
			SuggestURLUpdate: true,
			SuggestedURL:     "https://example.com/new.xml",
		},
	}

	view := m.renderFetchErrorOverlay(72, newManagerChrome(72, CatppuccinMocha))
	if !containsString(view, "Feed permanently moved to new URL") {
		t.Fatalf("expected redirect note in fetch error overlay, got %q", view)
	}
	if containsString(view, "update URL") {
		t.Fatalf("expected fetch error overlay to omit URL update action, got %q", view)
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
	if !strings.Contains(got.statusMsg, "Tide update v1.1.0 dismissed") {
		t.Fatalf("expected Tide-specific dismiss status, got %q", got.statusMsg)
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

func TestLoadFeedsCmdCombinesLocalAndGReaderFeeds(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	if _, err := database.AddFeed("https://local.example.com/feed.xml", "Local Feed", ""); err != nil {
		t.Fatalf("AddFeed returned error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Source.GReaderURL = "https://rss.example.com/api/greader.php"
	cfg.Source.GReaderLogin = "alice"
	cfg.Source.GReaderPassword = "secret"

	m := NewModel(database, cfg, "v1.0.0")
	m.greaderClient.HTTPClient = &http.Client{Transport: uiRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://rss.example.com/api/greader.php/accounts/ClientLogin":
			return uiResponseWithBody(http.StatusOK, "Auth=test-token\n"), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/subscription/list?output=json":
			return uiResponseWithJSON(http.StatusOK, `{
				"subscriptions": [
					{"id":"feed/http://example.com/feed.xml","title":"Remote Feed","url":"http://example.com/feed.xml","htmlUrl":"http://example.com/"}
				]
			}`), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/unread-count?output=json":
			return uiResponseWithJSON(http.StatusOK, `{"unreadcounts":[{"id":"feed/http://example.com/feed.xml","count":4}]}`), nil
		default:
			t.Fatalf("unexpected request %s", req.URL.String())
			return nil, nil
		}
	})}
	msg := m.loadFeedsCmd()()
	m2, _ := m.Update(msg)
	m = m2.(Model)

	if len(m.feeds) != 2 {
		t.Fatalf("expected local + remote feeds, got %d", len(m.feeds))
	}
	var foundLocal, foundRemote bool
	for _, feed := range m.feeds {
		switch feed.Title {
		case "Local Feed":
			foundLocal = true
		case "Remote Feed":
			foundRemote = true
			if feed.UnreadCount != 4 {
				t.Fatalf("expected unread count 4, got %d", feed.UnreadCount)
			}
		}
	}
	if !foundLocal || !foundRemote {
		t.Fatalf("expected both local and remote feeds, got %#v", m.feeds)
	}
	if len(m.greaderStreams) != 1 {
		t.Fatalf("expected remote stream map to be populated, got %d entries", len(m.greaderStreams))
	}
}

func TestLoadArticlesCmdUsesGReaderFeedWhenSelected(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	cfg := config.DefaultConfig()
	cfg.Source.GReaderURL = "https://rss.example.com/api/greader.php"
	cfg.Source.GReaderLogin = "alice"
	cfg.Source.GReaderPassword = "secret"

	m := NewModel(database, cfg, "v1.0.0")
	m.greaderClient.HTTPClient = &http.Client{Transport: uiRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://rss.example.com/api/greader.php/accounts/ClientLogin":
			return uiResponseWithBody(http.StatusOK, "Auth=test-token\n"), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/subscription/list?output=json":
			return uiResponseWithJSON(http.StatusOK, `{
				"subscriptions": [
					{"id":"feed/http://example.com/feed.xml","title":"Remote Feed","url":"http://example.com/feed.xml","htmlUrl":"http://example.com/"}
				]
			}`), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/unread-count?output=json":
			return uiResponseWithJSON(http.StatusOK, `{"unreadcounts":[{"id":"feed/http://example.com/feed.xml","count":1}]}`), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/stream/contents/feed%2Fhttp:%2F%2Fexample.com%2Ffeed.xml?n=100&output=json":
			return uiResponseWithJSON(http.StatusOK, `{
				"items": [
					{
						"id":"tag:google.com,2005:reader/item/abc123",
						"title":"Remote Article",
						"published":1710000000,
						"alternate":[{"href":"https://example.com/articles/1"}],
						"summary":{"content":"<p>Hello remote world</p>"},
						"origin":{"streamId":"feed/http://example.com/feed.xml"}
					}
				]
			}`), nil
		default:
			t.Fatalf("unexpected request %s", req.URL.String())
			return nil, nil
		}
	})}
	msg := m.loadFeedsCmd()()
	m2, _ := m.Update(msg)
	m = m2.(Model)

	if len(m.feeds) != 1 {
		t.Fatalf("expected 1 remote feed, got %d", len(m.feeds))
	}

	msg = m.loadArticlesCmd(m.feeds[0].ID)()
	m2, _ = m.Update(msg)
	m = m2.(Model)

	if len(m.articles) != 1 {
		t.Fatalf("expected 1 remote article, got %d", len(m.articles))
	}
	if m.articles[0].Link != "https://example.com/articles/1" {
		t.Fatalf("unexpected remote article link %q", m.articles[0].Link)
	}
	if !strings.Contains(m.articles[0].Content, "Hello remote world") {
		t.Fatalf("expected remote content to be normalized into article body, got %q", m.articles[0].Content)
	}
}

func TestFeedManagerKeyStillOpensEditableOverlayWithRemoteFeedsLoaded(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	cfg := config.DefaultConfig()
	cfg.Source.GReaderURL = "https://rss.example.com/api/greader.php"
	cfg.Source.GReaderLogin = "alice"
	cfg.Source.GReaderPassword = "secret"

	m := NewModel(database, cfg, "v1.0.0")
	m2, _ := m.Update(FeedsLoadedMsg{
		Feeds: []db.Feed{{ID: -1, Title: "Remote Feed", URL: "https://example.com/feed"}},
		RemoteStreams: map[int64]string{
			-1: "feed/http://example.com/feed",
		},
	})
	m = m2.(Model)

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = m2.(Model)

	if m.overlay != overlayFeedManager {
		t.Fatalf("expected feed manager overlay, got %v", m.overlay)
	}
	if !m.feedManager.editable() {
		t.Fatal("expected feed manager to stay editable for the local add/manage flow")
	}
	if got := len(m.feedManager.feeds); got != 1 {
		t.Fatalf("expected remote feed to be present in manager data, got %d feeds", got)
	}
	if selected := m.feedManager.selectedFeedRow(); selected == nil || selected.ID != -1 {
		t.Fatalf("expected remote feed to be selected in manager, got %#v", selected)
	}
}

func TestAddKeyOpensAddDialogWithSourceToggle(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	cfg := config.DefaultConfig()
	cfg.Source.GReaderURL = "https://rss.example.com/api/greader.php"
	cfg.Source.GReaderLogin = "alice"
	cfg.Source.GReaderPassword = "secret"

	m := NewModel(database, cfg, "v1.0.0")
	m2, _ := m.Update(FeedsLoadedMsg{})
	m = m2.(Model)

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = m2.(Model)

	if m.overlay != overlayFeedManager {
		t.Fatalf("expected add key to open feed manager overlay, got %v", m.overlay)
	}
	if m.feedManager.mode != fmEdit {
		t.Fatalf("expected add key to enter add dialog, got mode %v", m.feedManager.mode)
	}
	if m.feedManager.listPaneFocused() {
		t.Fatal("expected add dialog to start focused on the right pane")
	}
	if m.feedManager.focusedField != fmFieldAddSource {
		t.Fatalf("expected add dialog to focus source toggle, got field %d", m.feedManager.focusedField)
	}
}

func TestFeedManagerOverlayShowsAddActionAndAOpensAddDialog(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	cfg := config.DefaultConfig()
	cfg.Source.GReaderURL = "https://rss.example.com/api/greader.php"
	cfg.Source.GReaderLogin = "alice"
	cfg.Source.GReaderPassword = "secret"

	m := NewModel(database, cfg, "v1.0.0")
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 96, Height: 24})
	m = m2.(Model)
	m2, _ = m.Update(FeedsLoadedMsg{
		Feeds: []db.Feed{{ID: -1, Title: "Remote Feed", URL: "https://example.com/feed"}},
		RemoteStreams: map[int64]string{
			-1: "feed/http://example.com/feed",
		},
	})
	m = m2.(Model)

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = m2.(Model)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "ADD FEED") {
		t.Fatalf("expected manager overlay footer to advertise add feed, got %q", view)
	}
	if strings.Contains(view, "BROWSE-ONLY") {
		t.Fatalf("expected editable manager overlay not to render browse-only footer, got %q", view)
	}

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = m2.(Model)

	if m.overlay != overlayFeedManager {
		t.Fatalf("expected manager overlay to remain open after a, got %v", m.overlay)
	}
	if m.feedManager.mode != fmEdit {
		t.Fatalf("expected a in manager list view to open add dialog, got mode %v", m.feedManager.mode)
	}
	if m.feedManager.listPaneFocused() {
		t.Fatal("expected add dialog from manager list view to start focused on the right pane")
	}
}

func TestFeedManagerOpensInListModeWithLeftPaneFocus(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	feedID, err := database.AddFeed("https://example.com/feed.xml", "Example Feed", "")
	if err != nil {
		t.Fatalf("save feed: %v", err)
	}
	feed := db.Feed{ID: feedID, Title: "Example Feed", URL: "https://example.com/feed.xml"}

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 96, Height: 24})
	m = m2.(Model)
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: []db.Feed{feed}})
	m = m2.(Model)

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = m2.(Model)

	if m.overlay != overlayFeedManager {
		t.Fatalf("expected manager overlay after m, got %v", m.overlay)
	}
	if m.feedManager.mode != fmList {
		t.Fatalf("expected manager to open in list mode, got %v", m.feedManager.mode)
	}
	if !m.feedManager.listPaneFocused() {
		t.Fatal("expected manager to open focused on the left pane")
	}
}

func TestRemoteFeedAddedMsgPersistsGReaderConfigAndTargetsStream(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
	m2, _ := m.Update(RemoteFeedAddedMsg{
		Source: config.SourceConfig{
			GReaderURL:      "https://rss.example.com/api/greader.php",
			GReaderLogin:    "alice",
			GReaderPassword: "secret",
		},
		StreamID: "feed/http://example.com/feed",
		Title:    "Remote Feed",
	})
	m = m2.(Model)

	if m.cfg.Source.GReaderURL != "https://rss.example.com/api/greader.php" {
		t.Fatalf("expected greader URL to be stored, got %q", m.cfg.Source.GReaderURL)
	}
	if m.cfg.Source.GReaderLogin != "alice" {
		t.Fatalf("expected greader login to be stored, got %q", m.cfg.Source.GReaderLogin)
	}
	if m.cfg.Source.GReaderPassword != "secret" {
		t.Fatalf("expected greader password to be stored, got %q", m.cfg.Source.GReaderPassword)
	}
	if m.greaderClient == nil {
		t.Fatal("expected greader client to be initialized after remote add")
	}
	if m.pendingSelectFeedID != remoteStableID("feed", "feed/http://example.com/feed") {
		t.Fatalf("expected remote add flow to target the added feed, got %d", m.pendingSelectFeedID)
	}

	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load config from disk: %v", err)
	}
	if saved.Source.GReaderURL != "https://rss.example.com/api/greader.php" {
		t.Fatalf("expected greader URL to persist to disk, got %q", saved.Source.GReaderURL)
	}
	if saved.Source.GReaderLogin != "alice" {
		t.Fatalf("expected greader login to persist to disk, got %q", saved.Source.GReaderLogin)
	}
	if saved.Source.GReaderPassword != "secret" {
		t.Fatalf("expected greader password to persist to disk, got %q", saved.Source.GReaderPassword)
	}
}

func TestRemoteFeedAddedMsgWithoutStreamShowsConnectedStatus(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
	m2, _ := m.Update(RemoteFeedAddedMsg{
		Source: config.SourceConfig{
			GReaderURL:      "https://rss.example.com/api/greader.php",
			GReaderLogin:    "alice",
			GReaderPassword: "secret",
		},
		FeedCount: 7,
	})
	m = m2.(Model)

	if m.pendingSelectFeedID != 0 {
		t.Fatalf("expected connect-only greader load not to target a specific feed, got %d", m.pendingSelectFeedID)
	}
	if m.statusMsg != "connected greader: 7 feeds" {
		t.Fatalf("expected connected status, got %q", m.statusMsg)
	}
}

func TestRemoteFeedAddedMsgSettingsOnlyShowsSavedStatus(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
	m2, _ := m.Update(RemoteFeedAddedMsg{
		SettingsOnly: true,
		Source: config.SourceConfig{
			GReaderURL:      "https://rss.example.com/api/greader.php",
			GReaderLogin:    "alice",
			GReaderPassword: "secret",
		},
	})
	m = m2.(Model)

	if m.statusMsg != "saved greader settings" {
		t.Fatalf("expected saved settings status, got %q", m.statusMsg)
	}
	if m.pendingSelectFeedID != 0 {
		t.Fatalf("expected settings-only save not to target a feed, got %d", m.pendingSelectFeedID)
	}
}

func TestRemoteFeedAddedMsgSaveFailureSurfacesStatus(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	blocked := t.TempDir() + "/config-blocker"
	if err := os.WriteFile(blocked, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocked)

	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0")
	m2, cmd := m.Update(RemoteFeedAddedMsg{
		Source: config.SourceConfig{
			GReaderURL:      "https://rss.example.com/api/greader.php",
			GReaderLogin:    "alice",
			GReaderPassword: "secret",
		},
		FeedCount: 7,
	})
	m = m2.(Model)

	if !strings.Contains(m.statusMsg, "greader config save failed") {
		t.Fatalf("expected save failure status, got %q", m.statusMsg)
	}
	if m.greaderClient == nil {
		t.Fatal("expected greader client to remain initialized in-memory after save failure")
	}
	if cmd == nil {
		t.Fatal("expected save failure path to return follow-up commands")
	}
}

func TestFeedManagerGReaderSaveCmdQuickAddsRemoteFeed(t *testing.T) {
	loginHit := false
	quickAddHit := false

	origTransport := http.DefaultTransport
	http.DefaultTransport = uiRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/greader.php/accounts/ClientLogin":
			loginHit = true
			return uiResponseWithBody(http.StatusOK, "SID=ignored\nAuth=test-token\n"), nil
		case "/api/greader.php/reader/api/0/subscription/quickadd":
			quickAddHit = true
			if got := req.Header.Get("Authorization"); got != "GoogleLogin auth=test-token" {
				t.Fatalf("expected auth header on quickadd request, got %q", got)
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read quickadd body: %v", err)
			}
			if got := string(body); got != "quickadd=https%3A%2F%2Fexample.com%2Ffeed.xml" {
				t.Fatalf("unexpected quickadd body %q", got)
			}
			return uiResponseWithJSON(http.StatusOK, `{"numResults":1,"query":"https://example.com/feed.xml","streamId":"feed/http://example.com/feed.xml","streamName":"Example Feed"}`), nil
		default:
			t.Fatalf("unexpected request path %s", req.URL.Path)
			return nil, nil
		}
	})
	defer func() { http.DefaultTransport = origTransport }()

	fm := NewFeedManagerWithSource(nil, config.SourceConfig{})
	fm.focusAdd()
	fm.addSourceIdx = fmAddSourceGReader
	fm.titleInput.SetValue("Custom Title")
	fm.urlInput.SetValue("https://example.com/feed.xml")
	fm.greaderURLInput.SetValue("https://rss.example.com/api/greader.php")
	fm.greaderLoginInput.SetValue("alice")
	fm.greaderPasswordInput.SetValue("secret")

	msg := fm.saveCmd()()
	got, ok := msg.(RemoteFeedAddedMsg)
	if !ok {
		t.Fatalf("expected RemoteFeedAddedMsg, got %T", msg)
	}
	if got.Err != nil {
		t.Fatalf("expected successful remote add, got error %v", got.Err)
	}
	if !loginHit || !quickAddHit {
		t.Fatalf("expected login and quickadd requests, login=%v quickadd=%v", loginHit, quickAddHit)
	}
	if got.StreamID != "feed/http://example.com/feed.xml" {
		t.Fatalf("expected stream id from quickadd, got %q", got.StreamID)
	}
	if got.Title != "Example Feed" {
		t.Fatalf("expected remote title from quickadd, got %q", got.Title)
	}
	if got.Source.GReaderURL != "https://rss.example.com/api/greader.php" || got.Source.GReaderLogin != "alice" || got.Source.GReaderPassword != "secret" {
		t.Fatalf("unexpected persisted source config: %#v", got.Source)
	}
}

func TestFeedManagerGReaderSaveCmdAllowsBlankFeedURL(t *testing.T) {
	loginHit := false
	listHit := false
	quickAddHit := false

	origTransport := http.DefaultTransport
	http.DefaultTransport = uiRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/greader.php/accounts/ClientLogin":
			loginHit = true
			return uiResponseWithBody(http.StatusOK, "SID=ignored\nAuth=test-token\n"), nil
		case "/api/greader.php/reader/api/0/subscription/list":
			listHit = true
			if req.URL.RawQuery != "output=json" {
				t.Fatalf("unexpected list query %q", req.URL.RawQuery)
			}
			if got := req.Header.Get("Authorization"); got != "GoogleLogin auth=test-token" {
				t.Fatalf("expected auth header on list request, got %q", got)
			}
			return uiResponseWithJSON(http.StatusOK, `{"subscriptions":[{"id":"feed/http://example.com/feed.xml","title":"Example Feed"}]}`), nil
		case "/api/greader.php/reader/api/0/subscription/quickadd":
			quickAddHit = true
			t.Fatal("did not expect quickadd when feed URL is blank")
			return nil, nil
		default:
			t.Fatalf("unexpected request path %s", req.URL.Path)
			return nil, nil
		}
	})
	defer func() { http.DefaultTransport = origTransport }()

	fm := NewFeedManagerWithSource(nil, config.SourceConfig{})
	fm.focusAdd()
	fm.addSourceIdx = fmAddSourceGReader
	fm.greaderURLInput.SetValue("https://rss.example.com/api/greader.php")
	fm.greaderLoginInput.SetValue("alice")
	fm.greaderPasswordInput.SetValue("secret")

	msg := fm.saveCmd()()
	got, ok := msg.(RemoteFeedAddedMsg)
	if !ok {
		t.Fatalf("expected RemoteFeedAddedMsg, got %T", msg)
	}
	if got.Err != nil {
		t.Fatalf("expected successful greader connect, got error %v", got.Err)
	}
	if !loginHit || !listHit || quickAddHit {
		t.Fatalf("expected login and subscription list only, login=%v list=%v quickadd=%v", loginHit, listHit, quickAddHit)
	}
	if got.StreamID != "" {
		t.Fatalf("expected no target stream for blank feed URL, got %q", got.StreamID)
	}
	if got.FeedCount != 1 {
		t.Fatalf("expected feed count from subscription list, got %d", got.FeedCount)
	}
}

func TestFeedManagerGReaderSettingsEditSaveCmdDoesNotHitNetwork(t *testing.T) {
	loginHit := false
	listHit := false
	quickAddHit := false

	origTransport := http.DefaultTransport
	http.DefaultTransport = uiRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/greader.php/accounts/ClientLogin":
			loginHit = true
		case "/api/greader.php/reader/api/0/subscription/list":
			listHit = true
		case "/api/greader.php/reader/api/0/subscription/quickadd":
			quickAddHit = true
		}
		t.Fatalf("unexpected network request in settings-only save: %s", req.URL.Path)
		return nil, nil
	})
	defer func() { http.DefaultTransport = origTransport }()

	fm := NewFeedManagerWithSource(nil, config.SourceConfig{
		GReaderURL:      "https://rss.example.com/api/greader.php",
		GReaderLogin:    "alice",
		GReaderPassword: "secret",
	})
	fm.focusRemoteSettingsEdit(db.Feed{
		ID:    -1,
		Title: "Remote Feed",
		URL:   "https://example.com/feed.xml",
	})
	fm.greaderURLInput.SetValue("https://rss.example.com/api/greader.php")
	fm.greaderLoginInput.SetValue("alice")
	fm.greaderPasswordInput.SetValue("secret")

	msg := fm.saveCmd()()
	got, ok := msg.(RemoteFeedAddedMsg)
	if !ok {
		t.Fatalf("expected RemoteFeedAddedMsg, got %T", msg)
	}
	if got.Err != nil {
		t.Fatalf("expected successful settings save, got error %v", got.Err)
	}
	if !got.SettingsOnly {
		t.Fatal("expected settings-only result")
	}
	if got.StreamID != "" {
		t.Fatalf("expected settings-only save not to return a stream id, got %q", got.StreamID)
	}
	if loginHit || listHit || quickAddHit {
		t.Fatalf("expected settings-only save not to hit network, login=%v list=%v quickAdd=%v", loginHit, listHit, quickAddHit)
	}
}

type uiRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn uiRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func uiResponseWithBody(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func uiResponseWithJSON(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
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
	m.sidebarCursor = 0

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
	m.sidebarCursor = 0

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
