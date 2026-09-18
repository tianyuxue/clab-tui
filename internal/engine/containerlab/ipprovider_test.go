package containerlab

import (
	"testing"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestClabEngineImplementsInterfaceIPProvider(t *testing.T) {
	var _ engine.InterfaceIPProvider = (*ClabEngine)(nil)
}

func TestClabEngineImplementsNodeMonitorProvider(t *testing.T) {
	var _ engine.NodeMonitorProvider = (*ClabEngine)(nil)
}

func TestParseIPJSON(t *testing.T) {
	out := `[{"ifname":"mgmt0","addr_info":[{"family":"inet","local":"172.20.20.3/24"},{"family":"inet6","local":"fe80::/64"}]}]`
	ips, ok := parseIPJSON(out)
	if !ok {
		t.Fatal("expected parse success")
	}
	if ips["mgmt0"] != "172.20.20.3" {
		t.Fatalf("expected mgmt0=172.20.20.3, got %q", ips["mgmt0"])
	}
	if len(ips) != 1 {
		t.Fatalf("expected only inet entries, got %v", ips)
	}
}

func TestParseIPText(t *testing.T) {
	out := `1: lo: <LOOPBACK> mtu 65536
    inet 127.0.0.1/8 scope host lo
2: eth0@if12: <BROADCAST> mtu 1500
    inet 10.0.0.5/24 brd 10.0.0.255
`
	ips := parseIPText(out)
	if ips["eth0"] != "10.0.0.5" {
		t.Fatalf("expected eth0=10.0.0.5, got %q", ips["eth0"])
	}
	if ips["lo"] != "127.0.0.1" {
		t.Fatalf("expected lo=127.0.0.1, got %q", ips["lo"])
	}
}
