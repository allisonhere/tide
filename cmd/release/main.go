// Command release is Tide's interactive release driver.
//
// It deliberately leaves binary construction and GitHub Release creation to
// .github/workflows/release.yml. Locally it validates the checkout, commits the
// selected worktree, pushes main, and pushes the version tag that starts CI.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	screenConfigure screen = iota
	screenReview
	screenRunning
	screenDone
)

type bumpKind int

const (
	bumpPatch bumpKind = iota
	bumpMinor
	bumpMajor
)

var (
	versionRE = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)
	accent    = lipgloss.Color("#cba6f7")
	green     = lipgloss.Color("#a6e3a1")
	red       = lipgloss.Color("#f38ba8")
	yellow    = lipgloss.Color("#f9e2af")
	muted     = lipgloss.Color("#7f849c")
	title     = lipgloss.NewStyle().Bold(true).Foreground(accent)
	selected  = lipgloss.NewStyle().Foreground(accent).Bold(true)
	dim       = lipgloss.NewStyle().Foreground(muted)
	okStyle   = lipgloss.NewStyle().Foreground(green)
	errStyle  = lipgloss.NewStyle().Foreground(red)
	warnStyle = lipgloss.NewStyle().Foreground(yellow)
)

type repoInfo struct {
	root       string
	branch     string
	remoteURL  string
	latestTag  string
	status     []string
	authorName string
	authorMail string
	releaseURL string
	actionsURL string
}

type releaseStep struct {
	name string
	run  func() (string, error)
}

type stepResultMsg struct {
	index  int
	output string
	err    error
}

type model struct {
	info          repoInfo
	screen        screen
	width         int
	height        int
	focus         int
	bump          bumpKind
	nextVersion   string
	commitInput   textinput.Model
	spinner       spinner.Model
	steps         []releaseStep
	stepIndex     int
	stepState     []int // 0 pending, 1 running, 2 passed, 3 failed
	logs          []string
	failure       error
	published     bool
	initialErr    error
	confirmArmed  bool
	confirmExpiry time.Time
}

func main() {
	info, err := inspectRepo()
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 120
	ti.Width = 58

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(accent)

	m := model{
		info:        info,
		screen:      screenConfigure,
		bump:        bumpPatch,
		commitInput: ti,
		spinner:     sp,
		initialErr:  err,
	}
	m.refreshVersion()
	m.commitInput.SetValue("chore: release " + m.nextVersion)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "release UI:", err)
		os.Exit(1)
	}
}

