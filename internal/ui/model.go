package ui

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tide/internal/config"
	"tide/internal/db"
	"tide/internal/feed"
)

// ── Enums ────────────────────────────────────────────────────────────────────

type pane int

const (
	paneFeeds pane = iota
	paneArticles
	paneContent
)

type overlayMode int

const (
	overlayNone overlayMode = iota
	overlayQuitConfirm
	overlaySearch
	overlayThemePicker
	overlayFeedManager
	overlayHelp
)

// ── Model ────────────────────────────────────────────────────────────────────

type Model struct {
	db  *db.DB
	cfg config.Config

	width, height int
	focused       pane

	// Feed pane
	feeds      []db.Feed
	feedCursor int

	// Article pane
	articles         []db.Article
	filteredArticles []db.Article
	articleCursor    int
	listOffset       int
	searchQuery      string

	// Content pane
	viewport viewport.Model

	// Overlays / inputs
	overlay     overlayMode
	searchInput textinput.Model

	// Theme
	confirmedTheme int
	activeTheme    int
	styles         Styles
	themeCursor    int

	// Feed manager (delegate)
	feedManager FeedManager

	// Status
	statusMsg string
	statusErr bool

	// Async
	refreshing  map[int64]bool
	spinner     spinner.Model
	mdConverter *md.Converter

	firstLoad           bool  // true until the initial FeedsLoadedMsg is processed
	pendingSelectFeedID int64 // select this feed when FeedsLoadedMsg arrives
	keys                KeyMap
}

var dbgLog *log.Logger

func init() {
	f, _ := os.OpenFile("/tmp/rss-debug.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	dbgLog = log.New(f, "", log.Ltime|log.Lmicroseconds)
}

func NewModel(database *db.DB, cfg config.Config) Model {
	_, themeIdx := ThemeByName(cfg.Theme)

	si := textinput.New()
	si.Placeholder = "search articles..."
	si.CharLimit = 100

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))

	m := Model{
		db:             database,
		cfg:            cfg,
		focused:        paneFeeds,
		confirmedTheme: themeIdx,
		activeTheme:    themeIdx,
		styles:         BuildStyles(BuiltinThemes[themeIdx]),
		feedManager:    NewFeedManager(database),
		searchInput:    si,
		spinner:        sp,
		refreshing:     make(map[int64]bool),
		mdConverter:    md.NewConverter("", true, nil),
		firstLoad:      true,
		keys:           DefaultKeys,
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadFeedsCmd(), m.spinner.Tick)
}

