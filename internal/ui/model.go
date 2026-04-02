package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tide/internal/ai"
	"tide/internal/config"
	"tide/internal/db"
	"tide/internal/feed"
	"tide/internal/greader"
	"tide/internal/update"
)

// ── Enums ────────────────────────────────────────────────────────────────────

type pane int

const (
	paneFeeds pane = iota
	paneArticles
	paneContent
)

type sidebarRowKind int

const (
	rowKindFolder sidebarRowKind = iota
	rowKindFeed
)

type sidebarRow struct {
	kind     sidebarRowKind
	folderID int64
	feedID   int64
}

type overlayMode int

const (
	overlayNone overlayMode = iota
	overlayQuitConfirm
	overlaySearch
	overlayThemePicker
	overlayFeedManager
	overlayHelp
	overlayFetchError // fetch-error details for a single feed
	overlaySettings
	overlayUpdateConfirm
	overlaySummary
)

type updateState int

const (
	updateStateIdle updateState = iota
	updateStateChecking
	updateStateAvailable
	updateStateDownloading
	updateStateInstalling
	updateStateInstalled
	updateStateNeedsElevation
	updateStateError
)

// ── Model ────────────────────────────────────────────────────────────────────

type Model struct {
	db  *db.DB
	cfg config.Config

	currentVersion string
	updater        *update.Updater

	width, height int
	focused       pane

	// Feed pane
	feeds            []db.Feed
	folders          []db.Folder
	sidebarRows      []sidebarRow
	sidebarCursor    int
	collapsedFolders map[int64]bool

	// Article pane
	articles         []db.Article
	filteredArticles []db.Article
	articleCursor    int
	listOffset       int
	searchQuery      string

	// Content pane
	viewport viewport.Model

	// Help overlay
	helpVP viewport.Model

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

	// Fetch error details overlay
	lastFetchError *feed.FetchResult

	// Async
	refreshing  map[int64]bool
	spinner     spinner.Model
	mdConverter *md.Converter

	firstLoad           bool  // true until the initial FeedsLoadedMsg is processed
	pendingSelectFeedID int64 // select this feed when FeedsLoadedMsg arrives
	keys                KeyMap

	// Settings overlay
	settings Settings

	// Optional Google Reader-compatible source
	greaderClient  *greader.Client
	greaderStreams map[int64]string

	// Update flow
	updateState          updateState
	updateInfo           update.ReleaseInfo
	updateInfoFresh      bool
	downloadedUpdate     *update.DownloadedAsset
	updateInstall        update.InstallResult
	updateErr            string
	updateDismissed      bool
	pendingUpdateInstall bool

	// AI summary overlay
	summarizer        ai.Summarizer // nil when not configured
	summaryArticle    db.Article
	summaryGenerating bool
	summaryErr        string
}

