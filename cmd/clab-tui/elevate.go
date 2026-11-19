package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// elevate re-executes the current command as root via sudo. It replaces the
// current process so the terminal, signals, and exit code stay intact.
func elevate(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating clab-tui executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("re-running as root: sudo not found in PATH: %w", err)
	}
	return syscall.Exec(sudo, sudoArgv(exe, args), os.Environ())
}

// sudoArgv builds the sudo command line: sudo, the absolute clab-tui path, and
// the original args without --sudo. sudo must receive the resolved absolute
// path; root's PATH may differ.
func sudoArgv(exe string, args []string) []string {
	argv := []string{"sudo", exe}
	return append(argv, stripSudoFlag(args)...)
}

// shouldElevate reports whether --sudo was requested and the process is not
// already root.
func shouldElevate(sudo bool, euid int) bool {
	return sudo && euid != 0
}

// stripSudoFlag removes --sudo and --sudo=... from args so the re-executed
// process does not try to elevate itself again. Everything after a `--`
// delimiter is positional and is preserved verbatim.
func stripSudoFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for i, arg := range args {
		if arg == "--" {
			out = append(out, args[i:]...)
			return out
		}
		if arg == "--sudo" || strings.HasPrefix(arg, "--sudo=") {
			continue
		}
		out = append(out, arg)
	}
	return out
}
