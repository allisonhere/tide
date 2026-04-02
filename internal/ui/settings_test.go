package ui

import (
	"math"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tide/internal/config"
)

func TestSettingsTextInputsAcceptMovementRunes(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setFocusedPane(settingsPaneDetail)
	s.focusedField = sfBrowser
	s.browserInput.SetValue("")
	s.applyFocus()

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, DefaultKeys)

	if next.focusedField != sfBrowser {
		t.Fatalf("expected browser field to keep focus, got %v", next.focusedField)
	}
	if got := next.browserInput.Value(); got != "k" {
		t.Fatalf("expected browser input to receive typed rune, got %q", got)
	}
}

func TestSettingsTextInputsAcceptOpenBracket(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setFocusedPane(settingsPaneDetail)
	s.focusedField = sfBrowser
	s.browserInput.SetValue("")
	s.applyFocus()

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}}, DefaultKeys)

	if next.focusedField != sfBrowser {
		t.Fatalf("expected browser field to keep focus, got %v", next.focusedField)
	}
	if got := next.browserInput.Value(); got != "[" {
		t.Fatalf("expected browser input to receive typed rune, got %q", got)
	}
}

func TestSettingsBadgeWidthStaysStableAcrossFocus(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	chrome := newManagerChrome(62, CatppuccinMocha)

	focused := s.renderBadge("ON", true, chrome)
	unfocused := s.renderBadge("ON", false, chrome)

	if lipgloss.Width(focused) != lipgloss.Width(unfocused) {
		t.Fatalf("expected focused and unfocused badges to have equal width, got %d and %d", lipgloss.Width(focused), lipgloss.Width(unfocused))
	}
}

func TestSettingsStartsFocusedOnSidebar(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})

	if s.focusedPane != settingsPaneSidebar {
		t.Fatalf("expected settings to start focused on sidebar, got %v", s.focusedPane)
	}
	if s.activeSection != ssDisplay {
		t.Fatalf("expected settings to start on DISPLAY section, got %v", s.activeSection)
	}
}

func TestSettingsLeftKeyMovesFocusToSidebar(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyLeft}, DefaultKeys)

	if next.focusedPane != settingsPaneSidebar {
		t.Fatalf("expected left key to move focus to sidebar, got %v", next.focusedPane)
	}
	if next.activeSection != ssDisplay {
		t.Fatalf("expected active section to remain DISPLAY, got %v", next.activeSection)
	}
}

func TestSettingsSidebarDownChangesSection(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setFocusedPane(settingsPaneSidebar)

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyDown}, DefaultKeys)

	if next.activeSection != ssFeeds {
		t.Fatalf("expected active section to move to FEEDS, got %v", next.activeSection)
	}
	if next.focusedField != sfFeedMaxBody {
		t.Fatalf("expected FEEDS section to restore feed max body field, got %v", next.focusedField)
	}
}

func TestSettingsSidebarRightEntersDetail(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setFocusedPane(settingsPaneSidebar)
	s.setActiveSection(ssFeeds)

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyRight}, DefaultKeys)

	if next.focusedPane != settingsPaneDetail {
		t.Fatalf("expected right key to enter detail pane, got %v", next.focusedPane)
	}
	if next.focusedField != sfFeedMaxBody {
		t.Fatalf("expected FEEDS section to focus feed max body field, got %v", next.focusedField)
	}
}

func TestSettingsRestoresLastFocusedFieldPerSection(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssDisplay)
	s.setFocusedField(sfBrowser)
	s.setActiveSection(ssAI)
	s.setFocusedField(sfSavePath)

	s.setActiveSection(ssDisplay)
	if s.focusedField != sfBrowser {
		t.Fatalf("expected DISPLAY section to restore browser focus, got %v", s.focusedField)
	}

	s.setActiveSection(ssAI)
	if s.focusedField != sfSavePath {
		t.Fatalf("expected AI section to restore save path focus, got %v", s.focusedField)
	}
}

func TestSettingsTextInputLeftArrowMovesToSidebarAtCursorStart(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssDisplay)
	s.setFocusedPane(settingsPaneDetail)
	s.setFocusedField(sfBrowser)
	s.browserInput.SetValue("abc")
	s.browserInput.CursorStart()

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyLeft}, DefaultKeys)

	if next.focusedPane != settingsPaneSidebar {
		t.Fatalf("expected text input to move focus to sidebar at cursor start, got %v", next.focusedPane)
	}
	if next.activeSection != ssDisplay {
		t.Fatalf("expected active section to remain DISPLAY, got %v", next.activeSection)
	}
}