func NewModel(database *db.DB, cfg config.Config, currentVersion string) Model {
	_, themeIdx := ThemeByName(cfg.Theme)

	si := textinput.New()
	si.Placeholder = "search articles..."
	si.CharLimit = 100

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))

	summarizer, _ := ai.New(cfg.AI)

	m := Model{
		db:               database,
		cfg:              cfg,
		currentVersion:   currentVersion,
		updater:          update.New(),
		focused:          paneFeeds,
		confirmedTheme:   themeIdx,
		activeTheme:      themeIdx,
		styles:           BuildStyles(BuiltinThemes[themeIdx]),
		feedManager:      NewFeedManager(database),
		searchInput:      si,
		spinner:          sp,
		refreshing:       make(map[int64]bool),
		collapsedFolders: map[int64]bool{},
		mdConverter:      md.NewConverter("", true, nil),
		greaderStreams:   map[int64]string{},
		firstLoad:        true,
		keys:             DefaultKeys,
		summarizer:       summarizer,
	}
	m.resetSourceClient()
	m.restoreCachedUpdateState()
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadFeedsCmd(), m.spinner.Tick}
	if cmd := m.maybeCheckForUpdatesCmd(false); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
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
		if m.overlay == overlayHelp {
			m.resetHelpVP()
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

	case UpdateCheckedMsg:
		m.updateErr = ""
		m.cfg.Updates.LastCheckedUnix = time.Now().Unix()
		if msg.Err != nil {
			m.pendingUpdateInstall = false
			m.updateState = updateStateError
			m.updateErr = msg.Err.Error()
			m.syncSettingsUpdateState()
			config.Save(m.cfg) //nolint:errcheck
			if msg.Manual {
				m.setStatus("update check failed: "+msg.Err.Error(), true)
				return m, m.clearStatusCmd()
			}
			return m, nil
		}

		m.updateInfo = msg.Result.Latest
		m.updateInfoFresh = true
		m.updateDismissed = msg.Result.Latest.Version != "" && msg.Result.Latest.Version == m.cfg.Updates.DismissedVersion
		if msg.Result.Available {
			m.updateState = updateStateAvailable
			m.cfg.Updates.AvailableVersion = msg.Result.Latest.Version
			m.cfg.Updates.AvailableSummary = msg.Result.Latest.Summary
			m.cfg.Updates.AvailablePublished = msg.Result.Latest.PublishedAt.Unix()
			m.syncSettingsUpdateState()
			config.Save(m.cfg) //nolint:errcheck
			if m.pendingUpdateInstall {
				m.pendingUpdateInstall = false
				if !m.updateDismissed {
					m.overlay = overlayUpdateConfirm
					return m, nil
				}
			}
			if !m.updateDismissed && (msg.Manual || update.IsStableVersion(m.currentVersion)) {
				return m, nil
			}
			return m, nil
		}

		m.updateState = updateStateIdle
		m.pendingUpdateInstall = false
		m.updateDismissed = false
		m.cfg.Updates.DismissedVersion = ""
		m.clearCachedAvailableUpdate()
		config.Save(m.cfg) //nolint:errcheck
		m.syncSettingsUpdateState()
		if msg.Manual {
			m.setStatus("Tide is up to date", false)
			return m, m.clearStatusCmd()
		}
		return m, nil

	case UpdateDownloadedMsg:
		if msg.Err != nil {
			m.updateState = updateStateError
			m.updateErr = msg.Err.Error()
			m.syncSettingsUpdateState()
			m.setStatus("update download failed: "+msg.Err.Error(), true)
			return m, m.clearStatusCmd()
		}
		m.downloadedUpdate = &msg.Asset
		m.updateState = updateStateInstalling
		m.syncSettingsUpdateState()
		return m, m.installUpdateCmd(msg.Asset)

	case UpdateInstalledMsg:
		if msg.Err != nil {
			m.updateState = updateStateError
			m.updateErr = msg.Err.Error()
			m.syncSettingsUpdateState()
			m.setStatus("update failed: "+msg.Err.Error(), true)
			return m, m.clearStatusCmd()
		}
		m.updateInstall = msg.Result
		m.downloadedUpdate = nil
		if msg.Result.RequiresManual {
			m.updateState = updateStateNeedsElevation
			m.syncSettingsUpdateState()
			m.setStatus("update downloaded; admin permission required", true)
			return m, m.clearStatusCmd()
		}
		m.updateState = updateStateInstalled
		m.updateDismissed = false
		m.cfg.Updates.DismissedVersion = ""
		m.clearCachedAvailableUpdate()
		config.Save(m.cfg) //nolint:errcheck
		m.syncSettingsUpdateState()
		m.setStatus("Tide updated to "+msg.Result.Version+" · restart when ready", false)
		return m, m.clearStatusCmd()

	case RestartedMsg:
		if msg.Err != nil {
			m.setStatus(msg.Err.Error(), true)
			return m, m.clearStatusCmd()
		}
		return m, tea.Quit

	case FeedsLoadedMsg:
		if msg.Err != nil && len(msg.Feeds) == 0 && len(msg.Folders) == 0 && len(msg.RemoteStreams) == 0 {
			m.greaderStreams = map[int64]string{}
			m.feeds = nil
			m.folders = nil
			m.rebuildSidebar()
			m.clearArticles()
			m.setStatus(msg.Err.Error(), true)
			return m, m.clearStatusCmd()
		}
		prevKind, prevID := m.currentSidebarSelection()
		m.feeds = msg.Feeds
		m.folders = msg.Folders
		m.greaderStreams = msg.RemoteStreams
		if m.greaderStreams == nil {
			m.greaderStreams = map[int64]string{}
		}
		statusCmd := tea.Cmd(nil)
		if msg.Err != nil {
			m.setStatus(msg.Err.Error(), true)
			statusCmd = m.clearStatusCmd()
		}
		m.rebuildSidebar()
		isFirstLoad := m.firstLoad
		m.firstLoad = false
		if m.pendingSelectFeedID != 0 {
			for i, row := range m.sidebarRows {
				if row.kind == rowKindFeed && row.feedID == m.pendingSelectFeedID {
					m.sidebarCursor = i
					break
				}
			}
			m.pendingSelectFeedID = 0
		} else if prevID != 0 {
			for i, row := range m.sidebarRows {
				if row.kind == prevKind {
					if row.kind == rowKindFeed && row.feedID == prevID {
						m.sidebarCursor = i
						break
					}
					if row.kind == rowKindFolder && row.folderID == prevID {
						m.sidebarCursor = i
						break
					}
				}
			}
		}
		m.sidebarCursor = clamp(m.sidebarCursor, 0, max(0, len(m.sidebarRows)-1))
		if m.overlay == overlayFeedManager && m.feedManager.mode == fmList {
			m.feedManager.setData(m.feeds, m.folders)
			if feed := m.selectedFeed(); feed != nil {
				m.feedManager.selectFeed(feed.ID)
			} else if folderID, ok := m.selectedFolderID(); ok && folderID >= 0 {
				m.feedManager.selectFolder(folderID)
			}
		}
		if len(m.feeds) == 0 {
			m.clearArticles()
			return m, nil
		}
		cmds := []tea.Cmd{}
		if selected := m.selectedFeed(); selected != nil {
			cmds = append(cmds, m.loadArticlesCmd(selected.ID))
		} else {
			m.clearArticles()
		}
		// Only auto-refresh on startup — manual refresh uses f/F keys.
		if isFirstLoad {
			for _, f := range m.feeds {
				if m.isRemoteFeed(f.ID) {
					continue
				}
				cmds = append(cmds, m.refreshFeedCmd(f.ID, f.URL, false))
			}
		}
		if statusCmd != nil {
			cmds = append(cmds, statusCmd)
		}
		return m, tea.Batch(cmds...)

	case ArticlesLoadedMsg:
		if msg.Err != nil {
			if selected := m.selectedFeed(); selected != nil && msg.FeedID == selected.ID {
				m.clearArticles()
			}
			m.setStatus(msg.Err.Error(), true)
			return m, m.clearStatusCmd()
		}
		if selected := m.selectedFeed(); selected != nil && msg.FeedID == selected.ID {
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
			r := msg.Result
			if r != nil {
				friendly := r.FriendlyMessage()
				if r.HasDetails() && msg.Manual {
					// Show error details overlay for manually triggered single-feed refresh.
					m.lastFetchError = r
					m.overlay = overlayFetchError
					m.setStatus(fmt.Sprintf("refresh failed: %s", friendly), true)
				} else {
					m.setStatus(fmt.Sprintf("refresh failed: %s", friendly), true)
					return m, m.clearStatusCmd()
				}
			} else {
				m.setStatus(fmt.Sprintf("refresh failed: %v", msg.Err), true)
				return m, m.clearStatusCmd()
			}
			return m, nil
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
		if r := msg.Result; r != nil && r.SuggestURLUpdate {
			if err := m.db.UpdateFeedURL(msg.FeedID, r.SuggestedURL); err != nil {
				m.setStatus(fmt.Sprintf("URL update failed: %v", err), true)
			} else {
				m.setStatus(fmt.Sprintf("feed URL updated to %s", r.SuggestedURL), false)
			}
		}
		if selected := m.selectedFeed(); selected != nil && msg.FeedID == selected.ID {
			cmds = append(cmds, m.loadArticlesCmd(msg.FeedID))
		}
		cmds = append(cmds, m.loadFeedsCmd())
		cmds = append(cmds, m.clearStatusCmd())
		return m, tea.Batch(cmds...)

	case FeedSavedMsg:
		m.feedManager.busy = false
		m.feedManager.busyMsg = ""
		if msg.Err != nil {
			m.feedManager.statusMsg = fmt.Sprintf("SAVE FAILED: %v", msg.Err)
			m.setStatus(fmt.Sprintf("save failed: %v", msg.Err), true)
			return m, m.clearStatusCmd()
		}
		m.feedManager = m.newFeedManager()
		m.feedManager.mode = fmList
		m.feedManager.selectFeed(msg.Feed.ID)
		m.feedManager.statusMsg = fmt.Sprintf("SAVED: %s", strings.ToUpper(msg.Feed.Title))
		m.setStatus(fmt.Sprintf("saved: %s", msg.Feed.Title), false)
		m.pendingSelectFeedID = msg.Feed.ID
		return m, tea.Batch(m.loadFeedsCmd(), m.clearStatusCmd())

	case RemoteFeedAddedMsg:
		m.feedManager.busy = false
		m.feedManager.busyMsg = ""
		if msg.Err != nil {
			m.feedManager.statusMsg = fmt.Sprintf("SAVE FAILED: %v", msg.Err)
			m.setStatus(fmt.Sprintf("save failed: %v", msg.Err), true)
			return m, m.clearStatusCmd()
		}
		m.cfg.Source.GReaderURL = msg.Source.GReaderURL
		m.cfg.Source.GReaderLogin = msg.Source.GReaderLogin
		m.cfg.Source.GReaderPassword = msg.Source.GReaderPassword
		config.Save(m.cfg) //nolint:errcheck
		m.resetSourceClient()
		m.feedManager = m.newFeedManager()
		m.feedManager.mode = fmList
		if msg.StreamID == "" {
			m.feedManager.statusMsg = fmt.Sprintf("CONNECTED GREADER · %d FEEDS", msg.FeedCount)
			m.setStatus(fmt.Sprintf("connected greader: %d feeds", msg.FeedCount), false)
			return m, tea.Batch(m.loadFeedsCmd(), m.clearStatusCmd())
		}
		title := strings.TrimSpace(msg.Title)
		if title == "" {
			title = "Google Reader feed"
		}
		m.feedManager.statusMsg = fmt.Sprintf("ADDED: %s", strings.ToUpper(title))
		m.setStatus(fmt.Sprintf("added: %s", title), false)
		if msg.StreamID != "" {
			m.pendingSelectFeedID = remoteStableID("feed", msg.StreamID)
		}
		return m, tea.Batch(m.loadFeedsCmd(), m.clearStatusCmd())

	case FeedDeletedMsg:
		m.feedManager.busy = false
		m.feedManager.busyMsg = ""
		if msg.Err != nil {
			m.feedManager.statusMsg = fmt.Sprintf("DELETE FAILED: %v", msg.Err)
			m.setStatus(fmt.Sprintf("delete failed: %v", msg.Err), true)
			return m, m.clearStatusCmd()
		}
		m.sidebarCursor = 0
		m.articleCursor = 0
		m.clearArticles()
		m.feedManager = m.newFeedManager()
		m.feedManager.mode = fmList
		m.feedManager.statusMsg = "DELETED FEED"
		return m, m.loadFeedsCmd()

	case FolderSavedMsg:
		m.feedManager.busy = false
		m.feedManager.busyMsg = ""
		if msg.Err != nil {
			m.feedManager.statusMsg = fmt.Sprintf("SAVE FAILED: %v", msg.Err)
			m.setStatus(fmt.Sprintf("save failed: %v", msg.Err), true)
			return m, m.clearStatusCmd()
		}
		m.setStatus(fmt.Sprintf("saved folder: %s", msg.Folder.Name), false)
		m.feedManager = m.newFeedManager()
		m.feedManager.mode = fmList
		m.feedManager.selectFolder(msg.Folder.ID)
		m.feedManager.statusMsg = fmt.Sprintf("SAVED FOLDER: %s", strings.ToUpper(msg.Folder.Name))
		return m, tea.Batch(m.loadFeedsCmd(), m.clearStatusCmd())

	case FolderDeletedMsg:
		m.feedManager.busy = false
		m.feedManager.busyMsg = ""
		if msg.Err != nil {
			m.feedManager.statusMsg = fmt.Sprintf("DELETE FAILED: %v", msg.Err)
			m.setStatus(fmt.Sprintf("delete failed: %v", msg.Err), true)
			return m, m.clearStatusCmd()
		}
		m.feedManager = m.newFeedManager()
		m.feedManager.mode = fmList
		m.feedManager.statusMsg = "DELETED FOLDER"
		return m, tea.Batch(m.loadFeedsCmd(), m.clearStatusCmd())

	case OPMLImportedMsg:
		m.feedManager.busy = false
		m.feedManager.busyMsg = ""
		if msg.Err != nil {
			m.feedManager.statusMsg = fmt.Sprintf("IMPORT FAILED: %v", msg.Err)
			m.setStatus(fmt.Sprintf("import failed: %v", msg.Err), true)
		}
		m.feedManager = m.newFeedManager()
		m.feedManager.mode = fmList
		if msg.Err == nil {
			m.setStatus(fmt.Sprintf("imported %d feeds", msg.Count), false)
			m.feedManager.statusMsg = fmt.Sprintf("IMPORTED %d FEEDS", msg.Count)
		}
		return m, tea.Batch(m.loadFeedsCmd(), m.clearStatusCmd())

	case OPMLExportedMsg:
		if msg.Err != nil {
			m.setStatus(fmt.Sprintf("export failed: %v", msg.Err), true)
		} else {
			m.setStatus(fmt.Sprintf("exported to %s", msg.Path), false)
		}
		return m, m.clearStatusCmd()

	case ArticleReadUpdatedMsg:
		if msg.Err != nil {
			m.setStatus(fmt.Sprintf("mark read failed: %v", msg.Err), true)
			return m, m.clearStatusCmd()
		}

		for i := range m.articles {
			if m.articles[i].ID == msg.ArticleID {
				m.articles[i].Read = msg.Read
				break
			}
		}
		m.applyFilter()

		if len(m.filteredArticles) == 0 {
			m.articleCursor = 0
			m.listOffset = 0
			m.viewport.SetContent("")
			return m, m.loadFeedsCmd()
		}

		if idx := m.indexOfFilteredArticle(msg.ArticleID); msg.Advance && idx >= 0 && idx == m.articleCursor && idx < len(m.filteredArticles)-1 {
			m.articleCursor = idx + 1
			visible := max(1, m.articleRowsVisible())
			if m.articleCursor >= m.listOffset+visible {
				m.listOffset = m.articleCursor - visible + 1
			}
		} else {
			m.articleCursor = clamp(m.articleCursor, 0, max(0, len(m.filteredArticles)-1))
			maxOffset := max(0, len(m.filteredArticles)-1)
			m.listOffset = clamp(m.listOffset, 0, maxOffset)
		}

		current := m.filteredArticles[m.articleCursor]
		m.viewport.SetContent(m.renderArticleContent(current))
		m.viewport.GotoTop()

		cmds := []tea.Cmd{m.loadFeedsCmd()}
		if current.ID != msg.ArticleID || !msg.Read {
			if cmd := m.maybeFetchArticleContentCmd(current); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case ArticleContentFetchedMsg:
		if msg.Err != nil {
			return m, nil
		}
		if !m.articleIsRemote(msg.ArticleID) {
			if err := m.db.UpdateArticleContent(msg.ArticleID, msg.Content); err != nil {
				return m, nil
			}
		}
		for i := range m.articles {
			if m.articles[i].ID == msg.ArticleID {
				m.articles[i].Content = msg.Content
			}
		}
		m.applyFilter()
		for i := range m.filteredArticles {
			if m.filteredArticles[i].ID == msg.ArticleID && i == m.articleCursor {
				m.viewport.SetContent(m.renderArticleContent(m.filteredArticles[i]))
				m.viewport.GotoTop()
			}
		}
		return m, nil

	case AISummaryFetchedMsg:
		m.summaryGenerating = false
		if msg.Err != nil {
			m.summaryErr = msg.Err.Error()
			return m, nil
		}
		if !m.articleIsRemote(msg.ArticleID) {
			_ = m.db.SaveSummary(msg.ArticleID, msg.Summary)
		}
		for i := range m.articles {
			if m.articles[i].ID == msg.ArticleID {
				m.articles[i].Summary = msg.Summary
			}
		}
		m.applyFilter()
		if m.summaryArticle.ID == msg.ArticleID {
			m.summaryArticle.Summary = msg.Summary
		}
		return m, nil

	case SummarySavedMsg:
		if msg.Err != nil {
			m.setStatus(fmt.Sprintf("save failed: %v", msg.Err), true)
		} else {
			m.setStatus("saved → "+msg.Path, false)
		}
		return m, m.clearStatusCmd()

	case ClipboardCopiedMsg:
		if msg.Err != nil {
			m.setStatus("copy failed: "+msg.Err.Error(), true)
		} else {
			m.setStatus("copied to clipboard", false)
		}
		return m, m.clearStatusCmd()

	case ErrMsg:
		m.setStatus(msg.Err.Error(), true)
		return m, m.clearStatusCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)

	default:
		if m.overlay == overlayFeedManager {
			return m.handleFeedManager(msg)
		}
		if m.overlay == overlaySettings {
			return m.handleSettings(msg)
		}
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
		m.resetHelpVP()
		return m, nil

	case keyMatches(msg, m.keys.FeedManager):
		m.overlay = overlayFeedManager
		m.feedManager = m.newFeedManager()
		return m, nil

	case keyMatches(msg, m.keys.Add):
		m.overlay = overlayFeedManager
		m.feedManager = m.newFeedManager()
		m.feedManager.focusAdd()
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
		if m.focused < paneContent {
			m.focused++
		}
		return m, nil

	case keyMatches(msg, m.keys.Up):
		return m.handleUp()

	case keyMatches(msg, m.keys.Down):
		return m.handleDown()

	case keyMatches(msg, m.keys.Enter):
		if m.focused == paneFeeds && m.toggleSelectedFolder() {
			return m, nil
		}
		if m.focused == paneArticles && len(m.filteredArticles) > 0 {
			m.focused = paneContent
			if m.cfg.Display.MarkReadOnOpen && !m.isRemoteFeed(m.filteredArticles[m.articleCursor].FeedID) {
				return m, m.setArticleReadCmd(m.filteredArticles[m.articleCursor].ID, true, false)
			}
		}
		return m, nil

	case keyMatches(msg, m.keys.Back):
		if m.focused == paneContent {
			m.focused = paneArticles
		}
		return m, nil

	case keyMatches(msg, m.keys.Refresh):
		if selected := m.selectedFeed(); selected != nil {
			if m.isRemoteFeed(selected.ID) {
				return m, tea.Batch(m.loadFeedsCmd(), m.loadArticlesCmd(selected.ID))
			}
			return m, m.refreshFeedCmd(selected.ID, selected.URL, true)
		}
		return m, nil

	case keyMatches(msg, m.keys.RefreshAll):
		var cmds []tea.Cmd
		for _, f := range m.feeds {
			if m.isRemoteFeed(f.ID) {
				continue
			}
			cmds = append(cmds, m.refreshFeedCmd(f.ID, f.URL, false))
		}
		if m.greaderClient != nil {
			cmds = append(cmds, m.loadFeedsCmd())
		}
		return m, tea.Batch(cmds...)

	case keyMatches(msg, m.keys.MarkRead):
		if len(m.filteredArticles) > 0 {
			if m.isRemoteFeed(m.filteredArticles[m.articleCursor].FeedID) {
				m.setStatus("Google Reader mode is browse-only for now", false)
				return m, m.clearStatusCmd()
			}
			a := m.filteredArticles[m.articleCursor]
			advance := m.focused == paneArticles
			if !advance && a.Read {
				return m, nil
			}
			return m, m.setArticleReadCmd(a.ID, true, advance)
		}
		return m, nil

	case keyMatches(msg, m.keys.MarkAllRead):
		if feed := m.selectedFeed(); feed != nil {
			if m.isRemoteFeed(feed.ID) {
				m.setStatus("Google Reader mode is browse-only for now", false)
				return m, m.clearStatusCmd()
			}
			return m, m.markAllReadCmd(feed.ID)
		}
		if folderID, ok := m.selectedFolderID(); ok {
			if m.selectedFolderHasRemoteFeeds() {
				m.setStatus("Google Reader mode is browse-only for now", false)
				return m, m.clearStatusCmd()
			}
			return m, m.markFolderReadCmd(folderID)
		}
		return m, nil

	case keyMatches(msg, m.keys.OpenBrowser):
		if len(m.filteredArticles) > 0 {
			return m, m.openBrowserCmd(m.filteredArticles[m.articleCursor].Link)
		}
		return m, nil

	case keyMatches(msg, m.keys.Summary):
		if m.focused != paneFeeds && len(m.filteredArticles) > 0 {
			return m.openSummary()
		}
		return m, nil

	case keyMatches(msg, m.keys.Settings):
		m.settings = newSettings(m.cfg, m.settingsUpdateState())
		m.overlay = overlaySettings
		return m, nil

	case msg.String() == "U":
		if m.showAvailableUpdatePrompt() {
			m.overlay = overlayUpdateConfirm
			return m, nil
		}
		return m, nil

	case msg.String() == "i":
		if m.showAvailableUpdatePrompt() {
			return m, m.dismissAvailableUpdate()
		}
		return m, nil

	case msg.String() == " ":
		if m.focused == paneFeeds && m.toggleSelectedFolder() {
			return m, nil
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
		if m.sidebarCursor > 0 {
			m.sidebarCursor--
			if selected := m.selectedFeed(); selected != nil {
				return m, m.loadArticlesCmd(selected.ID)
			}
			m.clearArticles()
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
		if m.sidebarCursor < len(m.sidebarRows)-1 {
			m.sidebarCursor++
			if selected := m.selectedFeed(); selected != nil {
				return m, m.loadArticlesCmd(selected.ID)
			}
			m.clearArticles()
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
		case "n", "esc":
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
			return m, setTermBgCmd(m.styles.Theme.Bg)
		}
		return m, nil

	case overlayFeedManager:
		return m.handleFeedManager(msg)

	case overlayHelp:
		if keyMatches(msg, m.keys.Back, m.keys.Help, m.keys.Quit) {
			m.overlay = overlayNone
			return m, nil
		}
		var cmd tea.Cmd
		m.helpVP, cmd = m.helpVP.Update(msg)
		return m, cmd

	case overlayFetchError:
		switch msg.String() {
		case "esc", "q", "enter":
			m.overlay = overlayNone
			m.lastFetchError = nil
			return m, m.clearStatusCmd()
		}
		return m, nil

	case overlaySettings:
		return m.handleSettings(msg)

	case overlayUpdateConfirm:
		switch {
		case keyMatches(msg, m.keys.Confirm):
			m.overlay = overlayNone
			m.updateState = updateStateDownloading
			m.syncSettingsUpdateState()
			return m, m.downloadUpdateCmd(m.updateInfo)
		case keyMatches(msg, m.keys.Cancel):
			m.overlay = overlayNone
			return m, nil
		}
		return m, nil

	case overlaySummary:
		return m.handleSummaryKey(msg)
	}

	return m, nil
}

func (m Model) handleSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	newS, cmd, done := m.settings.Update(msg, m.keys)
	m.settings = newS
	action := m.settings.takeAction()
	switch action {
	case settingsActionCheckUpdates:
		m.pendingUpdateInstall = false
		m.updateState = updateStateChecking
		m.updateErr = ""
		m.syncSettingsUpdateState()
		return m, m.checkForUpdatesCmd(true)
	case settingsActionInstallUpdate:
		m.pendingUpdateInstall = true
		m.updateState = updateStateChecking
		m.updateErr = ""
		m.syncSettingsUpdateState()
		return m, m.checkForUpdatesCmd(true)
	case settingsActionDismissVersion:
		return m, m.dismissAvailableUpdate()
	case settingsActionRestartAfterUpdate:
		if m.updateInstall.Restartable {
			return m, restartProcessCmd(m.updateInstall.ExecutablePath)
		}
		return m, nil
	case settingsActionOpenRepo, settingsActionOpenIssues:
		if url := settingsActionURL(action); url != "" {
			return m, m.openBrowserCmd(url)
		}
		return m, nil
	}
	if done {
		if m.settings.shouldSave {
			m.cfg = m.settings.ApplyTo(m.cfg)
			feed.SetMaxFeedBodyBytes(m.cfg.Feed.MaxBodyMiB << 20)
			config.Save(m.cfg)
			m.summarizer, _ = ai.New(m.cfg.AI)
			m.resetSourceClient()
			m.overlay = overlayNone
			m.sidebarCursor = 0
			m.articleCursor = 0
			m.clearArticles()
			return m, m.loadFeedsCmd()
		}
		m.overlay = overlayNone
		return m, nil
	}
	return m, cmd
}

func (m Model) handleSummaryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.Back), keyMatches(msg, m.keys.Summary):
		m.overlay = overlayNone
		return m, nil
	case keyMatches(msg, m.keys.CopyText):
		if !m.summaryGenerating && m.summaryErr == "" && m.summaryArticle.Summary != "" {
			return m, copyToClipboardCmd(m.summaryArticle.Summary)
		}
	case keyMatches(msg, m.keys.SaveMD):
		if !m.summaryGenerating && m.summaryErr == "" && m.summaryArticle.Summary != "" {
			return m, saveSummaryMDCmd(m.summaryArticle, m.summaryArticle.Summary, m.cfg.AI.SavePath)
		}
	}
	return m, nil
}

