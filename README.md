# Tide

![Tide demo](tide.gif)

A terminal RSS reader built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss).

The reusable themed UI toolkit derived from Tide is available as
[`tideui`](https://github.com/allisonhere/tideui).

## Features

- Three-pane layout: feeds, articles, content
- Status bar reminders: feed manager, settings, search, and help keys (`m` · `S` · `/` · `?`)
- Comfortable or compact list spacing (Settings → Display; configurable in `config.toml`)
- Live theme switching with full preview
- Theme-aware dialogs and overlays
- Feed manager: add, edit, delete, import/export OPML
- Google Reader-compatible source support, including FreshRSS
- Full-text search across stored local articles, including titles, content, and AI summaries
- Unread-only filtering and in-article find
- Mark read/unread, open in browser
- Optional actionable links in content pane (Settings → Display)
- AI summaries with copy and save-to-Markdown actions
- 19 built-in themes, including customizable VT52 and VT100 palettes
- Terminal background sync (OSC 11)

## Themes

- `catppuccin-mocha`
- `catppuccin-latte`
- `catppuccin-frappe`
- `catppuccin-macchiato`
- `nord`
- `dracula`
- `gruvbox-dark`
- `gruvbox-light`
- `tokyo-night`
- `tokyo-night-day`
- `rose-pine`
- `rose-pine-moon`
- `rose-pine-dawn`
- `one-dark`
- `magenta-geode`
- `coral-sunset`
- `lavender-fields-forever`
- `vt100`
- `vt52`

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/allisonhere/tide/main/install.sh | sh
```

Installs to `/usr/local/bin` by default. To install elsewhere:

```bash
INSTALL_DIR=~/.local/bin curl -fsSL https://raw.githubusercontent.com/allisonhere/tide/main/install.sh | sh
```

Or build from source:

```bash
git clone https://github.com/allisonhere/tide
cd tide
go build -o tide .
```

## Usage

```bash
tide
```

Config and database are stored in `~/.config/rss/`.

If you have **no feeds yet**, use **`a`** (add) or **`m`** (feed manager) to subscribe. **`Enter`** does not open the add dialog on an empty feed list.

The **bottom status line** always shows **`m` feed manager**, **`S` settings**, **`/` search**, and **`?` help** on the left (alongside feed status). That matches the shortcuts documented in **`?` help**.

## Feed Manager And Sources

Open the feed manager with `m`. Tide keeps one shared manager flow for local feeds and Google Reader-compatible sources.

- Press **`a`** to open the add dialog from the main screen
- Opening the manager with **`m`** starts on the left list/details pane
- From a text field, **`←`** moves back to the left pane once the cursor is already at the start of the field
- The **`Source`** toggle in the add form switches between **`Local`** and **`GReader`**

**Local** source:

- **Feed names** come from the feed metadata. Add/edit only asks for **`URL`**, folder, and optional new-folder color.

**GReader** source (FreshRSS, etc.):

- **Feed names** come from the server: the add form explains that the **name is taken from the feed** when you subscribe (no separate rename field). Article and feed titles are shown with **HTML entities decoded** (for example apostrophes), including in the sidebar.
- Fields: optional feed **`URL`**, **`API URL`**, **`Login`**, **`Password`**
- If **`URL`** is blank, Tide saves the connection and loads your existing remote subscriptions
- If **`URL`** is set, Tide **quick-adds** that subscription on the remote server
- Press **`e`** on a remote feed to edit **GReader connection** settings (credentials and read-only name/URL summary). Saving applies API credentials and keeps labels aligned with the remote feed
- Folder placement for remote feeds is **not** changed from that screen; existing folder prefs stay in Tide’s database
- Selecting a remote feed in the manager shows connection info on the right, with the password masked

FreshRSS works through its Google Reader-compatible endpoint. Use the full API URL, for example:

```text
https://example.com/FreshRSS/p/api/greader.php
```

Saved Google Reader credentials are stored in `~/.config/rss/config.toml` under `[source]`. Treat this file like a secret (it can contain passwords and API keys). Local folder assignments and remote-feed prefs are stored in Tide’s SQLite database.

## Settings

Open settings with `S`.

The settings overlay uses a category list on the left and a focused detail pane on the right. Current categories are `DISPLAY`, `FEEDS`, `UPDATES`, `AI`, and `ABOUT`.

Display options:
- Toggle Unicode icons for pane headers and item state markers
- Switch between relative and absolute dates
- Toggle mark-read-on-open
- Toggle mark-read-on-focus
- Toggle the content focus line
- Start feeds in unread-only mode
- Select any built-in theme, with custom background, foreground, and accent colors for VT52/VT100
- Set the article reading width (`0` means no limit)
- Toggle actionable article links (shows a `LINKS` block in content pane)
- Filter links out of article body text
- **Layout density:** comfortable (extra vertical spacing in lists) or **compact** (default; more rows on small terminals)
- Set a custom browser command
- Toggle the quit confirmation

Feed options:
- Set the maximum feed body size accepted during parsing

Update options:
- Toggle startup update checks
- Check for the latest Tide release manually
- Install the latest release in place when the current binary path is writable
- If admin permission is required, Tide shows the exact install command to run

AI summary options:
- Provider: `none`, `OpenAI`, `Claude`, `Gemini`, or `Ollama`
- API key for OpenAI, Claude, or Gemini
- Ollama URL and model for local summaries
- Test the configured provider connection
- Save path for exported Markdown summaries
- Optionally mark an article read after summarizing it

About:
- Open the Tide repository and issues page from inside Settings
- Includes a small signed note and project tagline

Settings are saved to `~/.config/rss/config.toml`.

## AI Summaries

Tide can summarize the currently selected article when focus is in the `Articles` or `Content` pane.

- Press `s` to open an AI summary for the selected article
- Press `c` in the summary dialog to copy the summary
- Press `M` in the summary dialog to save it as `.md`
- If AI is not configured, Tide shows a prompt to open Settings with `S`

Supported providers and current built-in models:
- OpenAI
  Uses `gpt-4o-mini`
- Claude
  Uses `claude-haiku-4-5-20251001`
- Gemini
  Uses `gemini-1.5-flash`
- Ollama
  Uses your configured local model, default `llama3.2`

Provider requirements:
- OpenAI: set an OpenAI API key
- Claude: set an Anthropic API key
- Gemini: set a Google AI Studio API key
- Ollama: run a local Ollama server and choose a model

Summary behavior:
- Tide sends the article title and up to the first 4000 characters of article content
- The app asks the provider for a concise 3-5 sentence summary
- Requests use a 30 second timeout
- Generated summaries are cached per article and can be reopened without regenerating them
- Saved summaries are written as Markdown files to your configured save path

Default Ollama settings:
- URL: `http://localhost:11434`
- Model: `llama3.2`

Example `config.toml`:

```toml
theme = "catppuccin-mocha"

[display]
icons = false
date_format = "relative"
mark_read_on_open = true
mark_read_on_focus = false
focus_line = true
default_unread_only = false
actionable_links = false
filter_links = false
reading_width = 0
feed_pane_width_percent = 28
article_pane_height_percent = 40
confirm_quit = true
browser = ""
density = "compact"

[feed]
max_body_mib = 10

[ai]
provider = "ollama" # openai | claude | gemini | ollama | ""
openai_key = ""
claude_key = ""
gemini_key = ""
ollama_url = "http://localhost:11434"
ollama_model = "llama3.2"
save_path = "~/"
mark_read_on_summarize = false

[source]
greader_url = ""
greader_login = ""
greader_password = ""
```

Feed fetch limits:
- `feed.max_body_mib` controls the maximum feed response size accepted for parsing
- Default is `10`
- If a feed exceeds this limit, Tide returns a clear “feed is too large to parse” error instead of a misleading XML syntax error

## Self Updates

Tide can check GitHub releases for a newer version and install the matching binary for your platform.

- Startup checks are enabled by default and run at most once every 24 hours
- Manual checks and installs live in the `UPDATES` section of Settings (`S`)
- Tide never downloads or installs an update until you explicitly choose `Update now`
- After a successful install, Tide offers `Restart now`
- If the install target needs elevated permissions, Tide shows a manual `sudo install ...` command instead of failing silently

## Making A Release

Maintainers can prepare and publish a release with the deployment TUI:

```bash
./release.sh
```

The TUI:

- Detects the latest semantic-version tag and offers patch, minor, or major bumps
- Shows every worktree change before staging it
- Requires a second release confirmation before making remote changes
- Checks that Git author name and email are configured before doing any release work
- Runs the complete Go test suite and checks the patch for whitespace errors
- Fetches `origin/main`, refuses to release from another branch, and stops if local `main` is behind
- Stages all worktree changes, creates the chosen release commit when needed, and pushes `main`
- Creates and pushes an annotated version tag

Pushing the tag starts the GitHub Actions release workflow. CI tests again, builds Linux and macOS archives for x86-64 and ARM64, publishes SHA-256 checksums, generates release notes, and creates the GitHub release. The installer verifies those checksums when they are available. If the final tag push fails after the local tag was created, the TUI prints the exact command needed to resume safely.

## Keyboard Shortcuts

### Navigation
| Key | Action |
|-----|--------|
| `Tab` / `]`, `Shift-Tab` / `[` | Cycle panes forward or backward |
| `h/←` `l/→` | Move between panes |
| `j/↓` `k/↑` | Navigate within pane |
| `Shift+←` / `Shift+→` | Resize feed pane |
| `Shift+↑` / `Shift+↓` | Resize articles/content split |
| `Enter` | Open article |
| `Esc` | Back |

### Articles
| Key | Action |
|-----|--------|
| `r` | Toggle read/unread |
| `R` | Mark the selected feed or folder read |
| `u` | Toggle unread-only view |
| `o` | Open selected content link (when enabled) or article URL |
| `Ctrl+N` / `Alt+N` | Next actionable link in content pane |
| `Ctrl+P` / `Alt+P` | Previous actionable link in content pane |
| `Ctrl+F` | Find text in the current article; `Enter`/`↓` selects the next match and `↑` the previous match |
| `/` | Search titles, content, and summaries across all stored local articles |

Search results are ranked by relevance and include their source feed and a matching excerpt. Press `Enter` to jump to a result. Google Reader-compatible articles are loaded from the remote service rather than stored locally, so they are not included in library search.

When `Display → Focus line` is enabled, the content pane highlights the current readable line. `j/↓` and `k/↑` move the focus line and scroll only when needed.

When `Display → Actionable article links` is enabled, the content pane renders a `LINKS` block at the bottom. The currently selected link is highlighted and opened with `o`.

### AI Summary
| Key | Action |
|-----|--------|
| `s` | AI summary for selected article when focus is in Articles or Content |
| `c` | Copy summary in summary dialog |
| `M` | Save summary as `.md` in summary dialog |

### Feeds
| Key | Action |
|-----|--------|
| `f` | Refresh feed |
| `F` / `I` | Refresh all |
| `m` | Feed manager |
| `a` | Add a feed or GReader source from anywhere |

### Feed Manager
| Key | Action |
|-----|--------|
| `a` | Add feed or GReader source |
| `n` | Add folder |
| `Enter` | Browse selected remote feed, edit selected local feed, or enter the form from the left pane |
| `e` | Edit selected local feed or GReader settings |
| `v` | Move the selected feed to a folder |
| `d` | Delete selected local feed |
| `i` | Import OPML |
| `x` | Export OPML |

### App
| Key | Action |
|-----|--------|
| `T` | Theme picker |
| `S` | Settings |
| `/` | Search stored local articles across all feeds |
| `?` | Help |
| `q` | Quit |

The same **`m`**, **`S`**, **`/`**, and **`?`** shortcuts are repeated on the **status bar** (bottom line) whenever the main reader is visible.
