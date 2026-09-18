# Security Policy

## Supported versions

Only the latest published release is supported with security fixes.

## Reporting a vulnerability

Please do not report security vulnerabilities through public GitHub issues.

Use GitHub's private vulnerability reporting on this repository:
<https://github.com/tianyuxue/clab-tui/security/advisories/new>

Include a description, reproduction steps, affected version, and impact. We will
acknowledge reports as soon as possible and coordinate a fix and disclosure.

## Scope notes

clab-tui is a local terminal UI. It shells out to `containerlab`, the Docker CLI,
and `tc`/`docker exec`; with `sudo` or specific Linux capabilities it can run packet
capture and eBPF tracing. Reports about privilege escalation, command injection via
lab file contents, or unsafe handling of captured data are in scope. Vulnerabilities
in containerlab, Docker, or the Linux kernel itself should be reported upstream.
