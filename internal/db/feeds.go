package db

import (
	"fmt"
	"time"
)

type Feed struct {
	ID          int64
	URL         string
	Title       string
	Description string
	FaviconURL  string
	LastFetched time.Time
	UnreadCount int64
}

func (db *DB) ListFeeds() ([]Feed, error) {
	rows, err := db.Query(`
		SELECT f.id, f.url, f.title, f.description, f.favicon_url, f.last_fetched,
		       COUNT(CASE WHEN a.read = 0 THEN 1 END) AS unread_count
		FROM feeds f
		LEFT JOIN articles a ON a.feed_id = f.id
		GROUP BY f.id
		ORDER BY f.title COLLATE NOCASE
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var f Feed
		var lastFetched int64
		if err := rows.Scan(&f.ID, &f.URL, &f.Title, &f.Description, &f.FaviconURL, &lastFetched, &f.UnreadCount); err != nil {
			return nil, err
		}
		f.LastFetched = time.Unix(lastFetched, 0)
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (db *DB) GetFeed(id int64) (Feed, error) {
	var f Feed
	var lastFetched int64
	err := db.QueryRow(
		`SELECT id, url, title, description, favicon_url, last_fetched FROM feeds WHERE id = ?`, id,
	).Scan(&f.ID, &f.URL, &f.Title, &f.Description, &f.FaviconURL, &lastFetched)
	if err != nil {
		return Feed{}, err
	}
	f.LastFetched = time.Unix(lastFetched, 0)
	return f, nil
}

func (db *DB) AddFeed(url, title, description string) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO feeds (url, title, description) VALUES (?, ?, ?)`,
		url, title, description,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) UpdateFeed(id int64, title, url string) error {
	_, err := db.Exec(`UPDATE feeds SET title = ?, url = ? WHERE id = ?`, title, url, id)
	return err
}

func (db *DB) UpdateFeedMeta(id int64, title, description, faviconURL string, lastFetched time.Time) error {
	_, err := db.Exec(
		`UPDATE feeds SET title = ?, description = ?, favicon_url = ?, last_fetched = ? WHERE id = ?`,
		title, description, faviconURL, lastFetched.Unix(), id,
	)
	return err
}

func (db *DB) TouchFeedFetched(id int64, lastFetched time.Time) error {
	_, err := db.Exec(`UPDATE feeds SET last_fetched = ? WHERE id = ?`, lastFetched.Unix(), id)
	return err
}

func (db *DB) DeleteFeed(id int64) error {
	res, err := db.Exec(`DELETE FROM feeds WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("feed %d not found", id)
	}
	return nil
}
