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
				{keys.NextPane.Help().Key, "next pane"},
				{keys.PrevPane.Help().Key, "previous pane"},
				{keys.Left.Help().Key, "move left across panes"},
				{keys.Right.Help().Key, "move right across panes"},
				bind(keys.Up),
				bind(keys.Down),
				{keys.Enter.Help().Key, "open article / enter pane"},
				bind(keys.Back),
			},
		},
		{
			name: "Articles / Content",
			entries: []entry{
				{keys.MarkRead.Help().Key, "mark read; next article in list"},
				{keys.MarkAllRead.Help().Key, "mark current feed as read"},
				{keys.OpenBrowser.Help().Key, "open selected article in browser"},
				{keys.Search.Help().Key, "search articles in current feed"},
			},
		},
		{
			name: "Feeds",
			entries: []entry{
				bind(keys.Refresh),
				bind(keys.RefreshAll),
				bind(keys.FeedManager),
				{keys.Add.Help().Key, "add feed from anywhere"},
			},
		},
		{
			name: "Feed Manager",
			entries: []entry{
				{keys.Add.Help().Key, "add feed or GReader source"},
				{keys.AddFolder.Help().Key, "add folder"},
				{keys.Enter.Help().Key, "edit selected feed / enter form"},
				{keys.Left.Help().Key, "from field start, back to left list"},
				{keys.Edit.Help().Key, "edit selected local feed"},
				{keys.Delete.Help().Key, "delete selected local feed"},
				bind(keys.Import),
				bind(keys.Export),
			},
		},
		{
			name: "Summary Overlay",
			entries: []entry{
				{keys.Summary.Help().Key, "AI summary from articles or content"},
				{keys.CopyText.Help().Key, "copy summary"},
				{keys.SaveMD.Help().Key, "save summary as .md"},
			},
		},
		{
			name: "App",
			entries: []entry{
				bind(keys.ThemePicker),
				bind(keys.Settings),
				{"U", "install available Tide update"},
				{"i", "ignore available Tide update"},
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

	lines = append(lines, styles.OverlayHint.Render("[esc / ? / q] close help"))

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Width(width).
		Padding(1, 3).
		Render(content)
}
