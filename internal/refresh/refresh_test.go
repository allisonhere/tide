package refresh

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allisonhere/tide/internal/db"
)

const rssBody = `<?xml version="1.0"?>
<rss version="2.0"><channel><title>Test Feed</title>
<item><guid>a</guid><title>First</title><link>https://example.com/a</link><description>&lt;p&gt;one&lt;/p&gt;</description><pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate></item>
<item><guid>b</guid><title>Second</title><link>https://example.com/b</link><description>two</description><pubDate>Tue, 03 Jan 2006 15:04:05 GMT</pubDate></item>
</channel></rss>`

func openDB(t *testing.T) *db.DB {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	database, err := db.Open()
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func feedServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/broken" {
			http.Error(w, "gone", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(rssBody))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestStoreCountsNewArticlesAndKeepsExistingState(t *testing.T) {
	database := openDB(t)
	srv := feedServer(t)
	feedID, err := database.AddFeed(srv.URL+"/feed", "", "")
	if err != nil {
		t.Fatalf("AddFeed: %v", err)
	}

	first := Fetch(feedID, srv.URL+"/feed")
	if !first.Result.IsSuccess() {
		t.Fatalf("fetch failed: %v", first.Result.Err)
	}
	stored, err := Store(database, first)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if stored.New != 2 {
		t.Fatalf("expected 2 new articles, got %d", stored.New)
	}
	got, err := database.GetFeed(feedID)
	if err != nil || got.Title != "Test Feed" {
		t.Fatalf("expected feed title from the fetch, got %q (err %v)", got.Title, err)
	}

	articles, err := database.ListArticles(feedID)
	if err != nil || len(articles) != 2 {
		t.Fatalf("ListArticles: %d articles, err %v", len(articles), err)
	}
	if err := database.MarkRead(articles[0].ID, true); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if err := database.SetStarred(articles[0].ID, true); err != nil {
		t.Fatalf("SetStarred: %v", err)
	}

	stored, err = Store(database, Fetch(feedID, srv.URL+"/feed"))
	if err != nil {
		t.Fatalf("second Store: %v", err)
	}
	if stored.New != 0 {
		t.Fatalf("expected no new articles on re-fetch, got %d", stored.New)
	}
	articles, _ = database.ListArticles(feedID)
	if !articles[0].Read || !articles[0].Starred {
		t.Fatalf("re-fetch reset read/starred state: %+v", articles[0])
	}
}

func TestAllRecordsFailuresWithoutStoppingOtherFeeds(t *testing.T) {
	database := openDB(t)
	srv := feedServer(t)
	goodID, _ := database.AddFeed(srv.URL+"/feed", "Good", "")
	if _, err := database.AddFeed(srv.URL+"/broken", "Broken", ""); err != nil {
		t.Fatalf("AddFeed: %v", err)
	}
	feeds, err := database.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds: %v", err)
	}

	summary := All(database, feeds, 4)
	if summary.Feeds != 2 || summary.New != 2 {
		t.Fatalf("expected 2 feeds and 2 new articles, got %+v", summary)
	}
	if len(summary.Failed) != 1 || summary.Failed[0].Feed.Title != "Broken" || summary.Failed[0].Err == nil {
		t.Fatalf("expected the broken feed reported, got %+v", summary.Failed)
	}
	if n, _ := database.CountUnread(); n != 2 {
		t.Fatalf("expected the good feed's 2 articles stored unread, got %d", n)
	}
	if articles, _ := database.ListArticles(goodID); len(articles) != 2 {
		t.Fatalf("expected good feed stored, got %d", len(articles))
	}
}
