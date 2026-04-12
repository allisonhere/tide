package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tide/internal/config"
	"tide/internal/db"
	"tide/internal/ui"
)

func main() {
	tmp := os.TempDir()
	_ = os.Setenv("XDG_DATA_HOME", tmp)
	_ = os.Setenv("XDG_CONFIG_HOME", tmp)

	database, err := db.Open()
	if err != nil {
		panic(err)
	}
	defer database.Close()

	model := ui.NewModel(database, config.DefaultConfig(), "dev", false)
	next, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = next.(ui.Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = next.(ui.Model)

	fmt.Print(ansi.Strip(model.View()))
}
