package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tide/internal/db"
	"github.com/allisonhere/tide/internal/feed"
	"github.com/allisonhere/tide/internal/refresh"
)

const usage = `Usage: tide [flag]

With no flag, tide opens the reader.

  --refresh        refresh every local feed, then exit
  --unread-count   print the number of unread articles
  --bar            print unread status as Waybar/Omarchy bar JSON
  --version, -v    print the version
  --help, -h       show this help

--refresh, --unread-count and --bar cover local feeds only, not a Google
Reader or FreshRSS source. They are safe to run while the reader is open.
`

// refreshWorkers matches the TUI's refresh queue: at most four fetches at once.
const refreshWorkers = 4

// runCommand handles the headless flags. handled is false when args name
// none of them, so the reader should start.
func runCommand(args []string, stdout, stderr io.Writer) (code int, handled bool) {
	for _, a := range args {
		switch a {
		case "--help", "-help", "-h":
			fmt.Fprint(stdout, usage)
			return 0, true
		case "--refresh":
			return withDB(stderr, func(database *db.DB) int { return refreshCommand(database, stdout, stderr) }), true
		case "--unread-count":
			return withDB(stderr, func(database *db.DB) int {
				n, err := database.CountUnread()
				if err != nil {
					fmt.Fprintln(stderr, "error counting unread articles:", err)
					return 1
				}
				fmt.Fprintln(stdout, n)
				return 0
			}), true
		case "--bar":
			return withDB(stderr, func(database *db.DB) int {
				feeds, err := database.ListFeeds()
				if err != nil {
					fmt.Fprintln(stderr, "error listing feeds:", err)
					return 1
				}
				out, _ := json.Marshal(barStatus(feeds))
				fmt.Fprintln(stdout, string(out))
				return 0
			}), true
		}
	}
	return 0, false
}

func withDB(stderr io.Writer, fn func(*db.DB) int) int {
	database, err := db.Open()
	if err != nil {
		fmt.Fprintln(stderr, "error opening database:", err)
		return 1
	}
	defer database.Close()
	return fn(database)
}

func refreshCommand(database *db.DB, stdout, stderr io.Writer) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, "warning: could not load config:", err)
		cfg = config.DefaultConfig()
	}
	feed.SetMaxFeedBodyBytes(cfg.Feed.MaxBodyMiB << 20)

	feeds, err := database.ListFeeds()
	if err != nil {
		fmt.Fprintln(stderr, "error listing feeds:", err)
		return 1
	}
	summary := refresh.All(database, feeds, refreshWorkers, cfg.Feed.MuteKeywords)
	for _, f := range summary.Failed {
		fmt.Fprintf(stderr, "%s: %v\n", f.Feed.Title, f.Err)
	}
	fmt.Fprintf(stdout, "refreshed %d feeds, %d new, %d failed\n", summary.Feeds, summary.New, len(summary.Failed))
	return 0
}

// barJSON is the Waybar custom-module format, which the Omarchy bar's command
// modules also read.
type barJSON struct {
	Text    string `json:"text"`
	Tooltip string `json:"tooltip"`
	Class   string `json:"class"`
}

// barTooltipFeeds caps how many feeds the tooltip lists.
const barTooltipFeeds = 5

// barStatus summarises unread counts for a status bar. The text is empty when
// nothing is unread, which hides the module.
func barStatus(feeds []db.Feed) barJSON {
	var total int64
	unread := make([]db.Feed, 0, len(feeds))
	for _, f := range feeds {
		if f.UnreadCount > 0 {
			total += f.UnreadCount
			unread = append(unread, f)
		}
	}
	if total == 0 {
		return barJSON{Tooltip: "Tide: all caught up", Class: "empty"}
	}
	sort.SliceStable(unread, func(i, j int) bool { return unread[i].UnreadCount > unread[j].UnreadCount })
	tooltip := fmt.Sprintf("Tide: %d unread", total)
	for i, f := range unread {
		if i == barTooltipFeeds {
			tooltip += fmt.Sprintf("\n…and %d more feeds", len(unread)-barTooltipFeeds)
			break
		}
		tooltip += fmt.Sprintf("\n%s  %d", f.Title, f.UnreadCount)
	}
	return barJSON{Text: fmt.Sprintf(" %d", total), Tooltip: tooltip, Class: "unread"}
}
