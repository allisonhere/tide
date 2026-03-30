package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tide/internal/config"
	"tide/internal/db"
	"tide/internal/feed"
	"tide/internal/ui"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch strings.TrimSpace(os.Args[1]) {
		case "--version", "-version", "-v":
			fmt.Printf("tide %s\n", resolvedVersion())
			return
		}
	}

	database, err := db.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error opening database:", err)
		os.Exit(1)
	}
	defer database.Close()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not load config:", err)
		cfg = config.DefaultConfig()
	}

	if setBG, resetBG := ui.TerminalBackgroundSequences(cfg.Theme); setBG != "" {
		fmt.Print(setBG)
		defer fmt.Print(resetBG)
	}
	feed.SetMaxFeedBodyBytes(cfg.Feed.MaxBodyMiB << 20)

	model := ui.NewModel(database, cfg)
	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	defer func() {
		if r := recover(); r != nil {
			p.Kill()
			fmt.Fprintln(os.Stderr, "panic:", r)
			os.Exit(1)
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func resolvedVersion() string {
	if version != "" && version != "dev" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}

	revision := ""
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision != "" {
		if len(revision) > 7 {
			revision = revision[:7]
		}
		if modified {
			revision += "-dirty"
		}
		return revision
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	if out, err := exec.Command("git", "describe", "--always", "--dirty").Output(); err == nil {
		if desc := strings.TrimSpace(string(out)); desc != "" {
			return desc
		}
	}

	return version
}
