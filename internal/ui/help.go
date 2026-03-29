package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderHelp(width, height int, styles Styles, keys KeyMap) string {
	type entry struct{ key, desc string }
	type section struct {
		name    string
		entries []entry
	}

	sections := []section{
		{
			name: "Navigation",
			entries: []entry{
				{"Tab / Shift-Tab", "cycle panes"},
				{"h / ←   l / →", "move between panes"},
				{"j / ↓   k / ↑", "navigate within pane"},
				{"Enter", "open article in content pane"},
				{"Esc", "return to articles from content"},
			},
		},
		{
			name: "Articles",
			entries: []entry{
				{"r", "toggle read / unread"},
				{"R", "mark all articles read"},
				{"o", "open article in browser"},
				{"/", "search / filter articles"},
			},
		},
		{
			name: "Feeds",
			entries: []entry{
				{"f", "refresh selected feed"},
				{"F", "refresh all feeds"},
				{"m", "open feed manager"},
			},
		},
		{
			name: "Feed Manager",
			entries: []entry{
				{"a", "add feed"},
				{"e / Enter", "edit selected feed"},
				{"d", "delete feed (with confirmation)"},
				{"i", "import OPML file"},
				{"x", "export OPML to ~/.config/rss/feeds.opml"},
			},
		},
		{
			name: "App",
			entries: []entry{
				{"T", "open theme picker (live preview)"},
				{"?", "this help screen"},
				{"q", "quit (with confirmation)"},
			},
		},
	}

	lines := []string{
		styles.HelpSection.Render("Help — Keyboard Shortcuts"),
		"",
	}

	for _, s := range sections {
		lines = append(lines, styles.HelpSection.Render(s.name))
		for _, e := range s.entries {
			line := styles.HelpKey.Render(e.key) + styles.HelpDesc.Render(e.desc)
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}

	lines = append(lines, styles.OverlayHint.Render("[esc / ? / q] close"))

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Width(width).Height(height).
		Padding(1, 3).
		Render(content)
}
