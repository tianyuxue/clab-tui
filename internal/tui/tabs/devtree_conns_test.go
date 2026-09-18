package tabs

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestDevTreeStatusDotColors(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	cases := []struct {
		state engine.Status
		want  string // ANSI color code
	}{
		{engine.StatusRunning, "38;5;42"},
		{engine.StatusStopped, "38;5;196"},
		{engine.StatusPaused, "38;5;214"},
		{engine.StatusUnknown, "38;5;240"},
	}
	for _, tc := range cases {
		d := NewDevTree()
		d.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "n1", State: tc.state}}})
		v := d.View()
		if !strings.Contains(v, tc.want) {
			t.Fatalf("state %s: expected color %s in view:\n%s", tc.state, tc.want, v)
		}
	}
}

func TestDevTreeConnsVisibleByDefault(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
		},
		Links: []engine.Link{{A: "spine-01", PortA: "eth1", B: "leaf-01", PortB: "eth1", State: engine.LinkUp}},
	})
	d.SetSize(60, 20)
	if !d.expandedAll {
		t.Fatal("expected all connections expanded by default")
	}
	v := d.View()
	if !strings.Contains(v, "────") {
		t.Fatalf("expected connection lines by default:\n%s", v)
	}
}

func TestDevTreeToggleAll(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
		},
		Links: []engine.Link{{A: "spine-01", PortA: "eth1", B: "leaf-01", PortB: "eth1", State: engine.LinkUp}},
	})
	d.SetSize(60, 20)

	d.ToggleAll()
	if d.expandedAll {
		t.Fatal("expected expandedAll false after toggle")
	}
	v := d.View()
	if strings.Contains(v, "────") {
		t.Fatalf("expected no connections when all collapsed:\n%s", v)
	}

	d.ToggleAll()
	if !d.expandedAll {
		t.Fatal("expected expandedAll true after second toggle")
	}
}

func TestDevTreeEnterTogglesSingleNode(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
		},
		Links: []engine.Link{{A: "spine-01", PortA: "eth1", B: "leaf-01", PortB: "eth1", State: engine.LinkUp}},
	})
	d.SetSize(60, 20)
	// Start with all collapsed.
	d.ToggleAll()

	// Enter on the selected node should expand it only.
	first := d.Selected().Name
	d.ToggleSelected()
	v := d.View()
	if !strings.Contains(v, "────") {
		t.Fatalf("expected selected node connections after Enter:\n%s", v)
	}
	if !d.expandedSet[first] {
		t.Fatalf("expected %s in expandedSet", first)
	}

	// Enter again should collapse it.
	d.ToggleSelected()
	v = d.View()
	if strings.Contains(v, "────") {
		t.Fatalf("expected no connections after collapsing selected node:\n%s", v)
	}
}

func TestDevTreeSingleExpandWorksWhenAllCollapsed(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
		},
		Links: []engine.Link{{A: "spine-01", PortA: "eth1", B: "leaf-01", PortB: "eth1", State: engine.LinkUp}},
	})
	d.SetSize(60, 20)
	d.ToggleAll() // collapse all

	// Move to leaf-01 (second entry) and expand it.
	d.MoveDown()
	sel := d.Selected().Name
	d.ToggleSelected()
	if !d.expandedSet[sel] {
		t.Fatalf("expected %s in expandedSet", sel)
	}
	// The first node should NOT be expanded.
	if d.expandedSet[d.order[0].node.Name] {
		t.Fatalf("expected %s NOT expanded", d.order[0].node.Name)
	}
}

func TestToggleSelectedExitsExpandAll(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "n1"}, {Name: "n2"}}})
	d.ToggleAll() // default is expandedAll=true; toggle to false, then back on
	d.ToggleAll() // now expandedAll=true
	if !d.nodeExpanded("n1") || !d.nodeExpanded("n2") {
		t.Fatal("expected all expanded after ToggleAll")
	}
	d.ToggleSelected() // collapse n1 (cursor on first)
	if d.nodeExpanded("n1") {
		t.Fatal("expected n1 collapsed after ToggleSelected from expand-all")
	}
	if !d.nodeExpanded("n2") {
		t.Fatal("expected n2 still expanded")
	}
	// ToggleSelected again re-expands n1.
	d.ToggleSelected()
	if !d.nodeExpanded("n1") {
		t.Fatal("expected n1 re-expanded after second ToggleSelected")
	}
}

