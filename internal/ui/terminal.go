package ui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TerminalBackgroundSequences returns ANSI OSC sequences that set and later
// reset the terminal's default background color for xterm-compatible terminals.
func TerminalBackgroundSequences(themeName string) (set string, reset string) {
	theme, _ := ThemeByName(themeName)
	if theme.Bg == "" {
		return "", ""
	}
	return fmt.Sprintf("\x1b]11;%s\x07", string(theme.Bg)), "\x1b]111\x07"
}

// setTermBgCmd returns a Cmd that sets the terminal background color via OSC 11.
func setTermBgCmd(bg lipgloss.Color) tea.Cmd {
	return func() tea.Msg {
		fmt.Fprintf(os.Stdout, "\x1b]11;%s\x07", string(bg))
		return nil
	}
}