// ── Update ───────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport = viewport.New(m.contentBodyWidth(), m.contentBodyHeight())
		m.viewport.Style = lipgloss.NewStyle()
		if len(m.filteredArticles) > 0 {
			m.viewport.SetContent(m.renderArticleContent(m.filteredArticles[m.articleCursor]))
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case StatusClearMsg:
		m.statusMsg = ""
		m.statusErr = false
		return m, nil

	case FeedsLoadedMsg:
		m.feeds = msg.Feeds
		isFirstLoad := m.firstLoad
		m.firstLoad = false
		dbgLog.Printf("FeedsLoadedMsg: %d feeds, isFirstLoad=%v, overlay=%v", len(m.feeds), isFirstLoad, m.overlay)
		for _, f := range m.feeds {
			dbgLog.Printf("  feed id=%d title=%q", f.ID, f.Title)
		}

		if len(m.feeds) == 0 {
			return m, nil
		}

		// If we just saved a new feed, select it in the list.
		if m.pendingSelectFeedID != 0 {
			for i, f := range m.feeds {
				if f.ID == m.pendingSelectFeedID {
					m.feedCursor = i
					break
				}
			}
			m.pendingSelectFeedID = 0
		}
		m.feedCursor = clamp(m.feedCursor, 0, len(m.feeds)-1)
		cmds := []tea.Cmd{m.loadArticlesCmd(m.feeds[m.feedCursor].ID)}
		// Only auto-refresh on startup — manual refresh uses f/F keys.
		if isFirstLoad {
			for _, f := range m.feeds {
				cmds = append(cmds, m.refreshFeedCmd(f.ID, f.URL))
			}
		}
		return m, tea.Batch(cmds...)

	case ArticlesLoadedMsg:
		if len(m.feeds) > 0 && msg.FeedID == m.feeds[m.feedCursor].ID {
			m.articles = msg.Articles
			m.applyFilter()
			m.articleCursor = clamp(m.articleCursor, 0, max(0, len(m.filteredArticles)-1))
			m.listOffset = 0
			var cmd tea.Cmd
			if len(m.filteredArticles) > 0 {
				m.viewport.SetContent(m.renderArticleContent(m.filteredArticles[m.articleCursor]))
				m.viewport.GotoTop()
				cmd = m.maybeFetchArticleContentCmd(m.filteredArticles[m.articleCursor])
			}
			return m, cmd
		}
		return m, nil

	case FeedRefreshedMsg:
		delete(m.refreshing, msg.FeedID)
		if msg.Err != nil {
			m.setStatus(fmt.Sprintf("refresh failed: %v", msg.Err), true)
			return m, m.clearStatusCmd()
		}
		cmds := []tea.Cmd{}
		for _, a := range msg.Articles {
			if err := m.db.UpsertArticle(a); err != nil {
				continue
			}
		}
		if msg.Title != "" {
			m.db.UpdateFeedMeta(msg.FeedID, msg.Title, "", "", time.Now()) //nolint:errcheck
		} else {
			m.db.TouchFeedFetched(msg.FeedID, time.Now()) //nolint:errcheck
		}
		if len(m.feeds) > 0 && msg.FeedID == m.feeds[m.feedCursor].ID {
			cmds = append(cmds, m.loadArticlesCmd(msg.FeedID))
		}
		cmds = append(cmds, m.loadFeedsCmd())
		return m, tea.Batch(cmds...)

	case FeedSavedMsg:
		if msg.Err != nil {
			m.setStatus(fmt.Sprintf("save failed: %v", msg.Err), true)
			return m, m.clearStatusCmd()
		}
		m.overlay = overlayNone
		m.focused = paneArticles
		m.setStatus(fmt.Sprintf("saved: %s", msg.Feed.Title), false)
		m.feedManager = NewFeedManager(m.db)
		m.pendingSelectFeedID = msg.Feed.ID
		return m, tea.Batch(m.loadFeedsCmd(), m.clearStatusCmd())

	case FeedDeletedMsg:
		if msg.Err != nil {
			m.setStatus(fmt.Sprintf("delete failed: %v", msg.Err), true)
			return m, m.clearStatusCmd()
		}
		m.feedCursor = 0
		m.articleCursor = 0
		m.articles = nil
		m.filteredArticles = nil
		m.feedManager = NewFeedManager(m.db)
		return m, m.loadFeedsCmd()

	case OPMLImportedMsg:
		if msg.Err != nil {
			m.setStatus(fmt.Sprintf("import failed: %v", msg.Err), true)
		} else {
			m.setStatus(fmt.Sprintf("imported %d feeds", msg.Count), false)
		}
		m.feedManager = NewFeedManager(m.db)
		return m, tea.Batch(m.loadFeedsCmd(), m.clearStatusCmd())

	case OPMLExportedMsg:
		if msg.Err != nil {
			m.setStatus(fmt.Sprintf("export failed: %v", msg.Err), true)
		} else {
			m.setStatus(fmt.Sprintf("exported to %s", msg.Path), false)
		}
		return m, m.clearStatusCmd()

	case ArticleContentFetchedMsg:
		if msg.Err != nil {
			return m, nil
		}
		if err := m.db.UpdateArticleContent(msg.ArticleID, msg.Content); err != nil {
			return m, nil
		}
		for i := range m.articles {
			if m.articles[i].ID == msg.ArticleID {
				m.articles[i].Content = msg.Content
			}
		}
		for i := range m.filteredArticles {
			if m.filteredArticles[i].ID == msg.ArticleID {
				m.filteredArticles[i].Content = msg.Content
				if i == m.articleCursor {
					m.viewport.SetContent(m.renderArticleContent(m.filteredArticles[i]))
					m.viewport.GotoTop()
				}
			}
		}
		return m, nil

	case ErrMsg:
		m.setStatus(msg.Err.Error(), true)
		return m, m.clearStatusCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Overlay / window takes priority
	if m.overlay != overlayNone {
		return m.handleOverlayKey(msg)
	}
	return m.handleMainKey(msg)
}

