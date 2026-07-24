package db

import (
	"path/filepath"
	"testing"
	"time"
)

func newSearchDB(t *testing.T) *DB {
	t.Helper()
	db, err := openSQLite(filepath.Join(t.TempDir(), "rss.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.init(); err != nil {
		t.Fatal(err)
	}
	if !db.ftsAvailable {
		t.Fatal("expected FTS5 to be available in this build")
	}
	return db
}

func seedFeed(t *testing.T, db *DB, title string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO feeds (url, title) VALUES (?, ?)`, "https://example.com/"+title, title)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedArticle(t *testing.T, db *DB, feedID int64, guid, title, content string) {
	t.Helper()
	if err := db.UpsertArticle(Article{
		FeedID: feedID, GUID: guid, Title: title, Link: "https://example.com/" + guid,
		Content: content, PublishedAt: time.Unix(1700000000, 0),
	}); err != nil {
		t.Fatal(err)
	}
}

func titles(results []SearchResult) []string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.Title)
	}
	return out
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// The exact case the old substring matcher could not handle: terms that appear
// in a different order than typed.
func TestSearchMultiTermIsOrderIndependent(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "a1", "Async patterns in Rust", "pinning, futures and executors")
	seedArticle(t, db, f, "a2", "Baking sourdough", "starter hydration")

	got, err := db.SearchArticles("rust async", false, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(titles(got), "Async patterns in Rust") {
		t.Fatalf("expected multi-term match regardless of order, got %v", titles(got))
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly one hit, got %v", titles(got))
	}
}

// Body-only hits were invisible before: only titles were ever matched.
func TestSearchMatchesBodyAndSummary(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "b1", "Weekly roundup", "a deep dive on goroutine scheduling")

	got, err := db.SearchArticles("goroutine", false, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected body match, got %v", titles(got))
	}
	if got[0].Snippet == "" {
		t.Fatal("expected a snippet for an FTS hit")
	}

	// Summaries are indexed too, via the update trigger.
	var id int64
	if err := db.QueryRow(`SELECT id FROM articles WHERE guid = 'b1'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSummary(id, "covers zoxide and fzf tooling"); err != nil {
		t.Fatal(err)
	}
	got, err = db.SearchArticles("zoxide", false, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected summary to be searchable after SaveSummary, got %v", titles(got))
	}
}

func TestSearchRanksTitleHitsAboveBodyHits(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "t1", "Kubernetes operators", "an intro")
	// Several decoys so bm25's IDF term is meaningful rather than degenerate.
	for _, g := range []string{"d1", "d2", "d3", "d4"} {
		seedArticle(t, db, f, g, "Unrelated "+g, "nothing to see")
	}
	seedArticle(t, db, f, "t2", "Weekend notes", "played with kubernetes a bit")

	got, err := db.SearchArticles("kubernetes", false, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 hits, got %v", titles(got))
	}
	if got[0].Title != "Kubernetes operators" {
		t.Fatalf("expected the title hit ranked first, got %v", titles(got))
	}
}

