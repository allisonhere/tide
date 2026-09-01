package db

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB

	// ftsAvailable reports whether the articles_fts full-text index exists and
	// is usable. It is false on SQLite builds compiled without FTS5, in which
	// case SearchArticles falls back to a LIKE scan.
	ftsAvailable bool
}

func Open() (*DB, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, fmt.Errorf("data dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	path := filepath.Join(dir, "rss.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	conn.SetMaxOpenConns(1)

	db := &DB{DB: conn}
	if err := db.init(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("init db: %w", err)
	}
	if err := db.migrateLegacyConfigDB(path); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate legacy db: %w", err)
	}
	return db, nil
}

func (db *DB) init() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return err
		}
	}
	if err := db.migrate(); err != nil {
		return err
	}
	return db.migrateSchema()
}

// latestSchemaVersion is the PRAGMA user_version a fully migrated database
// reports. Bump it whenever a new migration block is added below.
const latestSchemaVersion = 9

// migrateSchema applies incremental ALTER TABLE migrations tracked by
// PRAGMA user_version so they are applied exactly once.
func (db *DB) migrateSchema() error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version < 1 {
		if _, err := db.Exec(`ALTER TABLE articles ADD COLUMN summary TEXT NOT NULL DEFAULT ''`); err != nil {
			// Ignore duplicate-column errors from a previously interrupted migration.
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
		if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
			return err
		}
	}
	if version < 2 {
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS folders (
				id       INTEGER PRIMARY KEY AUTOINCREMENT,
				name     TEXT    NOT NULL UNIQUE,
				position INTEGER NOT NULL DEFAULT 0
			)
		`); err != nil {
			return err
		}
		if _, err := db.Exec(`ALTER TABLE feeds ADD COLUMN folder_id INTEGER REFERENCES folders(id) ON DELETE SET NULL`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
		if _, err := db.Exec(`PRAGMA user_version = 2`); err != nil {
			return err
		}
	}
	if version < 3 {
		if _, err := db.Exec(`ALTER TABLE folders ADD COLUMN color TEXT NOT NULL DEFAULT ''`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
		if _, err := db.Exec(`PRAGMA user_version = 3`); err != nil {
			return err
		}
	}
	if version < 4 {
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS remote_feed_prefs (
				remote_feed_id INTEGER PRIMARY KEY,
				folder_id      INTEGER REFERENCES folders(id) ON DELETE SET NULL
			)
		`); err != nil {
			return err
		}
		if _, err := db.Exec(`PRAGMA user_version = 4`); err != nil {
			return err
		}
	}
	if version < 5 {
		if _, err := db.Exec(`ALTER TABLE remote_feed_prefs ADD COLUMN title TEXT NOT NULL DEFAULT ''`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
		if _, err := db.Exec(`PRAGMA user_version = 5`); err != nil {
			return err
		}
	}
	if version < 6 {
		if _, err := db.Exec(`ALTER TABLE feeds ADD COLUMN custom_title TEXT NOT NULL DEFAULT ''`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
		if _, err := db.Exec(`PRAGMA user_version = 6`); err != nil {
			return err
		}
	}
	if version < 7 {
		if err := db.migrateArticlesFTS(); err != nil {
			return err
		}
		if _, err := db.Exec(`PRAGMA user_version = 7`); err != nil {
			return err
		}
	}
	if version < 8 {
		if _, err := db.Exec(`ALTER TABLE articles ADD COLUMN starred INTEGER NOT NULL DEFAULT 0`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
		// Partial index: the Saved view and `is:starred` only ever ask for
		// starred = 1, and starred rows are a small fraction of the table.
		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_articles_starred ON articles(starred, published_at DESC) WHERE starred = 1`); err != nil {
			return err
		}
		if _, err := db.Exec(`PRAGMA user_version = 8`); err != nil {
			return err
		}
	}
	if version < 9 {
		if _, err := db.Exec(`ALTER TABLE articles ADD COLUMN image_url TEXT NOT NULL DEFAULT ''`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
		if _, err := db.Exec(`PRAGMA user_version = 9`); err != nil {
			return err
		}
	}
	// Later runs skip the migration block entirely, so probe the index to learn
	// whether searching can use it.
	db.ftsAvailable = db.probeFTS()
	return nil
}

// migrateArticlesFTS builds the full-text index over articles and the triggers
// that keep it in sync.
//
// The table is external-content (content='articles'), so it stores only terms
// and never duplicates article bodies. That form requires the 'delete' command
// syntax in the delete/update triggers — a plain DELETE FROM articles_fts would
// silently corrupt the index.
//
// A SQLite build without FTS5 is not fatal: the index is skipped and search
// falls back to a LIKE scan. The migration still records as applied so the app
// does not retry on every launch.
func (db *DB) migrateArticlesFTS() error {
	// The index spans summary, but a database that reached user_version >= 1 by
	// a path that never ran the summary migration will not have that column —
	// building the index over it would fail at 'rebuild' with "no such column".
	// Add it defensively, reusing the duplicate-column tolerance used above.
	if _, err := db.Exec(`ALTER TABLE articles ADD COLUMN summary TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}

	if _, err := db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(
			title, content, summary,
			content='articles',
			content_rowid='id',
			tokenize='porter unicode61'
		)
	`); err != nil {
		// No FTS5 in this build — degrade instead of bricking startup.
		db.ftsAvailable = false
		return nil
	}

	for _, stmt := range []string{
		`CREATE TRIGGER IF NOT EXISTS articles_fts_ai AFTER INSERT ON articles BEGIN
			INSERT INTO articles_fts(rowid, title, content, summary)
			VALUES (new.id, new.title, new.content, new.summary);
		END`,
		`CREATE TRIGGER IF NOT EXISTS articles_fts_ad AFTER DELETE ON articles BEGIN
			INSERT INTO articles_fts(articles_fts, rowid, title, content, summary)
			VALUES ('delete', old.id, old.title, old.content, old.summary);
		END`,
		`CREATE TRIGGER IF NOT EXISTS articles_fts_au AFTER UPDATE ON articles BEGIN
			INSERT INTO articles_fts(articles_fts, rowid, title, content, summary)
			VALUES ('delete', old.id, old.title, old.content, old.summary);
			INSERT INTO articles_fts(rowid, title, content, summary)
			VALUES (new.id, new.title, new.content, new.summary);
		END`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	// Backfill rows that predate the index. 'rebuild' is idempotent, so a
	// re-run after an interrupted migration is safe.
	if _, err := db.Exec(`INSERT INTO articles_fts(articles_fts) VALUES('rebuild')`); err != nil {
		return err
	}
	db.ftsAvailable = true
	return nil
}

// probeFTS reports whether articles_fts exists and answers queries.
func (db *DB) probeFTS() bool {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM articles_fts WHERE articles_fts MATCH 'tide_fts_probe'`).Scan(&n)
	return err == nil
}

func (db *DB) migrate() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS feeds (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			url          TEXT    NOT NULL UNIQUE,
			title        TEXT    NOT NULL DEFAULT '',
			description  TEXT    NOT NULL DEFAULT '',
			favicon_url  TEXT    NOT NULL DEFAULT '',
			last_fetched INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS articles (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			feed_id      INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
			guid         TEXT    NOT NULL,
			title        TEXT    NOT NULL DEFAULT '',
			link         TEXT    NOT NULL DEFAULT '',
			content      TEXT    NOT NULL DEFAULT '',
			published_at INTEGER NOT NULL DEFAULT 0,
			read         INTEGER NOT NULL DEFAULT 0,
			UNIQUE(feed_id, guid)
		);

		CREATE INDEX IF NOT EXISTS idx_articles_feed_id ON articles(feed_id);
		CREATE INDEX IF NOT EXISTS idx_articles_read    ON articles(read);
	`)
	return err
}

func (db *DB) migrateLegacyConfigDB(dataPath string) error {
	legacyPath, err := legacyConfigDBPath()
	if err != nil {
		return nil
	}
	if legacyPath == dataPath {
		return nil
	}
	if _, err := os.Stat(legacyPath + ".migrated"); err == nil {
		return nil
	}
	if _, err := os.Stat(legacyPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		if err := copyFile(legacyPath, dataPath); err != nil {
			return err
		}
		_ = os.Rename(legacyPath, legacyPath+".migrated")
		return nil
	}

	legacyConn, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		return err
	}
	defer legacyConn.Close()

	if _, err := legacyConn.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return err
	}

	rows, err := legacyConn.Query(`
		SELECT url, title, description, favicon_url, last_fetched
		FROM feeds
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type legacyFeed struct {
		ID          int64
		URL         string
		Title       string
		Description string
		FaviconURL  string
		LastFetched int64
	}

	urlToID := map[string]int64{}
	for rows.Next() {
		var f legacyFeed
		if err := rows.Scan(&f.URL, &f.Title, &f.Description, &f.FaviconURL, &f.LastFetched); err != nil {
			return err
		}
		if _, err := db.Exec(`
			INSERT OR IGNORE INTO feeds (url, title, description, favicon_url, last_fetched)
			VALUES (?, ?, ?, ?, ?)
		`, f.URL, f.Title, f.Description, f.FaviconURL, f.LastFetched); err != nil {
			return err
		}
		if _, err := db.Exec(`
			UPDATE feeds
			SET title = CASE WHEN title = '' THEN ? ELSE title END,
			    description = CASE WHEN description = '' THEN ? ELSE description END,
			    favicon_url = CASE WHEN favicon_url = '' THEN ? ELSE favicon_url END,
			    last_fetched = CASE WHEN last_fetched = 0 THEN ? ELSE last_fetched END
			WHERE url = ?
		`, f.Title, f.Description, f.FaviconURL, f.LastFetched, f.URL); err != nil {
			return err
		}

		var id int64
		if err := db.QueryRow(`SELECT id FROM feeds WHERE url = ?`, f.URL).Scan(&id); err != nil {
			return err
		}
		urlToID[f.URL] = id
	}
	if err := rows.Err(); err != nil {
		return err
	}

	articleRows, err := legacyConn.Query(`
		SELECT f.url, a.guid, a.title, a.link, a.content, a.published_at, a.read
		FROM articles a
		JOIN feeds f ON f.id = a.feed_id
	`)
	if err != nil {
		return nil
	}
	defer articleRows.Close()

	for articleRows.Next() {
		var feedURL, guid, title, link, content string
		var publishedAt int64
		var read int
		if err := articleRows.Scan(&feedURL, &guid, &title, &link, &content, &publishedAt, &read); err != nil {
			return err
		}
		feedID, ok := urlToID[feedURL]
		if !ok {
			continue
		}
		if _, err := db.Exec(`
			INSERT OR IGNORE INTO articles (feed_id, guid, title, link, content, published_at, read)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, feedID, guid, title, link, content, publishedAt, read); err != nil {
			return err
		}
	}
	if err := articleRows.Err(); err != nil {
		return err
	}

	// Rename legacy DB so it is not re-imported on next startup.
	_ = os.Rename(legacyPath, legacyPath+".migrated")
	return nil
}

func dataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(xdg, "rss"), nil
}

func legacyConfigDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "rss", "rss.db"), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return nil
}
