package ui

import (
	"math"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tide/internal/config"
)

func TestSettingsTextInputsAcceptMovementRunes(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.focusedField = sfBrowser
	s.browserInput.SetValue("")
	s.applyFocus()

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, DefaultKeys)

	if next.focusedField != sfBrowser {
		t.Fatalf("expected browser field to keep focus, got %v", next.focusedField)
	}
	if got := next.browserInput.Value(); got != "k" {
		t.Fatalf("expected browser input to receive typed rune, got %q", got)
	}
}

func TestSettingsTextInputsAcceptOpenBracket(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.focusedField = sfBrowser
	s.browserInput.SetValue("")
	s.applyFocus()

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}}, DefaultKeys)

	if next.focusedField != sfBrowser {
		t.Fatalf("expected browser field to keep focus, got %v", next.focusedField)
	}
	if got := next.browserInput.Value(); got != "[" {
		t.Fatalf("expected browser input to receive typed rune, got %q", got)
	}
}

func TestSettingsBadgeWidthStaysStableAcrossFocus(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	chrome := newManagerChrome(62, CatppuccinMocha)

	focused := s.renderBadge("ON", true, chrome)
	unfocused := s.renderBadge("ON", false, chrome)

	if lipgloss.Width(focused) != lipgloss.Width(unfocused) {
		t.Fatalf("expected focused and unfocused badges to have equal width, got %d and %d", lipgloss.Width(focused), lipgloss.Width(unfocused))
	}
}

func TestSettingsLeftKeyMovesFocusToSidebar(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyLeft}, DefaultKeys)

	if next.focusedPane != settingsPaneSidebar {
		t.Fatalf("expected left key to move focus to sidebar, got %v", next.focusedPane)
	}
	if next.activeSection != ssDisplay {
		t.Fatalf("expected active section to remain DISPLAY, got %v", next.activeSection)
	}
}

func TestSettingsSidebarDownChangesSection(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setFocusedPane(settingsPaneSidebar)

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyDown}, DefaultKeys)

	if next.activeSection != ssFeeds {
		t.Fatalf("expected active section to move to FEEDS, got %v", next.activeSection)
	}
	if next.focusedField != sfFeedMaxBody {
		t.Fatalf("expected FEEDS section to restore feed max field, got %v", next.focusedField)
	}
}

func TestSettingsSidebarRightEntersDetail(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setFocusedPane(settingsPaneSidebar)
	s.setActiveSection(ssFeeds)

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyRight}, DefaultKeys)

	if next.focusedPane != settingsPaneDetail {
		t.Fatalf("expected right key to enter detail pane, got %v", next.focusedPane)
	}
	if next.focusedField != sfFeedMaxBody {
		t.Fatalf("expected FEEDS section to focus feed max field, got %v", next.focusedField)
	}
}

func TestSettingsRestoresLastFocusedFieldPerSection(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssDisplay)
	s.setFocusedField(sfBrowser)
	s.setActiveSection(ssAI)
	s.setFocusedField(sfSavePath)

	s.setActiveSection(ssDisplay)
	if s.focusedField != sfBrowser {
		t.Fatalf("expected DISPLAY section to restore browser focus, got %v", s.focusedField)
	}

	s.setActiveSection(ssAI)
	if s.focusedField != sfSavePath {
		t.Fatalf("expected AI section to restore save path focus, got %v", s.focusedField)
	}
}

func TestSettingsTextInputLeftArrowDoesNotMoveToSidebar(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssDisplay)
	s.setFocusedPane(settingsPaneDetail)
	s.setFocusedField(sfBrowser)
	s.browserInput.SetValue("abc")

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyLeft}, DefaultKeys)

	if next.focusedPane != settingsPaneDetail {
		t.Fatalf("expected text input to keep detail focus, got %v", next.focusedPane)
	}
	if next.activeSection != ssDisplay {
		t.Fatalf("expected active section to remain DISPLAY, got %v", next.activeSection)
	}
}

