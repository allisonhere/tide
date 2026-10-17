package omarchy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BarModuleID is the id of Tide's unread-count module in the Omarchy bar.
const BarModuleID = "tide"

// barRefreshSeconds is how often the bar re-runs `tide --bar`. The shell has
// no way to nudge a command module, so this is also how quickly the count
// follows a feed being added or fetched. The command only reads the database.
const barRefreshSeconds = 5

// shellConfigPath is the user's Omarchy shell config, which owns the bar.
func shellConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "omarchy", "shell.json")
}

// defaultShellConfigPath is the packaged config the shell runs on until the
// user has a shell.json of their own.
func defaultShellConfigPath() string {
	root := os.Getenv("OMARCHY_PATH")
	if root == "" {
		root = "/usr/share/omarchy"
	}
	return filepath.Join(root, "config", "omarchy", "shell.json")
}

// BarAvailable reports whether this machine runs the Omarchy shell bar.
func BarAvailable() bool {
	for _, p := range []string{shellConfigPath(), defaultShellConfigPath()} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// BarModuleInstalled reports whether the bar layout carries Tide's module.
func BarModuleInstalled() bool {
	cfg, err := readShellConfig(shellConfigPath())
	if err != nil {
		return false
	}
	for _, section := range barSections(cfg) {
		for _, m := range section {
			if moduleID(m) == BarModuleID {
				return true
			}
		}
	}
	return false
}

// InstallBarModule adds Tide's unread count to the right of the bar, running
// executable with --bar. The user's shell.json is backed up first. Without
// one, it starts from Omarchy's defaults: the shell does not merge a user
// file over them, so writing only Tide's module would drop the stock bar.
func InstallBarModule(executable string) error {
	path := shellConfigPath()
	source := path
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		source = defaultShellConfigPath()
	}
	cfg, err := readShellConfig(source)
	if err != nil {
		return err
	}
	if BarModuleInstalled() {
		return refreshBarInterval(path)
	}

	bar := childObject(cfg, "bar")
	layout := childObject(bar, "layout")
	right, _ := layout["right"].([]any)
	module := map[string]any{
		"id":       BarModuleID,
		"type":     "command",
		"exec":     executable + " --bar",
		"interval": barRefreshSeconds,
		"tooltip":  "Tide",
		"onClick":  "omarchy-launch-or-focus-tui " + executable,
	}
	layout["right"] = append([]any{module}, right...)
	return writeShellConfig(path, cfg)
}

// refreshBarInterval brings an already-installed module's interval up to date,
// so installs from older versions stop polling slowly. It leaves the file
// alone when nothing needs changing.
func refreshBarInterval(path string) error {
	cfg, err := readShellConfig(path)
	if err != nil {
		return err
	}
	changed := false
	for _, section := range barSections(cfg) {
		for _, m := range section {
			obj, _ := m.(map[string]any)
			if moduleID(m) != BarModuleID {
				continue
			}
			if n, ok := obj["interval"].(json.Number); !ok || n.String() != fmt.Sprint(barRefreshSeconds) {
				obj["interval"] = barRefreshSeconds
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	return writeShellConfig(path, cfg)
}

// UninstallBarModule removes Tide's module from every bar section.
func UninstallBarModule() error {
	path := shellConfigPath()
	cfg, err := readShellConfig(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	layout := childObject(childObject(cfg, "bar"), "layout")
	removed := false
	for name, raw := range layout {
		section, ok := raw.([]any)
		if !ok {
			continue
		}
		kept := section[:0:0]
		for _, m := range section {
			if moduleID(m) == BarModuleID {
				removed = true
				continue
			}
			kept = append(kept, m)
		}
		layout[name] = kept
	}
	if !removed {
		return nil
	}
	return writeShellConfig(path, cfg)
}

func readShellConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // keep integers as written
	var cfg map[string]any
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, nil
}

// writeShellConfig backs up any existing file, then replaces it atomically
// so the shell, which reloads on save, never reads a half-written file.
func writeShellConfig(path string, cfg map[string]any) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		old, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		backup := fmt.Sprintf("%s.bak.%d", path, time.Now().Unix())
		if err := os.WriteFile(backup, old, mode); err != nil {
			return fmt.Errorf("back up %s: %w", path, err)
		}
	} else if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tide.tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func childObject(parent map[string]any, key string) map[string]any {
	child, ok := parent[key].(map[string]any)
	if !ok {
		child = map[string]any{}
		parent[key] = child
	}
	return child
}

func barSections(cfg map[string]any) [][]any {
	bar, _ := cfg["bar"].(map[string]any)
	layout, _ := bar["layout"].(map[string]any)
	var sections [][]any
	for _, raw := range layout {
		if section, ok := raw.([]any); ok {
			sections = append(sections, section)
		}
	}
	return sections
}

func moduleID(m any) string {
	obj, _ := m.(map[string]any)
	id, _ := obj["id"].(string)
	return strings.TrimSpace(id)
}
