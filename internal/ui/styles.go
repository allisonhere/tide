package ui

import "github.com/charmbracelet/lipgloss"

type Styles struct {
	Theme Theme

	// Pane containers
	FeedsPane          lipgloss.Style
	ArticlesPane       lipgloss.Style
	ContentPane        lipgloss.Style
	PaneHeaderActive   lipgloss.Style
	PaneHeaderInactive lipgloss.Style

	// Feed list items
	FeedItem         lipgloss.Style
	FeedItemSelected lipgloss.Style
	FeedItemActive   lipgloss.Style // cursor row
	UnreadBadge      lipgloss.Style

	// Article list items
	ArticleUnread   lipgloss.Style
	ArticleRead     lipgloss.Style
	ArticleSelected lipgloss.Style
	ArticleTime     lipgloss.Style
	UnreadDot       lipgloss.Style

	// Content pane
	ContentTitle lipgloss.Style
	ContentMeta  lipgloss.Style
	ContentBody  lipgloss.Style

	// Status bar
	StatusBar     lipgloss.Style
	StatusError   lipgloss.Style
	StatusSpinner lipgloss.Style

	// Overlay chrome
	Overlay      lipgloss.Style
	OverlayTitle lipgloss.Style
	OverlayHint  lipgloss.Style

	// Inputs inside overlays/feed manager
	InputFocused   lipgloss.Style
	InputUnfocused lipgloss.Style
	InputLabel     lipgloss.Style

	// Help screen
	HelpSection lipgloss.Style
	HelpKey     lipgloss.Style
	HelpDesc    lipgloss.Style

	// Spinner
	Spinner lipgloss.Style
}

func BuildStyles(t Theme) Styles {
	modalBg := modalSurface(t)
	modalBorder := t.OverlayBorder
	if modalBorder == "" {
		modalBorder = t.Border
	}
	modalAccent := t.BorderFocus
	if modalAccent == "" {
		modalAccent = modalBorder
	}
	modalFg := readableText(t.Fg, modalBg, 4.5)
	modalMuted := mutedText(modalFg, modalBg)
	accentFg := readableText(t.Fg, modalAccent, 4.5)

	paneBase := lipgloss.NewStyle().
		Background(t.Bg).
		BorderForeground(t.Border)

	focusedPane := lipgloss.NewStyle().
		Background(t.Bg).
		BorderForeground(t.BorderFocus)

	return Styles{
		Theme: t,

		FeedsPane: paneBase.
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderBackground(t.Bg).
			AlignVertical(lipgloss.Top),
		ArticlesPane: focusedPane.
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderBackground(t.Bg).
			AlignVertical(lipgloss.Top),
		ContentPane: lipgloss.NewStyle().
			Background(t.Bg),
		PaneHeaderActive: lipgloss.NewStyle().
			Background(t.Unread).
			Foreground(t.Bg).
			Bold(true).
			AlignHorizontal(lipgloss.Left),
		PaneHeaderInactive: lipgloss.NewStyle().
			Background(t.Border).
			Foreground(t.Fg).
			AlignHorizontal(lipgloss.Left),

		FeedItem: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(t.Fg).
			AlignHorizontal(lipgloss.Left),
		FeedItemSelected: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(t.BorderFocus).
			Bold(true).
			AlignHorizontal(lipgloss.Left),
		FeedItemActive: lipgloss.NewStyle().
			Background(t.Border).
			Foreground(t.Fg).
			AlignHorizontal(lipgloss.Left),
		UnreadBadge: lipgloss.NewStyle().
			Foreground(t.Unread).
			Bold(true),

		ArticleUnread: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(t.Fg).
			AlignHorizontal(lipgloss.Left),
		ArticleRead: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(t.Dimmed).
			AlignHorizontal(lipgloss.Left),
		ArticleSelected: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(t.BorderFocus).
			Bold(true).
			AlignHorizontal(lipgloss.Left),
		ArticleTime: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(t.Dimmed),
		UnreadDot: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(t.Unread),

		ContentTitle: lipgloss.NewStyle().
			Background(t.BorderFocus).
			Foreground(t.Bg).
			Bold(true).
			Padding(0, 1).
			MarginBottom(1),
		ContentMeta: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(t.Dimmed).
			Italic(true),
		ContentBody: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(t.Fg),

		StatusBar: lipgloss.NewStyle().
			Background(t.StatusBar).
			Foreground(t.StatusFg).
			Padding(0, 1),
		StatusError: lipgloss.NewStyle().
			Background(t.StatusBar).
			Foreground(t.Error).
			Bold(true).
			Padding(0, 1),
		StatusSpinner: lipgloss.NewStyle().
			Foreground(t.Unread),

		Overlay: lipgloss.NewStyle().
			Background(modalBg).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(modalBorder).
			Padding(1, 2),
		OverlayTitle: lipgloss.NewStyle().
			Foreground(accentFg).
			Background(modalAccent).
			Bold(true).
			Padding(0, 1).
			MarginBottom(1),
		OverlayHint: lipgloss.NewStyle().
			Background(modalBg).
			Foreground(modalMuted).
			MarginTop(1),

		InputFocused: lipgloss.NewStyle().
			Background(modalBg).
			Foreground(modalFg).
			Border(lipgloss.NormalBorder()).
			BorderForeground(modalAccent).
			Padding(0, 1),
		InputUnfocused: lipgloss.NewStyle().
			Background(modalBg).
			Foreground(modalFg).
			Border(lipgloss.NormalBorder()).
			BorderForeground(modalBorder).
			Padding(0, 1),
		InputLabel: lipgloss.NewStyle().
			Foreground(modalMuted),

		HelpSection: lipgloss.NewStyle().
			Foreground(t.BorderFocus).
			Bold(true).
			MarginTop(1),
		HelpKey: lipgloss.NewStyle().
			Foreground(t.Unread).
			Width(20),
		HelpDesc: lipgloss.NewStyle().
			Foreground(t.Fg),

		Spinner: lipgloss.NewStyle().
			Foreground(t.Unread),
	}
}