func TestSettingsAboutViewShowsLinksAndTagline(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)

	view := s.View(100, 30, newManagerChrome(100, CatppuccinMocha))

	for _, want := range []string{
		"ABOUT",
		"github.com/allisonhere",
		"Thanks for taking a look -allie",
		"Your feeds, no algorithm, no bullshit",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected about view to contain %q, got %q", want, view)
		}
	}
}

func TestSettingsAboutActions(t *testing.T) {
	tests := []struct {
		name       string
		field      settingsField
		wantAction settingsAction
	}{
		{name: "repo", field: sfAboutRepo, wantAction: settingsActionOpenRepo},
		{name: "issues", field: sfAboutIssues, wantAction: settingsActionOpenIssues},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSettings(config.DefaultConfig(), settingsUpdateState{})
			s.setActiveSection(ssAbout)
			s.setFocusedField(tt.field)

			next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter}, DefaultKeys)

			if got := next.takeAction(); got != tt.wantAction {
				t.Fatalf("expected action %v, got %v", tt.wantAction, got)
			}
		})
	}
}

func TestSettingsAboutAnimationTicksOnlyWhenVisible(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)

	next, cmd, _ := s.Update(settingsAboutPulseMsg{}, DefaultKeys)
	if next.aboutGradientFrame != 1 {
		t.Fatalf("expected about gradient frame to advance to 1, got %d", next.aboutGradientFrame)
	}
	if cmd == nil {
		t.Fatal("expected about animation tick to schedule another tick")
	}

	next.setActiveSection(ssDisplay)
	after, cmd, _ := next.Update(settingsAboutPulseMsg{}, DefaultKeys)
	if after.aboutGradientFrame != 0 {
		t.Fatalf("expected about gradient frame to reset outside ABOUT, got %d", after.aboutGradientFrame)
	}
	if cmd != nil {
		t.Fatal("expected no follow-up tick when ABOUT is not visible")
	}
}

func TestSettingsAboutAnimationDoesNotWrapEarly(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)

	for i := 0; i < 30; i++ {
		next, _, _ := s.Update(settingsAboutPulseMsg{}, DefaultKeys)
		s = next
	}

	if s.aboutGradientFrame != 30 {
		t.Fatalf("expected about gradient frame to continue smoothly past 24 ticks, got %d", s.aboutGradientFrame)
	}
}

