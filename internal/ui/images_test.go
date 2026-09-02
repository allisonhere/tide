package ui

import (
	stdimage "image"
	"strings"
	"testing"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tide/internal/db"
	img "github.com/allisonhere/tide/internal/image"
)

func newImageTestModel(t *testing.T, enabled bool) Model {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Display.ArticleImages = enabled
	m := NewModel(nil, cfg, "test", false)
	return m
}

func TestImageReservedRows_ZeroWhenDisabled(t *testing.T) {
	m := newImageTestModel(t, false)
	m.image = imageState{articleID: 1, status: imgReady, rows: 8}
	if got := m.imageReservedRows(); got != 0 {
		t.Fatalf("reserved rows with feature off = %d, want 0", got)
	}
}

func TestImageReservedRows_ZeroWhenRendererUnsupported(t *testing.T) {
	m := newImageTestModel(t, true) // enabled, but renderer is still the noop
	m.image = imageState{articleID: 1, status: imgReady, rows: 8}
	if got := m.imageReservedRows(); got != 0 {
		t.Fatalf("reserved rows with noop renderer = %d, want 0", got)
	}
}

func TestImageReservedRows_TracksStatusAndHidden(t *testing.T) {
	m := newImageTestModel(t, true)
	m.EnableImages(img.Capability{Supported: true, Protocol: img.ProtocolKitty}, img.NewKittyRenderer())
	m.image = imageState{articleID: 7, url: "u", status: imgLoading, rows: 8}

	if got := m.imageReservedRows(); got != 0 {
		t.Fatalf("loading status should reserve 0 rows, got %d", got)
	}

	m.image.status = imgReady
	if got := m.imageReservedRows(); got != 8 {
		t.Fatalf("ready status should reserve rows=8, got %d", got)
	}

	m.imgHidden[7] = true
	if got := m.imageReservedRows(); got != 0 {
		t.Fatalf("hidden article should reserve 0 rows, got %d", got)
	}
}

func enableImageTestRenderer(m *Model) {
	m.EnableImages(
		img.Capability{Supported: true, Protocol: img.ProtocolKitty, CellW: 8, CellH: 16},
		img.NewKittyRenderer(),
	)
}

func TestNoteContentArticleForImage_ResetsOnChange(t *testing.T) {
	m := newImageTestModel(t, true)
	enableImageTestRenderer(&m)
	m.image = imageState{articleID: 1, url: "old", status: imgReady, rows: 8}
	m.imgDirty = false

	m.noteContentArticleForImage(db.Article{ID: 2, ImageURL: "new"})

	if m.image.articleID != 2 || m.image.url != "new" {
		t.Fatalf("image state not reset for new article: %+v", m.image)
	}
	if m.image.status != imgIdle || m.image.rows != 0 {
		t.Fatalf("image state not cleared: %+v", m.image)
	}
	if !m.imgDirty {
		t.Fatal("switching to an article with an image should mark the model image-dirty")
	}
}

func TestNoteContentArticleForImage_NoDirtyWhenInactive(t *testing.T) {
	m := newImageTestModel(t, true) // setting on, but renderer is still the noop
	m.imgDirty = false
	m.noteContentArticleForImage(db.Article{ID: 2, ImageURL: "new"})
	if m.imgDirty {
		t.Fatal("no renderer -> nothing to fetch -> must not mark dirty")
	}
}

func TestNoteContentArticleForImage_SameArticleRefetchesWhenIdle(t *testing.T) {
	m := newImageTestModel(t, true)
	enableImageTestRenderer(&m)
	// Sitting on article 5 with an un-loaded image (e.g. images were just
	// enabled while this article was already open).
	m.image = imageState{articleID: 5, url: "u", status: imgIdle}
	m.imgDirty = false

	m.noteContentArticleForImage(db.Article{ID: 5, ImageURL: "u2"})

	if m.image.url != "u2" {
		t.Fatalf("url should refresh to the latest value, got %q", m.image.url)
	}
	if !m.imgDirty {
		t.Fatal("landing on an article with an idle unfetched image should mark dirty")
	}
}

func TestNoteContentArticleForImage_SameArticleKeepsResidentImage(t *testing.T) {
	m := newImageTestModel(t, true)
	enableImageTestRenderer(&m)
	m.image = imageState{articleID: 5, url: "u", status: imgReady, rows: 6}
	m.imgDirty = false

	m.noteContentArticleForImage(db.Article{ID: 5, ImageURL: "u"})

	if m.image.status != imgReady || m.image.rows != 6 {
		t.Fatalf("same-article call should not disturb a ready image: %+v", m.image)
	}
	if m.imgDirty {
		t.Fatal("a ready image should not be re-fetched")
	}
}

