# Permissions

**English** | [简体中文](zh-CN/permissions.md)

clab-tui works without root for most browsing and Docker-backed operations. Features
that need elevated privileges are checked up front; when a requirement is unmet,
clab-tui shows a notice and does **not** run the operation instead of failing silently.

## Requirement levels

| Operations | Requirement |
|------------|-------------|
| Browse topologies, open the graph view | None |
| Sessions, node logs, stats, node start/stop/restart/pause/unpause, netem, interface IPs | Docker access |
| Deploy, destroy, redeploy, live event stream, `inspect interfaces` | containerlab privileges |
| Packet capture and path trace | Trace capabilities |

## How capabilities are detected

On startup clab-tui checks:

- **root** — effective UID is 0.
- **Docker group** — your user is in the `docker` group.
- **containerlab SUID** — the `containerlab` binary has the setuid bit.
- **Linux capabilities** — `CAP_BPF`, `CAP_NET_ADMIN`, `CAP_SYS_ADMIN`, `CAP_SYS_PTRACE`.
- **BTF** — `/sys/kernel/btf/vmlinux` exists (required for eBPF).

## Granting access

### Docker access

Add your user to the `docker` group and re-login:

```bash
sudo usermod -aG docker "$USER"
```

Alternatively, run clab-tui with `sudo`.

### Deploy / destroy / events

containerlab needs root unless its binary is setuid. Either run clab-tui with `sudo`,
or enable SUID:

```bash
sudo chmod u+s "$(command -v containerlab)"
```

### Packet capture and trace

Tracing needs one of:

- run clab-tui with `sudo`; or
- grant the four capabilities and have kernel BTF available.

If the requirements are unmet, starting a trace or capture shows:

> packet tracing needs root (run with sudo) or CAP_BPF+CAP_NET_ADMIN+CAP_SYS_ADMIN+CAP_SYS_PTRACE

## Limited mode

When containerlab privileges are unavailable, the status bar shows `limited mode` and
live interface state may be unavailable. Links without live data are shown as
`UNKNOWN` rather than `DOWN` so a stale or missing reading is never mistaken for a
broken link.

## Notes

- Running with `sudo` changes the user that owns configuration and generated files.
- SUID on `containerlab` grants privilege to anyone who can execute it; only do this on
  a machine you trust.
- Docker group membership is effectively root-equivalent on the host.