func TestToggleAllResetsPerNodeState(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "n1"}, {Name: "n2"}}})
	// Collapse all, then expand n1 individually.
	d.ToggleAll()      // expandedAll=false (from default true)
	d.ToggleSelected() // n1 expanded in expandedSet
	if !d.nodeExpanded("n1") {
		t.Fatal("expected n1 expanded individually")
	}
	// ToggleAll → expand all, clears expandedSet.
	d.ToggleAll()
	if !d.nodeExpanded("n1") || !d.nodeExpanded("n2") {
		t.Fatal("expected all expanded after ToggleAll")
	}
	// ToggleAll → collapse all.
	d.ToggleAll()
	if d.nodeExpanded("n1") || d.nodeExpanded("n2") {
		t.Fatal("expected all collapsed after second ToggleAll")
	}
}

func TestDevTreeRendersInterfaces(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{
		Name: "n1",
		Interfaces: []engine.Interface{
			{Name: "eth0", MAC: "aa:bb:cc:dd:ee:ff", State: "up"},
			{Name: "eth1", MAC: "11:22:33:44:55:66", State: "down"},
			{Name: "eth2", State: "unknown"},
		},
	}}})
	d.expandedAll = true
	v := d.View()
	for _, want := range []string{"eth0", "aa:bb:cc:dd:ee:ff", "eth1", "11:22:33:44:55:66", "eth2", "UP", "DOWN", "UNKNOWN"} {
		if !strings.Contains(v, want) {
			t.Fatalf("expected %q in view:\n%s", want, v)
		}
	}
}

func TestDevTreeRendersInterfaceIP(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{
		Name:       "n1",
		Interfaces: []engine.Interface{{Name: "mgmt0", MAC: "aa:bb:cc:dd:ee:ff", State: "up"}},
	}}})
	d.SetInterfaceIPs("n1", map[string]string{"mgmt0": "172.20.20.3"})
	d.expandedAll = true
	v := d.View()
	if !strings.Contains(v, "172.20.20.3") {
		t.Fatalf("expected interface IP in view:\n%s", v)
	}
}

func TestDevTreeInterfaceIPsPerNode(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{
		{Name: "n1", Interfaces: []engine.Interface{{Name: "mgmt0", State: "up"}}},
		{Name: "n2", Interfaces: []engine.Interface{{Name: "mgmt0", State: "up"}}},
	}})
	d.expandedAll = true
	d.SetInterfaceIPs("n1", map[string]string{"mgmt0": "172.20.20.3"})
	v := d.View()
	if !strings.Contains(v, "172.20.20.3") {
		t.Fatalf("expected n1 mgmt0 IP in view:\n%s", v)
	}
	// n2's mgmt0 must NOT show n1's IP — but both are named "mgmt0", so count
	// occurrences: exactly one "172.20.20.3".
	if got := strings.Count(v, "172.20.20.3"); got != 1 {
		t.Fatalf("expected exactly 1 IP occurrence, got %d:\n%s", got, v)
	}
}

func TestDevTreeShowsMgmtIPWithoutExec(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{
		Name: "r1",
		IPv4: "172.20.20.3",
		Interfaces: []engine.Interface{
			{Name: "mgmt0", State: "up"},
			{Name: "e1-1", State: "up"},
		},
	}}})
	d.expandedAll = true
	// Do NOT call SetInterfaceIPs — mgmt IP should appear from Node.IPv4 alone.
	v := d.View()
	if !strings.Contains(v, "172.20.20.3") {
		t.Fatalf("expected mgmt IP from Node.IPv4 in view:\n%s", v)
	}
}

func TestDevTreeToggleInterfaces(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{
		Name:       "n1",
		Interfaces: []engine.Interface{{Name: "eth0", State: "up"}},
	}}})
	d.expandedAll = true
	if !strings.Contains(d.View(), "eth0") {
		t.Fatalf("expected eth0 shown by default")
	}
	d.ToggleInterfaces()
	if strings.Contains(d.View(), "eth0") {
		t.Fatalf("expected eth0 hidden after toggle")
	}
	d.ToggleInterfaces()
	if !strings.Contains(d.View(), "eth0") {
		t.Fatalf("expected eth0 back after second toggle")
	}
}

func TestDevTreePathHits(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "n1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})
	d.expandedAll = true
	d.IncrementPathHit("n1", "eth0", "IN")
	d.IncrementPathHit("n1", "eth0", "OUT")
	v := d.View()
	if !strings.Contains(v, "IN 1") || !strings.Contains(v, "OUT 1") {
		t.Fatalf("expected hit count in view:\n%s", v)
	}
	d.ClearPathHits()
	if strings.Contains(d.View(), "IN 1") || strings.Contains(d.View(), "OUT 1") {
		t.Fatalf("expected no hits after clear")
	}
}

