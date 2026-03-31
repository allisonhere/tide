package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tide/internal/config"
)

// ── Field index ───────────────────────────────────────────────────────────────

type settingsField int

const (
	sfIcons settingsField = iota
	sfDateFormat
	sfMarkReadOnOpen
	sfBrowser
	sfFeedMaxBody
	sfUpdateCheckOnStartup
	sfUpdateCheckNow
	sfUpdateInstallNow
	sfUpdateDismissVersion
	sfUpdateRestartNow
	sfAboutRepo
	sfAboutIssues
	sfProvider
	sfAPIKey    // visible when provider is openai/claude/gemini
	sfOllamaURL // visible when provider is ollama
	sfOllamaModel
	sfSavePath
)

type settingsSection int

const (
	ssDisplay settingsSection = iota
	ssFeeds
	ssUpdates
	ssAI
	ssAbout
	settingsSectionCount
)

type settingsPaneFocus int

const (
	settingsPaneSidebar settingsPaneFocus = iota
	settingsPaneDetail
)

type settingsAction int

const (
	settingsActionNone settingsAction = iota
	settingsActionCheckUpdates
	settingsActionInstallUpdate
	settingsActionDismissVersion
	settingsActionRestartAfterUpdate
	settingsActionOpenRepo
	settingsActionOpenIssues
)

const (
	tideRepoURL              = "https://github.com/allisonhere/tide"
	tideIssuesURL            = tideRepoURL + "/issues"
	settingsAboutPulsePeriod = 120 * time.Millisecond
	settingsAboutTwoColMinW  = 56
	settingsAboutCardGap     = 2
	settingsAboutFrameReset  = 4096
)

type settingsAboutPulseMsg struct{}

type settingsUpdateState struct {
	currentVersion   string
	state            updateState
	latestVersion    string
	publishedAt      time.Time
	summary          string
	lastChecked      time.Time
	err              string
	dismissed        bool
	manualCommand    string
	restartable      bool
	installedVersion string
}

var (
	aiProviderLabels      = []string{"none", "OpenAI", "Claude", "Gemini", "Ollama"}
	aiProviderIDs         = []string{"", "openai", "claude", "gemini", "ollama"}
	settingsSectionLabels = [settingsSectionCount]string{
		"DISPLAY",
		"FEEDS",
		"UPDATES",
		"AI",
		"ABOUT",
	}
)

func providerIndex(id string) int {
	for i, p := range aiProviderIDs {
		if p == id {
			return i
		}
	}
	return 0
}

// ── Settings ──────────────────────────────────────────────────────────────────

type Settings struct {
	// Display
	icons                bool
	dateAbsolute         bool // false = relative, true = absolute
	markReadOnOpen       bool
	browserInput         textinput.Model
	feedMaxBodyInput     textinput.Model
	updateCheckOnStartup bool
	update               settingsUpdateState
	action               settingsAction

	// AI
	providerIdx      int
	openaiInput      textinput.Model
	claudeInput      textinput.Model
	geminiInput      textinput.Model
	ollamaURLInput   textinput.Model
	ollamaModelInput textinput.Model
	savePathInput    textinput.Model

	activeSection      settingsSection
	focusedPane        settingsPaneFocus
	sectionField       [settingsSectionCount]settingsField
	focusedField       settingsField
	aboutGradientFrame int
	shouldSave         bool
	shouldExit         bool
}

func newSettings(cfg config.Config, updateState settingsUpdateState) Settings {
	mkInput := func(value, placeholder string, masked bool) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.CharLimit = 500
		t.SetValue(value)
		if masked {
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '●'
		}
		return t
	}

	s := Settings{
		icons:                cfg.Display.Icons,
		dateAbsolute:         cfg.Display.DateFormat == "absolute",
		markReadOnOpen:       cfg.Display.MarkReadOnOpen,
		browserInput:         mkInput(cfg.Display.Browser, "xdg-open", false),
		feedMaxBodyInput:     mkInput(strconv.Itoa(cfg.Feed.MaxBodyMiB), "10", false),
		updateCheckOnStartup: cfg.Updates.CheckOnStartup,
		update:               updateState,
		providerIdx:          providerIndex(cfg.AI.Provider),
		openaiInput:          mkInput(cfg.AI.OpenAIKey, "sk-...", true),
		claudeInput:          mkInput(cfg.AI.ClaudeKey, "sk-ant-...", true),
		geminiInput:          mkInput(cfg.AI.GeminiKey, "AIza...", true),
		ollamaURLInput:       mkInput(cfg.AI.OllamaURL, "http://localhost:11434", false),
		ollamaModelInput:     mkInput(cfg.AI.OllamaModel, "llama3.2", false),
		savePathInput:        mkInput(cfg.AI.SavePath, "~/", false),
		activeSection:        ssDisplay,
		focusedPane:          settingsPaneDetail,
		sectionField: [settingsSectionCount]settingsField{
			ssDisplay: sfIcons,
			ssFeeds:   sfFeedMaxBody,
			ssUpdates: sfUpdateCheckOnStartup,
			ssAI:      sfProvider,
			ssAbout:   sfAboutRepo,
		},
		focusedField: sfIcons,
	}
	s.applyFocus()
	return s
}

