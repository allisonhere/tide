package omarchy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// barEnv points the user config and Omarchy's packaged defaults at temp dirs
// and returns the user shell.json path.
func barEnv(t *testing.T, defaults string) string {
	t.Helper()
	cfgHome := t.TempDir()
	omarchyRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("OMARCHY_PATH", omarchyRoot)
	if defaults != "" {
		p := filepath.Join(omarchyRoot, "config", "omarchy", "shell.json")
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(defaults), 0o644)
	}
	return filepath.Join(cfgHome, "omarchy", "shell.json")
}

func rightIDs(t *testing.T, path string) []string {
	t.Helper()
	cfg, err := readShellConfig(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	right, _ := cfg["bar"].(map[string]any)["layout"].(map[string]any)["right"].([]any)
	var ids []string
	for _, m := range right {
		ids = append(ids, moduleID(m))
	}
	return ids
}

func TestInstallStartsFromDefaultsWhenUserHasNoConfig(t *testing.T) {
	user := barEnv(t, `{"version":1,"bar":{"layout":{"right":[{"id":"omarchy.clock"}]}}}`)
	if !BarAvailable() || BarModuleInstalled() {
		t.Fatal("expected the bar available and the module absent")
	}
	if err := InstallBarModule("/usr/bin/tide"); err != nil {
		t.Fatalf("InstallBarModule: %v", err)
	}
	if got := rightIDs(t, user); strings.Join(got, ",") != "tide,omarchy.clock" {
		t.Fatalf("expected Tide added ahead of the stock modules, got %q", got)
	}
	data, _ := os.ReadFile(user)
	if !strings.Contains(string(data), `"exec": "/usr/bin/tide --bar"`) || !strings.Contains(string(data), `"version": 1`) {
		t.Fatalf("unexpected config:\n%s", data)
	}
}

func TestInstallIsIdempotentAndBacksUp(t *testing.T) {
	user := barEnv(t, "")
	os.MkdirAll(filepath.Dir(user), 0o755)
	os.WriteFile(user, []byte(`{"bar":{"layout":{"left":[{"id":"omarchy.menu","size":12}],"right":[]}}}`), 0o600)

	for range 2 {
		if err := InstallBarModule("/usr/bin/tide"); err != nil {
			t.Fatalf("InstallBarModule: %v", err)
		}
	}
	if got := rightIDs(t, user); len(got) != 1 || got[0] != BarModuleID {
		t.Fatalf("expected one Tide module, got %q", got)
	}
	backups, _ := filepath.Glob(user + ".bak.*")
	if len(backups) != 1 {
		t.Fatalf("expected one backup of the original, got %v", backups)
	}
	data, _ := os.ReadFile(user)
	var cfg map[string]any
	json.Unmarshal(data, &cfg)
	if !strings.Contains(string(data), `"size": 12`) {
		t.Fatalf("expected other modules kept as written:\n%s", data)
	}
	if info, _ := os.Stat(user); info.Mode().Perm() != 0o600 {
		t.Fatalf("expected file mode kept, got %v", info.Mode().Perm())
	}
}

func TestUninstallRemovesOnlyTide(t *testing.T) {
	user := barEnv(t, "")
	os.MkdirAll(filepath.Dir(user), 0o755)
	os.WriteFile(user, []byte(`{"bar":{"layout":{"right":[{"id":"tide"},{"id":"omarchy.clock"}]}}}`), 0o644)

	if !BarModuleInstalled() {
		t.Fatal("expected the module detected")
	}
	if err := UninstallBarModule(); err != nil {
		t.Fatalf("UninstallBarModule: %v", err)
	}
	if got := rightIDs(t, user); len(got) != 1 || got[0] != "omarchy.clock" {
		t.Fatalf("expected only Tide removed, got %q", got)
	}
}

func TestBarUnavailableWithoutOmarchy(t *testing.T) {
	barEnv(t, "")
	if BarAvailable() {
		t.Fatal("expected no bar without any shell config")
	}
	if err := UninstallBarModule(); err != nil {
		t.Fatalf("expected uninstall to be a no-op, got %v", err)
	}
}
