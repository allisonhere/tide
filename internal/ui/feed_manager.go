package ui

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tide/internal/db"
	"tide/internal/feed"
	"tide/internal/opml"
)

type fmMode int

const (
	fmList fmMode = iota
	fmEdit
	fmImport
	fmConfirmDelete
)

type FeedManager struct {
	db         *db.DB
	feeds      []db.Feed
	cursor     int
	mode       fmMode
	editTarget int64 // 0 = new feed

	titleInput   textinput.Model
	urlInput     textinput.Model
	importInput  textinput.Model
	focusedField int // 0=title, 1=url

	shouldExit bool
	statusMsg  string
}

func NewFeedManager(database *db.DB) FeedManager {
	title := textinput.New()
	title.Placeholder = "Feed title"
	title.CharLimit = 200

	u := textinput.New()
	u.Placeholder = "https://example.com/feed.xml"
	u.CharLimit = 500

	imp := textinput.New()
	imp.Placeholder = "path to .opml file"
	imp.CharLimit = 500

	fm := FeedManager{
		db:          database,
		titleInput:  title,
		urlInput:    u,
		importInput: imp,
	}
	fm.reload()
	return fm
}

func (fm *FeedManager) reload() {
	feeds, _ := fm.db.ListFeeds()
	fm.feeds = feeds
	fm.cursor = clamp(fm.cursor, 0, max(0, len(feeds)-1))
}

func (fm *FeedManager) focusAdd() {
	fm.mode = fmEdit
	fm.editTarget = 0
	fm.titleInput.Reset()
	fm.urlInput.Reset()
	fm.focusedField = 0
	fm.titleInput.Focus()
	fm.urlInput.Blur()
}

func (fm FeedManager) Update(msg tea.Msg, keys KeyMap) (FeedManager, tea.Cmd) {
	// Route non-key messages to the focused textinput (cursor blink ticks etc.)
	if _, ok := msg.(tea.KeyMsg); !ok {
		switch fm.mode {
		case fmEdit:
			var cmd tea.Cmd
			if fm.focusedField == 0 {
				fm.titleInput, cmd = fm.titleInput.Update(msg)
			} else {
				fm.urlInput, cmd = fm.urlInput.Update(msg)
			}
			return fm, cmd
		case fmImport:
			var cmd tea.Cmd
			fm.importInput, cmd = fm.importInput.Update(msg)
			return fm, cmd
		}
		return fm, nil
	}
	key := msg.(tea.KeyMsg)
	switch fm.mode {
	case fmList:
		return fm.updateList(key, keys)
	case fmEdit:
		return fm.updateEdit(key, keys)
	case fmImport:
		return fm.updateImport(key, keys)
	case fmConfirmDelete:
		return fm.updateConfirmDelete(key, keys)
	}
	return fm, nil
}

func (fm FeedManager) updateList(msg tea.KeyMsg, keys KeyMap) (FeedManager, tea.Cmd) {
	switch {
	case keyMatches(msg, keys.Back):
		fm.shouldExit = true

	case keyMatches(msg, keys.Up):
		if fm.cursor > 0 {
			fm.cursor--
		}

	case keyMatches(msg, keys.Down):
		if fm.cursor < len(fm.feeds)-1 {
			fm.cursor++
		}

	case keyMatches(msg, keys.Add):
		fm.focusAdd()

	case keyMatches(msg, keys.Edit), keyMatches(msg, keys.Enter):
		if len(fm.feeds) > 0 {
			f := fm.feeds[fm.cursor]
			fm.editTarget = f.ID
			fm.titleInput.Reset()
			fm.titleInput.SetValue(f.Title)
			fm.urlInput.Reset()
			fm.urlInput.SetValue(f.URL)
			fm.focusedField = 0
			fm.titleInput.Focus()
			fm.urlInput.Blur()
			fm.mode = fmEdit
		}

	case keyMatches(msg, keys.Delete):
		if len(fm.feeds) > 0 {
			fm.mode = fmConfirmDelete
		}

	case keyMatches(msg, keys.Import):
		fm.importInput.Reset()
		fm.importInput.Focus()
		fm.mode = fmImport

	case keyMatches(msg, keys.Export):
		return fm, fm.exportCmd()
	}
	return fm, nil
}

