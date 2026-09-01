package db

import (
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
}

// articleColumns is the shared SELECT list. Kept in one place because
// scanArticle depends on the exact column order.
const articleColumns = `id, feed_id, guid, title, link, content, summary, image_url, published_at, read, starred`

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

// SetStarred saves or unsaves a single article.
func (db *DB) SetStarred(id int64, starred bool) error {
	v := 0
	if starred {
		v = 1
	}
	_, err := db.Exec(`UPDATE articles SET starred = ? WHERE id = ?`, v, id)
	return err
}

func (db *DB) UpsertArticle(a Article) error {
	_, err := db.Exec(`
		INSERT INTO articles (feed_id, guid, title, link, content, image_url, published_at, read)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(feed_id, guid) DO UPDATE SET
			title        = excluded.title,
			link         = excluded.link,
			content      = excluded.content,
			image_url    = CASE WHEN excluded.image_url != '' THEN excluded.image_url ELSE articles.image_url END,
			published_at = excluded.published_at
	`, a.FeedID, a.GUID, a.Title, a.Link, a.Content, a.ImageURL, a.PublishedAt.Unix())
	return err
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
	var publishedAt int64
	var read, starred int
	err := s.Scan(&a.ID, &a.FeedID, &a.GUID, &a.Title, &a.Link, &a.Content, &a.Summary, &a.ImageURL, &publishedAt, &read, &starred)
	if err != nil {
		return Article{}, err
	}
	a.PublishedAt = time.Unix(publishedAt, 0)
	a.Read = read != 0
	a.Starred = starred != 0
	return a, nil
}
