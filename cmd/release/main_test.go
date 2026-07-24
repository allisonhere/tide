package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestBumpVersion(t *testing.T) {
	tests := []struct {
		name string
		kind bumpKind
		want string
	}{
		{"patch", bumpPatch, "v1.2.4"},
		{"minor", bumpMinor, "v1.3.0"},
		{"major", bumpMajor, "v2.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bumpVersion("v1.2.3", tt.kind)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseVersionRejectsNonReleaseTags(t *testing.T) {
	for _, value := range []string{"", "1.2", "release-1.2.3", "v1.2.3-beta"} {
		if _, err := parseVersion(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestGitHubSlug(t *testing.T) {
	for remote, want := range map[string]string{
		"git@github.com:allisonhere/tide.git":       "allisonhere/tide",
		"https://github.com/allisonhere/tide.git":   "allisonhere/tide",
		"ssh://git@github.com/allisonhere/tide.git": "allisonhere/tide",
		"https://example.com/allisonhere/tide.git":  "",
	} {
		if got := githubSlug(remote); got != want {
			t.Fatalf("githubSlug(%q) = %q, want %q", remote, got, want)
		}
	}
}

func TestReleasePipelineEndsByPushingTag(t *testing.T) {
	info := repoInfo{root: t.TempDir(), branch: "main"}
	steps := buildReleaseSteps(info, "v1.2.3", "chore: release v1.2.3")
	if len(steps) < 8 {
		t.Fatalf("expected complete release pipeline, got %d steps", len(steps))
	}
	if got := steps[len(steps)-1].name; got != "Push release tag" {
		t.Fatalf("last step = %q, want Push release tag", got)
	}
}

func TestCommitInputAcceptsNavigationLetters(t *testing.T) {
	input := textinput.New()
	input.SetValue("release ")
	input.CursorEnd()
	input.Focus()
	m := model{
		screen:      screenConfigure,
		focus:       1,
		commitInput: input,
	}
	for _, char := range []rune("qhjklr") {
		next, _ := m.updateConfigure(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		m = next.(model)
	}
	if got, want := m.commitInput.Value(), "release qhjklr"; got != want {
		t.Fatalf("commit input = %q, want %q", got, want)
	}
	if m.screen != screenConfigure {
		t.Fatalf("typing in commit input changed screen to %v", m.screen)
	}
}

func TestReviewCannotStartReleaseOutsideMain(t *testing.T) {
	m := model{
		info:        repoInfo{branch: "feature"},
		screen:      screenReview,
		commitInput: textinput.New(),
		nextVersion: "v1.2.3",
	}
	next, cmd := m.updateReview(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := next.(model)
	if cmd != nil || got.confirmArmed || got.screen != screenReview {
		t.Fatal("release should remain blocked outside main")
	}
}

func TestReleasePipelineAgainstLocalRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	remote := filepath.Join(t.TempDir(), "remote.git")

	mustRun(t, "", "git", "init", "--bare", remote)
	mustRun(t, root, "git", "init", "-b", "main")
	mustRun(t, root, "git", "config", "user.name", "Release Test")
	mustRun(t, root, "git", "config", "user.email", "release@example.com")
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/release-test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, root, "git", "add", "-A")
	mustRun(t, root, "git", "commit", "-m", "initial")
	mustRun(t, root, "git", "remote", "add", "origin", remote)
	mustRun(t, root, "git", "push", "-u", "origin", "main")

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# release test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := runOutput(root, "git", "status", "--short")
	if err != nil {
		t.Fatal(err)
	}
	info := repoInfo{
		root:       root,
		branch:     "main",
		status:     nonEmptyLines(status),
		authorName: "Release Test",
		authorMail: "release@example.com",
	}
	for _, step := range buildReleaseSteps(info, "v1.0.1", "chore: release v1.0.1") {
		if output, err := step.run(); err != nil {
			t.Fatalf("%s failed: %v\n%s", step.name, err, output)
		}
	}

	mustRun(t, "", "git", "--git-dir", remote, "show-ref", "--verify", "refs/tags/v1.0.1")
	log := mustRun(t, root, "git", "log", "-1", "--pretty=%s")
	if log != "chore: release v1.0.1" {
		t.Fatalf("release commit = %q", log)
	}
}

func TestReviewCannotStartReleaseWithoutGitIdentity(t *testing.T) {
	m := model{
		info:        repoInfo{branch: "main"},
		screen:      screenReview,
		commitInput: textinput.New(),
		nextVersion: "v1.2.3",
	}
	next, cmd := m.updateReview(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := next.(model)
	if cmd != nil || got.confirmArmed || got.screen != screenReview {
		t.Fatal("release should remain blocked without a Git identity")
	}
}

func mustRun(t *testing.T, dir, command string, args ...string) string {
	t.Helper()
	output, err := runOutput(dir, command, args...)
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", command, args, err, output)
	}
	return output
}
