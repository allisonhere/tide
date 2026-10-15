package ui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tide/internal/omarchy"
)

// omarchyBarCmd adds Tide's unread count to the Omarchy bar, or removes it.
func omarchyBarCmd(enable bool) tea.Cmd {
	return func() tea.Msg {
		if !enable {
			return OmarchyBarUpdatedMsg{Err: omarchy.UninstallBarModule()}
		}
		executable, err := barExecutable()
		if err != nil {
			return OmarchyBarUpdatedMsg{Enabled: true, Err: err}
		}
		return OmarchyBarUpdatedMsg{Enabled: true, Err: omarchy.InstallBarModule(executable)}
	}
}

// barExecutable is the tide binary the bar should run: this one, by absolute
// path because the bar does not share the shell's PATH. A `go run` build lives
// in a temporary directory that is gone once it exits, so then it falls back
// to the tide on PATH.
func barExecutable() (string, error) {
	self, err := os.Executable()
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(self); err == nil {
			self = resolved
		}
		if !strings.HasPrefix(self, filepath.Clean(os.TempDir())+string(filepath.Separator)) {
			return self, nil
		}
	}
	installed, err := exec.LookPath("tide")
	if err != nil {
		return "", errors.New("no installed tide to run from the bar; install Tide first")
	}
	return filepath.Abs(installed)
}
