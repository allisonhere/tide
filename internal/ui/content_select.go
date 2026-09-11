package ui

import (
	"fmt"
	"strings"

	"github.com/allisonhere/tide/internal/db"
)

// Copying used to reach only as far as an AI summary: there was no way to get
// an article's own words, or its link, out of Tide and into anything else.
// Selection works in whole lines, anchored where the content focus line already
// is, so it rides the j/k movement that is there rather than introducing a
// second cursor. -allie

// startContentSelection begins a line selection at the focus line, or ends the
// one in progress. Pressing v twice is how you back out.
func (m *Model) startContentSelection() {
	if m.contentLineCount == 0 {
		return
	}
	if m.contentSelectionActive && !m.contentSelectionAll {
		m.clearContentSelection()
		return
	}
	m.contentSelectionActive = true
	m.contentSelectionAll = false
	m.contentSelectionAnchor = clamp(m.contentFocusLine, 0, m.contentLineCount-1)
}

// selectAllContentLines selects the whole article, so the common case — copy
// this article — does not require walking to the end of it.
func (m *Model) selectAllContentLines() {
	if m.contentLineCount == 0 {
		return
	}
	m.contentSelectionActive = true
	m.contentSelectionAll = true
	m.contentSelectionAnchor = 0
}

func (m *Model) clearContentSelection() {
	m.contentSelectionActive = false
	m.contentSelectionAll = false
	m.contentSelectionAnchor = 0
}

// contentSelectionRange is the inclusive line span currently selected, and
// whether anything is.
func (m Model) contentSelectionRange() (start, end int, ok bool) {
	if !m.contentSelectionActive || m.contentLineCount == 0 {
		return 0, 0, false
	}
	if m.contentSelectionAll {
		return 0, m.contentLineCount - 1, true
	}
	start = clamp(m.contentSelectionAnchor, 0, m.contentLineCount-1)
	end = clamp(m.contentFocusLine, 0, m.contentLineCount-1)
	if start > end {
		start, end = end, start
	}
	return start, end, true
}

func (m Model) contentLineSelected(line int) bool {
	start, end, ok := m.contentSelectionRange()
	if !ok {
		return false
	}
	return line >= start && line <= end
}

// contentSelectionText is the selected lines as plain text. With no selection
// it falls back to the whole article, which is what makes a bare c useful
// without entering visual mode first.
func (m Model) contentSelectionText() string {
	if len(m.contentLines) == 0 {
		return ""
	}
	start, end, ok := m.contentSelectionRange()
	if !ok {
		start, end = 0, len(m.contentLines)-1
	}
	start = clamp(start, 0, len(m.contentLines)-1)
	end = clamp(end, 0, len(m.contentLines)-1)

	lines := make([]string, 0, end-start+1)
	for _, line := range m.contentLines[start : end+1] {
		// The pane pads every line out to its width; that padding is layout,
		// not text, and has no business on the clipboard.
		lines = append(lines, strings.TrimRight(line, " \t"))
	}
	// Trim the blank lines the layout leaves at either end of a span.
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(dedent(lines), "\n")
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
// status line — "article" reads as a very different action from "12 lines".
func (m Model) contentCopyLabel() string {
	start, end, ok := m.contentSelectionRange()
	if !ok {
		return "article text"
	}
	if n := end - start + 1; n != 1 {
		return pluralLines(n)
	}
	return "1 line"
}

func pluralLines(n int) string {
	if n == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", n)
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
