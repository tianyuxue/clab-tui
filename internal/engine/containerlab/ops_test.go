package containerlab

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// newTestEngine builds a ClabEngine backed by a temp dir with one .clab.yml.
func newTestEngine(t *testing.T) *ClabEngine {
	t.Helper()
	dir := t.TempDir()
	topo := filepath.Join(dir, "mini.clab.yml")
	content := `
name: mini
topology:
  nodes:
    r1: {kind: linux}
  links: []
`
	if err := os.WriteFile(topo, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := New(WithBinary("containerlab"), WithLabDir(dir))
	if err != nil {
		t.Skipf("skipping: containerlab binary not available: %v", err)
	}
	return e
}

func TestEngineListLabs(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	labs, err := e.ListLabs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, lab := range labs {
		if lab.Name == "mini" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected lab mini, got %+v", labs)
	}
}

func TestBuildNetemArgs(t *testing.T) {
	args, err := buildNetemArgs("e1-1", engine.NetemState{Delay: "10ms", Jitter: "2ms", Loss: "1%", Rate: "10mbit", Corruption: "0.1%"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tc", "qdisc", "replace", "dev", "e1-1", "root", "netem", "delay", "10ms", "2ms", "loss", "1%", "rate", "10mbit", "corrupt", "0.1%"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", args, want)
	}
}

func TestBuildNetemArgsRejectsUnsafeValues(t *testing.T) {
	if _, err := buildNetemArgs("e1-1", engine.NetemState{Delay: "10ms; rm -rf /"}); err == nil {
		t.Fatal("expected invalid netem value error")
	}
}

func TestEngineGetLabMergesTopology(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	lab, err := e.GetLab(context.Background(), "mini")
	if err != nil {
		t.Fatal(err)
	}
	if lab.Name != "mini" || len(lab.Nodes) != 1 {
		t.Fatalf("unexpected lab: %+v", lab)
	}
}

func TestEngineGetLabUndeployedNodeStopped(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	lab, err := e.GetLab(context.Background(), "mini")
	if err != nil {
		t.Fatal(err)
	}
	if len(lab.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(lab.Nodes))
	}
	if lab.Nodes[0].State != engine.StatusStopped {
		t.Fatalf("undeployed node state = %q, want %q", lab.Nodes[0].State, engine.StatusStopped)
	}
}

func TestMergeLabStateUsesLiveImageWhenStaticEmpty(t *testing.T) {
	static := &engine.Lab{Nodes: []engine.Node{{Name: "r1"}}}
	live := &engine.Lab{Nodes: []engine.Node{{Name: "r1", Image: "debian:bookworm"}}}
	mergeLabState(static, live)
	if static.Nodes[0].Image != "debian:bookworm" {
		t.Fatalf("image = %q, want live image", static.Nodes[0].Image)
	}
}

func TestMergeLabStateKeepsStaticImage(t *testing.T) {
	static := &engine.Lab{Nodes: []engine.Node{{Name: "r1", Image: "custom:1"}}}
	live := &engine.Lab{Nodes: []engine.Node{{Name: "r1", Image: "other:2"}}}
	mergeLabState(static, live)
	if static.Nodes[0].Image != "custom:1" {
		t.Fatalf("image = %q, want static image preserved", static.Nodes[0].Image)
	}
}

func TestEngineMissingLab(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	if _, err := e.GetLab(context.Background(), "nope"); err != engine.ErrLabNotFound {
		t.Fatalf("expected ErrLabNotFound, got %v", err)
	}
}

func TestEngineSnapshotEmptyWithoutSource(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()
	snap := e.Snapshot()
	if len(snap.Labs) != 0 {
		t.Fatalf("expected empty snapshot, got %+v", snap.Labs)
	}
}

func TestEngineOptionalCapabilities(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	var _ engine.LogStreamer = e
	var _ engine.TerminalSessioner = e
	var _ engine.PacketCapturer = e
}

func TestEngineImplementsInterface(t *testing.T) {
	var _ engine.Engine = (*ClabEngine)(nil)
}

func TestStreamLinesSetsExitCode(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "fail.sh")
	script := "#!/bin/sh\necho 'boom' >&2\nexit 3\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ch, err := spawnWithLines(context.Background(), bin)
	if err != nil {
		t.Fatal(err)
	}
	code := -1
	var last engine.OutputLine
	for line := range ch {
		last = line
	}
	if !last.Done || last.Code != 3 {
		t.Fatalf("expected Done sentinel with Code 3, got %+v", last)
	}
	_ = code
}

