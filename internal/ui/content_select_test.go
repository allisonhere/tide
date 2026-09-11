package ui

import (
	"strings"
	"testing"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tide/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func selectionModel(t *testing.T, focusLine bool) Model {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Display.FocusLine = focusLine
	m := NewModel(nil, cfg, "v1.0.0", false)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	m.feeds = []db.Feed{{ID: 1, Title: "Feed One", URL: "https://example.com/1"}}
	m.sidebarRows = []sidebarRow{{kind: rowKindFeed, feedID: 1}}
	next, _ = m.Update(ArticlesLoadedMsg{FeedID: 1, Articles: []db.Article{{
		ID:      1,
		FeedID:  1,
		Title:   "Readable article",
		Link:    "https://example.com/article",
		Content: "alpha line\n\nbravo line\n\ncharlie line\n\ndelta line",
	}}})
	m = next.(Model)
	m.focused = paneContent
	return m
}

func press(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// The rendered article is kept as plain lines so a selection can be turned back
// into text; without it there is nothing to copy.
func TestContentLinesCapturedWithoutStyling(t *testing.T) {
	m := selectionModel(t, true)
	if len(m.contentLines) == 0 {
		t.Fatal("expected the rendered article to be captured as lines")
	}
	if m.contentLineCount != len(m.contentLines) {
		t.Fatalf("line count %d disagrees with captured lines %d", m.contentLineCount, len(m.contentLines))
	}
	joined := strings.Join(m.contentLines, "\n")
	if joined != ansi.Strip(joined) {
		t.Fatal("expected captured lines to carry no ANSI styling")
	}
	if !strings.Contains(joined, "alpha line") {
		t.Fatalf("expected the article body in the captured lines, got %q", joined)
	}
}

// v anchors a selection at the focus line, j/k extend it, and a second v backs
// out of it.
func TestVisualSelectAnchorsExtendsAndCancels(t *testing.T) {
	m := selectionModel(t, true)
	anchor := m.contentFocusLine

	next, _ := m.Update(press('v'))
	m = next.(Model)
	if !m.contentSelectionActive {
		t.Fatal("expected v to start a selection")
	}
	if m.contentSelectionAnchor != anchor {
		t.Fatalf("expected the selection anchored at the focus line %d, got %d", anchor, m.contentSelectionAnchor)
	}
	start, end, ok := m.contentSelectionRange()
	if !ok || start != end {
		t.Fatalf("expected a single-line selection to start, got %d..%d", start, end)
	}

	next, _ = m.Update(press('j'))
	m = next.(Model)
	start, end, _ = m.contentSelectionRange()
	if end <= start {
		t.Fatalf("expected j to extend the selection, got %d..%d", start, end)
	}

	next, _ = m.Update(press('v'))
	m = next.(Model)
	if m.contentSelectionActive {
		t.Fatal("expected a second v to cancel the selection")
	}
}

// Selecting upward from the anchor is the same span as selecting downward.
func TestVisualSelectExtendsBackwards(t *testing.T) {
	m := selectionModel(t, true)
	for i := 0; i < 3; i++ {
		next, _ := m.Update(press('j'))
		m = next.(Model)
	}
	anchor := m.contentFocusLine

	next, _ := m.Update(press('v'))
	m = next.(Model)
	next, _ = m.Update(press('k'))
	m = next.(Model)

	start, end, ok := m.contentSelectionRange()
	if !ok {
		t.Fatal("expected an active selection")
	}
	if end != anchor || start >= end {
		t.Fatalf("expected the span to run up to the anchor %d, got %d..%d", anchor, start, end)
	}
}

func TestVisualLineSelectsWholeArticle(t *testing.T) {
	m := selectionModel(t, true)

	next, _ := m.Update(press('V'))
	m = next.(Model)

	start, end, ok := m.contentSelectionRange()
	if !ok || start != 0 || end != m.contentLineCount-1 {
		t.Fatalf("expected the whole article selected, got %d..%d of %d", start, end, m.contentLineCount)
	}
}

// A selection is the article's, not the pane's: moving to another article drops
// it rather than leaving a stale span over new text.
func TestSelectionClearsWhenArticleChanges(t *testing.T) {
	m := selectionModel(t, true)
	next, _ := m.Update(press('V'))
	m = next.(Model)

	m.setViewportArticle(db.Article{ID: 2, FeedID: 1, Title: "Another", Content: "other body"})
	if m.contentSelectionActive {
		t.Fatal("expected switching articles to clear the selection")
	}
}

func TestEscapeCancelsSelectionBeforeLeavingThePane(t *testing.T) {
	m := selectionModel(t, true)
	next, _ := m.Update(press('V'))
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.contentSelectionActive {
		t.Fatal("expected esc to cancel the selection")
	}
	if m.focused != paneContent {
		t.Fatal("expected the first esc to stay in the content pane")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.focused != paneArticles {
		t.Fatal("expected a second esc to leave the content pane")
	}
}

// Copied text is the article's words — no layout padding, no styling, and no
// blank lines dangling off either end of the span.
func TestSelectionTextIsCleanPlainText(t *testing.T) {
	m := selectionModel(t, true)
	next, _ := m.Update(press('V'))
	m = next.(Model)

	text := m.contentSelectionText()
	if text == "" {
		t.Fatal("expected the whole-article selection to produce text")
	}
	if text != ansi.Strip(text) {
		t.Fatal("expected copied text to carry no styling")
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t") {
			t.Fatalf("expected trailing layout padding to be trimmed, got %q", line)
		}
	}
	if strings.TrimSpace(strings.Split(text, "\n")[0]) == "" {
		t.Fatal("expected leading blank lines to be trimmed")
	}
	if !strings.Contains(text, "alpha line") || !strings.Contains(text, "delta line") {
		t.Fatalf("expected the article body in the copied text, got %q", text)
	}
}

// With no selection open, c copies the whole article — the common case should
// not require entering visual mode first.
func TestCopyWithoutSelectionFallsBackToWholeArticle(t *testing.T) {
	m := selectionModel(t, true)
	if m.contentSelectionActive {
		t.Fatal("setup: expected no selection")
	}

	text := m.contentSelectionText()
	if !strings.Contains(text, "alpha line") || !strings.Contains(text, "delta line") {
		t.Fatalf("expected a bare copy to take the whole article, got %q", text)
	}
	if got := m.contentCopyLabel(); got != "article text" {
		t.Fatalf("expected the status to name the whole article, got %q", got)
	}
}

func TestCopyReportsSelectionSizeAndClearsIt(t *testing.T) {
	m := selectionModel(t, true)
	next, _ := m.Update(press('v'))
	m = next.(Model)
	next, _ = m.Update(press('j'))
	m = next.(Model)

	start, end, _ := m.contentSelectionRange()
	want := pluralLines(end - start + 1)
	if got := m.contentCopyLabel(); got != want {
		t.Fatalf("expected the copy label to count the span, got %q want %q", got, want)
	}

	next, _ = m.Update(press('c'))
	m = next.(Model)
	if m.contentSelectionActive {
		t.Fatal("expected copying to clear the selection")
	}
	if !strings.Contains(m.statusMsg, "copied") {
		t.Fatalf("expected the status line to confirm the copy, got %q", m.statusMsg)
	}
}

// The focus line is the selection's moving edge, so it has to move even for
// readers who keep the focus-line highlight switched off.
func TestSelectionExtendsWithFocusLineDisabled(t *testing.T) {
	m := selectionModel(t, false)
	next, _ := m.Update(press('v'))
	m = next.(Model)
	next, _ = m.Update(press('j'))
	m = next.(Model)

	start, end, ok := m.contentSelectionRange()
	if !ok || end <= start {
		t.Fatalf("expected the selection to extend with the focus line off, got %d..%d", start, end)
	}

	// Without a selection, j goes back to scrolling the viewport. It needs an
	// article taller than the pane to have somewhere to scroll to.
	scroll := selectionModel(t, false)
	scroll.setViewportArticle(db.Article{
		ID: 2, FeedID: 1, Title: "Long article",
		Content: strings.Repeat("a paragraph of body text\n\n", 60),
	})
	before := scroll.viewport.YOffset
	next, _ = scroll.Update(press('j'))
	scroll = next.(Model)
	if scroll.viewport.YOffset == before {
		t.Fatal("expected j to scroll the viewport when nothing is selected")
	}
}

func TestCopyLinkCopiesTheArticleURL(t *testing.T) {
	m := selectionModel(t, true)

	next, cmd := m.Update(press('L'))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected a clipboard command")
	}
	if !strings.Contains(m.statusMsg, "copied link") {
		t.Fatalf("expected the status line to confirm the link copy, got %q", m.statusMsg)
	}

	_, link, ok := m.currentArticleLink()
	if !ok || link != "https://example.com/article" {
		t.Fatalf("expected the article's link, got %q (ok=%v)", link, ok)
	}
}

func TestCopyLinkReportsArticlesWithoutOne(t *testing.T) {
	m := selectionModel(t, true)
	m.articles[0].Link = ""
	m.applyFilter()

	next, _ := m.Update(press('L'))
	m = next.(Model)
	if !strings.Contains(m.statusMsg, "no link") {
		t.Fatalf("expected a status message about the missing link, got %q", m.statusMsg)
	}
}

// Selected lines are painted, so the block is visible while it is being built.
func TestSelectedLinesAreHighlighted(t *testing.T) {
	m := selectionModel(t, true)
	plain := m.renderContentPane()

	next, _ := m.Update(press('V'))
	m = next.(Model)
	selected := m.renderContentPane()

	if plain == selected {
		t.Fatal("expected a selection to change how the content pane renders")
	}

	// Only the styling and the pane's hint line change; the article's own text
	// is left exactly as it was. Highlighting pads lines out to the pane width,
	// so compare the words rather than the trailing whitespace.
	plainBody := bodyLines(ansi.Strip(plain))
	selectedBody := bodyLines(ansi.Strip(selected))
	if plainBody != selectedBody {
		t.Fatalf("expected a selection to change only styling, not the text:\n%s\n---\n%s", plainBody, selectedBody)
	}

	// The header swaps to the keys that apply while selecting.
	if !strings.Contains(ansi.Strip(selected), "extend") || !strings.Contains(ansi.Strip(selected), "cancel") {
		t.Fatal("expected the pane header to offer the selection keys")
	}
	if !strings.Contains(ansi.Strip(plain), "select") {
		t.Fatal("expected the pane header to advertise selection when idle")
	}
}

// bodyLines is a pane rendering with its header row and trailing padding
// removed, so two renderings can be compared on the article text alone.
func bodyLines(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 2 {
		lines = lines[2:] // frame row, then the pane header
	}
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}