// ApplyTo merges the settings screen state back into a Config.
func (s Settings) ApplyTo(cfg config.Config) config.Config {
	cfg.Display.Icons = s.icons
	if s.dateAbsolute {
		cfg.Display.DateFormat = "absolute"
	} else {
		cfg.Display.DateFormat = "relative"
	}
	cfg.Display.MarkReadOnOpen = s.markReadOnOpen
	cfg.Display.Browser = strings.TrimSpace(s.browserInput.Value())
	if n, err := strconv.Atoi(strings.TrimSpace(s.feedMaxBodyInput.Value())); err == nil && n > 0 {
		cfg.Feed.MaxBodyMiB = n
	}
	cfg.Updates.CheckOnStartup = s.updateCheckOnStartup

	cfg.AI.Provider = aiProviderIDs[s.providerIdx]
	cfg.AI.OpenAIKey = strings.TrimSpace(s.openaiInput.Value())
	cfg.AI.ClaudeKey = strings.TrimSpace(s.claudeInput.Value())
	cfg.AI.GeminiKey = strings.TrimSpace(s.geminiInput.Value())
	cfg.AI.OllamaURL = strings.TrimSpace(s.ollamaURLInput.Value())
	cfg.AI.OllamaModel = strings.TrimSpace(s.ollamaModelInput.Value())
	cfg.AI.SavePath = strings.TrimSpace(s.savePathInput.Value())
	return cfg
}

func (s *Settings) setUpdateState(v settingsUpdateState) {
	s.update = v
	s.ensureSectionFieldVisible(ssUpdates)
}

func (s *Settings) takeAction() settingsAction {
	action := s.action
	s.action = settingsActionNone
	return action
}

func (s *Settings) setFocusedField(field settingsField) {
	s.focusedField = field
	s.sectionField[s.activeSection] = field
	s.applyFocus()
}

func (s *Settings) setFocusedPane(pane settingsPaneFocus) {
	s.focusedPane = pane
	s.applyFocus()
}

func (s *Settings) setActiveSection(section settingsSection) {
	if section < 0 || section >= settingsSectionCount {
		return
	}
	s.activeSection = section
	if section != ssAbout {
		s.aboutGradientFrame = 0
	}
	s.ensureSectionFieldVisible(section)
	s.focusedField = s.sectionField[section]
	s.applyFocus()
}

func (s *Settings) nextSection() settingsSection {
	return (s.activeSection + 1) % settingsSectionCount
}

func (s *Settings) prevSection() settingsSection {
	if s.activeSection == 0 {
		return settingsSectionCount - 1
	}
	return s.activeSection - 1
}

func (s *Settings) ensureSectionFieldVisible(section settingsSection) {
	fields := s.sectionFields(section)
	if len(fields) == 0 {
		return
	}
	current := s.sectionField[section]
	for _, f := range fields {
		if f == current {
			return
		}
	}
	s.sectionField[section] = fields[0]
	if s.activeSection == section {
		s.focusedField = fields[0]
		s.applyFocus()
	}
}

// ── Focus management ──────────────────────────────────────────────────────────

func (s *Settings) applyFocus() {
	s.browserInput.Blur()
	s.feedMaxBodyInput.Blur()
	s.openaiInput.Blur()
	s.claudeInput.Blur()
	s.geminiInput.Blur()
	s.ollamaURLInput.Blur()
	s.ollamaModelInput.Blur()
	s.savePathInput.Blur()

	if s.focusedPane != settingsPaneDetail {
		return
	}

	switch s.focusedField {
	case sfBrowser:
		s.browserInput.Focus()
	case sfFeedMaxBody:
		s.feedMaxBodyInput.Focus()
	case sfAPIKey:
		switch s.providerIdx {
		case 1:
			s.openaiInput.Focus()
		case 2:
			s.claudeInput.Focus()
		case 3:
			s.geminiInput.Focus()
		}
	case sfOllamaURL:
		s.ollamaURLInput.Focus()
	case sfOllamaModel:
		s.ollamaModelInput.Focus()
	case sfSavePath:
		s.savePathInput.Focus()
	}
}

func (s Settings) visibleFields() []settingsField {
	return s.sectionFields(s.activeSection)
}

func (s Settings) sectionFields(section settingsSection) []settingsField {
	switch section {
	case ssDisplay:
		return []settingsField{sfIcons, sfDateFormat, sfMarkReadOnOpen, sfBrowser}
	case ssFeeds:
		return []settingsField{sfFeedMaxBody}
	case ssUpdates:
		fields := []settingsField{sfUpdateCheckOnStartup, sfUpdateCheckNow}
		if s.update.latestVersion != "" {
			fields = append(fields, sfUpdateInstallNow, sfUpdateDismissVersion)
		}
		if s.update.restartable {
			fields = append(fields, sfUpdateRestartNow)
		}
		return fields
	case ssAI:
		fields := []settingsField{sfProvider}
		switch s.providerIdx {
		case 4:
			fields = append(fields, sfOllamaURL, sfOllamaModel)
		case 1, 2, 3:
			fields = append(fields, sfAPIKey)
		}
		fields = append(fields, sfSavePath)
		return fields
	case ssAbout:
		return []settingsField{sfAboutRepo, sfAboutIssues}
	default:
		return nil
	}
}

func settingsActionURL(action settingsAction) string {
	switch action {
	case settingsActionOpenRepo:
		return tideRepoURL
	case settingsActionOpenIssues:
		return tideIssuesURL
	default:
		return ""
	}
}

func settingsAboutPulseCmd() tea.Cmd {
	return tea.Tick(settingsAboutPulsePeriod, func(time.Time) tea.Msg {
		return settingsAboutPulseMsg{}
	})
}

func (s Settings) aboutPulseCmd() tea.Cmd {
	if s.activeSection != ssAbout {
		return nil
	}
	return settingsAboutPulseCmd()
}

func (s Settings) nextField() settingsField {
	fields := s.visibleFields()
	for i, f := range fields {
		if f == s.focusedField {
			if i < len(fields)-1 {
				return fields[i+1]
			}
			return fields[0]
		}
	}
	return fields[0]
}

