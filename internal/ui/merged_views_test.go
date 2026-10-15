package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tide/internal/db"
)

// newMergedViewModel stores two local feeds in folder 10, each holding one
// read and one unread article, and loads them into a sized model.
func newMergedViewModel(t *testing.T) (Model, *db.DB) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	t.Cleanup(func() { database.Close() })

	folderID, err := database.AddFolder("Tech", "")
	if err != nil {
		t.Fatalf("AddFolder: %v", err)
	}
	for i, name := range []string{"Alpha", "Beta"} {
		feedID, _ := database.AddFeed("https://example.com/"+name, name, "")
		if err := database.SetFeedFolder(feedID, folderID); err != nil {
			t.Fatalf("SetFeedFolder: %v", err)
		}
		if _, err := database.UpsertArticles([]db.Article{
			{FeedID: feedID, GUID: name + "-new", Title: name + " unread", PublishedAt: unixTestTime(int64(1710000100 + i))},
			{FeedID: feedID, GUID: name + "-old", Title: name + " read", PublishedAt: unixTestTime(int64(1710000000 + i)), Read: true},
		}); err != nil {
			t.Fatalf("UpsertArticles: %v", err)
		}
	}
	feeds, _ := database.ListFeeds()
	folders, _ := database.ListFolders()

	m := NewModel(database, config.DefaultConfig(), "v1.0.0", false)
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = m2.(Model)
	m2, _ = m.Update(FeedsLoadedMsg{Feeds: feeds, Folders: folders})
	return m2.(Model), database
}

// selectRow moves the sidebar cursor to the first row of kind and applies the
// article load that selecting it issues.
func selectRow(t *testing.T, m Model, kind sidebarRowKind) Model {
	t.Helper()
	for i, row := range m.sidebarRows {
		if row.kind == kind {
			m.sidebarCursor = i
			m2, _ := m.Update(m.selectionArticlesCmd()())
			return m2.(Model)
		}
	}
	t.Fatalf("no sidebar row of kind %v in %+v", kind, m.sidebarRows)
	return m
}

func titles(articles []db.Article) []string {
	var out []string
	for _, a := range articles {
		out = append(out, a.Title)
	}
	return out
}

func TestAllUnreadListsUnreadAcrossLocalFeeds(t *testing.T) {
	m, _ := newMergedViewModel(t)
	m = selectRow(t, m, rowKindAllUnread)

	got := titles(m.filteredArticles)
	if len(got) != 2 || got[0] != "Beta unread" || got[1] != "Alpha unread" {
		t.Fatalf("expected both feeds' unread articles newest first, got %q", got)
	}
	if title := m.articlesPaneTitle(); title != allUnreadRowLabel {
		t.Fatalf("expected the pane titled %q, got %q", allUnreadRowLabel, title)
	}
	if view := m.renderArticlesPane(); !strings.Contains(view, "Alpha ·") || !strings.Contains(view, "Beta ·") {
		t.Fatalf("expected merged rows to name their feed:\n%s", view)
	}
}

func TestFolderRowListsItsFeedsAndFollowsUnreadOnly(t *testing.T) {
	m, _ := newMergedViewModel(t)
	m.showUnreadOnly = false
	m = selectRow(t, m, rowKindFolder)
	if got := titles(m.filteredArticles); len(got) != 4 {
		t.Fatalf("expected all four articles with unread-only off, got %q", got)
	}

	m.showUnreadOnly = true
	m = selectRow(t, m, rowKindFolder)
	if got := titles(m.filteredArticles); len(got) != 2 {
		t.Fatalf("expected the two unread articles with unread-only on, got %q", got)
	}
}

func TestSingleFeedRowsDoNotNameTheirFeed(t *testing.T) {
	m, _ := newMergedViewModel(t)
	m = selectRow(t, m, rowKindFeed)
	if view := m.renderArticlesPane(); strings.Contains(view, "Alpha ·") {
		t.Fatalf("expected no feed tag in a single-feed list:\n%s", view)
	}
}

func TestStaleFolderLoadIsDropped(t *testing.T) {
	m, _ := newMergedViewModel(t)
	m = selectRow(t, m, rowKindFeed)
	before := titles(m.filteredArticles)

	m2, _ := m.Update(ArticlesLoadedMsg{Kind: rowKindFolder, FolderID: m.feeds[0].FolderID, Articles: []db.Article{{ID: 99, Title: "stale"}}})
	m = m2.(Model)
	if got := titles(m.filteredArticles); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Fatalf("expected a folder load to be ignored while a feed is selected, got %q", got)
	}
}

func TestMarkAllReadOnAllUnreadNeedsASecondPress(t *testing.T) {
	m, database := newMergedViewModel(t)
	m = selectRow(t, m, rowKindAllUnread)
	m.focused = paneFeeds
	r := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}

	next, _ := m.Update(r)
	m = next.(Model)
	if !m.markAllUnreadArmed {
		t.Fatal("expected the first R to ask for confirmation")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	if m.markAllUnreadArmed {
		t.Fatal("expected another key to cancel the pending mark-all-read")
	}

	m = selectRow(t, m, rowKindAllUnread)
	next, _ = m.Update(r)
	m = next.(Model)
	_, cmd := m.Update(r)
	if cmd == nil {
		t.Fatal("expected the second R to mark everything read")
	}
	if _, ok := cmd().(FeedsReadUpdatedMsg); !ok {
		t.Fatal("expected a FeedsReadUpdatedMsg from the mark-all-read")
	}
	if n, _ := database.CountUnread(); n != 0 {
		t.Fatalf("expected every local article read, got %d unread", n)
	}
}
