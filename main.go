package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tide/internal/db"
	"github.com/allisonhere/tide/internal/feed"
	"github.com/allisonhere/tide/internal/image"
	"github.com/allisonhere/tide/internal/ui"
)

var version = "dev"

func main() {
	code, restartExec := run()
	if restartExec != "" {
		argv := append([]string{restartExec}, os.Args[1:]...)
		if err := syscall.Exec(restartExec, argv, os.Environ()); err != nil {
			fmt.Fprintln(os.Stderr, "restart failed:", err)
			os.Exit(1)
		}
	}
	os.Exit(code)
}

// run owns resources that must be released before an in-app update restart.
// main re-execs only after this function returns, once Bubble Tea has restored
// the terminal and Tide has closed its database and reset terminal colors.
func run() (code int, restartExec string) {
	previewManualUpdate := false
	for _, a := range os.Args[1:] {
		switch strings.TrimSpace(a) {
		case "--version", "-version", "-v":
			fmt.Printf("tide %s\n", resolvedVersion())
			return 0, ""
		case "--preview-manual-update":
			previewManualUpdate = true
		}
	}

	database, err := db.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error opening database:", err)
		return 1, ""
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

	// --preview-manual-update: open Settings on Updates with a demo manual-install command (dev UI).
	model := ui.NewModel(database, cfg, resolvedVersion(), previewManualUpdate)

	// Article images: detect terminal graphics support once, before Bubble Tea
	// takes over the terminal. The active probe only runs when the feature is
	// enabled, so an unrelated startup pays nothing. The renderer emits escape
	// sequences into the Bubble Tea frame, so it needs no side channel.
	imgCap := image.Detect(os.Getenv, cfg.Display.ArticleImages)
	if cfg.Display.ArticleImages && imgCap.Supported {
		model.EnableImages(imgCap, image.NewKittyRenderer())
	} else {
		model.EnableImages(imgCap, nil)
	}
	// Belt-and-suspenders cleanup: after Bubble Tea releases the terminal, purge
	// any lingering images. Leaving the alt-screen clears them on most terminals.
	cleanupImages := func() { io.WriteString(os.Stdout, image.DeleteAllSequence()) }
	defer cleanupImages()

	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	defer func() {
		if r := recover(); r != nil {
			p.Kill()
			cleanupImages()
			fmt.Fprintln(os.Stderr, "panic:", r)
			code = 1
		}
	}()

	finalModel, err := p.Run()
	if err != nil {
		cleanupImages()
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1, ""
	}
	if model, ok := finalModel.(ui.Model); ok {
		restartExec = model.RestartExecPath()
	}
	return 0, restartExec
}

func resolvedVersion() string {
	if version != "" && version != "dev" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if ok {
		if resolved := resolvedVersionFromBuildInfo(info); resolved != "" {
			return resolved
		}
	}
	if desc := gitDescribeVersion(); desc != "" {
		return desc
	}

	return version
}

func resolvedVersionFromBuildInfo(info *debug.BuildInfo) string {
	if info == nil {
		return ""
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimSpace(info.Main.Version)
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
	if revision == "" {
		return ""
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}

func gitDescribeVersion() string {
	out, err := exec.Command("git", "describe", "--tags", "--dirty", "--always").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