func (fm FeedManager) updateEdit(msg tea.KeyMsg, keys KeyMap) (FeedManager, tea.Cmd) {
	switch {
	case keyMatches(msg, keys.Cancel):
		fm.mode = fmList
		fm.titleInput.Blur()
		fm.urlInput.Blur()

	case keyMatches(msg, keys.Tab), keyMatches(msg, keys.Up), keyMatches(msg, keys.Down):
		fm.focusedField = 1 - fm.focusedField
		if fm.focusedField == 0 {
			fm.titleInput.Focus()
			fm.urlInput.Blur()
		} else {
			fm.urlInput.Focus()
			fm.titleInput.Blur()
		}

	case keyMatches(msg, keys.Confirm):
		return fm, fm.saveCmd()

	default:
		var cmd tea.Cmd
		if fm.focusedField == 0 {
			fm.titleInput, cmd = fm.titleInput.Update(msg)
		} else {
			fm.urlInput, cmd = fm.urlInput.Update(msg)
		}
		return fm, cmd
	}
	return fm, nil
}

func (fm FeedManager) updateImport(msg tea.KeyMsg, keys KeyMap) (FeedManager, tea.Cmd) {
	switch {
	case keyMatches(msg, keys.Cancel):
		fm.mode = fmList
		fm.importInput.Blur()

	case keyMatches(msg, keys.Confirm):
		path := strings.TrimSpace(fm.importInput.Value())
		fm.mode = fmList
		fm.importInput.Blur()
		return fm, fm.importCmd(path)

	default:
		var cmd tea.Cmd
		fm.importInput, cmd = fm.importInput.Update(msg)
		return fm, cmd
	}
	return fm, nil
}

func (fm FeedManager) updateConfirmDelete(msg tea.KeyMsg, _ KeyMap) (FeedManager, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		fm.mode = fmList
		if len(fm.feeds) > 0 {
			return fm, fm.deleteCmd(fm.feeds[fm.cursor].ID)
		}
	case "n", "esc":
		fm.mode = fmList
	}
	return fm, nil
}

// ── Commands ──────────────────────────────────────────────────────────────────

func (fm *FeedManager) saveCmd() tea.Cmd {
	rawURL := strings.TrimSpace(fm.urlInput.Value())
	title := strings.TrimSpace(fm.titleInput.Value())
	editTarget := fm.editTarget
	database := fm.db

	return func() tea.Msg {
		u, err := url.Parse(rawURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return FeedSavedMsg{Err: fmt.Errorf("invalid URL: %s", rawURL)}
		}

		if editTarget != 0 {
			// Edit existing
			if err := database.UpdateFeed(editTarget, title, rawURL); err != nil {
				return FeedSavedMsg{Err: err}
			}
			f, _ := database.GetFeed(editTarget)
			return FeedSavedMsg{Feed: f}
		}

		// New feed — fetch and parse to get real title (follows auto-discovery)
		parsed, rawURL, err := feed.FetchAndParse(rawURL)
		if err != nil {
			return FeedSavedMsg{Err: err}
		}

		feedTitle := title
		if feedTitle == "" {
			feedTitle = parsed.Title
		}
		if feedTitle == "" {
			feedTitle = rawURL
		}

		id, err := database.AddFeed(rawURL, feedTitle, parsed.Description)
		if err != nil {
			return FeedSavedMsg{Err: err}
		}

		// Upsert articles immediately so the panes aren't empty
		conv := md.NewConverter("", true, nil)
		for _, item := range parsed.Items {
			content, _ := conv.ConvertString(item.Content)
			_ = database.UpsertArticle(db.Article{
				FeedID:      id,
				GUID:        item.GUID,
				Title:       item.Title,
				Link:        item.Link,
				Content:     content,
				PublishedAt: item.PublishedAt,
			})
		}

		f, _ := database.GetFeed(id)
		return FeedSavedMsg{Feed: f}
	}
}

