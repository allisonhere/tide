package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tide/internal/db"
)

var errTest = errors.New("boom")

// newStarModel builds a model sitting on a normal feed with two loaded
// articles, one of them already saved.
func newStarModel() Model {
	m := Model{
		width:  100,
		height: 30,
		styles: BuildStyles(CatppuccinMocha, "comfortable"),
		keys:   DefaultKeys,
		cfg:    config.DefaultConfig(),
		feeds:  []db.Feed{{ID: 1, Title: "Feed One", URL: "https://example.com/feed"}},
		articles: []db.Article{
			{ID: 1, FeedID: 1, Title: "Saved One", PublishedAt: unixTestTime(1710000200), Starred: true},
			{ID: 2, FeedID: 1, Title: "Plain Two", PublishedAt: unixTestTime(1710000100)},
		},
		collapsedFolders: map[int64]bool{},
		greaderStreams:   map[int64]string{},
		focused:          paneArticles,
	}
	m.rebuildSidebar()
	m.selectSidebarFeed(1)
	m.applyFilter()
	return m
}

func TestSidebarPinsSavedRowAboveFeedTree(t *testing.T) {
	m := newStarModel()
	m.folders = []db.Folder{{ID: 10, Name: "Tech"}}
	m.feeds = []db.Feed{{ID: 1, Title: "Feed One", FolderID: 10}}
	m.rebuildSidebar()

	if len(m.sidebarRows) == 0 || m.sidebarRows[0].kind != rowKindSaved {
		t.Fatalf("expected Saved to be the first sidebar row, got %+v", m.sidebarRows)
	}
	// Exactly one, and it survives a rebuild rather than accumulating.
	m.rebuildSidebar()
	saved := 0
	for _, row := range m.sidebarRows {
		if row.kind == rowKindSaved {
			saved++
		}
	}
	if saved != 1 {
		t.Fatalf("expected exactly one Saved row after rebuild, got %d", saved)
	}
}

func TestSavedRowIsNeitherFeedNorFolder(t *testing.T) {
	m := newStarModel()
	m.sidebarCursor = 0

	if !m.savedSelected() {
		t.Fatal("expected cursor on row 0 to be the Saved row")
	}
	if f := m.selectedFeed(); f != nil {
		t.Fatalf("expected Saved row to report no selected feed, got %+v", f)
	}
	if _, ok := m.selectedFolderID(); ok {
		t.Fatal("expected Saved row to report no selected folder")
	}
	// Enter must not try to collapse it like a folder.
	if m.toggleSelectedFolder() {
		t.Fatal("expected Saved row not to be collapsible")
	}
	kind, id := m.currentSidebarSelection()
	if kind != rowKindSaved || id != savedFeedID {
		t.Fatalf("currentSidebarSelection = (%v, %d), want (rowKindSaved, savedFeedID)", kind, id)
	}
}

func TestMovingToSavedRowLoadsStarredArticles(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := newStarModel()
	m.db = database
	m.focused = paneFeeds
	m.sidebarCursor = 1 // the feed row

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(Model)
	if !m.savedSelected() {
		t.Fatal("expected moving up from the first feed to land on Saved")
	}
	if cmd == nil {
		t.Fatal("expected selecting Saved to issue an article load")
	}
	msg, ok := cmd().(ArticlesLoadedMsg)
	if !ok {
		t.Fatalf("expected ArticlesLoadedMsg, got %T", cmd())
	}
	if msg.FeedID != savedFeedID {
		t.Fatalf("expected the saved sentinel feed id, got %d", msg.FeedID)
	}
}

// Article loads are async, so a result must only be applied if the sidebar is
// still on what asked for it.
func TestArticlesLoadIsCurrentGuardsSavedSentinel(t *testing.T) {
	m := newStarModel()

	m.sidebarCursor = 0 // Saved
	if !m.articlesLoadIsCurrent(savedFeedID) {
		t.Fatal("expected a saved load to apply while Saved is selected")
	}
	if m.articlesLoadIsCurrent(1) {
		t.Fatal("expected a feed load to be dropped while Saved is selected")
	}

	m.selectSidebarFeed(1)
	if m.articlesLoadIsCurrent(savedFeedID) {
		t.Fatal("expected a saved load to be dropped while a feed is selected")
	}
	if !m.articlesLoadIsCurrent(1) {
		t.Fatal("expected the selected feed's load to apply")
	}
}

