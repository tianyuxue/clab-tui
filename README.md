# clab-tui

[![tests](https://github.com/tianyuxue/clab-tui/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/tianyuxue/clab-tui/actions/workflows/test.yml)
[![Integration](https://github.com/tianyuxue/clab-tui/actions/workflows/integration.yml/badge.svg?branch=main)](https://github.com/tianyuxue/clab-tui/actions/workflows/integration.yml)
[![Release](https://github.com/tianyuxue/clab-tui/actions/workflows/release.yml/badge.svg)](https://github.com/tianyuxue/clab-tui/actions/workflows/release.yml)
![Go](https://img.shields.io/badge/go-1.25-00ADD8)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

> A terminal UI for [containerlab](https://containerlab.dev) — browse, deploy, and
> operate container-based network labs without leaving the keyboard.

**English** | [简体中文](README.zh-CN.md)

<!-- TODO screenshot: hero view of the topology tab
![clab-tui topology view](docs/assets/screenshots/topology.png)
-->

## Features

- **Lab lifecycle**: discover `.clab.yml` files, then deploy, destroy, and redeploy.
- **Topology tree**: role-grouped device tree (spine/leaf/border/fw/...) with live
  node and link state, interfaces, and a detail pane.
- **Node control**: start, stop, restart, pause, and unpause nodes.
- **Sessions**: embedded terminal sessions (`docker exec`) inside the TUI, with
  insert/normal modes.
- **Logs and stats**: live node logs, CPU/memory I/O stats, and per-interface counters.
- **Netem**: apply and clear delay, jitter, loss, rate, and corruption on interfaces.
- **Packet capture and trace**: eBPF capture on ingress/egress with full libpcap
  filters, plus a live path trace across a lab.
- **Graph view**: open a hierarchical topology graph in the browser.
- **Tabbed workspace**: switch between Topology, Sessions, Node Logs, and Ops Log
  without leaving the terminal.
- **Live updates**: refresh topology state and business-interface IPs automatically,
  or trigger a refresh manually; the default interval is 30 seconds.
- **Keyboard-driven**: vim-style navigation, context-aware help, and a modal `space`
  which-key menu that exposes the actions available in the current view.

## Requirements

clab-tui does **not** bundle containerlab. Install these first:

- **Linux** (uses Linux namespaces, eBPF, and the Docker CLI).
- **[containerlab](https://containerlab.dev/install/)** available as `containerlab` on `PATH`.
- **Docker** with a running daemon.

Some features need elevated privileges; see [Permissions](docs/permissions.md).
Building from source additionally requires **Go 1.25** and the C libraries listed below.

## Install

### Prebuilt binary (linux/amd64)

Download the latest archive from the
[Releases](https://github.com/tianyuxue/clab-tui/releases) page, verify it, and put
the binary on your `PATH`:

```bash
curl -fsSLO "https://github.com/tianyuxue/clab-tui/releases/latest/download/clab-tui_linux_amd64.tar.gz"
curl -fsSLO "https://github.com/tianyuxue/clab-tui/releases/latest/download/checksums.txt"
sha256sum -c checksums.txt
tar -xzf "clab-tui_linux_amd64.tar.gz"
sudo install -m 0755 "clab-tui_linux_amd64/clab-tui" /usr/local/bin/clab-tui
```

### Build from source

Static build (recommended; links libc, libpcap, and libcap statically):

```bash
sudo apt-get install -y libpcap-dev libcap-dev libsystemd-dev \
  libdbus-1-dev libibverbs-dev libnl-3-dev libnl-route-3-dev pkg-config
make build
```

If the static dependencies are unavailable, use the dynamic fallback:

```bash
make build-dynamic
```

## Quick start

```bash
# Scan a directory for labs (defaults to the current directory) and pick one in the UI
clab-tui --dir /path/to/labs

# Open a single lab directly
clab-tui --file /path/to/lab.clab.yml
```

Typical flow: pick a lab → `l` → Deploy → inspect the topology tree → press `o` for
node or interface actions (shell session, lifecycle control, logs, packet capture,
and Netem) → use the tabs to inspect sessions and logs → press `space` for the
context-aware which-key menu. See [Getting started](docs/getting-started.md).

## Permissions

clab-tui is usable without root for browsing and Docker-backed operations. Privileged
operations show a toast and are not executed when requirements are unmet — it never
fails silently. See [Permissions](docs/permissions.md) for the full model and how to
grant each capability.

## Documentation

- [Documentation index](docs/index.md)
- [Getting started](docs/getting-started.md)
- [Key bindings](docs/keybindings.md)
- [Permissions](docs/permissions.md)
- [Troubleshooting](docs/troubleshooting.md)

## Key bindings

The shortcuts below cover the most common actions. Press `?` for the expanded help
bar, or `space` for the modal menu for the current tab. In the tables, a label such
as “Topology” means the shortcut is active on that tab.

### Global

| Key | Action |
|-----|--------|
| `q` / `ctrl+c` | Quit (when no session or capture pane is using the key) |
| `tab` / `shift+tab` | Next / previous tab |
| `1`–`4` | Jump to Topology / Sessions / Node Logs / Ops Log |
| `space` | Open the context-aware which-key menu |
| `?` | Toggle the expanded help bar |

### Topology

| Key | Action |
|-----|--------|
| `j` / `k` | Move down / up |
| `g` / `G` | Jump to top / bottom |
| `ctrl+f` / `ctrl+b` | Page down / up |
| `enter` | Toggle the selected node's connections |
| `x` / `i` | Toggle all connection summaries / interface lines |
| `/` / `n` / `N` | Search / next / previous match |
| `t` / `f` | Start or stop path trace / change trace filter |
| `l` | Open lab actions: switch, deploy, destroy, redeploy, edit, or graph |
| `o` | Open node or interface actions |

### Logs and packet capture

| Key | Action |
|-----|--------|
| `f` | Toggle follow (Node Logs or Ops Log) |
| `c` | Clear (Node Logs or Ops Log) |
| `g` / `G` | Jump to top / bottom (Node Logs) |
| `q` / `c` / `p` / `w` | Close / clear / pause / save pcap (capture pane) |

### Sessions

Sessions start in **insert mode**, where every key is sent to the active shell,
including `ctrl+c` and `tab`. Press `ctrl+\` or double-tap `Esc` to enter normal
mode. In normal mode:

| Key | Action |
|-----|--------|
| `enter` / `space` | Return to insert mode |
| `s` | Open the session picker |
| assigned label (`a`–`z`) | Switch to the labeled session |
| `q` | Close the active session |
| `j` / `k` | Scroll the terminal viewport |

Full reference: [Key bindings](docs/keybindings.md).

## Project status

clab-tui is under active development. Interfaces and key bindings may change between
minor versions. See [CHANGELOG.md](CHANGELOG.md) and the
[Releases](https://github.com/tianyuxue/clab-tui/releases) page.

## Contributing

Issues and pull requests are welcome. Please read the issue and pull request
templates, and run `go test ./...` and `go vet ./...` before opening a PR.

## License

[Apache-2.0](LICENSE).

## Acknowledgements

- [containerlab](https://containerlab.dev) — the lab orchestration this UI drives.
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) and
  [Lip Gloss](https://github.com/charmbracelet/lipgloss) — the TUI stack.
- [vscode-containerlab](https://github.com/srl-labs/vscode-containerlab) — inspiration
  for a keyboard-first containerlab workflow.
