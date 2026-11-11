# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases before the public launch are not published; see the
[GitHub Releases](https://github.com/tianyuxue/clab-tui/releases) page for published versions.

## [Unreleased]

## [0.2.0] - 2026-11-11

### Added

- Background business-interface IP refresh for the topology tree. On lab load,
  deploy/redeploy, and edit-YAML reload, all running `linux` / SR Linux nodes
  are refreshed with a bounded worker pool; the tree keeps cached IPs for
  unselected nodes.
- `topology.auto_refresh` config option (default `true`) and a topology
  `Refresh` which-key group with `Refresh Now` and `Auto Refresh: on/off`.
  Auto refresh runs every 30 seconds; the which-key toggle is session-only.
- Status bar chips: `↻ 30s` / `↻ off` / `↻ …` for refresh state, `●` for
  tracing, and `⚠` for limited mode.

### Changed

- Background/full refreshes collect IPs only; selecting a node still collects
  monitor data, interface stats, and IPs.
- Slow/CLI-only kinds (cEOS/XRd) are excluded from background refresh until
  their commands and startup cost are verified.

### Fixed

- Topology tree `j`/`k` no longer scroll the viewport while the cursor is
  still inside it; the page scrolls only at the top/bottom edge. `ctrl+f` /
  `ctrl+b` now move the cursor and viewport by a page instead of being snapped
  back.
- A topology search with no matches clears after 3 seconds instead of staying
  in the tree footer forever.

## [0.1.4] - 2026-09-21

### Changed

- Release artifacts now use stable, version-free names
  (`clab-tui_linux_amd64.tar.gz` and `checksums.txt`), and the install
  documentation links to `releases/latest/download` so it always installs the
  newest release without a per-release version bump.

## [0.1.3] - 2026-09-21

### Added

- Sessions tab: each session tab shows the device (node) name and a stable
  switch label assigned in open order (`a`, `b`, `c`, ...), so sessions are
  easy to tell apart and switch with one key.
- Sessions tab: `esc esc` (double-tap) switches to normal mode, with a status
  bar prompt after the first `esc`; `ctrl+\` remains available as a fallback.
- Switching labs now closes the sessions of the previous lab, so a session
  never outlives its lab.

### Changed

- Opening a session always starts in insert mode; switching to an existing
  session keeps the current mode, so navigation stays in normal mode.
- Session switch labels now come from a fixed letter pool instead of being
  derived from the device name, removing collisions for names like `sw1`/`sw2`.

### Fixed

- Sessions tab: session tabs no longer show the long container name.
- Sessions tab: the switch label uses the same red as the top tab digits.

## [0.1.2] - 2026-09-20

### Added

- Topology space menu now lists "Toggle Details" (`x`) after Search, so the
  expand/collapse shortcut is discoverable.

### Fixed

- Sessions tab: the normal-mode hint now shows `enter` to return to insert mode
  (previously it incorrectly showed `ctrl+\`).
- Sessions tab: the selected session is highlighted with the same
  foreground-only style as the active tab, so the current session is clear.
- Sessions tab: the terminal cursor renders as a solid white block instead of a
  grey block that was hard to see.

## [0.1.1] - 2026-09-19

### Added

- `make install` and `make uninstall` targets. The binary installs to
  `~/.local/bin` by default; override with `BINDIR=/path`.

### Fixed

- Node detail pane now shows the full container image reference (e.g.
  `ghcr.io/nokia/srlinux:24.7.1`) instead of only the image tag.
- Undeployed labs report node state `stopped` (and links `down`) instead of
  `unknown`.
- Node state is re-read from `containerlab inspect` after lifecycle operations
  (start/stop/restart/deploy), so it stays accurate even when the event stream
  is unavailable, such as a non-root containerlab without SUID.

## [0.1.0] - 2026-09-18

### Added

- English and Simplified Chinese project documentation under `docs/`.
- `--version` flag and build-time version injection.
- Automated release workflow that publishes a static `linux/amd64` binary on `v*` tags.