func (fm *FeedManager) deleteCmd(feedID int64) tea.Cmd {
	database := fm.db
	return func() tea.Msg {
		err := database.DeleteFeed(feedID)
		return FeedDeletedMsg{FeedID: feedID, Err: err}
	}
}

func (fm *FeedManager) importCmd(path string) tea.Cmd {
	database := fm.db
	return func() tea.Msg {
		// Expand ~ if present
		if strings.HasPrefix(path, "~/") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, path[2:])
		}

		outlines, err := opml.Import(path)
		if err != nil {
			return OPMLImportedMsg{Err: err}
		}

		count := 0
		for _, o := range outlines {
			feedURL := o.XMLURL
			title := o.Title
			if title == "" {
				title = o.Text
			}
			if _, err := database.AddFeed(feedURL, title, ""); err == nil {
				count++
			}
		}
		return OPMLImportedMsg{Count: count}
	}
}

func (fm *FeedManager) exportCmd() tea.Cmd {
	feeds := fm.feeds
	return func() tea.Msg {
		path, err := opml.ExportPath()
		if err != nil {
			return OPMLExportedMsg{Err: err}
		}

		outlines := make([]opml.Outline, 0, len(feeds))
		for _, f := range feeds {
			outlines = append(outlines, opml.Outline{
				Text:   f.Title,
				Title:  f.Title,
				Type:   "rss",
				XMLURL: f.URL,
			})
		}

		err = opml.Export(outlines, path)
		return OPMLExportedMsg{Path: path, Err: err}
	}
}

// ── View ──────────────────────────────────────────────────────────────────────

