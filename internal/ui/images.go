package ui

import (
	"context"
	"fmt"
	stdimage "image"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tide/internal/db"
	img "github.com/allisonhere/tide/internal/image"
)

// imgDebugf writes a timestamped line to $TIDE_IMAGE_DEBUG (a file path) when
// that env var is set. It is a no-op otherwise, so it is safe to leave on the
// hot paths. Used to diagnose "no images" reports without a live terminal.
var (
	imgDebugOnce sync.Once
	imgDebugW    io.Writer
)

func imgDebugf(format string, args ...any) {
	imgDebugOnce.Do(func() {
		if p := os.Getenv("TIDE_IMAGE_DEBUG"); p != "" {
			if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				imgDebugW = f
			}
		}
	})
	if imgDebugW == nil {
		return
	}
	fmt.Fprintf(imgDebugW, "%s "+format+"\n",
		append([]any{time.Now().Format("15:04:05.000")}, args...)...)
}

// Aliases keep model.go free of a direct internal/image import while still
// letting its struct declare typed fields.
type (
	imageRenderer   = img.Renderer
	imageCapability = img.Capability
	imageFetcher    = img.Fetcher
)

// ── Article image subsystem (UI side) ────────────────────────────────────────
//
// The heavy lifting (detection, fetch, cache, decode, resize, Kitty escapes)
// lives in internal/image. This file holds only the Model-side lifecycle: when
// to fetch, how many rows to reserve in the Bubble Tea layout, and when to tell
// the renderer to draw or clear. All terminal writes happen here in Update
// (never in View, which has a value receiver and runs an unpredictable number
// of times).

// imgStatus tracks the lead image for the article currently shown in the
// Content pane.
type imgStatus int

const (
	imgIdle    imgStatus = iota // nothing to show (no URL, disabled, or not looked at yet)
	imgLoading                  // fetch/decode in flight
	imgReady                    // decoded + resized, rows reserved, drawable
	imgFailed                   // fetch or decode failed; fall back to text silently
)

// imageMaxRows caps how tall a lead image may render so it never dominates the
// reader. The effective cap also shrinks to fit a short content pane.
const imageMaxRows = img.DefaultMaxRows

// Float layout: when the content pane is wide enough the image sits at the
// top-left and the first `rows` lines of body text wrap to its right, magazine
// style. On a narrow reading column it falls back to a full-width blank band.
const (
	imageFloatGutter  = 2  // blank cells between the image and the text column
	imageFloatMinText = 30 // minimum readable text column beside the image
	imageFloatMaxCols = 46 // never let the floated image get wider than this
	imageFloatMinCols = 16 // ...or narrower than this
)

// imageState is the Model's single in-flight/resident image.
type imageState struct {
	articleID       int64
	url             string
	status          imgStatus
	rows            int            // cell rows the image occupies (0 unless status == imgReady)
	cols            int            // image placement width in cells
	resizedForWidth int            // contentBodyWidth() the current resize was made for
	floated         bool           // sized for the wrap-alongside layout (vs full-width band)
	floatActive     bool           // body is currently rendered with text wrapped beside the image
	src             stdimage.Image // decoded source, kept so re-fits on resize stay sharp
	pix             *stdimage.RGBA // decoded + resized frame handed to the renderer
	natW, natH      int            // decoded (pre-resize) pixel size
}

// imageNoopRenderer is the always-installed default until EnableImages swaps in
// a real backend.
func imageNoopRenderer() img.Renderer { return img.NewNoopRenderer() }

// newImageFetcher builds the lazy image fetcher, wiring in Tide's on-disk image
// cache when its directory can be resolved (cache failures are non-fatal).
func newImageFetcher() *img.Fetcher {
	dir, err := img.CacheDir()
	if err != nil {
		dir = ""
	}
	return img.NewFetcher(img.NewCache(dir))
}

