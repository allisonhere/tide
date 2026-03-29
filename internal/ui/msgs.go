package ui

import (
	"tide/internal/db"
	"tide/internal/feed"
)

type FeedsLoadedMsg struct{ Feeds []db.Feed }
type ArticlesLoadedMsg struct {
	FeedID   int64
	Articles []db.Article
}
type FeedRefreshedMsg struct {
	FeedID   int64
	Title    string
	Articles []db.Article
	Err      error
	Result   *feed.FetchResult // full result; always non-nil when coming from refreshFeedCmd
	Manual   bool              // true when triggered by user keypress (f), not startup auto-refresh
}
type FeedSavedMsg struct {
	Feed db.Feed
	Err  error
}
type FeedDeletedMsg struct {
	FeedID int64
	Err    error
}
type OPMLImportedMsg struct {
	Count int
	Err   error
}
type OPMLExportedMsg struct {
	Path string
	Err  error
}
type ArticleContentFetchedMsg struct {
	ArticleID int64
	Content   string
	Err       error
}
type FeedURLUpdatedMsg struct {
	FeedID int64
	NewURL string
	Err    error
}
type AISummaryFetchedMsg struct {
	ArticleID int64
	Summary   string
	Err       error
}
type SummarySavedMsg struct {
	Path string
	Err  error
}
type ClipboardCopiedMsg struct {
	Err error
}
type StatusClearMsg struct{}
type ErrMsg struct{ Err error }
