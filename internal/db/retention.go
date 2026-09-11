package db

import (
	"os"
	"path/filepath"
	"time"
)

// Nothing used to remove articles: every item ever fetched stayed in SQLite,
// and in the full-text index beside it, so a long-lived library only grew.
// Retention trims the part of that history nobody is going to read again.

// PruneReadArticles deletes read articles published before cutoff and reports
// how many rows went. The articles_fts delete trigger keeps the search index in
// step, so nothing extra is needed to stop pruned items turning up in search.
//
// Three kinds of article survive regardless of age, because each one represents
// something the user did rather than something a feed handed over:
//
//   - starred articles, which are the Saved view;
//   - articles with a stored AI summary, which cost a request to produce;
//   - unread articles, which the user has not had a chance to see yet.
//
// Articles with no publication date are also left alone. A missing date reads
// as the epoch, and pruning on that would empty a whole feed the first time it
// served an item without one.
func (db *DB) PruneReadArticles(cutoff time.Time) (int64, error) {
	res, err := db.Exec(`
		DELETE FROM articles
		WHERE read = 1
		  AND starred = 0
		  AND summary = ''
		  AND published_at > 0
		  AND published_at < ?
	`, cutoff.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Vacuum rebuilds the database file to hand freed pages back to the
// filesystem. SQLite does not shrink on DELETE alone, so without this a prune
// reclaims nothing visible on disk.
func (db *DB) Vacuum() error {
	_, err := db.Exec(`VACUUM`)
	return err
}

// ArticleCount is the number of stored articles, for the About screen.
func (db *DB) ArticleCount() (int64, error) {
	var n int64
	err := db.QueryRow(`SELECT COUNT(*) FROM articles`).Scan(&n)
	return n, err
}

// FileSize is the database's size on disk in bytes, walking the -wal and -shm
// sidecars so the figure matches what the filesystem actually holds. It returns
// 0 rather than an error when the path cannot be resolved: this only ever feeds
// a display line, and a missing number should not fail a screen.
func (db *DB) FileSize() int64 {
	dir, err := dataDir()
	if err != nil {
		return 0
	}
	var total int64
	for _, name := range []string{"rss.db", "rss.db-wal", "rss.db-shm"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
			total += info.Size()
		}
	}
	return total
}
