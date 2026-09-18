package containerlab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestSourceStopWaitsForEventProcess(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "events.sh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nwhile :; do :; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := newSource(bin, engine.NewStore(), newActorRegistry())
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := sourceDoneChannel(s)
	time.Sleep(20 * time.Millisecond)
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	default:
		t.Fatal("source Stop returned before event process finished")
	}
}

func TestSourceRestartsAfterEventProcessExit(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "events.sh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nsleep 0.1\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := newSource(bin, engine.NewStore(), newActorRegistry())
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	firstDone := sourceDoneChannel(s)
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first event process did not exit")
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("restart after event process exit: %v", err)
	}
	secondDone := sourceDoneChannel(s)
	if secondDone == firstDone {
		t.Fatal("restart reused the completed event process state")
	}
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("restarted event process did not exit")
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseRetainsSourceAfterStopTimeout(t *testing.T) {
	s := &source{cancel: func() {}, done: make(chan struct{})}
	e := &ClabEngine{src: s}
	err := e.Close()
	if err == nil {
		t.Fatal("Close returned nil after source stop timeout")
	}
	e.mu.Lock()
	retained := e.src
	e.mu.Unlock()
	if retained != s {
		t.Fatal("Close discarded a source that did not stop")
	}
	close(s.done)
	if err := e.Close(); err != nil {
		t.Fatalf("Close after source completion: %v", err)
	}
}

func sourceDoneChannel(s *source) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done
}

func TestSourceEventPipeline(t *testing.T) {
	containerLine := `{"timestamp":"2026-08-13T00:24:12Z","type":"container","action":"running","actor_id":"abc123","actor_name":"clab-mini-r1","actor_full_id":"abc1234","attributes":{"containerlab":"mini","clab-node-name":"r1","clab-node-longname":"clab-mini-r1","clab-node-group":"core","clab-node-kind":"linux","state":"running","mgmt_ipv4":"172.20.20.3/24","origin":"snapshot","clab-topo-file":"/x/mini.clab.yml"}}`
	ifaceLine := `{"timestamp":"2026-08-13T00:24:13Z","type":"interface","action":"snapshot","actor_id":"abc123","actor_name":"clab-mini-r1","attributes":{"lab":"mini","ifname":"eth1","index":"12","mtu":"9500","state":"up","type":"veth","origin":"netlink"}}`

	store := engine.NewStore()
	reg := newActorRegistry()
	lines := strings.Split(containerLine+"\n"+ifaceLine+"\n", "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if err := handleRawEventLine(store, reg, line); err != nil {
			t.Fatalf("handleRawEventLine: %v", err)
		}
	}

	snap := store.Snapshot()
	lab, ok := snap.Labs["mini"]
	if !ok {
		t.Fatalf("missing lab mini: %+v", snap.Labs)
	}
	if len(lab.Nodes) != 1 || lab.Nodes[0].Name != "r1" {
		t.Fatalf("unexpected nodes: %+v", lab.Nodes)
	}
	ifaces := lab.Nodes[0].Interfaces
	if len(ifaces) != 1 || ifaces[0].Name != "eth1" || ifaces[0].State != "up" {
		t.Fatalf("interface not mapped to node: %+v", ifaces)
	}
}

func TestHandleRawEventLineSkipsExecActions(t *testing.T) {
	store := engine.NewStore()
	reg := newActorRegistry()
	// Seed a running node.
	store.ApplyContainerEvent(engine.ContainerEvent{LabName: "l", NodeName: "r1", Container: "c", State: engine.StatusRunning})
	// An exec event arrives — must NOT change state.
	line := `{"type":"container","action":"exec_create: ip -j addr","actor_id":"abc","actor_name":"c","attributes":{"containerlab":"l","clab-node-name":"r1"}}`
	if err := handleRawEventLine(store, reg, line); err != nil {
		t.Fatal(err)
	}
	snap := store.Snapshot()
	node := snap.Labs["l"].Nodes[0]
	if node.State != engine.StatusRunning {
		t.Fatalf("expected state unchanged (running), got %v", node.State)
	}
}

func TestActorRegistry(t *testing.T) {
	reg := newActorRegistry()
	reg.Set("abc123", "mini", "r1", "clab-mini-r1")
	if reg.Lab("abc123") != "mini" || reg.Node("abc123") != "r1" {
		t.Fatalf("unexpected registry values")
	}
	reg.Delete("abc123")
	if reg.Node("abc123") != "" {
		t.Fatal("expected empty after delete")
	}
}

func TestActorRegistryLookupByLongName(t *testing.T) {
	reg := newActorRegistry()
	reg.Set("abc123", "mini", "r1", "clab-mini-r1")
	lab, node := reg.LookupByLongName("clab-mini-r1")
	if lab != "mini" || node != "r1" {
		t.Fatalf("unexpected lookup: %q %q", lab, node)
	}
}

func TestActorRegistryLookupByLongNameHyphen(t *testing.T) {
	reg := newActorRegistry()
	reg.Set("a1", "prod-fw", "r1", "clab-prod-fw-r1")
	reg.Set("a2", "prod", "fw-r1", "clab-prod-fw-r1")
	// Storing exact long names means the LAST matching entry wins deterministically.
	// Just assert it resolves to a real entry (either is fine, but not empty).
	lab, node := reg.LookupByLongName("clab-prod-fw-r1")
	if lab == "" || node == "" {
		t.Fatalf("expected a resolved entry, got %q %q", lab, node)
	}
}

func TestHandleRawEventLineInterfaceDelete(t *testing.T) {
	store := engine.NewStore()
	reg := newActorRegistry()
	store.ApplyContainerEvent(engine.ContainerEvent{LabName: "l", NodeName: "r1", Container: "clab-l-r1", State: engine.StatusRunning})

	containerLine := `{"type":"container","action":"running","actor_id":"abc123","actor_name":"clab-l-r1","attributes":{"containerlab":"l","clab-node-name":"r1","clab-node-longname":"clab-l-r1"}}`
	ifaceLine := `{"type":"interface","action":"snapshot","actor_id":"abc123","actor_name":"clab-l-r1","attributes":{"lab":"l","ifname":"monit","index":"3","state":"down","type":"veth"}}`
	delLine := `{"type":"interface","action":"delete","actor_id":"abc123","actor_name":"clab-l-r1","attributes":{"lab":"l","ifname":"monit","index":"3","state":"down","type":"veth"}}`

	for _, line := range []string{containerLine, ifaceLine} {
		if err := handleRawEventLine(store, reg, line); err != nil {
			t.Fatal(err)
		}
	}
	if ifaces := store.Snapshot().Labs["l"].Nodes[0].Interfaces; len(ifaces) != 1 {
		t.Fatalf("expected monit present before delete, got %+v", ifaces)
	}

	if err := handleRawEventLine(store, reg, delLine); err != nil {
		t.Fatal(err)
	}
	if ifaces := store.Snapshot().Labs["l"].Nodes[0].Interfaces; len(ifaces) != 0 {
		t.Fatalf("interface not removed on delete event: %+v", ifaces)
	}
}