func (m Model) handleMainKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.Quit):
		m.overlay = overlayQuitConfirm
		return m, nil

	case keyMatches(msg, m.keys.Help):
		m.overlay = overlayHelp
		return m, nil

	case keyMatches(msg, m.keys.FeedManager):
		m.overlay = overlayFeedManager
		m.feedManager = NewFeedManager(m.db)
		return m, nil

	case keyMatches(msg, m.keys.Add):
		m.overlay = overlayFeedManager
		m.feedManager = NewFeedManager(m.db)
		return m, nil

	case keyMatches(msg, m.keys.ThemePicker):
		m.overlay = overlayThemePicker
		m.themeCursor = m.confirmedTheme
		return m, nil

	case keyMatches(msg, m.keys.Search):
		m.overlay = overlaySearch
		m.searchInput.Reset()
		m.searchInput.Focus()
		return m, nil

	case keyMatches(msg, m.keys.NextPane):
		m.focused = pane((int(m.focused) + 1) % 3)
		return m, nil

	case keyMatches(msg, m.keys.PrevPane):
		m.focused = pane((int(m.focused) + 2) % 3)
		return m, nil

	case keyMatches(msg, m.keys.Left):
		if m.focused > paneFeeds {
			m.focused--
		}
		return m, nil

	case keyMatches(msg, m.keys.Right):
		if m.focused < paneArticles {
			m.focused++
		}
		return m, nil

	case keyMatches(msg, m.keys.Up):
		return m.handleUp()

	case keyMatches(msg, m.keys.Down):
		return m.handleDown()

	case keyMatches(msg, m.keys.Enter):
		if m.focused == paneArticles && len(m.filteredArticles) > 0 {
			m.focused = paneContent
			return m, m.markReadCmd(m.filteredArticles[m.articleCursor].ID, true)
		}
		return m, nil

	case keyMatches(msg, m.keys.Back):
		if m.focused == paneContent {
			m.focused = paneArticles
		}
		return m, nil

	case keyMatches(msg, m.keys.Refresh):
		if len(m.feeds) > 0 {
			f := m.feeds[m.feedCursor]
			return m, m.refreshFeedCmd(f.ID, f.URL)
		}
		return m, nil

	case keyMatches(msg, m.keys.RefreshAll):
		var cmds []tea.Cmd
		for _, f := range m.feeds {
			cmds = append(cmds, m.refreshFeedCmd(f.ID, f.URL))
		}
		return m, tea.Batch(cmds...)

	case keyMatches(msg, m.keys.MarkRead):
		if len(m.filteredArticles) > 0 {
			a := m.filteredArticles[m.articleCursor]
			return m, m.markReadCmd(a.ID, !a.Read)
		}
		return m, nil

	case keyMatches(msg, m.keys.MarkAllRead):
		if len(m.feeds) > 0 {
			return m, m.markAllReadCmd(m.feeds[m.feedCursor].ID)
		}
		return m, nil

	case keyMatches(msg, m.keys.OpenBrowser):
		if len(m.filteredArticles) > 0 {
			return m, openBrowserCmd(m.filteredArticles[m.articleCursor].Link)
		}
		return m, nil
	}

	// Forward scroll keys to viewport when content is focused
	if m.focused == paneContent {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleUp() (tea.Model, tea.Cmd) {
	switch m.focused {
	case paneFeeds:
		if m.feedCursor > 0 {
			m.feedCursor--
			return m, m.loadArticlesCmd(m.feeds[m.feedCursor].ID)
		}
	case paneArticles:
		if m.articleCursor > 0 {
			m.articleCursor--
			if m.articleCursor < m.listOffset {
				m.listOffset = m.articleCursor
			}
			if len(m.filteredArticles) > 0 {
				m.viewport.SetContent(m.renderArticleContent(m.filteredArticles[m.articleCursor]))
				m.viewport.GotoTop()
				return m, m.maybeFetchArticleContentCmd(m.filteredArticles[m.articleCursor])
			}
		}
	case paneContent:
		m.viewport.ScrollUp(3)
	}
	return m, nil
}

func (m Model) handleDown() (tea.Model, tea.Cmd) {
	switch m.focused {
	case paneFeeds:
		if m.feedCursor < len(m.feeds)-1 {
			m.feedCursor++
			return m, m.loadArticlesCmd(m.feeds[m.feedCursor].ID)
		}
	case paneArticles:
		if m.articleCursor < len(m.filteredArticles)-1 {
			m.articleCursor++
			visible := m.articleRowsVisible()
			if m.articleCursor >= m.listOffset+visible {
				m.listOffset = m.articleCursor - visible + 1
			}
			if len(m.filteredArticles) > 0 {
				m.viewport.SetContent(m.renderArticleContent(m.filteredArticles[m.articleCursor]))
				m.viewport.GotoTop()
				return m, m.maybeFetchArticleContentCmd(m.filteredArticles[m.articleCursor])
			}
		}
	case paneContent:
		m.viewport.ScrollDown(3)
	}
	return m, nil
}

func (m Model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayQuitConfirm:
		switch msg.String() {
		case "y", "enter":
			return m, tea.Quit
		case "n", "esc", "q":
			m.overlay = overlayNone
		}
		return m, nil

	case overlaySearch:
		switch {
		case keyMatches(msg, m.keys.Cancel):
			m.overlay = overlayNone
			m.searchQuery = ""
			m.applyFilter()
			m.articleCursor = 0
			m.listOffset = 0
		case keyMatches(msg, m.keys.Confirm):
			m.overlay = overlayNone
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.searchQuery = m.searchInput.Value()
			m.applyFilter()
			m.articleCursor = 0
			m.listOffset = 0
			return m, cmd
		}
		return m, nil

	case overlayThemePicker:
		prevTheme := m.activeTheme
		switch {
		case keyMatches(msg, m.keys.Up):
			if m.themeCursor > 0 {
				m.themeCursor--
				m.activeTheme = m.themeCursor
				m.styles = BuildStyles(BuiltinThemes[m.activeTheme])
				if len(m.filteredArticles) > 0 {
					m.viewport.SetContent(m.renderArticleContent(m.filteredArticles[m.articleCursor]))
				}
			}
		case keyMatches(msg, m.keys.Down):
			if m.themeCursor < len(BuiltinThemes)-1 {
				m.themeCursor++
				m.activeTheme = m.themeCursor
				m.styles = BuildStyles(BuiltinThemes[m.activeTheme])
				if len(m.filteredArticles) > 0 {
					m.viewport.SetContent(m.renderArticleContent(m.filteredArticles[m.articleCursor]))
				}
			}
		case keyMatches(msg, m.keys.Confirm):
			m.confirmedTheme = m.themeCursor
			m.overlay = overlayNone
			m.cfg.Theme = BuiltinThemes[m.confirmedTheme].Name
			config.Save(m.cfg)
			if len(m.filteredArticles) > 0 {
				m.viewport.SetContent(m.renderArticleContent(m.filteredArticles[m.articleCursor]))
			}
		case keyMatches(msg, m.keys.Cancel):
			m.activeTheme = m.confirmedTheme
			m.styles = BuildStyles(BuiltinThemes[m.activeTheme])
			m.overlay = overlayNone
			if len(m.filteredArticles) > 0 {
				m.viewport.SetContent(m.renderArticleContent(m.filteredArticles[m.articleCursor]))
			}
		}
		if m.activeTheme != prevTheme {
			return m, setTermBgCmd(BuiltinThemes[m.activeTheme].Bg)
		}
		return m, nil

	case overlayFeedManager:
		return m.handleFeedManagerKey(msg)

	case overlayHelp:
		if keyMatches(msg, m.keys.Back, m.keys.Help, m.keys.Quit) {
			m.overlay = overlayNone
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleFeedManagerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	newFM, cmd := m.feedManager.Update(msg, m.keys)
	m.feedManager = newFM

	if m.feedManager.shouldExit {
		m.overlay = overlayNone
		m.feedManager.shouldExit = false
		return m, m.loadFeedsCmd()
	}

	return m, cmd
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	right := lipgloss.JoinVertical(lipgloss.Left,
		m.renderArticlesPane(),
		m.renderContentPane(),
	)
	main := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderFeedsPane(),
		right,
	)
	view := lipgloss.JoinVertical(lipgloss.Left, main, m.renderStatusBar())

	if m.overlay != overlayNone {
		view = m.renderOverlay(view)
	}
	view = clampView(view, m.width, m.height, BuiltinThemes[m.activeTheme].Bg)
	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Background(BuiltinThemes[m.activeTheme].Bg).
		Render(view)
}

// ── Pane renderers ────────────────────────────────────────────────────────────

func (m Model) renderFeedsPane() string {
	w := m.feedsPaneWidth()
	innerW := w - 1 // account for right border
	dbgLog.Printf("renderFeedsPane: %d feeds, overlay=%v, w=%d", len(m.feeds), m.overlay, w)

	focused := m.focused == paneFeeds
	title := m.renderPaneHeader("Feeds", focused, innerW)
	rows := []string{title}

	for i, f := range m.feeds {
		badge := ""
		if f.UnreadCount > 0 {
			badge = m.styles.UnreadBadge.Render(fmt.Sprintf("(%d)", f.UnreadCount))
		}

		refreshing := m.refreshing[f.ID]
		prefix := "  "
		if i == m.feedCursor {
			prefix = "> "
			if refreshing {
				prefix = m.spinner.View() + " "
			}
			rows = append(rows, m.styles.FeedItemSelected.Width(innerW).Render(renderFeedRow(prefix, f.Title, badge, innerW)))
		} else {
			if refreshing {
				prefix = m.spinner.View() + " "
			}
			rows = append(rows, m.styles.FeedItem.Width(innerW).Render(renderFeedRow(prefix, f.Title, badge, innerW)))
		}
	}

	if len(m.feeds) == 0 {
		rows = append(rows, m.styles.FeedItem.Foreground(
			lipgloss.Color(BuiltinThemes[m.activeTheme].Dimmed),
		).Render("  press m to add feeds"))
	}
	footer := m.styles.ArticleRead.Width(innerW).Render(fmt.Sprintf("  %d feeds", len(m.feeds)))
	bodyHeight := max(0, m.mainHeight()-1)
	for len(rows) < bodyHeight {
		rows = append(rows, m.styles.FeedItem.Width(innerW).Render(""))
	}
	rows = append(rows, footer)

	border := m.styles.FeedsPane
	if focused {
		border = border.BorderForeground(BuiltinThemes[m.activeTheme].BorderFocus)
	}

	content := strings.Join(rows, "\n")
	return border.Width(w).Height(m.mainHeight()).Render(content)
}

func (m Model) renderArticlesPane() string {
	w := m.articlesPaneWidth()
	h := m.articlesPaneContentHeight()

	rows := []string{}
	visible := m.filteredArticles
	end := min(m.listOffset+m.articleRowsVisible(), len(visible))
	for i := m.listOffset; i < end; i++ {
		a := visible[i]
		age := relativeTime(a.PublishedAt)

		dot := "  "
		style := m.styles.ArticleRead
		if !a.Read {
			dot = "o "
			style = m.styles.ArticleUnread
		}
		if i == m.articleCursor {
			style = m.styles.ArticleSelected
		}

		rows = append(rows, style.Width(w-2).Render(renderArticleRow(dot, a.Title, age, w-2)))
	}

	if len(m.filteredArticles) == 0 {
		if m.searchQuery != "" {
			rows = append(rows, m.styles.ArticleRead.Render("  no results"))
		} else {
			rows = append(rows, m.styles.ArticleRead.Render("  no articles"))
		}
	}

	focused := m.focused == paneArticles
	border := m.styles.ArticlesPane
	if focused {
		border = border.BorderForeground(BuiltinThemes[m.activeTheme].BorderFocus)
	}
	title := "Articles"
	if m.searchQuery != "" {
		title = fmt.Sprintf("Articles [/%s]", m.searchQuery)
	}

	contentRows := append([]string{m.renderPaneHeader(title, focused, w-2)}, rows...)
	for len(contentRows) < h {
		contentRows = append(contentRows, m.styles.ArticleRead.Width(w-2).Render(""))
	}

	return lipgloss.NewStyle().
		Background(BuiltinThemes[m.activeTheme].Bg).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color(func() string {
			if focused {
				return string(BuiltinThemes[m.activeTheme].BorderFocus)
			}
			return string(BuiltinThemes[m.activeTheme].Border)
		}())).
		Width(w).Height(h).
		Render(strings.Join(contentRows, "\n"))
}

