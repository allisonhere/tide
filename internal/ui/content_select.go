package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tide/internal/db"
)

// Selection in the content pane follows vim: v is character-wise and V is
// line-wise, both anchored where the cursor is and extended by the usual
// movement keys. The cursor is the content focus line plus a column, which only
// exists while a selection is open — outside one the pane has no column cursor
// and h/l go on switching panes. -allie

type selectionMode int

const (
	selectionNone selectionMode = iota
	selectionChar               // v: from an anchor character to the cursor, inclusive
	selectionLine               // V: whole lines from the anchor line to the cursor
)

func (m Model) selectionActive() bool { return m.contentSelectionMode != selectionNone }

// startCharSelection handles v: opens a character-wise selection, switches an
// open line-wise one down to character-wise, or closes an open character-wise
// one — the same three things v does in vim.
func (m *Model) startCharSelection() {
	switch m.contentSelectionMode {
	case selectionChar:
		m.clearContentSelection()
	case selectionLine:
		m.contentSelectionMode = selectionChar
	default:
		m.beginSelection(selectionChar)
	}
}

// startLineSelection handles V, mirroring startCharSelection.
func (m *Model) startLineSelection() {
	switch m.contentSelectionMode {
	case selectionLine:
		m.clearContentSelection()
	case selectionChar:
		m.contentSelectionMode = selectionLine
	default:
		m.beginSelection(selectionLine)
	}
}

func (m *Model) beginSelection(mode selectionMode) {
	if m.contentLineCount == 0 {
		return
	}
	m.contentSelectionMode = mode
	m.contentFocusLine = clamp(m.contentFocusLine, 0, m.contentLineCount-1)
	// The pane indents every line, and there is no visible column cursor to
	// inherit a position from, so the cursor starts on the line's first real
	// character rather than out in the margin.
	m.contentFocusCol = m.firstTextCol(m.contentFocusLine)
	m.contentDesiredCol = m.contentFocusCol
	m.contentSelectionAnchorLine = m.contentFocusLine
	m.contentSelectionAnchorCol = m.contentFocusCol
}

func (m *Model) clearContentSelection() {
	m.contentSelectionMode = selectionNone
	m.contentSelectionAnchorLine = 0
	m.contentSelectionAnchorCol = 0
	m.contentFocusCol = 0
	m.contentDesiredCol = 0
}

// lineRunes is one content line with its trailing layout padding removed, so
// the cursor cannot wander off the end of the text into the pane's fill.
func (m Model) lineRunes(line int) []rune {
	if line < 0 || line >= len(m.contentLines) {
		return nil
	}
	return []rune(strings.TrimRight(m.contentLines[line], " \t"))
}

// firstTextCol is the column of a line's first non-space character, or 0 for a
// blank line.
func (m Model) firstTextCol(line int) int {
	for i, r := range m.lineRunes(line) {
		if r != ' ' && r != '\t' {
			return i
		}
	}
	return 0
}

// clampCol keeps a column inside a line's text. The lower bound is the line's
// first real character, not column 0: the pane indents every line, and that
// indent is layout rather than anything the article wrote — a cursor that could
// sit in it would drag it onto the clipboard. A blank line has exactly one
// position, column 0, the same way vim puts the cursor on an empty line.
func (m Model) clampCol(line, col int) int {
	runes := m.lineRunes(line)
	if len(runes) == 0 {
		return 0
	}
	return clamp(col, m.firstTextCol(line), len(runes)-1)
}

// moveSelectionCursorCol moves the cursor by characters and re-stickies the
// column, which is what makes a later j/k keep the position h/l just chose.
func (m *Model) moveSelectionCursorCol(delta int) {
	if !m.selectionActive() {
		return
	}
	m.contentFocusCol = m.clampCol(m.contentFocusLine, m.contentFocusCol+delta)
	m.contentDesiredCol = m.contentFocusCol
}

// moveSelectionCursorLine moves the cursor a line at a time. Unlike reading
// movement it visits every line, blank ones included: skipping them would make
// a selection jump over text it appears to cover. The column follows vim's
// sticky rule — it returns to the desired column wherever the line is long
// enough to hold it.
func (m *Model) moveSelectionCursorLine(delta int) {
	if !m.selectionActive() || m.contentLineCount == 0 {
		return
	}
	m.contentFocusLine = clamp(m.contentFocusLine+delta, 0, m.contentLineCount-1)
	m.contentFocusCol = m.clampCol(m.contentFocusLine, m.contentDesiredCol)
	m.ensureContentFocusVisible()
}

// selectionSpan is the selection in reading order: (startLine, startCol) to
// (endLine, endCol), inclusive at both ends. A line-wise selection reports
// whole lines regardless of where the columns are.
func (m Model) selectionSpan() (startLine, startCol, endLine, endCol int, ok bool) {
	if !m.selectionActive() || m.contentLineCount == 0 {
		return 0, 0, 0, 0, false
	}
	aLine := clamp(m.contentSelectionAnchorLine, 0, m.contentLineCount-1)
	fLine := clamp(m.contentFocusLine, 0, m.contentLineCount-1)
	aCol := m.clampCol(aLine, m.contentSelectionAnchorCol)
	fCol := m.clampCol(fLine, m.contentFocusCol)

	switch {
	case aLine < fLine:
		startLine, startCol, endLine, endCol = aLine, aCol, fLine, fCol
	case aLine > fLine:
		startLine, startCol, endLine, endCol = fLine, fCol, aLine, aCol
	default:
		startLine, endLine = aLine, aLine
		startCol, endCol = min(aCol, fCol), max(aCol, fCol)
	}

	if m.contentSelectionMode == selectionLine {
		startCol = 0
		endCol = max(0, len(m.lineRunes(endLine))-1)
	}
	return startLine, startCol, endLine, endCol, true
}

