# Tide

A terminal RSS reader built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss).

## Features

- Three-pane layout: feeds, articles, content
- Live theme switching with full preview
- Feed manager: add, edit, delete, import/export OPML
- Article search and filter
- Mark read/unread, open in browser
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
| `?` | Help |
| `q` | Quit |