func (s Settings) prevField() settingsField {
	fields := s.visibleFields()
	for i, f := range fields {
		if f == s.focusedField {
			if i > 0 {
				return fields[i-1]
			}
			return fields[len(fields)-1]
		}
	}
	return fields[len(fields)-1]
}

func (s Settings) isTextInput() bool {
	if s.focusedPane != settingsPaneDetail {
		return false
	}
	switch s.focusedField {
	case sfBrowser, sfFeedMaxBody, sfAPIKey, sfOllamaURL, sfOllamaModel, sfSavePath:
		return true
	}
	return false
}

func (s Settings) updateFocusedTextInput(msg tea.Msg) (Settings, tea.Cmd, bool) {
	var cmd tea.Cmd
	switch s.focusedField {
	case sfBrowser:
		s.browserInput, cmd = s.browserInput.Update(msg)
	case sfFeedMaxBody:
		s.feedMaxBodyInput, cmd = s.feedMaxBodyInput.Update(msg)
	case sfAPIKey:
		switch s.providerIdx {
		case 1:
			s.openaiInput, cmd = s.openaiInput.Update(msg)
		case 2:
			s.claudeInput, cmd = s.claudeInput.Update(msg)
		case 3:
			s.geminiInput, cmd = s.geminiInput.Update(msg)
		}
	case sfOllamaURL:
		s.ollamaURLInput, cmd = s.ollamaURLInput.Update(msg)
	case sfOllamaModel:
		s.ollamaModelInput, cmd = s.ollamaModelInput.Update(msg)
	case sfSavePath:
		s.savePathInput, cmd = s.savePathInput.Update(msg)
	}
	return s, cmd, false
}

// ── Update ────────────────────────────────────────────────────────────────────

func (s Settings) Update(msg tea.Msg, keys KeyMap) (Settings, tea.Cmd, bool) {
	if _, ok := msg.(settingsAboutPulseMsg); ok {
		if s.activeSection != ssAbout {
			s.aboutGradientFrame = 0
			return s, nil, false
		}
		s.aboutGradientFrame++
		if s.aboutGradientFrame >= settingsAboutFrameReset {
			s.aboutGradientFrame = 0
		}
		return s, settingsAboutPulseCmd(), false
	}

	// Route cursor-blink ticks to the active text input.
	if _, ok := msg.(tea.KeyMsg); !ok {
		if s.isTextInput() {
			return s.updateFocusedTextInput(msg)
		}
		return s, nil, false
	}

	key := msg.(tea.KeyMsg)
	if s.isTextInput() && key.Type == tea.KeyRunes {
		return s.updateFocusedTextInput(msg)
	}

	// Global: ctrl+s saves, esc cancels.
	switch key.String() {
	case "ctrl+s":
		s.shouldSave = true
		s.shouldExit = true
		return s, nil, true
	case "esc":
		s.shouldExit = true
		return s, nil, true
	}

	if s.focusedPane == settingsPaneSidebar {
		switch {
		case keyMatches(key, keys.Up):
			s.setActiveSection(s.prevSection())
			return s, s.aboutPulseCmd(), false
		case keyMatches(key, keys.Down):
			s.setActiveSection(s.nextSection())
			return s, s.aboutPulseCmd(), false
		case keyMatches(key, keys.Right), keyMatches(key, keys.Enter), keyMatches(key, keys.Tab):
			s.setFocusedPane(settingsPaneDetail)
			return s, nil, false
		default:
			return s, nil, false
		}
	}

	if !s.isTextInput() && keyMatches(key, keys.Left) {
		s.setFocusedPane(settingsPaneSidebar)
		return s, nil, false
	}

	// Tab / shift+tab navigate within the active section.
	switch {
	case keyMatches(key, keys.Tab):
		s.setFocusedField(s.nextField())
		return s, nil, false

	case key.String() == "shift+tab":
		s.setFocusedField(s.prevField())
		return s, nil, false
	}

	// Field-specific handling.
	switch s.focusedField {
	case sfIcons:
		if key.String() == " " || keyMatches(key, keys.Enter) {
			s.icons = !s.icons
		} else if keyMatches(key, keys.Down) {
			s.setFocusedField(s.nextField())
		} else if keyMatches(key, keys.Up) {
			s.setFocusedField(s.prevField())
		}

	case sfDateFormat:
		if key.String() == " " || keyMatches(key, keys.Enter) {
			s.dateAbsolute = !s.dateAbsolute
		} else if keyMatches(key, keys.Down) {
			s.setFocusedField(s.nextField())
		} else if keyMatches(key, keys.Up) {
			s.setFocusedField(s.prevField())
		}

	case sfMarkReadOnOpen:
		if key.String() == " " || keyMatches(key, keys.Enter) {
			s.markReadOnOpen = !s.markReadOnOpen
		} else if keyMatches(key, keys.Down) {
			s.setFocusedField(s.nextField())
		} else if keyMatches(key, keys.Up) {
			s.setFocusedField(s.prevField())
		}

	case sfUpdateCheckOnStartup:
		if key.String() == " " || keyMatches(key, keys.Enter) {
			s.updateCheckOnStartup = !s.updateCheckOnStartup
		} else if keyMatches(key, keys.Down) {
			s.setFocusedField(s.nextField())
		} else if keyMatches(key, keys.Up) {
			s.setFocusedField(s.prevField())
		}

	case sfUpdateCheckNow:
		switch {
		case key.String() == " " || keyMatches(key, keys.Enter):
			s.action = settingsActionCheckUpdates
		case keyMatches(key, keys.Down):
			s.setFocusedField(s.nextField())
		case keyMatches(key, keys.Up):
			s.setFocusedField(s.prevField())
		}

	case sfUpdateInstallNow:
		switch {
		case key.String() == " " || keyMatches(key, keys.Enter):
			s.action = settingsActionInstallUpdate
		case keyMatches(key, keys.Down):
			s.setFocusedField(s.nextField())
		case keyMatches(key, keys.Up):
			s.setFocusedField(s.prevField())
		}

	case sfUpdateDismissVersion:
		switch {
		case key.String() == " " || keyMatches(key, keys.Enter):
			s.action = settingsActionDismissVersion
		case keyMatches(key, keys.Down):
			s.setFocusedField(s.nextField())
		case keyMatches(key, keys.Up):
			s.setFocusedField(s.prevField())
		}

	case sfUpdateRestartNow:
		switch {
		case key.String() == " " || keyMatches(key, keys.Enter):
			s.action = settingsActionRestartAfterUpdate
		case keyMatches(key, keys.Down):
			s.setFocusedField(s.nextField())
		case keyMatches(key, keys.Up):
			s.setFocusedField(s.prevField())
		}

	case sfAboutRepo:
		switch {
		case key.String() == " " || keyMatches(key, keys.Enter):
			s.action = settingsActionOpenRepo
		case keyMatches(key, keys.Down):
			s.setFocusedField(s.nextField())
		case keyMatches(key, keys.Up):
			s.setFocusedField(s.prevField())
		}

	case sfAboutIssues:
		switch {
		case key.String() == " " || keyMatches(key, keys.Enter):
			s.action = settingsActionOpenIssues
		case keyMatches(key, keys.Down):
			s.setFocusedField(s.nextField())
		case keyMatches(key, keys.Up):
			s.setFocusedField(s.prevField())
		}

	case sfProvider:
		switch {
		case key.String() == " " || keyMatches(key, keys.Enter):
			s.providerIdx = (s.providerIdx + 1) % len(aiProviderLabels)
			s.ensureSectionFieldVisible(ssAI)
			s.setFocusedField(sfProvider)
		case keyMatches(key, keys.Down):
			s.setFocusedField(s.nextField())
		case keyMatches(key, keys.Up):
			s.setFocusedField(s.prevField())
		}

	case sfBrowser, sfFeedMaxBody, sfAPIKey, sfOllamaURL, sfOllamaModel, sfSavePath:
		// Enter advances to next field; everything else goes to the text input.
		if keyMatches(key, keys.Enter) {
			s.setFocusedField(s.nextField())
			return s, nil, false
		}
		if keyMatches(key, keys.Up) {
			s.setFocusedField(s.prevField())
			return s, nil, false
		}
		if keyMatches(key, keys.Down) {
			s.setFocusedField(s.nextField())
			return s, nil, false
		}
		return s.updateFocusedTextInput(msg)
	}

	return s, nil, false
}

