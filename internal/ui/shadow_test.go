package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/muesli/termenv"

	"github.com/allisonhere/tide/internal/config"
)

// Passing shadow=true must actually blend a drop shadow into the base behind
// the box, not render identically regardless of the flag.
func TestOverlayOnBaseShadowChangesRender(t *testing.T) {
	const w, h = 40, 12
	bg := CatppuccinMocha.Bg
	base := strings.TrimRight(strings.Repeat(strings.Repeat("x", w)+"\n", h), "\n")
	box := lipgloss.NewStyle().Background(lipgloss.Color("#444444")).Width(16).Height(4).Render("hi")

	without := overlayOnBase(base, box, w, h, bg, false)
	with := overlayOnBase(base, box, w, h, bg, true)

	if with == without {
		t.Fatal("overlayOnBase produced identical output with and without the shadow")
	}
	for _, out := range []string{without, with} {
		if got := strings.Count(out, "\n") + 1; got != h {
			t.Fatalf("line count = %d, want %d", got, h)
		}
		for _, line := range strings.Split(out, "\n") {
			if got := lipgloss.Width(line); got != w {
				t.Fatalf("line width = %d, want %d (%q)", got, w, ansi.Strip(line))
			}
		}
	}
	if !strings.Contains(ansi.Strip(with), "hi") {
		t.Fatal("shadowed overlay lost the box content")
	}
}

// A wide rune in the base straddling the column where the compositor slices a
// row must not drop a cell and lose the box's right border or the shadow
// sliver. Every row stays exactly w wide, wherever a wide rune lands.
func TestOverlayOnBaseShadowWideRunesKeepWidth(t *testing.T) {
	const w, h = 60, 24
	bg := CatppuccinMocha.Bg
	var b strings.Builder
	for i := 0; i < h; i++ {
		if i%2 == 0 {
			b.WriteString(strings.Repeat("-", w))
		} else {
			b.WriteString(clampView(strings.Repeat("x", i)+strings.Repeat("世", w), w, 1, bg))
		}
		if i < h-1 {
			b.WriteByte('\n')
		}
	}
	box := lipgloss.NewStyle().Background(lipgloss.Color("#444444")).Width(30).Height(10).Render("box")

	lines := strings.Split(overlayOnBase(b.String(), box, w, h, bg, true), "\n")
	if len(lines) != h {
		t.Fatalf("line count = %d, want %d", len(lines), h)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != w {
			t.Fatalf("row %d width = %d, want %d (%q)", i, got, w, ansi.Strip(line))
		}
	}
}

// The base comes back untouched when the shadow rectangle misses the viewport.
func TestBlendShadowRectNoIntersection(t *testing.T) {
	const w, h = 20, 6
	bg := CatppuccinMocha.Bg
	base := strings.TrimRight(strings.Repeat(strings.Repeat("y", w)+"\n", h), "\n")

	got := blendShadowRect(base, 0, h+5, 8, 3, w, h, bg, shadowColor(bg))
	if got != clampView(base, w, h, bg) {
		t.Fatal("blendShadowRect altered base for an off-screen shadow rectangle")
	}
}

// The shadow is always darker than the page. On a light theme it steps down
// until it clears the contrast floor; a near-black theme cannot reach the floor
// even with pure black, so there it settles on the darkest color available.
// Either way the strength depends on the theme's own background, which is what
// makes the shadow "vary".
func TestShadowColorDarkensOnLightAndDarkThemes(t *testing.T) {
	latte := shadowColor(CatppuccinLatte.Bg)
	if got := contrastRatio(latte, CatppuccinLatte.Bg); got < shadowBgMinContrast {
		t.Errorf("latte: shadow contrast %.2f is below the %.1f floor", got, shadowBgMinContrast)
	}

	mocha := shadowColor(CatppuccinMocha.Bg)
	if luminance(mocha) > 0.005 {
		t.Errorf("mocha: expected the darkest reachable shadow, got %s", mocha)
	}

	for name, theme := range map[string]Theme{"mocha": CatppuccinMocha, "latte": CatppuccinLatte} {
		if luminance(shadowColor(theme.Bg)) >= luminance(theme.Bg) {
			t.Errorf("%s: shadow is not darker than the page %s", name, theme.Bg)
		}
	}
	if latte == mocha {
		t.Error("expected different themes to get different shadow colors")
	}
}