func (m Model) renderContentPane() string {
	w := m.articlesPaneWidth()
	innerH := m.contentViewportHeight()
	bodyH := m.contentBodyHeight()

	focused := m.focused == paneContent
	borderColor := BuiltinThemes[m.activeTheme].Border
	if focused {
		borderColor = BuiltinThemes[m.activeTheme].BorderFocus
	}

	vp := m.viewport
	vp.Width = m.contentBodyWidth()
	vp.Height = bodyH

	inner := m.styles.ContentPane.
		Width(m.contentBodyWidth()).
		Height(innerH).
		Render(m.renderPaneHeader("Content", focused, w-2) + "\n" + vp.View())

	return lipgloss.NewStyle().
		Background(BuiltinThemes[m.activeTheme].Bg).
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(borderColor).
		Width(w).Height(innerH).
		Render(inner)
}

func (m Model) renderPaneHeader(label string, focused bool, width int) string {
	style := m.styles.PaneHeaderInactive
	prefix := "  "
	if focused {
		style = m.styles.PaneHeaderActive
		prefix = "> "
	}
	return style.Width(width).Render(renderFeedRow(prefix, label, "", width))
}

func (m Model) renderArticleContent(a db.Article) string {
	bodyWidth := m.contentBodyWidth()
	title := m.styles.ContentTitle.Width(bodyWidth).Render(truncate(a.Title, bodyWidth))
	meta := m.styles.ContentMeta.Width(bodyWidth).Render(truncate(a.PublishedAt.Format("Mon, 02 Jan 2006 15:04")+"  "+a.Link, bodyWidth))

	content := a.Content
	if content == "" {
		content = "No content available. Press o to open in browser."
	}
	body := m.styles.ContentBody.Width(bodyWidth).Render(formatArticleBody(content, bodyWidth))

	return title + "\n" + meta + "\n\n" + body
}