// ── View ──────────────────────────────────────────────────────────────────────

func (s Settings) View(width, height int, chrome managerChrome) string {
	header := renderManagerHeader("SETTINGS", width, chrome)
	gap := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render("")
	hints := s.viewHints(width, chrome)

	bodyH := max(1, height-lipgloss.Height(header)-lipgloss.Height(gap)-lipgloss.Height(hints))
	body := s.viewSplit(width, bodyH, chrome)

	return lipgloss.JoinVertical(lipgloss.Left, header, gap, body, hints)
}

func (s Settings) viewSplit(width, height int, chrome managerChrome) string {
	leftW := clamp(width/4, 14, 18)
	if width-leftW-1 < 32 {
		leftW = max(14, width-33)
	}
	rightW := max(18, width-leftW-1)
	left := s.viewSectionsPane(leftW, height, chrome)
	right := s.viewSectionPane(rightW, height, chrome)
	separator := lipgloss.NewStyle().Background(chrome.baseBg).Render(" ")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, separator, right)
}

func (s Settings) viewSectionsPane(width, height int, chrome managerChrome) string {
	rows := make([]string, 0, settingsSectionCount)
	for i, label := range settingsSectionLabels {
		selected := settingsSection(i) == s.activeSection
		rows = append(rows, s.renderSectionNavRow(width, label, selected, s.focusedPane == settingsPaneSidebar, chrome))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	title := "CATEGORIES"
	if s.focusedPane == settingsPaneSidebar {
		title = "CATEGORIES >"
	}
	section := clampView(renderManagerSection(title, body, chrome), width, height, chrome.baseBg)
	return lipgloss.NewStyle().Width(width).Height(height).Background(chrome.baseBg).Render(section)
}

func (s Settings) viewSectionPane(width, height int, chrome managerChrome) string {
	body := s.viewSectionBody(width, chrome)
	title := settingsSectionLabels[s.activeSection]
	if s.focusedPane == settingsPaneDetail {
		title += " >"
	}
	section := clampView(renderManagerSection(title, body, chrome), width, height, chrome.baseBg)
	return lipgloss.NewStyle().Width(width).Height(height).Background(chrome.baseBg).Render(section)
}

func (s Settings) viewSectionBody(width int, chrome managerChrome) string {
	if s.activeSection == ssAbout {
		return s.renderAboutSection(width, chrome)
	}

	ind := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).PaddingLeft(2)
	blank := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render("")

	// inputW: full inner width minus the 2-char left padding of ind.
	inputW := width - 3
	hintIndent := strings.Repeat(" ", labelColW)

	addToggle := func(lines []string, label string, on bool, field settingsField) []string {
		focused := s.focusedField == field
		row := s.renderToggle(label, on, focused, width-2, chrome)
		if hint := s.fieldHint(field); hint != "" {
			lines = append(lines,
				ind.Render(row),
				ind.Render(s.renderInlineHint(hintIndent+hint, width-2, chrome)),
				blank,
			)
			return lines
		}
		return append(lines,
			ind.Render(row),
			blank,
		)
	}

	addInput := func(lines []string, label string, input textinput.Model, field settingsField) []string {
		focused := s.focusedField == field
		compactSecretPreview := field == sfAPIKey
		fieldW := s.inputWidth(field, inputW)
		row := s.renderFieldLabel(label, focused, width-2, chrome) + renderTextInput(input, fieldW, focused, compactSecretPreview, chrome)
		if hint := s.fieldHint(field); hint != "" {
			lines = append(lines,
				ind.Render(row),
				ind.Render(s.renderInlineHint(hintIndent+hint, width-2, chrome)),
				blank,
			)
			return lines
		}
		return append(lines,
			ind.Render(row),
			blank,
		)
	}

	addValue := func(lines []string, label, value string, focused bool) []string {
		if value == "" {
			return lines
		}
		row := s.renderValueRow(label, value, focused, width-2, chrome)
		return append(lines,
			ind.Render(row),
			blank,
		)
	}

	addAction := func(lines []string, label, hint string, field settingsField) []string {
		focused := s.focusedField == field
		row := s.renderActionRow(label, hint, focused, width-2, chrome)
		return append(lines,
			ind.Render(row),
			blank,
		)
	}

	lines := []string{}
	switch s.activeSection {
	case ssDisplay:
		lines = addToggle(lines, "Icons", s.icons, sfIcons)
		lines = addToggle(lines, "Use relative dates", !s.dateAbsolute, sfDateFormat)
		lines = addToggle(lines, "Mark read on open", s.markReadOnOpen, sfMarkReadOnOpen)
		lines = addInput(lines, "Browser command", s.browserInput, sfBrowser)

	case ssFeeds:
		lines = addInput(lines, "Feed max size (MiB)", s.feedMaxBodyInput, sfFeedMaxBody)

	case ssUpdates:
		lines = addValue(lines, "Current version", s.update.currentVersion, false)
		lines = addToggle(lines, "Check on startup", s.updateCheckOnStartup, sfUpdateCheckOnStartup)
		if !s.update.lastChecked.IsZero() {
			lines = addValue(lines, "Last checked", relativeTime(s.update.lastChecked), false)
		}
		lines = addAction(lines, "Check now", "query latest release", sfUpdateCheckNow)
		if s.update.latestVersion != "" {
			lines = addValue(lines, "Latest version", s.update.latestVersion, false)
		}
		if !s.update.publishedAt.IsZero() {
			lines = addValue(lines, "Published", s.update.publishedAt.Format("Jan 2, 2006"), false)
		}
		lines = addValue(lines, "Status", s.update.statusLabel(), false)
		if s.update.summary != "" {
			lines = append(lines,
				ind.Render(s.renderInlineHint(hintIndent+s.update.summary, width-2, chrome)),
				blank,
			)
		}
		if s.update.latestVersion != "" {
			lines = addAction(lines, "Update now", "download and replace Tide", sfUpdateInstallNow)
			lines = addAction(lines, "Dismiss version", "hide prompts for this release", sfUpdateDismissVersion)
		}
		if s.update.manualCommand != "" {
			lines = addValue(lines, "Install command", s.update.manualCommand, false)
		}
		if s.update.restartable {
			lines = addAction(lines, "Restart now", "launch updated Tide", sfUpdateRestartNow)
		}

	case ssAI:
		lines = append(lines,
			ind.Render(s.renderProviderSelector(width-2, chrome)),
			blank,
		)
		switch s.providerIdx {
		case 1:
			lines = addInput(lines, "OpenAI key", s.openaiInput, sfAPIKey)
		case 2:
			lines = addInput(lines, "Claude key", s.claudeInput, sfAPIKey)
		case 3:
			lines = addInput(lines, "Gemini key", s.geminiInput, sfAPIKey)
		case 4:
			lines = addInput(lines, "Ollama URL", s.ollamaURLInput, sfOllamaURL)
			lines = addInput(lines, "Model", s.ollamaModelInput, sfOllamaModel)
		}
		lines = addInput(lines, "Save summaries to", s.savePathInput, sfSavePath)

	}

	if len(lines) == 0 {
		lines = append(lines, blank)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (s Settings) inputWidth(field settingsField, maxWidth int) int {
	switch field {
	case sfFeedMaxBody:
		return min(maxWidth, 12)
	case sfBrowser, sfSavePath:
		return min(maxWidth, 36)
	case sfOllamaURL, sfOllamaModel, sfAPIKey:
		return min(maxWidth, 44)
	default:
		return min(maxWidth, 32)
	}
}

func (s Settings) dateLabel() string {
	if s.dateAbsolute {
		return "absolute"
	}
	return "relative"
}

func (s Settings) renderSectionLabel(label string, width int, chrome managerChrome) string {
	bar := lipgloss.NewStyle().Background(chrome.accent).Width(2).Render("  ")
	title := chrome.sectionLabel.Copy().Foreground(chrome.text).Width(width - 2).Render(" " + label)
	return lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render(bar + title)
}

const labelColW = 20

func (s Settings) viewHints(width int, chrome managerChrome) string {
	if s.focusedPane == settingsPaneSidebar {
		return renderManagerActions(width, chrome,
			"↑/↓", "section",
			"→", "edit",
			"ctrl+s", "save",
			"esc", "cancel",
		)
	}
	return renderManagerActions(width, chrome,
		"←", "sections",
		"↑/↓", "field",
		"tab", "next",
		"ctrl+s", "save",
		"esc", "cancel",
	)
}

func (s Settings) renderSectionNavRow(width int, label string, selected, paneFocused bool, chrome managerChrome) string {
	textW := max(1, width-2)
	row := padRight(truncate(label, textW), textW)
	if selected {
		bg := chrome.surfaceBg
		fg := chrome.accent
		if paneFocused {
			bg = chrome.accent
			fg = chrome.accentFg
		}
		return lipgloss.NewStyle().
			Background(bg).
			Foreground(fg).
			Bold(true).
			Padding(0, 1).
			Render(row)
	}
	return lipgloss.NewStyle().
		Background(chrome.baseBg).
		Foreground(chrome.text).
		Padding(0, 1).
		Render(row)
}

func (s Settings) renderFieldLabel(label string, focused bool, _ int, chrome managerChrome) string {
	style := chrome.body
	if focused {
		style = style.Foreground(chrome.text).Bold(true)
	} else {
		style = style.Foreground(chrome.muted)
	}
	return style.Width(labelColW).Render(padRight(label, labelColW))
}

func (s Settings) renderValueRow(label, value string, focused bool, width int, chrome managerChrome) string {
	label = s.renderFieldLabel(label, focused, width, chrome)
	valueStyle := chrome.body.Foreground(chrome.text)
	if !focused {
		valueStyle = valueStyle.Foreground(chrome.text)
	}
	return label + valueStyle.Width(max(1, width-labelColW)).Render(value)
}

func (s Settings) renderActionRow(label, hint string, focused bool, width int, chrome managerChrome) string {
	label = s.renderFieldLabel(label, focused, width, chrome)

	badge := s.renderBadge("ENTER", focused, chrome)

	hintText := ""
	if hint != "" {
		hintText = "  " + hint
	}
	return label + badge + s.renderInlineHint(hintText, max(1, width-labelColW-8), chrome)
}

func (s Settings) renderToggle(label string, on bool, focused bool, width int, chrome managerChrome) string {
	label = s.renderFieldLabel(label, focused, width, chrome)

	val := "OFF"
	if on {
		val = "ON"
	}
	badge := s.renderBadge(val, focused, chrome)
	return label + badge + s.renderToggleHint(on, "enabled", "disabled", focused, chrome)
}

func (u settingsUpdateState) statusLabel() string {
	switch u.state {
	case updateStateChecking:
		return "checking for updates..."
	case updateStateAvailable:
		if u.dismissed {
			return "update dismissed"
		}
		return "update available"
	case updateStateDownloading:
		return "downloading update..."
	case updateStateInstalling:
		return "installing update..."
	case updateStateInstalled:
		if u.installedVersion != "" {
			return "updated to " + u.installedVersion
		}
		return "update installed"
	case updateStateNeedsElevation:
		return "admin permission required"
	case updateStateError:
		if u.err != "" {
			return u.err
		}
		return "update failed"
	default:
		if u.lastChecked.IsZero() {
			return "not checked yet"
		}
		return "up to date"
	}
}

func (s Settings) renderBadge(text string, focused bool, chrome managerChrome) string {
	text = strings.ToUpper(strings.TrimSpace(text))
	badgeW := 7
	if focused {
		return chrome.key.UnsetPadding().Width(badgeW).Align(lipgloss.Center).Render(text)
	}
	return lipgloss.NewStyle().
		Background(chrome.surfaceBg).
		Foreground(chrome.muted).
		Width(badgeW).
		Align(lipgloss.Center).
		Render(strings.ToLower(text))
}

// renderToggleHint appends a small hint showing the current value label.
func (s Settings) renderToggleHint(active bool, trueLabel, falseLabel string, focused bool, chrome managerChrome) string {
	val := falseLabel
	if active {
		val = trueLabel
	}
	style := chrome.keyLabel
	if !focused {
		style = lipgloss.NewStyle().Background(chrome.baseBg).Foreground(chrome.muted)
	}
	return style.Render("  " + val)
}

func (s Settings) renderProviderSelector(width int, chrome managerChrome) string {
	label := s.renderFieldLabel("Provider", s.focusedField == sfProvider, width, chrome)

	focused := s.focusedField == sfProvider
	providerName := aiProviderLabels[s.providerIdx]

	var selector string
	if focused {
		selector = s.renderBadge(providerName, true, chrome)
	} else {
		selector = lipgloss.NewStyle().
			Background(chrome.surfaceBg).
			Foreground(chrome.muted).
			Width(7).
			Align(lipgloss.Center).
			Render(strings.ToLower(providerName))
	}
	hint := s.renderInlineHint(" use enter to change", width-labelColW, chrome)
	if focused {
		hint = chrome.keyLabel.Render(" use enter to change")
	}
	return label + selector + hint
}

func (s Settings) fieldHint(field settingsField) string {
	switch field {
	case sfBrowser:
		return "leave blank to use the system default browser"
	case sfFeedMaxBody:
		return "larger feeds need more memory; default is 10 MiB"
	case sfAPIKey:
		return "only the active provider key is used"
	case sfOllamaURL:
		return "local Ollama endpoint"
	case sfSavePath:
		return "directory for exported markdown summaries"
	default:
		return ""
	}
}

func (s Settings) renderInlineHint(text string, width int, chrome managerChrome) string {
	return lipgloss.NewStyle().
		Background(chrome.baseBg).
		Foreground(chrome.muted).
		Width(max(1, width)).
		Render(text)
}

func (s Settings) renderAboutSection(width int, chrome managerChrome) string {
	ind := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).PaddingLeft(2)
	blank := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render("")
	bodyW := max(1, width-2)

	return lipgloss.JoinVertical(lipgloss.Left,
		ind.Render(s.renderAboutHero(bodyW, chrome)),
		blank,
		ind.Render(s.renderAboutLinks(bodyW, chrome)),
		blank,
		ind.Render(s.renderAboutClosingNote(bodyW, chrome)),
	)
}

func (s Settings) renderAboutHero(width int, chrome managerChrome) string {
	panelW := max(1, width-4)
	contentW := max(1, panelW-2)
	label := s.renderAboutWaveLine("TIDE", contentW, 0, false)
	tagline := wrapWords("Your feeds, no algorithm, no bullshit", max(12, contentW))
	taglineLines := strings.Split(tagline, "\n")

	lines := []string{label}
	for i, line := range taglineLines {
		crest, body := s.renderAboutBouncyWaveLine(line, contentW, i+1, true)
		lines = append(lines, crest, body)
	}

	panelBg := lipgloss.Color("#04141d")
	panel := lipgloss.NewStyle().
		Width(panelW).
		Background(panelBg).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#2e6f7f")).
		BorderBackground(panelBg).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))

	return lipgloss.NewStyle().Width(width).Background(chrome.baseBg).Render(panel)
}