func TestSettingsTextInputLeftArrowStaysInDetailWhenCursorCanMoveLeft(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssDisplay)
	s.setFocusedPane(settingsPaneDetail)
	s.setFocusedField(sfBrowser)
	s.browserInput.SetValue("abc")

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyLeft}, DefaultKeys)

	if next.focusedPane != settingsPaneDetail {
		t.Fatalf("expected text input to keep detail focus, got %v", next.focusedPane)
	}
	if got := next.browserInput.Position(); got != 2 {
		t.Fatalf("expected browser cursor to move left to position 2, got %d", got)
	}
}

func TestSettingsAPIKeyLeftArrowMovesToSidebarAtCursorStart(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Provider = "openai"
	cfg.AI.OpenAIKey = "sk-test"

	s := newSettings(cfg, settingsUpdateState{})
	s.setActiveSection(ssAI)
	s.setFocusedPane(settingsPaneDetail)
	s.setFocusedField(sfAPIKey)
	s.openaiInput.CursorStart()

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyLeft}, DefaultKeys)

	if next.focusedPane != settingsPaneSidebar {
		t.Fatalf("expected API key field to move focus to sidebar at cursor start, got %v", next.focusedPane)
	}
	if next.activeSection != ssAI {
		t.Fatalf("expected active section to remain AI, got %v", next.activeSection)
	}
}

func TestSettingsProviderLeftArrowCyclesBackwardAndKeepsDetailFocus(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAI)
	s.setFocusedPane(settingsPaneDetail)
	s.setFocusedField(sfProvider)
	s.providerIdx = providerIndex("openai")

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyLeft}, DefaultKeys)

	if next.focusedPane != settingsPaneDetail {
		t.Fatalf("expected provider picker to keep detail focus, got %v", next.focusedPane)
	}
	if got := aiProviderIDs[next.providerIdx]; got != "" {
		t.Fatalf("expected left arrow to move provider backward to none, got %q", got)
	}
}

func TestSettingsProviderRightArrowCyclesForward(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAI)
	s.setFocusedPane(settingsPaneDetail)
	s.setFocusedField(sfProvider)

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyRight}, DefaultKeys)

	if got := aiProviderIDs[next.providerIdx]; got != "openai" {
		t.Fatalf("expected right arrow to move provider forward to openai, got %q", got)
	}
}

func TestSettingsAPIKeyHintFlagsProviderMismatch(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Provider = "claude"
	cfg.AI.ClaudeKey = "sk-proj-test123"

	s := newSettings(cfg, settingsUpdateState{})

	if got := s.fieldHint(sfAPIKey); !strings.Contains(got, "looks like OpenAI, but Claude is selected") {
		t.Fatalf("expected mismatch hint, got %q", got)
	}
}

func TestSettingsAPIKeyHintConfirmsMatchingFormat(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Provider = "gemini"
	cfg.AI.GeminiKey = "AIzaTestKey"

	s := newSettings(cfg, settingsUpdateState{})

	if got := s.fieldHint(sfAPIKey); !strings.Contains(got, "format looks like Gemini") {
		t.Fatalf("expected matching format hint, got %q", got)
	}
}

func TestSettingsCtrlSSaveBlockedByInvalidSelectedProviderKey(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Provider = "claude"
	cfg.AI.ClaudeKey = "sk-proj-test123"

	s := newSettings(cfg, settingsUpdateState{})
	s.setActiveSection(ssAI)
	s.setFocusedPane(settingsPaneDetail)
	s.setFocusedField(sfSavePath)

	next, _, done := s.Update(tea.KeyMsg{Type: tea.KeyCtrlS}, DefaultKeys)

	if done {
		t.Fatal("expected invalid provider/key pair to block save")
	}
	if next.shouldSave {
		t.Fatal("expected invalid provider/key pair not to mark settings for save")
	}
	if next.activeSection != ssAI {
		t.Fatalf("expected validation failure to keep AI section active, got %v", next.activeSection)
	}
	if next.focusedField != sfAPIKey {
		t.Fatalf("expected validation failure to focus API key field, got %v", next.focusedField)
	}
	if !strings.Contains(next.saveError, "looks like OpenAI, but Claude is selected") {
		t.Fatalf("expected save error to explain mismatch, got %q", next.saveError)
	}
}