func TestSearchUnreadOnly(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "u1", "Rust news", "")
	seedArticle(t, db, f, "u2", "More rust", "")

	var id int64
	if err := db.QueryRow(`SELECT id FROM articles WHERE guid = 'u1'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkRead(id, true); err != nil {
		t.Fatal(err)
	}

	got, err := db.SearchArticles("rust", true, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "More rust" {
		t.Fatalf("expected only the unread hit, got %v", titles(got))
	}
}

func TestSearchResultCarriesFeedTitle(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "Fasterthanlime")
	seedArticle(t, db, f, "f1", "Pin and suffering", "")

	got, err := db.SearchArticles("suffering", false, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FeedTitle != "Fasterthanlime" {
		t.Fatalf("expected feed title on result, got %+v", got)
	}
}

// The index is external-content, so it only stays correct if every write path
// fires a trigger. This is the highest-risk part of the migration.
func TestSearchIndexStaysInSyncWithWrites(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	// The probe term must be unique to the body: a term shared with the title
	// would still match after a content update and prove nothing.
	seedArticle(t, db, f, "s1", "Original title", "body mentioning wombats")

	countFTS := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM articles_fts`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	search := func(q string) int {
		t.Helper()
		got, err := db.SearchArticles(q, false, 50)
		if err != nil {
			t.Fatal(err)
		}
		return len(got)
	}

	if countFTS() != 1 {
		t.Fatalf("insert trigger: expected 1 indexed row, got %d", countFTS())
	}

	var id int64
	if err := db.QueryRow(`SELECT id FROM articles WHERE guid = 's1'`).Scan(&id); err != nil {
		t.Fatal(err)
	}

	// UpdateArticleContent must reindex: old term gone, new term found.
	if err := db.UpdateArticleContent(id, "replacement body about pelicans"); err != nil {
		t.Fatal(err)
	}
	if search("wombats") != 0 {
		t.Fatal("update trigger: stale content still searchable")
	}
	if search("pelicans") != 1 {
		t.Fatal("update trigger: new content not searchable")
	}

	// Upserting the same guid updates in place and must not duplicate rows.
	seedArticle(t, db, f, "s1", "Revised title", "revised body")
	if countFTS() != 1 {
		t.Fatalf("upsert: expected index to stay at 1 row, got %d", countFTS())
	}
	if search("revised") != 1 {
		t.Fatal("upsert: revised text not searchable")
	}

	// Deleting the feed cascades to articles, which must clear the index.
	if _, err := db.Exec(`DELETE FROM feeds WHERE id = ?`, f); err != nil {
		t.Fatal(err)
	}
	if countFTS() != 0 {
		t.Fatalf("cascade delete: expected empty index, got %d rows", countFTS())
	}
	if search("revised") != 0 {
		t.Fatal("cascade delete: deleted article still searchable")
	}
}

// Raw user input is not valid FTS5 syntax. None of these may error, because
// they are typed on the way to a longer query.
func TestSearchHandlesHostileInput(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "h1", "C++ 20 concepts", `he said "hello" - then left`)

	for _, q := range []string{
		"-", "*", `"`, "^", ":", "()", "AND", "OR", "NOT", "NEAR(",
		"c++", "foo-bar", `"quoted"`, "a:b", "***", `""`, "  ", "\t",
	} {
		if _, err := db.SearchArticles(q, false, 50); err != nil {
			t.Fatalf("query %q errored: %v", q, err)
		}
	}

	// And real punctuation-bearing queries still find their article.
	for _, q := range []string{"c++", "concepts", "hello"} {
		got, err := db.SearchArticles(q, false, 50)
		if err != nil {
			t.Fatalf("query %q errored: %v", q, err)
		}
		if len(got) == 0 {
			t.Fatalf("expected %q to match the seeded article", q)
		}
	}
}

func TestBuildMatchQuery(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"rust", `"rust"*`},
		{"rust async", `"rust" "async"*`},
		{"c++ 20", `"c++" "20"*`},
		{`say "hi"`, `"say" """hi"""*`},
	} {
		if got := buildMatchQuery(tc.in); got != tc.want {
			t.Errorf("buildMatchQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Empty queries must not return the whole library.
func TestSearchEmptyQueryReturnsNothing(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "e1", "Something", "anything")

	for _, q := range []string{"", "   "} {
		got, err := db.SearchArticles(q, false, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("expected no results for %q, got %d", q, len(got))
		}
	}
}

// A pre-existing database must have its rows backfilled by the migration.
func TestMigrationBackfillsAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rss.db")
	db, err := openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Build a pre-FTS database with rows already present: base schema only, at
	// user_version 0, then let the whole migration ladder run over live data.
	if err := db.migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO feeds (url, title) VALUES ('https://e.example/f','Legacy')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO articles (feed_id, guid, title, content, published_at)
	                      VALUES (1,'g1','Legacy article','about ferrets',1700000000)`); err != nil {
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
		t.Fatalf("expected version %d, got %d", latestSchemaVersion, version)
	}
	got, err := db.SearchArticles("ferrets", false, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected migration to backfill pre-existing rows, got %d hits", len(got))
	}

	// Re-running must not double-index or error.
	if err := db.migrateSchema(); err != nil {
		t.Fatalf("re-running migration failed: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM articles_fts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 indexed row after re-migration, got %d", n)
	}
}