// EnableImages installs a real graphics renderer discovered at startup. It is
// called by main.go before the Bubble Tea program starts. cap is retained even
// when unsupported so the Settings screen can explain why images are inactive.
// A nil or unsupported renderer leaves the model in text-only mode.
func (m *Model) EnableImages(cap img.Capability, r img.Renderer) {
	m.imgCap = cap
	if r != nil && r.Supported() && cap.Supported {
		m.imgRenderer = r
	}
	imgDebugf("EnableImages cap.supported=%v proto=%q cell=%dx%d renderer=%T active=%v setting=%v",
		cap.Supported, cap.Protocol, cap.CellW, cap.CellH, m.imgRenderer, m.imagesActive(), m.cfg.Display.ArticleImages)
}

// imagesActive reports whether lead images should be fetched and drawn right
// now: the setting is on and a working renderer is installed.
func (m *Model) imagesActive() bool {
	return m.cfg.Display.ArticleImages && m.imgRenderer != nil && m.imgRenderer.Supported()
}

// noteContentArticleForImage is called by setViewportArticle. It keeps the
// image state pointed at the article now in the Content pane and flags the
// model dirty whenever that article has a lead image that still needs loading —
// which covers both switching articles and enabling the setting while already
// sitting on one.
func (m *Model) noteContentArticleForImage(a db.Article) {
	if m.image.articleID != a.ID {
		if m.imgCancel != nil {
			m.imgCancel()
			m.imgCancel = nil
		}
		m.image = imageState{articleID: a.ID, url: a.ImageURL, status: imgIdle}
	} else {
		m.image.url = a.ImageURL
	}

	if m.imagesActive() &&
		m.image.url != "" &&
		m.image.status == imgIdle &&
		!m.imgHidden[a.ID] {
		m.imgDirty = true
	}
	imgDebugf("noteContentArticle id=%d url=%q status=%d active=%v dirty=%v",
		a.ID, m.image.url, m.image.status, m.imagesActive(), m.imgDirty)
}

// articleImageReadyMsg carries the result of a lazy lead-image fetch+decode
// back to Update. req identifies which article/generation asked for it so a
// result that arrives after the user moved on is dropped.
type articleImageReadyMsg struct {
	req     img.Req
	decoded stdimage.Image
	err     error
}

// drainImageCmd runs once per Update via the wrapper in Update(). It keeps the
// resident image fitted to the current reading column and, when the model was
// flagged dirty (article switch, `i` toggle), starts the lazy download. The
// image itself is emitted as part of every View frame (see imageFrameSequence),
// so there are no draw timers or side-channel writes.
func (m Model) drainImageCmd(pending tea.Cmd) (tea.Model, tea.Cmd) {
	// A width change since the last resize means the resident image needs to be
	// re-fitted to the new reading column before the next frame draws it.
	if m.image.status == imgReady && m.image.src != nil &&
		m.image.resizedForWidth != m.contentBodyWidth() {
		m.refitResidentImage()
	}

	if !m.imgDirty {
		return m, pending
	}
	m.imgDirty = false
	m.imgGen++

	if m.imagesActive() &&
		m.image.url != "" &&
		!m.imgHidden[m.image.articleID] &&
		m.image.status == imgIdle {
		m.image.status = imgLoading
		imgDebugf("drain: starting fetch gen=%d id=%d url=%q", m.imgGen, m.image.articleID, m.image.url)
		fetch := m.fetchArticleImageCmd(img.Req{ArticleID: m.image.articleID, Gen: m.imgGen}, m.image.url)
		if pending == nil {
			return m, fetch
		}
		return m, tea.Batch(pending, fetch)
	}
	imgDebugf("drain: no fetch active=%v url=%q hidden=%v status=%d",
		m.imagesActive(), m.image.url, m.imgHidden[m.image.articleID], m.image.status)
	return m, pending
}

// imageFrameSequence is appended to the end of every View() frame. When the
// image should be visible it returns the Kitty transmit (only when the pixels
// changed) plus a placement at the image's absolute cell coordinates; otherwise
// it returns a one-line "hide" sequence. Because it rides the Bubble Tea frame,
// there is a single writer and it lands after every text cell is painted, so
// nothing overwrites the image and nothing can chop the escape sequence.
func (m Model) imageFrameSequence() string {
	r := m.imgRenderer
	if r == nil || !r.Supported() {
		return ""
	}
	if !m.imageShouldShow() {
		return r.Hide()
	}
	x, y := m.imageDrawOrigin()
	imgDebugf("frame: place at (%d,%d) %dx%d", x, y, m.image.cols, m.image.rows)
	return r.Frame(m.image.pix, x, y, m.image.cols, m.image.rows)
}

