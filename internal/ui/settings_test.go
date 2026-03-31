package ui

import (
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

func TestAboutWaveImpactPhaseSequence(t *testing.T) {
	tests := []struct {
		delta int
		want  aboutWavePhase
	}{
		{delta: -2, want: aboutWavePhaseNone},
		{delta: -1, want: aboutWavePhasePreImpact},
		{delta: 0, want: aboutWavePhaseImpact},
		{delta: 1, want: aboutWavePhaseWakeStrong},
		{delta: 2, want: aboutWavePhaseWakeSoft},
		{delta: 3, want: aboutWavePhaseNone},
	}

	for _, tt := range tests {
		if got := aboutWaveImpactPhase(tt.delta); got != tt.want {
			t.Fatalf("delta %d: expected phase %v, got %v", tt.delta, tt.want, got)
		}
	}
}

func TestSettingsAboutWaveBounceLiftsCharacterAtCrest(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)
	s.aboutGradientFrame = 9

	crest, body := s.renderAboutBouncyWaveLine("WAVE", 12, 0, true)
	crestRunes := []rune(ansi.Strip(crest))
	bodyRunes := []rune(ansi.Strip(body))

	if crestRunes[1] != 'A' {
		t.Fatalf("expected crest line to lift the rune under the wave crest, got %q", string(crestRunes))
	}
	if bodyRunes[1] != 'A' {
		t.Fatalf("expected body line to keep a ghosted baseline rune for a half-bounce effect, got %q", string(bodyRunes))
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
