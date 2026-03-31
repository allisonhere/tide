package ui

import (
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
)

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

	activeSection settingsSection
	focusedPane   settingsPaneFocus
	sectionField  [settingsSectionCount]settingsField
	focusedField  settingsField
	shouldSave    bool
	shouldExit    bool
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
	default:
		return nil
	}
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
			return s, nil, false
		case keyMatches(key, keys.Down):
			s.setActiveSection(s.nextSection())
			return s, nil, false
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
