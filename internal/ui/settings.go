package ui

import (
	"strings"

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
	sfProvider
	sfAPIKey    // visible when provider is openai/claude/gemini
	sfOllamaURL // visible when provider is ollama
	sfOllamaModel
	sfSavePath
)

var (
	aiProviderLabels = []string{"none", "OpenAI", "Claude", "Gemini", "Ollama"}
	aiProviderIDs    = []string{"", "openai", "claude", "gemini", "ollama"}
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
	icons          bool
	dateAbsolute   bool // false = relative, true = absolute
	markReadOnOpen bool
	browserInput   textinput.Model

	// AI
	providerIdx      int
	openaiInput      textinput.Model
	claudeInput      textinput.Model
	geminiInput      textinput.Model
	ollamaURLInput   textinput.Model
	ollamaModelInput textinput.Model
	savePathInput    textinput.Model

	focusedField settingsField
	shouldSave   bool
	shouldExit   bool
}

func newSettings(cfg config.Config) Settings {
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
		icons:          cfg.Display.Icons,
		dateAbsolute:   cfg.Display.DateFormat == "absolute",
		markReadOnOpen: cfg.Display.MarkReadOnOpen,
		browserInput:   mkInput(cfg.Display.Browser, "xdg-open", false),
		providerIdx:    providerIndex(cfg.AI.Provider),
		openaiInput:    mkInput(cfg.AI.OpenAIKey, "sk-...", true),
		claudeInput:    mkInput(cfg.AI.ClaudeKey, "sk-ant-...", true),
		geminiInput:    mkInput(cfg.AI.GeminiKey, "AIza...", true),
		ollamaURLInput: mkInput(cfg.AI.OllamaURL, "http://localhost:11434", false),
		ollamaModelInput: mkInput(cfg.AI.OllamaModel, "llama3.2", false),
		savePathInput:  mkInput(cfg.AI.SavePath, "~/", false),
		focusedField:   sfIcons,
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

	cfg.AI.Provider = aiProviderIDs[s.providerIdx]
	cfg.AI.OpenAIKey = strings.TrimSpace(s.openaiInput.Value())
	cfg.AI.ClaudeKey = strings.TrimSpace(s.claudeInput.Value())
	cfg.AI.GeminiKey = strings.TrimSpace(s.geminiInput.Value())
	cfg.AI.OllamaURL = strings.TrimSpace(s.ollamaURLInput.Value())
	cfg.AI.OllamaModel = strings.TrimSpace(s.ollamaModelInput.Value())
	cfg.AI.SavePath = strings.TrimSpace(s.savePathInput.Value())
	return cfg
}

// ── Focus management ──────────────────────────────────────────────────────────