func TestAboutMarineBackgroundChangesAcrossFrames(t *testing.T) {
	changed := false
	for _, col := range []int{4, 11, 19} {
		if aboutMarineBackground(0, 1, col, 24) != aboutMarineBackground(4, 1, col, 24) {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("expected about marine background to change across frames")
	}
}

func TestAboutSpotlightBrightensForeground(t *testing.T) {
	frame := 18
	spotCol := 26
	row := 2
	width := 48
	spot := aboutSpotlightSample(frame, row, spotCol, width)
	far := aboutSpotlightSample(frame, row, 0, width)
	if spot <= far {
		t.Fatalf("expected spotlight intensity %.2f to exceed far intensity %.2f", spot, far)
	}

	bg := aboutMarineBackground(frame, row, spotCol, width)
	spotFg := aboutMarineForeground(bg, spot)
	farFg := aboutMarineForeground(bg, far)
	if spotFg == farFg {
		t.Fatalf("expected spotlight foreground %q to differ from far foreground %q", spotFg, farFg)
	}
}

func TestAboutSpotlightIsApproximatelyRound(t *testing.T) {
	frame := 18
	width := 48
	cx, cy := aboutSpotlightCenter(frame, width)
	centerCol := int(math.Round(cx))
	centerRow := int(math.Round(cy))
	left := aboutSpotlightSample(frame, centerRow, centerCol-4, width)
	right := aboutSpotlightSample(frame, centerRow, centerCol+4, width)
	up := aboutSpotlightSample(frame, centerRow-1, centerCol, width)
	down := aboutSpotlightSample(frame, centerRow+1, centerCol, width)

	if math.Abs(left-right) > 0.12 {
		t.Fatalf("expected round spotlight horizontally, got left=%.2f right=%.2f", left, right)
	}
	if math.Abs(up-down) > 0.12 {
		t.Fatalf("expected round spotlight vertically, got up=%.2f down=%.2f", up, down)
	}
}

func TestAboutTaglineCodexSweepChangesAcrossFrames(t *testing.T) {
	changed := false
	for _, col := range []int{12, 20, 28} {
		if aboutTaglineCodexSweepSample(8, col, 48) != aboutTaglineCodexSweepSample(18, col, 48) {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("expected tagline codex sweep to change across frames")
	}
}

func TestAboutTaglineCodexHeadIsSharperThanSweep(t *testing.T) {
	frame := 18
	width := 48
	centerCol := int(math.Round(aboutTaglineCodexCenter(frame, width)))
	headNear := aboutTaglineCodexHeadSample(frame, centerCol, width)
	headFar := aboutTaglineCodexHeadSample(frame, centerCol+4, width)
	sweepFar := aboutTaglineCodexSweepSample(frame, centerCol+4, width)
	if headNear <= headFar {
		t.Fatalf("expected codex head %.2f to exceed far head %.2f", headNear, headFar)
	}
	if headFar >= sweepFar {
		t.Fatalf("expected codex head %.2f to fall off faster than sweep %.2f", headFar, sweepFar)
	}
}

func TestAboutTaglineCodexSweepIsVisiblyDifferent(t *testing.T) {
	bg := lipgloss.Color("#083a59")
	fg := lipgloss.Color("#eef8fb")
	sweep := 0.7
	head := 0.45
	if got := aboutTaglineCodexForeground(bg, fg, sweep, head); got == fg {
		t.Fatalf("expected codex foreground to differ from base foreground %q", fg)
	}
	if got := aboutTaglineCodexBackground(bg, sweep, head); got == bg {
		t.Fatalf("expected codex background to differ from base background %q", bg)
	}
}

func TestSettingsAboutTaglineStaysOnOneLine(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)
	s.aboutGradientFrame = 18
	view := s.renderAboutHero(56, newManagerChrome(84, CatppuccinMocha))
	if strings.Count(ansi.Strip(view), "Your feeds, no algorithm, no bullshit") != 1 {
		t.Fatalf("expected tagline to render once on a single line, got %q", ansi.Strip(view))
	}
}

func TestSettingsAboutHeroKeepsPreviousHeight(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)
	view := s.renderAboutHero(56, newManagerChrome(84, CatppuccinMocha))
	if got := lipgloss.Height(view); got != 5 {
		t.Fatalf("expected about hero height 5, got %d", got)
	}
}

func TestSettingsAboutGradientDoesNotShiftHeroLayout(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)
	chrome := newManagerChrome(84, CatppuccinMocha)

	before := s.renderAboutHero(56, chrome)
	s.aboutGradientFrame = 7
	after := s.renderAboutHero(56, chrome)

	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	if len(beforeLines) != len(afterLines) {
		t.Fatalf("line count changed across about animation frames: before=%d after=%d", len(beforeLines), len(afterLines))
	}
	for i := range beforeLines {
		if lipgloss.Width(beforeLines[i]) != lipgloss.Width(afterLines[i]) {
			t.Fatalf("line %d width changed across about animation frames: before=%d after=%d", i+1, lipgloss.Width(beforeLines[i]), lipgloss.Width(afterLines[i]))
		}
	}
}

func TestSettingsAboutLinksUseTwoColumnsWhenWide(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)
	chrome := newManagerChrome(96, CatppuccinMocha)

	wide := s.renderAboutLinks(60, chrome)
	narrow := s.renderAboutLinks(40, chrome)

	if lipgloss.Height(wide) >= lipgloss.Height(narrow) {
		t.Fatalf("expected wide about links to be shorter than stacked narrow layout: wide=%d narrow=%d", lipgloss.Height(wide), lipgloss.Height(narrow))
	}
}

func TestSettingsActionURLMapping(t *testing.T) {
	if got := settingsActionURL(settingsActionOpenRepo); got != tideRepoURL {
		t.Fatalf("expected repo action URL %q, got %q", tideRepoURL, got)
	}
	if got := settingsActionURL(settingsActionOpenIssues); got != tideIssuesURL {
		t.Fatalf("expected issues action URL %q, got %q", tideIssuesURL, got)
	}
}