// imageShouldShow reports whether the lead image belongs on screen right now.
func (m Model) imageShouldShow() bool {
	return m.imagesActive() &&
		m.overlay == overlayNone &&
		m.image.status == imgReady &&
		m.image.pix != nil &&
		m.image.rows > 0 &&
		!m.imgHidden[m.image.articleID] &&
		m.contentArticleID == m.image.articleID &&
		m.viewport.YOffset == 0 &&
		m.width >= 24 && m.height >= 8
}

// imageDrawOrigin returns the absolute 1-based (col, row) of the image's
// top-left cell. The ContentPane style carries no border/padding, so geometry
// is: feeds pane width + 1 body indent for the column; articles pane height + 1
// content header + imageBodyTopLine for the row (valid while viewport.YOffset
// == 0, which imageShouldShow guarantees).
func (m Model) imageDrawOrigin() (int, int) {
	x := m.feedsPaneWidth() + 1 + 1 // +indent, +1 for 1-based
	y := m.articlesPaneOuterHeight() + 1 + imageBodyTopLine + 1
	return x, y
}

// refitResidentImage re-runs the fit against the decoded source for a new
// reading-column width (e.g. after a terminal or pane resize).
func (m *Model) refitResidentImage() {
	if m.image.src == nil {
		return
	}
	m.resizeCurrentImage()
}

// refreshImageCellSize is called on WindowSizeMsg. A font-size change alters the
// terminal's cell pixel geometry; if the terminal now reports it (TIOCGWINSZ)
// and it differs, adopt it and force a re-fit so the reserved rows keep
// matching the image's real height.
func (m *Model) refreshImageCellSize() {
	if !m.imagesActive() {
		return
	}
	w, h := img.CellSizeFromWinsize()
	if w <= 0 || h <= 0 {
		return
	}
	if w != m.imgCap.CellW || h != m.imgCap.CellH {
		m.imgCap.CellW, m.imgCap.CellH = w, h
		m.image.resizedForWidth = -1 // drainImageCmd re-fits before the next frame
	}
}

// toggleCurrentArticleImage handles the `i` key: it flips the per-article hide
// flag, re-lays the content so the image space appears/disappears, and marks
// the model dirty so drainImageCmd fetches a not-yet-loaded image.
func (m *Model) toggleCurrentArticleImage() {
	if m.contentArticleID == 0 {
		return
	}
	id := m.contentArticleID
	m.imgHidden[id] = !m.imgHidden[id]

	if !m.imgHidden[id] && m.image.articleID == id && m.image.status == imgFailed {
		// Give a previously-failed image another chance on explicit un-hide.
		m.image.status = imgIdle
	}
	if a := m.currentContentArticle(); a != nil {
		m.setViewportArticle(*a)
	}
	m.imgDirty = true
}

// quitCmd ends the program. On-screen images are cleared by main.go's post-Run
// cleanup (a d=A written to stdout after Bubble Tea releases the terminal), and
// leaving the alt-screen removes them on most terminals anyway.
func (m Model) quitCmd() tea.Cmd { return tea.Quit }

// reconcileImagesAfterSettings is called after the Settings overlay writes back
// config. It turns the subsystem on/off to match Display.ArticleImages.
func (m *Model) reconcileImagesAfterSettings() {
	if m.imgCancel != nil {
		m.imgCancel()
		m.imgCancel = nil
	}
	m.image = imageState{}

	if !m.cfg.Display.ArticleImages {
		m.imgRenderer = img.NewNoopRenderer()
		return
	}

	m.imgDirty = true
	if m.imagesActive() {
		return // a real renderer is already installed
	}
	// The startup capability check (env-only when the feature was off at launch)
	// may already have found this terminal capable — the frame-embedded renderer
	// needs no /dev/tty handle, so it can be attached now.
	if m.imgCap.Supported {
		m.imgRenderer = img.NewKittyRenderer()
		imgDebugf("reconcile: attached runtime kitty renderer")
		return
	}
	imgDebugf("reconcile: cannot activate at runtime (cap.supported=%v)", m.imgCap.Supported)
	m.setStatus("Article images: restart Tide to activate on this terminal", false)
}

