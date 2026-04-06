# Kitty Graphics Support For Inline Article Images

## Summary
Implement Kitty graphics support on a dedicated feature branch before any code changes are made.

Chosen behavior:
- Start work on a new branch such as `feature/kitty-graphics`
- Render all discovered article images inline, in article order
- Auto-enable when Kitty graphics support is detected, but add a Display setting to turn it off
- Keep the current text-first reading experience as the fallback everywhere else

This should be implemented as a progressive enhancement: unsupported terminals, failed image loads, or unsupported image formats must leave the current text-only rendering intact.

## Implementation Changes
- Setup:
  - create and switch to a dedicated branch, e.g. `feature/kitty-graphics`
  - keep all Kitty graphics work isolated there until the rendering lifecycle is stable
- Config and settings:
  - add `display.kitty_graphics = true`
  - expose a `Kitty graphics` toggle in the `DISPLAY` settings section
  - enable graphics only when both the setting is on and terminal support is detected
- Content rendering:
  - extend content-pane rendering to parse image references from article content
  - preserve article order with a mixed content block model of text blocks and image blocks
  - render text placeholders into the viewport for image rows so scrolling remains deterministic
- Terminal graphics layer:
  - implement a native Kitty graphics helper in the UI layer
  - write protocol sequences directly to `/dev/tty`
  - clear and redraw graphics on resize, scroll, article change, and overlay transitions
- Image loading:
  - fetch image bytes lazily from discovered image URLs
  - resolve relative URLs against the article link where possible
  - cache decoded/render-ready images in memory for the current session
  - support PNG, JPEG, and GIF first; add WebP decode support if needed during implementation
- Limits for v1:
  - no animation
  - first frame only for animated assets
  - no zoom/select/open-image interaction
  - no persistent image storage in SQLite

## Public Interfaces / Types
- Config:
  - add `display.kitty_graphics` boolean with default `true`
- Settings:
  - add a `Kitty graphics` toggle under `DISPLAY`
- Internal UI:
  - add an article content block model for text plus image blocks
  - add a Kitty graphics renderer/helper responsible for draw, clear, and placement lifecycle

## Test Plan
- Content parsing:
  - markdown image syntax becomes image blocks
  - raw HTML `<img>` tags become image blocks
  - mixed text and images preserve order
  - text-only articles stay unchanged
- Capability and fallback:
  - disabled setting prevents graphics even in supported terminals
  - unsupported terminals fall back to text-only
  - supported terminals with setting enabled activate graphics mode
- Rendering lifecycle:
  - resize redraws images at correct coordinates
  - scroll updates visible images correctly
  - article switches clear old images and draw new ones
  - opening and closing overlays does not leave stale graphics behind
- Failure handling:
  - broken image URLs do not break article rendering
  - unsupported image formats fail gracefully
- Regression:
  - existing content wrapping, margins, and text rendering remain unchanged when graphics are off
  - non-Kitty terminals behave exactly as they do today

## Assumptions
- Work begins on a dedicated branch before implementation starts.
- Native Kitty graphics protocol is preferred over external helpers.
- “All images inline” means preserving article order, not only showing a lead image.
- Session-memory caching is sufficient for the first pass.
