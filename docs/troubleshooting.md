# Troubleshooting

**English** | [简体中文](zh-CN/troubleshooting.md)

## clab-tui reports it cannot find containerlab

`clab-tui` resolves `containerlab` from `PATH` unless `containerlab.bin_path` is set in
the config file. Install containerlab or set the path explicitly.

## Deploy, destroy, or trace shows a permission notice

This is expected when requirements are unmet. See [Permissions](permissions.md) for how
to grant Docker access, containerlab SUID, or trace capabilities.

## "limited mode" in the status bar

containerlab privileges are unavailable, so live interface state cannot be read.
Browsing still works. Run with `sudo` or enable containerlab SUID for full data.

## Links show UNKNOWN

`UNKNOWN` means live interface data is unavailable (usually the limited-mode case),
not that the link is down. See [Permissions](permissions.md).

## Docker permission denied

Add your user to the `docker` group and re-login, or run with `sudo`:

```bash
sudo usermod -aG docker "$USER"
```

## Static build fails with missing pkg-config libraries

The static build requires development packages. On Debian/Ubuntu:

```bash
sudo apt-get install -y libpcap-dev libcap-dev libsystemd-dev \
  libdbus-1-dev libibverbs-dev libnl-3-dev libnl-route-3-dev pkg-config
```

Or use `make build-dynamic` if the static libraries are not available.

## Graph view does not open a browser

clab-tui prints the graph URL as a toast and opens it with `xdg-open`. In headless or
remote environments the browser cannot open; visit the printed
`http://localhost:<port>` URL manually. The port defaults to `50080` and can be changed
with `CLAB_TUI_GRAPH_PORT`.

If the port is already in use, clab-tui reports a graph error; stop the conflicting
process or set a different `CLAB_TUI_GRAPH_PORT`.

## Config file is ignored

The config lives at `$XDG_CONFIG_HOME/clab-tui/config.yaml`, or
`~/.config/clab-tui/config.yaml` when `XDG_CONFIG_HOME` is unset. A parse error is
printed to stderr and defaults are used. Only `containerlab.bin_path` currently has an
effect; `timeout` and `theme` are reserved.

## Edit YAML opens the wrong editor

The editor comes from `$EDITOR` (default `vi`). Set `EDITOR` to your preferred editor.
