package ui

import "fmt"

// TerminalBackgroundSequences returns ANSI OSC sequences that set and later
// reset the terminal's default background color for xterm-compatible terminals.
func TerminalBackgroundSequences(themeName string) (set string, reset string) {
	theme, _ := ThemeByName(themeName)
	if theme.Bg == "" {
		return "", ""
	}
	return fmt.Sprintf("\x1b]11;%s\x07", string(theme.Bg)), "\x1b]111\x07"
}
