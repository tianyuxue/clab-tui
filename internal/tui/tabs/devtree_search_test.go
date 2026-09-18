package tabs

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

func searchTree() *DevTree {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
			{Name: "leaf-02", Group: "leaf"},
			{Name: "fw-01", Group: "fw"},
			{Name: "server-01", Group: "server"},
		},
	})
	return d
}

func TestSearchLocatesFirstMatch(t *testing.T) {
	d := searchTree()
	d.Search("leaf")
	sel := d.Selected()
	if sel == nil || sel.Name != "leaf-01" {
		t.Fatalf("expected cursor on leaf-01, got %+v", sel)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	d := searchTree()
	d.Search("FW")
	if d.Selected().Name != "fw-01" {
		t.Fatalf("expected fw-01, got %s", d.Selected().Name)
	}
}

func TestSearchHighlightsMatchingText(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "leaf-01", Interfaces: []engine.Interface{{Name: "eth1"}}}}})
	d.Search("leaf")
	v := d.View()
	if !strings.Contains(v, "48;5;226") {
		t.Fatalf("expected search highlight background:\n%s", v)
	}
}

func TestSearchNoMatchLeavesCursor(t *testing.T) {
	d := searchTree()
	d.MoveDown() // cursor on leaf-01
	d.Search("nonexistent")
	if d.Selected().Name != "leaf-01" {
		t.Fatalf("expected cursor unchanged, got %s", d.Selected().Name)
	}
}

func TestFindNextCycles(t *testing.T) {
	d := searchTree()
	d.Search("leaf")
	if d.Selected().Name != "leaf-01" {
		t.Fatalf("expected leaf-01, got %s", d.Selected().Name)
	}
	d.FindNext()
	if d.Selected().Name != "leaf-02" {
		t.Fatalf("expected leaf-02, got %s", d.Selected().Name)
	}
	d.FindNext() // wrap to leaf-01
	if d.Selected().Name != "leaf-01" {
		t.Fatalf("expected wrap to leaf-01, got %s", d.Selected().Name)
	}
}

func TestFindPrev(t *testing.T) {
	d := searchTree()
	d.Search("leaf")
	d.FindNext() // leaf-02
	d.FindPrev() // back to leaf-01
	if d.Selected().Name != "leaf-01" {
		t.Fatalf("expected leaf-01 after FindPrev, got %s", d.Selected().Name)
	}
}

func TestSearchClearedByEmptyTerm(t *testing.T) {
	d := searchTree()
	d.Search("leaf")
	d.Search("")
	// Empty term means no search active; cursor stays wherever it was
	// (Search("") should not move cursor).
	if d.Selected().Name != "leaf-01" {
		t.Fatalf("expected leaf-01 (cursor unchanged by empty search), got %s", d.Selected().Name)
	}
}

func TestSearchMatchesConnections(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{{Name: "spine-01", Group: "spine"}},
		Links: []engine.Link{{A: "spine-01", PortA: "eth1", B: "leaf-01", PortB: "eth1", State: engine.LinkUp}},
	})
	// Searching a neighbor's name should find the node that connects to it.
	d.Search("leaf-01")
	if d.Selected() == nil || d.Selected().Name != "spine-01" {
		t.Fatalf("expected search by neighbor to locate spine-01, got %+v", d.Selected())
	}
}

func TestSearchSelectsInterfaceRowAndFindNextMovesRows(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{{Name: "r1", Group: "leaf", Interfaces: []engine.Interface{{Name: "eth1"}, {Name: "eth2"}}}, {Name: "r2", Group: "leaf"}},
		Links: []engine.Link{{A: "r1", PortA: "eth1", B: "r2", PortB: "eth1", State: engine.LinkUp}},
	})
	d.Search("eth1")
	selection, ok := d.SelectedResource()
	if !ok || selection.Kind != ResourceInterface || selection.Interface != "eth1" {
		t.Fatalf("expected first eth1 interface row, got %+v", selection)
	}
	d.FindNext()
	selection, ok = d.SelectedResource()
	if !ok || selection.Kind != ResourceLink || selection.Interface != "eth1" {
		t.Fatalf("expected next eth1 link row, got %+v", selection)
	}
}

