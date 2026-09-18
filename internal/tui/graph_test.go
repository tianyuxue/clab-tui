package tui

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestGraphAddrDefault(t *testing.T) {
	os.Unsetenv("CLAB_TUI_GRAPH_PORT")
	if got := graphAddr(); got != ":50080" {
		t.Fatalf("graphAddr() = %q, want :50080", got)
	}
}

func TestGraphAddrEnvOverride(t *testing.T) {
	t.Setenv("CLAB_TUI_GRAPH_PORT", "8081")
	if got := graphAddr(); got != ":8081" {
		t.Fatalf("graphAddr() = %q, want :8081", got)
	}
}

func TestGraphURL(t *testing.T) {
	if got := graphURL(":50080"); got != "http://localhost:50080" {
		t.Fatalf("graphURL() = %q, want http://localhost:50080", got)
	}
}

func TestGraphCommandArgs(t *testing.T) {
	t.Setenv("CLAB_BIN", "/fake/clab")
	cmd := graphCommand("/tmp/lab.clab.yml", ":50080")
	want := []string{"/fake/clab", "graph", "-t", "/tmp/lab.clab.yml", "-s", ":50080"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("args = %v, want %v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
	if cmd.Path != "/fake/clab" {
		t.Fatalf("cmd.Path = %q, want /fake/clab", cmd.Path)
	}
}

func TestGraphCommandDefaultBinary(t *testing.T) {
	os.Unsetenv("CLAB_BIN")
	cmd := graphCommand("/tmp/lab.clab.yml", ":50080")
	if cmd.Args[0] != "containerlab" {
		t.Fatalf("default binary = %q, want containerlab", cmd.Args[0])
	}
}

func TestCmdlineMatchesGraph(t *testing.T) {
	cases := []struct {
		cmdline string
		want    bool
	}{
		{"containerlab graph -t /x.clab.yml -s :50080", true},
		{"/usr/bin/containerlab graph -t /x.clab.yml", true},
		{"clab-tui -dir /tmp", false},
		{"vim containerlab.graph", false},
	}
	for _, c := range cases {
		if got := cmdlineMatchesGraph(c.cmdline); got != c.want {
			t.Fatalf("cmdlineMatchesGraph(%q) = %v, want %v", c.cmdline, got, c.want)
		}
	}
}

func TestWaitPortReady(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	addr := ":" + port

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := waitPortReady(ctx, addr); err != nil {
		t.Fatalf("waitPortReady(%s) = %v, want nil", addr, err)
	}
}

func TestWaitPortReadyTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if err := waitPortReady(ctx, ":59999"); err == nil {
		t.Fatal("waitPortReady on closed port = nil, want error")
	}
}

func TestStopIdempotent(t *testing.T) {
	s := newGraphServer()
	s.stop()
	s.stop()
}

func TestGraphServerStartErrorPath(t *testing.T) {
	t.Setenv("CLAB_BIN", "/nonexistent/clab-binary")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	s := newGraphServer()
	if _, err := s.start("/tmp/lab.clab.yml"); err == nil {
		t.Fatal("start with missing binary = nil, want error")
	}
}

func TestCmdlineMatchesGraphCurrentProcess(t *testing.T) {
	if cmdlineMatchesGraph(processCmdline(os.Getpid())) {
		t.Fatal("current process cmdline wrongly matched as containerlab graph")
	}
}

func TestSweepRemovesStalePIDFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	pid := os.Getpid() + 999999
	if err := os.WriteFile(graphPIDFile(), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		t.Fatal(err)
	}
	sweepGraphOrphans(":50080")
	if _, err := os.Stat(graphPIDFile()); !os.IsNotExist(err) {
		t.Fatalf("stale pid file still present after sweep: %v", err)
	}
}
