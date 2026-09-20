# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases before the public launch are not published; see the
[GitHub Releases](https://github.com/tianyuxue/clab-tui/releases) page for published versions.

## [Unreleased]

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