func (m Model) handleFeedManager(msg tea.Msg) (tea.Model, tea.Cmd) {
	newFM, cmd, exit := m.feedManager.Update(msg, m.keys)
	m.feedManager = newFM
	if exit {
		browseFeedID := m.feedManager.browseFeedID
		editable := m.feedManager.editable()
		m.overlay = overlayNone
		if browseFeedID != 0 && m.selectSidebarFeed(browseFeedID) {
			m.clearArticles()
			return m, m.loadArticlesCmd(browseFeedID)
		}
		if editable {
			return m, m.loadFeedsCmd()
		}
		return m, nil
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
	view = clampView(view, m.width, m.height, m.styles.Theme.Bg)
	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Background(m.styles.Theme.Bg).
		Render(view)
}

// ── Pane renderers ────────────────────────────────────────────────────────────

func (m Model) renderFeedsPane() string {
	w := m.feedsPaneWidth()
	innerW := w - 1 // account for right border
	focused := m.focused == paneFeeds
	title := m.renderPaneHeader("Feeds", focused, innerW)
	rows := []string{title}

	for i, row := range m.sidebarRows {
		selected := i == m.sidebarCursor
		switch row.kind {
		case rowKindFolder:
			rows = append(rows, m.renderFolderHeader(row.folderID, selected, innerW))
		case rowKindFeed:
			if feed := m.feedByID(row.feedID); feed != nil {
				rows = append(rows, m.renderSidebarFeedRow(*feed, selected, innerW))
			}
		}
	}

	if len(m.sidebarRows) == 0 {
		rows = append(rows, m.styles.FeedItem.Foreground(
			lipgloss.Color(m.styles.Theme.Dimmed),
		).Render(m.emptyFeedsHint()))
	}
	footer := fmt.Sprintf("  %d feeds", len(m.feeds))
	if len(m.folders) > 0 {
		footer = fmt.Sprintf("  %d folders · %d feeds", len(m.folders), len(m.feeds))
	}
	footer = m.styles.ArticleRead.Width(innerW).Render(footer)
	bodyHeight := max(0, m.mainHeight()-1)
	for len(rows) < bodyHeight {
		rows = append(rows, m.styles.FeedItem.Width(innerW).Render(""))
	}
	rows = append(rows, footer)

	border := m.styles.FeedsPane
	if focused {
		border = border.BorderForeground(m.styles.Theme.BorderFocus)
	}

	content := strings.Join(rows, "\n")
	return border.Width(innerW).Height(m.mainHeight()).Render(content)
}

func (m Model) renderArticlesPane() string {
	w := m.articlesPaneWidth()
	h := m.articlesPaneContentHeight()
	articleUnread, articleRead, articleSelected, headerActive, borderColor, borderFocus := m.articleRowStyles()

	rows := []string{}
	visible := m.filteredArticles
	end := min(m.listOffset+m.articleRowsVisible(), len(visible))
	for i := m.listOffset; i < end; i++ {
		a := visible[i]
		age := relativeTime(a.PublishedAt)

		dot := m.articleRowPrefix(a.Read)
		style := articleRead
		if !a.Read {
			style = articleUnread
		}
		if i == m.articleCursor {
			style = articleSelected
		}

		rows = append(rows, style.Width(w-2).Render(renderArticleRow(dot, a.Title, age, w-2)))
	}

	if len(m.filteredArticles) == 0 {
		if m.searchQuery != "" {
			rows = append(rows, articleRead.Render("  no results"))
		} else {
			rows = append(rows, articleRead.Render("  no articles"))
		}
	}

	focused := m.focused == paneArticles
	border := m.styles.ArticlesPane
	if focused {
		border = border.BorderForeground(borderFocus)
	}
	title := "Articles"
	if m.searchQuery != "" {
		title = fmt.Sprintf("Articles [/%s]", m.searchQuery)
	}

	contentRows := append([]string{m.renderPaneHeaderWithAccent(title, focused, w, headerActive)}, rows...)
	for len(contentRows) < h {
		contentRows = append(contentRows, articleRead.Width(w-2).Render(""))
	}

	bg := m.styles.Theme.Bg
	return lipgloss.NewStyle().
		Background(bg).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color(func() string {
			if focused {
				return string(borderFocus)
			}
			return string(borderColor)
		}())).
		BorderBackground(bg).
		Width(w).Height(h).
		Render(strings.Join(contentRows, "\n"))
}

