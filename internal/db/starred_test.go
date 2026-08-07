package db

import (
	"testing"
)

// articleIDByGUID resolves a seeded article so tests can star it without
// hardcoding rowids.
func articleIDByGUID(t *testing.T, db *DB, guid string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM articles WHERE guid = ?`, guid).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func star(t *testing.T, db *DB, guid string) int64 {
	t.Helper()
	id := articleIDByGUID(t, db, guid)
	if err := db.SetStarred(id, true); err != nil {
		t.Fatal(err)
	}
	return id
}

// The realistic upgrade: a populated database from the previous release gains
// the column without disturbing the articles already in it.
func TestMigrateAddsStarredToExistingDatabase(t *testing.T) {
	db := newSearchDB(t) // fully migrated
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "a1", "Existing article", "body")

	// Rewind to the pre-starred schema, as an older install would be. The index
	// has to go first — SQLite refuses to drop a column an index still refers to.
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_articles_starred`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE articles DROP COLUMN starred`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 7`); err != nil {
		t.Fatal(err)
	}

	if err := db.migrateSchema(); err != nil {
		t.Fatal(err)
	}

	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != latestSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", latestSchemaVersion, version)
	}

	articles, err := db.ListArticles(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 || articles[0].Title != "Existing article" {
		t.Fatalf("expected the existing article to survive migration, got %+v", articles)
	}
	if articles[0].Starred {
		t.Fatal("expected existing articles to default to unstarred")
	}

	// Re-running the migration must be a no-op, not an error.
	if _, err := db.Exec(`PRAGMA user_version = 7`); err != nil {
		t.Fatal(err)
	}
	if err := db.migrateSchema(); err != nil {
		t.Fatalf("expected re-running the migration to be safe, got %v", err)
	}
}

func TestSetStarredRoundTrips(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "a1", "Async patterns in Rust", "futures and executors")

	id := articleIDByGUID(t, db, "a1")
	articles, err := db.ListArticles(f)
	if err != nil {
		t.Fatal(err)
	}
	if articles[0].Starred {
		t.Fatal("expected a fresh article to be unstarred")
	}

	if err := db.SetStarred(id, true); err != nil {
		t.Fatal(err)
	}
	if articles, err = db.ListArticles(f); err != nil {
		t.Fatal(err)
	}
	if !articles[0].Starred {
		t.Fatal("expected article to read back as starred")
	}

	if err := db.SetStarred(id, false); err != nil {
		t.Fatal(err)
	}
	if articles, err = db.ListArticles(f); err != nil {
		t.Fatal(err)
	}
	if articles[0].Starred {
		t.Fatal("expected unstar to clear the flag")
	}
}

// Saving must survive a feed refresh: UpsertArticle's ON CONFLICT branch
// rewrites the article's content, and must not reset the user's own flag.
func TestUpsertArticlePreservesStarred(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "a1", "Original title", "original body")
	star(t, db, "a1")

	// Same feed + guid, new content — exactly what a refetch produces.
	seedArticle(t, db, f, "a1", "Updated title", "updated body")

	articles, err := db.ListArticles(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected upsert to update in place, got %d articles", len(articles))
	}
	if articles[0].Title != "Updated title" {
		t.Fatalf("expected refreshed title, got %q", articles[0].Title)
	}
	if !articles[0].Starred {
		t.Fatal("expected a feed refresh to preserve the saved flag")
	}
}

func TestListStarredArticlesSpansFeedsNewestFirst(t *testing.T) {
	db := newSearchDB(t)
	f1 := seedFeed(t, db, "one")
	f2 := seedFeed(t, db, "two")
	seedArticle(t, db, f1, "a1", "Older saved", "body")
	seedArticle(t, db, f2, "a2", "Newer saved", "body")
	seedArticle(t, db, f1, "a3", "Not saved", "body")

	// seedArticle stamps a fixed publish time, so set them apart explicitly.
	if _, err := db.Exec(`UPDATE articles SET published_at = 100 WHERE guid = 'a1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE articles SET published_at = 200 WHERE guid = 'a2'`); err != nil {
		t.Fatal(err)
	}
	star(t, db, "a1")
	star(t, db, "a2")

	got, err := db.ListStarredArticles(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected only saved articles, got %d", len(got))
	}
	if got[0].Title != "Newer saved" || got[1].Title != "Older saved" {
		t.Fatalf("expected newest-first across feeds, got %v", []string{got[0].Title, got[1].Title})
	}

	n, err := db.CountStarred()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("CountStarred: got %d, want 2", n)
	}
}

