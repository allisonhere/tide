package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"tide/internal/config"
	"tide/internal/db"
	"tide/internal/feed"
	"tide/internal/ui"
)

func main() {
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
