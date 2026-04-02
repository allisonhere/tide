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
	FeedItem                  lipgloss.Style
	FeedItemSelected          lipgloss.Style
	FeedItemSelectedFocused   lipgloss.Style
	FeedItemSelectedUnfocused lipgloss.Style
	UnreadBadge               lipgloss.Style

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
	StatusHint    lipgloss.Style
	StatusNotice  lipgloss.Style

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

	selectedBg := func() lipgloss.Color {
		if isDark(t.Bg) {
			return adjustLightness(t.Bg, 0.12)
		}
		return adjustLightness(t.Bg, -0.12)
	}()
	selectedBgSoft := func() lipgloss.Color {
		if isDark(t.Bg) {
			return adjustLightness(t.Bg, 0.06)
		}
		return adjustLightness(t.Bg, -0.06)
	}()

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
			Background(t.BorderFocus).
			Foreground(readableText(t.Fg, t.BorderFocus, 4.5)).
			Bold(true).
			AlignHorizontal(lipgloss.Left),
		PaneHeaderInactive: lipgloss.NewStyle().
			Background(t.Border).
			Foreground(readableText(t.Fg, t.Border, 4.5)).
			AlignHorizontal(lipgloss.Left),

		FeedItem: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(t.Fg).
			AlignHorizontal(lipgloss.Left),
		FeedItemSelected: lipgloss.NewStyle().
			Background(selectedBg).
			Foreground(readableText(t.BorderFocus, selectedBg, 4.5)).
			Bold(true).
			AlignHorizontal(lipgloss.Left),
		FeedItemSelectedFocused: lipgloss.NewStyle().
			Background(selectedBg).
			Foreground(readableText(t.BorderFocus, selectedBg, 4.5)).
			Bold(true).
			AlignHorizontal(lipgloss.Left),
		FeedItemSelectedUnfocused: lipgloss.NewStyle().
			Background(selectedBgSoft).
			Foreground(readableText(t.BorderFocus, selectedBgSoft, 4.5)).
			Bold(true).
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
			Foreground(readableText(t.Dimmed, t.Bg, 3.0)).
			AlignHorizontal(lipgloss.Left),
		ArticleSelected: lipgloss.NewStyle().
			Background(func() lipgloss.Color {
				if isDark(t.Bg) {
					return adjustLightness(t.Bg, 0.08)
				}
				return adjustLightness(t.Bg, -0.08)
			}()).
			Foreground(readableText(t.BorderFocus, func() lipgloss.Color {
				if isDark(t.Bg) {
					return adjustLightness(t.Bg, 0.08)
				}
				return adjustLightness(t.Bg, -0.08)
			}(), 4.5)).
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
			Foreground(readableText(t.Bg, t.BorderFocus, 4.5)).
			Bold(true).
			Padding(0, 1).
			MarginBottom(1),
		ContentMeta: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(readableText(t.Dimmed, t.Bg, 3.0)).
			Italic(true),
		ContentBody: lipgloss.NewStyle().
			Background(t.Bg).
			Foreground(t.Fg),

		StatusBar: lipgloss.NewStyle().
			Background(t.StatusBar).
			Foreground(readableText(t.StatusFg, t.StatusBar, 4.5)).
			Padding(0, 1),
		StatusError: lipgloss.NewStyle().
			Background(t.StatusBar).
			Foreground(readableText(t.Error, t.StatusBar, 4.5)).
			Bold(true).
			Padding(0, 1),
		StatusSpinner: lipgloss.NewStyle().
			Foreground(t.Unread),
		StatusHint: lipgloss.NewStyle().
			Background(t.StatusBar).
			Foreground(readableText(t.StatusFg, t.StatusBar, 3.0)),
		StatusNotice: lipgloss.NewStyle().
			Background(t.BorderFocus).
			Foreground(readableText(t.Fg, t.BorderFocus, 4.5)).
			Bold(true).
			Padding(0, 1),

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
