package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tide/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func searchModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(nil, config.DefaultConfig(), "v1.0.0", false)
	m.width, m.height = 100, 30
	m.styles = BuildStyles(CatppuccinMocha, "comfortable")
	m.folders = []db.Folder{{ID: 10, Name: "Tech"}}
	m.feeds = []db.Feed{
		{ID: 1, Title: "Feed One", URL: "https://example.com/1", FolderID: 10},
		{ID: 2, Title: "Feed Two", URL: "https://example.com/2"},
	}
	m.sidebarRows = []sidebarRow{
		{kind: rowKindFolder, folderID: 10},
		{kind: rowKindFeed, feedID: 1},
		{kind: rowKindFeed, feedID: 2},
	}
	m.sidebarCursor = 1
	m.overlay = overlaySearch
	return m
}

func result(id, feedID int64, title, feed string) db.SearchResult {
	return db.SearchResult{
		Article:   db.Article{ID: id, FeedID: feedID, Title: title, PublishedAt: time.Unix(1700000000, 0)},
		FeedTitle: feed,
	}
}

// Results from a keystroke the user has already typed past must be dropped,
// otherwise a slow query can overwrite a newer one.
func TestSearchDiscardsStaleResults(t *testing.T) {
	m := searchModel(t)
	m.searchInput.SetValue("ru")
	m.startSearch() // seq 1
	m.searchInput.SetValue("rust")
	m.startSearch() // seq 2

	stale := SearchResultsMsg{Seq: 1, Query: "ru", Results: []db.SearchResult{result(1, 1, "Stale hit", "Feed One")}}
	next, _ := m.Update(stale)
	if got := next.(Model); len(got.searchResults) != 0 {
		t.Fatalf("expected stale results to be discarded, got %d", len(got.searchResults))
	}

	fresh := SearchResultsMsg{Seq: 2, Query: "rust", Results: []db.SearchResult{result(2, 1, "Fresh hit", "Feed One")}}
	next, _ = m.Update(fresh)
	got := next.(Model)
	if len(got.searchResults) != 1 || got.searchResults[0].Title != "Fresh hit" {
		t.Fatalf("expected current results to be applied, got %+v", got.searchResults)
	}
}

func TestSearchCursorNavigationClamps(t *testing.T) {
	m := searchModel(t)
	m.searchResults = []db.SearchResult{
		result(1, 1, "One", "Feed One"),
		result(2, 1, "Two", "Feed One"),
	}

	step := func(m Model, k tea.KeyMsg) Model {
		next, _ := m.Update(k)
		return next.(Model)
	}

	// Up at the top stays put.
	m = step(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.searchResultCursor != 0 {
		t.Fatalf("expected cursor to stay at 0, got %d", m.searchResultCursor)
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.searchResultCursor != 1 {
		t.Fatalf("expected cursor 1, got %d", m.searchResultCursor)
	}
	// Down past the end stays put.
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.searchResultCursor != 1 {
		t.Fatalf("expected cursor clamped to 1, got %d", m.searchResultCursor)
	}
}

// Opening a result must select the result's feed, not leave the previous one.
func TestSearchOpenResultSelectsItsFeed(t *testing.T) {
	m := searchModel(t)
	m.searchResults = []db.SearchResult{result(42, 2, "Living in Feed Two", "Feed Two")}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)

	if got.overlay != overlayNone {
		t.Fatalf("expected overlay closed, got %v", got.overlay)
	}
	feed := got.selectedFeed()
	if feed == nil || feed.ID != 2 {
		t.Fatalf("expected feed 2 selected, got %+v", feed)
	}
	if got.pendingArticleID != 42 {
		t.Fatalf("expected article 42 queued, got %d", got.pendingArticleID)
	}
}

// A result inside a collapsed folder has no sidebar row; opening it must reveal
// the folder rather than silently doing nothing.
func TestSearchOpenResultExpandsCollapsedFolder(t *testing.T) {
	m := searchModel(t)
	m.collapsedFolders[10] = true
	m.rebuildSidebar()
	if m.sidebarRowForFeed(1) >= 0 {
		t.Fatal("setup: expected feed 1 to be hidden by the collapsed folder")
	}

	m.searchResults = []db.SearchResult{result(7, 1, "Hidden away", "Feed One")}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)

	if got.collapsedFolders[10] {
		t.Fatal("expected the folder to be expanded")
	}
	feed := got.selectedFeed()
	if feed == nil || feed.ID != 1 {
		t.Fatalf("expected feed 1 selected after expanding, got %+v", feed)
	}
}

