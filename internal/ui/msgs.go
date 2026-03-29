package ui

import "tide/internal/db"

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
type StatusClearMsg struct{}
type ErrMsg struct{ Err error }
