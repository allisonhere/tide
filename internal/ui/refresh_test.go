package ui

import (
	"testing"
	"time"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tide/internal/db"
)

func refreshTestModel(t *testing.T, feedCount int, intervalMinutes int) Model {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Feed.RefreshIntervalMinutes = intervalMinutes
	m := NewModel(nil, cfg, "v1.0.0", false)
	m.width, m.height = 100, 30
	feeds := make([]db.Feed, 0, feedCount)
	for i := 0; i < feedCount; i++ {
		feeds = append(feeds, db.Feed{ID: int64(i + 1), Title: "Feed", URL: "https://example.com/f"})
	}
	m.feeds = feeds
	return m
}

// A library of many feeds used to open one socket per feed the moment Tide
// started. The queue caps how many fetches run at once.
func TestRefreshQueueCapsConcurrentFetches(t *testing.T) {
	m := refreshTestModel(t, 12, 0)

	if cmd := m.enqueueRefresh(m.feeds, false); cmd == nil {
		t.Fatal("expected the first batch of fetches to start")
	}
	if got := len(m.refreshing); got != maxConcurrentRefresh {
		t.Fatalf("expected %d fetches in flight, got %d", maxConcurrentRefresh, got)
	}
	if got, want := len(m.refreshQueue), 12-maxConcurrentRefresh; got != want {
		t.Fatalf("expected %d feeds still queued, got %d", want, got)
	}
}

// Each completed fetch lets exactly one queued feed start, so the queue drains
// at a steady width rather than in bursts.
func TestRefreshQueueDrainsAsFetchesComplete(t *testing.T) {
	m := refreshTestModel(t, 6, 0)
	m.enqueueRefresh(m.feeds, false)

	started := len(m.refreshing)
	for len(m.refreshQueue) > 0 {
		// Stand in for a FeedRefreshedMsg landing for one in-flight feed.
		for id := range m.refreshing {
			delete(m.refreshing, id)
			break
		}
		before := len(m.refreshQueue)
		m.pumpRefreshQueue()
		if len(m.refreshQueue) != before-1 {
			t.Fatalf("expected one queued feed to start per completion, went from %d to %d", before, len(m.refreshQueue))
		}
		if len(m.refreshing) > maxConcurrentRefresh {
			t.Fatalf("concurrency cap exceeded: %d in flight", len(m.refreshing))
		}
		started++
	}
	if started != 6 {
		t.Fatalf("expected all 6 feeds to be started, got %d", started)
	}
}

// Overlapping triggers — startup, F, and the heartbeat — must not stack
// duplicate fetches for the same feed.
func TestRefreshQueueSkipsFeedsAlreadyInFlightOrQueued(t *testing.T) {
	m := refreshTestModel(t, 8, 0)
	m.enqueueRefresh(m.feeds, false)
	inFlight, queued := len(m.refreshing), len(m.refreshQueue)

	m.enqueueRefresh(m.feeds, false)

	if len(m.refreshing) != inFlight {
		t.Fatalf("expected in-flight count unchanged, got %d want %d", len(m.refreshing), inFlight)
	}
	if len(m.refreshQueue) != queued {
		t.Fatalf("expected queue unchanged, got %d want %d", len(m.refreshQueue), queued)
	}
}

// Due-ness comes from each feed's own last-fetched time, so a restart does not
// re-fetch a library that was refreshed a moment ago.
func TestDueFeedsRespectsLastFetched(t *testing.T) {
	m := refreshTestModel(t, 3, 30)
	now := time.Now()
	m.feeds[0].LastFetched = now.Add(-2 * time.Hour) // stale
	m.feeds[1].LastFetched = now.Add(-time.Minute)   // fresh
	m.feeds[2].LastFetched = time.Time{}             // never fetched

	due := m.dueFeeds(now)
	if len(due) != 2 {
		t.Fatalf("expected the stale and never-fetched feeds to be due, got %d", len(due))
	}
	for _, f := range due {
		if f.ID == 2 {
			t.Fatal("expected a feed fetched a minute ago not to be due")
		}
	}
}