func settingsOverlayView(t *testing.T, theme string, shadow bool) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Theme = theme
	cfg.Display.Shadow = shadow
	m := NewModel(nil, cfg, "v1.0.0", false)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	m = next.(Model)
	m.settings = newSettings(m.cfg, m.settingsUpdateState())
	m.overlay = overlaySettings
	return m.View()
}

func TestOverlayShadowFollowsSettingAndSkipsPlainUI(t *testing.T) {
	withTrueColor(t)
	if settingsOverlayView(t, "catppuccin-mocha", true) == settingsOverlayView(t, "catppuccin-mocha", false) {
		t.Fatal("expected the shadow setting to change a dialog's render")
	}
	if settingsOverlayView(t, "vt52", true) != settingsOverlayView(t, "vt52", false) {
		t.Fatal("expected no shadow on the plain-ASCII VT52 theme")
	}
}

func TestShadowDefaultsOnAndRoundTripsThroughSettings(t *testing.T) {
	cfg := config.DefaultConfig()
	if !cfg.Display.Shadow {
		t.Fatal("expected the drop shadow to default on")
	}
	s := newSettings(cfg, settingsUpdateState{})
	if !s.shadow {
		t.Fatal("expected Settings to load the shadow setting")
	}
	s.shadow = false
	if s.ApplyTo(cfg).Display.Shadow {
		t.Fatal("expected ApplyTo to save the shadow off")
	}

	s.setActiveSection(ssDisplay)
	body := s.viewSectionBody(80, newManagerChrome(84, CatppuccinMocha, false))
	if _, ok := body.anchors[sfShadow]; !ok {
		t.Fatal("expected the drop shadow toggle in Settings → Display")
	}
}

// withTrueColor makes lipgloss emit 24-bit colors for the test, since the test
// environment has no terminal and would otherwise strip them all.
func withTrueColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// cellBackground is the resolved background of one screen cell of a rendered view.
func cellBackground(t *testing.T, view string, w, h, col, row int) lipgloss.Color {
	t.Helper()
	buf := cellbuf.NewBuffer(w, h)
	cellbuf.SetContent(buf, view)
	cell := buf.Cell(col, row)
	if cell == nil || cell.Style.Bg == nil {
		t.Fatalf("no background resolved at (%d,%d)", col, row)
	}
	r, g, b, _ := cell.Style.Bg.RGBA()
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8))
}

// The sliver beside and below the box is darker than the page with the shadow
// on, matches the page with it off, and is a different strength on a light
// theme than on a dark one.
func TestShadowSliverIsDarkerThanPageAndVariesWithTheme(t *testing.T) {
	withTrueColor(t)
	const w, h = 40, 12
	// A 16x4 box centred in 40x12 occupies columns 12..27, rows 4..7, so the
	// shadow sliver is column 28 (rows 5..8) and row 8 (columns 13..28).
	const sliverCol, sliverRow = 28, 6
	box := lipgloss.NewStyle().Background(lipgloss.Color("#888888")).Width(16).Height(4).Render("hi")

	ratios := map[string]float64{}
	for name, theme := range map[string]Theme{"mocha": CatppuccinMocha, "latte": CatppuccinLatte} {
		page := lipgloss.NewStyle().Background(theme.Bg).Width(w).Height(h).Render("")

		off := cellBackground(t, overlayOnBase(page, box, w, h, theme.Bg, false), w, h, sliverCol, sliverRow)
		if off != theme.Bg {
			t.Fatalf("%s: expected the page color without a shadow, got %s want %s", name, off, theme.Bg)
		}
		on := overlayOnBase(page, box, w, h, theme.Bg, true)
		side := cellBackground(t, on, w, h, sliverCol, sliverRow)
		bottom := cellBackground(t, on, w, h, 20, 8)
		far := cellBackground(t, on, w, h, 2, 2)
		if far != theme.Bg {
			t.Fatalf("%s: expected cells away from the box untouched, got %s", name, far)
		}
		for where, got := range map[string]lipgloss.Color{"right": side, "bottom": bottom} {
			if luminance(got) >= luminance(theme.Bg) {
				t.Errorf("%s: %s sliver %s is not darker than the page %s", name, where, got, theme.Bg)
			}
		}
		ratios[name] = contrastRatio(side, theme.Bg)
	}
	if ratios["mocha"] == ratios["latte"] {
		t.Errorf("expected the shadow's strength to vary by theme, both were %.2f", ratios["mocha"])
	}
}
