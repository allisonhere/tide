package ui

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tide/internal/config"
	"tide/internal/db"
	"tide/internal/feed"
	"tide/internal/greader"
	"tide/internal/opml"
)

type fmMode int

const (
	fmList fmMode = iota
	fmEdit
	fmFolderEdit
	fmImport
	fmConfirmDelete
)

type fmPaneFocus int

const (
	fmPaneList fmPaneFocus = iota
	fmPaneDetail
)

type fmRowKind int

const (
	fmRowFolder fmRowKind = iota
	fmRowFeed
)

type fmRow struct {
	kind     fmRowKind
	folderID int64
	feedID   int64
}

const (
	fmAddSourceLocal = iota
	fmAddSourceGReader
)

const (
	fmFieldAddSource = 5 + iota
	fmFieldGReaderURL
	fmFieldGReaderLogin
	fmFieldGReaderPassword
)

var fmAddSourceLabels = []string{"Local", "GReader"}

type feedManagerSource interface {
	Load() ([]db.Feed, []db.Folder, error)
	Name() string
	Editable() bool
	ShowUncategorizedFolder() bool
}

type dbFeedManagerSource struct {
	database *db.DB
}

func (s dbFeedManagerSource) Load() ([]db.Feed, []db.Folder, error) {
	if s.database == nil {
		return nil, nil, nil
	}
	feeds, err := s.database.ListFeeds()
	if err != nil {
		return nil, nil, err
	}
	folders, err := s.database.ListFolders()
	if err != nil {
		return nil, nil, err
	}
	return feeds, folders, nil
}

func (s dbFeedManagerSource) Name() string { return "Local" }

func (s dbFeedManagerSource) Editable() bool { return true }

func (s dbFeedManagerSource) ShowUncategorizedFolder() bool { return true }

type staticFeedManagerSource struct {
	name              string
	feeds             []db.Feed
	folders           []db.Folder
	editable          bool
	showUncategorized bool
}

func (s staticFeedManagerSource) Load() ([]db.Feed, []db.Folder, error) {
	feeds := append([]db.Feed(nil), s.feeds...)
	folders := append([]db.Folder(nil), s.folders...)
	return feeds, folders, nil
}

func (s staticFeedManagerSource) Name() string { return s.name }

func (s staticFeedManagerSource) Editable() bool { return s.editable }

func (s staticFeedManagerSource) ShowUncategorizedFolder() bool { return s.showUncategorized }

type FeedManager struct {
	db               *db.DB
	source           feedManagerSource
	feeds            []db.Feed
	folders          []db.Folder
	collapsedFolders map[int64]bool
	rows             []fmRow
	cursor           int
	mode             fmMode
	editTarget       int64 // 0 = new feed
	folderEditTarget int64

	titleInput           textinput.Model
	urlInput             textinput.Model
	importInput          textinput.Model
	newFolderInput       textinput.Model
	greaderURLInput      textinput.Model
	greaderLoginInput    textinput.Model
	greaderPasswordInput textinput.Model
	focusedField         int // 0=title, 1=url, 2=folder, 3=new folder, 4=color
	addSourceIdx         int
	folderCursor         int
	showNewFolder        bool
	colorCursor          int

	shouldExit   bool
	browseFeedID int64
	statusMsg    string
	busy         bool
	busyMsg      string
	paneFocus    fmPaneFocus
}

func (fm *FeedManager) setData(feeds []db.Feed, folders []db.Folder) {
	fm.feeds = append([]db.Feed(nil), feeds...)
	fm.folders = append([]db.Folder(nil), folders...)
	fm.rebuildRows()
	fm.cursor = clamp(fm.cursor, 0, max(0, len(fm.rows)-1))
	fm.folderCursor = clamp(fm.folderCursor, 0, max(0, len(fm.folderOptions())-1))
	fm.colorCursor = clamp(fm.colorCursor, 0, max(0, len(folderColorOptions)-1))
}

func NewFeedManager(database *db.DB) FeedManager {
	return NewFeedManagerWithSource(database, config.SourceConfig{})
}

func NewFeedManagerWithSource(database *db.DB, sourceCfg config.SourceConfig) FeedManager {
	return newFeedManager(database, dbFeedManagerSource{database: database}, sourceCfg)
}

func NewRemoteFeedManager(sourceName string, feeds []db.Feed, folders []db.Folder) FeedManager {
	return newFeedManager(nil, staticFeedManagerSource{
		name:              sourceName,
		feeds:             feeds,
		folders:           folders,
		editable:          false,
		showUncategorized: false,
	}, config.SourceConfig{})
}

func newFeedManager(database *db.DB, source feedManagerSource, sourceCfg config.SourceConfig) FeedManager {
	title := textinput.New()
	title.Placeholder = "Feed title"
	title.CharLimit = 200

	u := textinput.New()
	u.Placeholder = "https://example.com/feed.xml"
	u.CharLimit = 500

	imp := textinput.New()
	imp.Placeholder = "path to .opml file"
	imp.CharLimit = 500

	newFolder := textinput.New()
	newFolder.Placeholder = "Folder name"
	newFolder.CharLimit = 120

	greaderURL := textinput.New()
	greaderURL.Placeholder = "https://rss.example.com/api/greader.php"
	greaderURL.CharLimit = 500
	greaderURL.SetValue(sourceCfg.GReaderURL)

	greaderLogin := textinput.New()
	greaderLogin.Placeholder = "alice"
	greaderLogin.CharLimit = 200
	greaderLogin.SetValue(sourceCfg.GReaderLogin)

	greaderPassword := textinput.New()
	greaderPassword.Placeholder = "API password"
	greaderPassword.CharLimit = 500
	greaderPassword.EchoMode = textinput.EchoPassword
	greaderPassword.EchoCharacter = '●'
	greaderPassword.SetValue(sourceCfg.GReaderPassword)

	fm := FeedManager{
		db:                   database,
		source:               source,
		collapsedFolders:     map[int64]bool{},
		titleInput:           title,
		urlInput:             u,
		importInput:          imp,
		newFolderInput:       newFolder,
		greaderURLInput:      greaderURL,
		greaderLoginInput:    greaderLogin,
		greaderPasswordInput: greaderPassword,
	}
	fm.reload()
	return fm
}

func (fm *FeedManager) reload() {
	switch {
	case fm.source != nil:
		if feeds, folders, err := fm.source.Load(); err == nil {
			fm.feeds = feeds
			fm.folders = folders
		}
	case fm.db != nil:
		feeds, _ := fm.db.ListFeeds()
		folders, _ := fm.db.ListFolders()
		fm.feeds = feeds
		fm.folders = folders
	}
	fm.rebuildRows()
	fm.cursor = clamp(fm.cursor, 0, max(0, len(fm.rows)-1))
	fm.folderCursor = clamp(fm.folderCursor, 0, max(0, len(fm.folderOptions())-1))
	fm.colorCursor = clamp(fm.colorCursor, 0, max(0, len(folderColorOptions)-1))
}

func (fm *FeedManager) rebuildRows() {
	fm.rows = buildFeedManagerRows(fm.feeds, fm.folders, fm.showUncategorizedFolder())
}

func (fm FeedManager) managerRows() []fmRow {
	if len(fm.rows) > 0 {
		return fm.rows
	}
	return buildFeedManagerRows(fm.feeds, fm.folders, fm.showUncategorizedFolder())
}

func buildFeedManagerRows(feeds []db.Feed, folders []db.Folder, showUncategorized bool) []fmRow {
	_ = showUncategorized
	rows := make([]fmRow, 0, len(feeds)+len(folders)+1)
	for _, folder := range folders {
		rows = append(rows, fmRow{kind: fmRowFolder, folderID: folder.ID})
	}
	for _, feed := range feeds {
		rows = append(rows, fmRow{kind: fmRowFeed, feedID: feed.ID})
	}
	return rows
}

func (fm FeedManager) sourceName() string {
	if fm.source != nil && strings.TrimSpace(fm.source.Name()) != "" {
		return strings.TrimSpace(fm.source.Name())
	}
	return "Local"
}

func (fm FeedManager) editable() bool {
	if fm.source != nil {
		return fm.source.Editable()
	}
	return true
}

func (fm FeedManager) showUncategorizedFolder() bool {
	if fm.source != nil {
		return fm.source.ShowUncategorizedFolder()
	}
	return true
}

func (fm FeedManager) listSectionTitle() string {
	if fm.editable() {
		return "FOLDERS + FEEDS"
	}
	return "SUBSCRIPTIONS"
}

func (fm FeedManager) emptyStateMessage() string {
	if fm.editable() {
		return "NO FEEDS OR FOLDERS CONFIGURED"
	}
	return "NO SUBSCRIPTIONS CONFIGURED"
}

