// Package refresh fetches local feeds and stores what they return. The TUI
// runs Fetch off the main loop and Store on it; the headless `tide --refresh`
// runs both through All.
package refresh

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"

	"github.com/allisonhere/tide/internal/db"
	"github.com/allisonhere/tide/internal/feed"
)

var converter = md.NewConverter("", true, nil)

// Fetched is one feed's fetch, ready to store.
type Fetched struct {
	FeedID      int64
	Title       string
	Description string
	FaviconURL  string
	Articles    []db.Article
	Result      *feed.FetchResult // always non-nil from Fetch
}

// Fetch downloads one feed and converts its items. On failure only FeedID and
// Result are set, and Result.Err says why.
func Fetch(feedID int64, url string) Fetched {
	result := feed.FetchFeed(url)
	f := Fetched{FeedID: feedID, Result: result}
	if !result.IsSuccess() {
		return f
	}
	f.Title = result.Feed.Title
	f.Description = result.Feed.Description
	f.FaviconURL = result.Feed.FaviconURL
	f.Articles = Articles(feedID, result.Feed)
	return f
}

// Articles turns a parsed feed's items into rows for feedID, rendering each
// item's HTML as markdown.
func Articles(feedID int64, parsed *feed.ParsedFeed) []db.Article {
	articles := make([]db.Article, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		content, _ := converter.ConvertString(item.Content)
		articles = append(articles, db.Article{
			FeedID:      feedID,
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Content:     content,
			ImageURL:    item.ImageURL,
			PublishedAt: item.PublishedAt,
			Author:      item.Author,
			Categories:  item.Categories,
			UpdatedAt:   item.UpdatedAt,
		})
	}
	return articles
}

// Stored reports what Store wrote.
type Stored struct {
	New          int    // articles that were not in the database before
	URLUpdatedTo string // the feed's new URL after a permanent redirect, or ""
}

// Store writes a successful fetch: its articles, the feed's title,
// description and favicon (keeping the stored ones when the feed sends
// none), the fetch time, and the new URL after a permanent redirect. New
// articles whose title matches a mute keyword are stored already read.
func Store(database *db.DB, f Fetched, mute []string) (Stored, error) {
	var stored Stored
	n, err := database.UpsertArticles(ApplyMutes(f.Articles, mute))
	if err != nil {
		return stored, fmt.Errorf("store articles: %w", err)
	}
	stored.New = n

	now := time.Now()
	var errs []error
	if current, err := database.GetFeed(f.FeedID); err == nil {
		title := firstNonBlank(f.Title, current.Title)
		description := firstNonBlank(f.Description, current.Description)
		favicon := firstNonBlank(f.FaviconURL, current.FaviconURL)
		if err := database.UpdateFeedMeta(f.FeedID, title, description, favicon, now); err != nil {
			errs = append(errs, fmt.Errorf("update feed details: %w", err))
		}
	} else if err := database.TouchFeedFetched(f.FeedID, now); err != nil {
		errs = append(errs, fmt.Errorf("record fetch time: %w", err))
	}
	if r := f.Result; r != nil && r.SuggestURLUpdate {
		if err := database.UpdateFeedURL(f.FeedID, r.SuggestedURL); err != nil {
			errs = append(errs, fmt.Errorf("URL update failed: %w", err))
		} else {
			stored.URLUpdatedTo = r.SuggestedURL
		}
	}
	return stored, errors.Join(errs...)
}

// ApplyMutes marks articles read when their title contains a mute keyword,
// ignoring case. Read only takes effect on insert, so an article already
// stored keeps whatever state it has.
func ApplyMutes(articles []db.Article, mute []string) []db.Article {
	if len(mute) == 0 {
		return articles
	}
	lowered := make([]string, len(mute))
	for i, k := range mute {
		lowered[i] = strings.ToLower(k)
	}
	for i := range articles {
		title := strings.ToLower(articles[i].Title)
		for _, k := range lowered {
			if strings.Contains(title, k) {
				articles[i].Read = true
				break
			}
		}
	}
	return articles
}

func firstNonBlank(s, fallback string) string {
	if s = strings.TrimSpace(s); s != "" {
		return s
	}
	return fallback
}

// FeedError is one feed that could not be refreshed.
type FeedError struct {
	Feed db.Feed
	Err  error
}

// Summary totals a refresh of many feeds.
type Summary struct {
	Feeds  int
	New    int
	Failed []FeedError
}

// All refreshes feeds with at most workers fetches in flight, muting as Store
// does. One feed's failure is recorded in the summary and does not stop the
// rest.
func All(database *db.DB, feeds []db.Feed, workers int, mute []string) Summary {
	workers = max(1, workers)
	jobs := make(chan db.Feed)
	var (
		mu      sync.Mutex
		summary = Summary{Feeds: len(feeds)}
		wg      sync.WaitGroup
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fd := range jobs {
				fetched := Fetch(fd.ID, fd.URL)
				var stored Stored
				err := fetched.Result.Err
				if fetched.Result.IsSuccess() {
					stored, err = Store(database, fetched, mute)
				} else if err == nil {
					err = errors.New(fetched.Result.FriendlyMessage())
				}
				mu.Lock()
				summary.New += stored.New
				if err != nil {
					summary.Failed = append(summary.Failed, FeedError{Feed: fd, Err: err})
				}
				mu.Unlock()
			}
		}()
	}
	for _, fd := range feeds {
		jobs <- fd
	}
	close(jobs)
	wg.Wait()
	return summary
}