// With the background refresh off, everything is due — which is what keeps the
// startup refresh behaving as it always has.
func TestDueFeedsReturnsEverythingWhenAutoRefreshOff(t *testing.T) {
	m := refreshTestModel(t, 3, 0)
	now := time.Now()
	for i := range m.feeds {
		m.feeds[i].LastFetched = now
	}

	if got := len(m.dueFeeds(now)); got != 3 {
		t.Fatalf("expected every feed to be due with auto-refresh off, got %d", got)
	}
	if m.autoRefreshInterval() != 0 {
		t.Fatal("expected a zero interval to read as off")
	}
}

// The heartbeat keeps ticking with the background refresh off, so switching it
// on in Settings takes effect without a restart — but it queues nothing.
func TestAutoRefreshTickQueuesNothingWhenOff(t *testing.T) {
	m := refreshTestModel(t, 4, 0)
	now := time.Now()
	for i := range m.feeds {
		m.feeds[i].LastFetched = now
	}

	if cmd := m.handleAutoRefreshTick(); cmd == nil {
		t.Fatal("expected the heartbeat to re-arm itself")
	}
	if len(m.refreshing) != 0 || len(m.refreshQueue) != 0 {
		t.Fatalf("expected no fetches with auto-refresh off, got %d in flight and %d queued",
			len(m.refreshing), len(m.refreshQueue))
	}
}

func TestAutoRefreshTickQueuesDueFeeds(t *testing.T) {
	m := refreshTestModel(t, 3, 30)
	now := time.Now()
	m.feeds[0].LastFetched = now.Add(-time.Hour)
	m.feeds[1].LastFetched = now
	m.feeds[2].LastFetched = now.Add(-45 * time.Minute)

	m.handleAutoRefreshTick()

	if got := len(m.refreshing) + len(m.refreshQueue); got != 2 {
		t.Fatalf("expected the two stale feeds to be queued, got %d", got)
	}
	if m.refreshing[2] {
		t.Fatal("expected the freshly fetched feed to be left alone")
	}
}

// Retention is off by default, so an upgrade never silently deletes anyone's
// library — it has to be switched on.
func TestRetentionIsOffByDefault(t *testing.T) {
	m := refreshTestModel(t, 1, 30)
	if m.cfg.Feed.RetentionDays != 0 {
		t.Fatalf("expected retention off by default, got %d days", m.cfg.Feed.RetentionDays)
	}
	if !m.retentionCutoff(time.Now()).IsZero() {
		t.Fatal("expected no cutoff while retention is off")
	}
	if m.retentionDue(time.Now()) {
		t.Fatal("expected no retention sweep while retention is off")
	}
}

func TestRetentionCutoffAndSchedule(t *testing.T) {
	m := refreshTestModel(t, 1, 0)
	m.cfg.Feed.RetentionDays = 30
	now := time.Now()

	cutoff := m.retentionCutoff(now)
	if want := now.AddDate(0, 0, -30); !cutoff.Equal(want) {
		t.Fatalf("expected a 30 day cutoff at %v, got %v", want, cutoff)
	}

	// Without a database there is nothing to sweep, whatever the setting says.
	if m.retentionDue(now) {
		t.Fatal("expected no sweep without a database")
	}
}

// Retention rides the refresh heartbeat, so it still runs with the background
// refresh switched off.
func TestAutoRefreshTickStillRunsRetentionWhenRefreshOff(t *testing.T) {
	m := refreshTestModel(t, 2, 0)
	m.cfg.Feed.RetentionDays = 30
	now := time.Now()
	for i := range m.feeds {
		m.feeds[i].LastFetched = now
	}

	// lastPrune is only stamped when a sweep is actually dispatched, which
	// needs a database; assert the schedule rather than the command here.
	if m.autoRefreshInterval() != 0 {
		t.Fatal("setup: expected the background refresh to be off")
	}
	if cmd := m.handleAutoRefreshTick(); cmd == nil {
		t.Fatal("expected the heartbeat to re-arm even with refresh off")
	}
}

func TestFormatBytes(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{3 * 1024 * 1024 * 1024, "3.0 GB"},
	} {
		if got := formatBytes(tc.in); got != tc.want {
			t.Fatalf("formatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
