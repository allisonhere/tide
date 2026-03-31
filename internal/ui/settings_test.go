package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
