package db

import (
	"testing"
	"time"
)

func retentionTestDB(t *testing.T) *DB {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	database, err := Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// seedRetentionArticle inserts an article and returns its row id. UpsertArticle
// always writes read = 0 — a refresh must never change read state — so the
// flags are applied afterwards, the same way the app sets them.
func seedRetentionArticle(t *testing.T, database *DB, feedID int64, a Article) int64 {
	t.Helper()
	read, starred := a.Read, a.Starred
	if err := database.UpsertArticle(a); err != nil {
		t.Fatalf("seed article %q: %v", a.GUID, err)
	}
	var id int64
	if err := database.QueryRow(`SELECT id FROM articles WHERE feed_id = ? AND guid = ?`, feedID, a.GUID).Scan(&id); err != nil {
		t.Fatalf("look up seeded article %q: %v", a.GUID, err)
	}
	if read {
		if err := database.MarkRead(id, true); err != nil {
			t.Fatalf("mark %q read: %v", a.GUID, err)
		}
	}
	if starred {
		if err := database.SetStarred(id, true); err != nil {
			t.Fatalf("star %q: %v", a.GUID, err)
		}
	}
	return id
}

// Retention removes read history and nothing else: a star, a saved summary or
// an unread flag each keep an article regardless of age.
func TestPruneReadArticlesKeepsWhatTheUserInvestedIn(t *testing.T) {
	database := retentionTestDB(t)
	feedID, err := database.AddFeed("https://example.com/feed", "Feed One", "")
	if err != nil {
		t.Fatalf("add feed: %v", err)
	}

	old := time.Now().AddDate(0, 0, -90)
	recent := time.Now().AddDate(0, 0, -1)

	staleRead := seedRetentionArticle(t, database, feedID, Article{FeedID: feedID, GUID: "stale-read", Title: "Stale read", PublishedAt: old, Read: true})
	staleUnread := seedRetentionArticle(t, database, feedID, Article{FeedID: feedID, GUID: "stale-unread", Title: "Stale unread", PublishedAt: old})
	staleStarred := seedRetentionArticle(t, database, feedID, Article{FeedID: feedID, GUID: "stale-star", Title: "Stale starred", PublishedAt: old, Read: true, Starred: true})
	recentRead := seedRetentionArticle(t, database, feedID, Article{FeedID: feedID, GUID: "recent-read", Title: "Recent read", PublishedAt: recent, Read: true})
	noDate := seedRetentionArticle(t, database, feedID, Article{FeedID: feedID, GUID: "no-date", Title: "No date", Read: true})

	staleSummarized := seedRetentionArticle(t, database, feedID, Article{FeedID: feedID, GUID: "stale-summary", Title: "Stale summarized", PublishedAt: old, Read: true})
	if err := database.SaveSummary(staleSummarized, "an expensive summary"); err != nil {
		t.Fatalf("set summary: %v", err)
	}

	removed, err := database.PruneReadArticles(time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected exactly the stale read article to go, removed %d", removed)
	}

	for _, tc := range []struct {
		id   int64
		name string
		want bool
	}{
		{staleRead, "stale read", false},
		{staleUnread, "stale unread", true},
		{staleStarred, "stale starred", true},
		{staleSummarized, "stale summarized", true},
		{recentRead, "recent read", true},
		{noDate, "undated read", true},
	} {
		var n int
		if err := database.QueryRow(`SELECT COUNT(*) FROM articles WHERE id = ?`, tc.id).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tc.name, err)
		}
		if got := n == 1; got != tc.want {
			t.Fatalf("%s: present = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The articles_fts delete trigger has to keep up, or pruned articles go on
// turning up in search results with no row behind them.
func TestPruneReadArticlesDropsThemFromSearch(t *testing.T) {
	database := retentionTestDB(t)
	feedID, err := database.AddFeed("https://example.com/feed", "Feed One", "")
	if err != nil {
		t.Fatalf("add feed: %v", err)
	}
	old := time.Now().AddDate(0, 0, -90)
	seedRetentionArticle(t, database, feedID, Article{
		FeedID: feedID, GUID: "stale", Title: "Quokka sightings", Content: "quokka", PublishedAt: old, Read: true,
	})

	before, err := database.SearchArticles("quokka", false, 10)
	if err != nil {
		t.Fatalf("search before prune: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("expected the article to be searchable before the prune, got %d hits", len(before))
	}

	if _, err := database.PruneReadArticles(time.Now().AddDate(0, 0, -30)); err != nil {
		t.Fatalf("prune: %v", err)
	}

	after, err := database.SearchArticles("quokka", false, 10)
	if err != nil {
		t.Fatalf("search after prune: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected pruned articles to leave the search index, got %d hits", len(after))
	}
}

func TestArticleCountAndFileSize(t *testing.T) {
	database := retentionTestDB(t)
	feedID, err := database.AddFeed("https://example.com/feed", "Feed One", "")
	if err != nil {
		t.Fatalf("add feed: %v", err)
	}
	for _, guid := range []string{"a", "b", "c"} {
		seedRetentionArticle(t, database, feedID, Article{FeedID: feedID, GUID: guid, Title: guid, PublishedAt: time.Now()})
	}

	n, err := database.ArticleCount()
	if err != nil {
		t.Fatalf("article count: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 articles, got %d", n)
	}
	if size := database.FileSize(); size <= 0 {
		t.Fatalf("expected a positive database size, got %d", size)
	}
}

func TestVacuumRuns(t *testing.T) {
	database := retentionTestDB(t)
	if err := database.Vacuum(); err != nil {
		t.Fatalf("vacuum: %v", err)
	}
}