func (s Settings) renderAboutLinks(width int, chrome managerChrome) string {
	repoCard := s.renderAboutLinkCard(width, "REPOSITORY", "view source on GitHub", tideRepoURL, s.focusedField == sfAboutRepo, chrome)
	issuesCard := s.renderAboutLinkCard(width, "ISSUES", "report bugs or track changes", tideIssuesURL, s.focusedField == sfAboutIssues, chrome)

	if width < settingsAboutTwoColMinW {
		return lipgloss.JoinVertical(lipgloss.Left, repoCard, "", issuesCard)
	}

	leftW := max(20, (width-settingsAboutCardGap)/2)
	rightW := max(20, width-leftW-settingsAboutCardGap)
	repoCard = s.renderAboutLinkCard(leftW, "REPOSITORY", "view source on GitHub", tideRepoURL, s.focusedField == sfAboutRepo, chrome)
	issuesCard = s.renderAboutLinkCard(rightW, "ISSUES", "report bugs or track changes", tideIssuesURL, s.focusedField == sfAboutIssues, chrome)
	gap := lipgloss.NewStyle().Background(chrome.baseBg).Render(strings.Repeat(" ", settingsAboutCardGap))
	return lipgloss.JoinHorizontal(lipgloss.Top, repoCard, gap, issuesCard)
}