// fetchArticleImageCmd downloads and decodes url off the UI goroutine. The
// context is cancelled if the user navigates away before it completes.
func (m *Model) fetchArticleImageCmd(req img.Req, url string) tea.Cmd {
	fetcher := m.imgFetcher
	ctx, cancel := context.WithCancel(context.Background())
	m.imgCancel = cancel
	return func() tea.Msg {
		if fetcher == nil {
			return articleImageReadyMsg{req: req, err: context.Canceled}
		}
		data, _, err := fetcher.Fetch(ctx, url)
		if err != nil {
			return articleImageReadyMsg{req: req, err: err}
		}
		decoded, err := img.Decode(data)
		if err != nil {
			// Bad bytes: drop the cache entry so a later visit can retry.
			if fetcher.Cache != nil {
				fetcher.Cache.Delete(url)
			}
			return articleImageReadyMsg{req: req, err: err}
		}
		return articleImageReadyMsg{req: req, decoded: decoded}
	}
}

// handleArticleImageReady applies a completed fetch: it discards stale results,
// records failures silently (no placeholder), and on success resizes the image
// to the current reading column and reserves the corresponding rows.
func (m Model) handleArticleImageReady(msg articleImageReadyMsg) (tea.Model, tea.Cmd) {
	if !msg.req.Fresh(m.image.articleID, m.imgGen) {
		imgDebugf("ready: STALE req(id=%d gen=%d) cur(id=%d gen=%d)",
			msg.req.ArticleID, msg.req.Gen, m.image.articleID, m.imgGen)
		return m, nil
	}
	if msg.err != nil || msg.decoded == nil {
		imgDebugf("ready: FAILED id=%d err=%v", msg.req.ArticleID, msg.err)
		m.image.status = imgFailed
		m.image.pix = nil
		if a := m.currentContentArticle(); a != nil {
			m.setViewportArticle(*a)
		}
		return m, nil
	}

	m.image.src = msg.decoded
	m.resizeCurrentImage()
	imgDebugf("ready: OK id=%d status=%d rows=%d cols=%d floated=%v",
		msg.req.ArticleID, m.image.status, m.image.rows, m.image.cols, m.image.floated)
	if a := m.currentContentArticle(); a != nil {
		if m.image.status == imgReady {
			m.viewport.GotoTop()
			m.image.floatActive = m.wantFloatContent()
		}
		m.setViewportArticle(*a)
		if m.image.status == imgReady {
			m.contentFocusLine = m.firstFocusableForImage()
			m.ensureContentFocusVisible()
		}
	}
	return m, nil
}

// resizeCurrentImage scales the decoded source (m.image.src) to fit the reading
// column and the row cap, storing the result and the reserved row count. A
// content pane too short to host even a minimal image downgrades to text-only.
func (m *Model) resizeCurrentImage() {
	decoded := m.image.src
	cellW, cellH := m.imgCap.CellSize()

	maxRows := imageMaxRows
	if fit := m.contentBodyHeight() - 4; fit < maxRows {
		maxRows = fit
	}
	if maxRows < 3 || decoded == nil {
		m.image.status = imgFailed
		m.image.pix = nil
		m.image.rows = 0
		return
	}

	bodyW := m.contentBodyWidth()
	floated := m.imageFloatFits()
	drawCols := max(1, bodyW-1)
	if floated {
		drawCols = m.imageFloatCols()
	}
	maxPxW := drawCols * cellW
	maxPxH := maxRows * cellH

	resized := img.Fit(decoded, maxPxW, maxPxH)
	b := resized.Bounds()

	m.image.pix = resized
	m.image.natW, m.image.natH = decoded.Bounds().Dx(), decoded.Bounds().Dy()
	m.image.rows = img.RowsFor(b.Dy(), cellH, maxRows)
	m.image.cols = img.ColsFor(b.Dx(), cellW, drawCols)
	m.image.floated = floated
	m.image.resizedForWidth = bodyW
	m.image.status = imgReady
}