func (m Model) renderStatusBar() string {
	w := m.width

	if m.statusMsg != "" {
		style := m.styles.StatusBar
		if m.statusErr {
			style = m.styles.StatusError
		}
		return style.Width(w).Render(m.statusLine(m.statusMsg))
	}

	// Build status from current state
	parts := []string{}

	if len(m.feeds) > 0 {
		f := m.feeds[m.feedCursor]
		parts = append(parts, f.Title)
		if f.UnreadCount > 0 {
			parts = append(parts, fmt.Sprintf("%d unread", f.UnreadCount))
		}
		if !f.LastFetched.IsZero() && f.LastFetched.Unix() > 0 {
			parts = append(parts, "updated "+relativeTime(f.LastFetched))
		}
	}

	if len(m.refreshing) > 0 {
		parts = append(parts, m.styles.StatusSpinner.Render(
			m.spinner.View()+" refreshing..."),
		)
	}

	parts = append(parts, m.styles.ArticleRead.Render("? help"))

	return m.styles.StatusBar.Width(w).Render(m.statusLine(strings.Join(parts, "  ·  ")))
}

func (m Model) renderOverlay(base string) string {
	var box string

	switch m.overlay {
	case overlayQuitConfirm:
		content := m.styles.OverlayTitle.Render("Quit rss reader?") + "\n\n" +
			m.styles.OverlayHint.Render("[y / enter] quit   [n / esc] cancel")
		box = m.styles.Overlay.Render(content)

	case overlaySearch:
		content := m.styles.OverlayTitle.Render("Search Articles") + "\n\n" +
			m.searchInput.View() + "\n" +
			m.styles.OverlayHint.Render("[enter] apply   [esc] clear")
		box = m.styles.Overlay.Width(50).Render(content)

	case overlayThemePicker:
		box = m.styles.Overlay.Render(m.renderThemePicker())

	case overlayFeedManager:
		winW := min(m.width-4, 74)
		winH := min(m.height-4, 40)
		fmBg := lipgloss.Color("#0c0e14")
		inner := m.feedManager.View(winW, winH, m.styles)
		inner = clampView(inner, winW, strings.Count(inner, "\n")+1, fmBg)
		box = lipgloss.NewStyle().
			Background(fmBg).
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#7AA2F7")).
			Width(winW).
			Render(inner)

	case overlayHelp:
		winW := min(m.width-6, 90)
		winH := min(m.height-4, 38)
		t := BuiltinThemes[m.activeTheme]
		inner := clampView(renderHelp(winW-2, winH-2, m.styles, m.keys), winW-2, winH-2, t.Bg)
		box = lipgloss.NewStyle().
			Background(t.Bg).
			Border(lipgloss.NormalBorder()).
			BorderForeground(t.BorderFocus).
			Width(winW).Height(winH).
			Render(inner)
	}

	return overlayOnBase(base, box, m.width, m.height, BuiltinThemes[m.activeTheme].Bg)
}

