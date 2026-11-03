package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// renderWrappedArticleRow is also the source of truth for list row heights.
func (m Model) renderWrappedArticleRow(index int) string {
	a := m.filteredArticles[index]
	width := m.articlesPaneContentWidth()
	unread, read, selected, _, _, _ := m.articleRowStyles()
	style := read
	if !a.Read {
		style = unread
	}
	style = applyArticleRowState(style, selected, a.Starred, index == m.articleCursor, m.styles.Theme)
	prefix := m.articleRowPrefix(a.Read)
	if index == m.articleCursor {
		prefix = m.articleRowCursorPrefix()
	}
	star := m.articleRowStar(a.Starred)
	age := m.formatTime(a.PublishedAt)
	if m.multiFeedSelected() {
		age = truncate(m.feedDisplayTitle(a.FeedID), min(18, width/4)) + " · " + age
	}
	// Keep metadata on the first line, reserving the same column on continuation lines.
	age = truncate(age, max(0, width-lipgloss.Width(prefix+star)-3))
	titleWidth := max(1, width-lipgloss.Width(prefix+star)-lipgloss.Width(age)-2)
	lines := strings.Split(ansi.Wrap(normalizeInlineSpacing(unescapeDisplayText(a.Title)), titleWidth, ""), "\n")
	// An exceptionally long title must still fit within the pane.
	limit := max(1, m.articlesPaneContentHeight()-m.paneHeaderHeight())
	if len(lines) > limit {
		lines = lines[:limit]
		lines[limit-1] = truncate(lines[limit-1]+" …", titleWidth)
	}
	body := style.UnsetPadding()
	for i, line := range lines {
		left, mid, right := articleRowSegments(prefix, star, line, age, width)
		if i > 0 {
			left = strings.Repeat(" ", lipgloss.Width(prefix))
			mid = strings.Repeat(" ", lipgloss.Width(star))
			right = padRight(line, max(0, width-lipgloss.Width(left+mid)))
		}
		if accent := starColor(m.styles.Theme); i == 0 && a.Starred && accent != "" && !m.styles.PlainUI {
			lines[i] = renderStyledArticleRow(body, accent, left, mid, right, width)
		} else {
			lines[i] = body.Width(width).Render(left + mid + right)
		}
	}
	if len(lines) < limit {
		for i := 1; i < m.styles.ListItemLineStride(); i++ {
			lines = append(lines, body.Width(width).Render(""))
		}
	}
	return strings.Join(lines, "\n")
}
