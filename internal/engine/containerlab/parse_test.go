package containerlab

import (
	"os"
	"testing"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func splitEventLines(data []byte) []string {
	var lines []string
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			if i > start {
				lines = append(lines, string(data[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

func TestParseContainerEventLine(t *testing.T) {
	data := readTestdata(t, "events_container.json")
	ev, err := parseContainerEventLine(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if ev.LabName != "mini" || ev.NodeName != "r1" {
		t.Fatalf("unexpected lab/node: %+v", ev)
	}
	if ev.Container != "clab-mini-r1" || ev.State != engine.StatusRunning {
		t.Fatalf("unexpected container: %+v", ev)
	}
	if ev.IPv4 != "172.20.20.3/24" {
		t.Fatalf("unexpected ipv4: %q", ev.IPv4)
	}
	if ev.Origin != "snapshot" {
		t.Fatalf("unexpected origin: %q", ev.Origin)
	}
	if ev.Group != "core" {
		t.Fatalf("unexpected group: %q", ev.Group)
	}
}

func TestStateFromAction(t *testing.T) {
	cases := map[string]engine.Status{
		"start":   engine.StatusRunning,
		"restart": engine.StatusRunning,
		"unpause": engine.StatusRunning,
		"die":     engine.StatusStopped,
		"stop":    engine.StatusStopped,
		"kill":    engine.StatusStopped,
		"pause":   engine.StatusPaused,
		"bogus":   engine.StatusUnknown,
	}
	for action, want := range cases {
		if got := stateFromAction(action); got != want {
			t.Fatalf("stateFromAction(%q)=%v, want %v", action, got, want)
		}
	}
}

func TestParseContainerEventDieDerivesStopped(t *testing.T) {
	line := `{"type":"container","action":"die","actor_id":"abc","actor_name":"clab-x-r1","attributes":{"containerlab":"x","clab-node-name":"r1","clab-node-longname":"clab-x-r1"}}`
	ev, err := parseContainerEventLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StatusStopped {
		t.Fatalf("expected stopped from die action, got %v", ev.State)
	}
}

func TestParseInterfaceEventLine(t *testing.T) {
	data := readTestdata(t, "events_interface.json")
	lines := splitEventLines(data)
	if len(lines) != 3 {
		t.Fatalf("expected 3 interface events, got %d", len(lines))
	}
	ev, err := parseInterfaceEventLine(lines[2])
	if err != nil {
		t.Fatal(err)
	}
	if ev.Name != "eth1" || ev.State != "up" {
		t.Fatalf("unexpected iface: %+v", ev)
	}
	if ev.Index != 12 || ev.MTU != 9500 {
		t.Fatalf("unexpected fields: %+v", ev)
	}
	if ev.Origin != "netlink" {
		t.Fatalf("unexpected origin: %q", ev.Origin)
	}
}

func TestParseInspectJSON(t *testing.T) {
	data := readTestdata(t, "inspect.json")
	events, err := parseInspectContainers(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 container events, got %d", len(events))
	}
	if events[0].NodeName != "r1" || events[0].State != engine.StatusRunning {
		t.Fatalf("unexpected event: %+v", events[0])
	}
}

func TestParseInspectInterfacesJSON(t *testing.T) {
	data := readTestdata(t, "inspect_interfaces.json")
	ifaces, err := parseInspectInterfaces(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(ifaces) != 1 {
		t.Fatalf("expected 1 container, got %d", len(ifaces))
	}
	entry := ifaces[0]
	if len(entry.Interfaces) != 3 || entry.Interfaces[1].Name != "eth1" {
		t.Fatalf("unexpected interfaces: %+v", entry.Interfaces)
	}
}
