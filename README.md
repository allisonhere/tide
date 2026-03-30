# Tide

![Tide screenshot](screenshot.png)

A terminal RSS reader built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss).

## Features

- Three-pane layout: feeds, articles, content
- Live theme switching with full preview
- Theme-aware dialogs and overlays
- Feed manager: add, edit, delete, import/export OPML
- Article search and filter
- Mark read/unread, open in browser
- AI summaries with copy and save-to-Markdown actions
- 17 built-in themes
- Terminal background sync (OSC 11)

## Themes

catppuccin-mocha, catppuccin-latte, catppuccin-frappe, catppuccin-macchiato, nord, dracula, gruvbox-dark, gruvbox-light, tokyo-night, tokyo-night-day, rose-pine, rose-pine-moon, rose-pine-dawn, one-dark, magenta-geode, coral-sunset, lavender-fields-forever

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

## Settings

Open settings with `S`.

Display options:
- Toggle icons
- Switch between relative and absolute dates
- Toggle mark-read-on-open
- Set a custom browser command

AI summary options:
- Provider: `none`, `OpenAI`, `Claude`, `Gemini`, or `Ollama`
- API key for OpenAI, Claude, or Gemini
- Ollama URL and model for local summaries
- Save path for exported Markdown summaries

Settings are saved to `~/.config/rss/config.toml`.

## AI Summaries

Tide can summarize the currently selected article when focus is in the `Articles` or `Content` pane.

- Press `s` to open an AI summary for the selected article
- Press `c` in the summary dialog to copy the summary
- Press `m` in the summary dialog to save it as `.md`
- If AI is not configured, Tide shows a prompt to open Settings with `S`

Supported providers:
- OpenAI
- Claude
- Gemini
- Ollama

Default Ollama settings:
- URL: `http://localhost:11434`
- Model: `llama3.2`

## Keyboard Shortcuts

### Navigation
| Key | Action |
|-----|--------|
| `Tab` / `Shift-Tab` | Cycle panes |
| `h/←` `l/→` | Move between panes |
| `j/↓` `k/↑` | Navigate within pane |
| `Enter` | Open article |
| `Esc` | Back |

### Articles
| Key | Action |
|-----|--------|
| `r` | Toggle read/unread |
| `R` | Mark all read |
| `o` | Open in browser |
| `/` | Search |

### AI Summary
| Key | Action |
|-----|--------|
| `s` | AI summary for selected article when focus is in Articles or Content |
| `c` | Copy summary in summary dialog |
| `m` | Save summary as `.md` in summary dialog |

### Feeds
| Key | Action |
|-----|--------|
| `f` | Refresh feed |
| `F` | Refresh all |
| `m` | Feed manager |

### Feed Manager
| Key | Action |
|-----|--------|
| `a` | Add feed |
| `e` / `Enter` | Edit feed |
| `d` | Delete feed |
| `i` | Import OPML |
| `x` | Export OPML |

### App
| Key | Action |
|-----|--------|
| `T` | Theme picker |
| `S` | Settings |
| `?` | Help |
| `q` | Quit |