func inspectRepo() (repoInfo, error) {
	rootOut, err := runOutput("", "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return repoInfo{}, errors.New("run this command from inside a Git repository")
	}
	root := strings.TrimSpace(rootOut)
	branch, err := runOutput(root, "git", "branch", "--show-current")
	if err != nil {
		return repoInfo{}, fmt.Errorf("read branch: %w", err)
	}
	remote, err := runOutput(root, "git", "remote", "get-url", "origin")
	if err != nil {
		return repoInfo{}, errors.New("the repository needs an origin remote")
	}
	tags, err := runOutput(root, "git", "tag", "--list", "v[0-9]*", "--sort=-version:refname")
	if err != nil {
		return repoInfo{}, fmt.Errorf("read tags: %w", err)
	}
	latest := firstLine(tags)
	if latest == "" {
		latest = "v0.0.0"
	}
	if _, err := parseVersion(latest); err != nil {
		return repoInfo{}, fmt.Errorf("latest tag %q is not semantic versioning", latest)
	}
	statusOut, err := runOutput(root, "git", "status", "--short")
	if err != nil {
		return repoInfo{}, fmt.Errorf("read worktree: %w", err)
	}
	authorName, _ := runOutput(root, "git", "config", "--get", "user.name")
	authorMail, _ := runOutput(root, "git", "config", "--get", "user.email")
	remoteURL := strings.TrimSpace(remote)
	slug := githubSlug(remoteURL)
	info := repoInfo{
		root:       root,
		branch:     strings.TrimSpace(branch),
		remoteURL:  remoteURL,
		latestTag:  latest,
		status:     nonEmptyLines(statusOut),
		authorName: strings.TrimSpace(authorName),
		authorMail: strings.TrimSpace(authorMail),
	}
	if slug != "" {
		info.releaseURL = "https://github.com/" + slug + "/releases/tag/"
		info.actionsURL = "https://github.com/" + slug + "/actions/workflows/release.yml"
	}
	return info, nil
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.commitInput.Width = max(20, min(70, msg.Width-28))
		return m, nil

	case spinner.TickMsg:
		if m.screen == screenRunning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case stepResultMsg:
		if m.screen != screenRunning || msg.index != m.stepIndex {
			return m, nil
		}
		m.logs = append(m.logs, formatStepLog(m.steps[msg.index].name, msg.output, msg.err))
		if msg.err != nil {
			m.stepState[msg.index] = 3
			m.failure = fmt.Errorf("%s: %w", m.steps[msg.index].name, msg.err)
			if strings.TrimSpace(msg.output) != "" {
				m.failure = fmt.Errorf("%s\n%s", m.failure, strings.TrimSpace(msg.output))
			}
			m.screen = screenDone
			return m, nil
		}
		m.stepState[msg.index] = 2
		m.stepIndex++
		if m.stepIndex >= len(m.steps) {
			m.published = true
			m.screen = screenDone
			return m, nil
		}
		m.stepState[m.stepIndex] = 1
		return m, tea.Batch(m.spinner.Tick, runStepCmd(m.stepIndex, m.steps[m.stepIndex]))

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.screen {
		case screenConfigure:
			return m.updateConfigure(msg)
		case screenReview:
			return m.updateReview(msg)
		case screenRunning:
			// Commands are intentionally not abandoned halfway through a push.
			return m, nil
		case screenDone:
			if msg.String() == "q" || msg.String() == "esc" || msg.String() == "enter" {
				return m, tea.Quit
			}
		}
	}

	if m.screen == screenConfigure && m.focus == 1 {
		var cmd tea.Cmd
		m.commitInput, cmd = m.commitInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) updateConfigure(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.focus == 1 {
		switch msg.String() {
		case "esc", "tab", "shift+tab", "up", "down":
			m.commitInput.Blur()
			m.focus = 0
			return m, nil
		case "enter":
			if strings.TrimSpace(m.commitInput.Value()) != "" && m.initialErr == nil {
				m.commitInput.Blur()
				m.screen = screenReview
			}
			return m, nil
		default:
			var cmd tea.Cmd
			m.commitInput, cmd = m.commitInput.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "q", "esc":
		return m, tea.Quit
	case "tab", "down", "j":
		m.focus = 1
	case "shift+tab", "up", "k":
		m.focus = 1
	case "left", "h":
		if m.focus == 0 {
			m.bump = (m.bump + 2) % 3
			m.refreshVersionAndDefaultMessage()
		}
	case "right", "l", " ":
		if m.focus == 0 {
			m.bump = (m.bump + 1) % 3
			m.refreshVersionAndDefaultMessage()
		}
	case "enter":
		m.focus = 1
	}

	if m.focus == 1 {
		m.commitInput.Focus()
		return m, textinput.Blink
	}
	m.commitInput.Blur()
	return m, nil
}

func (m model) updateReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "b":
		m.screen = screenConfigure
		m.confirmArmed = false
		return m, nil
	case "r":
		if m.info.branch != "main" || !m.info.identityReady() {
			return m, nil
		}
		now := time.Now()
		if !m.confirmArmed || now.After(m.confirmExpiry) {
			m.confirmArmed = true
			m.confirmExpiry = now.Add(8 * time.Second)
			return m, nil
		}
		m.steps = buildReleaseSteps(m.info, m.nextVersion, strings.TrimSpace(m.commitInput.Value()))
		m.stepState = make([]int, len(m.steps))
		m.stepIndex = 0
		m.stepState[0] = 1
		m.logs = nil
		m.failure = nil
		m.screen = screenRunning
		return m, tea.Batch(m.spinner.Tick, runStepCmd(0, m.steps[0]))
	}
	return m, nil
}

func (m *model) refreshVersion() {
	next, err := bumpVersion(m.info.latestTag, m.bump)
	if err != nil {
		m.initialErr = err
		return
	}
	m.nextVersion = next
}

func (m *model) refreshVersionAndDefaultMessage() {
	oldVersion := m.nextVersion
	oldDefault := "chore: release " + oldVersion
	m.refreshVersion()
	if strings.TrimSpace(m.commitInput.Value()) == "" || m.commitInput.Value() == oldDefault {
		m.commitInput.SetValue("chore: release " + m.nextVersion)
	}
}