func TestSearchNMovesAcrossNodeAndInterfaceMatches(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{{Name: "eth1", Group: "leaf", Interfaces: []engine.Interface{{Name: "eth1"}}}, {Name: "r2", Group: "leaf"}},
		Links: []engine.Link{{A: "eth1", PortA: "eth1", B: "r2", PortB: "eth1", State: engine.LinkUp}},
	})
	d.Search("eth1")
	for _, want := range []ResourceSelection{
		{Kind: ResourceNode, Node: "eth1"},
		{Kind: ResourceInterface, Node: "eth1", Interface: "eth1"},
		{Kind: ResourceLink, Node: "eth1", Interface: "eth1", Neighbor: "r2", Port: "eth1"},
		{Kind: ResourceLink, Node: "r2", Interface: "eth1", Neighbor: "eth1", Port: "eth1"},
	} {
		got, ok := d.SelectedResource()
		if !ok || got != want {
			t.Fatalf("selection = %+v, want %+v", got, want)
		}
		d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	}
	got, _ := d.SelectedResource()
	if got.Kind != ResourceNode || got.Node != "eth1" {
		t.Fatalf("expected n to wrap to node result, got %+v", got)
	}
}

func TestSearchNodeAndLinkDoesNotMatchAllChildRows(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "ceos1", Interfaces: []engine.Interface{{Name: "mgmt0"}, {Name: "e1-1"}}},
			{Name: "srl1", Interfaces: []engine.Interface{{Name: "e1-1"}}},
		},
		Links: []engine.Link{{A: "ceos1", PortA: "e1-1", B: "srl1", PortB: "e1-1", State: engine.LinkUp}},
	})
	d.Search("os")
	selection, _ := d.SelectedResource()
	if selection.Kind != ResourceNode || selection.Node != "ceos1" {
		t.Fatalf("expected ceos1 node first, got %+v", selection)
	}
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	selection, _ = d.SelectedResource()
	if selection.Kind != ResourceLink || selection.Neighbor != "ceos1" {
		t.Fatalf("expected ceos1 link second, got %+v", selection)
	}
	if status := d.SearchStatus(); !strings.Contains(status, "2 matches") {
		t.Fatalf("expected exactly two matches, got %q", status)
	}
}

// TestSearchPrefersNodeNameOverNeighbor ensures searching a node's exact name
// locates that node, not an unrelated node that merely connects to it.
func TestSearchPrefersNodeNameOverNeighbor(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "node06", Group: "leaf"},
			{Name: "node12", Group: "leaf"},
		},
		Links: []engine.Link{{A: "node06", PortA: "eth1", B: "node12", PortB: "eth1", State: engine.LinkUp}},
	})
	// Both node06 (connects to node12) and node12 match "node12" by neighbor/
	// name. The search must land on node12 itself.
	d.Search("node12")
	if d.Selected() == nil || d.Selected().Name != "node12" {
		t.Fatalf("expected node12 (own name priority), got %+v", d.Selected())
	}
}

// TestSearchNeighborFallback verifies neighbor matching still works when no
// node name matches.
func TestSearchNeighborFallback(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{{Name: "srv-a", Group: "server"}},
		Links: []engine.Link{{A: "srv-a", PortA: "eth1", B: "fw-01", PortB: "eth1", State: engine.LinkUp}},
	})
	d.Search("fw-01") // no node named fw-01; srv-a connects to it
	if d.Selected() == nil || d.Selected().Name != "srv-a" {
		t.Fatalf("expected srv-a via neighbor fallback, got %+v", d.Selected())
	}
}

func TestSearchNoMatchShowsMessage(t *testing.T) {
	d := searchTree()
	d.Search("zzz-nonexistent")
	if !d.NoMatch() {
		t.Fatal("expected NoMatch true for nonexistent term")
	}
}

func TestSearchMatchNoMessage(t *testing.T) {
	d := searchTree()
	d.Search("leaf")
	if d.NoMatch() {
		t.Fatal("expected NoMatch false when match exists")
	}
}