func (fm FeedManager) listModeTitle() string {
	if fm.editable() {
		return "DETAILS"
	}
	return "SUBSCRIPTION"
}

func (fm FeedManager) listPaneFocused() bool {
	return fm.mode == fmList || fm.paneFocus == fmPaneList
}

func (fm *FeedManager) setBrowseOnlyStatus() {
	fm.statusMsg = strings.ToUpper(fm.sourceName()) + " IS BROWSE-ONLY"
}

func (fm *FeedManager) focusAdd() {
	fm.mode = fmEdit
	fm.paneFocus = fmPaneList
	fm.editTarget = 0
	fm.folderEditTarget = 0
	fm.addSourceIdx = fmAddSourceLocal
	fm.titleInput.Reset()
	fm.urlInput.Reset()
	fm.newFolderInput.Reset()
	fm.focusedField = fmFieldAddSource
	fm.folderCursor = 0
	fm.showNewFolder = false
	fm.colorCursor = 0
	fm.statusMsg = ""
	fm.busy = false
	fm.busyMsg = ""
	fm.blurEditInputs()
}

func (fm *FeedManager) prefillAddFormFromSelectedRemoteFeed() {
	if fm.mode != fmEdit || fm.editTarget != 0 {
		return
	}
	feed := fm.selectedFeedRow()
	if !fm.feedIsRemote(feed) {
		return
	}

	fm.addSourceIdx = fmAddSourceGReader
	fm.titleInput.Reset()
	fm.titleInput.SetValue(feed.Title)
	fm.urlInput.Reset()
	fm.urlInput.SetValue(feed.URL)
	fm.newFolderInput.Reset()
	fm.folderCursor = 0
	fm.showNewFolder = false
	fm.colorCursor = 0
	fm.focusedField = fmFieldAddSource
}

func (fm *FeedManager) focusFolderEdit(folder db.Folder) {
	fm.mode = fmFolderEdit
	fm.paneFocus = fmPaneDetail
	fm.folderEditTarget = folder.ID
	fm.titleInput.Reset()
	fm.titleInput.SetValue(folder.Name)
	fm.focusedField = 0
	fm.statusMsg = ""
	fm.busy = false
	fm.busyMsg = ""
	fm.titleInput.Focus()
	if _, idx, ok := folderColorByValue(folder.Color); ok {
		fm.colorCursor = idx
	} else {
		fm.colorCursor = 0
	}
}

func (fm *FeedManager) focusAddFolder() {
	fm.mode = fmFolderEdit
	fm.paneFocus = fmPaneDetail
	fm.folderEditTarget = 0
	fm.titleInput.Reset()
	fm.focusedField = 0
	fm.statusMsg = ""
	fm.busy = false
	fm.busyMsg = ""
	fm.titleInput.Focus()
	fm.colorCursor = 0
}

func (fm FeedManager) folderOptions() []string {
	options := make([]string, 0, len(fm.folders)+2)
	options = append(options, "(no folder)")
	for _, folder := range fm.folders {
		options = append(options, folder.Name)
	}
	options = append(options, "+ New folder")
	return options
}

func (fm FeedManager) currentFolderID() int64 {
	if fm.folderCursor <= 0 || fm.folderCursor > len(fm.folders) {
		return 0
	}
	return fm.folders[fm.folderCursor-1].ID
}

func (fm FeedManager) currentColorOption() folderColorOption {
	return folderColorOptions[clamp(fm.colorCursor, 0, len(folderColorOptions)-1)]
}

func (fm FeedManager) displayedColorOption() folderColorOption {
	if fm.mode == fmEdit && !fm.showNewFolder {
		if folder := fm.pickedFolder(); folder != nil {
			if option, _, ok := folderColorByValue(folder.Color); ok {
				return option
			}
		}
	}
	return fm.currentColorOption()
}

func (fm FeedManager) selectedRow() *fmRow {
	rows := fm.managerRows()
	if fm.cursor < 0 || fm.cursor >= len(rows) {
		return nil
	}
	row := rows[fm.cursor]
	return &row
}

func (fm FeedManager) selectedFeedRow() *db.Feed {
	row := fm.selectedRow()
	if row == nil || row.kind != fmRowFeed {
		return nil
	}
	for i := range fm.feeds {
		if fm.feeds[i].ID == row.feedID {
			return &fm.feeds[i]
		}
	}
	return nil
}

func (fm FeedManager) feedIsRemote(feed *db.Feed) bool {
	return feed != nil && feed.ID < 0
}

func (fm *FeedManager) setRemoteBrowseOnlyStatus() {
	fm.statusMsg = "REMOTE FEEDS ARE BROWSE-ONLY"
}

func (fm FeedManager) selectedFolder() *db.Folder {
	if row := fm.selectedRow(); row != nil && row.kind == fmRowFolder {
		for i := range fm.folders {
			if fm.folders[i].ID == row.folderID {
				return &fm.folders[i]
			}
		}
		return nil
	}
	id := fm.currentFolderID()
	if id != 0 {
		for i := range fm.folders {
			if fm.folders[i].ID == id {
				return &fm.folders[i]
			}
		}
	}
	return nil
}

func (fm FeedManager) pickedFolder() *db.Folder {
	id := fm.currentFolderID()
	if id == 0 {
		return nil
	}
	for i := range fm.folders {
		if fm.folders[i].ID == id {
			return &fm.folders[i]
		}
	}
	return nil
}

func (fm *FeedManager) selectFeed(feedID int64) {
	for i, row := range fm.managerRows() {
		if row.kind == fmRowFeed && row.feedID == feedID {
			fm.cursor = i
			return
		}
	}
}

func (fm *FeedManager) selectFolder(folderID int64) {
	for i, row := range fm.managerRows() {
		if row.kind == fmRowFolder && row.folderID == folderID {
			fm.cursor = i
			return
		}
	}
}

func (fm FeedManager) shouldShowColorPicker() bool {
	if fm.mode == fmEdit && fm.editTarget == 0 && fm.addSourceIdx == fmAddSourceGReader {
		return false
	}
	if fm.mode == fmEdit {
		return fm.showNewFolder || fm.pickedFolder() != nil
	}
	if fm.mode == fmFolderEdit {
		return true
	}
	return fm.showNewFolder || fm.selectedFolder() != nil
}

func (fm *FeedManager) syncFolderPicker() {
	maxIdx := max(0, len(fm.folderOptions())-1)
	fm.folderCursor = clamp(fm.folderCursor, 0, maxIdx)
	fm.showNewFolder = fm.folderCursor == len(fm.folderOptions())-1
	if fm.showNewFolder && fm.focusedField == 3 {
		fm.newFolderInput.Focus()
	} else {
		fm.newFolderInput.Blur()
	}
	fm.setColorCursorFromCurrentFolder()
}

func (fm *FeedManager) setFolderCursorForID(folderID int64) {
	fm.folderCursor = 0
	for i, folder := range fm.folders {
		if folder.ID == folderID {
			fm.folderCursor = i + 1
			break
		}
	}
	fm.syncFolderPicker()
	fm.setColorCursorFromCurrentFolder()
}

func (fm *FeedManager) focusCurrentEditField() {
	fm.blurEditInputs()

	switch fm.focusedField {
	case 0:
		fm.titleInput.Focus()
	case 1:
		fm.urlInput.Focus()
	case 3:
		if fm.showNewFolder {
			fm.newFolderInput.Focus()
		} else {
			fm.focusedField = 0
			fm.titleInput.Focus()
		}
	case 4:
		if !fm.shouldShowColorPicker() {
			fm.focusedField = 0
			fm.titleInput.Focus()
		}
	case fmFieldGReaderURL:
		fm.greaderURLInput.Focus()
	case fmFieldGReaderLogin:
		fm.greaderLoginInput.Focus()
	case fmFieldGReaderPassword:
		fm.greaderPasswordInput.Focus()
	}
}

func (fm *FeedManager) blurEditInputs() {
	fm.titleInput.Blur()
	fm.urlInput.Blur()
	fm.newFolderInput.Blur()
	fm.greaderURLInput.Blur()
	fm.greaderLoginInput.Blur()
	fm.greaderPasswordInput.Blur()
}

func (fm FeedManager) editFieldOrder() []int {
	if fm.editTarget == 0 {
		order := []int{fmFieldAddSource}
		if fm.addSourceIdx == fmAddSourceGReader {
			return append(order, 0, 1, fmFieldGReaderURL, fmFieldGReaderLogin, fmFieldGReaderPassword)
		}
		order = append(order, 0, 1, 2)
		if fm.showNewFolder {
			order = append(order, 3)
		}
		if fm.shouldShowColorPicker() {
			order = append(order, 4)
		}
		return order
	}
	order := []int{0, 1, 2}
	if fm.showNewFolder {
		order = append(order, 3)
	}
	if fm.shouldShowColorPicker() {
		order = append(order, 4)
	}
	return order
}

