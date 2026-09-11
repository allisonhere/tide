package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// retentionCheckInterval is how often the app looks for articles to prune.
// Retention is measured in days, so checking a few times a day is plenty, and
// keeping it well clear of the refresh heartbeat means a prune never competes
// with a fetch for the single database connection.
const retentionCheckInterval = 6 * time.Hour

// retentionVacuumThreshold is how many rows a prune must remove before it is
// worth rebuilding the database file. VACUUM rewrites the whole file and holds
// the connection while it does, so a handful of rows is not worth the stall.
const retentionVacuumThreshold = 500

// ArticlesPrunedMsg reports the result of a retention sweep.
type ArticlesPrunedMsg struct {
	Removed int64
	Err     error
}

// retentionCutoff is the age at which a read article is dropped, or the zero
// time when retention is switched off.
func (m Model) retentionCutoff(now time.Time) time.Time {
	days := m.cfg.Feed.RetentionDays
	if days <= 0 {
		return time.Time{}
	}
	return now.AddDate(0, 0, -days)
}

// retentionDue reports whether enough time has passed to look again. The first
// check runs on the first heartbeat rather than at startup, so a large prune
// and its VACUUM cannot stall the first frames behind the one database
// connection.
func (m Model) retentionDue(now time.Time) bool {
	if m.cfg.Feed.RetentionDays <= 0 || m.db == nil {
		return false
	}
	return m.lastPrune.IsZero() || now.Sub(m.lastPrune) >= retentionCheckInterval
}

// pruneArticlesCmd deletes read articles past the retention window, and
// reclaims the freed pages when the sweep was big enough to be worth it.
func (m *Model) pruneArticlesCmd(now time.Time) tea.Cmd {
	cutoff := m.retentionCutoff(now)
	if cutoff.IsZero() || m.db == nil {
		return nil
	}
	m.lastPrune = now
	database := m.db
	return func() tea.Msg {
		removed, err := database.PruneReadArticles(cutoff)
		if err != nil {
			return ArticlesPrunedMsg{Err: err}
		}
		if removed >= retentionVacuumThreshold {
			if err := database.Vacuum(); err != nil {
				// The rows are already gone; failing to shrink the file is not
				// worth reporting as a failed prune.
				return ArticlesPrunedMsg{Removed: removed}
			}
		}
		return ArticlesPrunedMsg{Removed: removed}
	}
}

// handleArticlesPruned reports the sweep and reloads what it touched. A prune
// can remove rows the article pane is currently showing, so the visible list
// and the unread counts both have to come back from the database.
func (m *Model) handleArticlesPruned(msg ArticlesPrunedMsg) tea.Cmd {
	if msg.Err != nil {
		m.setStatus(fmt.Sprintf("could not apply retention: %v", msg.Err), true)
		return m.clearStatusCmd()
	}
	if msg.Removed == 0 {
		return nil
	}
	m.setStatus(fmt.Sprintf("retention: removed %s", pluralArticles(msg.Removed)), false)
	cmds := []tea.Cmd{m.loadFeedsCmd(), m.clearStatusCmd()}
	if len(m.filteredArticles) > 0 {
		m.restoreArticleID = m.filteredArticles[m.articleCursor].ID
	}
	return tea.Batch(cmds...)
}

func pluralArticles(n int64) string {
	if n == 1 {
		return "1 article"
	}
	return fmt.Sprintf("%d articles", n)
}

// formatBytes renders a byte count for the storage line in Settings.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < 3 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
