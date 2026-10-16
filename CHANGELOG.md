# Changelog

All notable changes to Tide are documented in this file. The same notes are
bundled into the app: Settings → Changelog.

## Unreleased

### Added

- **All Unread** row under Saved: unread articles from every local feed, newest first, with the
  total as its badge. Selecting a **folder** now lists its feeds' articles merged the same way
  (following the `u` unread-only toggle) instead of clearing the pane, and each row in a merged
  list names its feed. `f` refreshes the feeds in view; `R` on All Unread marks everything read
  after a second press.
- **Mute keywords**: Settings → Feeds → *Mute titles containing* (`feed.mute_keywords`). A newly
  fetched article whose title contains one of the comma-separated keywords, ignoring case, is
  stored already read. Articles already in your library are left alone.
- **Command line**: `tide --refresh`, `--unread-count`, `--bar` and `--help` run without opening
  the reader, and are safe to run while it is open. `--bar` prints Waybar-style JSON for a status
  bar.
- **Omarchy bar**: on Omarchy, Settings → Feeds → *Show in Omarchy bar* adds or removes Tide's
  unread count in the bar. Your `shell.json` is backed up first.
- **Settings → Changelog**: read these release notes inside the app. They are bundled with the
  build, so they work offline and always match the installed version.
- **Experimental Windows builds** (x86_64 and ARM64) are attached to each release as zip files
  holding `tide.exe`, and the release workflow now vets the code for Windows. In-app updates are not
  available on Windows yet, so download new releases by hand.
- **Drop shadow** behind dialogs (Settings, search, the theme picker and the rest): a darkened sliver
  blended into whatever is behind it, so it works over any theme. It steps down further on a light
  theme than on a near-black one, and stays off on the plain-ASCII VT52 theme. On by default;
  Settings → Display → Panes → *Modal drop shadow* (`display.shadow`) turns it off.

### Changed

- Settings groups its fields: Display is now Appearance, Panes, Reading and Behavior, and Feeds is
  Omarchy bar, Refresh, Library and Filtering. The panel is wider so hints fit on one line.
- In the Settings sidebar only the selected section is capitalised, as in tidemail; the others are
  title case.
- The in-app help (`?`) and the README now cover All Unread, folder views, mute keywords, the
  command line, the Omarchy bar, the Changelog and the drop shadow.
- The README said the article database lives in `~/.config/rss/`; it is in
  `~/.local/share/rss/` (`XDG_DATA_HOME`), with only the config in `~/.config/rss/`.

### Fixed

- Read articles were never loaded for local feeds, so the `u` toggle and
  `display.default_unread_only` could not show them and a fully read feed looked empty (#11). `u`
  also now works while the feed list has focus.
- Two Tide processes can share the database, for example `tide --refresh` from cron alongside the
  reader, instead of failing with a locked-database error.

## v1.2.1

### Added

- A release console (`deploy.sh`) and AUR packaging, so Tide installs on Arch Linux as
  [`tide-bin`](https://aur.archlinux.org/packages/tide-bin).

## v1.2.0

### Added

- **Background refresh**: feeds refresh on their own every 30 minutes by default
  (`feed.refresh_interval_minutes`; `0` turns it off). Fetches run through a queue capped at four
  at a time, and each feed's own last-fetched time decides when it is due.
- **Article expiry**: Settings → Feeds → *Delete read after (days)* (`feed.retention_days`) drops
  read articles past that age. Off by default, and it never touches starred, summarized or unread
  articles.
- **Vim-style selection** in the content pane: `v` selects by character, `V` by line, `c` / `y`
  copies the selection or the whole article, and `L` copies the article's link.
- **Pane borders**: each pane is drawn in its own square or rounded frame that follows focus
  (Settings → Display).
- The selected article row is marked with an arrow, as the feeds pane already did.

### Fixed

- Saving settings no longer drops the sidebar back on Saved and the article list back at the top;
  it returns to the feed, article and scroll position you left.

## Earlier releases

### Notes

- Notes for versions before v1.2.0 are on the GitHub releases page.