func TestSettingsAboutViewShowsLinksAndTagline(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)

	view := s.View(100, 30, newManagerChrome(100, CatppuccinMocha))

	for _, want := range []string{
		"ABOUT",
		"github.com/allisonhere",
		"Thanks for taking a look -allie",
		"Your feeds, no algorithm, no bullshit",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected about view to contain %q, got %q", want, view)
		}
	}
}

func TestSettingsAboutActions(t *testing.T) {
	tests := []struct {
		name       string
		field      settingsField
		wantAction settingsAction
	}{
		{name: "repo", field: sfAboutRepo, wantAction: settingsActionOpenRepo},
		{name: "issues", field: sfAboutIssues, wantAction: settingsActionOpenIssues},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSettings(config.DefaultConfig(), settingsUpdateState{})
			s.setActiveSection(ssAbout)
			s.setFocusedPane(settingsPaneDetail)
			s.setFocusedField(tt.field)

			next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter}, DefaultKeys)

			if got := next.takeAction(); got != tt.wantAction {
				t.Fatalf("expected action %v, got %v", tt.wantAction, got)
			}
		})
	}
}

func TestSettingsAboutAnimationTicksOnlyWhenVisible(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)

	next, cmd, _ := s.Update(settingsAboutPulseMsg{}, DefaultKeys)
	if next.aboutGradientFrame != 1 {
		t.Fatalf("expected about gradient frame to advance to 1, got %d", next.aboutGradientFrame)
	}
	if cmd == nil {
		t.Fatal("expected about animation tick to schedule another tick")
	}

	next.setActiveSection(ssDisplay)
	after, cmd, _ := next.Update(settingsAboutPulseMsg{}, DefaultKeys)
	if after.aboutGradientFrame != 0 {
		t.Fatalf("expected about gradient frame to reset outside ABOUT, got %d", after.aboutGradientFrame)
	}
	if cmd != nil {
		t.Fatal("expected no follow-up tick when ABOUT is not visible")
	}
}

func TestSettingsAboutAnimationDoesNotWrapEarly(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)

	for i := 0; i < 30; i++ {
		next, _, _ := s.Update(settingsAboutPulseMsg{}, DefaultKeys)
		s = next
	}

	if s.aboutGradientFrame != 30 {
		t.Fatalf("expected about gradient frame to continue smoothly past 24 ticks, got %d", s.aboutGradientFrame)
	}
}

func TestAboutHeroBackgroundStaysStaticAcrossFrames(t *testing.T) {
	for _, col := range []int{4, 11, 19} {
		if got, want := aboutHeroBackground(0, 2, col, 24), aboutHeroBackground(8, 2, col, 24); got != want {
			t.Fatalf("expected static hero background at col %d, got %q want %q", col, got, want)
		}
	}
}

func TestAboutHeroSpacerRowsAreBlank(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	for _, row := range []int{1, 3} {
		line := ansi.Strip(s.renderAboutHeroTextLine("", 28, row, false))
		if strings.TrimSpace(line) != "" {
			t.Fatalf("expected blank spacer row %d, got %q", row, line)
		}
	}
}

func TestAboutHeroRevealMaskRevealsSceneUnderShimmer(t *testing.T) {
	frame := 18
	width := 48
	headCol := int(math.Round(math.Mod(float64(frame)*0.78, math.Max(6, float64(width-1))+16) - 8))
	revealNear, shimmerNear := aboutHeroRevealMask(frame, 2, headCol, width)
	revealFar, shimmerFar := aboutHeroRevealMask(frame, 2, 0, width)
	if revealNear <= revealFar {
		t.Fatalf("expected reveal %.2f to exceed far reveal %.2f", revealNear, revealFar)
	}
	sceneBg := aboutHeroBackground(frame, 2, headCol, width)
	nearBg := aboutHeroMaskedBackground(sceneBg, revealNear, shimmerNear)
	farBg := aboutHeroMaskedBackground(sceneBg, revealFar, shimmerFar)
	if nearBg == farBg {
		t.Fatalf("expected shimmer reveal background %q to differ from masked background %q", nearBg, farBg)
	}
}

func TestSettingsAboutTaglineStaysOnOneLine(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)
	s.aboutGradientFrame = 18
	view := s.renderAboutHero(56, newManagerChrome(84, CatppuccinMocha))
	if strings.Count(ansi.Strip(view), "Your feeds, no algorithm, no bullshit") != 1 {
		t.Fatalf("expected tagline to render once on a single line, got %q", ansi.Strip(view))
	}
}