func (m Model) renderContentPane() string {
	w := m.articlesPaneWidth()
	innerH := m.contentViewportHeight()
	bodyH := m.contentBodyHeight()
	bg := m.styles.Theme.Bg

	focused := m.focused == paneContent

	vp := m.viewport
	vp.Width = w
	vp.Height = bodyH
	vp.Style = lipgloss.NewStyle().Background(bg)
	body := clampView(vp.View(), w, bodyH, bg)

	inner := m.styles.ContentPane.
		Width(w).
		Height(innerH).
		Render(m.renderPaneHeader("Content", focused, w) + "\n" + body)

	return lipgloss.NewStyle().
		Background(bg).
		Width(w).Height(innerH).
		Render(inner)
}

func (m Model) renderPaneHeader(label string, focused bool, width int) string {
	return m.renderPaneHeaderWithAccent(label, focused, width, m.styles.PaneHeaderActive)
}

func (m Model) renderPaneHeaderWithAccent(label string, focused bool, width int, activeStyle lipgloss.Style) string {
	style := m.styles.PaneHeaderInactive
	prefix := "    "
	title := m.headerLabel(label)
	if focused {
		style = activeStyle
		prefix = "> "
	}
	return style.Width(width).Render(renderFeedRow(prefix, title, "", width))
}

func (m Model) renderArticleContent(a db.Article) string {
	contentWidth := max(1, m.articlesPaneWidth()-1)
	bodyWidth := m.contentBodyWidth()
	title := " " + m.styles.ContentTitle.Width(contentWidth).Render(truncate(a.Title, contentWidth))
	meta := " " + m.styles.ContentMeta.Width(contentWidth).Render(truncate(a.PublishedAt.Format("Mon, 02 Jan 2006 15:04")+"  "+a.Link, contentWidth))

	content := a.Content
	if content == "" {
		content = "No content available. Press o to open in browser."
	}
	body := indentBlock(m.styles.ContentBody.Width(bodyWidth).Render(formatArticleBody(content, bodyWidth)), 1)

	return fillViewWidth(title+"\n"+meta+"\n\n"+body, m.articlesPaneWidth(), m.styles.Theme.Bg)
}

func (m Model) renderStatusBar() string {
	w := m.width
	updateInfoPart := m.statusUpdateInfoPart()
	updateActionPart := m.statusUpdateActionPart()

	if m.statusMsg != "" {
		style := m.styles.StatusBar
		if m.statusErr {
			style = m.styles.StatusError
		}
		parts := []string{m.statusMsg}
		if updateInfoPart != "" && !m.statusMsgCoversUpdateState() {
			parts = append(parts, updateInfoPart)
		}
		return style.Width(w).Render(m.statusLine(strings.Join(parts, "  ·  "), updateActionPart))
	}

	// Build status from current state
	parts := []string{}

	if updateInfoPart != "" {
		parts = append(parts, updateInfoPart)
	}

	if len(m.feeds) > 0 {
		f := m.selectedFeed()
		if f != nil {
			parts = append(parts, f.Title)
			if f.UnreadCount > 0 {
				parts = append(parts, fmt.Sprintf("%d unread", f.UnreadCount))
			}
			if !f.LastFetched.IsZero() && f.LastFetched.Unix() > 0 {
				parts = append(parts, "updated "+relativeTime(f.LastFetched))
			}
		} else if folderID, ok := m.selectedFolderID(); ok {
			parts = append(parts, m.folderName(folderID))
			if unread := m.folderUnreadCount(folderID); unread > 0 {
				parts = append(parts, fmt.Sprintf("%d unread", unread))
			}
		}
	}

	if len(m.refreshing) > 0 {
		parts = append(parts, m.styles.StatusSpinner.Render(
			m.spinner.View()+" refreshing..."),
		)
	}

	parts = append(parts, m.styles.StatusHint.Render("? help"))

	return m.styles.StatusBar.Width(w).Render(m.statusLine(strings.Join(parts, "  ·  "), updateActionPart))
}

func (m Model) statusUpdateInfoPart() string {
	switch m.updateState {
	case updateStateChecking:
		return m.styles.StatusSpinner.Render(m.spinner.View() + " checking Tide updates...")
	case updateStateAvailable:
		return ""
	case updateStateInstalled:
		if m.updateInstall.Version != "" {
			return "restart to use Tide " + m.updateInstall.Version
		}
	}
	return ""
}