// imageFloatCols is the target width (cells) of a floated image: about 40% of
// the reading column, clamped to a sane band.
func (m Model) imageFloatCols() int {
	c := m.contentBodyWidth() * 2 / 5
	if c > imageFloatMaxCols {
		c = imageFloatMaxCols
	}
	if c < imageFloatMinCols {
		c = imageFloatMinCols
	}
	return c
}

// imageFloatFits reports whether there is room for the image plus a gutter plus
// a readable text column beside it. Otherwise the layout uses a full-width band.
func (m Model) imageFloatFits() bool {
	return m.contentBodyWidth()-1-m.imageFloatCols()-imageFloatGutter >= imageFloatMinText
}

// wantFloatContent reports whether the Content viewport should currently be
// rendered with text wrapped beside the image (image ready, sized for float,
// this article, scrolled to the very top).
func (m Model) wantFloatContent() bool {
	return m.imagesActive() &&
		m.image.status == imgReady &&
		m.image.floated &&
		m.image.rows > 0 &&
		!m.imgHidden[m.image.articleID] &&
		m.contentArticleID == m.image.articleID &&
		m.viewport.YOffset == 0
}

// applyImageLayout adjusts the assembled article body for the current image
// layout: indent the first `rows` lines past a floated image, or prepend a
// full-width blank band, or (floated but scrolled away) leave it untouched.
func (m Model) applyImageLayout(body string) string {
	if !m.imagesActive() || m.image.status != imgReady ||
		m.imgHidden[m.image.articleID] || m.image.rows <= 0 {
		return body
	}
	if m.image.floated {
		if m.image.floatActive {
			return indentFirstLines(body, m.image.rows, m.image.cols+imageFloatGutter)
		}
		return body // scrolled past the top: full-width text, image not drawn
	}
	return strings.Repeat("\n", m.image.rows) + body
}

// indentFirstLines prepends `indent` spaces to the first n lines of s, padding
// with blank indented lines when s has fewer than n lines so the image box is
// never clipped by a short article.
func indentFirstLines(s string, n, indent int) string {
	if n <= 0 || indent <= 0 {
		return s
	}
	pad := strings.Repeat(" ", indent)
	lines := strings.Split(s, "\n")
	for i := 0; i < n; i++ {
		if i < len(lines) {
			lines[i] = pad + lines[i]
		} else {
			lines = append(lines, pad)
		}
	}
	return strings.Join(lines, "\n")
}

// firstFocusableForImage picks where keyboard focus should start once the image
// is shown. With the wrap-alongside layout the text beside the image is real
// readable text, so focus starts on the first focusable line; with the
// full-width band it starts on the first line below the band.
func (m Model) firstFocusableForImage() int {
	if m.image.floated && m.image.floatActive {
		return firstFocusableLine(m.contentFocusable)
	}
	floor := imageBodyTopLine + m.image.rows
	for i := floor; i < len(m.contentFocusable); i++ {
		if m.contentFocusable[i] {
			return i
		}
	}
	return firstFocusableLine(m.contentFocusable)
}

// syncImageFloatLayout re-renders the Content viewport when the wrap-alongside
// state needs to flip (e.g. the user scrolled away from the top, or back to
// it). Called from the Update wrapper. Returns true if it re-rendered.
func (m *Model) syncImageFloatLayout() bool {
	want := m.wantFloatContent()
	if want == m.image.floatActive {
		return false
	}
	m.image.floatActive = want
	a := m.currentContentArticle()
	if a == nil {
		return false
	}
	off := m.viewport.YOffset
	m.setViewportArticle(*a) // same-article path keeps the focus line; re-lays body
	m.viewport.SetYOffset(off)
	imgDebugf("float: layout flipped -> active=%v", want)
	return true
}

// imageReservedRows is read synchronously by the Content pane renderer. It
// returns the number of blank rows to leave at the top of the article body for
// the image. Zero unless an image is decoded, sized, active and not hidden for
// this article.
func (m Model) imageReservedRows() int {
	if !m.cfg.Display.ArticleImages || m.imgRenderer == nil || !m.imgRenderer.Supported() {
		return 0
	}
	if m.imgHidden[m.image.articleID] {
		return 0
	}
	if m.image.status != imgReady {
		return 0
	}
	return m.image.rows
}