func (fm FeedManager) View(width, height int, styles Styles) string {
	contentW := min(width, 74)
	chrome := newManagerChrome(contentW)
	header := renderManagerHeader(contentW, chrome)
	status := ""
	hints := ""

	var body string
	switch fm.mode {
	case fmEdit:
		hints = fm.viewHints(contentW, styles)
	case fmImport:
		hints = fm.viewHints(contentW, styles)
	case fmConfirmDelete:
		hints = fm.viewHints(contentW, styles)
	}

	if fm.statusMsg != "" {
		status = chrome.statusBar.Render(truncate(strings.ToUpper(strings.ReplaceAll(fm.statusMsg, "\n", " ")), max(1, contentW-4)))
	}

	bodyH := max(1, height-lipgloss.Height(header)-lipgloss.Height(status)-lipgloss.Height(hints))
	switch fm.mode {
	case fmEdit:
		body = fm.viewEdit(contentW, bodyH, styles)
	case fmImport:
		body = fm.viewImport(contentW, bodyH, styles)
	case fmConfirmDelete:
		body = fm.viewConfirmDelete(contentW, bodyH, styles)
	default:
		body = fm.viewList(contentW, bodyH, styles)
	}

	parts := []string{header, "", body}
	if status != "" {
		parts = append(parts, status)
	}
	if hints != "" && fm.mode != fmList {
		parts = append(parts, hints)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (fm FeedManager) viewList(width, height int, styles Styles) string {
	chrome := newManagerChrome(width)
	sectionW := max(12, width-1)
	content := []string{}

	if len(fm.feeds) == 0 {
		content = append(content,
			renderManagerSection("YOUR FEEDS", renderManagerPanel(sectionW, "NO FEEDS CONFIGURED", chrome), chrome),
			renderManagerSection("SOURCE", chrome.body.Copy().Render("PRESS A TO ADD A FEED OR I TO IMPORT OPML."), chrome),
		)
	} else {
		listRows := make([]string, 0, len(fm.feeds))
		for i, f := range fm.feeds {
			title := strings.ToUpper(truncate(f.Title, max(8, sectionW-6)))
			if i == fm.cursor {
				listRows = append(listRows, renderManagerSelectedRow(sectionW, title, chrome))
				continue
			}
			listRows = append(listRows, renderManagerRow(sectionW, title, chrome))
		}
		feedList := lipgloss.JoinVertical(lipgloss.Left, append([]string{""}, listRows...)...)
		content = append(content, renderManagerSection("YOUR FEEDS", feedList, chrome))
		content = append(content, "")

		current := fm.feeds[fm.cursor]
		sourceLine := renderManagerSourceLine(sectionW, strings.ToUpper(truncate(current.URL, max(8, sectionW-4))), chrome)
		content = append(content, renderManagerSection("SOURCE", renderManagerPanel(sectionW, sourceLine, chrome), chrome))
	}

	footer := renderManagerActions(width, chrome,
		"a", "add",
		"e", "edit",
		"d", "delete",
		"i", "import",
		"x", "export",
		"esc", "back",
	)
	main := lipgloss.NewStyle().Background(chrome.baseBg).PaddingLeft(2).Render(
		lipgloss.JoinVertical(lipgloss.Left, content...),
	)
	return lipgloss.JoinVertical(lipgloss.Left, main, footer)
}

func renderManagerInset(spaces int, s string) string {
	if spaces <= 0 {
		return s
	}
	return strings.Repeat(" ", spaces) + s
}

func (fm FeedManager) viewEdit(width, height int, styles Styles) string {
	chrome := newManagerChrome(width)
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		renderManagerSection("Title", renderTextInput(fm.titleInput, width-3, fm.focusedField == 0, chrome), chrome),
		"",
		renderManagerSection("URL", renderTextInput(fm.urlInput, width-3, fm.focusedField == 1, chrome), chrome),
		"",
	)
	return lipgloss.NewStyle().Background(chrome.baseBg).PaddingLeft(2).Render(content)
}

func (fm FeedManager) viewImport(width, height int, styles Styles) string {
	chrome := newManagerChrome(width)
	return lipgloss.NewStyle().Background(chrome.baseBg).PaddingLeft(2).Render(
		renderManagerSection("01. IMPORT OPML", renderManagerInput(width-3, fm.importInput.Value(), "PATH TO OPML FILE...", true, chrome), chrome),
	)
}

func (fm FeedManager) viewConfirmDelete(width, height int, styles Styles) string {
	chrome := newManagerChrome(width)
	if len(fm.feeds) == 0 {
		return ""
	}
	name := fm.feeds[fm.cursor].Title
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		renderManagerSection("01. DELETE FEED", renderManagerPanel(width-3, strings.ToUpper(truncate(name, width-7)), chrome), chrome),
		renderManagerSection("WARNING", chrome.body.Render("ALL ARTICLES FROM THIS FEED WILL BE REMOVED."), chrome),
	)
	return lipgloss.NewStyle().Background(chrome.baseBg).PaddingLeft(2).Render(content)
}

func (fm FeedManager) viewHints(width int, styles Styles) string {
	chrome := newManagerChrome(width)
	switch fm.mode {
	case fmEdit:
		return renderManagerActions(width, chrome,
			"tab", "next field",
			"enter", "save feed",
			"esc", "cancel",
		)
	case fmImport:
		return renderManagerActions(width, chrome,
			"enter", "import",
			"esc", "cancel",
		)
	case fmConfirmDelete:
		return renderManagerActions(width, chrome,
			"y", "confirm",
			"esc", "cancel",
		)
	default:
		return ""
	}
}

type managerChrome struct {
	baseBg        lipgloss.Color
	surfaceBg     lipgloss.Color
	accent        lipgloss.Color
	highlight     lipgloss.Color
	border        lipgloss.Color
	text          lipgloss.Color
	muted         lipgloss.Color
	header        lipgloss.Style
	sectionLabel  lipgloss.Style
	body          lipgloss.Style
	panel         lipgloss.Style
	panelSelected lipgloss.Style
	key           lipgloss.Style
	keyLabel      lipgloss.Style
	statusBar     lipgloss.Style
}