func (fm FeedManager) greaderFeedURL() string {
	return strings.TrimSpace(fm.urlInput.Value())
}

func (fm *FeedManager) advanceEditField() {
	order := fm.editFieldOrder()
	for i, field := range order {
		if field == fm.focusedField {
			fm.focusedField = order[(i+1)%len(order)]
			fm.focusCurrentEditField()
			return
		}
	}
	fm.focusedField = order[0]
	fm.focusCurrentEditField()
}

func (fm FeedManager) isEditTextInputFocused() bool {
	switch fm.focusedField {
	case 0, 1:
		return true
	case 3:
		return fm.showNewFolder
	case fmFieldGReaderURL, fmFieldGReaderLogin, fmFieldGReaderPassword:
		return fm.editTarget == 0 && fm.addSourceIdx == fmAddSourceGReader
	default:
		return false
	}
}

func (fm FeedManager) focusedEditTextInputCursorPosition() int {
	switch fm.focusedField {
	case 0:
		return fm.titleInput.Position()
	case 1:
		return fm.urlInput.Position()
	case 3:
		if fm.showNewFolder {
			return fm.newFolderInput.Position()
		}
	case fmFieldGReaderURL:
		if fm.editTarget == 0 && fm.addSourceIdx == fmAddSourceGReader {
			return fm.greaderURLInput.Position()
		}
	case fmFieldGReaderLogin:
		if fm.editTarget == 0 && fm.addSourceIdx == fmAddSourceGReader {
			return fm.greaderLoginInput.Position()
		}
	case fmFieldGReaderPassword:
		if fm.editTarget == 0 && fm.addSourceIdx == fmAddSourceGReader {
			return fm.greaderPasswordInput.Position()
		}
	}
	return -1
}

func (fm FeedManager) updateFocusedEditInput(msg tea.Msg) (FeedManager, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case fm.focusedField == 0:
		fm.titleInput, cmd = fm.titleInput.Update(msg)
	case fm.focusedField == 1:
		fm.urlInput, cmd = fm.urlInput.Update(msg)
	case fm.focusedField == 3 && fm.showNewFolder:
		fm.newFolderInput, cmd = fm.newFolderInput.Update(msg)
	case fm.focusedField == fmFieldGReaderURL:
		fm.greaderURLInput, cmd = fm.greaderURLInput.Update(msg)
	case fm.focusedField == fmFieldGReaderLogin:
		fm.greaderLoginInput, cmd = fm.greaderLoginInput.Update(msg)
	case fm.focusedField == fmFieldGReaderPassword:
		fm.greaderPasswordInput, cmd = fm.greaderPasswordInput.Update(msg)
	}
	return fm, cmd
}

func (fm FeedManager) updateFocusedFolderEditInput(msg tea.Msg) (FeedManager, tea.Cmd) {
	var cmd tea.Cmd
	if fm.focusedField == 0 {
		fm.titleInput, cmd = fm.titleInput.Update(msg)
	}
	return fm, cmd
}

func (fm *FeedManager) setColorCursorFromCurrentFolder() {
	fm.colorCursor = 0
	var color string
	if fm.showNewFolder {
		color = string(folderColorOptions[0].Color)
	} else if folder := fm.pickedFolder(); folder != nil {
		color = folder.Color
	}
	if _, idx, ok := folderColorByValue(color); ok {
		fm.colorCursor = idx
	}
}

func (fm FeedManager) Update(msg tea.Msg, keys KeyMap) (FeedManager, tea.Cmd, bool) {
	fm.browseFeedID = 0
	fm, cmd := fm.update(msg, keys)
	exit := fm.shouldExit
	fm.shouldExit = false
	return fm, cmd, exit
}