func TestDrainImageCmd_ClearsDirtyAndBumpsGen(t *testing.T) {
	m := newImageTestModel(t, true)
	m.imgDirty = true
	startGen := m.imgGen

	next, _ := m.drainImageCmd(nil)
	mm := next.(Model)

	if mm.imgDirty {
		t.Fatal("drainImageCmd should clear imgDirty")
	}
	if mm.imgGen != startGen+1 {
		t.Fatalf("imgGen = %d, want %d", mm.imgGen, startGen+1)
	}
}

func solidImage(w, h int) stdimage.Image {
	return stdimage.NewRGBA(stdimage.Rect(0, 0, w, h))
}

func TestHandleArticleImageReady_DropsStaleResult(t *testing.T) {
	m := newImageTestModel(t, true)
	m.EnableImages(img.Capability{Supported: true, Protocol: img.ProtocolKitty, CellW: 8, CellH: 16},
		img.NewKittyRenderer())
	m.image = imageState{articleID: 10, url: "u", status: imgLoading}
	m.imgGen = 5

	// Result for a superseded generation.
	next, _ := m.handleArticleImageReady(articleImageReadyMsg{
		req:     img.Req{ArticleID: 10, Gen: 4},
		decoded: solidImage(400, 200),
	})
	mm := next.(Model)
	if mm.image.status == imgReady {
		t.Fatal("stale result must not be applied")
	}

	// Result for a different article.
	next, _ = m.handleArticleImageReady(articleImageReadyMsg{
		req:     img.Req{ArticleID: 999, Gen: 5},
		decoded: solidImage(400, 200),
	})
	if next.(Model).image.status == imgReady {
		t.Fatal("result for another article must not be applied")
	}
}

func TestHandleArticleImageReady_FailureIsSilent(t *testing.T) {
	m := newImageTestModel(t, true)
	m.EnableImages(img.Capability{Supported: true, Protocol: img.ProtocolKitty, CellW: 8, CellH: 16},
		img.NewKittyRenderer())
	m.image = imageState{articleID: 3, url: "u", status: imgLoading, rows: 8}
	m.imgGen = 1

	next, _ := m.handleArticleImageReady(articleImageReadyMsg{
		req: img.Req{ArticleID: 3, Gen: 1},
		err: img.ErrNotImage,
	})
	mm := next.(Model)
	if mm.image.status != imgFailed {
		t.Fatalf("status = %v, want imgFailed", mm.image.status)
	}
	if mm.imageReservedRows() != 0 {
		t.Fatal("a failed image must reserve no rows")
	}
}

func TestResizeCurrentImage_DowngradesWhenPaneTooShort(t *testing.T) {
	m := newImageTestModel(t, true)
	m.EnableImages(img.Capability{Supported: true, Protocol: img.ProtocolKitty, CellW: 8, CellH: 16},
		img.NewKittyRenderer())
	// Tiny terminal -> contentBodyHeight() is at its floor, no room for an image.
	m.width, m.height = 80, 10
	m.image = imageState{articleID: 1, url: "u", status: imgLoading, src: solidImage(1200, 800)}

	m.resizeCurrentImage()
	if m.image.status != imgFailed || m.image.rows != 0 {
		t.Fatalf("short pane should downgrade to no-image, got status=%v rows=%d", m.image.status, m.image.rows)
	}
}

func TestImageShouldShow_Gating(t *testing.T) {
	m := newImageTestModel(t, true)
	m.EnableImages(img.Capability{Supported: true, Protocol: img.ProtocolKitty, CellW: 8, CellH: 16},
		img.NewKittyRenderer())
	m.width, m.height = 120, 40
	m.contentArticleID = 42
	m.image = imageState{
		articleID: 42, status: imgReady, rows: 6, cols: 40,
		pix: stdimage.NewRGBA(stdimage.Rect(0, 0, 320, 96)),
	}
	m.viewport.SetContent("x")
	m.viewport.GotoTop()

	if !m.imageShouldShow() {
		t.Fatal("image should be on screen at top of its article with no overlay")
	}

	off := m
	off.viewport.SetContent("a\nb\nc\nd\ne\nf\ng\nh\ni\nj")
	off.viewport.Height = 3
	off.viewport.SetYOffset(4)
	if off.imageShouldShow() {
		t.Fatal("image must hide once scrolled past the top")
	}

	ov := m
	ov.overlay = overlayHelp
	if ov.imageShouldShow() {
		t.Fatal("image must hide while an overlay is open")
	}

	other := m
	other.contentArticleID = 99
	if other.imageShouldShow() {
		t.Fatal("image must not show over a different article")
	}
}

