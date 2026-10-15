package db

import (
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	d, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestUpsertArticlesReadAppliesOnInsertOnly(t *testing.T) {
	d := openTestDB(t)
	feedID, _ := d.AddFeed("https://example.com/feed.xml", "Example", "")

	n, err := d.UpsertArticles([]Article{
		{FeedID: feedID, GUID: "muted", Title: "Muted", PublishedAt: time.Unix(2, 0), Read: true},
		{FeedID: feedID, GUID: "plain", Title: "Plain", PublishedAt: time.Unix(1, 0)},
	})
	if err != nil || n != 2 {
		t.Fatalf("first upsert: n=%d err=%v", n, err)
	}
	if unread, _ := d.CountUnread(); unread != 1 {
		t.Fatalf("expected the read-on-insert article stored read, got %d unread", unread)
	}

	// A re-fetch carries Read=false for both; existing rows keep their state
	// while the refreshed fields still update.
	n, err = d.UpsertArticles([]Article{
		{FeedID: feedID, GUID: "muted", Title: "Muted (edited)", PublishedAt: time.Unix(2, 0)},
		{FeedID: feedID, GUID: "plain", Title: "Plain", PublishedAt: time.Unix(1, 0), Read: true},
	})
	if err != nil || n != 0 {
		t.Fatalf("re-fetch upsert: n=%d err=%v", n, err)
	}
	articles, _ := d.ListArticles(feedID)
	if len(articles) != 2 || articles[0].Title != "Muted (edited)" || !articles[0].Read || articles[1].Read {
		t.Fatalf("existing rows changed read state or missed the update: %+v", articles)
	}
}

func TestOpenSharesDatabaseWhileAnotherConnectionWrites(t *testing.T) {
	d := openTestDB(t)
	feedID, _ := d.AddFeed("https://example.com/feed.xml", "Example", "")

	other, err := Open()
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer other.Close()

	tx, err := d.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := tx.Exec(`UPDATE feeds SET title = 'Locked' WHERE id = ?`, feedID); err != nil {
		t.Fatalf("locking write: %v", err)
	}
	time.AfterFunc(200*time.Millisecond, func() { tx.Commit() })

	// Without a busy timeout this fails at once with SQLITE_BUSY.
	if _, err := other.UpsertArticles([]Article{{FeedID: feedID, GUID: "x", Title: "X", PublishedAt: time.Unix(1, 0)}}); err != nil {
		t.Fatalf("expected the second writer to wait for the lock, got %v", err)
	}
}

func TestListArticlesForFeedsMergesAndFilters(t *testing.T) {
	d := openTestDB(t)
	a, _ := d.AddFeed("https://example.com/a.xml", "A", "")
	b, _ := d.AddFeed("https://example.com/b.xml", "B", "")
	c, _ := d.AddFeed("https://example.com/c.xml", "C", "")
	if _, err := d.UpsertArticles([]Article{
		{FeedID: a, GUID: "a1", Title: "A1", PublishedAt: time.Unix(3, 0)},
		{FeedID: b, GUID: "b1", Title: "B1", PublishedAt: time.Unix(2, 0), Read: true},
		{FeedID: c, GUID: "c1", Title: "C1", PublishedAt: time.Unix(1, 0)},
	}); err != nil {
		t.Fatalf("UpsertArticles: %v", err)
	}

	all, err := d.ListArticlesForFeeds([]int64{a, b}, false)
	if err != nil || len(all) != 2 || all[0].GUID != "a1" || all[1].GUID != "b1" {
		t.Fatalf("expected A and B newest first, got %+v (err %v)", all, err)
	}
	unread, _ := d.ListArticlesForFeeds([]int64{a, b}, true)
	if len(unread) != 1 || unread[0].GUID != "a1" {
		t.Fatalf("expected only A's unread article, got %+v", unread)
	}
	if none, err := d.ListArticlesForFeeds(nil, false); err != nil || none != nil {
		t.Fatalf("expected nothing for no feeds, got %+v (err %v)", none, err)
	}
}
