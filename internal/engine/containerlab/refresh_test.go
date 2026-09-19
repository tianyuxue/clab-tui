package containerlab

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// TestRefreshLivePicksUpExternalStateChange verifies that RefreshLive re-reads
// `containerlab inspect` so node state stays accurate when the event stream is
// unavailable (non-root containerlab): after an external stop, a refresh must
// report the node as stopped.
func TestRefreshLivePicksUpExternalStateChange(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state")
	if err := os.WriteFile(stateFile, []byte("running"), 0o644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(dir, "containerlab")
	script := `#!/bin/sh
case "$1" in
  inspect)
    if [ "$2" = "interfaces" ]; then
      echo "[]"
      exit 0
    fi
    st=$(cat "` + stateFile + `" 2>/dev/null || echo unknown)
    printf '{"mini":[{"lab_name":"mini","name":"clab-mini-r1","container_id":"abc123","image":"debian:bookworm","kind":"linux","state":"%s","status":"fixture","ipv4_address":"172.20.20.3/24"}]}\n' "$st"
    exit 0
    ;;
  events)
    exit 0
    ;;
esac
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	topo := filepath.Join(dir, "mini.clab.yml")
	if err := os.WriteFile(topo, []byte("name: mini\ntopology:\n  nodes:\n    r1: {kind: linux}\n  links: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := New(WithBinary(bin), WithLabDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	lab, err := e.GetLab(context.Background(), "mini")
	if err != nil {
		t.Fatal(err)
	}
	if len(lab.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(lab.Nodes))
	}
	if got := lab.Nodes[0].State; got != engine.StatusRunning {
		t.Fatalf("initial state = %q, want %q", got, engine.StatusRunning)
	}

	// Simulate an external lifecycle change and refresh.
	if err := os.WriteFile(stateFile, []byte("exited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshLive(context.Background()); err != nil {
		t.Fatalf("RefreshLive: %v", err)
	}
	lab, err = e.GetLab(context.Background(), "mini")
	if err != nil {
		t.Fatal(err)
	}
	if got := lab.Nodes[0].State; got != engine.StatusStopped {
		t.Fatalf("after refresh state = %q, want %q", got, engine.StatusStopped)
	}
}