func (s *Settings) applyFocus() {
	s.browserInput.Blur()
	s.openaiInput.Blur()
	s.claudeInput.Blur()
	s.geminiInput.Blur()
	s.ollamaURLInput.Blur()
	s.ollamaModelInput.Blur()
	s.savePathInput.Blur()

	switch s.focusedField {
	case sfBrowser:
		s.browserInput.Focus()
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
	fields := []settingsField{sfIcons, sfDateFormat, sfMarkReadOnOpen, sfBrowser, sfProvider}
	switch s.providerIdx {
	case 4: // ollama
		fields = append(fields, sfOllamaURL, sfOllamaModel)
	case 1, 2, 3: // openai, claude, gemini
		fields = append(fields, sfAPIKey)
	}
	fields = append(fields, sfSavePath)
	return fields
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
	switch s.focusedField {
	case sfBrowser, sfAPIKey, sfOllamaURL, sfOllamaModel, sfSavePath:
		return true
	}
	return false
}

// ── Update ────────────────────────────────────────────────────────────────────

func (s Settings) Update(msg tea.Msg, keys KeyMap) (Settings, tea.Cmd, bool) {
	// Route cursor-blink ticks to the active text input.
	if _, ok := msg.(tea.KeyMsg); !ok {
		if s.isTextInput() {
			var cmd tea.Cmd
			switch s.focusedField {
			case sfBrowser:
				s.browserInput, cmd = s.browserInput.Update(msg)
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
		return s, nil, false
	}

	key := msg.(tea.KeyMsg)

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

	// Tab / shift+tab / j / k navigate between fields.
	switch {
	case keyMatches(key, keys.Tab):
		s.focusedField = s.nextField()
		s.applyFocus()
		return s, nil, false

	case keyMatches(key, keys.PrevPane) || key.String() == "shift+tab":
		s.focusedField = s.prevField()
		s.applyFocus()
		return s, nil, false
	}

	// Field-specific handling.
	switch s.focusedField {
	case sfIcons:
		if key.String() == " " || keyMatches(key, keys.Enter) {
			s.icons = !s.icons
		} else if keyMatches(key, keys.Down) || keyMatches(key, keys.Right) {
			s.focusedField = s.nextField()
			s.applyFocus()
		} else if keyMatches(key, keys.Up) || keyMatches(key, keys.Left) {
			s.focusedField = s.prevField()
			s.applyFocus()
		}

	case sfDateFormat:
		if key.String() == " " || keyMatches(key, keys.Enter) {
			s.dateAbsolute = !s.dateAbsolute
		} else if keyMatches(key, keys.Down) || keyMatches(key, keys.Right) {
			s.focusedField = s.nextField()
			s.applyFocus()
		} else if keyMatches(key, keys.Up) || keyMatches(key, keys.Left) {
			s.focusedField = s.prevField()
			s.applyFocus()
		}

	case sfMarkReadOnOpen:
		if key.String() == " " || keyMatches(key, keys.Enter) {
			s.markReadOnOpen = !s.markReadOnOpen
		} else if keyMatches(key, keys.Down) || keyMatches(key, keys.Right) {
			s.focusedField = s.nextField()
			s.applyFocus()
		} else if keyMatches(key, keys.Up) || keyMatches(key, keys.Left) {
			s.focusedField = s.prevField()
			s.applyFocus()
		}

	case sfProvider:
		switch {
		case keyMatches(key, keys.Left):
			if s.providerIdx > 0 {
				s.providerIdx--
			} else {
				s.providerIdx = len(aiProviderLabels) - 1
			}
			// Re-validate focusedField against new visible fields.
			s.focusedField = sfProvider
			s.applyFocus()
		case keyMatches(key, keys.Right) || keyMatches(key, keys.Enter):
			s.providerIdx = (s.providerIdx + 1) % len(aiProviderLabels)
			s.focusedField = sfProvider
			s.applyFocus()
		case keyMatches(key, keys.Down):
			s.focusedField = s.nextField()
			s.applyFocus()
		case keyMatches(key, keys.Up):
			s.focusedField = s.prevField()
			s.applyFocus()
		}

	case sfBrowser, sfAPIKey, sfOllamaURL, sfOllamaModel, sfSavePath:
		// Enter advances to next field; everything else goes to the text input.
		if keyMatches(key, keys.Enter) {
			s.focusedField = s.nextField()
			s.applyFocus()
			return s, nil, false
		}
		if keyMatches(key, keys.Up) {
			s.focusedField = s.prevField()
			s.applyFocus()
			return s, nil, false
		}
		if keyMatches(key, keys.Down) {
			s.focusedField = s.nextField()
			s.applyFocus()
			return s, nil, false
		}
		var cmd tea.Cmd
		switch s.focusedField {
		case sfBrowser:
			s.browserInput, cmd = s.browserInput.Update(msg)
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

	return s, nil, false
}

// ── View ──────────────────────────────────────────────────────────────────────

func (s Settings) View(width, height int, chrome managerChrome) string {
	header := renderManagerHeader("SETTINGS", width, chrome)
	gap := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render("")

	body := s.viewBody(width, chrome)
	hints := renderManagerActions(width, chrome,
		"ctrl+s", "save",
		"esc", "cancel",
	)

	bodyH := max(1, height-lipgloss.Height(header)-lipgloss.Height(gap)-lipgloss.Height(hints))
	body = clampView(body, width, bodyH, chrome.baseBg)

	return lipgloss.JoinVertical(lipgloss.Left, header, gap, body, hints)
}

func (s Settings) viewBody(width int, chrome managerChrome) string {
	ind := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).PaddingLeft(2)
	blank := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render("")

	// inputW: full inner width minus the 2-char left padding of ind.
	inputW := width - 3

	addToggle := func(lines []string, label string, on bool, field settingsField) []string {
		focused := s.focusedField == field
		return append(lines,
			ind.Render(s.renderToggle(label, on, focused, width-2, chrome)),
			blank,
		)
	}

	addInput := func(lines []string, label string, input textinput.Model, field settingsField) []string {
		focused := s.focusedField == field
		return append(lines,
			ind.Render(s.renderFieldLabel(label, focused, width-2, chrome)),
			ind.Render(renderTextInput(input, inputW, focused, chrome)),
			blank,
		)
	}

	lines := []string{
		s.renderSectionLabel("DISPLAY", width, chrome),
		blank,
	}
	lines = addToggle(lines, "Icons", s.icons, sfIcons)
	lines = addToggle(lines, "Dates — "+s.dateLabel(), !s.dateAbsolute, sfDateFormat)
	lines = addToggle(lines, "Mark read on open", s.markReadOnOpen, sfMarkReadOnOpen)
	lines = addInput(lines, "Browser command", s.browserInput, sfBrowser)

	lines = append(lines,
		s.renderSectionLabel("AI SUMMARIES", width, chrome),
		blank,
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

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (s Settings) dateLabel() string {
	if s.dateAbsolute {
		return "absolute"
	}
	return "relative"
}

func (s Settings) renderSectionLabel(label string, width int, chrome managerChrome) string {
	return chrome.sectionLabel.Width(width).Render(label)
}

const labelColW = 20

func (s Settings) renderFieldLabel(label string, focused bool, _ int, chrome managerChrome) string {
	style := chrome.body
	if focused {
		style = style.Foreground(chrome.text).Bold(true)
	} else {
		style = style.Foreground(chrome.muted)
	}
	return style.Width(labelColW).Render(padRight(label, labelColW))
}

func (s Settings) renderToggle(label string, on bool, focused bool, width int, chrome managerChrome) string {
	label = s.renderFieldLabel(label, focused, width, chrome)

	val := " OFF "
	if on {
		val = " ON  "
	}
	var badge string
	if focused {
		badge = chrome.key.Render(val)
	} else {
		badge = lipgloss.NewStyle().
			Background(chrome.surfaceBg).
			Foreground(chrome.muted).
			Padding(0, 0).
			Render(val)
	}
	return label + badge
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
		left := chrome.keyLabel.Render(" ◀ ")
		name := chrome.key.Render(" " + providerName + " ")
		right := chrome.keyLabel.Render(" ▶ ")
		selector = left + name + right
	} else {
		selector = lipgloss.NewStyle().
			Background(chrome.surfaceBg).
			Foreground(chrome.muted).
			Render(" " + providerName + " ")
	}
	return label + selector
}