// selectedColumns is the column range highlighted on one line, or ok=false when
// that line falls outside the selection. A line fully inside a multi-line span
// reports its whole width.
func (m Model) selectedColumns(line int) (startCol, endCol int, ok bool) {
	startLine, sCol, endLine, eCol, active := m.selectionSpan()
	if !active || line < startLine || line > endLine {
		return 0, 0, false
	}
	last := max(0, len(m.lineRunes(line))-1)
	// A line-wise selection paints from the very start so the block has a
	// straight left edge; a character-wise one starts at the text, for the same
	// reason the cursor cannot enter the indent.
	first := 0
	if m.contentSelectionMode == selectionChar {
		first = m.firstTextCol(line)
	}
	switch {
	case startLine == endLine:
		return sCol, eCol, true
	case line == startLine:
		return sCol, last, true
	case line == endLine:
		return first, eCol, true
	default:
		return first, last, true
	}
}

func (m Model) contentLineSelected(line int) bool {
	_, _, ok := m.selectedColumns(line)
	return ok
}

// contentSelectionText is the selected text. With no selection open it falls
// back to the whole article, which is what makes a bare c useful without
// entering visual mode first.
func (m Model) contentSelectionText() string {
	if len(m.contentLines) == 0 {
		return ""
	}
	startLine, _, endLine, _, ok := m.selectionSpan()
	if !ok {
		return m.linewiseText(0, len(m.contentLines)-1)
	}
	if m.contentSelectionMode == selectionLine {
		return m.linewiseText(startLine, endLine)
	}

	lines := make([]string, 0, endLine-startLine+1)
	for line := startLine; line <= endLine; line++ {
		// Take the range the renderer highlights, so what is copied is exactly
		// what was shown as selected.
		from, to, ok := m.selectedColumns(line)
		if !ok {
			continue
		}
		runes := m.lineRunes(line)
		from = clamp(from, 0, len(runes))
		to = clamp(to+1, from, len(runes))
		lines = append(lines, string(runes[from:to]))
	}
	// Character-wise means exactly these characters: no dedent, no trimming.
	return strings.Join(lines, "\n")
}

// linewiseText is whole lines, cleaned up the way a block of prose should
// arrive on the clipboard.
func (m Model) linewiseText(startLine, endLine int) string {
	lines := make([]string, 0, endLine-startLine+1)
	for line := startLine; line <= endLine; line++ {
		lines = append(lines, string(m.lineRunes(line)))
	}
	return strings.Join(trimBlankEnds(dedent(lines)), "\n")
}

// trimBlankEnds drops the blank lines the layout leaves at either end of a span.
func trimBlankEnds(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// dedent removes the indent the content pane adds to every line, without
// flattening indentation the article meant — a quote or a code block keeps its
// shape relative to the rest of the block.
func dedent(lines []string) []string {
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, " "))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	if indent <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if len(line) >= indent {
			out[i] = line[indent:]
			continue
		}
		out[i] = strings.TrimLeft(line, " ")
	}
	return out
}

// contentCopyLabel describes what a copy just put on the clipboard, for the
// status line — "article text" reads as a very different action from "12 lines".
func (m Model) contentCopyLabel() string {
	startLine, startCol, endLine, endCol, ok := m.selectionSpan()
	if !ok {
		return "article text"
	}
	if m.contentSelectionMode == selectionChar && startLine == endLine {
		return pluralChars(endCol - startCol + 1)
	}
	return pluralLines(endLine - startLine + 1)
}

func pluralLines(n int) string {
	if n == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", n)
}

func pluralChars(n int) string {
	if n == 1 {
		return "1 character"
	}
	return fmt.Sprintf("%d characters", n)
}

// renderSelectedSpan paints columns [startCol, endCol] of an already-rendered
// line and leaves the rest of that line's own styling intact — a character-wise
// selection has to sit inside a line without flattening it. The selected slice
// is stripped first: an inline reset inside it would punch a hole in the
// selection's background.
func renderSelectedSpan(line string, runes []rune, startCol, endCol int, style lipgloss.Style) string {
	if len(runes) == 0 {
		// An empty line still shows one cell of selection, so a blank line in
		// the middle of a span does not read as a gap.
		return style.Render(" ")
	}
	startCol = clamp(startCol, 0, len(runes)-1)
	endCol = clamp(endCol, startCol, len(runes)-1)

	// Columns are rune positions, but slicing a styled string works in display
	// cells, so convert through the width of the prefix.
	leftW := lipgloss.Width(string(runes[:startCol]))
	midW := lipgloss.Width(string(runes[startCol : endCol+1]))

	left := ansi.Truncate(line, leftW, "")
	mid := ansi.Truncate(ansi.TruncateLeft(line, leftW, ""), midW, "")
	right := ansi.TruncateLeft(line, leftW+midW, "")
	return left + style.Render(ansi.Strip(mid)) + right
}

// currentArticleLink is the link of whatever the content pane is showing, or
// of the selected row when the content pane is empty.
func (m Model) currentArticleLink() (db.Article, string, bool) {
	if len(m.filteredArticles) == 0 {
		return db.Article{}, "", false
	}
	a := m.filteredArticles[clamp(m.articleCursor, 0, len(m.filteredArticles)-1)]
	link := strings.TrimSpace(a.Link)
	if link == "" {
		return a, "", false
	}
	return a, link, true
}