func TestParseSearchFilters(t *testing.T) {
	cases := []struct {
		in          string
		wantText    string
		wantStarred bool
	}{
		{"rust async", "rust async", false},
		{"is:starred", "", true},
		{"is:starred rust", "rust", true},
		{"rust is:starred", "rust", true},
		{"IS:Starred rust", "rust", true},
		{"is:saved rust", "rust", true},
		{"is:star rust", "rust", true},
		// Not a filter — must survive as ordinary search text.
		{"is:read rust", "is:read rust", false},
		{"", "", false},
	}
	for _, tc := range cases {
		text, starred := parseSearchFilters(tc.in)
		if text != tc.wantText || starred != tc.wantStarred {
			t.Errorf("parseSearchFilters(%q) = (%q, %v), want (%q, %v)",
				tc.in, text, starred, tc.wantText, tc.wantStarred)
		}
	}
}

// The filter has to be stripped before buildMatchQuery, which quotes every
// token; otherwise "is:starred" is searched for as literal article text.
func TestSearchStarredFilterIsNotSearchedAsText(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "a1", "Async patterns in Rust", "futures and executors")
	seedArticle(t, db, f, "a2", "Rust error handling", "results and options")
	star(t, db, "a1")

	got, err := db.SearchArticles("rust is:starred", false, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected the filter to narrow to saved matches, got %v", titles(got))
	}
	if got[0].Title != "Async patterns in Rust" {
		t.Fatalf("got %q, want the saved article", got[0].Title)
	}
	if !got[0].Starred {
		t.Fatal("expected the result to carry its starred flag")
	}

	// Without the filter both articles still match.
	if got, err = db.SearchArticles("rust", false, 50); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected unfiltered search to match both, got %v", titles(got))
	}
}

// A bare filter has no search terms, so there is no MATCH expression. It should
// browse everything saved rather than return nothing.
func TestSearchBareStarredFilterBrowsesSaved(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "a1", "Saved one", "body")
	seedArticle(t, db, f, "a2", "Saved two", "body")
	seedArticle(t, db, f, "a3", "Unsaved", "body")
	star(t, db, "a1")
	star(t, db, "a2")

	got, err := db.SearchArticles("is:starred", false, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected a bare filter to list all saved, got %v", titles(got))
	}
	for _, r := range got {
		if !r.Starred {
			t.Fatalf("unstarred article %q leaked into is:starred", r.Title)
		}
		if r.FeedTitle != "blog" {
			t.Fatalf("expected feed title to be populated, got %q", r.FeedTitle)
		}
	}

	// An empty query still returns nothing.
	if got, err = db.SearchArticles("   ", false, 50); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty query to return no results, got %d", len(got))
	}
}

func TestSearchStarredFilterCombinesWithUnreadOnly(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "a1", "Saved and read", "body")
	seedArticle(t, db, f, "a2", "Saved and unread", "body")
	readID := star(t, db, "a1")
	star(t, db, "a2")
	if err := db.MarkRead(readID, true); err != nil {
		t.Fatal(err)
	}

	got, err := db.SearchArticles("is:starred", true, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Saved and unread" {
		t.Fatalf("expected unread-only to apply alongside is:starred, got %v", titles(got))
	}
}

// The LIKE fallback covers SQLite builds without FTS5 and must honour the same
// filter semantics as the indexed path.
func TestSearchArticlesLikeHonoursStarredFilter(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "a1", "Async patterns in Rust", "futures")
	seedArticle(t, db, f, "a2", "Rust error handling", "results")
	star(t, db, "a1")

	got, err := db.searchArticlesLike("rust", false, true, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Async patterns in Rust" {
		t.Fatalf("expected LIKE fallback to filter to saved, got %v", titles(got))
	}
	if !got[0].Starred {
		t.Fatal("expected LIKE fallback to scan the starred column")
	}
}
