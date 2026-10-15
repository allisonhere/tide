package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	home, _ := os.UserHomeDir()
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".local", "share")
	}
	path := filepath.Join(xdg, "rss", "rss.db")
	fmt.Println("DB path:", path)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		fmt.Println("open error:", err)
		return
	}
	defer db.Close()

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		fmt.Println("pragma error:", err)
	}

	rows, err := db.Query(`
		SELECT f.id, f.url, f.title, f.last_fetched,
		       COUNT(CASE WHEN a.read=0 THEN 1 END) as unread
		FROM feeds f LEFT JOIN articles a ON a.feed_id = f.id
		GROUP BY f.id ORDER BY f.title COLLATE NOCASE
	`)
	if err != nil {
		fmt.Println("query error:", err)
		return
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, lastFetched, unread int64
		var url, title string
		if err := rows.Scan(&id, &url, &title, &lastFetched, &unread); err != nil {
			fmt.Println("scan error:", err)
			continue
		}
		fmt.Printf("  feed id=%d title=%q url=%q last_fetched=%d unread=%d\n", id, title, url, lastFetched, unread)
		n++
	}
	fmt.Printf("total feeds: %d, rows.Err: %v\n", n, rows.Err())

	// Check articles
	var acount int
	if err := db.QueryRow("SELECT COUNT(*) FROM articles").Scan(&acount); err != nil {
		fmt.Println("count error:", err)
	}
	fmt.Printf("total articles in DB: %d\n", acount)
}