func TestDeployTimedSpawnErrorReturnsWithoutNilStreamDrain(t *testing.T) {
	e := newTestEngine(t)
	e.bin = filepath.Join(t.TempDir(), "missing-containerlab")
	topo := filepath.Join(e.labDir, "mini.clab.yml")
	ch, err := e.Deploy(context.Background(), topo, engine.WithTimeout(1))
	if err == nil {
		t.Fatal("Deploy returned nil error for a spawn failure")
	}
	if ch != nil {
		t.Fatal("Deploy returned a stream after spawn failure")
	}
}

func TestDeployTimedOutputHasSingleDoneForCaller(t *testing.T) {
	e := newTestEngine(t)
	bin := filepath.Join(t.TempDir(), "containerlab.sh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho deploy-output\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.bin = bin
	topo := filepath.Join(e.labDir, "mini.clab.yml")
	ch, err := e.Deploy(context.Background(), topo, engine.WithTimeout(1))
	if err != nil {
		t.Fatal(err)
	}
	doneCount := 0
	seenOutput := false
	for line := range ch {
		if line.Done {
			doneCount++
			if line.Code != 0 {
				t.Fatalf("deploy Done code = %d, want 0", line.Code)
			}
		} else if line.Line == "deploy-output" {
			seenOutput = true
		}
	}
	if doneCount != 1 {
		t.Fatalf("caller received %d Done sentinels, want exactly one", doneCount)
	}
	if !seenOutput {
		t.Fatal("caller did not receive command output")
	}
}

// fakeDocker installs a fake `docker` binary on PATH that records its args to
// recordFile and exits successfully. Returns the record file path.
func fakeDocker(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	recordFile := filepath.Join(dir, "docker-args.txt")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" > %s\nexit 0\n", recordFile)
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return recordFile
}

// TestNodeControlUsesDocker verifies Start/Stop/Restart/Pause/Unpause dispatch
// to `docker <cmd> <container>` rather than a containerlab subcommand (which
// does not exist in current containerlab builds).
func TestNodeControlUsesDocker(t *testing.T) {
	recordFile := fakeDocker(t)
	e := newTestEngine(t)
	defer e.Close()

	// Seed the store with a stopped node so containerForNode resolves.
	e.store.ApplyContainerEvent(engine.ContainerEvent{
		LabName: "mini", NodeName: "r1", Container: "clab-mini-r1",
		ContainerID: "abc123", State: engine.StatusStopped,
	})

	cases := []struct {
		name string
		fn   func() (<-chan engine.OutputLine, error)
		want string
	}{
		{"start", func() (<-chan engine.OutputLine, error) {
			return e.StartNode(context.Background(), "mini", "r1")
		}, "start clab-mini-r1"},
		{"stop", func() (<-chan engine.OutputLine, error) {
			return e.StopNode(context.Background(), "mini", "r1")
		}, "stop clab-mini-r1"},
		{"restart", func() (<-chan engine.OutputLine, error) {
			return e.RestartNode(context.Background(), "mini", "r1")
		}, "restart clab-mini-r1"},
		{"pause", func() (<-chan engine.OutputLine, error) {
			return e.PauseNode(context.Background(), "mini", "r1")
		}, "pause clab-mini-r1"},
		{"unpause", func() (<-chan engine.OutputLine, error) {
			return e.UnpauseNode(context.Background(), "mini", "r1")
		}, "unpause clab-mini-r1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch, err := tc.fn()
			if err != nil {
				t.Fatalf("call failed: %v", err)
			}
			for line := range ch {
				if line.Done {
					if line.Code != 0 {
						t.Fatalf("command failed: %s", line.Line)
					}
					break
				}
			}
			args, err := os.ReadFile(recordFile)
			if err != nil {
				t.Fatalf("docker was not invoked: %v", err)
			}
			got := strings.TrimSpace(string(args))
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestEngineUndeployedLinksDown(t *testing.T) {
	dir := t.TempDir()
	topo := filepath.Join(dir, "mini.clab.yml")
	content := `
name: mini
topology:
  nodes:
    r1: {kind: linux}
    r2: {kind: linux}
  links:
    - endpoints: ["r1:e1", "r2:e1"]
`
	if err := os.WriteFile(topo, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := New(WithBinary("containerlab"), WithLabDir(dir))
	if err != nil {
		t.Skipf("skipping: containerlab binary not available: %v", err)
	}
	defer e.Close()

	lab, err := e.GetLab(context.Background(), "mini")
	if err != nil {
		t.Fatal(err)
	}
	if len(lab.Links) != 1 || lab.Links[0].State != engine.LinkDown {
		t.Fatalf("expected undeployed links Down, got %+v", lab.Links)
	}
}