func (m Model) statusUpdateActionPart() string {
	if !m.showAvailableUpdatePrompt() {
		return ""
	}
	return m.styles.StatusNotice.Render("App update available  U install  i ignore")
}

func (m Model) statusMsgCoversUpdateState() bool {
	msg := strings.ToLower(m.statusMsg)
	switch m.updateState {
	case updateStateChecking:
		return strings.Contains(msg, "checking update")
	case updateStateAvailable:
		return strings.Contains(msg, "app update")
	case updateStateInstalled:
		return strings.Contains(msg, "restart") || strings.Contains(msg, "tide updated to")
	default:
		return false
	}
}

func (m Model) showAvailableUpdatePrompt() bool {
	return m.updateState == updateStateAvailable && m.updateInfo.Version != "" && !m.updateDismissed
}

func (m Model) renderOverlay(base string) string {
	var box string

	switch m.overlay {
	case overlayQuitConfirm:
		quitW := 40
		qt := m.styles.Theme
		chrome := newManagerChrome(quitW, qt)
		header := renderManagerHeader("QUIT TIDE?", quitW, chrome)
		body := lipgloss.NewStyle().
			Background(chrome.baseBg).
			Foreground(chrome.text).
			Width(quitW).
			Padding(1, 2).
			Render("QUIT TIDE?")
		actions := renderManagerActions(quitW, chrome,
			"y", "quit",
			"esc", "cancel",
		)
		inner := lipgloss.JoinVertical(lipgloss.Left, header, body, actions)
		inner = clampView(inner, quitW, strings.Count(inner, "\n")+1, chrome.baseBg)
		box = renderChromeOverlayBox(inner, quitW, chrome, chrome.accent)

	case overlaySearch:
		box = m.renderSearchOverlay()

	case overlayThemePicker:
		winW := min(m.width-4, 40)
		chrome := newManagerChrome(winW, m.styles.Theme)
		inner := m.renderThemePicker(winW, chrome)
		inner = clampView(inner, winW, strings.Count(inner, "\n")+1, chrome.baseBg)
		box = renderChromeOverlayBox(inner, winW, chrome, chrome.accent)

	case overlayFeedManager:
		winW := min(m.width-4, 74)
		winH := min(m.height-4, 40)
		chrome := newManagerChrome(winW, m.styles.Theme)
		m.feedManager.collapsedFolders = m.collapsedFolders
		inner := m.feedManager.View(winW, winH, m.styles, m.iconsEnabled())
		inner = clampView(inner, winW, strings.Count(inner, "\n")+1, chrome.baseBg)
		box = renderChromeOverlayBox(inner, winW, chrome, chrome.accent)

	case overlayHelp:
		winW := min(m.width-6, 90)
		winH := min(m.height-4, 38)
		t := m.styles.Theme
		surface := modalSurface(t)
		border := t.OverlayBorder
		if border == "" {
			border = t.BorderFocus
		}
		m.helpVP.Style = lipgloss.NewStyle().Background(surface)
		box = lipgloss.NewStyle().
			Background(surface).
			Border(lipgloss.NormalBorder()).
			BorderForeground(border).
			Width(winW).Height(winH).
			Render(m.helpVP.View())

	case overlayFetchError:
		if m.lastFetchError != nil {
			winW := min(m.width-4, 70)
			et := m.styles.Theme
			chrome := newManagerChrome(winW, et)
			inner := m.renderFetchErrorOverlay(winW, chrome)
			inner = clampView(inner, winW, strings.Count(inner, "\n")+1, chrome.baseBg)
			box = renderChromeOverlayBox(inner, winW, chrome, chrome.accent)
		}

	case overlaySettings:
		winW := min(m.width-4, 62)
		winH := min(m.height-4, 36)
		chrome := newManagerChrome(winW, m.styles.Theme)
		inner := m.settings.View(winW, winH, chrome)
		inner = clampView(inner, winW, strings.Count(inner, "\n")+1, chrome.baseBg)
		box = renderChromeOverlayBox(inner, winW, chrome, chrome.accent)

	case overlayUpdateConfirm:
		winW := min(m.width-8, 72)
		chrome := newManagerChrome(winW, m.styles.Theme)
		inner := m.renderUpdateConfirmOverlay(winW, chrome)
		inner = clampView(inner, winW, strings.Count(inner, "\n")+1, chrome.baseBg)
		box = renderChromeOverlayBox(inner, winW, chrome, chrome.accent)

	case overlaySummary:
		winW := min(m.width-8, 76)
		winH := min(m.height-6, 20)
		chrome := newManagerChrome(winW, m.styles.Theme)
		inner := m.renderSummaryOverlay(winW, winH, chrome)
		inner = clampView(inner, winW, strings.Count(inner, "\n")+1, chrome.baseBg)
		box = renderChromeOverlayBox(inner, winW, chrome, chrome.accent)
	}

	return overlayOnBase(base, box, m.width, m.height, m.styles.Theme.Bg)
}

func renderChromeOverlayBox(inner string, width int, chrome managerChrome, border lipgloss.Color) string {
	return lipgloss.NewStyle().
		Background(chrome.baseBg).
		Border(lipgloss.NormalBorder()).
		BorderForeground(border).
		BorderBackground(chrome.baseBg).
		Width(width).
		Render(inner)
}

func renderStyledOverlayBox(inner string, width int, styles Styles) string {
	return styles.Overlay.Width(width).Render(inner)
}

func (m Model) renderSearchOverlay() string {
	surface := modalSurface(m.styles.Theme)
	accent := m.styles.Theme.BorderFocus
	if accent == "" {
		accent = m.styles.Theme.OverlayBorder
	}
	text := readableText(m.styles.Theme.Fg, surface, 4.5)
	muted := mutedText(text, surface)

	input := m.searchInput
	input.Width = 42
	input.PromptStyle = lipgloss.NewStyle().Background(surface).Foreground(accent).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Background(surface).Foreground(text)
	input.PlaceholderStyle = lipgloss.NewStyle().Background(surface).Foreground(muted)
	input.Cursor.Style = lipgloss.NewStyle().Background(accent).Foreground(contrastFg(accent))
	input.Cursor.TextStyle = lipgloss.NewStyle().Background(accent).Foreground(contrastFg(accent))

	content := m.styles.OverlayTitle.Render("Search Articles") + "\n\n" +
		input.View() + "\n" +
		m.styles.OverlayHint.Render("[enter] apply   [esc] clear")
	return renderStyledOverlayBox(content, 50, m.styles)
}

func (m Model) renderSummaryOverlay(width, height int, chrome managerChrome) string {
	header := renderManagerHeader("AI SUMMARY", width, chrome)

	var bodyText string
	switch {
	case m.summaryGenerating:
		bodyText = m.spinner.View() + " Generating summary…"
	case m.summaryErr != "":
		bodyText = "Error: " + m.summaryErr
	default:
		bodyText = formatSummaryBody(m.summaryArticle.Summary, width-4)
	}

	body := lipgloss.NewStyle().
		Background(chrome.baseBg).
		Foreground(chrome.text).
		Width(width).
		Padding(1, 2).
		Render(bodyText)

	var hints string
	if !m.summaryGenerating && m.summaryErr == "" {
		provider := ""
		if m.summarizer != nil {
			provider = "  ·  " + m.summarizer.ProviderName()
		}
		providerLine := lipgloss.NewStyle().
			Background(chrome.baseBg).
			Foreground(chrome.muted).
			Width(width).
			Padding(0, 2).
			Render(provider)
		hints = lipgloss.JoinVertical(lipgloss.Left,
			providerLine,
			renderManagerActions(width, chrome, "c", "copy", "m", "save .md", "esc", "close"),
		)
	} else {
		hints = renderManagerActions(width, chrome, "esc", "close")
	}

	bodyH := max(1, height-lipgloss.Height(header)-lipgloss.Height(hints))
	body = clampView(body, width, bodyH, chrome.baseBg)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, hints)
}

