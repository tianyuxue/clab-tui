//go:build integration

package containerlab

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/gopacket/pcapgo"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// TestIntegrationCaptureShowsVLANOnTrunk verifies the end-to-end capture path
// on a VLAN-filtering bridge trunk: the kernel keeps the 802.1Q tag in skb
// metadata (veth VLAN offload), so the eBPF program must synthesize it back
// into the copied frame. A "vlan 10" capture filter also proves the tag is
// reconstructed before the libpcap filter runs.
func TestIntegrationCaptureShowsVLANOnTrunk(t *testing.T) {
	if !integrationEnabled(t) {
		t.Skip("CLAB_TUI_SKIP_INTEGRATION set")
	}
	requireRoot(t)
	requireImage(t, traceIntegrationImage)
	bin := integrationBinary(t)

	dir := t.TempDir()
	topo := filepath.Join(dir, "vlan-capture.clab.yml")
	content := `name: vlan-capture
topology:
  nodes:
    sw1:
      kind: linux
      image: ` + traceIntegrationImage + `
      exec:
        - sysctl -qw net.ipv6.conf.all.disable_ipv6=1
        - ip link add br0 type bridge vlan_filtering 1
        - ip link set br0 up
        - ip link set eth1 master br0
        - ip link set eth2 master br0
        - ip link set eth1 up
        - ip link set eth2 up
        - bridge vlan add dev eth1 vid 10 pvid untagged
        - bridge vlan add dev eth2 vid 10
    sw2:
      kind: linux
      image: ` + traceIntegrationImage + `
      exec:
        - sysctl -qw net.ipv6.conf.all.disable_ipv6=1
        - ip link add br0 type bridge vlan_filtering 1
        - ip link set br0 up
        - ip link set eth1 master br0
        - ip link set eth2 master br0
        - ip link set eth1 up
        - ip link set eth2 up
        - bridge vlan add dev eth1 vid 10
        - bridge vlan add dev eth2 vid 10 pvid untagged
    h1:
      kind: linux
      image: ` + traceIntegrationImage + `
      exec:
        - sysctl -qw net.ipv6.conf.all.disable_ipv6=1
        - ip addr add 10.10.10.1/24 dev eth1
        - ip link set eth1 up
    h2:
      kind: linux
      image: ` + traceIntegrationImage + `
      exec:
        - sysctl -qw net.ipv6.conf.all.disable_ipv6=1
        - ip addr add 10.10.10.2/24 dev eth1
        - ip link set eth1 up
  links:
    - endpoints: ["h1:eth1", "sw1:eth1"]
    - endpoints: ["sw1:eth2", "sw2:eth1"]
    - endpoints: ["sw2:eth2", "h2:eth1"]
`
	if err := os.WriteFile(topo, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := New(WithBinary(bin), WithLabDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	registerIntegrationCleanup(t, e, "vlan-capture")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	deployCh, err := e.Deploy(ctx, topo)
	if err != nil {
		t.Fatal(err)
	}
	if output, code := collectOutput(t, deployCh); code != 0 {
		t.Fatalf("deploy failed (exit %d):\n%s", code, output)
	}
	_, _ = e.ListLabs(ctx)
	time.Sleep(2 * time.Second)

	pcapPath := filepath.Join(dir, "trunk.pcap")
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()
	events, stop, err := e.Capture(cctx, "vlan-capture", "sw1", "eth2",
		engine.WithCaptureFilter("vlan 10"), engine.WithCaptureFile(pcapPath))
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	defer stop()

	go func() {
		time.Sleep(time.Second)
		if out, err := exec.Command("docker", "exec", "clab-vlan-capture-h1",
			"ping", "-c", "3", "-W", "1", "10.10.10.2").CombinedOutput(); err != nil {
			t.Logf("ping: %v\n%s", err, out)
		}
		if out, err := exec.Command("docker", "exec", "clab-vlan-capture-h1", "sh", "-c",
			"curl -s -m 1 http://10.10.10.2/ >/dev/null 2>&1 || true").CombinedOutput(); err != nil {
			t.Logf("curl: %v\n%s", err, out)
		}
	}()

	var sawICMP, sawTCP bool
	deadline := time.After(20 * time.Second)
	for !sawICMP || !sawTCP {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("capture channel closed before VLAN traffic arrived")
			}
			if len(ev.Pkt.Payload) < 22 {
				t.Fatalf("short payload for %s event: %d bytes", ev.Pkt.Proto, len(ev.Pkt.Payload))
			}
			if ev.Pkt.Payload[12] != 0x81 || ev.Pkt.Payload[13] != 0x00 {
				t.Fatalf("%s event is missing the 802.1Q TPID: % x", ev.Pkt.Proto, ev.Pkt.Payload[:18])
			}
			if got := binary.BigEndian.Uint16(ev.Pkt.Payload[14:16]) & 0x0fff; got != 10 {
				t.Fatalf("VLAN id = %d, want 10: % x", got, ev.Pkt.Payload[:18])
			}
			if ev.Pkt.Payload[16] == 0x08 && ev.Pkt.Payload[17] == 0x00 {
				ipTotalLen := int(binary.BigEndian.Uint16(ev.Pkt.Payload[20:22]))
				if want := 18 + ipTotalLen; ev.Pkt.Len != want {
					t.Fatalf("%s wire length = %d, want %d (Ethernet+VLAN+IP total length)", ev.Pkt.Proto, ev.Pkt.Len, want)
				}
			}
			switch ev.Pkt.Proto {
			case "icmp":
				sawICMP = true
			case "tcp":
				if ev.Pkt.SrcPort == 80 || ev.Pkt.DstPort == 80 {
					sawTCP = true
				}
			}
			t.Logf("vlan event: %s %s:%d -> %s:%d len=%d",
				ev.Pkt.Proto, ev.Pkt.SrcIP, ev.Pkt.SrcPort, ev.Pkt.DstIP, ev.Pkt.DstPort, ev.Pkt.Len)
		case <-deadline:
			t.Fatalf("timed out waiting for VLAN-tagged traffic (icmp=%v tcp=%v)", sawICMP, sawTCP)
		}
	}

	if err := stop(); err != nil {
		t.Fatalf("stop capture: %v", err)
	}

	// The exported pcap must carry the tag as real packet bytes, not metadata.
	f, err := os.Open(pcapPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := pcapgo.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tagged := 0
	for {
		data, _, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(data) < 18 || data[12] != 0x81 || data[13] != 0x00 {
			t.Fatalf("pcap packet %d is missing the 802.1Q tag: % x", tagged, data[:min(len(data), 18)])
		}
		tagged++
	}
	if tagged == 0 {
		t.Fatal("pcap contains no packets")
	}
	t.Logf("pcap contains %d VLAN-tagged packets", tagged)

	// Phase 2: disable veth VLAN offload so the bridge inserts the tag in-band.
	// parse_ip must skip it and still report L3/L4 fields (the metadata path
	// above starts directly at the inner ethertype).
	if out, err := exec.Command("docker", "exec", "clab-vlan-capture-sw1",
		"ethtool", "-K", "eth2", "tx-vlan-offload", "off", "rx-vlan-offload", "off").CombinedOutput(); err != nil {
		t.Fatalf("disable VLAN offload: %v\n%s", err, out)
	}
	features, err := exec.Command("docker", "exec", "clab-vlan-capture-sw1", "ethtool", "-k", "eth2").Output()
	if err != nil {
		t.Fatalf("read ethtool features: %v", err)
	}
	if !strings.Contains(string(features), "tx-vlan-offload: off") {
		t.Fatalf("veth still offloads VLAN tags:\n%s", features)
	}

	cctx2, ccancel2 := context.WithCancel(context.Background())
	defer ccancel2()
	inband, stop2, err := e.Capture(cctx2, "vlan-capture", "sw1", "eth2",
		engine.WithCaptureFilter("vlan 10 and icmp"))
	if err != nil {
		t.Fatalf("Capture (in-band): %v", err)
	}
	defer stop2()
	trafficDone := make(chan struct{})
	go func() {
		defer close(trafficDone)
		time.Sleep(time.Second)
		if out, err := exec.Command("docker", "exec", "clab-vlan-capture-h1",
			"ping", "-c", "3", "-W", "1", "10.10.10.2").CombinedOutput(); err != nil {
			t.Logf("ping (in-band): %v\n%s", err, out)
		}
	}()

	select {
	case ev, ok := <-inband:
		if !ok {
			t.Fatal("in-band capture channel closed early")
		}
		if len(ev.Pkt.Payload) < 18 || ev.Pkt.Payload[12] != 0x81 || ev.Pkt.Payload[13] != 0x00 {
			t.Fatalf("in-band event is missing the 802.1Q tag: % x", ev.Pkt.Payload[:18])
		}
		if ev.Pkt.Proto != "icmp" || ev.Pkt.SrcIP != "10.10.10.1" || ev.Pkt.DstIP != "10.10.10.2" {
			t.Fatalf("in-band VLAN L3 parse failed: proto=%q src=%q dst=%q", ev.Pkt.Proto, ev.Pkt.SrcIP, ev.Pkt.DstIP)
		}
		if ev.Pkt.Len != 102 {
			t.Fatalf("in-band ICMP wire length = %d, want 102", ev.Pkt.Len)
		}
		t.Logf("in-band vlan event: %s %s -> %s len=%d", ev.Pkt.Proto, ev.Pkt.SrcIP, ev.Pkt.DstIP, ev.Pkt.Len)
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for in-band VLAN traffic")
	}
	select {
	case <-trafficDone:
	case <-time.After(5 * time.Second):
	}
}
