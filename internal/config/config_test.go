package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigIncludesUpdateDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Updates.CheckOnStartup {
		t.Fatal("expected update checks to be enabled by default")
	}
	if cfg.Updates.CheckIntervalHours != 24 {
		t.Fatalf("expected 24 hour update interval, got %d", cfg.Updates.CheckIntervalHours)
	}
}

func TestLoadPreservesUpdateConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgPath := filepath.Join(dir, "rss", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := `
theme = "catppuccin-mocha"

[display]
icons = true
date_format = "relative"
mark_read_on_open = true
browser = ""

[feed]
max_body_mib = 10

[updates]
check_on_startup = false
check_interval_hours = 12
last_checked_unix = 1710000000
dismissed_version = "v1.2.3"
available_version = "v1.3.0"
available_summary = "New version available."
available_published_unix = 1710001234
`
	if err := os.WriteFile(cfgPath, []byte(data), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Updates.CheckOnStartup {
		t.Fatal("expected check_on_startup to be false")
	}
	if cfg.Updates.CheckIntervalHours != 12 {
		t.Fatalf("expected interval 12, got %d", cfg.Updates.CheckIntervalHours)
	}
	if cfg.Updates.LastCheckedUnix != 1710000000 {
		t.Fatalf("unexpected last_checked_unix: %d", cfg.Updates.LastCheckedUnix)
	}
	if cfg.Updates.DismissedVersion != "v1.2.3" {
		t.Fatalf("unexpected dismissed_version: %q", cfg.Updates.DismissedVersion)
	}
	if cfg.Updates.AvailableVersion != "v1.3.0" {
		t.Fatalf("unexpected available_version: %q", cfg.Updates.AvailableVersion)
	}
	if cfg.Updates.AvailableSummary != "New version available." {
		t.Fatalf("unexpected available_summary: %q", cfg.Updates.AvailableSummary)
	}
	if cfg.Updates.AvailablePublished != 1710001234 {
		t.Fatalf("unexpected available_published_unix: %d", cfg.Updates.AvailablePublished)
	}
}