func (m Model) renderUpdateConfirmOverlay(width int, chrome managerChrome) string {
	header := renderManagerHeader("INSTALL TIDE UPDATE?", width, chrome)

	target, _ := os.Executable()
	bodyLines := []string{
		"Install Tide " + m.updateInfo.Version + "?",
	}
	if summary := strings.TrimSpace(m.updateInfo.Summary); summary != "" {
		bodyLines = append(bodyLines, "", "What's new: "+summary)
	}
	bodyLines = append(bodyLines,
		"",
		"Asset: "+m.updateInfo.AssetName+".tar.gz",
		"Target: "+target,
		"",
		"The update will download first, then replace the current binary if the install path is writable.",
	)
	bodyText := strings.Join(bodyLines, "\n")

	body := lipgloss.NewStyle().
		Background(chrome.baseBg).
		Foreground(chrome.text).
		Width(width).
		Padding(1, 2, 0, 2).
		Render(bodyText)

	note := lipgloss.NewStyle().
		Background(chrome.baseBg).
		Foreground(chrome.muted).
		Width(width).
		Padding(0, 2, 1, 2).
		Render("Also available in Settings > Updates")

	actions := renderManagerActions(width, chrome, "enter", "install", "esc", "cancel")
	return lipgloss.JoinVertical(lipgloss.Left, header, body, note, actions)
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

func (m Model) renderThemePicker(width int, chrome managerChrome) string {
	header := renderManagerHeader("THEME", width, chrome)
	rows := make([]string, 0, len(BuiltinThemes))
	for i, t := range BuiltinThemes {
		if i == m.themeCursor {
			rows = append(rows, renderManagerSelectedRow(width, "▶ "+t.Name, chrome, m.styles))
		} else {
			rows = append(rows, clampView(
				lipgloss.NewStyle().
					Background(chrome.baseBg).
					Foreground(chrome.text).
					Padding(0, 1).
					Render("  "+t.Name),
				width,
				1,
				chrome.baseBg,
			))
		}
	}
	body := clampView(lipgloss.JoinVertical(lipgloss.Left, rows...), width, len(rows), chrome.baseBg)
	hints := renderManagerActions(width, chrome, "enter", "confirm", "esc", "revert")
	return lipgloss.JoinVertical(lipgloss.Left, header, body, hints)
}

// ── Commands ─────────────────────────────────────────────────────────────────

func (m *Model) loadFeedsCmd() tea.Cmd {
	db := m.db
	client := m.greaderClient
	return func() tea.Msg {
		feeds, err := db.ListFeeds()
		if err != nil {
			return FeedsLoadedMsg{Err: err}
		}
		folders, err := db.ListFolders()
		if err != nil {
			return FeedsLoadedMsg{Err: err}
		}
		streams := map[int64]string{}
		if client != nil {
			remoteFeeds, remoteStreams, remoteErr := m.loadGReaderFeeds(context.Background())
			feeds = append(feeds, remoteFeeds...)
			if remoteStreams != nil {
				streams = remoteStreams
			}
			return FeedsLoadedMsg{Feeds: feeds, Folders: folders, RemoteStreams: streams, Err: remoteErr}
		}
		return FeedsLoadedMsg{Feeds: feeds, Folders: folders, RemoteStreams: streams}
	}
}

func (m *Model) loadArticlesCmd(feedID int64) tea.Cmd {
	if m.isRemoteFeed(feedID) {
		return func() tea.Msg {
			articles, err := m.loadGReaderArticles(context.Background(), feedID)
			return ArticlesLoadedMsg{FeedID: feedID, Articles: articles, Err: err}
		}
	}
	return func() tea.Msg {
		articles, err := m.db.ListArticles(feedID)
		if err != nil {
			return ArticlesLoadedMsg{FeedID: feedID, Err: err}
		}
		return ArticlesLoadedMsg{FeedID: feedID, Articles: articles}
	}
}

func (m *Model) maybeCheckForUpdatesCmd(manual bool) tea.Cmd {
	if manual {
		return m.checkForUpdatesCmd(true)
	}
	if !m.cfg.Updates.CheckOnStartup {
		return nil
	}
	lastChecked := time.Unix(m.cfg.Updates.LastCheckedUnix, 0)
	if m.cfg.Updates.LastCheckedUnix > 0 {
		interval := time.Duration(m.cfg.Updates.CheckIntervalHours) * time.Hour
		if interval > 0 && time.Since(lastChecked) < interval {
			return nil
		}
	}
	return m.checkForUpdatesCmd(false)
}

func (m *Model) checkForUpdatesCmd(manual bool) tea.Cmd {
	m.updateState = updateStateChecking
	updater := m.updater
	currentVersion := m.currentVersion
	return func() tea.Msg {
		result, err := updater.Check(currentVersion)
		return UpdateCheckedMsg{Result: result, Manual: manual, Err: err}
	}
}

func (m *Model) downloadUpdateCmd(info update.ReleaseInfo) tea.Cmd {
	updater := m.updater
	return func() tea.Msg {
		asset, err := updater.Download(info)
		return UpdateDownloadedMsg{Asset: asset, Err: err}
	}
}

func (m *Model) installUpdateCmd(asset update.DownloadedAsset) tea.Cmd {
	updater := m.updater
	currentExec, _ := os.Executable()
	return func() tea.Msg {
		result, err := updater.Install(asset, currentExec)
		return UpdateInstalledMsg{Result: result, Err: err}
	}
}

func (m *Model) refreshFeedCmd(feedID int64, feedURL string, manual bool) tea.Cmd {
	m.refreshing[feedID] = true
	conv := m.mdConverter
	return func() tea.Msg {
		result := feed.FetchFeed(feedURL)
		if !result.IsSuccess() {
			return FeedRefreshedMsg{FeedID: feedID, Err: result.Err, Result: result, Manual: manual}
		}

		parsed := result.Feed
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
			Result:   result,
			Manual:   manual,
		}
	}
}

func (m *Model) setArticleReadCmd(articleID int64, read, advance bool) tea.Cmd {
	return func() tea.Msg {
		if err := m.db.MarkRead(articleID, read); err != nil {
			return ArticleReadUpdatedMsg{ArticleID: articleID, Read: read, Advance: advance, Err: err}
		}
		return ArticleReadUpdatedMsg{ArticleID: articleID, Read: read, Advance: advance}
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
		for i := range m.articles {
			if m.articles[i].FeedID == feedID {
				m.articles[i].Read = true
			}
		}
		m.applyFilter()
		return m.loadFeedsCmd()()
	}
}

func (m *Model) markFolderReadCmd(folderID int64) tea.Cmd {
	feedIDs := make([]int64, 0)
	for _, feed := range m.feeds {
		if feed.FolderID == folderID {
			feedIDs = append(feedIDs, feed.ID)
		}
	}
	return func() tea.Msg {
		for _, feedID := range feedIDs {
			if err := m.db.MarkAllRead(feedID); err != nil {
				return ErrMsg{err}
			}
		}
		return m.loadFeedsCmd()()
	}
}

func (m *Model) clearStatusCmd() tea.Cmd {
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg {
		return StatusClearMsg{}
	})
}

func restartProcessCmd(executablePath string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(executablePath, os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return RestartedMsg{Err: fmt.Errorf("restart Tide: %w", err)}
		}
		return RestartedMsg{}
	}
}

func (m Model) openBrowserCmd(url string) tea.Cmd {
	browser := m.cfg.Display.Browser
	return func() tea.Msg {
		var cmd *exec.Cmd
		if browser != "" {
			cmd = exec.Command(browser, url)
		} else {
			switch runtime.GOOS {
			case "darwin":
				cmd = exec.Command("open", url)
			default:
				cmd = exec.Command("xdg-open", url)
			}
		}
		_ = cmd.Start()
		return nil
	}
}

func (m Model) openSummary() (tea.Model, tea.Cmd) {
	if len(m.filteredArticles) == 0 {
		return m, nil
	}
	a := m.filteredArticles[m.articleCursor]

	// If we already have a cached summary, show it immediately.
	if a.Summary != "" {
		m.summaryArticle = a
		m.summaryGenerating = false
		m.summaryErr = ""
		m.overlay = overlaySummary
		return m, nil
	}

	// No AI provider configured — prompt the user to set one up.
	if m.summarizer == nil {
		m.setStatus("AI not configured — press S to open settings", false)
		return m, m.clearStatusCmd()
	}

	m.summaryArticle = a
	m.summaryGenerating = true
	m.summaryErr = ""
	m.overlay = overlaySummary
	return m, m.aiSummarizeCmd(a)
}

func (m *Model) aiSummarizeCmd(a db.Article) tea.Cmd {
	summarizer := m.summarizer
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		summary, err := summarizer.Summarize(ctx, a.Title, a.Content)
		return AISummaryFetchedMsg{ArticleID: a.ID, Summary: summary, Err: err}
	}
}

func copyToClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		candidates := [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
			{"pbcopy"},
		}
		for _, args := range candidates {
			path, err := exec.LookPath(args[0])
			if err != nil {
				continue
			}
			cmd := exec.Command(path, args[1:]...)
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return ClipboardCopiedMsg{}
			}
		}
		return ClipboardCopiedMsg{Err: fmt.Errorf("no clipboard tool found (wl-copy/xclip/xsel/pbcopy)")}
	}
}

func saveSummaryMDCmd(a db.Article, summary, savePath string) tea.Cmd {
	return func() tea.Msg {
		if savePath == "" {
			savePath = "~/"
		}
		if strings.HasPrefix(savePath, "~/") {
			home, _ := os.UserHomeDir()
			savePath = filepath.Join(home, savePath[2:])
		}
		if err := os.MkdirAll(savePath, 0o755); err != nil {
			return SummarySavedMsg{Err: err}
		}

		filename := summaryFilename(a.Title)
		fullPath := filepath.Join(savePath, filename)

		content := fmt.Sprintf("# %s\n\n**Source:** %s\n**Published:** %s\n\n---\n\n%s\n",
			a.Title,
			a.Link,
			a.PublishedAt.Format("Mon, 02 Jan 2006"),
			summary,
		)
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return SummarySavedMsg{Err: err}
		}
		return SummarySavedMsg{Path: fullPath}
	}
}

func summaryFilename(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-':
			if b.Len() > 0 {
				s := b.String()
				if s[len(s)-1] != '-' {
					b.WriteByte('-')
				}
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "summary"
	}
	if len(s) > 50 {
		s = s[:50]
	}
	return s + ".md"
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m *Model) rebuildSidebar() {
	m.sidebarRows = buildSidebarRows(m.feeds, m.folders, m.collapsedFolders, true)
	m.sidebarCursor = clamp(m.sidebarCursor, 0, max(0, len(m.sidebarRows)-1))
}

func buildSidebarRows(feeds []db.Feed, folders []db.Folder, collapsedFolders map[int64]bool, showUncategorized bool) []sidebarRow {
	if len(folders) == 0 {
		rows := make([]sidebarRow, 0, len(feeds))
		for _, feed := range feeds {
			rows = append(rows, sidebarRow{kind: rowKindFeed, feedID: feed.ID})
		}
		return rows
	}

	byFolder := make(map[int64][]int64)
	for _, feed := range feeds {
		byFolder[feed.FolderID] = append(byFolder[feed.FolderID], feed.ID)
	}

	rows := make([]sidebarRow, 0, len(feeds)+len(folders)+1)
	for _, folder := range folders {
		rows = append(rows, sidebarRow{kind: rowKindFolder, folderID: folder.ID})
		if collapsedFolders[folder.ID] {
			continue
		}
		for _, feedID := range byFolder[folder.ID] {
			rows = append(rows, sidebarRow{kind: rowKindFeed, feedID: feedID})
		}
	}
	if uncategorized := byFolder[0]; len(uncategorized) > 0 {
		if showUncategorized {
			rows = append(rows, sidebarRow{kind: rowKindFolder, folderID: 0})
			if collapsedFolders[0] {
				return rows
			}
		}
		for _, feedID := range uncategorized {
			rows = append(rows, sidebarRow{kind: rowKindFeed, feedID: feedID})
		}
	}
	return rows
}

