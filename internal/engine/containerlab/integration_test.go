//go:build integration

package containerlab

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

const traceIntegrationImage = "nicolaka/netshoot:v0.13"

// TestIntegrationDeploySmoke deploys a 2-node linux lab, verifies the snapshot
// converges, then destroys it. Requires containerlab (SUID root) + docker.
// Run with: go test -tags integration ./internal/engine/containerlab/ -run TestIntegrationDeploySmoke -v -timeout 180s
func TestIntegrationDeploySmoke(t *testing.T) {
	if os.Getenv("CLAB_TUI_SKIP_INTEGRATION") != "" {
		t.Skip("CLAB_TUI_SKIP_INTEGRATION set")
	}
	bin := os.Getenv("CLAB_BIN")
	if bin == "" {
		bin = "containerlab"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skip("containerlab not in PATH")
	}

	dir := t.TempDir()
	topo := filepath.Join(dir, "smoke.clab.yml")
	content := `name: smoke
topology:
  nodes:
    r1:
      kind: linux
      image: publicmirror.azurecr.io/debian:bookworm
      group: core
    r2:
      kind: linux
      image: publicmirror.azurecr.io/debian:bookworm
      group: leaf
  links:
    - endpoints: ["r1:eth1", "r2:eth1"]
`
	if err := os.WriteFile(topo, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := New(WithBinary(bin), WithLabDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	registerIntegrationCleanup(t, e, "smoke")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	ch, err := e.Deploy(ctx, topo)
	if err != nil {
		t.Fatal(err)
	}
	deployOut, code := collectOutput(t, ch)
	if code != 0 {
		t.Fatalf("deploy failed (exit %d):\n%s", code, deployOut)
	}

	// Trigger lazy start (event stream + inspect seeding) so the store fills.
	labs, err := e.ListLabs(ctx)
	if err != nil {
		t.Fatalf("ListLabs: %v", err)
	}
	t.Logf("labs after deploy: %+v", labs)

	// Wait for snapshot to converge to 2 running nodes.
	deadline := time.Now().Add(30 * time.Second)
	for {
		snap := e.Snapshot()
		lab := snap.Labs["smoke"]
		if lab != nil && len(lab.Nodes) == 2 {
			allRunning := true
			for _, n := range lab.Nodes {
				if n.State != engine.StatusRunning {
					allRunning = false
				}
			}
			if allRunning {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("snapshot did not converge: %+v", snap.Labs)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Wait until any node has interfaces.
	deadline = time.Now().Add(30 * time.Second)
	for {
		snap := e.Snapshot()
		if snap.Labs["smoke"] == nil {
			t.Fatal("smoke lab missing")
		}
		found := false
		for _, n := range snap.Labs["smoke"].Nodes {
			if len(n.Interfaces) > 0 {
				found = true
				break
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no node interfaces appeared")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// TestIntegrationSessionOpens verifies OpenSession works against a real node:
// it runs a PTY shell and reads echo output. Requires docker + a deployed lab.
func TestIntegrationSessionOpens(t *testing.T) {
	if os.Getenv("CLAB_TUI_SKIP_INTEGRATION") != "" {
		t.Skip("CLAB_TUI_SKIP_INTEGRATION set")
	}
	bin := os.Getenv("CLAB_BIN")
	if bin == "" {
		bin = "containerlab"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skip("containerlab not in PATH")
	}

	dir := t.TempDir()
	topo := filepath.Join(dir, "ses.clab.yml")
	content := `name: ses
topology:
  nodes:
    r1:
      kind: linux
      image: publicmirror.azurecr.io/debian:bookworm
`
	if err := os.WriteFile(topo, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := New(WithBinary(bin), WithLabDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	registerIntegrationCleanup(t, e, "ses")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Deploy so the node exists.
	ch, err := e.Deploy(ctx, topo)
	if err != nil {
		t.Fatal(err)
	}
	deployOut, code := collectOutput(t, ch)
	if code != 0 {
		t.Fatalf("deploy failed (exit %d):\n%s", code, deployOut)
	}
	h, err := e.OpenSession(ctx, "ses", "r1", engine.SessionModeShell)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer h.Close()

	// Write a command and expect output.
	_, err = h.Stdin.Write([]byte("echo hello-session\n"))
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	var out bytes.Buffer
	for {
		select {
		case chunk, ok := <-h.Output:
			if !ok {
				t.Fatal("session output closed early")
			}
			out.Write(chunk)
			if bytes.Contains(out.Bytes(), []byte("hello-session")) {
				return // echoed back through the PTY
			}
		case <-time.After(time.Until(deadline)):
			t.Fatalf("session output did not contain hello-session; got %q", out.String())
		}
	}
}

// TestIntegrationSessionRawBytesStreams verifies the session output channel
// delivers raw bytes (including ANSI control sequences from a full-screen
// program) untouched, so the UI's terminal emulator can render them.
func TestIntegrationSessionRawBytesStreams(t *testing.T) {
	if os.Getenv("CLAB_TUI_SKIP_INTEGRATION") != "" {
		t.Skip("CLAB_TUI_SKIP_INTEGRATION set")
	}
	bin := os.Getenv("CLAB_BIN")
	if bin == "" {
		bin = "containerlab"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skip("containerlab not in PATH")
	}

	dir := t.TempDir()
	topo := filepath.Join(dir, "ses.clab.yml")
	content := `name: ses
topology:
  nodes:
    r1:
      kind: linux
      image: publicmirror.azurecr.io/debian:bookworm
`
	if err := os.WriteFile(topo, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := New(WithBinary(bin), WithLabDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	registerIntegrationCleanup(t, e, "ses")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	ch, err := e.Deploy(ctx, topo)
	if err != nil {
		t.Fatal(err)
	}
	deployOutput, deployCode := collectOutput(t, ch)
	if deployCode != 0 {
		t.Fatalf("deploy failed (exit %d):\n%s", deployCode, deployOutput)
	}
	h, err := e.OpenSession(ctx, "ses", "r1", engine.SessionModeShell)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer h.Close()

	// Run a command that emits ANSI color, then exit; the escapes must arrive
	// in the raw stream.
	_, _ = h.Stdin.Write([]byte("printf '\\x1b[31mRED\\x1b[0m ansi-test\\n'; exit\n"))
	deadline := time.Now().Add(10 * time.Second)
	var out bytes.Buffer
	for {
		select {
		case chunk, ok := <-h.Output:
			if !ok {
				break
			}
			out.Write(chunk)
			if bytes.Contains(out.Bytes(), []byte("ansi-test")) && bytes.Contains(out.Bytes(), []byte("\x1b[31m")) {
				return
			}
		case <-time.After(time.Until(deadline)):
			t.Fatalf("did not see ansi-test with ESC[31m; got %q", out.String())
		}
	}
}

// TestIntegrationPacketTraceCapable verifies the capability check distinguishes
// root (available) from non-root (unavailable with reason).
func TestIntegrationPacketTraceCapable(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()
	ctx := context.Background()
	r := e.PacketTraceCapable(ctx)
	// As root: available. As non-root: unavailable with a reason.
	t.Logf("PacketTraceCapable: available=%v reason=%q", r.Available, r.Reason)
	if os.Geteuid() == 0 {
		if !r.Available {
			t.Fatalf("expected available as root, got %+v", r)
		}
	} else {
		if r.Available {
			t.Fatal("expected unavailable as non-root")
		}
		if r.Reason == "" {
			t.Fatal("expected a reason when unavailable")
		}
	}
}

// TestIntegrationTracePath verifies TracePath attaches eBPF and reports events
// when traffic flows. Requires root + a deployed lab with inter-node traffic.
// Run: sudo env PATH=$PATH go test -tags integration ./internal/engine/containerlab/ -run TestIntegrationTracePath -v -timeout 180s
func TestIntegrationTracePath(t *testing.T) {
	if os.Getenv("CLAB_TUI_SKIP_INTEGRATION") != "" {
		t.Skip("CLAB_TUI_SKIP_INTEGRATION set")
	}
	bin := os.Getenv("CLAB_BIN")
	if bin == "" {
		bin = "containerlab"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skip("containerlab not in PATH")
	}
	if os.Geteuid() != 0 {
		t.Skip("eBPF attach requires root; run with sudo")
	}

	dir := t.TempDir()
	topo := filepath.Join(dir, "trace.clab.yml")
	content := `name: trace
topology:
  nodes:
    r1:
      kind: linux
      image: ` + traceIntegrationImage + `
    r2:
      kind: linux
      image: ` + traceIntegrationImage + `
  links:
    - endpoints: ["r1:eth1", "r2:eth1"]
`
	os.WriteFile(topo, []byte(content), 0o644)

	e, err := New(WithBinary(bin), WithLabDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	registerIntegrationCleanup(t, e, "trace")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	ch, err := e.Deploy(ctx, topo)
	if err != nil {
		t.Fatal(err)
	}
	deployOutput, deployCode := collectOutput(t, ch)
	if deployCode != 0 {
		t.Fatalf("deploy failed (exit %d):\n%s", deployCode, deployOutput)
	}
	// Trigger lazy-start + store sync so container info is populated.
	_, _ = e.ListLabs(ctx)
	time.Sleep(2 * time.Second)

	// Start trace on the business link (r1-r2 eth1).
	tctx, tcancel := context.WithCancel(context.Background())
	defer tcancel()
	events, stop, err := e.TracePath(tctx, "trace", "")
	if err != nil {
		t.Fatalf("TracePath: %v", err)
	}
	defer stop()

	// Generate traffic on the mgmt network (r1 ping r2's mgmt IP).
	// The mgmt IPs come from containerlab inspect; use the container's own
	// management IP. Simplest: ping via the other container.
	// Note: business-port traffic (eth1) needs IP config; management traffic
	// verifies the attach works at all.
	trafficDone := make(chan error, 1)
	go func() {
		time.Sleep(2 * time.Second)
		// ping r2 from r1 (management plane)
		cmd := exec.Command("docker", "exec", "clab-trace-r1", "ping", "-c", "3", "172.20.20.2")
		trafficDone <- cmd.Run()
	}()

	deadline := time.Now().Add(20 * time.Second)
	count := 0
	trafficFinished := false
	for {
		select {
		case err := <-trafficDone:
			if err != nil {
				t.Fatalf("generate management traffic: %v", err)
			}
			trafficFinished = true
			if count >= 3 {
				return
			}
		case ev, ok := <-events:
			if !ok {
				t.Fatal("trace event channel closed early")
			}
			count++
			t.Logf("event: node=%s iface=%s pkt=%s", ev.Node, ev.Iface, ev.Pkt.Summary)
			if count >= 3 && trafficFinished {
				return // got events — attach works
			}
		case <-time.After(time.Until(deadline)):
			t.Fatalf("no trace events within 20s (got %d)", count)
		}
	}
}

// TestIntegrationTracePathBusinessBidirectional verifies the direction-aware
// trace path on an actual business link. It configures IPs on eth1, generates
// traffic in one direction, and requires all four observations: sender OUT,
// receiver IN, receiver OUT, and sender IN.
// Run with: sudo env PATH=$PATH go test -tags integration ./internal/engine/containerlab/ -run TestIntegrationTracePathBusinessBidirectional -v -timeout 180s
func TestIntegrationTracePathBusinessBidirectional(t *testing.T) {
	if os.Getenv("CLAB_TUI_SKIP_INTEGRATION") != "" {
		t.Skip("CLAB_TUI_SKIP_INTEGRATION set")
	}
	if os.Geteuid() != 0 {
		t.Skip("eBPF attach requires root; run with sudo")
	}
	bin := os.Getenv("CLAB_BIN")
	if bin == "" {
		bin = "containerlab"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skip("containerlab not in PATH")
	}

	dir := t.TempDir()
	topo := filepath.Join(dir, "trace-business.clab.yml")
	content := `name: trace-business
topology:
  nodes:
    r1:
      kind: linux
      image: ` + traceIntegrationImage + `
    r2:
      kind: linux
      image: ` + traceIntegrationImage + `
  links:
    - endpoints: ["r1:eth1", "r2:eth1"]
`
	if err := os.WriteFile(topo, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := New(WithBinary(bin), WithLabDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	registerIntegrationCleanup(t, e, "trace-business")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	deploy, err := e.Deploy(ctx, topo)
	if err != nil {
		t.Fatal(err)
	}
	deployOutput, deployCode := collectOutput(t, deploy)
	if deployCode != 0 {
		t.Fatalf("deploy failed (exit %d):\n%s", deployCode, deployOutput)
	}
	if _, err := e.ListLabs(ctx); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct {
		container string
		address   string
	}{
		{"clab-trace-business-r1", "10.0.0.1/24"},
		{"clab-trace-business-r2", "10.0.0.2/24"},
	} {
		for _, args := range [][]string{
			{"exec", spec.container, "ip", "addr", "add", spec.address, "dev", "eth1"},
			{"exec", spec.container, "ip", "link", "set", "eth1", "up"},
		} {
			if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
				t.Fatalf("docker %v: %v: %s", args, err, out)
			}
		}
	}

	tctx, tcancel := context.WithCancel(context.Background())
	defer tcancel()
	events, stop, err := e.TracePath(tctx, "trace-business", "icmp")
	if err != nil {
		t.Fatalf("TracePath: %v", err)
	}
	defer stop()

	trafficDone := make(chan error, 1)
	go func() {
		time.Sleep(2 * time.Second)
		trafficDone <- exec.Command("docker", "exec", "clab-trace-business-r1", "ping", "-I", "eth1", "-c", "3", "-W", "1", "10.0.0.2").Run()
	}()

	want := map[string]bool{
		"r1/OUT": false,
		"r2/IN":  false,
		"r2/OUT": false,
		"r1/IN":  false,
	}
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	trafficFinished := false
	for {
		select {
		case err := <-trafficDone:
			if err != nil {
				t.Fatalf("generate business traffic: %v", err)
			}
			trafficFinished = true
			all := true
			for _, seen := range want {
				all = all && seen
			}
			if all {
				return
			}
		case ev, ok := <-events:
			if !ok {
				t.Fatal("trace event channel closed before all directions arrived")
			}
			key := ev.Node + "/" + ev.Pkt.Direction
			if _, exists := want[key]; exists {
				want[key] = true
			}
			all := true
			for _, seen := range want {
				all = all && seen
			}
			if all && trafficFinished {
				return
			}
		case <-deadline.C:
			t.Fatalf("missing direction events: %+v", want)
		}
	}
}
