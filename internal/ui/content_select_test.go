package ui

import (
	"strings"
	"testing"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tide/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// withColor turns styling on for a test: the default Ascii profile emits no
// escape codes, so a styling assertion would pass against plain text.
func withColor(t *testing.T) {
	t.Helper()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
}

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

func send(m Model, keys ...rune) Model {
	for _, r := range keys {
		next, _ := m.Update(press(r))
		m = next.(Model)
	}
	return m
}

// ── capture ──────────────────────────────────────────────────────────────────

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

// ── v: character-wise ────────────────────────────────────────────────────────

// v anchors at the cursor and selects one character; l extends it along the
// line, the way vim's character-wise visual mode does.
func TestCharSelectionStartsAtOneCharacterAndExtends(t *testing.T) {
	m := send(selectionModel(t, true), 'v')
	if m.contentSelectionMode != selectionChar {
		t.Fatalf("expected v to open a character-wise selection, got mode %v", m.contentSelectionMode)
	}
	startLine, startCol, endLine, endCol, ok := m.selectionSpan()
	if !ok || startLine != endLine || startCol != endCol {
		t.Fatalf("expected a single character selected, got %d:%d..%d:%d", startLine, startCol, endLine, endCol)
	}
	if got := m.contentSelectionText(); len([]rune(got)) != 1 {
		t.Fatalf("expected one character of text, got %q", got)
	}

	m = send(m, 'l', 'l', 'l')
	if got := m.contentSelectionText(); got != "alph" {
		t.Fatalf("expected l to extend along the line, got %q", got)
	}
}

// The cursor starts on the line's first real character, not out in the pane's
// indent, so v immediately selects text rather than a space.
func TestCharSelectionSkipsThePaneIndent(t *testing.T) {
	m := send(selectionModel(t, true), 'v')
	if got := m.contentSelectionText(); strings.TrimSpace(got) == "" {
		t.Fatalf("expected the cursor to start on text, got %q", got)
	}
	if m.contentFocusCol != m.firstTextCol(m.contentFocusLine) {
		t.Fatalf("expected the cursor at the first text column %d, got %d",
			m.firstTextCol(m.contentFocusLine), m.contentFocusCol)
	}
}

// h walks back toward the anchor and past it, which flips which end of the
// span is the start — the span stays in reading order either way.
func TestCharSelectionExtendsBackwardsPastTheAnchor(t *testing.T) {
	m := send(selectionModel(t, true), 'v', 'l', 'l', 'l', 'l')
	if got := m.contentSelectionText(); got != "alpha" {
		t.Fatalf("setup: expected %q, got %q", "alpha", got)
	}

	m = send(m, 'h', 'h', 'h', 'h', 'h', 'h')
	startLine, startCol, endLine, endCol, _ := m.selectionSpan()
	if startLine != endLine {
		t.Fatal("expected the span to stay on one line")
	}
	if startCol > endCol {
		t.Fatalf("expected the span in reading order, got %d..%d", startCol, endCol)
	}
	if got := m.contentSelectionText(); got == "" {
		t.Fatal("expected text once the cursor crossed the anchor")
	}
}

// A character-wise span across lines takes the tail of the first line, whole
// lines in between, and the head of the last.
func TestCharSelectionAcrossLinesTakesPartialEnds(t *testing.T) {
	m := send(selectionModel(t, true), 'v')
	// Run to the end of "alpha line", drop two lines to "bravo line" — the
	// sticky column lands the cursor at its end too — then walk back so the
	// last line is cut short.
	for i := 0; i < 20; i++ {
		m = send(m, 'l')
	}
	m = send(m, 'j', 'j')
	for i := 0; i < 6; i++ {
		m = send(m, 'h')
	}

	text := m.contentSelectionText()
	lines := strings.Split(text, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected a three-line span, got %d: %q", len(lines), text)
	}
	if !strings.HasSuffix(lines[0], "line") {
		t.Fatalf("expected the first line to run to its end, got %q", lines[0])
	}
	if strings.Contains(lines[2], "line") {
		t.Fatalf("expected the last line to stop at the cursor, got %q", lines[2])
	}
}

// The pane indents every line, and that indent is layout rather than article
// text, so the cursor cannot enter it and it never reaches the clipboard.
func TestCursorCannotEnterThePaneIndent(t *testing.T) {
	m := send(selectionModel(t, true), 'v')
	first := m.firstTextCol(m.contentFocusLine)
	if first == 0 {
		t.Skip("this article renders flush; nothing to test")
	}

	m = send(m, 'h', 'h', 'h')
	if m.contentFocusCol != first {
		t.Fatalf("expected h to stop at the first text column %d, got %d", first, m.contentFocusCol)
	}

	// A span that continues onto later lines starts them at their text too.
	span := send(selectionModel(t, true), 'v')
	for i := 0; i < 20; i++ {
		span = send(span, 'l')
	}
	span = send(span, 'j', 'j')
	for _, line := range strings.Split(span.contentSelectionText(), "\n")[1:] {
		if strings.HasPrefix(line, " ") {
			t.Fatalf("expected continuation lines to start at their text, got %q", line)
		}
	}
}

// Character-wise copies exactly the characters covered: no dedent, no trimming.
func TestCharSelectionTextIsExact(t *testing.T) {
	m := send(selectionModel(t, true), 'v', 'l', 'l', 'l', 'l')
	if got := m.contentSelectionText(); got != "alpha" {
		t.Fatalf("expected an exact character span, got %q", got)
	}
}

// ── V: line-wise ─────────────────────────────────────────────────────────────

// V takes whole lines from the anchor to the cursor, wherever the columns are.
func TestLineSelectionTakesWholeLines(t *testing.T) {
	m := send(selectionModel(t, true), 'l', 'l')
	m = send(m, 'V')
	if m.contentSelectionMode != selectionLine {
		t.Fatalf("expected V to open a line-wise selection, got mode %v", m.contentSelectionMode)
	}
	if got := m.contentSelectionText(); got != "alpha line" {
		t.Fatalf("expected the whole line regardless of the column, got %q", got)
	}

	m = send(m, 'j', 'j')
	got := m.contentSelectionText()
	if !strings.Contains(got, "alpha line") || !strings.Contains(got, "bravo line") {
		t.Fatalf("expected j to extend by whole lines, got %q", got)
	}
}

// Line-wise text arrives flush: the pane's indent is layout, not the article.
func TestLineSelectionTextIsDedented(t *testing.T) {
	m := send(selectionModel(t, true), 'V', 'j', 'j', 'j', 'j')
	for _, line := range strings.Split(m.contentSelectionText(), "\n") {
		if strings.HasPrefix(line, " ") {
			t.Fatalf("expected the pane indent to be stripped, got %q", line)
		}
	}

	if got := dedent([]string{"  alpha", "", "    indented", "  bravo"}); !equalLines(got, []string{"alpha", "", "  indented", "bravo"}) {
		t.Fatalf("expected relative indentation to survive, got %#v", got)
	}
	if got := dedent([]string{"alpha", "  bravo"}); !equalLines(got, []string{"alpha", "  bravo"}) {
		t.Fatalf("expected already-flush text to be left alone, got %#v", got)
	}
}

// ── mode switching and cancelling ────────────────────────────────────────────

// In vim the other visual key switches mode and keeps the anchor; the same key
// closes the selection.
func TestVisualKeysSwitchModeAndCancel(t *testing.T) {
	m := send(selectionModel(t, true), 'v', 'l', 'l')
	anchor := m.contentSelectionAnchorLine

	m = send(m, 'V')
	if m.contentSelectionMode != selectionLine {
		t.Fatal("expected V to switch a character-wise selection to line-wise")
	}
	if m.contentSelectionAnchorLine != anchor {
		t.Fatal("expected the anchor to survive the mode switch")
	}

	m = send(m, 'v')
	if m.contentSelectionMode != selectionChar {
		t.Fatal("expected v to switch back to character-wise")
	}

	m = send(m, 'v')
	if m.selectionActive() {
		t.Fatal("expected the same key again to close the selection")
	}

	m = send(m, 'V', 'V')
	if m.selectionActive() {
		t.Fatal("expected V twice to close the selection")
	}
}

func TestEscapeCancelsSelectionBeforeLeavingThePane(t *testing.T) {
	m := send(selectionModel(t, true), 'V')

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.selectionActive() {
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

// A selection belongs to the article it was made in.
func TestSelectionClearsWhenArticleChanges(t *testing.T) {
	m := send(selectionModel(t, true), 'V')
	m.setViewportArticle(db.Article{ID: 2, FeedID: 1, Title: "Another", Content: "other body"})
	if m.selectionActive() {
		t.Fatal("expected switching articles to clear the selection")
	}
}

// ── cursor movement ──────────────────────────────────────────────────────────

// h/l are the character cursor only while selecting; the rest of the time they
// go on switching panes, since the pane has no column cursor of its own.
func TestArrowsSwitchPanesUnlessSelecting(t *testing.T) {
	m := selectionModel(t, true)
	m = send(m, 'h')
	if m.focused != paneArticles {
		t.Fatalf("expected h to move to the previous pane, got %v", m.focused)
	}

	sel := send(selectionModel(t, true), 'v', 'l')
	if sel.focused != paneContent {
		t.Fatal("expected l to stay in the content pane while selecting")
	}
	if sel.contentFocusCol == sel.contentSelectionAnchorCol {
		t.Fatal("expected l to move the character cursor while selecting")
	}
}

// vim keeps a desired column: stepping onto a short line and back off it
// restores the column the cursor had.
func TestCursorColumnIsStickyAcrossLines(t *testing.T) {
	m := send(selectionModel(t, true), 'v')
	for i := 0; i < 8; i++ {
		m = send(m, 'l')
	}
	want := m.contentFocusCol
	if want == 0 {
		t.Fatal("setup: expected the cursor to have moved along the line")
	}

	m = send(m, 'j') // a blank line, which can only hold column 0
	if m.contentFocusCol != 0 {
		t.Fatalf("expected the cursor to clamp on a blank line, got %d", m.contentFocusCol)
	}
	m = send(m, 'j') // back onto a full line
	if m.contentFocusCol != want {
		t.Fatalf("expected the desired column %d to be restored, got %d", want, m.contentFocusCol)
	}
}

// Reading movement hops between readable lines; a selection has to visit every
// line, or it would jump over text it appears to cover.
func TestSelectionMovementVisitsEveryLine(t *testing.T) {
	m := send(selectionModel(t, true), 'v')
	start := m.contentFocusLine
	m = send(m, 'j')
	if m.contentFocusLine != start+1 {
		t.Fatalf("expected j to move exactly one line while selecting, got %d from %d", m.contentFocusLine, start)
	}

	// Without a selection, j skips the blank line between paragraphs.
	reading := selectionModel(t, true)
	readingStart := reading.contentFocusLine
	reading = send(reading, 'j')
	if reading.contentFocusLine != readingStart+2 {
		t.Fatalf("expected reading movement to skip the blank line, got %d from %d", reading.contentFocusLine, readingStart)
	}
}

// The cursor moves while selecting even for readers who keep the focus-line
// highlight off: during a selection it is the span's edge, not a highlight.
func TestSelectionMovesWithFocusLineDisabled(t *testing.T) {
	m := send(selectionModel(t, false), 'v', 'j')
	startLine, _, endLine, _, ok := m.selectionSpan()
	if !ok || endLine <= startLine {
		t.Fatalf("expected the selection to extend with the focus line off, got %d..%d", startLine, endLine)
	}

	// Without a selection, j goes back to scrolling the viewport. It needs an
	// article taller than the pane to have somewhere to scroll to.
	scroll := selectionModel(t, false)
	scroll.setViewportArticle(db.Article{
		ID: 2, FeedID: 1, Title: "Long article",
		Content: strings.Repeat("a paragraph of body text\n\n", 60),
	})
	before := scroll.viewport.YOffset
	scroll = send(scroll, 'j')
	if scroll.viewport.YOffset == before {
		t.Fatal("expected j to scroll the viewport when nothing is selected")
	}
}

// ── copying ──────────────────────────────────────────────────────────────────

// With no selection open, c copies the whole article — the common case should
// not require entering visual mode first.
func TestCopyWithoutSelectionFallsBackToWholeArticle(t *testing.T) {
	m := selectionModel(t, true)
	if m.selectionActive() {
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

func TestCopyReportsWhatWasTakenAndClearsTheSelection(t *testing.T) {
	chars := send(selectionModel(t, true), 'v', 'l', 'l')
	if got := chars.contentCopyLabel(); got != "3 characters" {
		t.Fatalf("expected a character count for a one-line span, got %q", got)
	}

	lines := send(selectionModel(t, true), 'V', 'j', 'j')
	if got := lines.contentCopyLabel(); got != "3 lines" {
		t.Fatalf("expected a line count for a line-wise span, got %q", got)
	}

	m := send(chars, 'c')
	if m.selectionActive() {
		t.Fatal("expected copying to clear the selection")
	}
	if !strings.Contains(m.statusMsg, "copied") {
		t.Fatalf("expected the status line to confirm the copy, got %q", m.statusMsg)
	}
}

// y copies as well as c. y is also the Yes binding, but that only ever answers
// a confirm overlay, which never shares a key path with the main UI.
func TestCopyKeysAreCAndY(t *testing.T) {
	for _, k := range []rune{'c', 'y'} {
		m := send(selectionModel(t, true), 'V')
		next, cmd := m.Update(press(k))
		m = next.(Model)
		if cmd == nil {
			t.Fatalf("%c: expected a clipboard command", k)
		}
		if m.selectionActive() {
			t.Fatalf("%c: expected copying to clear the selection", k)
		}
		if !strings.Contains(m.statusMsg, "copied") {
			t.Fatalf("%c: expected the status line to confirm the copy, got %q", k, m.statusMsg)
		}
	}
}

func TestYStillConfirmsQuit(t *testing.T) {
	m := selectionModel(t, true)
	m.overlay = overlayQuitConfirm
	if _, cmd := m.Update(press('y')); cmd == nil {
		t.Fatal("expected y to confirm the quit overlay")
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
	if _, link, ok := m.currentArticleLink(); !ok || link != "https://example.com/article" {
		t.Fatalf("expected the article's link, got %q (ok=%v)", link, ok)
	}
}

func TestCopyLinkReportsArticlesWithoutOne(t *testing.T) {
	m := selectionModel(t, true)
	m.articles[0].Link = ""
	m.applyFilter()

	m = send(m, 'L')
	if !strings.Contains(m.statusMsg, "no link") {
		t.Fatalf("expected a status message about the missing link, got %q", m.statusMsg)
	}
}

// ── rendering ────────────────────────────────────────────────────────────────

// A character-wise selection sits inside its line: the covered characters are
// highlighted and the rest of the line is left as it was.
func TestCharSelectionHighlightsOnlyItsCharacters(t *testing.T) {
	withColor(t)
	m := selectionModel(t, true)
	plain := m.renderContentPane()
	sel := send(m, 'v', 'l', 'l').renderContentPane()

	if bodyLines(plain) == bodyLines(sel) {
		t.Fatal("expected a selection to restyle the article body")
	}
	if bodyLines(ansi.Strip(plain)) != bodyLines(ansi.Strip(sel)) {
		t.Fatal("expected a selection to change only styling, not the text")
	}

}

// renderSelectedSpan is where a character-wise selection stays inside its line:
// only the covered cells are repainted, and the text comes through unchanged.
func TestRenderSelectedSpanPaintsOnlyItsColumns(t *testing.T) {
	withColor(t)
	styles := BuildStyles(CatppuccinMocha, "compact", "square")
	line := "alpha bravo charlie"
	runes := []rune(line)

	got := renderSelectedSpan(line, runes, 6, 10, styles.ContentFocusLine)
	if ansi.Strip(got) != line {
		t.Fatalf("expected the text to survive styling, got %q", ansi.Strip(got))
	}
	if !strings.HasPrefix(got, "alpha ") {
		t.Fatalf("expected the text before the span to be left alone, got %q", got)
	}
	if !strings.HasSuffix(got, " charlie") {
		t.Fatalf("expected the text after the span to be left alone, got %q", got)
	}
	if got == line {
		t.Fatal("expected the selected columns to be styled")
	}

	// A whole-line span still comes back as the same text.
	full := renderSelectedSpan(line, runes, 0, len(runes)-1, styles.ContentFocusLine)
	if ansi.Strip(full) != line {
		t.Fatalf("expected a full-line span to preserve the text, got %q", ansi.Strip(full))
	}

	// A blank line shows one cell, so it does not read as a gap mid-span.
	if blank := renderSelectedSpan("", nil, 0, 0, styles.ContentFocusLine); ansi.Strip(blank) != " " {
		t.Fatalf("expected a blank line to show one selected cell, got %q", ansi.Strip(blank))
	}
}

// A line-wise selection paints whole lines.
func TestLineSelectionHighlightsWholeLines(t *testing.T) {
	m := send(selectionModel(t, true), 'V')
	if !m.contentLineSelected(m.contentFocusLine) {
		t.Fatal("expected the cursor's line to be selected")
	}
	startCol, endCol, ok := m.selectedColumns(m.contentFocusLine)
	if !ok || startCol != 0 || endCol != len(m.lineRunes(m.contentFocusLine))-1 {
		t.Fatalf("expected the whole line covered, got %d..%d", startCol, endCol)
	}
}

// The header swaps to the keys that apply while selecting.
func TestPaneHeaderShowsSelectionKeys(t *testing.T) {
	idle := ansi.Strip(selectionModel(t, true).renderContentPane())
	selecting := ansi.Strip(send(selectionModel(t, true), 'v').renderContentPane())

	if !strings.Contains(idle, "select") {
		t.Fatal("expected the pane header to advertise selection when idle")
	}
	if !strings.Contains(selecting, "extend") || !strings.Contains(selecting, "cancel") {
		t.Fatal("expected the pane header to offer the selection keys")
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

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

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