func TestDevTreePathHitsPreservedOnSetTopology(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "n1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})
	d.IncrementPathHit("n1", "eth0", "IN")
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "n1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})
	if !strings.Contains(d.View(), "IN 1") {
		t.Fatalf("expected hits preserved across SetTopology")
	}
	d.ClearPathHits()
	if strings.Contains(d.View(), "IN 1") {
		t.Fatalf("expected hits cleared after ClearPathHits")
	}
}

func TestDevTreeTreeStyleConnections(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{{Name: "n1", Group: "g"}, {Name: "n2", Group: "g"}},
		Links: []engine.Link{{A: "n1", PortA: "eth1", B: "n2", PortB: "eth1", State: engine.LinkUp}},
	})
	d.expandedAll = true
	v := d.View()
	if !strings.Contains(v, "└") {
		t.Fatalf("expected └ tree terminal in connection line:\n%s", v)
	}
	if !strings.Contains(v, "────") {
		t.Fatalf("expected plain line ──── in connection:\n%s", v)
	}
	if strings.Contains(v, "●───") {
		t.Fatalf("expected no dot in connection line:\n%s", v)
	}
}

func TestDevTreePathHitsBlinkConnection(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "n1", Group: "g", Interfaces: []engine.Interface{{Name: "eth1", State: "up"}}},
			{Name: "n2", Group: "g"},
		},
		Links: []engine.Link{{A: "n1", PortA: "eth1", B: "n2", PortB: "eth1", State: engine.LinkUp}},
	})
	d.expandedAll = true
	d.IncrementPathHit("n1", "eth1", "IN")
	d.SetPathBlink(true)
	v := d.View()
	// The model-driven blink phase uses bold green when active rather than
	// relying on terminal support for ANSI SGR blink animation.
	if !strings.Contains(v, "\x1b[1;38;5;226m") {
		t.Fatalf("expected active highlight style on hit connection:\n%s", v)
	}
}

func TestDevTreePathHitDirectionsUseDistinctColors(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "n1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})
	d.expandedAll = true
	d.IncrementPathHit("n1", "eth0", "IN")
	d.IncrementPathHit("n1", "eth0", "OUT")
	v := d.View()
	if !strings.Contains(v, "IN 1") || !strings.Contains(v, "OUT 1") {
		t.Fatalf("expected directional counts:\n%s", v)
	}
	if !strings.Contains(v, "38;5;39") || !strings.Contains(v, "38;5;214") {
		t.Fatalf("expected distinct ingress/egress colors:\n%s", v)
	}
}

func TestDevTreeLinkStateRendering(t *testing.T) {
	cases := []struct {
		state engine.LinkState
		want  string
	}{
		{engine.LinkUp, "UP"},
		{engine.LinkDown, "DOWN"},
		{engine.LinkUnknown, "UNKNOWN"},
	}
	for _, tc := range cases {
		d := NewDevTree()
		d.SetSize(100, 40)
		d.ToggleInterfaces()
		d.SetTopology(&engine.Lab{
			Nodes: []engine.Node{
				{Name: "leaf1", Group: "leaf", State: engine.StatusRunning},
				{Name: "spine1", Group: "spine", State: engine.StatusRunning},
			},
			Links: []engine.Link{{A: "leaf1", PortA: "e1-1", B: "spine1", PortB: "e1-1", State: tc.state}},
		})
		view := d.View()
		if !strings.Contains(view, tc.want) {
			t.Fatalf("link state %q: view missing %q\n%s", tc.state, tc.want, view)
		}
	}
}

func TestDevTreePathBlinkIsExplicitlyToggled(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{{Name: "n1", Group: "g", Interfaces: []engine.Interface{{Name: "eth1", State: "up"}}}, {Name: "n2", Group: "g"}},
		Links: []engine.Link{{A: "n1", PortA: "eth1", B: "n2", PortB: "eth1", State: engine.LinkUp}},
	})
	d.expandedAll = true
	d.IncrementPathHit("n1", "eth1", "IN")
	d.SetPathBlink(false)
	off := d.View()
	d.SetPathBlink(true)
	on := d.View()
	if off == on {
		t.Fatal("expected path highlight render to change between blink phases")
	}
	if !strings.Contains(on, "38;5;42") {
		t.Fatal("expected blink enabled when path blink is on")
	}
}
