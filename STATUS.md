# Status — 2026-09-01

## Current branch: feat/article-images

## What's working
- Optional article images in the Content pane via the Kitty graphics protocol (`Settings → Display → Article images`, off by default): conservative terminal detection (Kitty/Ghostty/WezTerm/Konsole) with cell-pixel-size query, lazy fetch with disk cache under `~/.cache/rss/images/`, aspect-preserving resize capped at ~12 rows. Wide panes float the image top-left with the first lines of text wrapped beside it; narrow panes use a full-width band above the text. The Kitty escape sequence rides the Bubble Tea `View()` frame (single writer, no side channel) so it cannot be chopped and never overwrites the image. `i` toggles per article; cleanup on scroll/resize/overlay/theme/article-change/quit. Clean text-only fallback on unsupported terminals. Isolated in `internal/image/`; `TIDE_IMAGE_DEBUG=<file>` logs the lifecycle and `go run ./cmd/imgcheck [feed-url]` tests each stage in isolation.
- Feed manager overlay: add / edit / delete / import OPML / export OPML
- Blinking cursor in add/edit inputs via bubbles textinput
- Accent left-border focus indicator on inputs; title field focused first on new add
- Background bleed fully fixed in feed manager dialog (labels, inputs, spacers, wrappers)
- Robust feed fetching: up to 10 redirects, loop detection, 307/308 method preservation, bot-protection detection, full error classification (9 kinds), TUI error details overlay, permanent-redirect URL update suggestion
- Large-feed handling: explicit size limit with configurable `feed.max_body_mib` instead of truncated XML parse failures
- Settings modal polish: compact aligned rows, dedicated FEEDS section, shorter inputs, inline helper text
- 18 feed fetcher tests passing
- deploy.sh release tool, install.sh curl installer, GitHub Actions release CI

## Known issues / next up
- Feed manager stability pass landed: shared detail-pane geometry, explicit background coverage, and regression tests for narrow widths and cancel/focus transitions
- Keep monitoring screenshots for theme-specific Lip Gloss edge cases, but there is no current known reproducible background-bleed case in the feed manager

## Key bg-bleed patterns (summary)
1. bubbles pads input.View() with plain spaces → TrimRight + re-pad with fieldBg
2. lipgloss doesn't re-emit bg after ANSI resets → sub-styles need Background(fieldBg)
3. JoinVertical pads with plain spaces → all elements must be same width (contentW)
4. PaddingLeft without Background → always add Background to padding styles

## Repo
github.com/allisonhere/tide
