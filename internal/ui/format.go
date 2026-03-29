package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func formatArticleBody(content string, width int) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	paras := splitArticleParagraphs(content)
	out := make([]string, 0, len(paras))
	for _, p := range paras {
		if p == "" {
			continue
		}
		out = append(out, formatArticleParagraph(p, width))
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n\n")
}

func splitArticleParagraphs(content string) []string {
	raw := strings.Split(content, "\n\n")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func formatArticleParagraph(p string, width int) string {
	lines := strings.Split(strings.TrimSpace(p), "\n")
	if len(lines) == 0 {
		return ""
	}

	trimmed := strings.TrimSpace(lines[0])
	switch {
	case strings.HasPrefix(trimmed, "#"):
		return wrapWords(strings.TrimSpace(strings.TrimLeft(trimmed, "#")), width)
	case strings.HasPrefix(trimmed, ">"):
		quote := normalizeInlineSpacing(strings.TrimSpace(strings.TrimLeft(trimmed, ">")))
		return wrapWords("│ "+quote, width)
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
		items := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(strings.TrimLeft(strings.TrimLeft(line, "-"), "*"))
			if line == "" {
				continue
			}
			items = append(items, wrapBullet(line, width))
		}
		return strings.Join(items, "\n")
	default:
		return wrapWords(normalizeInlineSpacing(strings.Join(lines, " ")), width)
	}
}

func wrapBullet(text string, width int) string {
	if width <= 2 {
		return "• " + text
	}
	wrapped := wrapWords(text, width-2)
	lines := strings.Split(wrapped, "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = "• " + lines[i]
		} else {
			lines[i] = "  " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func wrapWords(text string, width int) string {
	text = normalizeInlineSpacing(text)
	if text == "" || width <= 1 {
		return text
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	lines := []string{words[0]}
	for _, word := range words[1:] {
		current := lines[len(lines)-1]
		if lipgloss.Width(current)+1+lipgloss.Width(word) <= width {
			lines[len(lines)-1] = current + " " + word
			continue
		}
		lines = append(lines, truncate(word, width))
	}
	return strings.Join(lines, "\n")
}

func normalizeInlineSpacing(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