func overlayOnBase(base, box string, width, height int, bg lipgloss.Color) string {
	base = clampView(base, width, height, bg)

	boxLines := strings.Split(box, "\n")
	boxH := len(boxLines)
	boxW := 0
	for _, l := range boxLines {
		if w := lipgloss.Width(l); w > boxW {
			boxW = w
		}
	}

	// Center position — matches lipgloss.Center, lipgloss.Center
	overlayX := (width - boxW) / 2
	overlayY := (height - boxH) / 2
	if overlayX < 0 {
		overlayX = 0
	}
	if overlayY < 0 {
		overlayY = 0
	}
	rightStart := overlayX + boxW

	baseLines := strings.Split(base, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}

	result := make([]string, height)
	for y := 0; y < height; y++ {
		baseLine := baseLines[y]
		boxRow := y - overlayY
		if boxRow < 0 || boxRow >= boxH {
			result[y] = baseLine
			continue
		}
		left := ansi.Cut(baseLine, 0, overlayX)
		right := ansi.Cut(baseLine, rightStart, width)
		result[y] = left + boxLines[boxRow] + right
	}
	return strings.Join(result, "\n")
}

func (m Model) renderThemePicker() string {
	title := m.styles.OverlayTitle.Render("Theme")
	rows := []string{title}
	selected := lipgloss.NewStyle().Foreground(m.styles.FeedItemSelected.GetForeground()).Bold(true)
	normal := lipgloss.NewStyle().Foreground(m.styles.FeedItem.GetForeground())
	for i, t := range BuiltinThemes {
		if i == m.themeCursor {
			rows = append(rows, selected.Render("▶ "+t.Name))
		} else {
			rows = append(rows, normal.Render("  "+t.Name))
		}
	}
	hintStyle := lipgloss.NewStyle().
		Foreground(m.styles.OverlayHint.GetForeground()).
		Background(m.styles.Overlay.GetBackground())
	rows = append(rows, "", hintStyle.Render("[enter] confirm   [esc] revert"))
	return strings.Join(rows, "\n")
}

// ── Commands ─────────────────────────────────────────────────────────────────

