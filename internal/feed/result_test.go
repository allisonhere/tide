package feed

import (
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
)

func TestSetMaxFeedBodyBytesFallsBackToDefault(t *testing.T) {
	orig := maxFeedBodyBytes
	t.Cleanup(func() { maxFeedBodyBytes = orig })

	SetMaxFeedBodyBytes(0)

	if maxFeedBodyBytes != defaultMaxFeedBodyBytes {
		t.Fatalf("expected default limit, got %d", maxFeedBodyBytes)
	}
}

func TestFeedTooLargeFriendlyMessage(t *testing.T) {
	orig := maxFeedBodyBytes
	t.Cleanup(func() { maxFeedBodyBytes = orig })
	SetMaxFeedBodyBytes(defaultMaxFeedBodyBytes)

	r := &FetchResult{Kind: KindFeedTooLarge}

	if !strings.Contains(r.FriendlyMessage(), "too large") {
		t.Fatalf("expected too-large message, got %q", r.FriendlyMessage())
	}
	if !r.HasDetails() {
		t.Fatal("expected too-large result to report details")
	}
}

func TestParseItemFallbackGUIDIsStableWithoutGUIDOrLink(t *testing.T) {
	published := time.Unix(1710000000, 0)
	item := parseItem(&gofeed.Item{
		Title:           "Same Title",
		Description:     "Same Description",
		PublishedParsed: &published,
	})
	item2 := parseItem(&gofeed.Item{
		Title:           "Same Title",
		Description:     "Same Description",
		PublishedParsed: &published,
	})

	if item.GUID != item2.GUID {
		t.Fatalf("expected stable fallback GUID, got %q and %q", item.GUID, item2.GUID)
	}
	if !strings.HasPrefix(item.GUID, "fallback:") {
		t.Fatalf("expected fallback prefix, got %q", item.GUID)
	}
}

func TestParseItemFallbackGUIDDiffersForDifferentItems(t *testing.T) {
	first := parseItem(&gofeed.Item{Title: "One", Description: "Alpha"})
	second := parseItem(&gofeed.Item{Title: "Two", Description: "Beta"})

	if first.GUID == second.GUID {
		t.Fatalf("expected distinct fallback GUIDs, got %q", first.GUID)
	}
}

func TestParseItemPopulatesAuthorCategoriesUpdated(t *testing.T) {
	published := time.Unix(1710000000, 0)
	updated := published.Add(48 * time.Hour)
	item := parseItem(&gofeed.Item{
		Title:           "T",
		Link:            "https://example.com/a",
		PublishedParsed: &published,
		UpdatedParsed:   &updated,
		Author:          &gofeed.Person{Name: "  Ada Lovelace  "},
		Authors:         []*gofeed.Person{{Name: "Someone Else"}},
		Categories:      []string{" Tech ", "tech", "", "Science"},
	})

	if item.Author != "Ada Lovelace" {
		t.Fatalf("Author = %q, want %q (item.Author wins over Authors)", item.Author, "Ada Lovelace")
	}
	if got := item.Categories; len(got) != 2 || got[0] != "Tech" || got[1] != "Science" {
		t.Fatalf("Categories = %#v, want [Tech Science] (trimmed, deduped, order kept)", got)
	}
	if !item.UpdatedAt.Equal(updated) {
		t.Fatalf("UpdatedAt = %v, want %v", item.UpdatedAt, updated)
	}
}

func TestParseItemAuthorFallsBackToAuthorsList(t *testing.T) {
	item := parseItem(&gofeed.Item{
		Title:   "T",
		Link:    "https://example.com/b",
		Authors: []*gofeed.Person{{Name: ""}, {Name: "Grace Hopper"}},
	})
	if item.Author != "Grace Hopper" {
		t.Fatalf("Author = %q, want first named entry in Authors", item.Author)
	}
	if item.UpdatedAt != (time.Time{}) {
		t.Fatalf("UpdatedAt = %v, want zero when the feed gives no <updated>", item.UpdatedAt)
	}
}