func (fm FeedManager) update(msg tea.Msg, keys KeyMap) (FeedManager, tea.Cmd) {
	// Route non-key messages to the focused textinput (cursor blink ticks etc.)
	if _, ok := msg.(tea.KeyMsg); !ok {
		if fm.busy {
			return fm, nil
		}
		switch fm.mode {
		case fmEdit:
			return fm.updateFocusedEditInput(msg)
		case fmFolderEdit:
			return fm.updateFocusedFolderEditInput(msg)
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
	case fmFolderEdit:
		return fm.updateFolderEdit(key, keys)
	case fmImport:
		return fm.updateImport(key, keys)
	case fmConfirmDelete:
		return fm.updateConfirmDelete(key, keys)
	}
	return fm, nil
}

func (fm FeedManager) updateList(msg tea.KeyMsg, keys KeyMap) (FeedManager, tea.Cmd) {
	if fm.busy {
		return fm, nil
	}
	switch {
	case keyMatches(msg, keys.Back):
		fm.shouldExit = true

	case keyMatches(msg, keys.Up):
		if fm.cursor > 0 {
			fm.cursor--
		}

	case keyMatches(msg, keys.Down):
		if fm.cursor < len(fm.managerRows())-1 {
			fm.cursor++
		}

	case keyMatches(msg, keys.Add):
		if !fm.editable() {
			fm.setBrowseOnlyStatus()
			return fm, nil
		}
		fm.focusAdd()

	case keyMatches(msg, keys.AddFolder):
		if !fm.editable() {
			fm.setBrowseOnlyStatus()
			return fm, nil
		}
		fm.focusAddFolder()

	case keyMatches(msg, keys.Edit):
		if !fm.editable() {
			fm.setBrowseOnlyStatus()
			return fm, nil
		}
		if row := fm.selectedRow(); row != nil {
			if row.kind == fmRowFolder {
				if folder := fm.selectedFolder(); folder != nil {
					fm.focusFolderEdit(*folder)
				}
				return fm, nil
			}
			if f := fm.selectedFeedRow(); f != nil {
				if fm.feedIsRemote(f) {
					fm.setRemoteBrowseOnlyStatus()
					return fm, nil
				}
				fm.editTarget = f.ID
				fm.folderEditTarget = 0
				fm.statusMsg = ""
				fm.busy = false
				fm.busyMsg = ""
				fm.titleInput.Reset()
				fm.titleInput.SetValue(f.Title)
				fm.urlInput.Reset()
				fm.urlInput.SetValue(f.URL)
				fm.newFolderInput.Reset()
				fm.focusedField = 0
				fm.setFolderCursorForID(f.FolderID)
				fm.focusCurrentEditField()
				fm.mode = fmEdit
				fm.paneFocus = fmPaneDetail
			}
		}

	case keyMatches(msg, keys.Enter):
		if f := fm.selectedFeedRow(); f != nil && fm.feedIsRemote(f) {
			fm.browseFeedID = f.ID
			fm.shouldExit = true
			return fm, nil
		}
		if !fm.editable() {
			if f := fm.selectedFeedRow(); f != nil {
				fm.browseFeedID = f.ID
				fm.shouldExit = true
			}
			return fm, nil
		}
		if row := fm.selectedRow(); row != nil {
			if row.kind == fmRowFolder {
				if folder := fm.selectedFolder(); folder != nil {
					fm.focusFolderEdit(*folder)
				}
				return fm, nil
			}
			if f := fm.selectedFeedRow(); f != nil {
				if fm.feedIsRemote(f) {
					fm.browseFeedID = f.ID
					fm.shouldExit = true
					return fm, nil
				}
				fm.editTarget = f.ID
				fm.folderEditTarget = 0
				fm.statusMsg = ""
				fm.busy = false
				fm.busyMsg = ""
				fm.titleInput.Reset()
				fm.titleInput.SetValue(f.Title)
				fm.urlInput.Reset()
				fm.urlInput.SetValue(f.URL)
				fm.newFolderInput.Reset()
				fm.focusedField = 0
				fm.setFolderCursorForID(f.FolderID)
				fm.focusCurrentEditField()
				fm.mode = fmEdit
				fm.paneFocus = fmPaneDetail
			}
		}

	case keyMatches(msg, keys.Delete):
		if !fm.editable() {
			fm.setBrowseOnlyStatus()
			return fm, nil
		}
		if f := fm.selectedFeedRow(); f != nil && fm.feedIsRemote(f) {
			fm.setRemoteBrowseOnlyStatus()
			return fm, nil
		}
		if fm.selectedRow() != nil {
			fm.mode = fmConfirmDelete
		}

	case keyMatches(msg, keys.Import):
		if !fm.editable() {
			fm.setBrowseOnlyStatus()
			return fm, nil
		}
		fm.statusMsg = ""
		fm.busy = false
		fm.busyMsg = ""
		fm.importInput.Reset()
		fm.importInput.Focus()
		fm.mode = fmImport

	case keyMatches(msg, keys.Export):
		if !fm.editable() {
			fm.setBrowseOnlyStatus()
			return fm, nil
		}
		return fm, fm.exportCmd()
	}
	return fm, nil
}

func (fm FeedManager) updateEdit(msg tea.KeyMsg, keys KeyMap) (FeedManager, tea.Cmd) {
	if fm.busy {
		return fm, nil
	}
	if fm.paneFocus == fmPaneList {
		switch {
		case keyMatches(msg, keys.Cancel):
			fm.mode = fmList
			fm.paneFocus = fmPaneList
			fm.blurEditInputs()
		case keyMatches(msg, keys.Up):
			if fm.cursor > 0 {
				fm.cursor--
			}
		case keyMatches(msg, keys.Down):
			if fm.cursor < len(fm.managerRows())-1 {
				fm.cursor++
			}
		case keyMatches(msg, keys.Right), keyMatches(msg, keys.Tab), keyMatches(msg, keys.Confirm):
			fm.prefillAddFormFromSelectedRemoteFeed()
			fm.paneFocus = fmPaneDetail
			fm.focusCurrentEditField()
		}
		return fm, nil
	}
	if fm.isEditTextInputFocused() && keyMatches(msg, keys.Left) {
		if fm.focusedEditTextInputCursorPosition() == 0 {
			fm.paneFocus = fmPaneList
			fm.blurEditInputs()
			return fm, nil
		}
		return fm.updateFocusedEditInput(msg)
	}
	if fm.isEditTextInputFocused() && msg.Type == tea.KeyRunes {
		return fm.updateFocusedEditInput(msg)
	}
	switch {
	case keyMatches(msg, keys.Cancel):
		fm.mode = fmList
		fm.paneFocus = fmPaneList
		fm.blurEditInputs()

	case keyMatches(msg, keys.Tab), keyMatches(msg, keys.Down):
		fm.advanceEditField()

	case keyMatches(msg, keys.Up):
		order := fm.editFieldOrder()
		for i, field := range order {
			if field == fm.focusedField {
				fm.focusedField = order[(i+len(order)-1)%len(order)]
				fm.focusCurrentEditField()
				return fm, nil
			}
		}
		fm.focusedField = order[0]
		fm.focusCurrentEditField()

	case fm.focusedField == fmFieldAddSource && (keyMatches(msg, keys.Left) || keyMatches(msg, keys.Right) || keyMatches(msg, keys.Enter) || msg.String() == " "):
		fm.addSourceIdx = (fm.addSourceIdx + 1) % len(fmAddSourceLabels)
		fm.folderCursor = 0
		fm.showNewFolder = false
		fm.focusCurrentEditField()

	case fm.focusedField == 2 && keyMatches(msg, keys.Left):
		if fm.folderCursor > 0 {
			fm.folderCursor--
			fm.syncFolderPicker()
			if !fm.showNewFolder && fm.focusedField == 3 {
				fm.focusedField = 2
			}
			fm.focusCurrentEditField()
		}

	case fm.focusedField == 2 && keyMatches(msg, keys.Right):
		if fm.folderCursor < len(fm.folderOptions())-1 {
			fm.folderCursor++
			fm.syncFolderPicker()
			fm.focusCurrentEditField()
		}

	case fm.focusedField == 4 && keyMatches(msg, keys.Left):
		if fm.showNewFolder && fm.colorCursor > 0 {
			fm.colorCursor--
		}

	case fm.focusedField == 4 && keyMatches(msg, keys.Right):
		if fm.showNewFolder && fm.colorCursor < len(folderColorOptions)-1 {
			fm.colorCursor++
		}

	case keyMatches(msg, keys.Confirm):
		if fm.focusedField == fmFieldAddSource {
			fm.addSourceIdx = (fm.addSourceIdx + 1) % len(fmAddSourceLabels)
			fm.focusCurrentEditField()
			return fm, nil
		}
		if fm.editTarget == 0 && fm.addSourceIdx == fmAddSourceGReader {
			if fm.greaderFeedURL() == "" {
				fm.busyMsg = "LOADING GREADER FEEDS..."
			} else {
				fm.busyMsg = "ADDING GREADER FEED..."
			}
		} else if fm.editTarget != 0 {
			fm.busyMsg = "SAVING FEED..."
		} else {
			fm.busyMsg = "ADDING FEED..."
		}
		fm.statusMsg = fm.busyMsg
		fm.busy = true
		return fm, fm.saveCmd()

	default:
		return fm.updateFocusedEditInput(msg)
	}
	return fm, nil
}

func (fm FeedManager) updateFolderEdit(msg tea.KeyMsg, keys KeyMap) (FeedManager, tea.Cmd) {
	if fm.busy {
		return fm, nil
	}
	if fm.focusedField == 0 && msg.Type == tea.KeyRunes {
		return fm.updateFocusedFolderEditInput(msg)
	}
	switch {
	case keyMatches(msg, keys.Cancel):
		fm.mode = fmList
		fm.titleInput.Blur()

	case keyMatches(msg, keys.Tab), keyMatches(msg, keys.Down):
		if fm.focusedField == 0 {
			fm.focusedField = 4
		} else {
			fm.focusedField = 0
		}
		fm.focusCurrentEditField()

	case keyMatches(msg, keys.Up):
		if fm.focusedField == 0 {
			fm.focusedField = 4
		} else {
			fm.focusedField = 0
		}
		fm.focusCurrentEditField()

	case fm.focusedField == 4 && keyMatches(msg, keys.Left):
		if fm.colorCursor > 0 {
			fm.colorCursor--
		}

	case fm.focusedField == 4 && keyMatches(msg, keys.Right):
		if fm.colorCursor < len(folderColorOptions)-1 {
			fm.colorCursor++
		}

	case keyMatches(msg, keys.Confirm):
		fm.busyMsg = "SAVING FOLDER..."
		fm.statusMsg = fm.busyMsg
		fm.busy = true
		return fm, fm.saveFolderCmd()

	default:
		return fm.updateFocusedFolderEditInput(msg)
	}
	return fm, nil
}

func (fm FeedManager) updateImport(msg tea.KeyMsg, keys KeyMap) (FeedManager, tea.Cmd) {
	if fm.busy {
		return fm, nil
	}
	switch {
	case keyMatches(msg, keys.Cancel):
		fm.mode = fmList
		fm.importInput.Blur()

	case keyMatches(msg, keys.Confirm):
		path := strings.TrimSpace(fm.importInput.Value())
		fm.statusMsg = "IMPORTING OPML..."
		fm.busyMsg = fm.statusMsg
		fm.busy = true
		return fm, fm.importCmd(path)

	default:
		var cmd tea.Cmd
		fm.importInput, cmd = fm.importInput.Update(msg)
		return fm, cmd
	}
	return fm, nil
}

func (fm FeedManager) updateConfirmDelete(msg tea.KeyMsg, _ KeyMap) (FeedManager, tea.Cmd) {
	if fm.busy {
		return fm, nil
	}
	switch msg.String() {
	case "y":
		fm.mode = fmList
		if row := fm.selectedRow(); row != nil {
			if row.kind == fmRowFolder {
				fm.statusMsg = "DELETING FOLDER..."
				fm.busyMsg = fm.statusMsg
				fm.busy = true
				return fm, fm.deleteFolderCmd(row.folderID)
			}
			fm.statusMsg = "DELETING FEED..."
			fm.busyMsg = fm.statusMsg
			fm.busy = true
			return fm, fm.deleteCmd(row.feedID)
		}
	case "n", "esc":
		fm.mode = fmList
	}
	return fm, nil
}

// ── Commands ──────────────────────────────────────────────────────────────────

func validateHTTPURL(raw string, fieldName string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%s is required", fieldName)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid %s: %s", fieldName, raw)
	}
	return nil
}

