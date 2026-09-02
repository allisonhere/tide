package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tide/internal/db"
)

func TestArticleReadingStats(t *testing.T) {
	cases := []struct {
		content     string
		wantWords   int
		wantMinutes int
	}{
		{"", 0, 0},
		{"one", 1, 1},
		{strings.Repeat("word ", 226), 226, 2},
		{strings.Repeat("word ", 225), 225, 1},
	}
	for _, c := range cases {
		w, m := articleReadingStats(c.content)
		if w != c.wantWords || m != c.wantMinutes {
			t.Fatalf("articleReadingStats(%d words) = (%d, %d), want (%d, %d)",
				len(strings.Fields(c.content)), w, m, c.wantWords, c.wantMinutes)
		}
	}
}

func TestGroupThousands(t *testing.T) {
	for in, want := range map[int]string{0: "0", 42: "42", 999: "999", 1000: "1,000", 12345: "12,345", 1234567: "1,234,567"} {
		if got := groupThousands(in); got != want {
			t.Fatalf("groupThousands(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestMetaShowsUpdated(t *testing.T) {
	pub := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	if metaShowsUpdated(db.Article{PublishedAt: pub}) {
		t.Fatal("zero UpdatedAt must not show")
	}
	if metaShowsUpdated(db.Article{PublishedAt: pub, UpdatedAt: pub.Add(2 * time.Hour)}) {
		t.Fatal("same-day update must not show")
	}
	if !metaShowsUpdated(db.Article{PublishedAt: pub, UpdatedAt: pub.Add(48 * time.Hour)}) {
		t.Fatal("a later calendar day must show")
	}
}

func TestRenderArticleMetaOmitsEmptyFields(t *testing.T) {
	m := newImageTestModel(t, true)
	m.width, m.height = 160, 50

	// Nothing but the always-present state line.
	out := m.renderArticleMeta(db.Article{}, 40)
	lines := strings.Split(ansi.Strip(out), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "unread") {
		t.Fatalf("bare article meta = %#v, want a single unread line", lines)
	}

	// Author + tags + read + starred present.
	out = m.renderArticleMeta(db.Article{
		Author: "Ada Lovelace", Content: "one two three four five",
		Read: true, Starred: true, Categories: []string{"Tech", "Science"},
	}, 40)
	got := ansi.Strip(out)
	for _, want := range []string{"By Ada Lovelace", "min read", "read", "saved", "#Tech", "#Science"} {
		if !strings.Contains(got, want) {
			t.Fatalf("meta block missing %q:\n%s", want, got)
		}
	}
}