func TestImageDrawOrigin_MatchesLayout(t *testing.T) {
	m := newImageTestModel(t, true)
	m.width, m.height = 160, 50

	x, y := m.imageDrawOrigin()
	// x: feeds pane width, +1 for the 1-space body indent, +1 for 1-based cols.
	wantX := m.feedsPaneWidth() + 2
	// y: articles pane height rows, +1 content header, +imageBodyTopLine, +1 for 1-based.
	wantY := m.articlesPaneOuterHeight() + 1 + imageBodyTopLine + 1
	if x != wantX || y != wantY {
		t.Fatalf("imageDrawOrigin() = (%d,%d), want (%d,%d)", x, y, wantX, wantY)
	}
	if x < 1 || y < 1 {
		t.Fatal("draw origin must be 1-based and positive")
	}
}

func TestApplyLeadLayout_Modes(t *testing.T) {
	m := newImageTestModel(t, true)
	enableImageTestRenderer(&m)
	m.width, m.height = 160, 50
	m.contentArticleID = 1
	art := db.Article{ID: 1, FeedID: 0, Content: "one two three four"}
	m.image = imageState{
		articleID: 1, status: imgReady, rows: 3, cols: 20,
		pix: stdimage.NewRGBA(stdimage.Rect(0, 0, 160, 96)),
	}
	body := "L0\nL1\nL2\nL3\nL4"

	// Side-by-side + beside active: first `rows` lines are the image gutter plus
	// metadata; the body follows below the band.
	m.image.sideBySide, m.image.metaBeside = true, true
	got := m.applyLeadLayout(art, body)
	gutter := repeatSpace(1 + 20 + imageMetaGutter)
	lines := strings.Split(got, "\n")
	for i := 0; i < 3; i++ {
		if !strings.HasPrefix(lines[i], gutter) {
			t.Fatalf("beside layout: line %d = %q, want gutter prefix", i, lines[i])
		}
	}
	if !strings.HasSuffix(got, "\n\n"+body) {
		t.Fatalf("beside layout should leave one blank row between image and body: %q", got)
	}

	// Full-width band (narrow column / scrolled away): blank band reserves the
	// rows, metadata stacks below it.
	m.image.sideBySide = false
	got = m.applyLeadLayout(art, body)
	if !strings.HasPrefix(got, "\n\n\n") {
		t.Fatalf("band layout must reserve a blank band: %q", got)
	}
	if !strings.Contains(got, "unread") {
		t.Fatal("band layout must still stack the metadata block")
	}

	// No image (hidden): plain reading view, body untouched, no metadata block.
	m.imgHidden[1] = true
	if got = m.applyLeadLayout(art, body); got != body {
		t.Fatalf("hidden image must leave the body untouched: %q", got)
	}
}

func TestImageMetaColumnFits_NarrowFallsBack(t *testing.T) {
	m := newImageTestModel(t, true)
	m.width, m.height = 200, 50
	if !m.imageMetaColumnFits() {
		t.Fatal("a wide pane should allow the beside-the-image metadata column")
	}

	m.width, m.height = 50, 50 // narrow content column
	if m.imageMetaColumnFits() {
		t.Fatal("a narrow reading column must fall back to the full-width band")
	}
}

func TestWantMetaBeside_Gating(t *testing.T) {
	m := newImageTestModel(t, true)
	enableImageTestRenderer(&m)
	m.width, m.height = 160, 50
	m.contentArticleID = 9
	m.image = imageState{articleID: 9, status: imgReady, rows: 4, sideBySide: true}
	m.viewport.SetContent("x")
	m.viewport.GotoTop()

	if !m.wantMetaBeside() {
		t.Fatal("ready side-by-side image at top should want metadata beside")
	}

	scrolled := m
	scrolled.viewport.SetContent("a\nb\nc\nd\ne\nf\ng")
	scrolled.viewport.Height = 3
	scrolled.viewport.SetYOffset(2)
	if scrolled.wantMetaBeside() {
		t.Fatal("scrolled away -> must not want metadata beside")
	}

	m.image.sideBySide = false
	if m.wantMetaBeside() {
		t.Fatal("full-width band image must not want metadata beside")
	}
}

