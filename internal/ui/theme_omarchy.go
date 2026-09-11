package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/allisonhere/tide/internal/omarchy"
)

// ThemeNameMatchOmarchy is the pseudo-theme that follows the current Omarchy
// desktop theme, contrast-corrected. It is not a member of BuiltinThemes — its
// colours are resolved at runtime — but it is offered by PickableThemes().
const ThemeNameMatchOmarchy = "match-omarchy"

// omarchyIndex is the virtual picker index for "match-omarchy": one past the
// real built-in themes.
var omarchyIndex = len(BuiltinThemes)

// omarchyPlaceholderTheme is the theme picker row and the preview/fallback base
// before (or when) the live Omarchy palette resolves. It reuses a known-good
// built-in palette.
var omarchyPlaceholderTheme = func() Theme {
	t := CatppuccinMocha
	t.Name = ThemeNameMatchOmarchy
	return t
}()

// isMatchOmarchy reports whether name selects the Omarchy-following theme.
func isMatchOmarchy(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), ThemeNameMatchOmarchy)
}

// omarchyTheme maps a raw Omarchy palette onto Tide's Theme struct. The result
// still needs contrastCorrectTheme before use.
func omarchyTheme(p omarchy.Palette) Theme {
	c := func(s string) lipgloss.Color { return lipgloss.Color(s) }

	bg := c(p.Background)
	fg := c(p.Foreground)

	border := c(p.Muted)
	if p.Muted == "" {
		border = mixColors(fg, bg, 0.5)
	}
	accent := c(p.Accent)
	if p.Accent == "" {
		accent = fg
	}
	statusBg := c(p.StatusBg)
	if p.StatusBg == "" {
		if isDark(bg) {
			statusBg = adjustLightness(bg, 0.05)
		} else {
			statusBg = adjustLightness(bg, -0.05)
		}
	}
	unread := c(p.Ok)
	if p.Ok == "" {
		unread = accent
	}
	errColor := c(p.Error)
	if p.Error == "" {
		errColor = accent
	}

	return Theme{
		Name:          ThemeNameMatchOmarchy,
		Bg:            bg,
		Fg:            fg,
		Border:        border,
		BorderFocus:   accent,
		Selected:      accent,
		Unread:        unread,
		Dimmed:        border,
		StatusBar:     statusBg,
		StatusFg:      fg,
		Error:         errColor,
		Overlay:       statusBg,
		OverlayBorder: accent,
	}
}

// omarchyFocusMinContrast is the contrast floor for the focused-pane border —
// higher than the text bar so the focus highlight stays the strongest element.
const omarchyFocusMinContrast = 7.0

// nudgeSurfaceForRatio lightens/darkens surf (starting from bg) until it clears
// minRatio against bg itself, so a status-bar surface that matches the page
// background still reads as distinct. Tide's color.go has no
// selectionBgForRatio, so this is the local equivalent.
func nudgeSurfaceForRatio(bg lipgloss.Color, minRatio float64) lipgloss.Color {
	const step = 0.03
	dir := step
	if !isDark(bg) {
		dir = -step
	}
	cur := bg
	for range 30 {
		next := adjustLightness(cur, dir)
		if next == cur {
			break
		}
		cur = next
		if contrastRatio(cur, bg) >= minRatio {
			return cur
		}
	}
	return cur
}

// contrastCorrectTheme nudges each directly-consumed Theme field until it
// clears Tide's readability floors against the theme background, so an
// arbitrary desktop palette can't produce an unreadable UI. Light/dark-agnostic
// — the helpers branch on isDark(t.Bg) internally.
func contrastCorrectTheme(t Theme) Theme {
	out := t

	out.Fg = readableText(t.Fg, t.Bg, 4.5)
	out.Dimmed = mutedText(out.Fg, t.Bg)
	out.BorderFocus = accentReadableOn(t.BorderFocus, t.Bg, omarchyFocusMinContrast)
	out.Selected = accentReadableOn(t.Selected, t.Bg, 4.5)
	out.OverlayBorder = accentReadableOn(t.OverlayBorder, t.Bg, 4.5)
	out.Border = accentReadableOn(t.Border, t.Bg, 3.0)
	out.Unread = accentReadableOn(t.Unread, t.Bg, 3.0)
	out.Error = accentReadableOn(t.Error, t.Bg, 4.5)

	if contrastRatio(out.StatusBar, t.Bg) < 1.2 {
		out.StatusBar = nudgeSurfaceForRatio(t.Bg, 2.0)
		out.Overlay = out.StatusBar
	}
	out.StatusFg = readableText(t.StatusFg, out.StatusBar, 4.5)

	return out
}

// resolveOmarchyTheme reads the live Omarchy palette and returns a
// contrast-corrected Theme. ok is false when Omarchy isn't available.
func resolveOmarchyTheme() (Theme, bool) {
	p, ok := omarchy.CurrentPalette()
	if !ok {
		return Theme{}, false
	}
	return contrastCorrectTheme(omarchyTheme(p)), true
}

// currentOmarchyThemeName returns the active Omarchy theme slug for a settings
// hint, or "" when Omarchy isn't available.
func currentOmarchyThemeName() string {
	if p, ok := omarchy.CurrentPalette(); ok {
		return p.Name
	}
	return ""
}

// omarchyThemeTickMsg drives the live-follow poll while the active theme is
// "match-omarchy".
type omarchyThemeTickMsg struct{}

const omarchyWatchInterval = 2 * time.Second

func omarchyWatchCmd() tea.Cmd {
	return tea.Tick(omarchyWatchInterval, func(time.Time) tea.Msg { return omarchyThemeTickMsg{} })
}

func omarchySignature() string { return omarchy.CurrentSignature() }

// startOmarchyWatchIfNeeded records the current signature and returns the
// live-follow poll command when the active theme is "match-omarchy" and no poll
// loop is already running; otherwise nil.
func (m *Model) startOmarchyWatchIfNeeded() tea.Cmd {
	if !isMatchOmarchy(m.cfg.Theme) {
		return nil
	}
	m.omarchySig = omarchySignature()
	if m.omarchyWatching {
		return nil
	}
	m.omarchyWatching = true
	return omarchyWatchCmd()
}

// handleOmarchyThemeTick re-resolves the Omarchy palette whenever the desktop
// theme changed and re-arms itself while "match-omarchy" is active; it stops
// (returns no command) once the theme is something else.
func (m Model) handleOmarchyThemeTick() (tea.Model, tea.Cmd) {
	if !isMatchOmarchy(m.cfg.Theme) {
		m.omarchyWatching = false
		return m, nil
	}
	m.omarchyWatching = true
	sig := omarchySignature()
	if sig == m.omarchySig {
		return m, omarchyWatchCmd()
	}
	m.omarchySig = sig
	merged, _ := MergedThemeFromConfig(m.cfg)
	m.styles = BuildStyles(merged, m.cfg.Display.Density, m.cfg.Display.PaneCorners)
	if len(m.filteredArticles) > 0 {
		m.setViewportArticle(m.filteredArticles[m.articleCursor])
	}
	return m, tea.Batch(omarchyWatchCmd(), setTermBgCmd(merged.Bg))
}
