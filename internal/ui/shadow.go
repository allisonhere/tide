package ui

import (
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/muesli/termenv"
)

// Modal drop shadow: a darkened sliver down the right and along the bottom of
// an overlay, blended into whatever is actually behind it. The shadow colour
// is derived from the page background, so how strong it reads varies with the
// theme: a pale theme steps down further than a near-black one.

// shadowBgMinContrast is the contrast floor a modal drop shadow's background
// must clear against the page background it sits on, so the shadow reads as a
// shadow instead of an invisible near-match.
const shadowBgMinContrast = 1.6

// shadowOffsetX and shadowOffsetY position the drop shadow relative to the
// modal. The modal covers all of the shadow rectangle except a bottom-and-right
// sliver, which reads as the shadow. One cell on each side: a 2-wide vertical
// band reads as a heavy bar next to the modal rather than a shadow.
const shadowOffsetX, shadowOffsetY = 1, 1

// shadowAlpha is how strongly the shadow rectangle darkens whatever is behind
// it (0 = invisible, 1 = the flat shadow colour with nothing of the original
// showing through). It has to be strong enough to look like a shadow.
const shadowAlpha = 0.925

// shadowFgAlpha is deliberately much weaker than shadowAlpha. Blending a
// glyph's foreground as hard as the background would make text under the
// shadow unreadable; this dims it while keeping it legible.
const shadowFgAlpha = 0.55

// shadowColor darkens bg until it clears shadowBgMinContrast against bg
// itself, always in the darkening direction regardless of theme: a shadow
// should always read as "darker", not "further away", on light and dark
// themes alike.
func shadowColor(bg lipgloss.Color) lipgloss.Color {
	const step = 0.03
	const maxSteps = 30
	cur := bg
	for range maxSteps {
		next := adjustLightness(cur, -step)
		if next == cur {
			break
		}
		cur = next
		if contrastRatio(cur, bg) >= shadowBgMinContrast {
			return cur
		}
	}
	return cur
}

// blendBg lerps a cell's currently resolved color toward target by alpha
// (0 = unchanged, 1 = fully target). existing is the color cellbuf resolved
// for the cell; nil means the cell never had one set, so it shows the page's
// own default background, and page stands in for it.
func blendBg(existing ansi.Color, page, target lipgloss.Color, alpha float64) ansi.RGBColor {
	var er, eg, eb float64
	if existing == nil {
		er, eg, eb, _ = hexToRGB(page)
	} else {
		r, g, b, _ := existing.RGBA()
		er, eg, eb = float64(r>>8)/255, float64(g>>8)/255, float64(b>>8)/255
	}
	tr, tg, tb, _ := hexToRGB(target)
	return ansi.RGBColor{
		R: uint8(clamp01(er+(tr-er)*alpha)*255 + 0.5),
		G: uint8(clamp01(eg+(tg-eg)*alpha)*255 + 0.5),
		B: uint8(clamp01(eb+(tb-eb)*alpha)*255 + 0.5),
	}
}

// blendShadowRect darkens the rectangle of base behind the modal's shadow by
// blending each cell's own already-resolved colors toward shadowBg, instead of
// painting an opaque rectangle over it: a real drop shadow rather than a flat
// cutout. It round-trips only the affected rows through a cellbuf grid, which
// parses ANSI styling into per-cell colors and serializes the result back.
// Render emits "\r\n" between lines (cellbuf's own convention); that is
// normalized back to "\n" so the result still fits the plain strings.Split the
// compositor uses.
//
// Scoped to rows y..y+h rather than the whole viewport: parsing and
// serializing the entire base every frame, most of which the shadow never
// touches, is wasteful. clampView already guarantees those rows are
// self-contained, uniformly width-padded lines safe to splice back by index.
func blendShadowRect(base string, x, y, w, h, totalWidth, totalHeight int, page, shadowBg lipgloss.Color) string {
	base = clampView(base, totalWidth, totalHeight, page)
	rowStart, rowEnd := max(0, y), min(totalHeight, y+h)
	if rowStart >= rowEnd {
		return base // the shadow rectangle misses the viewport entirely
	}
	lines := strings.Split(base, "\n")
	sliceHeight := rowEnd - rowStart
	slice := strings.Join(lines[rowStart:rowEnd], "\n")

	buf := cellbuf.NewBuffer(totalWidth, sliceHeight)
	cellbuf.SetContent(buf, slice)
	// blendBg always returns a raw 24-bit color, but cellbuf.Render has no
	// color-profile awareness of its own: convert to the profile everything
	// else downgrades through, or a non-truecolor terminal would drop the
	// shadow cells' unsupported color codes.
	profile := activeColorProfile()
	for row := 0; row < sliceHeight; row++ {
		for col := max(0, x); col < min(totalWidth, x+w); col++ {
			cell := buf.Cell(col, row)
			if cell == nil {
				continue
			}
			blended := *cell
			blended.Style.Bg = profile.Convert(blendBg(cell.Style.Bg, page, shadowBg, shadowAlpha))
			// A glyph inside the shadow needs darkening too, not just the space
			// around it, or text rendered right up to a pane's edge (where the
			// sliver sits) keeps its bright color. Skip a nil Fg rather than
			// fabricating one.
			if cell.Style.Fg != nil {
				blended.Style.Fg = profile.Convert(blendBg(cell.Style.Fg, page, shadowBg, shadowFgAlpha))
			}
			buf.SetCell(col, row, &blended)
		}
	}
	rendered := strings.Split(strings.ReplaceAll(cellbuf.Render(buf), "\r\n", "\n"), "\n")
	if len(rendered) != sliceHeight {
		// Defensive: if cellbuf's line count ever differs from what was asked
		// for, keep the unshadowed rows rather than splice in a corrupted view.
		return base
	}
	// Splice a re-rendered row back only if cellbuf reproduced it at exactly
	// totalWidth. Its width model differs from lipgloss/x-ansi: a wide rune, an
	// OSC-8 hyperlink or a trimmed trailing run in the base can come back a
	// column or two short, and splicing that in would shift everything after it
	// on the row. Rows cellbuf mangled keep their original content: a one-row
	// gap in the shadow, never a corrupted line.
	for i, r := range rendered {
		if lipgloss.Width(r) == totalWidth {
			lines[rowStart+i] = r
		}
	}
	return strings.Join(lines, "\n")
}

// activeColorProfile bridges lipgloss's global color profile (a termenv
// profile, the one every other Render call implicitly downgrades through) to
// the distinct colorprofile.Profile type cellbuf's Convert expects.
func activeColorProfile() colorprofile.Profile {
	switch lipgloss.ColorProfile() {
	case termenv.TrueColor:
		return colorprofile.TrueColor
	case termenv.ANSI256:
		return colorprofile.ANSI256
	case termenv.ANSI:
		return colorprofile.ANSI
	default:
		return colorprofile.ASCII
	}
}
