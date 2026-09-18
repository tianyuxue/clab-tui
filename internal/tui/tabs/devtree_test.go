package tabs

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestDevTreeBuildsRoleGroups(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
			{Name: "fw-01", Group: "fw"},
			{Name: "srv-01", Group: ""}, // no role
		},
	})
	// Groups: spine, leaf, fw + (no role) for the unassigned node.
	if len(d.groups) != 4 {
		t.Fatalf("expected 4 role groups (spine, leaf, fw, no-role), got %d: %v", len(d.groups), d.groups)
	}
	if len(d.groups["(no role)"]) != 1 {
		t.Fatalf("expected 1 unassigned node, got %d", len(d.groups["(no role)"]))
	}
}

func TestDevTreeGroupsNodesByRole(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "spine-02", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
		},
	})
	if len(d.groups["spine"]) != 2 {
		t.Fatalf("expected 2 spine nodes, got %d", len(d.groups["spine"]))
	}
	if len(d.groups["leaf"]) != 1 {
		t.Fatalf("expected 1 leaf node, got %d", len(d.groups["leaf"]))
	}
}

func TestDevTreeGroupsUnassigned(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "srv-01", Group: ""},
			{Name: "srv-02", Group: ""},
		},
	})
	if len(d.groups["(no role)"]) != 2 {
		t.Fatalf("expected 2 unassigned nodes under '(no role)', got %d", len(d.groups["(no role)"]))
	}
}

func TestDevTreeViewShowsRoles(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
		},
		Links: []engine.Link{
			{A: "spine-01", PortA: "eth1", B: "leaf-01", PortB: "eth1", State: engine.LinkUp},
		},
	})
	d.SetSize(60, 20)
	v := d.View()
	if !strings.Contains(v, "spine") {
		t.Fatalf("expected role header 'spine' in view:\n%s", v)
	}
	if !strings.Contains(v, "spine-01") {
		t.Fatalf("expected node spine-01 in view:\n%s", v)
	}
	if !strings.Contains(v, "leaf-01") {
		t.Fatalf("expected node leaf-01 in view:\n%s", v)
	}
}

func TestDevTreeSetTopologySmallerLabClampsCursor(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{
		{Name: "a", Group: "core"}, {Name: "b", Group: "core"}, {Name: "c", Group: "core"},
	}})
	d.MoveDown()
	d.MoveDown() // cursor = 2
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "a", Group: "core"}}})
	if got := d.Cursor(); got != 0 {
		t.Fatalf("expected cursor clamped to 0, got %d", got)
	}
	_ = d.View() // must not panic
}

func TestDevTreeFooterNoticeRendersInsidePane(t *testing.T) {
	d := NewDevTree()
	d.SetSize(30, 5)
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "r1"}}})
	d.SetFooterNotice("ERROR: capture failed")
	v := d.View()
	if !strings.Contains(v, "ERROR: capture failed") {
		t.Fatalf("expected footer notice in tree pane:\n%s", v)
	}
	if got := len(strings.Split(v, "\n")); got != 5 {
		t.Fatalf("tree pane rendered %d lines, want 5", got)
	}
}

func TestDevTreeLongFooterStaysSingleLine(t *testing.T) {
	d := NewDevTree()
	d.SetSize(20, 5)
	d.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "r1"}}})
	d.SetFooterNotice("/very-long-search-term 12 matches [1/12] n/N next/previous Ctrl-f/Ctrl-b page")
	if got := len(strings.Split(d.View(), "\n")); got != 5 {
		t.Fatalf("long footer changed pane height to %d", got)
	}
}

func TestDevTreeCtrlFAndCtrlBPageViewport(t *testing.T) {
	d := NewDevTree()
	d.SetSize(40, 3)
	nodes := make([]engine.Node, 12)
	for i := range nodes {
		nodes[i] = engine.Node{Name: fmt.Sprintf("node-%02d", i)}
	}
	d.SetTopology(&engine.Lab{Nodes: nodes})
	d.View()
	d.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if d.vp.YOffset == 0 {
		t.Fatal("expected ctrl+f to page down")
	}
	before := d.vp.YOffset
	d.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	if d.vp.YOffset >= before {
		t.Fatal("expected ctrl+b to page up")
	}
}

func TestDevTreeCursorMoves(t *testing.T) {
	d := NewDevTree()
	d.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
			{Name: "leaf-02", Group: "leaf"},
		},
	})
	// Roles sort alphabetically: leaf before spine, so first is leaf-01.
	if d.Selected().Name != "leaf-01" {
		t.Fatalf("expected first selection leaf-01 (sorted), got %s", d.Selected().Name)
	}
	d.MoveDown()
	if d.Selected().Name != "leaf-02" {
		t.Fatalf("expected selection leaf-02 after move down, got %s", d.Selected().Name)
	}
	d.MoveDown()
	if d.Selected().Name != "spine-01" {
		t.Fatalf("expected selection spine-01 after second move, got %s", d.Selected().Name)
	}
	// Should not go past last node.
	d.MoveDown()
	if d.Selected().Name != "spine-01" {
		t.Fatalf("expected selection to stay spine-01 at bottom, got %s", d.Selected().Name)
	}
}
