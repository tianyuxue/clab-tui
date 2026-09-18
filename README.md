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
- **Keyboard-driven**: vim-style navigation and a `space` which-key menu.

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
VERSION=v0.1.0
curl -fsSLO "https://github.com/tianyuxue/clab-tui/releases/download/${VERSION}/clab-tui_${VERSION}_linux_amd64.tar.gz"
curl -fsSLO "https://github.com/tianyuxue/clab-tui/releases/download/${VERSION}/checksums.txt"
sha256sum -c checksums.txt
tar -xzf "clab-tui_${VERSION}_linux_amd64.tar.gz"
sudo install -m 0755 "clab-tui_${VERSION}_linux_amd64/clab-tui" /usr/local/bin/clab-tui
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
node actions (shell session, start/stop/restart, logs) → press `space` for the
which-key menu. See [Getting started](docs/getting-started.md).

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

| Key | Action |
|-----|--------|
| `tab` / `shift+tab` | Next / previous tab |
| `1`–`4` | Jump to Topology / Sessions / Node Logs / Ops Log |
| `space` | Open the which-key menu |
| `l` | Lab actions (Topology) |
| `o` | Node / interface actions (Topology) |
| `/` | Search (Topology) |
| `t` / `f` | Toggle trace / change trace filter (Topology) |
| `?` | Toggle the expanded help bar |
| `q` | Quit |

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
