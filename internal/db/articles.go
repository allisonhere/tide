package db

import (
	"strings"
	"time"
)

type Article struct {
	ID          int64
	FeedID      int64
	GUID        string
	Title       string
	Link        string
	Content     string
	Summary     string
	ImageURL    string
	PublishedAt time.Time
	Read        bool
	// Starred marks an article the user saved for later. It is deliberately
	// independent of Read: saving is about intent to return, not about whether
	// the article has been opened.
	Starred bool
	// Author is a single display name ("" when the feed gives none).
	Author string
	// Categories are the feed-supplied tags for the item, trimmed and
	// de-duplicated. Stored newline-joined in a single TEXT column.
	Categories []string
	// UpdatedAt is the item's last-modified time, distinct from PublishedAt.
	// Zero when the feed gives none.
	UpdatedAt time.Time
}

// articleColumns is the shared SELECT list. Kept in one place because
// scanArticle depends on the exact column order.
const articleColumns = `id, feed_id, guid, title, link, content, summary, image_url, published_at, read, starred, author, categories, updated_at`

// joinCategories / splitCategories move the Categories slice in and out of its
// single newline-separated TEXT column.
func joinCategories(c []string) string { return strings.Join(c, "\n") }

func splitCategories(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (db *DB) ListArticles(feedID int64) ([]Article, error) {
	rows, err := db.Query(`
		SELECT `+articleColumns+`
		FROM articles
		WHERE feed_id = ?
		ORDER BY published_at DESC
		LIMIT 100
	`, feedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

func (db *DB) ListUnreadArticles(feedID int64) ([]Article, error) {
	rows, err := db.Query(`
		SELECT `+articleColumns+`
		FROM articles
		WHERE feed_id = ? AND read = 0
		ORDER BY published_at DESC
		LIMIT 100
	`, feedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// defaultStarredLimit caps the Saved view. It is higher than the per-feed limit
// because Saved spans the whole library and is the one list a user curates by
// hand rather than one that grows on its own.
const defaultStarredLimit = 500

// ListStarredArticles returns saved articles across every local feed, newest
// first.
//
// Remote (Google Reader) articles never reach the articles table, so they can
// not appear here; the UI blocks starring them rather than silently dropping
// them.
func (db *DB) ListStarredArticles(limit int) ([]Article, error) {
	if limit <= 0 {
		limit = defaultStarredLimit
	}
	rows, err := db.Query(`
		SELECT `+articleColumns+`
		FROM articles
		WHERE starred = 1
		ORDER BY published_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// CountStarred is the badge number for the Saved virtual feed.
func (db *DB) CountStarred() (int64, error) {
	var n int64
	err := db.QueryRow(`SELECT COUNT(*) FROM articles WHERE starred = 1`).Scan(&n)
	return n, err
}

// CountUnread is the number of unread articles across every local feed.
func (db *DB) CountUnread() (int64, error) {
	var n int64
	err := db.QueryRow(`SELECT COUNT(*) FROM articles WHERE read = 0`).Scan(&n)
	return n, err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SetStarred saves or unsaves a single article.
func (db *DB) SetStarred(id int64, starred bool) error {
	v := 0
	if starred {
		v = 1
	}
	_, err := db.Exec(`UPDATE articles SET starred = ? WHERE id = ?`, v, id)
	return err
}

// UpsertArticle stores one article; see UpsertArticles.
func (db *DB) UpsertArticle(a Article) error {
	_, err := db.UpsertArticles([]Article{a})
	return err
}

// UpsertArticles stores fetched articles in one transaction and reports how
// many were new. A new row takes a.Read, so a caller can store an article
// already read; an existing row keeps its read, starred and summary state and
// only takes the refreshed fields.
func (db *DB) UpsertArticles(articles []Article) (inserted int, err error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	for _, a := range articles {
		res, err := tx.Exec(`
			INSERT INTO articles (feed_id, guid, title, link, content, image_url, published_at, read, author, categories, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(feed_id, guid) DO NOTHING
		`, a.FeedID, a.GUID, a.Title, a.Link, a.Content, a.ImageURL, a.PublishedAt.Unix(),
			boolToInt(a.Read), a.Author, joinCategories(a.Categories), articleUpdatedUnix(a.UpdatedAt))
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			inserted++
			continue
		}
		if _, err := tx.Exec(`
			UPDATE articles SET
				title        = ?,
				link         = ?,
				content      = ?,
				image_url    = CASE WHEN ? != '' THEN ? ELSE image_url END,
				published_at = ?,
				author       = ?,
				categories   = ?,
				updated_at   = ?
			WHERE feed_id = ? AND guid = ?
		`, a.Title, a.Link, a.Content, a.ImageURL, a.ImageURL, a.PublishedAt.Unix(),
			a.Author, joinCategories(a.Categories), articleUpdatedUnix(a.UpdatedAt), a.FeedID, a.GUID); err != nil {
			return 0, err
		}
	}
	return inserted, tx.Commit()
}

func (db *DB) MarkRead(id int64, read bool) error {
	v := 0
	if read {
		v = 1
	}
	_, err := db.Exec(`UPDATE articles SET read = ? WHERE id = ?`, v, id)
	return err
}

func (db *DB) MarkAllRead(feedID int64) error {
	_, err := db.Exec(`UPDATE articles SET read = 1 WHERE feed_id = ?`, feedID)
	return err
}

func (db *DB) UpdateArticleContent(id int64, content string) error {
	_, err := db.Exec(`UPDATE articles SET content = ? WHERE id = ?`, content, id)
	return err
}

func (db *DB) SaveSummary(id int64, summary string) error {
	_, err := db.Exec(`UPDATE articles SET summary = ? WHERE id = ?`, summary, id)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanArticle(s scanner) (Article, error) {
	var a Article
	var publishedAt, updatedAt int64
	var read, starred int
	var categories string
	err := s.Scan(&a.ID, &a.FeedID, &a.GUID, &a.Title, &a.Link, &a.Content, &a.Summary, &a.ImageURL, &publishedAt, &read, &starred, &a.Author, &categories, &updatedAt)
	if err != nil {
		return Article{}, err
	}
	a.PublishedAt = time.Unix(publishedAt, 0)
	a.Read = read != 0
	a.Starred = starred != 0
	a.Categories = splitCategories(categories)
	if updatedAt > 0 {
		a.UpdatedAt = time.Unix(updatedAt, 0)
	}
	return a, nil
}

// articleUpdatedUnix stores a zero UpdatedAt as 0 rather than a negative epoch.
func articleUpdatedUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