func (m model) View() string {
	if m.initialErr != nil {
		return frame(m.width, title.Render("Tide release")+"\n\n"+errStyle.Render("✗ "+m.initialErr.Error())+"\n\n"+dim.Render("q quit"))
	}
	switch m.screen {
	case screenConfigure:
		return m.viewConfigure()
	case screenReview:
		return m.viewReview()
	case screenRunning:
		return m.viewRunning()
	default:
		return m.viewDone()
	}
}

func (m model) viewConfigure() string {
	var b strings.Builder
	b.WriteString(title.Render("Tide release"))
	b.WriteString("\n")
	b.WriteString(dim.Render("Prepare a tested semantic-version release through GitHub Actions."))
	b.WriteString("\n\n")
	b.WriteString(label("Repository"))
	b.WriteString(filepath.Base(m.info.root))
	b.WriteString("\n")
	b.WriteString(label("Branch"))
	if m.info.branch == "main" {
		b.WriteString(okStyle.Render(m.info.branch))
	} else {
		b.WriteString(warnStyle.Render(m.info.branch + " (release requires main)"))
	}
	b.WriteString("\n")
	b.WriteString(label("Latest tag"))
	b.WriteString(m.info.latestTag)
	b.WriteString("\n")
	b.WriteString(label("Changes"))
	b.WriteString(fmt.Sprintf("%d file(s)", len(m.info.status)))
	b.WriteString("\n")
	b.WriteString(label("Git author"))
	if m.info.identityReady() {
		b.WriteString(okStyle.Render(m.info.authorName + " <" + m.info.authorMail + ">"))
	} else {
		b.WriteString(warnStyle.Render("not configured"))
	}
	b.WriteString("\n\n")

	cursor := "  "
	if m.focus == 0 {
		cursor = selected.Render("› ")
	}
	b.WriteString(cursor + "Version bump      " + bumpPicker(m.bump) + "  →  " + selected.Render(m.nextVersion))
	b.WriteString("\n\n")
	cursor = "  "
	if m.focus == 1 {
		cursor = selected.Render("› ")
	}
	b.WriteString(cursor + "Commit message    ")
	b.WriteString(m.commitInput.View())
	b.WriteString("\n\n")
	if len(m.info.status) == 0 {
		b.WriteString(warnStyle.Render("No uncommitted files; the current HEAD will be tagged after validation."))
	} else {
		b.WriteString(dim.Render("The release commit stages every worktree change (git add -A)."))
	}
	b.WriteString("\n\n")
	b.WriteString(dim.Render("↑/↓ fields  ←/→ choose bump  enter review  esc quit"))
	return frame(m.width, b.String())
}