func newManagerChrome(width int) managerChrome {
	baseBg := lipgloss.Color("#0c0e14")
	surfaceBg := lipgloss.Color("#111319")
	accent := lipgloss.Color("#7AA2F7")
	highlight := lipgloss.Color("#bb9af7")
	border := lipgloss.Color("#434751")
	text := lipgloss.Color("#c8d3f5")
	muted := lipgloss.Color("#7f8490")

	return managerChrome{
		baseBg:    baseBg,
		surfaceBg: surfaceBg,
		accent:    accent,
		highlight: highlight,
		border:    border,
		text:      text,
		muted:     muted,
		header: lipgloss.NewStyle().
			Width(width).
			Background(accent).
			Foreground(baseBg).
			Bold(true).
			Padding(0, 1),
		sectionLabel: lipgloss.NewStyle().
			Background(baseBg).
			Foreground(muted).
			Bold(true),
		body: lipgloss.NewStyle().
			Background(baseBg).
			Foreground(text),
		panel: lipgloss.NewStyle().
			Width(max(1, width-4)).
			Background(surfaceBg).
			Foreground(text).
			Border(lipgloss.NormalBorder()).
			BorderForeground(border).
			BorderBackground(surfaceBg).
			Padding(0, 1),
		panelSelected: lipgloss.NewStyle().
			Background(highlight).
			Foreground(baseBg).
			Bold(true).
			Padding(0, 1),
		key: lipgloss.NewStyle().
			Background(accent).
			Foreground(baseBg).
			Bold(true).
			Padding(0, 1),
		keyLabel: lipgloss.NewStyle().
			Background(baseBg).
			Foreground(muted),
		statusBar: lipgloss.NewStyle().
			Width(width).
			Background(surfaceBg).
			Foreground(accent).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(border).
			Padding(0, 1),
	}
}

func renderManagerHeader(width int, chrome managerChrome) string {
	titleText := "TIDE"
	controls := "ARCH"
	gap := max(1, width-lipgloss.Width(titleText)-lipgloss.Width(controls)-2)
	return chrome.header.Render(titleText + strings.Repeat(" ", gap) + controls)
}

func renderManagerSection(label, body string, chrome managerChrome) string {
	w := lipgloss.Width(body)
	styledLabel := chrome.sectionLabel.Width(w).Render(label)
	return lipgloss.JoinVertical(lipgloss.Left, styledLabel, body)
}

func renderManagerSectionLabel(label string, chrome managerChrome) string {
	return chrome.sectionLabel.Render(label)
}

func renderManagerPanel(width int, content string, chrome managerChrome) string {
	panelW := max(1, width-4) // total width incl. padding, excl. border
	textW := max(1, panelW-2) // subtract Padding(0,1) on each side
	bgStyle := lipgloss.NewStyle().Background(chrome.surfaceBg)
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		l = ansi.Truncate(l, textW, "")
		if pad := textW - lipgloss.Width(l); pad > 0 {
			l += bgStyle.Render(strings.Repeat(" ", pad))
		}
		// No reset — preserve bg state so panel's right padding/border inherit surfaceBg
		lines[i] = l
	}
	panel := chrome.panel.Width(panelW).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(width).Background(chrome.baseBg).Render(panel)
}

func renderTextInput(input textinput.Model, width int, focused bool, chrome managerChrome) string {
	// Single bg for all states — sub-styles must match the container exactly so
	// ANSI resets emitted between segments don't expose terminal bg.
	// Focus is indicated solely by the accent left border.
	const fieldBg = lipgloss.Color("#1e2235")

	input.Width = max(1, width-6) // border(1) + padding(2) + prompt(2) + cursor(1)
	input.PromptStyle = lipgloss.NewStyle().Background(fieldBg).Foreground(chrome.accent).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Background(fieldBg).Foreground(chrome.text)
	input.PlaceholderStyle = lipgloss.NewStyle().Background(fieldBg).Foreground(chrome.muted)
	input.Cursor.Style = lipgloss.NewStyle().Background(chrome.accent).Foreground(chrome.baseBg)
	input.Cursor.TextStyle = lipgloss.NewStyle().Background(chrome.accent).Foreground(chrome.baseBg)

	inner := lipgloss.NewStyle().Background(fieldBg).Width(width - 1).Padding(0, 1).Render(input.View())

	barColor := lipgloss.Color(fieldBg)
	if focused {
		barColor = chrome.accent
	}
	return lipgloss.NewStyle().
		Background(fieldBg).
		BorderLeft(true).
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(barColor).
		BorderBackground(fieldBg).
		Render(inner)
}

