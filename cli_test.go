package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tide/internal/db"
)

func TestBarStatusHidesWhenNothingUnread(t *testing.T) {
	got := barStatus([]db.Feed{{Title: "Quiet", UnreadCount: 0}})
	if got.Text != "" || got.Class != "empty" {
		t.Fatalf("expected an empty, hidden module, got %+v", got)
	}
}

func TestBarStatusTotalsAndRanksFeeds(t *testing.T) {
	feeds := []db.Feed{
		{Title: "Small", UnreadCount: 1},
		{Title: "Big", UnreadCount: 7},
		{Title: "None", UnreadCount: 0},
	}
	got := barStatus(feeds)
	if got.Text != " 8" || got.Class != "unread" {
		t.Fatalf("unexpected text/class: %+v", got)
	}
	if !strings.HasPrefix(got.Tooltip, "Tide: 8 unread\nBig  7\nSmall  1") || strings.Contains(got.Tooltip, "None") {
		t.Fatalf("unexpected tooltip %q", got.Tooltip)
	}
}

func TestBarStatusCapsTooltipFeeds(t *testing.T) {
	var feeds []db.Feed
	for i := range barTooltipFeeds + 2 {
		feeds = append(feeds, db.Feed{Title: string(rune('A' + i)), UnreadCount: 1})
	}
	if got := barStatus(feeds).Tooltip; !strings.HasSuffix(got, "…and 2 more feeds") {
		t.Fatalf("expected the tooltip to cap its feed list, got %q", got)
	}
}

func TestRunCommandUnreadCountAndBar(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	feedID, _ := database.AddFeed("https://example.com/feed.xml", "Example", "")
	if _, err := database.UpsertArticles([]db.Article{
		{FeedID: feedID, GUID: "a", Title: "A", PublishedAt: time.Unix(1, 0)},
		{FeedID: feedID, GUID: "b", Title: "B", PublishedAt: time.Unix(2, 0)},
	}); err != nil {
		t.Fatalf("UpsertArticles: %v", err)
	}
	database.Close()

	var out, errOut bytes.Buffer
	code, handled := runCommand([]string{"--unread-count"}, &out, &errOut)
	if !handled || code != 0 || out.String() != "2\n" {
		t.Fatalf("--unread-count: handled=%v code=%d out=%q err=%q", handled, code, out.String(), errOut.String())
	}

	out.Reset()
	code, handled = runCommand([]string{"--bar"}, &out, &errOut)
	var bar barJSON
	if !handled || code != 0 || json.Unmarshal(out.Bytes(), &bar) != nil || bar.Text != " 2" {
		t.Fatalf("--bar: handled=%v code=%d out=%q err=%q", handled, code, out.String(), errOut.String())
	}
}

func TestRunCommandLeavesReaderFlagsAlone(t *testing.T) {
	if _, handled := runCommand([]string{"--preview-manual-update"}, &bytes.Buffer{}, &bytes.Buffer{}); handled {
		t.Fatal("expected a non-headless flag to start the reader")
	}
}