func TestToggleStarKeyIssuesStarCommand(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	for _, k := range []string{"*", "b"} {
		t.Run(k, func(t *testing.T) {
			m := newStarModel()
			m.db = database
			m.articleCursor = 1 // the unstarred article

			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
			if cmd == nil {
				t.Fatalf("expected %q to issue a star command", k)
			}
			msg, ok := cmd().(ArticleStarUpdatedMsg)
			if !ok {
				t.Fatalf("expected ArticleStarUpdatedMsg, got %T", cmd())
			}
			if msg.ArticleID != 2 {
				t.Fatalf("expected the selected article, got %d", msg.ArticleID)
			}
			if msg.WasStarred || !msg.Starred {
				t.Fatalf("expected an unstarred article to become starred, got was=%v now=%v", msg.WasStarred, msg.Starred)
			}
		})
	}
}

func TestToggleStarUnstarsAnAlreadyStarredArticle(t *testing.T) {
	database, err := db.Open()
	if err != nil {
		t.Skip("cannot open DB:", err)
	}
	defer database.Close()

	m := newStarModel()
	m.db = database
	m.articleCursor = 0 // already starred

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("*")})
	if cmd == nil {
		t.Fatal("expected a star command")
	}
	msg := cmd().(ArticleStarUpdatedMsg)
	if !msg.WasStarred || msg.Starred {
		t.Fatalf("expected toggle to unstar, got was=%v now=%v", msg.WasStarred, msg.Starred)
	}
}

// Remote articles are never written to the articles table, so there is no row
// to carry the flag. Refusing loudly beats appearing to save and losing it.
func TestToggleStarRefusedForRemoteArticles(t *testing.T) {
	m := newStarModel()
	m.greaderStreams = map[int64]string{1: "stream/1"}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("*")})
	m = next.(Model)
	if !m.statusErr {
		t.Fatal("expected an error status for a remote article")
	}
	if !containsString(m.statusMsg, "remote") {
		t.Fatalf("expected the status to explain the refusal, got %q", m.statusMsg)
	}
}

func TestStarUpdateFlipsFlagAndBadgeInFeedView(t *testing.T) {
	m := newStarModel()
	m.savedCount = 1

	next, _ := m.Update(ArticleStarUpdatedMsg{ArticleID: 2, WasStarred: false, Starred: true})
	m = next.(Model)

	if len(m.articles) != 2 {
		t.Fatalf("expected the feed view to keep every article, got %d", len(m.articles))
	}
	if !m.articles[1].Starred {
		t.Fatal("expected the saved flag to be applied in place")
	}
	if m.savedCount != 2 {
		t.Fatalf("expected the Saved badge to increment, got %d", m.savedCount)
	}
}

// The Saved view is defined by the flag being set, so clearing it must drop the
// row instead of leaving a phantom entry behind.
func TestUnstarringInSavedViewRemovesTheRow(t *testing.T) {
	m := newStarModel()
	m.sidebarCursor = 0 // Saved
	m.savedCount = 2
	m.articles = []db.Article{
		{ID: 1, FeedID: 1, Title: "Saved One", Starred: true},
		{ID: 2, FeedID: 1, Title: "Saved Two", Starred: true},
	}
	m.applyFilter()
	m.articleCursor = 1

	next, _ := m.Update(ArticleStarUpdatedMsg{ArticleID: 2, WasStarred: true, Starred: false})
	m = next.(Model)

	if len(m.filteredArticles) != 1 {
		t.Fatalf("expected the unstarred row to leave the Saved view, got %d", len(m.filteredArticles))
	}
	if m.filteredArticles[0].ID != 1 {
		t.Fatalf("expected the remaining row to be the still-saved article, got %d", m.filteredArticles[0].ID)
	}
	if m.articleCursor != 0 {
		t.Fatalf("expected the cursor to clamp into range, got %d", m.articleCursor)
	}
	if m.savedCount != 1 {
		t.Fatalf("expected the Saved badge to decrement, got %d", m.savedCount)
	}
}

