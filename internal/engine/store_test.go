package engine

import (
	"testing"
	"time"
)

func collect(t *testing.T, ch <-chan Change, cancel func(), timeout time.Duration) []Change {
	t.Helper()
	defer cancel()
	var out []Change
	timeoutCh := time.After(timeout)
	for {
		select {
		case c, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, c)
		case <-timeoutCh:
			return out
		}
	}
}

func TestStore_ApplyContainerEventAddsNode(t *testing.T) {
	s := NewStore()
	ch, cancel := s.Subscribe()
	defer cancel()

	s.ApplyContainerEvent(ContainerEvent{
		LabName: "mini", NodeName: "r1", Container: "clab-mini-r1",
		State: StatusRunning, Group: "core",
	})

	snap := s.Snapshot()
	lab, ok := snap.Labs["mini"]
	if !ok || len(lab.Nodes) != 1 {
		t.Fatalf("expected 1 node in lab mini, got %+v", snap.Labs)
	}
	if lab.Nodes[0].Name != "r1" || lab.Nodes[0].State != StatusRunning {
		t.Fatalf("unexpected node: %+v", lab.Nodes[0])
	}
	changes := collect(t, ch, cancel, 100*time.Millisecond)
	found := false
	for _, c := range changes {
		if c.Type == ChangeNodeAdded && c.NodeName == "r1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ChangeNodeAdded for r1, got %+v", changes)
	}
}

func TestStore_IdempotentUpdateNoChange(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})

	ch, cancel := s.Subscribe()
	defer cancel()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})

	changes := collect(t, ch, cancel, 100*time.Millisecond)
	if len(changes) != 0 {
		t.Fatalf("idempotent update should not emit changes, got %+v", changes)
	}
}

func TestStore_ApplyInterfaceEvent(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})

	s.ApplyInterfaceEvent(InterfaceEvent{
		LabName: "mini", NodeName: "r1", Name: "eth1", State: "up", Index: 12,
	})

	snap := s.Snapshot()
	ifaces := snap.Labs["mini"].Nodes[0].Interfaces
	if len(ifaces) != 1 || ifaces[0].Name != "eth1" || ifaces[0].State != "up" {
		t.Fatalf("unexpected interfaces: %+v", ifaces)
	}
}

func TestStore_SnapshotIsDeepCopy(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})

	snap := s.Snapshot()
	snap.Labs["mini"].Nodes[0].Name = "HACKED"

	again := s.Snapshot()
	if again.Labs["mini"].Nodes[0].Name != "r1" {
		t.Fatal("snapshot should be a deep copy")
	}
}

func TestStore_ResetClears(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1"})
	s.Reset()
	if len(s.Snapshot().Labs) != 0 {
		t.Fatal("reset should clear all labs")
	}
}

func TestStore_LinkStateDerivation(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r2", State: StatusRunning})
	s.SetLinks("mini", []Link{{A: "r1", PortA: "eth1", B: "r2", PortB: "eth1"}})

	// No interface state yet -> unknown
	lab := s.Snapshot().Labs["mini"]
	if lab.Links[0].State != LinkUnknown {
		t.Fatalf("expected unknown, got %s", lab.Links[0].State)
	}

	// Both up -> up
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "mini", NodeName: "r1", Name: "eth1", State: "up"})
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "mini", NodeName: "r2", Name: "eth1", State: "up"})
	if s.Snapshot().Labs["mini"].Links[0].State != LinkUp {
		t.Fatal("expected link up")
	}

	// One down -> down
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "mini", NodeName: "r2", Name: "eth1", State: "down"})
	if s.Snapshot().Labs["mini"].Links[0].State != LinkDown {
		t.Fatal("expected link down")
	}
}

func TestStore_StoppedDoesNotRemoveNode(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusStopped})
	if n := s.Snapshot().Labs["mini"].Nodes[0]; n.State != StatusStopped {
		t.Fatalf("expected stopped state, got %v", n.State)
	}
}

func TestStore_NetemUpdateEmitsChange(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "mini", NodeName: "r1", Name: "eth1", State: "up"})

	ch, cancel := s.Subscribe()
	defer cancel()
	s.ApplyInterfaceEvent(InterfaceEvent{
		LabName: "mini", NodeName: "r1", Name: "eth1", State: "up",
		Netem: &NetemState{Delay: "10ms"},
	})

	changes := collect(t, ch, cancel, 100*time.Millisecond)
	if len(changes) != 1 || changes[0].Type != ChangeInterfaceUpdated {
		t.Fatalf("expected ChangeInterfaceUpdated for netem change, got %+v", changes)
	}
	ifaces := s.Snapshot().Labs["mini"].Nodes[0].Interfaces
	if ifaces[0].Netem == nil || ifaces[0].Netem.Delay != "10ms" {
		t.Fatalf("netem not stored: %+v", ifaces[0])
	}
}

func TestStore_StatsUpdateEmitsChange(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "mini", NodeName: "r1", Name: "eth1", State: "up"})

	ch, cancel := s.Subscribe()
	defer cancel()
	s.ApplyInterfaceEvent(InterfaceEvent{
		LabName: "mini", NodeName: "r1", Name: "eth1", State: "up",
		Stats: &InterfaceStats{RxBps: 1024},
	})

	changes := collect(t, ch, cancel, 100*time.Millisecond)
	if len(changes) != 1 || changes[0].Type != ChangeInterfaceUpdated {
		t.Fatalf("expected ChangeInterfaceUpdated for stats change, got %+v", changes)
	}
}