func (s Settings) renderAboutLinkCard(width int, title, hint, url string, focused bool, chrome managerChrome) string {
	bg := chrome.surfaceBg
	border := chrome.border
	titleFg := chrome.text
	bodyFg := chrome.muted
	urlFg := chrome.accent
	if focused {
		if isDark(bg) {
			bg = adjustLightness(bg, 0.07)
		} else {
			bg = adjustLightness(bg, -0.07)
		}
		border = chrome.accent
		titleFg = chrome.text
		bodyFg = readableText(chrome.text, bg, 3.5)
		urlFg = titleFg
	}

	panelW := max(1, width-4)
	textW := max(1, panelW-2)
	badge := s.renderBadge("ENTER", focused, chrome)
	titleText := lipgloss.NewStyle().Background(bg).Foreground(titleFg).Bold(true).Render(title)
	titleGap := max(1, textW-lipgloss.Width(titleText)-lipgloss.Width(badge))
	titleLine := titleText + lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", titleGap)) + badge

	bodyStyle := lipgloss.NewStyle().Background(bg).Foreground(bodyFg)
	urlStyle := lipgloss.NewStyle().Background(bg).Foreground(urlFg).Bold(focused)
	content := strings.Join([]string{
		clampView(titleLine, textW, 1, bg),
		bodyStyle.Width(textW).Render(truncate(hint, textW)),
		urlStyle.Width(textW).Render(truncate(url, textW)),
	}, "\n")

	card := lipgloss.NewStyle().
		Width(panelW).
		Background(bg).
		Border(lipgloss.NormalBorder()).
		BorderForeground(border).
		BorderBackground(bg).
		Padding(0, 1).
		Render(content)

	return lipgloss.NewStyle().Width(width).Background(chrome.baseBg).Render(card)
}