func (m Model) newFeedManager() FeedManager {
	fm := NewFeedManagerWithSource(m.db, m.cfg.Source)
	fm.setData(m.feeds, m.folders)
	if feed := m.selectedFeed(); feed != nil {
		fm.selectFeed(feed.ID)
	} else if folderID, ok := m.selectedFolderID(); ok {
		if folderID >= 0 {
			fm.selectFolder(folderID)
		}
	}
	return fm
}

func (m *Model) selectSidebarFeed(feedID int64) bool {
	for i, row := range m.sidebarRows {
		if row.kind == rowKindFeed && row.feedID == feedID {
			m.sidebarCursor = i
			return true
		}
	}
	return false
}

func (m Model) selectedFolderHasRemoteFeeds() bool {
	folderID, ok := m.selectedFolderID()
	if !ok {
		return false
	}
	for _, feed := range m.feeds {
		if feed.FolderID == folderID && m.isRemoteFeed(feed.ID) {
			return true
		}
	}
	return false
}

func (m *Model) clearArticles() {
	m.articles = nil
	m.filteredArticles = nil
	m.articleCursor = 0
	m.listOffset = 0
	m.viewport.SetContent("")
	m.viewport.GotoTop()
}

func (m Model) settingsUpdateState() settingsUpdateState {
	lastChecked := time.Time{}
	if m.cfg.Updates.LastCheckedUnix > 0 {
		lastChecked = time.Unix(m.cfg.Updates.LastCheckedUnix, 0)
	}
	return settingsUpdateState{
		currentVersion:   m.currentVersion,
		state:            m.updateState,
		latestVersion:    m.updateInfo.Version,
		latestIsFresh:    m.updateInfoFresh,
		publishedAt:      m.updateInfo.PublishedAt,
		summary:          m.updateInfo.Summary,
		lastChecked:      lastChecked,
		err:              m.updateErr,
		dismissed:        m.updateDismissed,
		manualCommand:    m.updateInstall.ManualCommand,
		restartable:      m.updateInstall.Restartable,
		installedVersion: m.updateInstall.Version,
	}
}

func (m *Model) syncSettingsUpdateState() {
	m.settings.setUpdateState(m.settingsUpdateState())
}

func (m *Model) dismissAvailableUpdate() tea.Cmd {
	if m.updateInfo.Version == "" {
		return nil
	}
	m.cfg.Updates.DismissedVersion = m.updateInfo.Version
	config.Save(m.cfg) //nolint:errcheck
	m.updateDismissed = true
	m.syncSettingsUpdateState()
	m.setStatus("Tide update "+m.updateInfo.Version+" dismissed", false)
	return m.clearStatusCmd()
}

func (m *Model) restoreCachedUpdateState() {
	version := strings.TrimSpace(m.cfg.Updates.AvailableVersion)
	if version == "" {
		return
	}
	if !update.IsNewerVersion(version, m.currentVersion) {
		m.clearCachedAvailableUpdate()
		return
	}
	publishedAt := time.Time{}
	if m.cfg.Updates.AvailablePublished > 0 {
		publishedAt = time.Unix(m.cfg.Updates.AvailablePublished, 0)
	}
	m.updateInfo = update.ReleaseInfo{
		Version:     version,
		Summary:     strings.TrimSpace(m.cfg.Updates.AvailableSummary),
		PublishedAt: publishedAt,
	}
	m.updateInfoFresh = false
	m.updateState = updateStateAvailable
	m.updateDismissed = version == m.cfg.Updates.DismissedVersion
}

func (m *Model) clearCachedAvailableUpdate() {
	m.cfg.Updates.AvailableVersion = ""
	m.cfg.Updates.AvailableSummary = ""
	m.cfg.Updates.AvailablePublished = 0
}

func (m Model) currentSidebarSelection() (sidebarRowKind, int64) {
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(m.sidebarRows) {
		return rowKindFeed, 0
	}
	row := m.sidebarRows[m.sidebarCursor]
	if row.kind == rowKindFolder {
		return rowKindFolder, row.folderID
	}
	return rowKindFeed, row.feedID
}

func (m Model) selectedFeed() *db.Feed {
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(m.sidebarRows) {
		return nil
	}
	row := m.sidebarRows[m.sidebarCursor]
	if row.kind != rowKindFeed {
		return nil
	}
	return m.feedByID(row.feedID)
}

func (m Model) selectedFolderID() (int64, bool) {
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(m.sidebarRows) {
		return 0, false
	}
	row := m.sidebarRows[m.sidebarCursor]
	if row.kind != rowKindFolder {
		return 0, false
	}
	return row.folderID, true
}

func (m Model) feedByID(feedID int64) *db.Feed {
	for i := range m.feeds {
		if m.feeds[i].ID == feedID {
			return &m.feeds[i]
		}
	}
	return nil
}

func (m Model) folderName(folderID int64) string {
	if folderID == 0 {
		return "Uncategorized"
	}
	for _, folder := range m.folders {
		if folder.ID == folderID {
			return folder.Name
		}
	}
	return "Folder"
}

func (m Model) folderColor(folderID int64) lipgloss.Color {
	if folderID == 0 {
		return ""
	}
	for _, folder := range m.folders {
		if folder.ID == folderID && folder.Color != "" {
			return lipgloss.Color(folder.Color)
		}
	}
	return ""
}

func (m Model) selectedFeedFolderColor() lipgloss.Color {
	if feed := m.selectedFeed(); feed != nil {
		return m.folderColor(feed.FolderID)
	}
	return ""
}

func (m Model) folderUnreadCount(folderID int64) int64 {
	var total int64
	for _, feed := range m.feeds {
		if feed.FolderID == folderID {
			total += feed.UnreadCount
		}
	}
	return total
}

func (m Model) folderHeaderStyle(folderID int64, selected bool) lipgloss.Style {
	accent := m.folderColor(folderID)
	style := m.styles.FeedItem.Copy().Foreground(lipgloss.Color(m.styles.Theme.Dimmed)).Bold(true)
	if accent != "" {
		style = style.Foreground(accent)
	}
	if selected {
		style = m.sidebarSelectedStyle(accent).Copy().Bold(true)
	}
	return style
}

func (m Model) folderBadgeStyle(folderID int64, selected bool) lipgloss.Style {
	accent := m.folderColor(folderID)
	if selected {
		return m.sidebarSelectedBadgeStyle(accent)
	}
	if accent == "" {
		return m.styles.UnreadBadge
	}
	return m.styles.UnreadBadge.Copy().Foreground(accent)
}

func (m Model) feedAccentStyle(feed db.Feed, selected bool) lipgloss.Style {
	style := m.styles.FeedItem
	accent := m.folderColor(feed.FolderID)
	if accent != "" {
		style = style.Copy().Foreground(accent)
	}
	if selected {
		style = m.sidebarSelectedStyle(accent)
	}
	return style
}

func (m Model) feedBadgeStyle(feed db.Feed, selected bool) lipgloss.Style {
	accent := m.folderColor(feed.FolderID)
	if selected {
		return m.sidebarSelectedBadgeStyle(accent)
	}
	if accent == "" {
		return m.styles.UnreadBadge
	}
	return m.styles.UnreadBadge.Copy().Foreground(accent)
}

func (m Model) sidebarSelectedStyle(accent lipgloss.Color) lipgloss.Style {
	if m.focused == paneFeeds {
		if accent != "" {
			return m.styles.FeedItemSelectedFocused.Copy().
				Background(accent).
				Foreground(readableText(m.styles.Theme.Fg, accent, 4.5))
		}
		return m.styles.FeedItemSelectedFocused
	}

	style := m.styles.FeedItemSelectedUnfocused
	if accent != "" && contrastRatio(accent, terminalColorAsColor(style.GetBackground())) >= 3 {
		style = style.Copy().Foreground(accent)
	}
	return style
}

func (m Model) sidebarSelectedBadgeStyle(accent lipgloss.Color) lipgloss.Style {
	style := m.sidebarSelectedStyle(accent)
	return m.styles.UnreadBadge.Copy().Foreground(terminalColorAsColor(style.GetForeground()))
}

func terminalColorAsColor(c lipgloss.TerminalColor) lipgloss.Color {
	if c == nil {
		return ""
	}
	return lipgloss.Color(fmt.Sprint(c))
}

func (m Model) articleRowStyles() (lipgloss.Style, lipgloss.Style, lipgloss.Style, lipgloss.Style, lipgloss.Color, lipgloss.Color) {
	accent := m.selectedFeedFolderColor()
	unread := m.styles.ArticleUnread
	read := m.styles.ArticleRead
	selected := m.styles.ArticleSelected
	headerActive := m.styles.PaneHeaderActive
	border := m.styles.Theme.Border
	borderFocus := m.styles.Theme.BorderFocus
	if accent != "" {
		unread = unread.Copy().Foreground(accent)
		selected = selected.Copy().Foreground(accent)
		headerActive = headerActive.Copy().
			Background(accent).
			Foreground(readableText(m.styles.Theme.Fg, accent, 4.5))
		borderFocus = accent
	}
	return unread, read, selected, headerActive, border, borderFocus
}