// Unread-only would otherwise hide the very article the user chose to open.
func TestSelectPendingArticleLiftsUnreadOnly(t *testing.T) {
	m := searchModel(t)
	m.showUnreadOnly = true
	m.articles = []db.Article{
		{ID: 1, Title: "Unread one"},
		{ID: 2, Title: "Read target", Read: true},
	}
	m.applyFilter()
	if len(m.filteredArticles) != 1 {
		t.Fatalf("setup: expected unread-only to hide the read article, got %d", len(m.filteredArticles))
	}

	m.pendingArticleID = 2
	m.selectPendingArticle()

	if m.showUnreadOnly {
		t.Fatal("expected unread-only to be lifted so the target is reachable")
	}
	if m.articleCursor != m.indexOfFilteredArticle(2) || m.articleCursor < 0 {
		t.Fatalf("expected cursor on the target article, got %d", m.articleCursor)
	}
	if m.pendingArticleID != 0 {
		t.Fatal("expected pending article to be consumed")
	}
}

func TestSearchEscapeClearsState(t *testing.T) {
	m := searchModel(t)
	m.searchInput.SetValue("rust")
	m.searchResults = []db.SearchResult{result(1, 1, "One", "Feed One")}
	m.searchResultCursor = 0

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := next.(Model)

	if got.overlay != overlayNone {
		t.Fatalf("expected overlay closed, got %v", got.overlay)
	}
	if len(got.searchResults) != 0 || got.searchInput.Value() != "" || got.searchQuery != "" {
		t.Fatalf("expected search state cleared, got results=%d input=%q query=%q",
			len(got.searchResults), got.searchInput.Value(), got.searchQuery)
	}
}

func TestSearchOverlayRendersResults(t *testing.T) {
	m := searchModel(t)
	m.searchInput.SetValue("rust")
	m.searchResults = []db.SearchResult{
		{
			Article:   db.Article{ID: 1, FeedID: 1, Title: "Async Rust", PublishedAt: time.Unix(1700000000, 0)},
			FeedTitle: "Feed One",
			Snippet:   "…the " + db.SnippetOpen + "async" + db.SnippetClose + " runtime…",
		},
		{
			Article:   db.Article{ID: 2, FeedID: 2, Title: "Tokio notes", PublishedAt: time.Unix(1700000000, 0)},
			FeedTitle: "Feed Two",
		},
	}

	view := ansi.Strip(m.renderSearchOverlay(72, newManagerChrome(72, CatppuccinMocha, false)))

	for _, want := range []string{"2 matches", "Async Rust", "Feed One", "Tokio notes", "async runtime", "enter open"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected overlay to contain %q, got:\n%s", want, view)
		}
	}
	// Snippet delimiters are styling markers and must never reach the screen.
	if strings.ContainsAny(view, db.SnippetOpen+db.SnippetClose) {
		t.Fatal("expected snippet delimiters to be consumed by styling")
	}
}

func TestSearchOverlayStatusStates(t *testing.T) {
	chrome := newManagerChrome(72, CatppuccinMocha, false)
	for _, tc := range []struct{ name, query, want string }{
		{"empty", "", "matches titles and article text"},
		{"no matches", "zzz", "no matches"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := searchModel(t)
			m.searchInput.SetValue(tc.query)
			got := ansi.Strip(m.renderSearchStatus(72, chrome))
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// Snippets come from SQLite; unbalanced or absent delimiters must not panic or
// leak control characters.
func TestRenderSnippetHandlesMalformedInput(t *testing.T) {
	chrome := newManagerChrome(72, CatppuccinMocha, false)
	for _, in := range []string{
		"",
		"plain text",
		db.SnippetOpen + "unclosed",
		db.SnippetClose + "stray close",
		db.SnippetOpen + db.SnippetOpen + "double" + db.SnippetClose,
		strings.Repeat("long ", 100),
	} {
		got := ansi.Strip(renderSnippet(in, 40, chrome))
		if strings.ContainsAny(got, db.SnippetOpen+db.SnippetClose) {
			t.Fatalf("delimiters leaked for input %q: %q", in, got)
		}
		if lipglossWidthSafe(got) > 40 {
			t.Fatalf("snippet exceeded width for input %q: %q", in, got)
		}
	}
}

func lipglossWidthSafe(s string) int {
	w := 0
	for _, line := range strings.Split(s, "\n") {
		if n := len([]rune(line)); n > w {
			w = n
		}
	}
	return w
}
