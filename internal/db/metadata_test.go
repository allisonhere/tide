package db

import (
	"testing"
	"time"
)

// An existing database from the previous release gains author / categories /
// updated_at without disturbing the articles already in it.
func TestMigrateAddsMetadataColumnsToExistingDatabase(t *testing.T) {
	db := newSearchDB(t) // fully migrated
	f := seedFeed(t, db, "blog")
	seedArticle(t, db, f, "a1", "Existing article", "body")

	for _, col := range []string{"updated_at", "categories", "author"} {
		if _, err := db.Exec(`ALTER TABLE articles DROP COLUMN ` + col); err != nil {
			t.Fatalf("rewind drop %s: %v", col, err)
		}
	}
	if _, err := db.Exec(`PRAGMA user_version = 9`); err != nil {
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
		t.Fatalf("existing article should survive migration, got %+v", articles)
	}
	if articles[0].Author != "" || len(articles[0].Categories) != 0 || !articles[0].UpdatedAt.IsZero() {
		t.Fatalf("migrated rows should default to empty metadata, got %+v", articles[0])
	}

	// Re-running the migration must be a no-op, not an error.
	if _, err := db.Exec(`PRAGMA user_version = 9`); err != nil {
		t.Fatal(err)
	}
	if err := db.migrateSchema(); err != nil {
		t.Fatalf("re-running the migration should be safe, got %v", err)
	}
}

func TestUpsertArticleRoundTripsMetadata(t *testing.T) {
	db := newSearchDB(t)
	f := seedFeed(t, db, "blog")

	updated := time.Unix(1710000000, 0)
	if err := db.UpsertArticle(Article{
		FeedID: f, GUID: "a1", Title: "Original", Link: "https://example.com/a1",
		Content: "body", PublishedAt: time.Unix(1700000000, 0),
		Author: "Ada Lovelace", Categories: []string{"Tech", "Science"}, UpdatedAt: updated,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := db.ListArticles(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 article, got %d", len(got))
	}
	a := got[0]
	if a.Author != "Ada Lovelace" {
		t.Fatalf("Author = %q", a.Author)
	}
	if len(a.Categories) != 2 || a.Categories[0] != "Tech" || a.Categories[1] != "Science" {
		t.Fatalf("Categories = %#v, want [Tech Science] in order", a.Categories)
	}
	if !a.UpdatedAt.Equal(updated) {
		t.Fatalf("UpdatedAt = %v, want %v", a.UpdatedAt, updated)
	}

	// A refresh (same feed+guid) rewrites metadata but leaves read/starred alone.
	id := articleIDByGUID(t, db, "a1")
	if err := db.SetStarred(id, true); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertArticle(Article{
		FeedID: f, GUID: "a1", Title: "Updated", Link: "https://example.com/a1",
		Content: "new body", PublishedAt: time.Unix(1700000000, 0),
		Author: "Grace Hopper", Categories: []string{"History"},
	}); err != nil {
		t.Fatal(err)
	}
	if got, err = db.ListArticles(f); err != nil {
		t.Fatal(err)
	}
	a = got[0]
	if a.Author != "Grace Hopper" || len(a.Categories) != 1 || a.Categories[0] != "History" {
		t.Fatalf("refresh should rewrite metadata, got author=%q cats=%#v", a.Author, a.Categories)
	}
	if !a.UpdatedAt.IsZero() {
		t.Fatalf("refresh with no <updated> should clear updated_at, got %v", a.UpdatedAt)
	}
	if !a.Starred {
		t.Fatal("refresh must preserve the saved flag")
	}
}