func (m *Model) loadFeedsCmd() tea.Cmd {
	db := m.db
	return func() tea.Msg {
		feeds, err := db.ListFeeds()
		dbgLog.Printf("loadFeedsCmd result: %d feeds, err=%v, db=%p", len(feeds), err, db)
		if err != nil {
			return ErrMsg{err}
		}
		return FeedsLoadedMsg{feeds}
	}
}

func (m *Model) loadArticlesCmd(feedID int64) tea.Cmd {
	return func() tea.Msg {
		articles, err := m.db.ListArticles(feedID)
		if err != nil {
			return ErrMsg{err}
		}
		return ArticlesLoadedMsg{FeedID: feedID, Articles: articles}
	}
}

func (m *Model) refreshFeedCmd(feedID int64, feedURL string) tea.Cmd {
	m.refreshing[feedID] = true
	conv := m.mdConverter
	return func() tea.Msg {
		parsed, _, err := feed.FetchAndParse(feedURL)
		if err != nil {
			return FeedRefreshedMsg{FeedID: feedID, Err: err}
		}

		articles := make([]db.Article, 0, len(parsed.Items))
		for _, item := range parsed.Items {
			content, _ := conv.ConvertString(item.Content)
			articles = append(articles, db.Article{
				FeedID:      feedID,
				GUID:        item.GUID,
				Title:       item.Title,
				Link:        item.Link,
				Content:     content,
				PublishedAt: item.PublishedAt,
			})
		}
		return FeedRefreshedMsg{
			FeedID:   feedID,
			Articles: articles,
			Title:    parsed.Title,
		}
	}
}

func (m *Model) markReadCmd(articleID int64, read bool) tea.Cmd {
	return func() tea.Msg {
		if err := m.db.MarkRead(articleID, read); err != nil {
			return ErrMsg{err}
		}
		// Update in-memory
		for i := range m.articles {
			if m.articles[i].ID == articleID {
				m.articles[i].Read = read
			}
		}
		for i := range m.filteredArticles {
			if m.filteredArticles[i].ID == articleID {
				m.filteredArticles[i].Read = read
			}
		}
		return m.loadFeedsCmd()()
	}
}

func (m *Model) maybeFetchArticleContentCmd(a db.Article) tea.Cmd {
	if !shouldFetchArticleContent(a) {
		return nil
	}
	return func() tea.Msg {
		content, err := feed.FetchArticleText(a.Link)
		if err != nil {
			return ArticleContentFetchedMsg{ArticleID: a.ID, Err: err}
		}
		return ArticleContentFetchedMsg{ArticleID: a.ID, Content: content}
	}
}

func (m *Model) markAllReadCmd(feedID int64) tea.Cmd {
	return func() tea.Msg {
		if err := m.db.MarkAllRead(feedID); err != nil {
			return ErrMsg{err}
		}
		return m.loadArticlesCmd(feedID)()
	}
}

func (m *Model) clearStatusCmd() tea.Cmd {
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg {
		return StatusClearMsg{}
	})
}

func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		_ = cmd.Start()
		return nil
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m *Model) applyFilter() {
	q := strings.ToLower(m.searchQuery)
	if q == "" {
		m.filteredArticles = m.articles
		return
	}
	filtered := m.filteredArticles[:0]
	for _, a := range m.articles {
		if strings.Contains(strings.ToLower(a.Title), q) {
			filtered = append(filtered, a)
		}
	}
	m.filteredArticles = filtered
}

func (m *Model) setStatus(msg string, isErr bool) {
	m.statusMsg = msg
	m.statusErr = isErr
}

func shouldFetchArticleContent(a db.Article) bool {
	content := strings.TrimSpace(a.Content)
	if a.Link == "" {
		return false
	}
	if len(content) >= 500 && strings.Count(content, "\n") >= 3 {
		return false
	}
	return true
}

func formatArticleBody(content string, width int) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	paras := splitArticleParagraphs(content)
	out := make([]string, 0, len(paras))
	for _, p := range paras {
		if p == "" {
			continue
		}
		out = append(out, formatArticleParagraph(p, width))
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n\n")
}

func splitArticleParagraphs(content string) []string {
	raw := strings.Split(content, "\n\n")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func formatArticleParagraph(p string, width int) string {
	lines := strings.Split(strings.TrimSpace(p), "\n")
	if len(lines) == 0 {
		return ""
	}

	trimmed := strings.TrimSpace(lines[0])
	switch {
	case strings.HasPrefix(trimmed, "#"):
		return wrapWords(strings.TrimSpace(strings.TrimLeft(trimmed, "#")), width)
	case strings.HasPrefix(trimmed, ">"):
		quote := normalizeInlineSpacing(strings.TrimSpace(strings.TrimLeft(trimmed, ">")))
		return wrapWords("│ "+quote, width)
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
		items := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(strings.TrimLeft(strings.TrimLeft(line, "-"), "*"))
			if line == "" {
				continue
			}
			items = append(items, wrapBullet(line, width))
		}
		return strings.Join(items, "\n")
	default:
		return wrapWords(normalizeInlineSpacing(strings.Join(lines, " ")), width)
	}
}

