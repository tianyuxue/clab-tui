# Getting started

**English** | [简体中文](zh-CN/getting-started.md)

## Prerequisites

clab-tui does not bundle containerlab. Install these first:

- **Linux**.
- **[containerlab](https://containerlab.dev/install/)** on `PATH`.
- **Docker** with a running daemon.
- For building from source: **Go 1.25**.

## Install

### Option A: prebuilt binary (linux/amd64)

Download from the [Releases](https://github.com/tianyuxue/clab-tui/releases) page,
verify the checksum, and install:

```bash
VERSION=v0.1.0
curl -fsSLO "https://github.com/tianyuxue/clab-tui/releases/download/${VERSION}/clab-tui_${VERSION}_linux_amd64.tar.gz"
curl -fsSLO "https://github.com/tianyuxue/clab-tui/releases/download/${VERSION}/checksums.txt"
sha256sum -c checksums.txt
tar -xzf "clab-tui_${VERSION}_linux_amd64.tar.gz"
sudo install -m 0755 "clab-tui_${VERSION}_linux_amd64/clab-tui" /usr/local/bin/clab-tui
```

### Option B: build from source

The default build is static and links libc, libpcap, and libcap statically. On
Debian/Ubuntu install the development packages first:

```bash
sudo apt-get install -y libpcap-dev libcap-dev libsystemd-dev \
  libdbus-1-dev libibverbs-dev libnl-3-dev libnl-route-3-dev pkg-config
make build
```

If the static libraries are unavailable, use the dynamic fallback:

```bash
make build-dynamic
```

<!-- TODO screenshot: static build output
![Building clab-tui](assets/screenshots/getting-started-build.png)
-->

## Configuration

clab-tui reads `$XDG_CONFIG_HOME/clab-tui/config.yaml`, falling back to
`~/.config/clab-tui/config.yaml`. Defaults are used when the file is absent.

```yaml
containerlab:
  bin_path: ""    # path to the containerlab binary; empty = resolve from PATH
  timeout: 300    # reserved
theme: default     # reserved
```

Only `containerlab.bin_path` is currently honored.

## Run

```bash
# Scan a directory for .clab.yml / .clab.yaml files (defaults to the current directory)
clab-tui --dir /path/to/labs

# Open a single lab directly, skipping the startup picker
clab-tui --file /path/to/lab.clab.yml
```

`--dir` and `--file` are mutually exclusive. On startup clab-tui prints how many lab
files it found to stderr.

<!-- TODO screenshot: lab picker
![Lab picker](assets/screenshots/getting-started-lab-picker.png)
-->

## Basic workflow

1. **Pick a lab.** With `--dir`, the lab picker opens at startup. Select a lab and
   press `Enter`. Use `l` → Switch Lab at any time to change labs.
2. **Deploy.** Press `l` → Deploy. Output streams into the Ops Log tab. This requires
   containerlab privileges (see [Permissions](permissions.md)).
3. **Explore the topology.** The Topology tab shows a role-grouped device tree on the
   left and a detail pane on the right. Move with `j`/`k`, expand a node's connections
   with `Enter`, toggle all connections with `x`, and toggle interfaces with `i`.

   <!-- TODO screenshot: topology tree with detail pane
   ![Topology tree](assets/screenshots/getting-started-topology.png)
   -->
4. **Operate on nodes and interfaces.** Press `o` to open the actions menu. For a node:
   SSH/shell, Start, Stop, Restart, Pause, Unpause, View Logs. For an interface:
   Packet Capture, Set Netem, Clear Netem.

   <!-- TODO screenshot: node actions menu
   ![Node actions](assets/screenshots/getting-started-node-actions.png)
   -->
5. **Open a session.** Choose SSH to open an embedded shell session. Press `ctrl+\` for
   normal mode, `s` to switch sessions, and `q` in normal mode to close.

   <!-- TODO screenshot: embedded session
   ![Session](assets/screenshots/getting-started-session.png)
   -->
6. **Capture traffic.** On an interface, choose Packet Capture, pick a filter preset
   (BGP, OSPF, ICMP, ARP, All, or Custom), and press `w` in the capture pane to save a
   pcap. Requires `CAP_BPF`-class privileges.

   <!-- TODO screenshot: capture pane
   ![Capture pane](assets/screenshots/capture.png)
   -->
7. **Destroy.** Press `l` → Destroy when finished.

## Next steps

- [Key bindings](keybindings.md)
- [Permissions](permissions.md)
- [Troubleshooting](troubleshooting.md)