func (m model) viewReview() string {
	var b strings.Builder
	b.WriteString(title.Render("Review release"))
	b.WriteString("\n\n")
	b.WriteString(label("Version"))
	b.WriteString(selected.Render(m.nextVersion))
	b.WriteString("\n")
	b.WriteString(label("Commit"))
	b.WriteString(m.commitInput.Value())
	b.WriteString("\n")
	b.WriteString(label("Push"))
	b.WriteString("origin/main, then tag " + m.nextVersion)
	b.WriteString("\n")
	b.WriteString(label("Release"))
	b.WriteString("GitHub Actions builds archives, checksums, and release notes")
	b.WriteString("\n\n")
	b.WriteString(dim.Render("Files that will be staged"))
	b.WriteString("\n")
	if len(m.info.status) == 0 {
		b.WriteString("  (none — current HEAD will be released)\n")
	} else {
		limit := min(len(m.info.status), max(3, m.height-18))
		for _, line := range m.info.status[:limit] {
			b.WriteString("  " + line + "\n")
		}
		if limit < len(m.info.status) {
			b.WriteString(dim.Render(fmt.Sprintf("  … and %d more", len(m.info.status)-limit)) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(dim.Render("Pipeline: test → diff check → sync check → stage → commit → push → tag → release"))
	b.WriteString("\n\n")
	if m.info.branch != "main" {
		b.WriteString(errStyle.Render("Cannot release: switch to main first."))
		b.WriteString("\n\n")
	}
	if !m.info.identityReady() {
		b.WriteString(errStyle.Render("Cannot release: configure git user.name and user.email first."))
		b.WriteString("\n\n")
	}
	if m.confirmArmed && time.Now().Before(m.confirmExpiry) {
		b.WriteString(warnStyle.Render("Press r again to publish " + m.nextVersion + "."))
	} else {
		b.WriteString(dim.Render("r release  esc back  q quit"))
	}
	return frame(m.width, b.String())
}

func (m model) viewRunning() string {
	var b strings.Builder
	b.WriteString(title.Render("Publishing " + m.nextVersion))
	b.WriteString("\n\n")
	for i, step := range m.steps {
		icon := dim.Render("○")
		switch m.stepState[i] {
		case 1:
			icon = m.spinner.View()
		case 2:
			icon = okStyle.Render("✓")
		case 3:
			icon = errStyle.Render("✗")
		}
		b.WriteString(fmt.Sprintf(" %s  %s\n", icon, step.name))
	}
	if len(m.logs) > 0 {
		b.WriteString("\n")
		b.WriteString(dim.Render("Latest output"))
		b.WriteString("\n")
		lines := nonEmptyLines(m.logs[len(m.logs)-1])
		start := max(0, len(lines)-max(3, m.height-len(m.steps)-10))
		for _, line := range lines[start:] {
			b.WriteString(dim.Render("  " + truncate(line, max(20, m.width-8))))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(dim.Render("Please keep this terminal open until the tag is pushed."))
	return frame(m.width, b.String())
}

func (m model) viewDone() string {
	var b strings.Builder
	if m.published {
		b.WriteString(okStyle.Bold(true).Render("✓ " + m.nextVersion + " release started"))
		b.WriteString("\n\n")
		b.WriteString("The tag is on GitHub and the release workflow is building the platform archives.")
		if m.info.actionsURL != "" {
			b.WriteString("\n\n")
			b.WriteString(label("Actions"))
			b.WriteString(m.info.actionsURL)
		}
		if m.info.releaseURL != "" {
			b.WriteString("\n")
			b.WriteString(label("Release"))
			b.WriteString(m.info.releaseURL + m.nextVersion)
		}
	} else {
		b.WriteString(errStyle.Bold(true).Render("✗ Release stopped"))
		b.WriteString("\n\n")
		b.WriteString(errStyle.Render(m.failure.Error()))
		b.WriteString("\n\n")
		if m.stepIndex < len(m.steps) && m.steps[m.stepIndex].name == "Push release tag" {
			b.WriteString(warnStyle.Render("The local tag may exist. Resume with:"))
			b.WriteString("\n  git push origin refs/tags/" + m.nextVersion)
		} else {
			b.WriteString("Fix the reported issue, then run the release TUI again.")
		}
	}
	b.WriteString("\n\n")
	b.WriteString(dim.Render("enter/q close"))
	return frame(m.width, b.String())
}

func buildReleaseSteps(info repoInfo, version, commitMessage string) []releaseStep {
	root := info.root
	return []releaseStep{
		{
			name: "Verify Git author identity",
			run: func() (string, error) {
				name, _ := runOutput(root, "git", "config", "--get", "user.name")
				email, _ := runOutput(root, "git", "config", "--get", "user.email")
				name, email = strings.TrimSpace(name), strings.TrimSpace(email)
				if name == "" || email == "" {
					return "", errors.New(`Git author identity is missing; run:
  git config user.name "Your Name"
  git config user.email "you@example.com"`)
				}
				return name + " <" + email + ">", nil
			},
		},
		commandStep("Run test suite", root, "go", "test", "./..."),
		commandStep("Check patch whitespace", root, "git", "diff", "--check"),
		commandStep("Fetch tags and main", root, "git", "fetch", "--tags", "origin", "main"),
		{
			name: "Verify branch and remote state",
			run: func() (string, error) {
				if info.branch != "main" {
					return "", fmt.Errorf("releases must be made from main, not %s", info.branch)
				}
				if out, err := runOutput(root, "git", "rev-parse", "-q", "--verify", "refs/tags/"+version); err == nil {
					return out, fmt.Errorf("tag %s already exists locally", version)
				}
				counts, err := runOutput(root, "git", "rev-list", "--left-right", "--count", "HEAD...origin/main")
				if err != nil {
					return counts, fmt.Errorf("compare with origin/main: %w", err)
				}
				parts := strings.Fields(counts)
				if len(parts) != 2 {
					return counts, errors.New("could not interpret branch comparison")
				}
				behind, _ := strconv.Atoi(parts[1])
				if behind > 0 {
					return counts, fmt.Errorf("main is behind origin/main by %d commit(s); pull or rebase first", behind)
				}
				return "main is ready; target tag is available", nil
			},
		},
		{
			name: "Verify worktree is unchanged",
			run: func() (string, error) {
				status, err := runOutput(root, "git", "status", "--short")
				if err != nil {
					return status, fmt.Errorf("read worktree: %w", err)
				}
				before := strings.Join(info.status, "\n")
				if strings.TrimSpace(status) != strings.TrimSpace(before) {
					return status, errors.New("worktree changed after the review; rerun the TUI to inspect it")
				}
				return fmt.Sprintf("%d reviewed file(s)", len(info.status)), nil
			},
		},
		commandStep("Stage worktree", root, "git", "add", "-A"),
		{
			name: "Create release commit",
			run: func() (string, error) {
				if err := exec.Command("git", "-C", root, "diff", "--cached", "--quiet").Run(); err == nil {
					return "No changes to commit; releasing current HEAD.", nil
				}
				return runOutput(root, "git", "commit", "-m", commitMessage)
			},
		},
		commandStep("Push main", root, "git", "push", "origin", "HEAD:main"),
		commandStep("Create annotated tag", root, "git", "tag", "-a", version, "-m", "Release "+version),
		commandStep("Push release tag", root, "git", "push", "origin", "refs/tags/"+version),
	}
}

func (r repoInfo) identityReady() bool {
	return strings.TrimSpace(r.authorName) != "" && strings.TrimSpace(r.authorMail) != ""
}

func commandStep(name, dir, command string, args ...string) releaseStep {
	return releaseStep{
		name: name,
		run: func() (string, error) {
			return runOutput(dir, command, args...)
		},
	}
}

func runStepCmd(index int, step releaseStep) tea.Cmd {
	return func() tea.Msg {
		output, err := step.run()
		return stepResultMsg{index: index, output: output, err: err}
	}
}

func runOutput(dir, command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return strings.TrimSpace(output.String()), err
}

type semVersion struct {
	major int
	minor int
	patch int
}

func parseVersion(raw string) (semVersion, error) {
	match := versionRE.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return semVersion{}, fmt.Errorf("%q is not a vMAJOR.MINOR.PATCH version", raw)
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return semVersion{major: major, minor: minor, patch: patch}, nil
}

func bumpVersion(raw string, bump bumpKind) (string, error) {
	v, err := parseVersion(raw)
	if err != nil {
		return "", err
	}
	switch bump {
	case bumpMajor:
		v.major++
		v.minor = 0
		v.patch = 0
	case bumpMinor:
		v.minor++
		v.patch = 0
	default:
		v.patch++
	}
	return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch), nil
}

func githubSlug(remote string) string {
	remote = strings.TrimSpace(strings.TrimSuffix(remote, ".git"))
	switch {
	case strings.HasPrefix(remote, "git@github.com:"):
		return strings.TrimPrefix(remote, "git@github.com:")
	case strings.HasPrefix(remote, "ssh://git@github.com/"):
		return strings.TrimPrefix(remote, "ssh://git@github.com/")
	case strings.HasPrefix(remote, "https://github.com/"):
		return strings.TrimPrefix(remote, "https://github.com/")
	default:
		return ""
	}
}

func firstLine(s string) string {
	lines := nonEmptyLines(s)
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}

func nonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func formatStepLog(name, output string, err error) string {
	if output == "" {
		output = "completed"
	}
	if err != nil {
		return name + ": " + output + "\n" + err.Error()
	}
	return name + ": " + output
}

func bumpPicker(current bumpKind) string {
	names := []string{"patch", "minor", "major"}
	var out []string
	for i, name := range names {
		if bumpKind(i) == current {
			out = append(out, selected.Render("‹ "+name+" ›"))
		} else {
			out = append(out, dim.Render(name))
		}
	}
	return strings.Join(out, "  ")
}

func label(s string) string {
	return dim.Width(16).Render(s)
}

func frame(width int, content string) string {
	w := min(max(20, width-4), 96)
	return lipgloss.NewStyle().
		Width(w).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Render(content)
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