func TestSyncImageMetaLayout_Flip(t *testing.T) {
	m := newImageTestModel(t, true)
	enableImageTestRenderer(&m)
	m.width, m.height = 160, 50
	m.filteredArticles = []db.Article{{ID: 3, Title: "T", Content: "para one\n\npara two\n\npara three", ImageURL: "u"}}
	m.articleCursor = 0
	m.setViewportArticle(m.filteredArticles[0])
	m.image.status, m.image.sideBySide, m.image.rows, m.image.cols = imgReady, true, 3, 20
	m.viewport.GotoTop()

	if !m.syncImageMetaLayout() {
		t.Fatal("expected a layout flip to beside")
	}
	if !m.image.metaBeside {
		t.Fatal("metaBeside should now be true")
	}
	// Idempotent second call.
	if m.syncImageMetaLayout() {
		t.Fatal("no flip expected on the second call")
	}
}

func repeatSpace(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

func countNL(s string) int {
	n := 0
	for _, r := range s {
		if r == '\n' {
			n++
		}
	}
	return n
}

func TestResizeCurrentImage_ReservesCappedRows(t *testing.T) {
	m := newImageTestModel(t, true)
	m.EnableImages(img.Capability{Supported: true, Protocol: img.ProtocolKitty, CellW: 8, CellH: 16},
		img.NewKittyRenderer())
	m.width, m.height = 120, 60          // plenty of vertical room
	m.image.src = solidImage(4000, 4000) // very tall square

	m.resizeCurrentImage()
	if m.image.status != imgReady {
		t.Fatalf("status = %v, want imgReady", m.image.status)
	}
	if m.image.rows < 1 || m.image.rows > imageMaxRows {
		t.Fatalf("reserved rows %d outside [1,%d]", m.image.rows, imageMaxRows)
	}
	if m.image.resizedForWidth != m.contentBodyWidth() {
		t.Fatalf("resizedForWidth = %d, want %d", m.image.resizedForWidth, m.contentBodyWidth())
	}
}

func TestView_EmbedsKittyPlacementWhenImageReady(t *testing.T) {
	m := newImageTestModel(t, true)
	enableImageTestRenderer(&m)
	m.width, m.height = 160, 48
	m.filteredArticles = []db.Article{{ID: 1, Title: "Headline", Content: "first paragraph of body text\n\nsecond paragraph", ImageURL: "u"}}
	m.articleCursor = 0
	m.setViewportArticle(m.filteredArticles[0])
	m.image = imageState{
		articleID: 1, url: "u", status: imgReady, rows: 6, cols: 24,
		pix: stdimage.NewRGBA(stdimage.Rect(0, 0, 192, 96)),
	}
	m.viewport.GotoTop()

	out := m.View()
	if !strings.Contains(out, "\x1b_Ga=p,i=1974") {
		t.Fatal("View should embed a Kitty placement when the image is ready and at the top of its article")
	}
	if !strings.HasSuffix(out, "\x1b8") {
		t.Fatalf("View should end with the placement's cursor-restore, got tail %q", out[max(0, len(out)-16):])
	}
	// The escape rides the frame as a suffix, so it survives clampView / the
	// outer Render untouched.
	if strings.Count(out, "\x1b_Ga=p,i=1974") != 1 {
		t.Fatal("exactly one placement expected")
	}

	// Scrolled off the top -> hide, never place.
	m.viewport.SetContent(strings.Repeat("line\n", 200))
	m.viewport.Height = 5
	m.viewport.SetYOffset(12)
	out = m.View()
	if strings.Contains(out, "\x1b_Ga=p,i=1974") {
		t.Fatal("a scrolled-away View must not place the image")
	}
	if !strings.Contains(out, "\x1b_Ga=d,d=i,i=1974") {
		t.Fatal("a scrolled-away View should emit the hide sequence")
	}

	// Overlay open -> hide.
	m.viewport.SetYOffset(0)
	m.overlay = overlaySettings
	if strings.Contains(m.View(), "\x1b_Ga=p,i=1974") {
		t.Fatal("View with an overlay open must not place the image")
	}

	// Feature turned off -> reconcile swaps in the noop renderer -> no output.
	m.overlay = overlayNone
	m.cfg.Display.ArticleImages = false
	m.reconcileImagesAfterSettings()
	if strings.Contains(m.View(), "\x1b_G") {
		t.Fatal("no Kitty output at all once the setting is off")
	}
}
