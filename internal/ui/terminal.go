package ui

import (
	"fmt"
	"os"

	"github.com/allisonhere/tideui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// openTTY opens /dev/tty for writing terminal control sequences safely,
// without going through BubbleTea's renderer-owned stdout.
func openTTY() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_WRONLY, 0)
}

// TerminalBackgroundSequences returns ANSI OSC sequences that set and later
// reset the terminal's default background color for xterm-compatible terminals.
// Unless transparent is set, the default is one step away from the theme's
// background: terminals such as kitty apply their background opacity to every
// cell whose color equals the default background, and Tide paints every cell in
// the theme background, so an exact match would make the whole UI see-through.
func TerminalBackgroundSequences(themeName string, transparent bool) (set string, reset string) {
	theme, _ := ThemeByName(themeName)
	return tideui.TerminalBackgroundSequencesFor(theme, transparent)
}

// setTermBgCmd returns a Cmd that sets the terminal background color via OSC 11.
// Writes to /dev/tty rather than os.Stdout because BubbleTea owns the stdout
// renderer; interleaved writes corrupt the display.
func setTermBgCmd(bg lipgloss.Color) tea.Cmd {
	return func() tea.Msg {
		if tty, err := openTTY(); err == nil {
			fmt.Fprintf(tty, "\x1b]11;%s\x07", string(bg))
			_ = tty.Close()
		}
		return nil
	}
}

// termBgCmd sets the terminal's default background for a theme background,
// honoring the transparent-background setting.
func (m Model) termBgCmd(bg lipgloss.Color) tea.Cmd {
	return setTermBgCmd(tideui.TerminalDefaultBg(bg, m.cfg.Display.Transparent))
}
