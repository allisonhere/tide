package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tide/internal/config"
)

func TestSettingsTextInputsAcceptMovementRunes(t *testing.T) {
	s := newSettings(config.DefaultConfig())
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
	s := newSettings(config.DefaultConfig())
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