func TestUnstarringLastArticleEmptiesSavedView(t *testing.T) {
	m := newStarModel()
	m.sidebarCursor = 0
	m.savedCount = 1
	m.articles = []db.Article{{ID: 1, FeedID: 1, Title: "Only", Starred: true}}
	m.applyFilter()

	next, _ := m.Update(ArticleStarUpdatedMsg{ArticleID: 1, WasStarred: true, Starred: false})
	m = next.(Model)

	if len(m.filteredArticles) != 0 {
		t.Fatalf("expected an empty Saved view, got %d", len(m.filteredArticles))
	}
	if m.savedCount != 0 {
		t.Fatalf("expected a zero Saved badge, got %d", m.savedCount)
	}
	if m.contentArticleID != 0 {
		t.Fatal("expected the content pane to clear when nothing is left")
	}
}

func TestStarUpdateErrorSurfacesAndLeavesStateAlone(t *testing.T) {
	m := newStarModel()
	m.savedCount = 1

	next, _ := m.Update(ArticleStarUpdatedMsg{ArticleID: 2, Starred: true, Err: errTest})
	m = next.(Model)

	if !m.statusErr {
		t.Fatal("expected a failed star to set an error status")
	}
	if m.savedCount != 1 {
		t.Fatalf("expected a failed star not to move the badge, got %d", m.savedCount)
	}
	if m.articles[1].Starred {
		t.Fatal("expected a failed star not to flip the flag")
	}
}

// "save" is tide's word for persisting an edit — a feed, a folder, OPML, or
// settings all report "save failed: …". A starring failure must not be
// indistinguishable from those, and starring status messages must not borrow the
// vocabulary either.
func TestStarStatusMessagesDoNotCollideWithEditSaves(t *testing.T) {
	feedSaveErr := func() string {
		m := newStarModel()
		next, _ := m.Update(FeedSavedMsg{Err: errTest})
		return next.(Model).statusMsg
	}()

	cases := []struct {
		name string
		msg  tea.Msg
	}{
		{"star failed", ArticleStarUpdatedMsg{ArticleID: 2, Starred: true, Err: errTest}},
		{"starred", ArticleStarUpdatedMsg{ArticleID: 2, Starred: true}},
		{"unstarred", ArticleStarUpdatedMsg{ArticleID: 1, WasStarred: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newStarModel()
			next, _ := m.Update(tc.msg)
			got := next.(Model).statusMsg

			if got == feedSaveErr {
				t.Fatalf("starring status %q is identical to the feed-save status", got)
			}
			if containsString(got, "save") {
				t.Fatalf("expected starring status to avoid save vocabulary, got %q", got)
			}
			if !containsString(got, "star") {
				t.Fatalf("expected starring status to name the action, got %q", got)
			}
		})
	}

	// The remote-feed refusal is on the key path rather than the message path.
	m := newStarModel()
	m.greaderStreams = map[int64]string{1: "stream/1"}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("*")})
	if got := next.(Model).statusMsg; containsString(got, "save") || !containsString(got, "star") {
		t.Fatalf("expected the remote refusal to use star vocabulary, got %q", got)
	}
}

func TestSavedSidebarRowRendersLabelAndCount(t *testing.T) {
	m := newStarModel()
	m.savedCount = 3

	row := ansi.Strip(m.renderSavedRow(false, 30))
	if !containsString(row, savedRowLabel) {
		t.Fatalf("expected the Saved label, got %q", row)
	}
	if !containsString(row, "(3)") {
		t.Fatalf("expected the saved count badge, got %q", row)
	}

	m.savedCount = 0
	if row = ansi.Strip(m.renderSavedRow(false, 30)); containsString(row, "(0)") {
		t.Fatalf("expected no badge when nothing is saved, got %q", row)
	}
}

func TestArticleListMarksStarredArticles(t *testing.T) {
	m := newStarModel()

	pane := ansi.Strip(m.renderArticlesPane())
	if !containsString(pane, m.styles.StarGlyph()) {
		t.Fatalf("expected the saved article to carry a star, got %q", pane)
	}

	// With nothing saved the glyph must not appear at all.
	m.articles[0].Starred = false
	m.applyFilter()
	if pane = ansi.Strip(m.renderArticlesPane()); containsString(pane, m.styles.StarGlyph()) {
		t.Fatalf("expected no star when nothing is saved, got %q", pane)
	}
}

