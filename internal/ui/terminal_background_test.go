package ui

import (
	"strings"
	"testing"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// kitty makes every cell whose color equals the default background
// see-through, so the solid default must never set the exact theme color.
func TestTerminalBackgroundSequencesKeepSolidThemesOpaque(t *testing.T) {
	for _, theme := range BuiltinThemes {
		exact := "\x1b]11;" + string(theme.Bg) + "\x07"
		solid, reset := TerminalBackgroundSequences(theme.Name, false)
		if solid == "" || reset == "" {
			t.Fatalf("%s: no OSC strings", theme.Name)
		}
		if strings.Contains(solid, exact) {
			t.Errorf("%s: solid mode sets the default background to the exact theme color", theme.Name)
		}
		if clear, _ := TerminalBackgroundSequences(theme.Name, true); clear != exact {
			t.Errorf("%s: transparent mode must set the exact theme color, got %q", theme.Name, clear)
		}
	}
}

func TestTransparentSettingRendersWithoutThemeBackground(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })

	m := NewModel(nil, config.DefaultConfig(), "test", false)
	m.width, m.height = 120, 36
	bg := m.styles.Theme.Bg
	probe := lipgloss.NewStyle().Background(bg).Render("x")
	params := probe[2:strings.IndexByte(probe, 'm')]
	if !strings.Contains(m.View(), params) {
		t.Fatal("solid mode must paint the theme background")
	}
	m.cfg.Display.Transparent = true
	view := m.View()
	if strings.Contains(view, params) {
		t.Fatal("transparent mode left cells painted in the theme background")
	}
	if view != tideui.TransparentBackground(view, bg) {
		t.Fatal("transparent view should already be fully rewritten")
	}
}

func TestTransparentSettingRoundTrips(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.Display.Transparent {
		t.Fatal("transparent background must default to off")
	}
	cfg.Display.Transparent = true
	s := newSettings(cfg, settingsUpdateState{})
	if !s.transparent || !s.ApplyTo(cfg).Display.Transparent {
		t.Fatal("transparent setting lost between config and settings")
	}
	s.transparent = false
	if s.ApplyTo(cfg).Display.Transparent {
		t.Fatal("turning transparency off did not save")
	}
}