func (s Settings) renderAboutClosingNote(width int, chrome managerChrome) string {
	signoff := lipgloss.NewStyle().
		Background(chrome.baseBg).
		Foreground(chrome.muted).
		Italic(true).
		Width(max(1, width)).
		Align(lipgloss.Center).
		Render("Thanks for taking a look -allie")

	heart := lipgloss.NewStyle().
		Background(chrome.baseBg).
		Foreground(lipgloss.Color("#e64553")).
		Bold(true).
		Width(max(1, width)).
		Align(lipgloss.Center).
		Render("♥")

	return lipgloss.JoinVertical(lipgloss.Left, signoff, heart)
}

func (s Settings) renderAboutWaveLine(text string, width, lineIndex int, bold bool) string {
	center, bandWidth := s.aboutWaveCenter(width, lineIndex)
	runes := []rune(text)
	if len(runes) > width {
		runes = []rune(truncate(text, width))
	}

	var b strings.Builder
	for i := 0; i < width; i++ {
		ch := ' '
		if i < len(runes) {
			ch = runes[i]
		}
		b.WriteString(renderAboutWaveCell(ch, i-center, bandWidth, bold))
	}
	return b.String()
}

func (s Settings) renderAboutBouncyWaveLine(text string, width, lineIndex int, bold bool) (string, string) {
	center, bandWidth := s.aboutWaveCenter(width, lineIndex)
	runes := []rune(text)
	if len(runes) > width {
		runes = []rune(truncate(text, width))
	}

	var crest strings.Builder
	var body strings.Builder
	for i := 0; i < width; i++ {
		ch := ' '
		if i < len(runes) {
			ch = runes[i]
		}

		delta := i - center
		phase := aboutWaveImpactPhase(delta)

		crestCh := ' '
		if phase == aboutWavePhaseImpact && ch != ' ' {
			crestCh = ch
		}

		crest.WriteString(renderAboutWaveCell(crestCh, delta, bandWidth, bold))
		switch {
		case ch == ' ':
			body.WriteString(renderAboutWaveCell(ch, delta, bandWidth, bold))
		case phase == aboutWavePhaseImpact:
			body.WriteString(renderAboutWaveTintedCell(ch, delta, bandWidth, false, 0.38))
		case phase == aboutWavePhasePreImpact:
			body.WriteString(renderAboutWaveTintedCell(ch, delta, bandWidth, false, 0.58))
		case phase == aboutWavePhaseWakeStrong:
			body.WriteString(renderAboutWaveTintedCell(ch, delta, bandWidth, false, 0.50))
		case phase == aboutWavePhaseWakeSoft:
			body.WriteString(renderAboutWaveTintedCell(ch, delta, bandWidth, false, 0.72))
		default:
			body.WriteString(renderAboutWaveCell(ch, delta, bandWidth, bold))
		}
	}
	return crest.String(), body.String()
}

