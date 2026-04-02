# Kitty Graphics — Status

Branch: `feature/kitty-graphics`
PR: https://github.com/allisonhere/tide/pull/1

## What's done

- Lead image URL extraction from feeds (gofeed Image, first `<img>`, image enclosures)
- `image_url` column in articles table (migration v5, auto-runs on startup)
- Terminal detection via `TERM_PROGRAM` (kitty, WezTerm, Ghostty)
- Async image fetch, decode (PNG/JPEG/GIF), and bilinear scaling to fit content pane
- Session-level image cache (`map[int64]*kittyImage` on Model)
- Direct placement via `/dev/tty` with ANSI cursor positioning + kitty `a=p`
- Image cleared on article switch, re-placed on resize
- `display.kitty_graphics` config toggle (default true) + Settings UI entry
- All tests pass, clean build

## What needs testing / likely issues

- **Screen position math** — `kittyImageScreenRow()` and `kittyImageScreenCol()` in `model.go` calculate the image's screen coordinates from pane dimensions. These are estimates and may be off by 1-2 rows/cols depending on borders and padding. Visually verify and adjust the constants.
- **Scrolling** — image is only placed when viewport is at top (`YOffset <= 3`). Scrolling down should hide it, but the clear-on-scroll path isn't wired yet. Currently the image ghost may linger until you switch articles.
- **Cell size assumption** — hardcoded 8x16 px per cell in `kitty.go`. Terminals with different font sizes will get wrong aspect ratios. Could query cell size via `\x1b[16t` later.
- **Large images** — no max height cap. A tall image could push all text below the fold. Should clamp `ki.rows` to e.g. half the viewport height.
- **Overlay transitions** — opening settings/help/theme picker doesn't clear the kitty image yet. May leave a ghost image behind the overlay.

## Key files

| File | What's there |
|------|-------------|
| `internal/ui/kitty.go` | Detection, fetch, scale, transmit, place, delete |
| `internal/ui/model.go:1398` | `renderArticleContent` — blank line reservation |
| `internal/ui/model.go:1984` | `maybeFetchKittyImageCmd`, screen position helpers, place/clear |
| `internal/feed/parser.go:137` | `extractLeadImage`, `firstImgSrc` |
| `internal/db/db.go:123` | Migration v5 (image_url column) |
| `internal/config/config.go:24` | `KittyGraphics` field |
| `internal/ui/settings.go` | `sfKittyGraphics` toggle |

## Next steps

1. Fix scroll — clear image when `YOffset > imageRows + 3`, re-place when scrolling back to top
2. Clear image on overlay open (settings, help, theme, feed manager)
3. Cap image height to ~40% of viewport
4. Query actual cell pixel size for accurate scaling
5. Test in kitty and Ghostty (only verified in WezTerm so far)
