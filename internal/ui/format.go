package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// readingWordsPerMinute is the assumed silent-reading pace for the "N min read"
// estimate in the article metadata block.
const readingWordsPerMinute = 225

// articleReadingStats returns the word count and estimated read time in whole
// minutes (>=1 when there is any text). Article content reaches the UI already
// converted to markdown/plain text, so a whitespace split is close enough; the
// minor inflation from markdown punctuation is acceptable for an estimate.
func articleReadingStats(content string) (words, minutes int) {
	words = len(strings.Fields(content))
	if words == 0 {
		return 0, 0
	}
	minutes = (words + readingWordsPerMinute - 1) / readingWordsPerMinute
	if minutes < 1 {
		minutes = 1
	}
	return words, minutes
}

func formatArticleBody(content string, width int, plainUI bool) string {
	content = stripEmailInvisibles(content)
	content = strings.ReplaceAll(content, "\r\n", "\n")
	paras := splitArticleParagraphs(content)
	out := make([]string, 0, len(paras))
	for _, p := range paras {
		if p == "" {
			continue
		}
		out = append(out, formatArticleParagraph(p, width, plainUI))
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n\n")
}

func formatSummaryBody(content string, width int, plainUI bool) string {
	content = stripEmailInvisibles(content)
	content = strings.ReplaceAll(content, "\r\n", "\n")
	paras := splitArticleParagraphs(content)
	if len(paras) == 1 {
		paras = splitDenseSummaryParagraph(paras[0])
	}
	out := make([]string, 0, len(paras))
	for _, p := range paras {
		if p == "" {
			continue
		}
		out = append(out, formatSummaryParagraph(p, width, plainUI))
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

func formatArticleParagraph(p string, width int, plainUI bool) string {
	lines := strings.Split(strings.TrimSpace(p), "\n")
	if len(lines) == 0 {
		return ""
	}

	quoteBar := "│ "
	if plainUI {
		quoteBar = "| "
	}
	trimmed := strings.TrimSpace(lines[0])
	switch {
	case strings.HasPrefix(trimmed, "#"):
		return wrapWords(strings.TrimSpace(strings.TrimLeft(trimmed, "#")), width)
	case strings.HasPrefix(trimmed, ">"):
		quote := normalizeInlineSpacing(strings.TrimSpace(strings.TrimLeft(trimmed, ">")))
		return wrapWords(quoteBar+quote, width)
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
		items := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(strings.TrimLeft(strings.TrimLeft(line, "-"), "*"))
			if line == "" {
				continue
			}
			items = append(items, wrapBullet(line, width, plainUI))
		}
		return strings.Join(items, "\n")
	default:
		return wrapWords(normalizeInlineSpacing(strings.Join(lines, " ")), width)
	}
}

func formatSummaryParagraph(p string, width int, plainUI bool) string {
	lines := strings.Split(strings.TrimSpace(p), "\n")
	if len(lines) == 0 {
		return ""
	}

	trimmed := strings.TrimSpace(lines[0])
	switch {
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
		items := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(strings.TrimLeft(strings.TrimLeft(line, "-"), "*"))
			if line == "" {
				continue
			}
			items = append(items, wrapBullet(line, width, plainUI))
		}
		return strings.Join(items, "\n")
	case isNumberedListItem(trimmed):
		items := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			num, body, ok := splitNumberedListItem(line)
			if !ok {
				items = append(items, wrapWords(normalizeInlineSpacing(line), width))
				continue
			}
			items = append(items, wrapNumberedBullet(num, body, width))
		}
		return strings.Join(items, "\n")
	default:
		return wrapWords(normalizeInlineSpacing(strings.Join(lines, " ")), width)
	}
}

func splitDenseSummaryParagraph(p string) []string {
	p = normalizeInlineSpacing(p)
	if p == "" {
		return nil
	}
	if strings.Contains(p, "\n") || strings.HasPrefix(p, "- ") || strings.HasPrefix(p, "* ") || isNumberedListItem(p) {
		return []string{p}
	}

	sentences := splitSentences(p)
	if len(sentences) < 3 {
		return []string{p}
	}

	paras := make([]string, 0, (len(sentences)+1)/2)
	for i := 0; i < len(sentences); i += 2 {
		end := min(i+2, len(sentences))
		paras = append(paras, strings.Join(sentences[i:end], " "))
	}
	return paras
}

func wrapBullet(text string, width int, plainUI bool) string {
	prefix := "• "
	indent := "  "
	if plainUI {
		prefix = "* "
		indent = "  "
	}
	if width <= 2 {
		return prefix + text
	}
	wrapped := wrapWords(text, width-2)
	lines := strings.Split(wrapped, "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
		} else {
			lines[i] = indent + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func wrapNumberedBullet(num, text string, width int) string {
	prefix := fmt.Sprintf("%s. ", num)
	if width <= lipgloss.Width(prefix) {
		return prefix + text
	}
	wrapped := wrapWords(text, width-lipgloss.Width(prefix))
	lines := strings.Split(wrapped, "\n")
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
		} else {
			lines[i] = indent + lines[i]
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
	s = stripEmailInvisibles(s)
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func stripEmailInvisibles(s string) string {
	if s == "" {
		return ""
	}
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u00ad', // soft hyphen
			'\u034f', // combining grapheme joiner
			'\u200b', // zero-width space
			'\u200c', // zero-width non-joiner
			'\u200d', // zero-width joiner
			'\u200e', // left-to-right mark
			'\u200f', // right-to-left mark
			'\u2060', // word joiner
			'\ufeff': // byte order mark / zero-width no-break space
			return -1
		default:
			return r
		}
	}, s)
}

func splitSentences(s string) []string {
	s = normalizeInlineSpacing(s)
	if s == "" {
		return nil
	}

	var sentences []string
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.', '!', '?':
			j := i + 1
			for j < len(s) && (s[j] == '"' || s[j] == '\'' || s[j] == ')' || s[j] == ']') {
				j++
			}
			if j == len(s) || s[j] == ' ' {
				part := strings.TrimSpace(s[start:j])
				if part != "" {
					sentences = append(sentences, part)
				}
				for j < len(s) && s[j] == ' ' {
					j++
				}
				start = j
				i = j - 1
			}
		}
	}
	if start < len(s) {
		tail := strings.TrimSpace(s[start:])
		if tail != "" {
			sentences = append(sentences, tail)
		}
	}
	if len(sentences) == 0 {
		return []string{s}
	}
	return sentences
}

func isNumberedListItem(s string) bool {
	_, _, ok := splitNumberedListItem(s)
	return ok
}

func splitNumberedListItem(s string) (num, body string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(s) || s[i] != '.' || s[i+1] != ' ' {
		return "", "", false
	}
	body = strings.TrimSpace(s[i+2:])
	if body == "" {
		return "", "", false
	}
	return s[:i], body, true
}
