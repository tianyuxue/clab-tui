package containerlab

import (
	"encoding/json"
	"testing"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestApplyInspectToStore(t *testing.T) {
	store := engine.NewStore()
	data := readTestdata(t, "inspect.json")

	if err := applyInspectData(store, nil, data); err != nil {
		t.Fatal(err)
	}
	snap := store.Snapshot()
	lab, ok := snap.Labs["mini"]
	if !ok || len(lab.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %+v", snap.Labs)
	}
	if lab.Nodes[0].Name != "r1" || lab.Nodes[0].State != engine.StatusRunning {
		t.Fatalf("unexpected node: %+v", lab.Nodes[0])
	}
}

func TestApplyInspectInterfacesResolvesNode(t *testing.T) {
	store := engine.NewStore()
	reg := newActorRegistry()
	data := readTestdata(t, "inspect.json")
	if err := applyInspectData(store, reg, data); err != nil {
		t.Fatal(err)
	}
	ifdata := readTestdata(t, "inspect_interfaces.json")
	if err := applyInspectInterfaces(store, reg, ifdata); err != nil {
		t.Fatal(err)
	}

	snap := store.Snapshot()
	lab := snap.Labs["mini"]
	r1 := findNode(lab, "r1")
	if r1 == nil {
		t.Fatalf("missing r1: %+v", lab)
	}
	if len(r1.Interfaces) == 0 {
		t.Fatal("expected interfaces on r1")
	}
}

func TestApplyInspectInterfacesSkipsUnknownOnly(t *testing.T) {
	store := engine.NewStore()
	reg := newActorRegistry()
	data := readTestdata(t, "inspect.json")
	if err := applyInspectData(store, reg, data); err != nil {
		t.Fatal(err)
	}

	// Build an interface fixture that includes a KNOWN container (clab-mini-r1,
	// present in inspect.json) and an UNKNOWN one (clab-ghost-x).
	var entries []map[string]any
	var known []map[string]any
	if err := json.Unmarshal(readTestdata(t, "inspect_interfaces.json"), &known); err != nil {
		t.Fatal(err)
	}
	entries = append(entries, known...)
	entries = append(entries, map[string]any{
		"name": "clab-ghost-x",
		"interfaces": []any{
			map[string]any{"name": "eth9", "state": "up", "type": "veth", "ifindex": 99, "mtu": 1500},
		},
	})
	combined, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}

	if err := applyInspectInterfaces(store, reg, combined); err != nil {
		t.Fatal(err)
	}

	snap := store.Snapshot()
	lab := snap.Labs["mini"]
	if lab == nil {
		t.Fatal("mini lab missing")
	}
	// Known container r1 must receive interfaces.
	r1 := findNode(lab, "r1")
	if r1 == nil {
		t.Fatalf("r1 missing: %+v", lab)
	}
	if len(r1.Interfaces) == 0 {
		t.Fatalf("expected interfaces on r1 (known container), got %+v", r1.Interfaces)
	}
	// The unknown container's interfaces must land nowhere.
	for _, n := range lab.Nodes {
		for _, iface := range n.Interfaces {
			if iface.Name == "eth9" {
				t.Fatalf("interface eth9 from unknown container leaked onto %s", n.Name)
			}
		}
	}
}

func findNode(lab *engine.Lab, name string) *engine.Node {
	if lab == nil {
		return nil
	}
	for i := range lab.Nodes {
		if lab.Nodes[i].Name == name {
			return &lab.Nodes[i]
		}
	}
	return nil
}
