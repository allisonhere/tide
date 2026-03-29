package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

func renderHelp(width int, styles Styles, keys KeyMap) string {
	type entry struct{ key, desc string }
	type section struct {
		name    string
		entries []entry
	}

	bind := func(b key.Binding) entry {
		h := b.Help()
		return entry{h.Key, h.Desc}
	}

	sections := []section{
		{
			name: "Navigation",
			entries: []entry{
				bind(keys.NextPane),
				bind(keys.PrevPane),
				bind(keys.Left),
				bind(keys.Right),
				bind(keys.Up),
				bind(keys.Down),
				bind(keys.Enter),
				bind(keys.Back),
			},
		},
		{
			name: "Articles",
			entries: []entry{
				bind(keys.MarkRead),
				bind(keys.MarkAllRead),
				bind(keys.OpenBrowser),
				bind(keys.Search),
			},
		},
		{
			name: "Feeds",
			entries: []entry{
				bind(keys.Refresh),
				bind(keys.RefreshAll),
				bind(keys.FeedManager),
			},
		},
		{
			name: "Feed Manager",
			entries: []entry{
				bind(keys.Add),
				bind(keys.Edit),
				bind(keys.Delete),
				bind(keys.Import),
				bind(keys.Export),
			},
		},
		{
			name: "App",
			entries: []entry{
				bind(keys.ThemePicker),
				bind(keys.Help),
				bind(keys.Quit),
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
		Width(width).
		Padding(1, 3).
		Render(content)
}
