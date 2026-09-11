package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tide/internal/db"
)

// Feed fetching goes through a queue rather than straight to a command, for two
// reasons: a library of fifty feeds used to open fifty sockets the moment Tide
// started, and the background refresh would do the same thing every interval.
// The queue caps how many fetches are in flight and feeds the next one in as
// each result lands. -allie
const (
	// maxConcurrentRefresh is how many feeds may be fetched at once. Small
	// enough to be polite to a shared connection, large enough that a slow
	// server does not stall the whole library behind it.
	maxConcurrentRefresh = 4

	// autoRefreshHeartbeat is how often the background refresh wakes up to look
	// for due feeds. It is deliberately finer than any sane refresh interval:
	// due-ness is decided from each feed's own last-fetched time, so the
	// heartbeat only bounds how late a refresh can be, and a settings change
	// takes effect within a minute without restarting anything.
	autoRefreshHeartbeat = time.Minute
)

type refreshTarget struct {
	feedID int64
	url    string
	manual bool
}

// autoRefreshTickMsg wakes the background refresh.
type autoRefreshTickMsg struct{}

func autoRefreshTickCmd() tea.Cmd {
	return tea.Tick(autoRefreshHeartbeat, func(time.Time) tea.Msg { return autoRefreshTickMsg{} })
}

// autoRefreshInterval is the configured background refresh period, or 0 when
// the background refresh is switched off.
func (m Model) autoRefreshInterval() time.Duration {
	mins := m.cfg.Feed.RefreshIntervalMinutes
	if mins <= 0 {
		return 0
	}
	return time.Duration(mins) * time.Minute
}

// dueFeeds returns the local feeds whose last successful fetch is older than
// the refresh interval. A feed that has never been fetched is always due, and
// with the interval switched off every feed is due — which is what makes the
// startup refresh behave as it always has.
//
// Due-ness comes from the stored last-fetched time rather than a timer, so
// restarting Tide does not re-fetch a library that was refreshed a minute ago,
// and feeds naturally spread themselves across the interval instead of moving
// in lockstep.
func (m Model) dueFeeds(now time.Time) []db.Feed {
	interval := m.autoRefreshInterval()
	due := make([]db.Feed, 0, len(m.feeds))
	for _, f := range m.feeds {
		if m.isRemoteFeed(f.ID) {
			continue // remote feeds refresh through the source sync, not per-feed fetches
		}
		if interval > 0 && !f.LastFetched.IsZero() && now.Sub(f.LastFetched) < interval {
			continue
		}
		due = append(due, f)
	}
	return due
}

// enqueueRefresh queues local feeds for fetching and starts as many as the
// concurrency cap allows. Feeds already in flight or already queued are
// skipped, so overlapping triggers (startup, F, the heartbeat) cannot stack
// duplicate fetches for the same feed.
func (m *Model) enqueueRefresh(feeds []db.Feed, manual bool) tea.Cmd {
	for _, f := range feeds {
		if m.isRemoteFeed(f.ID) || m.refreshing[f.ID] || m.refreshQueued(f.ID) {
			continue
		}
		m.refreshQueue = append(m.refreshQueue, refreshTarget{feedID: f.ID, url: f.URL, manual: manual})
	}
	return m.pumpRefreshQueue()
}

func (m Model) refreshQueued(feedID int64) bool {
	for _, t := range m.refreshQueue {
		if t.feedID == feedID {
			return true
		}
	}
	return false
}

// pumpRefreshQueue starts queued fetches up to the concurrency cap. It is
// called whenever work is added and again as each fetch finishes, so the queue
// drains at a steady width instead of all at once.
func (m *Model) pumpRefreshQueue() tea.Cmd {
	var cmds []tea.Cmd
	for len(m.refreshQueue) > 0 && len(m.refreshing) < maxConcurrentRefresh {
		t := m.refreshQueue[0]
		m.refreshQueue = m.refreshQueue[1:]
		// refreshFeedCmd marks the feed in flight, which is what bounds this loop.
		cmds = append(cmds, m.refreshFeedCmd(t.feedID, t.url, t.manual))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// handleAutoRefreshTick re-arms the heartbeat and, when the background refresh
// is on, queues whatever has come due.
func (m *Model) handleAutoRefreshTick() tea.Cmd {
	cmds := []tea.Cmd{autoRefreshTickCmd()}
	// Retention rides the same heartbeat, and runs on its own schedule whether
	// or not the background refresh is switched on.
	now := time.Now()
	if m.retentionDue(now) {
		if cmd := m.pruneArticlesCmd(now); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if m.autoRefreshInterval() == 0 {
		return tea.Batch(cmds...)
	}
	if due := m.dueFeeds(now); len(due) > 0 {
		if cmd := m.enqueueRefresh(due, false); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	// A remote source has no per-feed URLs to fetch; reloading the feed list is
	// what pulls new items from it.
	if m.greaderClient != nil && m.sourceSyncDue(now) {
		m.lastSourceSync = now
		cmds = append(cmds, m.loadFeedsCmd())
	}
	return tea.Batch(cmds...)
}

// sourceSyncDue rate-limits remote source syncs to the same interval the local
// feeds use. The remote list has no per-feed fetch time to consult, so this one
// is tracked on the model.
func (m Model) sourceSyncDue(now time.Time) bool {
	interval := m.autoRefreshInterval()
	if interval == 0 {
		return false
	}
	return m.lastSourceSync.IsZero() || now.Sub(m.lastSourceSync) >= interval
}
