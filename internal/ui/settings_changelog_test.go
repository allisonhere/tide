package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tide/internal/config"
)

const sampleChangelog = `# Changelog

Preamble that is not shown.

## Unreleased

## v2.0.0

### Added

- **Bold** feature with ` + "`code`" + ` and a [link](https://example.com) in it.
  A wrapped continuation line.
- Second item

### Fixed

- A bug

## v1.0.0
`

func TestParseChangelogDropsPreambleAndEmptyReleases(t *testing.T) {
	blocks := parseChangelog(sampleChangelog)
	var got []string
	for _, b := range blocks {
		got = append(got, b.text)
	}
	want := []string{
		"v2.0.0", "Added",
		"Bold feature with code and a link in it. A wrapped continuation line.",
		"Second item", "Fixed", "A bug",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected blocks:\n got %q\nwant %q", got, want)
	}
}

func changelogSettings(t *testing.T, source string) Settings {
	t.Helper()
	old := changelogSource
	changelogSource = source
	t.Cleanup(func() { changelogSource = old })
	s := newSettings(config.DefaultConfig(), settingsUpdateState{})
	s.setActiveSection(ssChangelog)
	s.setFocusedPane(settingsPaneDetail)
	return s
}

func longChangelog() string {
	var b strings.Builder
	b.WriteString("## v1.0.0\n\n### Added\n\n")
	for i := 0; i < 40; i++ {
		b.WriteString("- item number " + string(rune('A'+i%26)) + "\n")
	}
	return b.String()
}

func TestChangelogSectionShowsReleaseNotes(t *testing.T) {
	s := changelogSettings(t, sampleChangelog)
	view := ansi.Strip(s.View(84, 24, newManagerChrome(84, CatppuccinMocha, false)))
	for _, want := range []string{"CHANGELOG", "v2.0.0", "Added", "Second item"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in the changelog view:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Preamble") {
		t.Fatalf("expected the preamble hidden:\n%s", view)
	}
}

func TestChangelogSectionWithoutNotes(t *testing.T) {
	s := changelogSettings(t, "")
	view := ansi.Strip(s.View(84, 24, newManagerChrome(84, CatppuccinMocha, false)))
	if !strings.Contains(view, "No changelog is bundled") {
		t.Fatalf("expected a no-changelog message:\n%s", view)
	}
}

func TestChangelogScrollsAndClamps(t *testing.T) {
	s := changelogSettings(t, longChangelog())
	chrome := newManagerChrome(84, CatppuccinMocha, false)
	draw := func() string { return ansi.Strip(s.View(84, 14, chrome)) }
	key := func(k string) {
		s.handleChangelogKey(k)
	}

	draw()
	if !strings.Contains(draw(), "item number A") {
		t.Fatal("expected the top of the changelog first")
	}
	key("up")
	if s.changelogScroll != 0 {
		t.Fatalf("expected no scrolling above the top, got %d", s.changelogScroll)
	}
	key("down")
	key("down")
	if s.changelogScroll != 2 {
		t.Fatalf("expected two lines scrolled, got %d", s.changelogScroll)
	}
	key("pgdown")
	if s.changelogScroll <= 2 {
		t.Fatalf("expected a page down to move further, got %d", s.changelogScroll)
	}

	key("G")
	end := draw()
	if !strings.Contains(end, "item number N") {
		t.Fatalf("expected the last items at the end:\n%s", end)
	}
	last := s.changelogScroll
	key("down")
	if s.changelogScroll != last {
		t.Fatalf("expected scrolling to stop at the end, went from %d to %d", last, s.changelogScroll)
	}
	// Coming back from the end must take one press per line, not a backlog.
	key("up")
	if s.changelogScroll != last-1 {
		t.Fatalf("expected up from the end to move one line, got %d from %d", s.changelogScroll, last)
	}
	key("g")
	if s.changelogScroll != 0 {
		t.Fatalf("expected g to jump to the top, got %d", s.changelogScroll)
	}
	if s.handleChangelogKey("x") {
		t.Fatal("expected unrelated keys to be left for the settings handler")
	}
}

func TestChangelogScrollsFromTheKeyboardAndResetsOnLeave(t *testing.T) {
	s := changelogSettings(t, longChangelog())
	chrome := newManagerChrome(84, CatppuccinMocha, false)
	s.View(84, 14, chrome)

	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, DefaultKeys)
	s = next
	if s.changelogScroll != 1 {
		t.Fatalf("expected j to scroll the changelog, got %d", s.changelogScroll)
	}
	if s.focusedPane != settingsPaneDetail {
		t.Fatal("expected scrolling to keep focus in the detail pane")
	}

	s.setActiveSection(ssAI)
	s.setActiveSection(ssChangelog)
	if s.changelogScroll != 0 {
		t.Fatalf("expected the changelog to reopen at the top, got %d", s.changelogScroll)
	}
}

func TestChangelogSitsBetweenUpdatesAndAI(t *testing.T) {
	if settingsSectionLabels[ssChangelog] != "CHANGELOG" || ssChangelog != ssUpdates+1 || ssAI != ssChangelog+1 {
		t.Fatalf("expected CHANGELOG between UPDATES and AI, got %v", settingsSectionLabels)
	}
}