func TestStore_SnapshotDeepCopiesExtra(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{
		LabName: "mini", NodeName: "r1", State: StatusRunning,
	})
	// Add an interface with Extra directly via store internals is not exposed;
	// verify Node.Extra deep copy behavior through ApplyContainerEvent preserving Extra.
	snap := s.Snapshot()
	snap.Labs["mini"].Nodes[0].Extra = map[string]string{"hack": "1"}
	again := s.Snapshot()
	if len(again.Labs["mini"].Nodes[0].Extra) != 0 {
		t.Fatal("mutating returned snapshot Extra should not affect store")
	}
}

func TestStore_IfaceEqualNetemNilness(t *testing.T) {
	if !ifaceEqual(Interface{Name: "eth1", State: "up"}, Interface{Name: "eth1", State: "up"}) {
		t.Fatal("nil Netem/Stats should compare equal")
	}
	if ifaceEqual(Interface{Name: "eth1", Netem: &NetemState{Delay: "10ms"}}, Interface{Name: "eth1"}) {
		t.Fatal("netem presence should differ")
	}
}

func TestStore_RemoveNode(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r2", State: StatusRunning})
	ch, cancel := s.Subscribe()
	defer cancel()
	s.RemoveNode("mini", "r1")
	snap := s.Snapshot()
	if len(snap.Labs["mini"].Nodes) != 1 || snap.Labs["mini"].Nodes[0].Name != "r2" {
		t.Fatalf("expected only r2, got %+v", snap.Labs["mini"].Nodes)
	}
	changes := collect(t, ch, cancel, 100*time.Millisecond)
	if len(changes) < 1 || changes[0].Type != ChangeNodeRemoved || changes[0].NodeName != "r1" {
		t.Fatalf("expected ChangeNodeRemoved for r1, got %+v", changes)
	}
}

func TestApplyInterfaceEventIgnoredForStoppedNode(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "l", NodeName: "n1", Container: "c", State: StatusStopped})
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "l", NodeName: "n1", Name: "eth0", State: "up"})
	node := s.node("l", "n1")
	if node == nil {
		t.Fatal("node missing")
	}
	if len(node.Interfaces) != 0 {
		t.Fatalf("expected no interfaces for stopped node, got %d", len(node.Interfaces))
	}
}

func TestClearNodeInterfaces(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "l", NodeName: "n1", Container: "c", State: StatusRunning})
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "l", NodeName: "n1", Name: "eth0", State: "up"})
	s.ClearNodeInterfaces("l", "n1")
	node := s.node("l", "n1")
	if node == nil {
		t.Fatal("node gone")
	}
	if len(node.Interfaces) != 0 {
		t.Fatalf("expected interfaces cleared, got %d", len(node.Interfaces))
	}
}

func TestStore_RemoveLab(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})
	ch, cancel := s.Subscribe()
	defer cancel()
	s.RemoveLab("mini")
	if len(s.Snapshot().Labs) != 0 {
		t.Fatal("expected no labs after RemoveLab")
	}
	changes := collect(t, ch, cancel, 100*time.Millisecond)
	if len(changes) < 1 || changes[0].Type != ChangeLabRemoved {
		t.Fatalf("expected ChangeLabRemoved, got %+v", changes)
	}
}

func TestStore_InterfaceRenameByIfindex(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})

	// A veth is created with a transient host-side name, then renamed into the
	// container keeping the same ifindex. The transient name must not linger.
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "mini", NodeName: "r1", Name: "clab-abcdef12", State: "down", Index: 101})
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "mini", NodeName: "r1", Name: "e1-1", State: "up", Index: 101})

	ifaces := s.Snapshot().Labs["mini"].Nodes[0].Interfaces
	if len(ifaces) != 1 {
		t.Fatalf("expected a single interface after same-ifindex rename, got %+v", ifaces)
	}
	if ifaces[0].Name != "e1-1" || ifaces[0].State != "up" {
		t.Fatalf("interface not renamed in place: %+v", ifaces[0])
	}
}

func TestStore_InterfaceRenameByIfindexEth0Mgmt0(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})

	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "mini", NodeName: "r1", Name: "eth0", State: "down", Index: 2})
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "mini", NodeName: "r1", Name: "mgmt0", State: "up", Index: 2})

	ifaces := s.Snapshot().Labs["mini"].Nodes[0].Interfaces
	if len(ifaces) != 1 || ifaces[0].Name != "mgmt0" {
		t.Fatalf("eth0->mgmt0 rename left a stale interface: %+v", ifaces)
	}
}

func TestStore_RemoveInterface(t *testing.T) {
	s := NewStore()
	s.ApplyContainerEvent(ContainerEvent{LabName: "mini", NodeName: "r1", State: StatusRunning})
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "mini", NodeName: "r1", Name: "eth1", State: "up", Index: 12})
	s.ApplyInterfaceEvent(InterfaceEvent{LabName: "mini", NodeName: "r1", Name: "monit", State: "down", Index: 3})

	s.RemoveInterface("mini", "r1", "monit")

	ifaces := s.Snapshot().Labs["mini"].Nodes[0].Interfaces
	if len(ifaces) != 1 || ifaces[0].Name != "eth1" {
		t.Fatalf("removed interface still present: %+v", ifaces)
	}
}
