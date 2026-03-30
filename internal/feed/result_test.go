package feed

import (
	"strings"
	"testing"
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