// The whole point of the dedicated column: a starred row and an unstarred row must
// start their titles at the same screen column.
func TestStarColumnKeepsTitlesAligned(t *testing.T) {
	m := newStarModel() // article 0 is saved, article 1 is not
	m.articles[0].Title = "Saved title"
	m.articles[1].Title = "Plain title"
	m.applyFilter()

	lines := strings.Split(ansi.Strip(m.renderArticlesPane()), "\n")
	savedCol, plainCol := -1, -1
	for _, line := range lines {
		if i := strings.Index(line, "Saved title"); i >= 0 {
			savedCol = lipgloss.Width(line[:i])
		}
		if i := strings.Index(line, "Plain title"); i >= 0 {
			plainCol = lipgloss.Width(line[:i])
		}
	}
	if savedCol < 0 || plainCol < 0 {
		t.Fatalf("expected both titles to render, got saved=%d plain=%d", savedCol, plainCol)
	}
	if savedCol != plainCol {
		t.Fatalf("expected titles to align, saved starts at %d and plain at %d", savedCol, plainCol)
	}
}

// Ported from tidemail's TestRenderArticleRowStarColumnKeepsWidth: the column
// must not change the row's total width.
func TestRenderArticleRowStarColumnKeepsWidth(t *testing.T) {
	starred := renderArticleRow("· ", "★ ", "Subject", "2m", 24)
	plain := renderArticleRow("· ", "  ", "Subject", "2m", 24)

	if !strings.Contains(starred, "★") {
		t.Fatalf("expected star glyph in starred row, got %q", starred)
	}
	if strings.Contains(plain, "★") {
		t.Fatalf("did not expect star glyph in non-starred row, got %q", plain)
	}
	if a, b := lipgloss.Width(starred), lipgloss.Width(plain); a != b || a != 24 {
		t.Fatalf("expected both rows width 24, got starred=%d plain=%d", a, b)
	}
}

// Regression: the star used to be rendered as a nested colored run inside the
// row's own style. lipgloss does not re-open an outer style after an inner
// reset, so everything after the glyph lost the row's text color and saved
// tint. Each segment must now repeat the row's background.
func TestStarredRowKeepsItsTintAfterTheStar(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	m := newStarModel()
	m.articles[0].Title = "Saved title"
	m.articles[1].Starred = false
	m.applyFilter()
	m.articleCursor = 1 // keep the cursor off the saved row so the tint shows

	var savedLine string
	for _, line := range strings.Split(m.renderArticlesPane(), "\n") {
		if strings.Contains(ansi.Strip(line), "Saved title") {
			savedLine = line
			break
		}
	}
	if savedLine == "" {
		t.Fatal("expected the saved row to render")
	}

	tint := starredRowBackground(m.styles.Theme)
	if tint == m.styles.Theme.Bg {
		t.Fatal("expected this theme to produce a distinct saved tint")
	}
	// The background sequence must be re-stated for the title, i.e. it appears
	// after the point where the star's own run ends.
	bgSeq := backgroundSeq(tint)
	idx := strings.Index(savedLine, "Saved title")
	if idx < 0 {
		t.Fatalf("expected the title in the styled row, got %q", savedLine)
	}
	if !strings.Contains(savedLine[:idx], bgSeq) {
		t.Fatalf("expected the tint %q to be re-applied before the title, got %q", bgSeq, savedLine)
	}
	// The title must not be left bare: the last escape before it has to set the
	// tint, not be a reset.
	lastOpen := strings.LastIndex(savedLine[:idx], bgSeq)
	lastReset := strings.LastIndex(savedLine[:idx], "\x1b[0m")
	if lastReset > lastOpen {
		t.Fatalf("expected no dangling reset between the star and the title, got %q", savedLine)
	}
}

// Regression: the star's segment styling must not re-apply the row style's
// padding. In the comfortable density that padding is a trailing spacer line,
// so rendering three segments stacked three spacers and blew up the pane's
// line count.
func TestStarredRowOccupiesOneRowInEveryDensity(t *testing.T) {
	for _, density := range []string{"compact", "comfortable"} {
		t.Run(density, func(t *testing.T) {
			m := newStarModel()
			m.styles = BuildStyles(CatppuccinMocha, density)
			m.articles[0].Title = "Saved title"
			m.articles[1].Title = "Plain title"
			m.applyFilter()

			lines := strings.Split(ansi.Strip(m.renderArticlesPane()), "\n")
			savedAt, plainAt := -1, -1
			for i, line := range lines {
				if strings.Contains(line, "Saved title") {
					savedAt = i
				}
				if strings.Contains(line, "Plain title") {
					plainAt = i
				}
			}
			if savedAt < 0 || plainAt < 0 {
				t.Fatalf("expected both rows to render, got saved=%d plain=%d", savedAt, plainAt)
			}
			// Whatever the per-row stride is, the saved row must use the same
			// one as the unstarred row that follows it.
			stride := plainAt - savedAt
			want := 1
			if density == "comfortable" {
				want = 2
			}
			if stride != want {
				t.Fatalf("expected a %d-line stride in %s density, got %d", want, density, stride)
			}
		})
	}
}

