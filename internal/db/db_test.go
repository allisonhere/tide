package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestOpenMigratesLegacyConfigDB(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, ".local", "share"))

	legacyDir := filepath.Join(tmp, ".config", "rss")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	legacy, err := openSQLite(filepath.Join(legacyDir, "rss.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.init(); err != nil {
		t.Fatal(err)
	}

	feedID, err := legacy.AddFeed("https://example.com/feed.xml", "Example Feed", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.UpsertArticle(Article{
		FeedID:      feedID,
		GUID:        "article-1",
		Title:       "Hello",
		Link:        "https://example.com/hello",
		Content:     "content",
		PublishedAt: unixTime(1710000000),
	}); err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	db, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	feeds, err := db.ListFeeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 1 {
		t.Fatalf("expected 1 migrated feed, got %d", len(feeds))
	}
	if feeds[0].Title != "Example Feed" {
		t.Fatalf("expected migrated title %q, got %q", "Example Feed", feeds[0].Title)
	}

	articles, err := db.ListArticles(feeds[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected 1 migrated article, got %d", len(articles))
	}
}

func openSQLite(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	return &DB{conn}, nil
}

func unixTime(ts int64) time.Time {
	return time.Unix(ts, 0)
}
