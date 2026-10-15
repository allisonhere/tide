package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tide/internal/db"
)

// omarchyBarEnv fakes an Omarchy install with stock bar defaults, an empty
// user config dir, and an installed tide on PATH. It returns the user
// shell.json path.
func omarchyBarEnv(t *testing.T) string {
	t.Helper()
	cfgHome, omarchyRoot, binDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("OMARCHY_PATH", omarchyRoot)
	t.Setenv("PATH", binDir)
	defaults := filepath.Join(omarchyRoot, "config", "omarchy", "shell.json")
	os.MkdirAll(filepath.Dir(defaults), 0o755)
	os.WriteFile(defaults, []byte(`{"bar":{"layout":{"right":[{"id":"omarchy.clock"}]}}}`), 0o644)
	current := "#!/bin/sh\necho '{\"text\":\"\",\"tooltip\":\"\",\"class\":\"empty\"}'\n"
	os.WriteFile(filepath.Join(binDir, "tide"), []byte(current), 0o755)
	return filepath.Join(cfgHome, "omarchy", "shell.json")
}

func TestOmarchyBarToggleOnlyOfferedWithOmarchy(t *testing.T) {
	omarchyBarEnv(t)
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	if !containsField(s.sectionFields(ssFeeds), sfOmarchyBar) {
		t.Fatal("expected the bar toggle in Feeds when Omarchy is installed")
	}

	t.Setenv("OMARCHY_PATH", t.TempDir())
	s = newSettings(config.DefaultConfig(), settingsUpdateState{})
	if containsField(s.sectionFields(ssFeeds), sfOmarchyBar) {
		t.Fatal("expected no bar toggle without Omarchy")
	}
}

func containsField(fields []settingsField, f settingsField) bool {
	for _, x := range fields {
		if x == f {
			return true
		}
	}
	return false
}

func TestSavingOmarchyBarToggleUpdatesTheBar(t *testing.T) {
	shellJSON := omarchyBarEnv(t)
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	save := func(m Model, on bool) Model {
		t.Helper()
		m.settings = newSettings(m.cfg, m.settingsUpdateState())
		m.settings.omarchyBar = on
		m.settings.setFocusedPane(settingsPaneSidebar)
		m.overlay = overlaySettings
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = next.(Model)
		if cmd == nil {
			t.Fatal("expected the save to return commands")
		}
		batch, _ := cmd().(tea.BatchMsg)
		for _, sub := range batch {
			if sub == nil {
				continue
			}
			if msg, ok := sub().(OmarchyBarUpdatedMsg); ok {
				if msg.Err != nil || msg.Enabled != on {
					t.Fatalf("unexpected bar update %+v", msg)
				}
				next, _ := m.Update(msg)
				return next.(Model)
			}
		}
		t.Fatal("expected the save to update the Omarchy bar")
		return m
	}

	m := NewModel(database, config.DefaultConfig(), "v1.0.0", false)
	m = save(m, true)
	data, _ := os.ReadFile(shellJSON)
	if !strings.Contains(string(data), `"id": "tide"`) || !strings.Contains(string(data), "/tide --bar") || !strings.Contains(string(data), "omarchy.clock") {
		t.Fatalf("expected Tide added beside the stock bar:\n%s", data)
	}
	if !strings.Contains(m.statusMsg, "added") {
		t.Fatalf("expected a confirmation in the status line, got %q", m.statusMsg)
	}

	m = save(m, false)
	data, _ = os.ReadFile(shellJSON)
	if strings.Contains(string(data), `"id": "tide"`) {
		t.Fatalf("expected Tide removed from the bar:\n%s", data)
	}
}

// A tide that predates --bar ignores the flag and starts the reader, and its
// terminal escape codes end up as the bar's text. The toggle must refuse to
// point the bar at one, and leave the bar config alone.
func TestOmarchyBarRefusesATideThatCannotPrintBarStatus(t *testing.T) {
	shellJSON := omarchyBarEnv(t)
	// Replace the fake installed tide with an old one: no --bar, junk on stdout.
	binDir := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	old := "#!/bin/sh\nprintf '\\033]11;#1e1e2e\\007\\033_Ga=d,d=A\\033\\\\\\033]111\\007'\n"
	if err := os.WriteFile(filepath.Join(binDir, "tide"), []byte(old), 0o755); err != nil {
		t.Fatal(err)
	}

	msg, ok := omarchyBarCmd(true)().(OmarchyBarUpdatedMsg)
	if !ok || msg.Err == nil {
		t.Fatalf("expected the toggle to be refused, got %+v", msg)
	}
	if !strings.Contains(msg.Err.Error(), "--bar") {
		t.Fatalf("expected the error to say why, got %q", msg.Err)
	}
	if _, err := os.Stat(shellJSON); err == nil {
		t.Fatal("expected the bar config to be left untouched")
	}
}