func renderManagerInput(width int, value, placeholder string, focused bool, chrome managerChrome) string {
	textW := max(1, width-1)
	inputBg := lipgloss.Color("#1e2235")
	dimBg := lipgloss.Color("#13161f")
	bg := inputBg
	if !focused {
		bg = dimBg
	}
	cursor := lipgloss.NewStyle().Background(bg).Foreground(chrome.accent).Bold(true)
	text := lipgloss.NewStyle().Background(bg).Foreground(chrome.text)
	ghost := lipgloss.NewStyle().Background(bg).Foreground(chrome.muted)

	value = strings.TrimSpace(value)
	var line string
	if value == "" {
		line = cursor.Render("> ") + ghost.Render(placeholder)
	} else if focused {
		line = cursor.Render("> ") + text.Render(value)
	} else {
		line = text.Render(value)
	}
	return lipgloss.NewStyle().Background(bg).Padding(0, 1).Render(clampView(line, textW, 1, bg))
}

func renderManagerRow(width int, title string, chrome managerChrome) string {
	rowW := max(1, width-4)
	textW := max(1, rowW-2)
	rowBg := lipgloss.Color("#1e2235")
	row := lipgloss.NewStyle().
		Width(rowW).
		Background(rowBg).
		Foreground(chrome.text).
		Padding(0, 1).
		Render(truncate(title, textW))
	return lipgloss.NewStyle().Width(width).Background(chrome.baseBg).Render(row)
}

func renderManagerSelectedRow(width int, title string, chrome managerChrome) string {
	rowW := max(1, width-4)
	textW := max(1, rowW-2)
	rowBg := lipgloss.Color("#1e2235")
	row := lipgloss.NewStyle().
		Width(rowW).
		Background(rowBg).
		Foreground(chrome.highlight).
		Bold(true).
		Padding(0, 1).
		Render(truncate(title, textW))
	return lipgloss.NewStyle().Width(width).Background(chrome.baseBg).Render(row)
}

func renderManagerSourceLine(width int, value string, chrome managerChrome) string {
	return lipgloss.NewStyle().
		Width(width).
		Background(chrome.surfaceBg).
		Foreground(chrome.accent).
		Render(padRight(value, width))
}

func renderManagerActions(width int, chrome managerChrome, pairs ...string) string {
	bar := lipgloss.NewStyle().
		Width(width).
		Background(chrome.baseBg).
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(chrome.border).
		Padding(0, 0)
	parts := make([]string, 0, len(pairs)/2)
	spacer := lipgloss.NewStyle().Background(chrome.baseBg).Render(" ")
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, lipgloss.JoinHorizontal(
			lipgloss.Left,
			chrome.key.Render(strings.ToUpper(pairs[i])),
			spacer,
			chrome.keyLabel.Render(strings.ToUpper(pairs[i+1])),
		))
	}
	if len(parts) == 0 {
		return bar.Render(clampView("", width, 1, chrome.baseBg))
	}
	bg := lipgloss.NewStyle().Background(chrome.baseBg)
	sep := bg.Render("  ")
	left := strings.Join(parts[:max(0, len(parts)-1)], sep)
	right := parts[len(parts)-1]
	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	row := clampView(left+bg.Render(strings.Repeat(" ", gap))+right, width, 1, chrome.baseBg)
	return bar.Render(row)
}