func TestSettingsAboutHeroKeepsPreviousHeight(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)
	view := s.renderAboutHero(56, newManagerChrome(84, CatppuccinMocha))
	if got := lipgloss.Height(view); got != 6 {
		t.Fatalf("expected about hero height 6, got %d", got)
	}
}

func TestSettingsAboutGradientDoesNotShiftHeroLayout(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)
	chrome := newManagerChrome(84, CatppuccinMocha)

	before := s.renderAboutHero(56, chrome)
	s.aboutGradientFrame = 7
	after := s.renderAboutHero(56, chrome)

	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	if len(beforeLines) != len(afterLines) {
		t.Fatalf("line count changed across about animation frames: before=%d after=%d", len(beforeLines), len(afterLines))
	}
	for i := range beforeLines {
		if lipgloss.Width(beforeLines[i]) != lipgloss.Width(afterLines[i]) {
			t.Fatalf("line %d width changed across about animation frames: before=%d after=%d", i+1, lipgloss.Width(beforeLines[i]), lipgloss.Width(afterLines[i]))
		}
	}
}

func TestSettingsAboutLinksUseTwoColumnsWhenWide(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAbout)
	chrome := newManagerChrome(96, CatppuccinMocha)

	wide := s.renderAboutLinks(60, chrome)
	narrow := s.renderAboutLinks(40, chrome)

	if lipgloss.Height(wide) >= lipgloss.Height(narrow) {
		t.Fatalf("expected wide about links to be shorter than stacked narrow layout: wide=%d narrow=%d", lipgloss.Height(wide), lipgloss.Height(narrow))
	}
}

func TestSettingsActionURLMapping(t *testing.T) {
	if got := settingsActionURL(settingsActionOpenRepo); got != tideRepoURL {
		t.Fatalf("expected repo action URL %q, got %q", tideRepoURL, got)
	}
	if got := settingsActionURL(settingsActionOpenIssues); got != tideIssuesURL {
		t.Fatalf("expected issues action URL %q, got %q", tideIssuesURL, got)
	}
}

func TestSettingsScrollOffsetCentersFocusedLineWhenNeeded(t *testing.T) {
	if got := settingsScrollOffset(5, 1, 8); got != 0 {
		t.Fatalf("expected no scroll when content fits, got %d", got)
	}
	if got := settingsScrollOffset(20, 12, 6); got != 9 {
		t.Fatalf("expected centered scroll offset 9, got %d", got)
	}
	if got := settingsScrollOffset(20, 19, 6); got != 14 {
		t.Fatalf("expected scroll to clamp at end, got %d", got)
	}
}

func TestSettingsRightPaneScrollsToFocusedFieldOnShortView(t *testing.T) {
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssAI)
	s.providerIdx = 4
	s.ensureSectionFieldVisible(ssAI)
	s.setFocusedField(sfSavePath)

	chrome := newManagerChrome(62, CatppuccinMocha)
	view := ansi.Strip(s.View(62, 12, chrome))

	if !strings.Contains(view, "Save summaries to") {
		t.Fatalf("expected short settings view to scroll save path into view, got %q", view)
	}
	if strings.Contains(view, "Provider") {
		t.Fatalf("expected provider row to scroll out of the short detail pane, got %q", view)
	}
}

func TestSettingsAPIKeySummaryUsesCompactSecretState(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Provider = "openai"
	cfg.AI.OpenAIKey = "sk-abcdefghijklmnopqrstuvwxyz123456"

	s := newSettings(cfg, settingsUpdateState{})
	s.setActiveSection(ssAI)
	s.setFocusedField(sfSavePath)

	view := ansi.Strip(s.View(84, 22, newManagerChrome(84, CatppuccinMocha)))

	if strings.Contains(view, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("expected API key summary to hide the raw key, got %q", view)
	}
	if !strings.Contains(view, "saved") || !strings.Contains(view, "chars") {
		t.Fatalf("expected compact API key summary, got %q", view)
	}
}

