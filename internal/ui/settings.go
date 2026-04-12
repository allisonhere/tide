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

// shellCommandKeywords are highlighted as “keywords” in the manual install code block.
var shellCommandKeywords = map[string]bool{
	"sudo": true, "su": true, "doas": true, "install": true, "cp": true, "mv": true,
	"chmod": true, "chown": true, "rm": true, "cd": true, "export": true,
	"echo": true, "curl": true, "wget": true, "tar": true, "unzip": true,
	"dnf": true, "apt": true, "pacman": true, "brew": true, "nix-env": true,
}

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
	sfUpdateManualCommand
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
	settingsActionCopyManualInstall
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
	latestIsFresh    bool
	publishedAt      time.Time
	summary          string
	lastChecked      time.Time
	err              string
	dismissed        bool
	manualCommand    string
	restartable      bool
	installedVersion string
}

type settingsSectionBody struct {
	lines   []string
	anchors map[settingsField]int
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
	saveError          string
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
		focusedPane:          settingsPaneSidebar,
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

func (s *Settings) clearSaveError() {
	s.saveError = ""
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
		if s.update.manualCommand != "" {
			fields = append(fields, sfUpdateManualCommand)
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
	s.clearSaveError()
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

func selectedAIKeyFormat(value string) string {
	value = strings.TrimSpace(value)
	switch {
	case strings.HasPrefix(value, "sk-ant-"):
		return "Claude"
	case strings.HasPrefix(value, "AIza"):
		return "Gemini"
	case strings.HasPrefix(value, "sk-"):
		return "OpenAI"
	default:
		return ""
	}
}

func selectedAIKeyExpectation(providerIdx int) (providerName, expectedPrefix string) {
	switch providerIdx {
	case 1:
		return "OpenAI", "sk-"
	case 2:
		return "Claude", "sk-ant-"
	case 3:
		return "Gemini", "AIza"
	default:
		return "", ""
	}
}

func (s Settings) selectedAIKeyValue() string {
	switch s.providerIdx {
	case 1:
		return strings.TrimSpace(s.openaiInput.Value())
	case 2:
		return strings.TrimSpace(s.claudeInput.Value())
	case 3:
		return strings.TrimSpace(s.geminiInput.Value())
	default:
		return ""
	}
}

func (s Settings) selectedAIKeyValidation() (string, bool) {
	providerName, expectedPrefix := selectedAIKeyExpectation(s.providerIdx)
	if providerName == "" {
		return "", true
	}

	value := s.selectedAIKeyValue()
	if value == "" {
		return fmt.Sprintf("%s key is empty; expected prefix %s", providerName, expectedPrefix), false
	}

	format := selectedAIKeyFormat(value)
	if format == "" {
		return fmt.Sprintf("%s keys usually start with %s", providerName, expectedPrefix), false
	}
	if format != providerName {
		return fmt.Sprintf("looks like %s, but %s is selected", format, providerName), false
	}
	return fmt.Sprintf("format looks like %s", providerName), true
}

func (s Settings) focusedTextInputCursorPosition() int {
	switch s.focusedField {
	case sfBrowser:
		return s.browserInput.Position()
	case sfFeedMaxBody:
		return s.feedMaxBodyInput.Position()
	case sfAPIKey:
		switch s.providerIdx {
		case 1:
			return s.openaiInput.Position()
		case 2:
			return s.claudeInput.Position()
		case 3:
			return s.geminiInput.Position()
		}
	case sfOllamaURL:
		return s.ollamaURLInput.Position()
	case sfOllamaModel:
		return s.ollamaModelInput.Position()
	case sfSavePath:
		return s.savePathInput.Position()
	}
	return -1
}

func (s Settings) isPickerField() bool {
	switch s.focusedField {
	case sfProvider:
		return true
	}
	return false
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

	// Global: ctrl+s saves; esc from detail moves to sidebar, esc from sidebar exits.
	switch key.String() {
	case "ctrl+s":
		if msg, ok := s.selectedAIKeyValidation(); !ok {
			s.saveError = msg
			s.setActiveSection(ssAI)
			s.setFocusedPane(settingsPaneDetail)
			s.setFocusedField(sfAPIKey)
			return s, nil, false
		}
		s.shouldSave = true
		s.shouldExit = true
		return s, nil, true
	case "esc":
		if s.focusedPane == settingsPaneDetail {
			s.setFocusedPane(settingsPaneSidebar)
			return s, nil, false
		}
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

	if s.isTextInput() && key.Type == tea.KeyLeft {
		if s.focusedTextInputCursorPosition() == 0 {
			s.setFocusedPane(settingsPaneSidebar)
			return s, nil, false
		}
		return s.updateFocusedTextInput(msg)
	}

	if !s.isTextInput() && !s.isPickerField() && keyMatches(key, keys.Left) {
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

	case sfUpdateManualCommand:
		switch {
		case key.String() == " " || keyMatches(key, keys.Enter) || keyMatches(key, keys.CopyText):
			s.action = settingsActionCopyManualInstall
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
		case keyMatches(key, keys.Left):
			s.clearSaveError()
			s.providerIdx = (s.providerIdx + len(aiProviderLabels) - 1) % len(aiProviderLabels)
			s.ensureSectionFieldVisible(ssAI)
			s.setFocusedField(sfProvider)
		case key.String() == " " || keyMatches(key, keys.Enter) || keyMatches(key, keys.Right):
			s.clearSaveError()
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
		subtitle := ""
		if settingsSection(i) == ssAI && s.providerIdx > 0 {
			subtitle = strings.ToLower(aiProviderLabels[s.providerIdx])
		}
		rows = append(rows, s.renderSectionNavRow(width, label, subtitle, selected, s.focusedPane == settingsPaneSidebar, chrome))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	title := "CATEGORIES"
	if s.focusedPane == settingsPaneSidebar {
		title = "CATEGORIES >"
	}
	section := clampView(renderManagerSection(title, body, chrome, s.focusedPane == settingsPaneSidebar), width, height, chrome.baseBg)
	return lipgloss.NewStyle().Width(width).Height(height).Background(chrome.baseBg).Render(section)
}

func (s Settings) viewSectionPane(width, height int, chrome managerChrome) string {
	title := settingsSectionLabels[s.activeSection]
	if s.focusedPane == settingsPaneDetail {
		title += " >"
	}
	body := s.viewSectionBody(width, chrome)
	titleStyle := chrome.sectionLabel
	if s.focusedPane == settingsPaneDetail {
		titleStyle = chrome.sectionLabelActive
	}
	titleRow := titleStyle.Width(width).Render(title)
	bodyHeight := max(1, height-1)
	section := lipgloss.JoinVertical(lipgloss.Left, titleRow, s.scrollSectionBody(body, width, bodyHeight, chrome))
	return lipgloss.NewStyle().Width(width).Height(height).Background(chrome.baseBg).Render(section)
}

func (s Settings) scrollSectionBody(body settingsSectionBody, width, height int, chrome managerChrome) string {
	if len(body.lines) == 0 {
		return clampView("", width, height, chrome.baseBg)
	}
	offset := 0
	if anchor, ok := body.anchors[s.focusedField]; ok {
		offset = settingsScrollOffset(len(body.lines), anchor, height)
	}
	end := min(len(body.lines), offset+height)
	return clampView(strings.Join(body.lines[offset:end], "\n"), width, height, chrome.baseBg)
}

func settingsScrollOffset(totalLines, anchorLine, height int) int {
	if totalLines <= height || height <= 0 {
		return 0
	}
	anchorLine = clamp(anchorLine, 0, totalLines-1)
	offset := anchorLine - height/2
	return clamp(offset, 0, totalLines-height)
}

func (s Settings) viewSectionBody(width int, chrome managerChrome) settingsSectionBody {
	if s.activeSection == ssAbout {
		return s.renderAboutSection(width, chrome)
	}

	ind := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).PaddingLeft(2)
	blank := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render("")
	body := settingsSectionBody{anchors: make(map[settingsField]int)}
	addLine := func(line string) {
		body.lines = append(body.lines, line)
	}
	markAnchor := func(field settingsField) {
		body.anchors[field] = len(body.lines)
	}

	hintIndent := strings.Repeat(" ", labelColW)

	addToggle := func(label string, on bool, field settingsField) {
		focused := s.focusedField == field
		row := s.renderToggle(label, on, focused, width-2, chrome)
		markAnchor(field)
		addLine(ind.Render(row))
		if hint := s.fieldHint(field); hint != "" {
			addLine(ind.Render(s.renderInlineHint(hintIndent+hint, width-2, chrome)))
		}
		addLine(blank)
	}

	addInput := func(label string, input textinput.Model, field settingsField) {
		focused := s.focusedField == field
		rowFieldW := max(1, width-2-labelColW-1)
		fieldW := min(rowFieldW, s.inputWidth(field, rowFieldW))
		row := s.renderFieldLabel(label, focused, width-2, chrome) + renderTextInput(input, fieldW, focused, false, chrome)
		markAnchor(field)
		addLine(ind.Render(row))
		if hint := s.fieldHint(field); hint != "" {
			addLine(ind.Render(s.renderInlineHint(hintIndent+hint, width-2, chrome)))
		}
		addLine(blank)
	}

	addSecretField := func(label string, input textinput.Model, field settingsField) {
		focused := s.focusedField == field
		markAnchor(field)
		if focused {
			hintW := max(1, width-2-labelColW-7)
			header := s.renderFieldLabel(label, true, width-2, chrome) +
				s.renderBadge("EDIT", true, chrome) +
				lipgloss.NewStyle().
					Background(chrome.baseBg).
					Foreground(chrome.muted).
					Width(hintW).
					Render(" "+truncate("masked while typing", max(1, hintW-1)))
			addLine(ind.Render(header))
			addLine(ind.Render(renderSecretEditor(input, width-2, chrome)))
			if hint := s.fieldHint(field); hint != "" {
				addLine(ind.Render(s.renderInlineHint(hint, width-2, chrome)))
			}
		} else {
			row := s.renderFieldLabel(label, false, width-2, chrome) +
				renderSecretSummary(input.Value(), max(1, width-2-labelColW), chrome)
			addLine(ind.Render(row))
			if hint := s.fieldHint(field); hint != "" {
				addLine(ind.Render(s.renderInlineHint(hintIndent+hint, width-2, chrome)))
			}
		}
		addLine(blank)
	}

	addValue := func(label, value string, focused bool) {
		if value == "" {
			return
		}
		row := s.renderValueRow(label, value, focused, width-2, chrome)
		addLine(ind.Render(row))
		addLine(blank)
	}

	addAction := func(label, hint string, field settingsField) {
		focused := s.focusedField == field
		row := s.renderActionRow(label, hint, focused, width-2, chrome)
		markAnchor(field)
		addLine(ind.Render(row))
		addLine(blank)
	}
	switch s.activeSection {
	case ssDisplay:
		addToggle("Icons", s.icons, sfIcons)
		addToggle("Use relative dates", !s.dateAbsolute, sfDateFormat)
		addToggle("Mark read on open", s.markReadOnOpen, sfMarkReadOnOpen)
		addInput("Browser command", s.browserInput, sfBrowser)

	case ssFeeds:
		addInput("Feed max size (MiB)", s.feedMaxBodyInput, sfFeedMaxBody)

	case ssUpdates:
		addValue("Current version", s.update.currentVersion, false)
		addToggle("Check on startup", s.updateCheckOnStartup, sfUpdateCheckOnStartup)
		if !s.update.lastChecked.IsZero() {
			addValue("Last checked", relativeTime(s.update.lastChecked), false)
		}
		addAction("Check now", "query latest release", sfUpdateCheckNow)
		if s.update.latestVersion != "" {
			addValue("Latest version", s.update.latestVersion, false)
		}
		if !s.update.publishedAt.IsZero() {
			addValue("Published", s.update.publishedAt.Format("Jan 2, 2006"), false)
		}
		addValue("Status", s.update.statusLabel(), false)
		if s.update.summary != "" {
			addLine(ind.Render(s.renderInlineHint(hintIndent+s.update.summary, width-2, chrome)))
			addLine(blank)
		}
		if s.update.latestVersion != "" {
			addAction("Update now", "Update now", sfUpdateInstallNow)
			addAction("Ignore", "", sfUpdateDismissVersion)
		}
		if s.update.manualCommand != "" {
			focused := s.focusedField == sfUpdateManualCommand
			markAnchor(sfUpdateManualCommand)
			for _, line := range s.manualInstallCommandLines(width-2, s.update.manualCommand, focused, chrome) {
				addLine(ind.Render(line))
			}
			if hint := s.fieldHint(sfUpdateManualCommand); hint != "" {
				addLine(ind.Render(s.renderInlineHint(hintIndent+hint, width-2, chrome)))
			}
			addLine(blank)
		}
		if s.update.restartable {
			addAction("Restart now", "launch updated Tide", sfUpdateRestartNow)
		}

	case ssAI:
		markAnchor(sfProvider)
		addLine(ind.Render(s.renderProviderSelector(width-2, chrome)))
		addLine(blank)
		switch s.providerIdx {
		case 1:
			addSecretField("OpenAI key", s.openaiInput, sfAPIKey)
		case 2:
			addSecretField("Claude key", s.claudeInput, sfAPIKey)
		case 3:
			addSecretField("Gemini key", s.geminiInput, sfAPIKey)
		case 4:
			addInput("Ollama URL", s.ollamaURLInput, sfOllamaURL)
			addInput("Model", s.ollamaModelInput, sfOllamaModel)
		}
		addInput("Save summaries to", s.savePathInput, sfSavePath)

	}

	if len(body.lines) == 0 {
		body.lines = append(body.lines, blank)
	}

	return body
}

func (s Settings) inputWidth(field settingsField, maxWidth int) int {
	switch field {
	case sfFeedMaxBody:
		return min(maxWidth, 12)
	case sfBrowser, sfSavePath:
		return min(maxWidth, 36)
	case sfOllamaURL, sfOllamaModel:
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

// Wide enough for "Feed max size (MiB)" with sectionLabel horizontal padding inside Width().
const labelColW = 22

func (s Settings) viewHints(width int, chrome managerChrome) string {
	if s.focusedPane == settingsPaneSidebar {
		return renderManagerActions(width, chrome,
			"↑/↓", "section",
			"→", "edit",
			"ctrl+s", "save",
			"esc", "close",
		)
	}
	if s.isPickerField() {
		return renderManagerActions(width, chrome,
			"←/→", "change",
			"↑/↓", "field",
			"tab", "next",
			"ctrl+s", "save",
			"esc", "categories",
		)
	}
	if s.activeSection == ssUpdates && s.focusedField == sfUpdateManualCommand {
		return renderManagerActions(width, chrome,
			"enter", "copy",
			"c", "copy",
			"tab", "next",
			"ctrl+s", "save",
			"esc", "categories",
		)
	}
	return renderManagerActions(width, chrome,
		"←", "sections",
		"↑/↓", "field",
		"tab", "next",
		"ctrl+s", "save",
		"esc", "categories",
	)
}

func (s Settings) renderSectionNavRow(width int, label, subtitle string, selected, paneFocused bool, chrome managerChrome) string {
	bg, fg, subFg := chrome.baseBg, chrome.text, chrome.muted
	bold := false
	if selected {
		bg = chrome.surfaceBg
		fg = chrome.accent
		subFg = chrome.accent
		bold = true
		if paneFocused {
			bg = chrome.accent
			fg = chrome.accentFg
			subFg = chrome.accentFg
		}
	}

	innerW := max(1, width-2)
	var row string
	if subtitle == "" {
		row = padRight(truncate(label, innerW), innerW)
	} else {
		subW := lipgloss.Width(subtitle)
		labelW := max(1, innerW-subW-1)
		left := lipgloss.NewStyle().Background(bg).Foreground(fg).Bold(bold).Width(labelW).Render(truncate(label, labelW))
		spacer := lipgloss.NewStyle().Background(bg).Width(1).Render("")
		right := lipgloss.NewStyle().Background(bg).Foreground(subFg).Render(subtitle)
		row = left + spacer + right
	}
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(fg).
		Bold(bold).
		Padding(0, 1).
		Render(row)
}

func (s Settings) renderFieldLabel(label string, focused bool, _ int, chrome managerChrome) string {
	style := chrome.sectionLabel
	if focused {
		style = chrome.sectionLabelActive
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

// manualInstallCommandLines renders the "Install command" label, COPY badge, and bordered code block (one terminal line each).
// rowContentW is the usable width for this row (matches other settings rows, typically pane width − 2).
func (s Settings) manualInstallCommandLines(rowContentW int, command string, focused bool, chrome managerChrome) []string {
	labelRow := s.renderFieldLabel("Install command", focused, rowContentW, chrome) + s.renderBadge("COPY", focused, chrome)
	borderFg := lipgloss.Color("#4a4a4a")
	if focused {
		borderFg = chrome.accent
	}
	// Dark “terminal” panel; border + horizontal padding consume 4 cells (│ + pad + pad + │).
	codeBg := lipgloss.Color("#0a0a0a")
	kwFg := lipgloss.Color("#89b4fa")
	pathFg := lipgloss.Color("#a6e3a1")
	flagFg := lipgloss.Color("#fab387")
	numFg := lipgloss.Color("#f5c2e7")
	defFg := lipgloss.Color("#c6cad3")

	innerTextW := max(1, rowContentW-4)
	wrapped := wrapShellCommand(command, innerTextW)
	styledLines := make([]string, 0, len(wrapped))
	for _, line := range wrapped {
		styled := styleShellCommandLine(line, codeBg, kwFg, pathFg, flagFg, numFg, defFg)
		styledLines = append(styledLines, padStyledCodeLine(styled, codeBg, innerTextW))
	}
	inner := lipgloss.JoinVertical(lipgloss.Left, styledLines...)
	box := lipgloss.NewStyle().
		Background(codeBg).
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderFg).
		Padding(0, 1).
		Width(rowContentW).
		Render(inner)
	lines := []string{labelRow}
	lines = append(lines, strings.Split(box, "\n")...)
	return lines
}

func styleShellCommandLine(line string, bg, kwFg, pathFg, flagFg, numFg, defFg lipgloss.Color) string {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return lipgloss.NewStyle().Background(bg).Foreground(defFg).Render("")
	}
	var b strings.Builder
	for i, tok := range parts {
		if i > 0 {
			b.WriteString(lipgloss.NewStyle().Background(bg).Foreground(defFg).Render(" "))
		}
		fg := shellTokenForeground(tok, kwFg, pathFg, flagFg, numFg, defFg)
		b.WriteString(lipgloss.NewStyle().Background(bg).Foreground(fg).Render(tok))
	}
	return b.String()
}

func shellTokenForeground(tok string, kwFg, pathFg, flagFg, numFg, defFg lipgloss.Color) lipgloss.Color {
	if strings.HasPrefix(tok, "-") {
		return flagFg
	}
	trim := strings.Trim(tok, `"'`)
	low := strings.ToLower(trim)
	if shellCommandKeywords[low] {
		return kwFg
	}
	if strings.Contains(tok, "/") || strings.HasPrefix(tok, "~/") {
		return pathFg
	}
	if isNumericShellToken(trim) {
		return numFg
	}
	return defFg
}

func isNumericShellToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func padStyledCodeLine(styled string, bg lipgloss.Color, targetCells int) string {
	w := lipgloss.Width(styled)
	if w >= targetCells {
		return styled
	}
	pad := targetCells - w
	return styled + lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", pad))
}

func wrapShellCommand(s string, maxW int) []string {
	s = strings.TrimSpace(s)
	if maxW < 1 {
		maxW = 1
	}
	if s == "" {
		return []string{""}
	}
	words := strings.Fields(s)
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, w := range words {
		if lipgloss.Width(w) > maxW {
			flush()
			out = append(out, hardWrapString(w, maxW)...)
			continue
		}
		try := w
		if cur.Len() > 0 {
			try = cur.String() + " " + w
		}
		if lipgloss.Width(try) <= maxW {
			if cur.Len() > 0 {
				cur.WriteString(" ")
			}
			cur.WriteString(w)
		} else {
			flush()
			cur.WriteString(w)
		}
	}
	flush()
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func hardWrapString(s string, maxW int) []string {
	if maxW < 1 {
		maxW = 1
	}
	var out []string
	runes := []rune(s)
	for len(runes) > 0 {
		var b strings.Builder
		for len(runes) > 0 {
			nextR := runes[0]
			cand := b.String() + string(nextR)
			if b.Len() > 0 && lipgloss.Width(cand) > maxW {
				break
			}
			if b.Len() == 0 && lipgloss.Width(string(nextR)) > maxW {
				b.WriteRune(nextR)
				runes = runes[1:]
				break
			}
			b.WriteRune(nextR)
			runes = runes[1:]
		}
		if b.Len() > 0 {
			out = append(out, b.String())
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
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
		return "checking for Tide updates..."
	case updateStateAvailable:
		if u.dismissed {
			return "Tide update dismissed"
		}
		return "Tide update available"
	case updateStateDownloading:
		return "downloading Tide update..."
	case updateStateInstalling:
		return "installing Tide update..."
	case updateStateInstalled:
		if u.installedVersion != "" {
			return "Tide updated to " + u.installedVersion
		}
		return "Tide update installed"
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
	pickerW := max(1, width-labelColW)
	return label + renderSettingsPicker(pickerW, providerName, focused, chrome)
}

func renderSettingsPicker(width int, value string, focused bool, chrome managerChrome) string {
	maxTextW := max(1, width-5)
	bg := chrome.surfaceBg
	fg := chrome.text
	accentFg := chrome.muted
	if focused {
		bg = chrome.accent
		fg = chrome.accentFg
		accentFg = chrome.accentFg
	}
	value = truncate(value, maxTextW)
	text := lipgloss.NewStyle().Background(bg).Foreground(fg)
	accent := lipgloss.NewStyle().Background(bg).Foreground(accentFg).Bold(true)
	line := accent.Render("◀ ") + text.Render(value) + accent.Render(" ▶")
	return lipgloss.NewStyle().Background(bg).Padding(0, 1).Render(line)
}

func (s Settings) fieldHint(field settingsField) string {
	switch field {
	case sfBrowser:
		return "leave blank to use the system default browser"
	case sfFeedMaxBody:
		return "larger feeds need more memory; default is 10 MiB"
	case sfAPIKey:
		if s.saveError != "" {
			return s.saveError
		}
		if msg, ok := s.selectedAIKeyValidation(); msg != "" {
			if ok {
				return msg + "; only the active provider key is used"
			}
			return msg
		}
		return "only the active provider key is used; press enter or tab when done"
	case sfOllamaURL:
		return "local Ollama endpoint"
	case sfSavePath:
		return "directory for exported markdown summaries"
	case sfUpdateManualCommand:
		return "enter or c copies the command"
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

func (s Settings) renderAboutSection(width int, chrome managerChrome) settingsSectionBody {
	ind := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).PaddingLeft(2)
	blank := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render("")
	bodyW := max(1, width-2)
	lines := []string{}
	addBlock := func(block string) int {
		start := len(lines)
		lines = append(lines, strings.Split(block, "\n")...)
		return start
	}
	heroStart := addBlock(ind.Render(s.renderAboutHero(bodyW, chrome)))
	_ = heroStart
	lines = append(lines, blank)
	linksStart := addBlock(ind.Render(s.renderAboutLinks(bodyW, chrome)))
	lines = append(lines, blank)
	addBlock(ind.Render(s.renderAboutClosingNote(bodyW, chrome)))
	return settingsSectionBody{
		lines: lines,
		anchors: map[settingsField]int{
			sfAboutRepo:   linksStart,
			sfAboutIssues: linksStart,
		},
	}
}

func (s Settings) renderAboutHero(width int, chrome managerChrome) string {
	panelW := max(1, width-4)
	contentW := max(1, panelW-2)
	tagline := aboutCenterText(truncate("Your feeds, no algorithm, no bullshit", contentW), contentW)
	lines := []string{
		s.renderAboutHeroTextLine(aboutCenterText("TIDE", contentW), contentW, 0, true),
		s.renderAboutHeroTextLine("", contentW, 1, false),
		s.renderAboutHeroTextLine(tagline, contentW, 2, false),
		s.renderAboutHeroTextLine("", contentW, 3, false),
	}

	panelBg := lipgloss.Color("#000000")
	panel := lipgloss.NewStyle().
		Width(panelW).
		Background(panelBg).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#193847")).
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

	heartBlock := lipgloss.NewStyle().
		Background(chrome.baseBg).
		PaddingTop(2).
		Render(heart)

	return lipgloss.NewStyle().
		Background(chrome.baseBg).
		PaddingTop(4).
		Render(lipgloss.JoinVertical(lipgloss.Left, signoff, heartBlock))
}

func (s Settings) renderAboutHeroTextLine(text string, width, row int, bold bool) string {
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
		sceneBg := aboutHeroBackground(s.aboutGradientFrame, row, i, width)
		sceneFg := aboutHeroTextForeground(sceneBg, row, ch)
		reveal, shimmer := aboutHeroRevealMask(s.aboutGradientFrame, row, i, width)
		bg := aboutHeroMaskedBackground(sceneBg, reveal, shimmer)
		fg := aboutHeroMaskedForeground(sceneFg, bg, reveal, shimmer, ch != ' ')
		b.WriteString(renderAboutHeroCell(ch, bg, fg, bold && reveal > 0.32))
	}
	return b.String()
}

func (s Settings) renderAboutHeroWaveLine(width, row int) string {
	pattern := aboutHeroWavePattern(s.aboutGradientFrame, row, width)

	var b strings.Builder
	for i, ch := range []rune(pattern) {
		sceneBg := aboutHeroBackground(s.aboutGradientFrame, row, i, width)
		foam := aboutHeroFoamSample(s.aboutGradientFrame, row, i, width)
		sceneFg := aboutHeroWaveForeground(sceneBg, row, ch, foam)
		reveal, shimmer := aboutHeroRevealMask(s.aboutGradientFrame, row, i, width)
		bg := aboutHeroMaskedBackground(sceneBg, reveal, shimmer)
		fg := aboutHeroMaskedForeground(sceneFg, bg, reveal, shimmer, ch != ' ')
		b.WriteString(renderAboutHeroCell(ch, bg, fg, foam > 0.45 && ch != ' ' && reveal > 0.28))
	}
	return b.String()
}

func renderAboutHeroCell(ch rune, bg, fg lipgloss.Color, bold bool) string {
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(fg).
		Bold(bold).
		Render(string(ch))
}

func aboutHeroBackground(frame, row, col, width int) lipgloss.Color {
	skyTop := lipgloss.Color("#071524")
	skyMid := lipgloss.Color("#0d2d45")
	horizon := lipgloss.Color("#15556f")
	waterTop := lipgloss.Color("#0c3b56")
	waterMid := lipgloss.Color("#0a3149")
	waterDeep := lipgloss.Color("#072235")
	foamGlow := lipgloss.Color("#5ca6bf")

	x := float64(col) / math.Max(1, float64(width-1))

	switch row {
	case 0:
		glow := clamp01(0.22 + 0.18*math.Sin(x*math.Pi))
		bg := blendLipglossColor(skyTop, skyMid, glow)
		bg = blendLipglossColor(bg, horizon, clamp01(0.12+0.10*math.Sin(x*6.4)))
		return bg
	case 1:
		horizonGlow := clamp01(0.42 + 0.24*math.Sin((x-0.5)*math.Pi))
		bg := blendLipglossColor(skyMid, horizon, horizonGlow)
		return blendLipglossColor(bg, foamGlow, clamp01(0.05+0.08*math.Sin(x*9.0)))
	case 2:
		swell := clamp01(0.26 + 0.18*math.Sin(x*8.8) + 0.12*math.Sin(x*15.0))
		bg := blendLipglossColor(waterTop, waterMid, swell)
		return blendLipglossColor(bg, horizon, clamp01(0.10+0.08*math.Sin(x*6.5)))
	default:
		trough := clamp01(0.22 + 0.16*math.Sin(x*10.5) + 0.08*math.Sin(x*21.0))
		bg := blendLipglossColor(waterMid, waterDeep, trough)
		return blendLipglossColor(bg, foamGlow, clamp01(0.06+0.05*math.Sin(x*12.0)))
	}
}

func aboutHeroTextForeground(bg lipgloss.Color, row int, ch rune) lipgloss.Color {
	if ch == ' ' {
		return readableText(lipgloss.Color("#dceef6"), bg, 4.5)
	}
	switch row {
	case 0:
		return readableText(lipgloss.Color("#f4fbff"), bg, 5)
	default:
		return readableText(lipgloss.Color("#c2dde9"), bg, 4.5)
	}
}

func aboutHeroWavePattern(frame, row, width int) string {
	switch row {
	case 2:
		return aboutRepeatToLen("   ~~~      __/\\__      ~~~~      _/\\_      ", width)
	default:
		return aboutRepeatToLen("  __/\\\\____/\\\\\\___..___/\\\\____/\\\\\\__  ", width)
	}
}

func aboutHeroFoamSample(frame, row, col, width int) float64 {
	return 0
}

func aboutHeroWaveForeground(bg lipgloss.Color, row int, ch rune, foam float64) lipgloss.Color {
	base := lipgloss.Color("#9fd1df")
	if row == 3 {
		base = lipgloss.Color("#d8f2fb")
	}
	if ch == ' ' {
		base = lipgloss.Color("#6ba0b3")
	}
	if ch == '.' {
		base = lipgloss.Color("#eefcff")
	}
	if foam > 0 && ch != ' ' {
		base = blendLipglossColor(base, lipgloss.Color("#ffffff"), clamp01(foam*0.75))
	}
	if contrastRatio(base, bg) < 4.5 {
		return readableText(base, bg, 4.5)
	}
	return base
}

func aboutHeroRevealMask(frame, row, col, width int) (float64, float64) {
	span := math.Max(6, float64(width-1))
	travel := span + 16
	head := math.Mod(float64(frame)*0.78, travel) - 8
	center := head + float64(row)*1.2
	x := float64(col)

	trailWidth := math.Max(4.5, float64(width)*0.12)
	headWidth := math.Max(1.4, float64(width)*0.03)
	trailCenter := center - 4.2

	trail := math.Exp(-math.Pow((x-trailCenter)/trailWidth, 2))
	band := math.Exp(-math.Pow((x-center)/(trailWidth*0.62), 2))
	shimmer := math.Exp(-math.Pow((x-center)/headWidth, 2))

	reveal := clamp01(trail*0.72 + band*0.54)
	return reveal, clamp01(shimmer)
}

func aboutHeroMaskedBackground(sceneBg lipgloss.Color, reveal, shimmer float64) lipgloss.Color {
	bg := blendLipglossColor(lipgloss.Color("#000000"), sceneBg, clamp01(reveal))
	if shimmer > 0 {
		bg = blendLipglossColor(bg, lipgloss.Color("#7edfff"), clamp01(shimmer*0.22))
	}
	return bg
}

func aboutHeroMaskedForeground(sceneFg, bg lipgloss.Color, reveal, shimmer float64, visible bool) lipgloss.Color {
	if !visible || reveal <= 0.03 {
		return lipgloss.Color("#000000")
	}
	fg := blendLipglossColor(lipgloss.Color("#000000"), sceneFg, clamp01(reveal*1.08))
	if shimmer > 0 {
		fg = blendLipglossColor(fg, lipgloss.Color("#ffffff"), clamp01(0.32+shimmer*0.46))
	}
	if contrastRatio(fg, bg) < 4.5 {
		return readableText(fg, bg, 4.5)
	}
	return fg
}

func aboutCenterText(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return string(runes[:width])
	}
	left := (width - len(runes)) / 2
	right := width - len(runes) - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func aboutRepeatToLen(pattern string, n int) string {
	if pattern == "" {
		return strings.Repeat(" ", n)
	}
	var b strings.Builder
	for b.Len() < n+len(pattern) {
		b.WriteString(pattern)
	}
	return b.String()[:n]
}

func aboutScrollLeft(pattern string, width, offset int) string {
	base := aboutRepeatToLen(pattern, width+len(pattern))
	shift := offset % len(pattern)
	return base[shift : shift+width]
}

func aboutScrollRight(pattern string, width, offset int) string {
	shift := offset % len(pattern)
	base := aboutRepeatToLen(pattern, width+len(pattern))
	start := len(pattern) - shift
	if start == len(pattern) {
		start = 0
	}
	return base[start : start+width]
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

func rgbLipglossColor(r, g, b int) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
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
