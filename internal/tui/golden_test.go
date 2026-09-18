package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/tianyuxue/clab-tui/internal/capability"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestTopologyGolden(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })

	m := New(nil, nil, WithCapabilities(capability.Capabilities{ClabSUID: true}))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{
		Name: "golden-lab",
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine", State: engine.StatusRunning, Interfaces: []engine.Interface{{Name: "eth1", State: "up"}}},
			{Name: "leaf-01", Group: "leaf", State: engine.StatusStopped},
		},
		Links: []engine.Link{{A: "spine-01", PortA: "eth1", B: "leaf-01", PortB: "eth1", State: engine.LinkDown}},
	})

	got := strings.TrimRight(m.View(), "\n")
	path := filepath.Join("testdata", "golden", "topology.golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v; run UPDATE_GOLDEN=1 go test ./internal/tui -run TestTopologyGolden", path, err)
	}
	if got != strings.TrimRight(string(want), "\n") {
		t.Fatalf("topology frame differs from %s\nrun UPDATE_GOLDEN=1 go test ./internal/tui -run TestTopologyGolden to update", path)
	}
}
