package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	installed, err = filepath.Abs(installed)
	if err != nil {
		return "", err
	}
	// This binary is not the one the bar would run, so check that one: a tide
	// that predates --bar ignores the flag and starts the reader, and the
	// terminal escape codes it prints become the bar's text.
	if err := checkBarOutput(installed); err != nil {
		return "", err
	}
	return installed, nil
}

// checkBarOutput runs executable --bar and requires the status JSON the bar
// reads, so a tide that cannot print it is never wired into the bar.
func checkBarOutput(executable string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, executable, "--bar").Output()
	var status struct {
		Class string `json:"class"`
	}
	if err != nil || json.Unmarshal(bytes.TrimSpace(out), &status) != nil || status.Class == "" {
		return fmt.Errorf("%s does not support --bar (it is older than this version); update Tide, then turn this on again", executable)
	}
	return nil
}