func wrapBullet(text string, width int) string {
	if width <= 2 {
		return "• " + text
	}
	wrapped := wrapWords(text, width-2)
	lines := strings.Split(wrapped, "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = "• " + lines[i]
		} else {
			lines[i] = "  " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func wrapWords(text string, width int) string {
	text = normalizeInlineSpacing(text)
	if text == "" || width <= 1 {
		return text
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	lines := []string{words[0]}
	for _, word := range words[1:] {
		current := lines[len(lines)-1]
		if lipgloss.Width(current)+1+lipgloss.Width(word) <= width {
			lines[len(lines)-1] = current + " " + word
			continue
		}
		if lipgloss.Width(word) > width {
			if lipgloss.Width(current) < width {
				lines = append(lines, truncate(word, width))
			} else {
				lines = append(lines, truncate(word, width))
			}
			continue
		}
		lines = append(lines, word)
	}
	return strings.Join(lines, "\n")
}

func normalizeInlineSpacing(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func keyMatches(msg tea.KeyMsg, bindings ...key.Binding) bool {
	for _, b := range bindings {
		if key.Matches(msg, b) {
			return true
		}
	}
	return false
}

func truncate(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxW {
		return s
	}
	runes := []rune(s)
	if maxW <= 3 {
		return string(runes[:maxW])
	}
	return string(runes[:maxW-1]) + "…"
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 4*7*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return t.Format("Jan 2")
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m Model) statusLine(s string) string {
	maxW := max(0, m.width-4) // leave room for status bar padding
	s = strings.ReplaceAll(s, "\n", " ")
	return truncate(s, maxW)
}

func renderFeedRow(prefix, title, badge string, width int) string {
	prefixW := lipgloss.Width(prefix)
	badgeW := lipgloss.Width(badge)
	gapW := 0
	if badge != "" {
		gapW = 1
	}
	nameW := max(0, width-prefixW-badgeW-gapW)
	name := truncate(title, nameW)
	row := prefix + padRight(name, nameW)
	if badge != "" {
		row += " " + badge
	}
	return padRight(row, width)
}

func renderArticleRow(prefix, title, age string, width int) string {
	prefixW := lipgloss.Width(prefix)
	ageW := lipgloss.Width(age)
	gapW := 2
	titleW := max(0, width-prefixW-ageW-gapW)
	row := prefix + padRight(truncate(title, titleW), titleW) + strings.Repeat(" ", gapW) + age
	return padRight(row, width)
}

func padRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-lipgloss.Width(s))
}

func clampView(view string, width, height int, bg lipgloss.Color) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	bgStyle := lipgloss.NewStyle().Background(bg)
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		line = ansi.Truncate(line, width, "")
		if !strings.HasSuffix(line, ansi.ResetStyle) {
			line += ansi.ResetStyle
		}
		pad := width - lipgloss.Width(line)
		if pad > 0 {
			line += bgStyle.Render(strings.Repeat(" ", pad))
		}
		lines[i] = line
	}
	for len(lines) < height {
		lines = append(lines, bgStyle.Render(strings.Repeat(" ", width)))
	}
	return strings.Join(lines, "\n")
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

// ── Dimension helpers ─────────────────────────────────────────────────────────

func (m Model) feedsPaneWidth() int    { return int(float64(m.width) * 0.28) }
func (m Model) articlesPaneWidth() int { return m.width - m.feedsPaneWidth() }
func (m Model) mainHeight() int        { return m.height - 1 }
func (m Model) articlesPaneOuterHeight() int {
	return max(3, int(float64(m.mainHeight())*0.40))
}
func (m Model) articlesPaneContentHeight() int {
	return max(2, m.articlesPaneOuterHeight()-1)
}
func (m Model) articleRowsVisible() int {
	return max(0, m.articlesPaneContentHeight()-1)
}
func (m Model) contentPaneOuterHeight() int {
	return max(3, m.mainHeight()-m.articlesPaneOuterHeight())
}
func (m Model) contentViewportHeight() int {
	return max(1, m.contentPaneOuterHeight()-2)
}
func (m Model) contentBodyHeight() int {
	return max(1, m.contentViewportHeight()-1)
}
func (m Model) contentBodyWidth() int {
	return max(1, m.articlesPaneWidth()-2)
}