func TestSettingsAPIKeyFocusShowsExpandedInlineEditor(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Provider = "openai"

	s := newSettings(cfg, settingsUpdateState{})
	s.setActiveSection(ssAI)
	s.setFocusedField(sfAPIKey)

	view := ansi.Strip(s.View(84, 22, newManagerChrome(84, CatppuccinMocha)))

	if !strings.Contains(view, "masked while typing") {
		t.Fatalf("expected focused API key editor header, got %q", view)
	}
	if !strings.Contains(view, "sk-...") {
		t.Fatalf("expected expanded editor to show placeholder, got %q", view)
	}
}

func TestSettingsAPIKeySummaryDoesNotWrapAcrossLines(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Provider = "openai"
	cfg.AI.OpenAIKey = "sk-abcdefghijklmnopqrstuvwxyz123456"

	s := newSettings(cfg, settingsUpdateState{})
	s.setActiveSection(ssAI)
	s.setFocusedField(sfSavePath)

	body := s.viewSectionBody(48, newManagerChrome(64, CatppuccinMocha))
	stripped := ansi.Strip(strings.Join(body.lines, "\n"))

	if strings.Count(stripped, "saved") != 1 {
		t.Fatalf("expected API key summary to stay on one line, got %q", stripped)
	}
}

func TestSettingsProviderSelectorStaysSingleLineInNarrowPane(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Provider = "openai"

	s := newSettings(cfg, settingsUpdateState{})
	s.setActiveSection(ssAI)

	row := s.renderProviderSelector(42, newManagerChrome(62, CatppuccinMocha))
	if got := lipgloss.Height(row); got != 1 {
		t.Fatalf("expected provider selector to stay on one line, got height %d", got)
	}
	stripped := ansi.Strip(row)
	if !strings.Contains(stripped, "◀") || !strings.Contains(stripped, "▶") {
		t.Fatalf("expected provider selector to render side arrows, got %q", stripped)
	}
	if got := lipgloss.Width(row); got >= 42 {
		t.Fatalf("expected provider selector to fit content instead of filling width 42, got width %d", got)
	}
}

func TestSettingsFocusedAPIKeyHeaderStaysSingleLine(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Provider = "openai"

	s := newSettings(cfg, settingsUpdateState{})
	s.setActiveSection(ssAI)
	s.setFocusedField(sfAPIKey)

	body := s.viewSectionBody(48, newManagerChrome(64, CatppuccinMocha))
	for _, line := range body.lines {
		if strings.Contains(ansi.Strip(line), "OpenAI key") {
			if got := lipgloss.Height(line); got != 1 {
				t.Fatalf("expected API key header to stay on one line, got height %d in %q", got, ansi.Strip(line))
			}
			return
		}
	}
	t.Fatal("expected focused API key header to be present")
}

func TestSettingsSavePathRowStaysSingleLineInNarrowPane(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Provider = "openai"
	cfg.AI.SavePath = "~/summaries/export"

	s := newSettings(cfg, settingsUpdateState{})
	s.setActiveSection(ssAI)
	s.setFocusedField(sfSavePath)

	body := s.viewSectionBody(48, newManagerChrome(64, CatppuccinMocha))
	for _, line := range body.lines {
		if strings.Contains(ansi.Strip(line), "Save summaries to") {
			if got := lipgloss.Height(line); got != 1 {
				t.Fatalf("expected save path row to stay on one line, got height %d in %q", got, ansi.Strip(line))
			}
			return
		}
	}
	t.Fatal("expected save path row to be present")
}

func TestSettingsFeedsSectionShowsFeedMaxSizeFieldOnly(t *testing.T) {
	cfg := config.DefaultConfig()

	s := newSettings(cfg, settingsUpdateState{})
	s.setActiveSection(ssFeeds)

	view := ansi.Strip(s.View(96, 24, newManagerChrome(96, CatppuccinMocha)))

	if !strings.Contains(view, "Feed max size (MiB)") {
		t.Fatalf("expected feeds settings view to contain feed max size, got %q", view)
	}
	for _, unwanted := range []string{"GReader API URL", "Login", "Password", "Source"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("expected feeds settings view to hide %q, got %q", unwanted, view)
		}
	}
}

func TestSettingsApplyToPreservesSourceConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Source.GReaderURL = "https://rss.example.com/api/greader.php"
	cfg.Source.GReaderLogin = "alice"
	cfg.Source.GReaderPassword = "secret"

	s := newSettings(cfg, settingsUpdateState{})
	got := s.ApplyTo(cfg)

	if got.Source != cfg.Source {
		t.Fatalf("expected settings save to preserve hidden source config, got %#v want %#v", got.Source, cfg.Source)
	}
}