// backgroundSeq is the SGR sequence lipgloss emits for a truecolor background.
func backgroundSeq(c lipgloss.Color) string {
	r, g, b, ok := hexToRGB(c)
	if !ok {
		return ""
	}
	return fmt.Sprintf("48;2;%d;%d;%d", int(r*255+0.5), int(g*255+0.5), int(b*255+0.5))
}

// The retro terminal themes are deliberately monochrome, the same reason
// folderColor drops folder accents there.
func TestRetroTerminalThemesDropStarAccentAndTint(t *testing.T) {
	for _, theme := range []Theme{VT100, VT52} {
		t.Run(string(theme.Name), func(t *testing.T) {
			if got := starColor(theme); got != "" {
				t.Fatalf("expected no star accent on %s, got %q", theme.Name, got)
			}
			if got := starredRowBackground(theme); got != theme.Bg {
				t.Fatalf("expected no row tint on %s, got %q", theme.Name, got)
			}

			m := newStarModel()
			m.styles = BuildStyles(theme, "comfortable")
			unread, read, selected, _, _, _ := m.articleRowStyles()
			_ = unread
			tinted := applyArticleRowState(read, selected, true, false, theme)
			if tinted.GetBackground() != read.GetBackground() {
				t.Fatalf("expected a saved row to keep the plain background on %s", theme.Name)
			}
			// The glyph still appears, just without color.
			if star := m.articleRowStar(true); lipgloss.Width(star) != 2 {
				t.Fatalf("expected a 2-cell star column on %s, got %q", theme.Name, star)
			}
		})
	}
}

// The cursor row reads as selected first; the saved tint must not override it.
func TestCursorRowWinsOverStarredTint(t *testing.T) {
	m := newStarModel()
	_, read, selected, _, _, _ := m.articleRowStyles()

	onCursor := applyArticleRowState(read, selected, true, true, m.styles.Theme)
	if onCursor.GetBackground() != selected.GetBackground() {
		t.Fatal("expected the selected style to win on the cursor row")
	}

	offCursor := applyArticleRowState(read, selected, true, false, m.styles.Theme)
	if offCursor.GetBackground() == read.GetBackground() {
		t.Fatal("expected a saved row off the cursor to pick up the tint")
	}
}

func TestSearchResultStarIsAFixedColumn(t *testing.T) {
	m := newStarModel()
	chrome := newManagerChrome(60, m.styles.Theme, m.styles.PlainUI)

	starred := m.searchResultStar(true, chrome)
	plain := m.searchResultStar(false, chrome)

	if lipgloss.Width(starred) != 2 || lipgloss.Width(plain) != 2 {
		t.Fatalf("expected a 2-cell column either way, got starred=%d plain=%d",
			lipgloss.Width(starred), lipgloss.Width(plain))
	}
	if !containsString(ansi.Strip(starred), m.styles.StarGlyph()) {
		t.Fatalf("expected the glyph in the starred slot, got %q", ansi.Strip(starred))
	}
	if containsString(ansi.Strip(plain), m.styles.StarGlyph()) {
		t.Fatalf("did not expect a glyph in the empty slot, got %q", ansi.Strip(plain))
	}
}

func TestSavedViewRetitlesArticlesPaneAndExplainsEmptiness(t *testing.T) {
	m := newStarModel()
	m.sidebarCursor = 0 // Saved
	m.articles = nil
	m.applyFilter()

	pane := ansi.Strip(m.renderArticlesPane())
	if !containsString(pane, savedRowLabel) {
		t.Fatalf("expected the pane to be titled %q, got %q", savedRowLabel, pane)
	}
	if !containsString(pane, "nothing starred yet") {
		t.Fatalf("expected an explanatory empty state, got %q", pane)
	}
	if containsString(pane, "no articles") {
		t.Fatalf("expected the Saved view not to use the generic empty text, got %q", pane)
	}
}