func (m *Model) toggleSelectedFolder() bool {
	folderID, ok := m.selectedFolderID()
	if !ok {
		return false
	}
	m.collapsedFolders[folderID] = !m.collapsedFolders[folderID]
	m.rebuildSidebar()
	for i, row := range m.sidebarRows {
		if row.kind == rowKindFolder && row.folderID == folderID {
			m.sidebarCursor = i
			break
		}
	}
	m.clearArticles()
	return true
}

func (m *Model) applyFilter() {
	q := strings.ToLower(m.searchQuery)
	if q == "" {
		m.filteredArticles = m.articles
		return
	}
	filtered := make([]db.Article, 0, len(m.articles))
	for _, a := range m.articles {
		if strings.Contains(strings.ToLower(a.Title), q) {
			filtered = append(filtered, a)
		}
	}
	m.filteredArticles = filtered
}

func (m Model) indexOfFilteredArticle(articleID int64) int {
	for i := range m.filteredArticles {
		if m.filteredArticles[i].ID == articleID {
			return i
		}
	}
	return -1
}

func (m *Model) setStatus(msg string, isErr bool) {
	m.statusMsg = msg
	m.statusErr = isErr
}

func (m Model) renderFetchErrorOverlay(w int, chrome managerChrome) string {
	r := m.lastFetchError
	if r == nil {
		return ""
	}

	textW := max(1, w-4)
	bg := chrome.baseBg
	surf := chrome.surfaceBg
	accent := chrome.accent
	muted := chrome.muted
	text := chrome.text

	label := func(s string) string {
		return lipgloss.NewStyle().Background(bg).Foreground(muted).Width(14).Render(s)
	}
	val := func(s string) string {
		return lipgloss.NewStyle().Background(bg).Foreground(text).Render(s)
	}
	accentLine := func(s string) string {
		return lipgloss.NewStyle().Background(bg).Foreground(accent).Bold(true).Render(s)
	}
	row := func(k, v string) string {
		return lipgloss.NewStyle().Background(bg).Width(textW).
			Render(label(k) + val(v))
	}

	header := renderManagerHeader("FETCH ERROR", w, chrome)

	// Title line
	title := accentLine(r.FriendlyMessage())

	// Detail rows
	rows := []string{""}
	if r.StatusCode != 0 {
		rows = append(rows, row("Status:", fmt.Sprintf("%d", r.StatusCode)))
	}
	if r.ContentType != "" {
		ct := r.ContentType
		if len(ct) > textW-16 {
			ct = ct[:textW-16]
		}
		rows = append(rows, row("Content-Type:", ct))
	}
	origURL := r.OriginalURL
	if len(origURL) > textW-16 {
		origURL = "…" + origURL[len(origURL)-(textW-17):]
	}
	rows = append(rows, row("Original URL:", origURL))
	if r.FinalURL != r.OriginalURL {
		finalURL := r.FinalURL
		if len(finalURL) > textW-16 {
			finalURL = "…" + finalURL[len(finalURL)-(textW-17):]
		}
		rows = append(rows, row("Final URL:", finalURL))
	}

	// Redirect chain
	if len(r.RedirectChain) > 1 {
		rows = append(rows, "")
		rows = append(rows, lipgloss.NewStyle().Background(bg).Foreground(muted).
			Render(fmt.Sprintf("Redirects (%d):", len(r.RedirectChain)-1)))
		for _, u := range r.RedirectChain {
			display := u
			if len(display) > textW-4 {
				display = "…" + display[len(display)-(textW-5):]
			}
			rows = append(rows, lipgloss.NewStyle().Background(bg).Foreground(text).
				Render("  → "+display))
		}
	}

	// Snippet
	if r.Snippet != "" {
		rows = append(rows, "")
		rows = append(rows, lipgloss.NewStyle().Background(bg).Foreground(muted).Render("Preview:"))
		snip := strings.ReplaceAll(r.Snippet, "\n", " ")
		snip = strings.Join(strings.Fields(snip), " ")
		if len(snip) > textW-2 {
			snip = snip[:textW-2] + "…"
		}
		rows = append(rows, lipgloss.NewStyle().Background(surf).Foreground(text).
			Width(textW).Padding(0, 1).Render(snip))
	}

	if r.SuggestURLUpdate {
		rows = append(rows, "")
		rows = append(rows, lipgloss.NewStyle().Background(bg).Foreground(accent).
			Render("↳ Feed permanently moved to new URL"))
	}
	actions := renderManagerActions(w, chrome, "esc", "dismiss")

	body := lipgloss.NewStyle().Background(bg).Width(w).Padding(0, 2).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, strings.Join(rows, "\n")))

	return lipgloss.JoinVertical(lipgloss.Left, header, body, actions)
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
	if lipgloss.Width(s) <= maxW {
		return s
	}
	if maxW <= 1 {
		return ansi.Truncate(s, maxW, "")
	}
	return ansi.Truncate(s, maxW-1, "") + "…"
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

func (m Model) statusLine(left, right string) string {
	maxW := max(0, m.width-4) // leave room for status bar padding
	left = strings.ReplaceAll(left, "\n", " ")
	right = strings.ReplaceAll(right, "\n", " ")
	if right == "" {
		return truncate(left, maxW)
	}

	right = truncate(right, maxW)
	rightW := lipgloss.Width(right)
	if rightW >= maxW {
		return right
	}
	if left == "" {
		return strings.Repeat(" ", maxW-rightW) + right
	}

	const gap = 2
	leftW := maxW - rightW - gap
	if leftW <= 0 {
		return right
	}

	left = truncate(left, leftW)
	return padRight(left, leftW) + strings.Repeat(" ", gap) + right
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

func folderDisplayLabel(name string, collapsed, icons bool) string {
	if !icons {
		return name
	}
	if collapsed {
		return "󰉖 " + name
	}
	return "󰉋 " + name
}

func feedDisplayLabel(name string, icons bool) string {
	if !icons {
		return name
	}
	return "\U000f046b " + name
}

func (m Model) renderFolderHeader(folderID int64, selected bool, width int) string {
	icon := "v "
	label := m.folderName(folderID)
	if m.iconsEnabled() {
		icon = "▾ "
		label = folderDisplayLabel(label, false, true)
	}
	if m.collapsedFolders[folderID] {
		icon = "> "
		if m.iconsEnabled() {
			icon = "▸ "
			label = folderDisplayLabel(m.folderName(folderID), true, true)
		}
	}
	badge := ""
	if unread := m.folderUnreadCount(folderID); unread > 0 {
		badge = m.folderBadgeStyle(folderID, selected).Render(fmt.Sprintf("(%d)", unread))
	}
	row := renderFeedRow(icon, label, badge, width)
	style := m.folderHeaderStyle(folderID, selected)
	return style.Width(width).Render(row)
}

func (m Model) renderSidebarFeedRow(feed db.Feed, selected bool, width int) string {
	badge := ""
	if feed.UnreadCount > 0 {
		badge = m.feedBadgeStyle(feed, selected).Render(fmt.Sprintf("(%d)", feed.UnreadCount))
	}

	prefix := "    "
	title := feed.Title
	if m.iconsEnabled() {
		title = feedDisplayLabel(title, true)
	} else {
		prefix = "    " + m.feedRowPrefix(selected)
	}
	if m.refreshing[feed.ID] {
		prefix = "    " + m.spinner.View() + " "
	}
	row := renderFeedRow(prefix, title, badge, width)
	style := m.feedAccentStyle(feed, selected)
	return style.Width(width).Render(row)
}

func renderArticleRow(prefix, title, age string, width int) string {
	prefixW := lipgloss.Width(prefix)
	ageW := lipgloss.Width(age)
	gapW := 2
	titleW := max(0, width-prefixW-ageW-gapW)
	row := prefix + padRight(truncate(title, titleW), titleW) + strings.Repeat(" ", gapW) + age
	return padRight(row, width)
}

func (m Model) iconsEnabled() bool {
	return m.cfg.Display.Icons
}

func (m Model) headerLabel(label string) string {
	if !m.iconsEnabled() {
		return label
	}
	switch label {
	case "Feeds":
		return "◉ Feeds"
	case "Content":
		return "▣ Content"
	}
	if strings.HasPrefix(label, "Articles") {
		return strings.Replace(label, "Articles", "≣ Articles", 1)
	}
	return label
}

func (m Model) feedRowPrefix(selected bool) string {
	if !m.iconsEnabled() {
		if selected {
			return "> "
		}
		return "  "
	}
	if selected {
		return "▸ "
	}
	return "◦ "
}

func (m Model) articleRowPrefix(read bool) string {
	if !m.iconsEnabled() {
		if read {
			return "  "
		}
		return "o "
	}
	if read {
		return "· "
	}
	return "● "
}

func (m Model) emptyFeedsHint() string {
	if m.iconsEnabled() {
		return "  ＋ press m to add feeds"
	}
	return "  press m to add feeds"
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

func fillViewWidth(view string, width int, bg lipgloss.Color) string {
	if width <= 0 || view == "" {
		return view
	}
	return clampView(view, width, strings.Count(view, "\n")+1, bg)
}

func indentBlock(view string, pad int) string {
	if view == "" || pad <= 0 {
		return view
	}
	prefix := strings.Repeat(" ", pad)
	lines := strings.Split(view, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func (m *Model) resetHelpVP() {
	winW := min(m.width-6, 90)
	winH := min(m.height-4, 38)
	vpW := winW - 2 // inside border
	vpH := winH - 2 // inside border
	m.helpVP = viewport.New(vpW, vpH)
	m.helpVP.SetContent(renderHelp(vpW, m.styles, m.keys))
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
