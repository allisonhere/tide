package ui

import (
	"tide/internal/config"
	"tide/internal/db"
	"tide/internal/feed"
	"tide/internal/update"
)

type FeedsLoadedMsg struct {
	Feeds         []db.Feed
	Folders       []db.Folder
	RemoteStreams map[int64]string
	Err           error
}
type ArticlesLoadedMsg struct {
	FeedID   int64
	Articles []db.Article
	Err      error
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
type RemoteFeedAddedMsg struct {
	StreamID  string
	Title     string
	FeedCount int
	SettingsOnly bool
	Source    config.SourceConfig
	Err       error
}
type FeedDeletedMsg struct {
	FeedID int64
	Err    error
}
type FolderSavedMsg struct {
	Folder db.Folder
	Err    error
}
type FolderDeletedMsg struct {
	FolderID int64
	Err      error
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
type ArticleReadUpdatedMsg struct {
	ArticleID int64
	Read      bool
	Advance   bool
	Err       error
}
type UpdateCheckedMsg struct {
	Result update.CheckResult
	Manual bool
	Err    error
}
type UpdateDownloadedMsg struct {
	Asset update.DownloadedAsset
	Err   error
}
type UpdateInstalledMsg struct {
	Result update.InstallResult
	Err    error
}
type RestartedMsg struct {
	Err error
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