func (s Settings) aboutWaveCenter(width, lineIndex int) (int, int) {
	bandWidth := max(8, width/3)
	travel := max(1, width+bandWidth*2)
	center := (s.aboutGradientFrame + lineIndex*2) % travel
	center -= bandWidth
	return center, bandWidth
}

func renderAboutWaveCell(ch rune, delta, bandWidth int, bold bool) string {
	bg := aboutWaveColor(delta, bandWidth)
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(aboutWaveForeground(bg)).
		Bold(bold).
		Render(string(ch))
}

func renderAboutWaveTintedCell(ch rune, delta, bandWidth int, bold bool, mix float64) string {
	bg := aboutWaveColor(delta, bandWidth)
	tint := blendLipglossColor(bg, aboutWaveForeground(bg), mix)
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(tint).
		Bold(bold).
		Render(string(ch))
}

type aboutWavePhase int

const (
	aboutWavePhaseNone aboutWavePhase = iota
	aboutWavePhasePreImpact
	aboutWavePhaseImpact
	aboutWavePhaseWakeStrong
	aboutWavePhaseWakeSoft
)

func aboutWaveImpactPhase(delta int) aboutWavePhase {
	switch delta {
	case -1:
		return aboutWavePhasePreImpact
	case 0:
		return aboutWavePhaseImpact
	case 1:
		return aboutWavePhaseWakeStrong
	case 2:
		return aboutWavePhaseWakeSoft
	default:
		return aboutWavePhaseNone
	}
}

func aboutWaveForeground(bg lipgloss.Color) lipgloss.Color {
	fg := lipgloss.Color("#eef8fb")
	if contrastRatio(lipgloss.Color("#103847"), bg) >= 4.5 {
		fg = lipgloss.Color("#103847")
	}
	return fg
}

func aboutWaveColor(delta, bandWidth int) lipgloss.Color {
	base := lipgloss.Color("#0b2b36")
	if delta < -bandWidth || delta > bandWidth {
		return base
	}

	if delta <= 0 {
		t := float64(delta+bandWidth) / float64(max(1, bandWidth))
		switch {
		case t < 0.45:
			return blendLipglossColor(base, lipgloss.Color("#2f8f97"), t/0.45)
		case t < 0.75:
			return blendLipglossColor(lipgloss.Color("#2f8f97"), lipgloss.Color("#58b8be"), (t-0.45)/0.30)
		case t < 0.92:
			return blendLipglossColor(lipgloss.Color("#58b8be"), lipgloss.Color("#81cfd5"), (t-0.75)/0.17)
		default:
			return blendLipglossColor(lipgloss.Color("#81cfd5"), lipgloss.Color("#a9dfe2"), (t-0.92)/0.08)
		}
	}

	t := float64(delta) / float64(max(1, bandWidth))
	switch {
	case t < 0.10:
		return blendLipglossColor(lipgloss.Color("#a9dfe2"), lipgloss.Color("#81cfd5"), t/0.10)
	case t < 0.26:
		return blendLipglossColor(lipgloss.Color("#81cfd5"), lipgloss.Color("#73c0d8"), (t-0.10)/0.16)
	case t < 0.52:
		return blendLipglossColor(lipgloss.Color("#73c0d8"), lipgloss.Color("#4f97bb"), (t-0.26)/0.26)
	default:
		return blendLipglossColor(lipgloss.Color("#4f97bb"), base, (t-0.52)/0.48)
	}
}

func blendLipglossColor(a, b lipgloss.Color, t float64) lipgloss.Color {
	ar, ag, ab, okA := hexToRGB(a)
	br, bg, bb, okB := hexToRGB(b)
	if !okA || !okB {
		return a
	}
	t = math.Max(0, math.Min(1, t))
	r := ar + (br-ar)*t
	g := ag + (bg-ag)*t
	bl := ab + (bb-ab)*t
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
		uint8(math.Round(r*255)),
		uint8(math.Round(g*255)),
		uint8(math.Round(bl*255)),
	))
}