func (fm *FeedManager) saveCmd() tea.Cmd {
	rawURL := strings.TrimSpace(fm.urlInput.Value())
	title := strings.TrimSpace(fm.titleInput.Value())
	newFolderName := strings.TrimSpace(fm.newFolderInput.Value())
	greaderURL := strings.TrimSpace(fm.greaderURLInput.Value())
	greaderLogin := strings.TrimSpace(fm.greaderLoginInput.Value())
	greaderPassword := strings.TrimSpace(fm.greaderPasswordInput.Value())
	editTarget := fm.editTarget
	addSourceIdx := fm.addSourceIdx
	folderID := fm.currentFolderID()
	createFolder := fm.showNewFolder
	selectedColor := string(fm.currentColorOption().Color)
	database := fm.db

	return func() tea.Msg {
		if editTarget == 0 && addSourceIdx == fmAddSourceGReader {
			if err := validateHTTPURL(greaderURL, "API URL"); err != nil {
				return RemoteFeedAddedMsg{Err: err}
			}
			if strings.TrimSpace(greaderLogin) == "" {
				return RemoteFeedAddedMsg{Err: fmt.Errorf("login is required")}
			}
			if strings.TrimSpace(greaderPassword) == "" {
				return RemoteFeedAddedMsg{Err: fmt.Errorf("password is required")}
			}
			client := greader.New(greaderURL, greaderLogin, greaderPassword)
			if rawURL == "" {
				subscriptions, err := client.ListSubscriptions(context.Background())
				if err != nil {
					return RemoteFeedAddedMsg{Err: err}
				}
				return RemoteFeedAddedMsg{
					Source: config.SourceConfig{
						GReaderURL:      greaderURL,
						GReaderLogin:    greaderLogin,
						GReaderPassword: greaderPassword,
					},
					FeedCount: len(subscriptions),
				}
			}
			if err := validateHTTPURL(rawURL, "feed URL"); err != nil {
				return RemoteFeedAddedMsg{Err: err}
			}
			result, err := client.QuickAdd(context.Background(), rawURL)
			if err != nil {
				return RemoteFeedAddedMsg{Err: err}
			}
			remoteTitle := strings.TrimSpace(result.StreamName)
			if remoteTitle == "" {
				remoteTitle = title
			}
			if remoteTitle == "" {
				remoteTitle = strings.TrimSpace(result.Query)
			}
			if remoteTitle == "" {
				remoteTitle = rawURL
			}
			return RemoteFeedAddedMsg{
				Source: config.SourceConfig{
					GReaderURL:      greaderURL,
					GReaderLogin:    greaderLogin,
					GReaderPassword: greaderPassword,
				},
				StreamID: result.StreamID,
				Title:    remoteTitle,
			}
		}

		u, err := url.Parse(rawURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return FeedSavedMsg{Err: fmt.Errorf("invalid URL: %s", rawURL)}
		}

		if createFolder && newFolderName != "" {
			folderID, err = database.AddFolder(newFolderName, selectedColor)
			if err != nil {
				return FeedSavedMsg{Err: err}
			}
		} else if createFolder {
			folderID = 0
		}

		if editTarget != 0 {
			// Edit existing
			if err := database.UpdateFeed(editTarget, title, rawURL); err != nil {
				return FeedSavedMsg{Err: err}
			}
			if err := database.SetFeedFolder(editTarget, folderID); err != nil {
				return FeedSavedMsg{Err: err}
			}
			if folderID != 0 && createFolder {
				if err := database.SetFolderColor(folderID, selectedColor); err != nil {
					return FeedSavedMsg{Err: err}
				}
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
		if err := database.SetFeedFolder(id, folderID); err != nil {
			return FeedSavedMsg{Err: err}
		}
		if folderID != 0 && createFolder {
			if err := database.SetFolderColor(folderID, selectedColor); err != nil {
				return FeedSavedMsg{Err: err}
			}
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

func (fm *FeedManager) saveFolderCmd() tea.Cmd {
	folderID := fm.folderEditTarget
	name := strings.TrimSpace(fm.titleInput.Value())
	color := string(fm.currentColorOption().Color)
	database := fm.db

	return func() tea.Msg {
		if folderID == 0 {
			id, err := database.AddFolder(name, color)
			if err != nil {
				return FolderSavedMsg{Err: err}
			}
			folders, err := database.ListFolders()
			if err != nil {
				return FolderSavedMsg{Err: err}
			}
			for _, folder := range folders {
				if folder.ID == id {
					return FolderSavedMsg{Folder: folder}
				}
			}
			return FolderSavedMsg{Err: fmt.Errorf("folder %d not found", id)}
		}
		if err := database.RenameFolder(folderID, name); err != nil {
			return FolderSavedMsg{Err: err}
		}
		if err := database.SetFolderColor(folderID, color); err != nil {
			return FolderSavedMsg{Err: err}
		}
		folders, err := database.ListFolders()
		if err != nil {
			return FolderSavedMsg{Err: err}
		}
		for _, folder := range folders {
			if folder.ID == folderID {
				return FolderSavedMsg{Folder: folder}
			}
		}
		return FolderSavedMsg{Err: fmt.Errorf("folder %d not found", folderID)}
	}
}

func (fm *FeedManager) deleteFolderCmd(folderID int64) tea.Cmd {
	database := fm.db
	return func() tea.Msg {
		err := database.DeleteFolder(folderID)
		return FolderDeletedMsg{FolderID: folderID, Err: err}
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

func (fm FeedManager) View(width, height int, styles Styles, icons bool) string {
	contentW := min(width, 74)
	chrome := newManagerChrome(contentW, styles.Theme)
	header := renderManagerHeader("MANAGER", contentW, chrome)
	status := ""
	hints := ""

	if fm.mode != fmList {
		hints = fm.viewHints(contentW, chrome)
	}

	if fm.statusMsg != "" {
		status = chrome.statusBar.Render(truncate(strings.ToUpper(strings.ReplaceAll(fm.statusMsg, "\n", " ")), max(1, contentW-4)))
	}

	statusH, hintsH := 0, 0
	if status != "" {
		statusH = lipgloss.Height(status)
	}
	if hints != "" {
		hintsH = lipgloss.Height(hints)
	}
	spacerH := 1
	bodyH := max(1, height-lipgloss.Height(header)-spacerH-statusH-hintsH)
	if fm.mode == fmList {
		bodyH = max(1, bodyH-lipgloss.Height(fm.viewListActions(contentW, chrome)))
	}
	body := fm.viewSplit(contentW, bodyH, chrome, styles, icons)

	spacer := lipgloss.NewStyle().Background(chrome.baseBg).Width(contentW).Render("")
	parts := []string{header, spacer, body}
	if status != "" {
		parts = append(parts, status)
	}
	if hints != "" && fm.mode != fmList {
		parts = append(parts, hints)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (fm FeedManager) viewSplit(width, height int, chrome managerChrome, styles Styles, icons bool) string {
	leftW := clamp(width/4, 18, 20)
	if width-leftW-1 < 24 {
		leftW = max(18, width-25)
	}
	rightW := max(18, width-leftW-1)
	left := fm.viewListPane(leftW, height, chrome, styles, icons)
	right := fm.viewWorkspacePane(rightW, height, chrome, styles)
	separator := lipgloss.NewStyle().Background(chrome.baseBg).Render(" ")
	main := lipgloss.JoinHorizontal(lipgloss.Top, left, separator, right)
	if fm.mode == fmList {
		return lipgloss.JoinVertical(lipgloss.Left, main, fm.viewListActions(width, chrome))
	}
	return main
}

func (fm FeedManager) viewListPane(width, height int, chrome managerChrome, styles Styles, icons bool) string {
	rows := fm.managerRows()
	if len(rows) == 0 {
		body := renderManagerPanel(width, fm.emptyStateMessage(), chrome)
		section := clampView(renderManagerPaneSection(fm.listSectionTitle(), body, fm.listPaneFocused(), chrome), width, height, chrome.baseBg)
		return lipgloss.NewStyle().Width(width).Height(height).Background(chrome.baseBg).Render(section)
	}
	listRows := make([]string, 0, len(rows))
	for i, row := range rows {
		switch row.kind {
		case fmRowFolder:
			folder := fm.folderByID(row.folderID)
			if folder == nil {
				continue
			}
			label := strings.ToUpper(truncate(folder.Name, max(8, width-6)))
			listRows = append(listRows, renderManagerFolderRow(width, label, folder.Color, fm.collapsedFolders[folder.ID], chrome, styles, i == fm.cursor, icons))
		case fmRowFeed:
			feed := fm.feedByID(row.feedID)
			if feed == nil {
				continue
			}
			title := strings.ToUpper(truncate(feed.Title, max(8, width-6)))
			if i == fm.cursor {
				listRows = append(listRows, renderManagerSelectedRow(width, feedDisplayLabel(title, icons), chrome, styles))
				continue
			}
			color := ""
			if folder := fm.folderByID(feed.FolderID); folder != nil {
				color = folder.Color
			}
			listRows = append(listRows, renderManagerFeedRow(width, title, color, chrome, icons))
		}
	}
	section := clampView(renderManagerPaneSection(fm.listSectionTitle(), lipgloss.JoinVertical(lipgloss.Left, listRows...), fm.listPaneFocused(), chrome), width, height, chrome.baseBg)
	return lipgloss.NewStyle().Width(width).Height(height).Background(chrome.baseBg).Render(section)
}

func (fm FeedManager) viewListDetails(width int, chrome managerChrome) string {
	if row := fm.selectedRow(); row != nil {
		switch row.kind {
		case fmRowFolder:
			if folder := fm.folderByID(row.folderID); folder != nil {
				colorName := "THEME DEFAULT"
				if option, _, ok := folderColorByValue(folder.Color); ok {
					colorName = strings.ToUpper(option.Name)
				} else if folder.Color != "" {
					colorName = strings.ToUpper(folder.Color)
				}
				body := strings.ToUpper(truncate(folder.Name, max(8, width-4))) + "\n" +
					fmt.Sprintf("FEEDS: %d\nUNREAD: %d\nCOLOR: %s", fm.folderFeedCount(folder.ID), fm.folderUnreadCount(folder.ID), colorName)
				return renderManagerPanel(width, body, chrome)
			}
		case fmRowFeed:
			if feed := fm.feedByID(row.feedID); feed != nil {
				sourceLine := renderManagerSourceLine(width, strings.ToUpper(truncate(feed.URL, max(8, width-4))), chrome)
				if fm.feedIsRemote(feed) {
					apiURL := strings.TrimSpace(fm.greaderURLInput.Value())
					if apiURL == "" {
						apiURL = "not set"
					}
					login := strings.TrimSpace(fm.greaderLoginInput.Value())
					if login == "" {
						login = "not set"
					}
					password := maskedPreview(strings.TrimSpace(fm.greaderPasswordInput.Value()), 12)
					if password == "" {
						password = "not set"
					}
					lines := []string{
						sourceLine,
						"SOURCE: GOOGLE READER",
						"API URL: " + strings.ToUpper(apiURL),
						"LOGIN: " + strings.ToUpper(login),
						"PASSWORD: " + password,
					}
					if category := strings.TrimSpace(feed.Description); category != "" {
						lines = append(lines, "CATEGORY: "+strings.ToUpper(category))
					}
					return renderManagerPanel(width, strings.Join(lines, "\n"), chrome)
				}
				if !fm.editable() {
					lines := []string{sourceLine, "SOURCE: " + strings.ToUpper(fm.sourceName())}
					if category := strings.TrimSpace(feed.Description); category != "" {
						lines = append(lines, "CATEGORY: "+strings.ToUpper(category))
					}
					return renderManagerPanel(width, strings.Join(lines, "\n"), chrome)
				}
				if folder := fm.folderByID(feed.FolderID); folder != nil {
					return renderManagerPanel(width, sourceLine+"\nFOLDER: "+strings.ToUpper(folder.Name), chrome)
				}
				return renderManagerPanel(width, sourceLine+"\nFOLDER: UNCATEGORIZED", chrome)
			}
		}
	}
	return renderManagerPanel(width, "NO SELECTION", chrome)
}

func (fm FeedManager) viewWorkspacePane(width, height int, chrome managerChrome, styles Styles) string {
	title := fm.listModeTitle()
	body := fm.viewListDetails(width, chrome)
	switch fm.mode {
	case fmEdit:
		if !fm.listPaneFocused() {
			title = "ADD FEED"
			if fm.editTarget != 0 {
				title = "EDIT FEED"
			}
			body = fm.viewEdit(width, height, chrome, styles)
		}
	case fmFolderEdit:
		title = "ADD FOLDER"
		if fm.folderEditTarget != 0 {
			title = "EDIT FOLDER"
		}
		body = fm.viewFolderEdit(width, height, chrome, styles)
	case fmImport:
		title = "IMPORT OPML"
		body = fm.viewImport(width, height, chrome)
	case fmConfirmDelete:
		title = fm.confirmDeleteTitle()
		body = fm.viewConfirmDelete(width, height, chrome)
	}
	section := clampView(renderManagerPaneSection(title, body, !fm.listPaneFocused(), chrome), width, height, chrome.baseBg)
	return lipgloss.NewStyle().Width(width).Height(height).Background(chrome.baseBg).Render(section)
}

func (fm FeedManager) viewListActions(width int, chrome managerChrome) string {
	if !fm.editable() {
		sourceName := fm.sourceName()
		if sourceName == "" {
			sourceName = "remote"
		}
		return renderManagerActionGroups(width, chrome,
			[]string{"enter", "browse", "esc", "back"},
			[]string{"mode", "browse-only", "source", sourceName},
		)
	}
	return renderManagerActionGroups(width, chrome,
		[]string{"a", "add feed", "n", "add folder", "e", "edit", "d", "delete"},
		[]string{"i", "import", "x", "export", "esc", "back"},
	)
}

func renderManagerInset(spaces int, s string) string {
	if spaces <= 0 {
		return s
	}
	return strings.Repeat(" ", spaces) + s
}

func (fm FeedManager) feedByID(id int64) *db.Feed {
	for i := range fm.feeds {
		if fm.feeds[i].ID == id {
			return &fm.feeds[i]
		}
	}
	return nil
}

func (fm FeedManager) folderByID(id int64) *db.Folder {
	for i := range fm.folders {
		if fm.folders[i].ID == id {
			return &fm.folders[i]
		}
	}
	return nil
}

func (fm FeedManager) addSourceLabel() string {
	if fm.addSourceIdx < 0 || fm.addSourceIdx >= len(fmAddSourceLabels) {
		return fmAddSourceLabels[0]
	}
	return fmAddSourceLabels[fm.addSourceIdx]
}

func (fm FeedManager) viewEdit(width, height int, chrome managerChrome, styles Styles) string {
	gap := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render("")
	detailFocused := !fm.listPaneFocused()
	contentRows := []string{}
	if fm.editTarget == 0 {
		contentRows = append(contentRows,
			renderManagerSection("Source", renderManagerPicker(width-3, fm.addSourceLabel(), detailFocused && fm.focusedField == fmFieldAddSource, chrome, styles), chrome),
			gap,
		)
	}
	if fm.editTarget == 0 && fm.addSourceIdx == fmAddSourceGReader {
		contentRows = append(contentRows,
			renderManagerSection("Title", renderTextInput(fm.titleInput, width-3, detailFocused && fm.focusedField == 0, false, chrome), chrome),
			gap,
			renderManagerSection("URL (optional)", renderTextInput(fm.urlInput, width-3, detailFocused && fm.focusedField == 1, false, chrome), chrome),
			gap,
			renderManagerSection("API URL", renderTextInput(fm.greaderURLInput, width-3, detailFocused && fm.focusedField == fmFieldGReaderURL, false, chrome), chrome),
			gap,
			renderManagerSection("Login", renderTextInput(fm.greaderLoginInput, width-3, detailFocused && fm.focusedField == fmFieldGReaderLogin, false, chrome), chrome),
			gap,
			renderManagerSection("Password", renderTextInput(fm.greaderPasswordInput, width-3, detailFocused && fm.focusedField == fmFieldGReaderPassword, true, chrome), chrome),
		)
	} else {
		contentRows = append(contentRows,
			renderManagerSection("Title", renderTextInput(fm.titleInput, width-3, detailFocused && fm.focusedField == 0, false, chrome), chrome),
			gap,
			renderManagerSection("URL", renderTextInput(fm.urlInput, width-3, detailFocused && fm.focusedField == 1, false, chrome), chrome),
			gap,
			renderManagerSection("Folder", renderManagerPicker(width-3, fm.folderOptions()[fm.folderCursor], detailFocused && fm.focusedField == 2, chrome, styles), chrome),
		)
		if fm.showNewFolder {
			contentRows = append(contentRows,
				gap,
				renderManagerSection("New", renderTextInput(fm.newFolderInput, width-3, detailFocused && fm.focusedField == 3, false, chrome), chrome),
			)
		}
		if fm.shouldShowColorPicker() {
			contentRows = append(contentRows,
				gap,
				renderManagerSection("Color", renderManagerColorPicker(width-3, fm.displayedColorOption(), detailFocused && fm.focusedField == 4, chrome, styles), chrome),
			)
		}
	}
	content := lipgloss.JoinVertical(lipgloss.Left, contentRows...)
	return lipgloss.NewStyle().Background(chrome.baseBg).Width(width).PaddingLeft(2).Render(content)
}

func (fm FeedManager) viewFolderEdit(width, height int, chrome managerChrome, styles Styles) string {
	gap := lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render("")
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		renderManagerSection("Name", renderTextInput(fm.titleInput, width-3, fm.focusedField == 0, false, chrome), chrome),
		gap,
		renderManagerSection("Color", renderManagerColorPicker(width-3, fm.currentColorOption(), fm.focusedField == 4, chrome, styles), chrome),
	)
	return lipgloss.NewStyle().Background(chrome.baseBg).Width(width).PaddingLeft(2).Render(content)
}

func (fm FeedManager) viewImport(width, height int, chrome managerChrome) string {
	return lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render(
		renderManagerSection("PATH", renderManagerInput(width-1, fm.importInput.Value(), "PATH TO OPML FILE...", true, chrome), chrome),
	)
}

func (fm FeedManager) viewConfirmDelete(width, height int, chrome managerChrome) string {
	row := fm.selectedRow()
	if row == nil {
		return renderManagerPanel(width, "NO SELECTION", chrome)
	}
	name := ""
	warning := ""
	if row.kind == fmRowFolder {
		if folder := fm.folderByID(row.folderID); folder != nil {
			name = folder.Name
			warning = "FEEDS IN THIS FOLDER WILL BE KEPT AND MOVED TO UNCATEGORIZED."
		}
	} else if feed := fm.feedByID(row.feedID); feed != nil {
		name = feed.Title
		warning = "ALL ARTICLES FROM THIS FEED WILL BE REMOVED."
	}
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		renderManagerSection("TARGET", renderManagerPanel(width, strings.ToUpper(truncate(name, width-4)), chrome), chrome),
		lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Background(chrome.baseBg).Foreground(lipgloss.Color("#f7768e")).Bold(true).Width(max(12, width)).Render("WARNING"),
			chrome.body.Width(max(12, width)).Render(warning),
		),
	)
	return lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render(content)
}

func (fm FeedManager) viewHints(width int, chrome managerChrome) string {
	if fm.busy {
		return renderManagerActions(width, chrome, "working", strings.ToLower(fm.busyMsg))
	}
	switch fm.mode {
	case fmEdit:
		if fm.paneFocus == fmPaneList {
			return renderManagerActions(width, chrome,
				"↑/↓", "browse list",
				"enter", "edit form",
				"esc", "cancel",
			)
		}
		enterLabel := "save feed"
		pickLabel := "pick"
		if fm.editTarget == 0 {
			enterLabel = "add feed"
			if fm.addSourceIdx == fmAddSourceGReader && fm.greaderFeedURL() == "" {
				enterLabel = "load feeds"
			}
		}
		if fm.focusedField == fmFieldAddSource {
			enterLabel = "toggle source"
			pickLabel = "toggle"
		}
		return renderManagerActions(width, chrome,
			"tab", "next field",
			"←/→", pickLabel,
			"enter", enterLabel,
			"esc", "cancel",
		)
	case fmFolderEdit:
		return renderManagerActions(width, chrome,
			"tab", "next field",
			"←/→", "color",
			"enter", "save folder",
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
	baseBg             lipgloss.Color
	surfaceBg          lipgloss.Color
	fieldBg            lipgloss.Color
	accent             lipgloss.Color
	accentFg           lipgloss.Color
	highlight          lipgloss.Color
	border             lipgloss.Color
	text               lipgloss.Color
	muted              lipgloss.Color
	header             lipgloss.Style
	sectionLabel       lipgloss.Style
	sectionLabelActive lipgloss.Style
	body               lipgloss.Style
	panel              lipgloss.Style
	panelSelected      lipgloss.Style
	key                lipgloss.Style
	keyLabel           lipgloss.Style
	statusBar          lipgloss.Style
}

func newManagerChrome(width int, t Theme) managerChrome {
	baseBg := modalSurface(t)
	surfaceDelta := 0.04
	fieldDelta := 0.08
	if !isDark(baseBg) {
		surfaceDelta = -surfaceDelta
		fieldDelta = -fieldDelta
	}
	surfaceBg := adjustLightness(baseBg, surfaceDelta)
	fieldBg := adjustLightness(baseBg, fieldDelta)
	accent := t.BorderFocus
	if accent == "" {
		accent = t.OverlayBorder
	}
	if accent == "" {
		accent = t.Border
	}
	accentFg := contrastFg(accent)
	text := readableText(t.Fg, baseBg, 4.5)
	muted := mutedText(text, baseBg)
	highlight := accent
	border := t.OverlayBorder
	if border == "" {
		border = t.Border
	}
	if border == "" {
		border = accent
	}

	return managerChrome{
		baseBg:    baseBg,
		surfaceBg: surfaceBg,
		fieldBg:   fieldBg,
		accent:    accent,
		accentFg:  accentFg,
		highlight: highlight,
		border:    border,
		text:      text,
		muted:     muted,
		header: lipgloss.NewStyle().
			Width(width).
			Background(accent).
			Foreground(accentFg).
			Bold(true).
			Padding(0, 1),
		sectionLabel: lipgloss.NewStyle().
			Background(baseBg).
			Foreground(muted).
			Padding(0, 1).
			Bold(true),
		sectionLabelActive: lipgloss.NewStyle().
			Background(accent).
			Foreground(accentFg).
			Padding(0, 1).
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
			Foreground(accentFg).
			Bold(true).
			Padding(0, 1),
		keyLabel: lipgloss.NewStyle().
			Background(baseBg).
			Foreground(muted),
		statusBar: lipgloss.NewStyle().
			Width(width).
			Background(surfaceBg).
			Foreground(readableText(accent, surfaceBg, 3)).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(border).
			Padding(0, 1),
	}
}

func renderManagerHeader(title string, width int, chrome managerChrome) string {
	gap := max(0, width-lipgloss.Width(title)-2)
	return chrome.header.Render(title + strings.Repeat(" ", gap))
}

func renderManagerSection(label, body string, chrome managerChrome) string {
	w := lipgloss.Width(body)
	styledLabel := chrome.sectionLabel.Width(w).Render(label)
	return lipgloss.JoinVertical(lipgloss.Left, styledLabel, body)
}

func renderManagerPaneSection(label, body string, focused bool, chrome managerChrome) string {
	w := lipgloss.Width(body)
	style := chrome.sectionLabel
	if focused {
		style = chrome.sectionLabelActive
	}
	styledLabel := style.Width(w).Render(label)
	return lipgloss.JoinVertical(lipgloss.Left, styledLabel, body)
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

func renderTextInput(input textinput.Model, width int, focused bool, compactSecretPreview bool, chrome managerChrome) string {
	// Layout: border(1) | pad(1) | content(contentW) | pad(1)  →  total = width
	// bubbles input.Width is the text-only area (prompt "> " = 2 chars excluded).
	fieldBg := chrome.fieldBg
	contentW := max(1, width-3)

	input.Width = max(1, contentW-2)
	input.PromptStyle = lipgloss.NewStyle().Background(fieldBg).Foreground(chrome.accent).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Background(fieldBg).Foreground(chrome.text)
	placeholderFg := chrome.muted
	if focused {
		placeholderFg = chrome.accent
	}
	input.PlaceholderStyle = lipgloss.NewStyle().Background(fieldBg).Foreground(placeholderFg)
	input.Cursor.Style = lipgloss.NewStyle().Background(chrome.accent).Foreground(chrome.accentFg)
	input.Cursor.TextStyle = lipgloss.NewStyle().Background(chrome.accent).Foreground(chrome.accentFg)

	rendered := ""
	if compactSecretPreview && !focused && input.Value() != "" {
		preview := maskedPreview(input.Value(), 20)
		rendered = lipgloss.NewStyle().Background(fieldBg).Foreground(chrome.muted).Render(preview)
	} else {
		// bubbles pads input.View() with plain un-styled spaces to fill input.Width.
		// Those spaces carry no ANSI bg code, so they show terminal bg.
		// Strip them, then re-pad to contentW with explicit fieldBg-coloured spaces.
		rendered = strings.TrimRight(input.View(), " ")
	}
	if gap := contentW - lipgloss.Width(rendered); gap > 0 {
		rendered += lipgloss.NewStyle().Background(fieldBg).Render(strings.Repeat(" ", gap))
	}

	// Explicit bg-coloured padding on each side keeps the full row covered.
	pad := lipgloss.NewStyle().Background(fieldBg).Render(" ")
	inner := pad + rendered + pad

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

func renderSecretSummary(value string, width int, chrome managerChrome) string {
	if width <= 0 {
		return ""
	}

	badgeStyle := lipgloss.NewStyle().
		Background(chrome.surfaceBg).
		Foreground(chrome.muted).
		Width(7).
		Align(lipgloss.Center)

	detailStyle := lipgloss.NewStyle().
		Background(chrome.baseBg).
		Foreground(chrome.muted)

	badgeText := "empty"
	detailText := "not set"
	if value != "" {
		badgeText = "saved"
		detailText = fmt.Sprintf("%d chars • id %s", len([]rune(value)), secretFingerprint(value))
	}

	badge := badgeStyle.Render(badgeText)
	detailW := max(1, width-lipgloss.Width(badge))
	detail := detailStyle.Width(detailW).Render("  " + truncate(detailText, max(1, detailW-2)))
	return badge + detail
}

func renderSecretEditor(input textinput.Model, width int, chrome managerChrome) string {
	fieldBg := chrome.fieldBg
	contentW := max(1, width-4)

	input.Width = max(1, contentW-2)
	input.PromptStyle = lipgloss.NewStyle().Background(fieldBg).Foreground(chrome.accent).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Background(fieldBg).Foreground(chrome.text)
	input.PlaceholderStyle = lipgloss.NewStyle().Background(fieldBg).Foreground(chrome.accent)
	input.Cursor.Style = lipgloss.NewStyle().Background(chrome.accent).Foreground(chrome.accentFg)
	input.Cursor.TextStyle = lipgloss.NewStyle().Background(chrome.accent).Foreground(chrome.accentFg)

	rendered := strings.TrimRight(input.View(), " ")
	rendered = ansi.Truncate(rendered, contentW, "")
	if gap := contentW - lipgloss.Width(rendered); gap > 0 {
		rendered += lipgloss.NewStyle().Background(fieldBg).Render(strings.Repeat(" ", gap))
	}

	inner := lipgloss.NewStyle().
		Background(fieldBg).
		Padding(0, 1).
		Render(rendered)

	return lipgloss.NewStyle().
		Background(fieldBg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(chrome.accent).
		BorderBackground(fieldBg).
		Render(inner)
}

func maskedPreview(value string, limit int) string {
	maskCount := len([]rune(value))
	if maskCount == 0 {
		return ""
	}
	if limit < 1 {
		limit = 1
	}
	if maskCount <= limit {
		return strings.Repeat("●", maskCount)
	}
	return strings.Repeat("●", limit) + "…"
}

func secretFingerprint(value string) string {
	if value == "" {
		return ""
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%06x", h.Sum32())[:6]
}

func renderManagerInput(width int, value, placeholder string, focused bool, chrome managerChrome) string {
	textW := max(1, width-1)
	inputBg := chrome.fieldBg
	dimBg := chrome.surfaceBg
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

func renderManagerPicker(width int, value string, focused bool, chrome managerChrome, styles Styles) string {
	textW := max(1, width-1)
	bg := chrome.surfaceBg
	fg := chrome.text
	accentFg := chrome.muted
	if focused {
		bg = terminalColorAsColor(styles.FeedItemSelectedFocused.GetBackground())
		fg = terminalColorAsColor(styles.FeedItemSelectedFocused.GetForeground())
		accentFg = fg
	}
	text := lipgloss.NewStyle().Background(bg).Foreground(fg)
	accent := lipgloss.NewStyle().Background(bg).Foreground(accentFg).Bold(true)
	line := accent.Render("◀ ") + text.Render(truncate(value, max(1, textW-4))) + accent.Render(" ▶")
	return lipgloss.NewStyle().Background(bg).Padding(0, 1).Render(clampView(line, textW, 1, bg))
}

func renderManagerColorPicker(width int, option folderColorOption, focused bool, chrome managerChrome, styles Styles) string {
	textW := max(1, width-1)
	bg := chrome.surfaceBg
	fg := chrome.text
	accentFg := chrome.muted
	if focused {
		bg = terminalColorAsColor(styles.FeedItemSelectedFocused.GetBackground())
		fg = terminalColorAsColor(styles.FeedItemSelectedFocused.GetForeground())
		accentFg = fg
	}
	accent := lipgloss.NewStyle().Background(bg).Foreground(accentFg).Bold(true)
	nameStyle := lipgloss.NewStyle().Background(bg).Foreground(fg)
	swatch := lipgloss.NewStyle().
		Background(option.Color).
		Foreground(contrastFg(option.Color)).
		Bold(true).
		Render(" " + strings.ToUpper(option.Name[:min(3, len(option.Name))]) + " ")
	line := accent.Render("◀ ") + swatch + lipgloss.NewStyle().Background(bg).Render(" ") + nameStyle.Render(option.Name) + accent.Render(" ▶")
	return lipgloss.NewStyle().Background(bg).Padding(0, 1).Render(clampView(line, textW, 1, bg))
}

func renderManagerRow(width int, title string, chrome managerChrome) string {
	textW := max(1, width-2)
	rowBg := chrome.baseBg
	row := padRight(truncate(title, textW), textW)
	rendered := lipgloss.NewStyle().
		Background(rowBg).
		Foreground(chrome.text).
		Padding(0, 1).
		Render(row)
	return clampView(rendered, width, 1, rowBg)
}

func renderManagerFeedRow(width int, title, color string, chrome managerChrome, icons bool) string {
	textW := max(1, width-2)
	rowBg := chrome.baseBg
	style := lipgloss.NewStyle().
		Background(rowBg).
		Padding(0, 1)
	icon := ""
	if icons {
		icon = "\U000f046b "
	}
	iconW := lipgloss.Width(icon)
	nameW := max(1, textW-iconW)
	name := truncate(title, nameW)
	iconStyle := lipgloss.NewStyle().Foreground(chrome.text).Background(rowBg)
	nameStyle := lipgloss.NewStyle().Foreground(chrome.text).Background(rowBg)
	if color != "" {
		accent := lipgloss.Color(color)
		iconStyle = iconStyle.Foreground(accent)
		nameStyle = nameStyle.Foreground(accent)
	}
	content := iconStyle.Render(icon) + nameStyle.Render(name)
	fillW := max(0, textW-lipgloss.Width(icon)-lipgloss.Width(name))
	filler := lipgloss.NewStyle().Background(rowBg).Render(strings.Repeat(" ", fillW))
	row := content + filler
	return clampView(style.Render(row), width, 1, rowBg)
}

func renderManagerFolderRow(width int, title, color string, collapsed bool, chrome managerChrome, styles Styles, selected, icons bool) string {
	textW := max(1, width-2)
	rowBg := chrome.baseBg
	style := lipgloss.NewStyle().
		Background(rowBg).
		Padding(0, 1)
	if selected {
		style = managerSelectedListStyle(styles)
	}
	contentBg := rowBg
	contentFg := chrome.text
	if selected {
		contentBg = terminalColorAsColor(style.GetBackground())
		contentFg = terminalColorAsColor(style.GetForeground())
	}
	icon := ""
	if icons {
		icon = "󰉋 "
		if collapsed {
			icon = "󰉖 "
		}
	}
	iconW := lipgloss.Width(icon)
	nameW := max(1, textW-iconW)
	name := truncate(title, nameW)
	if selected {
		row := padRight(icon+name, textW)
		return clampView(style.Render(row), width, 1, contentBg)
	}
	iconStyle := lipgloss.NewStyle().Foreground(contentFg).Background(contentBg)
	nameStyle := lipgloss.NewStyle().Foreground(contentFg).Background(contentBg)
	if color != "" {
		accent := lipgloss.Color(color)
		iconStyle = iconStyle.Foreground(accent)
		nameStyle = nameStyle.Foreground(accent)
	}
	content := iconStyle.Render(icon) + nameStyle.Render(name)
	fillW := max(0, textW-lipgloss.Width(icon)-lipgloss.Width(name))
	filler := lipgloss.NewStyle().Background(contentBg).Render(strings.Repeat(" ", fillW))
	row := content + filler
	return clampView(style.Render(row), width, 1, contentBg)
}

func renderManagerSelectedRow(width int, title string, chrome managerChrome, styles Styles) string {
	textW := max(1, width-2)
	row := padRight(truncate(title, textW), textW)
	bg := terminalColorAsColor(managerSelectedListStyle(styles).GetBackground())
	return clampView(managerSelectedListStyle(styles).Render(row), width, 1, bg)
}

func managerSelectedListStyle(styles Styles) lipgloss.Style {
	bg := terminalColorAsColor(styles.FeedItemSelectedFocused.GetBackground())
	if bg != "" {
		if isDark(bg) {
			bg = adjustLightness(bg, 0.08)
		} else {
			bg = adjustLightness(bg, -0.08)
		}
	}
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(readableText(styles.Theme.Fg, bg, 4.5)).
		Bold(true).
		Padding(0, 1)
}

func renderManagerSourceLine(width int, value string, chrome managerChrome) string {
	return lipgloss.NewStyle().
		Width(width).
		Background(chrome.surfaceBg).
		Foreground(chrome.accent).
		Render(padRight(value, width))
}

func renderManagerActionGroups(width int, chrome managerChrome, primaryPairs, secondaryPairs []string) string {
	rows := []string{renderManagerActions(width, chrome, primaryPairs...)}
	if len(secondaryPairs) > 0 {
		rows = append(rows, renderManagerActions(width, chrome, secondaryPairs...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
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

func (fm FeedManager) folderFeedCount(folderID int64) int {
	total := 0
	for _, feed := range fm.feeds {
		if feed.FolderID == folderID {
			total++
		}
	}
	return total
}

func (fm FeedManager) folderUnreadCount(folderID int64) int64 {
	var total int64
	for _, feed := range fm.feeds {
		if feed.FolderID == folderID {
			total += feed.UnreadCount
		}
	}
	return total
}

func (fm FeedManager) confirmDeleteTitle() string {
	if row := fm.selectedRow(); row != nil && row.kind == fmRowFolder {
		return "DELETE FOLDER"
	}
	return "DELETE FEED"
}
