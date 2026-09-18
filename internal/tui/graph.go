package tui

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const defaultGraphPort = "50080"

// graphAddr returns the listen address for the graph HTTP server, e.g. ":50080".
// CLAB_TUI_GRAPH_PORT overrides the default port.
func graphAddr() string {
	port := os.Getenv("CLAB_TUI_GRAPH_PORT")
	if port == "" {
		port = defaultGraphPort
	}
	return ":" + port
}

// graphPort returns the port number portion of a ":50080" style address.
func graphPort(addr string) string {
	return strings.TrimPrefix(addr, ":")
}

// graphURL returns the browser URL for a ":50080" style address.
func graphURL(addr string) string {
	return "http://localhost:" + graphPort(addr)
}

// graphCommand builds the containerlab graph invocation. CLAB_BIN overrides the
// binary (also used by tests to point at a fake).
func graphCommand(topoPath, addr string) *exec.Cmd {
	bin := os.Getenv("CLAB_BIN")
	if bin == "" {
		bin = "containerlab"
	}
	return exec.Command(bin, "graph", "-t", topoPath, "-s", addr)
}

// graphPIDFile returns the path where the spawned graph server PID is stored so
// orphans can be swept on the next launch.
func graphPIDFile() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "clab-tui-graph.pid")
}

// cmdlineMatchesGraph reports whether a /proc/<pid>/cmdline string belongs to a
// containerlab graph server (guards against PID reuse killing a wrong process).
func cmdlineMatchesGraph(cmdline string) bool {
	isClab := false
	hasGraph := false
	for _, arg := range strings.Fields(cmdline) {
		switch {
		case filepath.Base(arg) == "containerlab":
			isClab = true
		case arg == "graph":
			hasGraph = true
		}
	}
	return isClab && hasGraph
}

// graphServer tracks a running containerlab graph HTTP server.
type graphServer struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	pidFile string
	addr    string
}

// graphLauncher is the interface the Model uses to start/stop the graph
// server. *graphServer implements it; tests inject a stub.
type graphLauncher interface {
	start(topoPath string) (string, error)
	stop()
}

func newGraphServer() *graphServer {
	return &graphServer{
		pidFile: graphPIDFile(),
		addr:    graphAddr(),
	}
}

// start spawns the graph server for the given topology and returns its URL.
// It stops any previously tracked server and sweeps orphaned ones first.
func (s *graphServer) start(topoPath string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	sweepGraphOrphans(s.addr)

	cmd := graphCommand(topoPath, s.addr)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start graph server: %w", err)
	}
	s.cmd = cmd

	if err := os.WriteFile(s.pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		s.stopLocked()
		return "", fmt.Errorf("write graph pid file: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := waitPortReady(ctx, s.addr); err != nil {
		s.stopLocked()
		return "", fmt.Errorf("graph server not ready: %w", err)
	}
	return graphURL(s.addr), nil
}

// stop kills the tracked server and removes the pid file. Idempotent.
func (s *graphServer) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

// stopLocked performs the stop assuming s.mu is held.
func (s *graphServer) stopLocked() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
		s.cmd = nil
	}
	_ = os.Remove(s.pidFile)
}

// waitPortReady polls until addr accepts TCP connections or ctx expires.
func waitPortReady(ctx context.Context, addr string) error {
	target := "localhost" + addr
	d := net.Dialer{Timeout: 250 * time.Millisecond}
	for {
		conn, err := d.DialContext(ctx, "tcp", target)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// processCmdline returns the space-joined cmdline of pid (empty if unreadable).
func processCmdline(pid int) string {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(string(b), "\x00", " ")
}

// sweepGraphOrphans kills any leftover containerlab graph server: first the one
// tracked in the pid file (if its cmdline matches), then any process listening
// on the address whose cmdline matches. Always removes the pid file.
func sweepGraphOrphans(addr string) {
	if b, err := os.ReadFile(graphPIDFile()); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
			if cmdlineMatchesGraph(processCmdline(pid)) {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		_ = os.Remove(graphPIDFile())
	}
	killGraphOnPort(addr)
}

// killGraphOnPort finds and kills a containerlab graph process listening on
// addr via `ss -ltnp`. Best-effort: silently returns if ss is missing.
func killGraphOnPort(addr string) {
	out, err := exec.Command("ss", "-ltnp").Output()
	if err != nil {
		return
	}
	port := graphPort(addr)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, ":"+port+" ") && !strings.Contains(line, ":"+port+"\t") {
			continue
		}
		i := strings.Index(line, "pid=")
		if i < 0 {
			continue
		}
		rest := line[i+4:]
		pidStr := ""
		for _, r := range rest {
			if r >= '0' && r <= '9' {
				pidStr += string(r)
			} else {
				break
			}
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 {
			continue
		}
		if cmdlineMatchesGraph(processCmdline(pid)) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}
