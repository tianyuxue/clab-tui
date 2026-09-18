package tabs

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestDetailPaneRendersNodeInfo(t *testing.T) {
	p := NewDetailPane()
	p.SetNode(&engine.Node{
		Name:      "srl1",
		Container: "clab-lab-srl1",
		Image:     "ghcr.io/nokia/srlinux:24.7.1",
		State:     engine.StatusRunning,
		StartedAt: time.Now().Add(-2 * time.Hour),
	})
	p.SetMonitor(&engine.NodeMonitor{CPUPercent: 3.5, MemUsed: 128 << 20, MemLimit: 1 << 30})
	v := p.View()
	for _, want := range []string{"srl1", "clab-lab-srl1", "24.7.1", "3.5%", "128"} {
		if !strings.Contains(v, want) {
			t.Fatalf("expected %q in detail pane:\n%s", want, v)
		}
	}
}

func TestDetailPaneRendersInterfaceStats(t *testing.T) {
	p := NewDetailPane()
	p.SetSize(60, 15)
	p.SetNode(&engine.Node{Name: "r1", Interfaces: []engine.Interface{{Name: "e1-1", State: "up"}}})
	p.SetInterfaceStats(map[string]*engine.InterfaceStats{
		"e1-1": {RxBytes: 1024, RxPackets: 10, RxBps: 512, TxBytes: 2048, TxPackets: 20, TxBps: 1024},
	})
	v := p.View()
	for _, want := range []string{"RX:", "TX:", "10 pkts", "20 pkts"} {
		if !strings.Contains(v, want) {
			t.Fatalf("expected %q in detail pane:\n%s", want, v)
		}
	}
}

func TestDetailPaneNilNode(t *testing.T) {
	p := NewDetailPane()
	if p.View() == "" {
		t.Fatal("expected non-empty view with nil node (placeholder)")
	}
}

func TestTruncateToHeight(t *testing.T) {
	s := "a\nb\nc\nd\ne\nf\ng"
	got := truncateToHeight(s, 3)
	if want := "a\nb\nc"; got != want {
		t.Fatalf("truncateToHeight(s,3)=%q, want %q", got, want)
	}
	// Content already ≤ height passes through unchanged.
	if got := truncateToHeight(s, 10); got != s {
		t.Fatalf("truncateToHeight(s,10)=%q, want original %q", got, s)
	}
	// A degenerate height must not empty the content (unsized pane).
	if got := truncateToHeight(s, 0); got != s {
		t.Fatalf("truncateToHeight(s,0)=%q, want original %q", got, s)
	}
}

// TestDetailPaneTruncatesToHeight guards the overflow case: a node with many
// interfaces must never render taller than the pane's height.
func TestDetailPaneTruncatesToHeight(t *testing.T) {
	p := NewDetailPane()
	p.SetSize(40, 4)
	p.SetNode(&engine.Node{
		Name:      "srl1",
		Image:     "ghcr.io/nokia/srlinux:24.7.1",
		State:     engine.StatusRunning,
		StartedAt: time.Now(),
	})
	v := p.View()
	if got := len(strings.Split(v, "\n")); got != 4 {
		t.Fatalf("detail pane rendered %d lines, want exactly 4:\n%s", got, v)
	}
}

func TestDetailPaneWrapsLongValuesToFixedWidth(t *testing.T) {
	p := NewDetailPane()
	p.SetSize(24, 12)
	p.SetNode(&engine.Node{
		Name:      "r1",
		Container: "clab-a-very-long-container-name-r1",
		Image:     "registry.example.com/network/long-image-name:latest",
		State:     engine.StatusRunning,
	})
	lines := strings.Split(p.View(), "\n")
	if len(lines) != 12 {
		t.Fatalf("detail pane rendered %d lines, want 12", len(lines))
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		if got := visualWidth(line); got != 24 {
			t.Fatalf("detail line width=%d, want 24: %q", got, line)
		}
	}
}

func TestDetailPaneStateColored(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	p := NewDetailPane()
	p.SetNode(&engine.Node{Name: "r1", State: engine.StatusRunning})
	if !strings.Contains(p.View(), "38;5;42") {
		t.Fatalf("expected green state:\n%s", p.View())
	}
	p.SetNode(&engine.Node{Name: "r1", State: engine.StatusStopped})
	if !strings.Contains(p.View(), "38;5;196") {
		t.Fatalf("expected red state:\n%s", p.View())
	}
}

func TestImageTag(t *testing.T) {
	cases := map[string]string{
		"ghcr.io/nokia/srlinux:24.7.1": "24.7.1",
		"ceos:4.32.0F":                 "4.32.0F",
		"debian:bookworm":              "bookworm",
		"no-tag-image":                 "no-tag-image",
	}
	for in, want := range cases {
		if got := imageTag(in); got != want {
			t.Fatalf("imageTag(%q)=%q, want %q", in, got, want)
		}
	}
}
