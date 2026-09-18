package containerlab

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestClabEngineImplementsPacketPathTracer(t *testing.T) {
	var _ engine.PacketPathTracer = (*ClabEngine)(nil)
}

func TestClabEngineImplementsCapabilityChecker(t *testing.T) {
	var _ engine.CapabilityChecker = (*ClabEngine)(nil)
}

func TestClabEngineImplementsPacketCapturer(t *testing.T) {
	var _ engine.PacketCapturer = (*ClabEngine)(nil)
}

func TestCapturePCAPWritesReadableEthernetFrame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.pcap")
	w, err := newCapturePCAP(path)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 0x08, 0x00, 0x45, 0}
	ev := pktEventRaw{Len: uint32(len(body))}
	copy(ev.Payload[:], body)
	if err := w.Write(ev); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := pcapgo.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	data, ci, err := r.ReadPacketData()
	if err != nil {
		t.Fatal(err)
	}
	if ci.Length != len(body) || !bytes.Equal(data, body) {
		t.Fatalf("unexpected pcap packet: length=%d data=%x", ci.Length, data)
	}
}

func TestCompileCaptureFilterSupportsTcpdumpExpressions(t *testing.T) {
	for _, expression := range []string{
		"host 10.0.0.1",
		"src host 10.0.0.1",
		"dst net 10.0.0.0/24",
		"tcp port 179",
	} {
		if filter, err := compileCaptureFilter(expression); err != nil || filter == nil {
			t.Fatalf("compileCaptureFilter(%q) = %v, %v; want a compiled filter", expression, filter, err)
		}
	}
}

func TestCompileCaptureFilterRejectsInvalidExpression(t *testing.T) {
	if _, err := compileCaptureFilter("src host"); err == nil {
		t.Fatal("expected invalid capture filter to return an error")
	}
}

func TestCaptureFilterMatchesEthernetPacket(t *testing.T) {
	eth := &layers.Ethernet{SrcMAC: []byte{0, 1, 2, 3, 4, 5}, DstMAC: []byte{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, SrcIP: []byte{10, 0, 0, 1}, DstIP: []byte{10, 0, 0, 2}, Protocol: layers.IPProtocolTCP}
	tcp := &layers.TCP{SrcPort: 179, DstPort: 443, SYN: true}
	tcp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true}, eth, ip, tcp); err != nil {
		t.Fatal(err)
	}
	packet := buf.Bytes()
	filter, err := compileCaptureFilter("src host 10.0.0.1 and tcp port 179")
	if err != nil {
		t.Fatal(err)
	}
	if !captureFilterMatches(filter, packet, len(packet)) {
		t.Fatal("expected filter to match the serialized packet")
	}
	other, err := compileCaptureFilter("src host 10.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	if captureFilterMatches(other, packet, len(packet)) {
		t.Fatal("expected non-matching source filter to reject the packet")
	}
	if captureFilterMatches(nil, packet, len(packet)) == false {
		t.Fatal("expected nil filter to match every packet")
	}
}

func TestCaptureBPFBoundsPayloadCopyToPacketLength(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "bpf", "capture.bpf.c"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "__u32 copy_len") {
		t.Fatal("BPF capture must bound payload copy length")
	}
	if !strings.Contains(text, "bpf_skb_load_bytes(skb, 0, ev->payload, copy_len)") {
		t.Fatal("BPF capture must copy the bounded packet length")
	}
}

func TestTCXAttachOptionsUseHeadAnchor(t *testing.T) {
	opts := tcxAttachOptions(nil, 32, ebpf.AttachTCXIngress)
	if opts.Interface != 32 {
		t.Fatalf("interface = %d, want 32", opts.Interface)
	}
	if opts.Attach != ebpf.AttachTCXIngress {
		t.Fatalf("attach type = %v, want ingress", opts.Attach)
	}
	if opts.Anchor == nil {
		t.Fatal("expected capture program to attach at the TCX chain head")
	}
}

// TestResolveContainerNameFromStore verifies resolveContainerName prefers the
// store-resolved container name (containerForNode) when populated.
func TestResolveContainerNameFromStore(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()
	e.store.ApplyContainerEvent(engine.ContainerEvent{
		LabName: "mini", NodeName: "r1", Container: "clab-mini-r1",
		State: engine.StatusRunning,
	})
	if got := e.resolveContainerName(context.Background(), "mini", "r1"); got != "clab-mini-r1" {
		t.Fatalf("expected clab-mini-r1, got %q", got)
	}
}

// TestResolveContainerNameInfersConvention verifies resolveContainerName falls
// back to the containerlab naming convention clab-<lab>-<node> when the store
// has no container info yet (freshly deployed lab), verifying via docker.
func TestResolveContainerNameInfersConvention(t *testing.T) {
	dir := t.TempDir()
	recordFile := filepath.Join(dir, "docker-args.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + recordFile + "\n" +
		"case \"$*\" in\n" +
		"  \"inspect --format {{.State.Pid}} clab-mini-r1\") echo \"12345\"; exit 0 ;;\n" +
		"  *) exit 1 ;;\n" +
		"esac\n"
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	e := newTestEngine(t)
	defer e.Close()
	if got := e.resolveContainerName(context.Background(), "mini", "r1"); got != "clab-mini-r1" {
		t.Fatalf("expected inferred clab-mini-r1, got %q", got)
	}
}

// TestResolveContainerNameUnresolvable verifies resolveContainerName returns ""
// when neither the store nor the naming-convention probe can resolve the node.
func TestResolveContainerNameUnresolvable(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	e := newTestEngine(t)
	defer e.Close()
	if got := e.resolveContainerName(context.Background(), "mini", "r1"); got != "" {
		t.Fatalf("expected empty container name, got %q", got)
	}
}

// TestListNodeIfaces verifies interface enumeration goes through
// `docker exec <container> ip -j link` and maps ifname→ifindex. This is the
// reliable path for srlinux and other kinds where /proc/<pid>/root is
// inaccessible even as root.
func TestListNodeIfaces(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "docker")
	script := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  \"exec clab-srlinux-lab-srl1 ip -j link\") printf '%s\\n' '[{\"ifname\":\"lo\",\"ifindex\":1},{\"ifname\":\"mgmt0\",\"ifindex\":2},{\"ifname\":\"monit_in\",\"ifindex\":4},{\"ifname\":\"e1-1\",\"ifindex\":32}]'; exit 0 ;;\n" +
		"  *) exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	e := newTestEngine(t)
	defer e.Close()
	m, err := e.listNodeIfaces(context.Background(), "clab-srlinux-lab-srl1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"lo": 1, "mgmt0": 2, "monit_in": 4, "e1-1": 32}
	if len(m) != len(want) {
		t.Fatalf("expected %v, got %v", want, m)
	}
	for k, v := range want {
		if m[k] != v {
			t.Fatalf("expected %s=%d, got %v", k, v, m)
		}
	}
}

// TestParseIPLinkJSON verifies the pure parser for `ip -j link` output:
// ifname→ifindex mapping, empty array, and skipping blank names / zero ifindex.
func TestParseIPLinkJSON(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want map[string]int
	}{
		{"srlinux links", `[{"ifname":"lo","ifindex":1},{"ifname":"mgmt0","ifindex":2},{"ifname":"e1-1","ifindex":32}]`, map[string]int{"lo": 1, "mgmt0": 2, "e1-1": 32}},
		{"empty array", `[]`, map[string]int{}},
		{"skips blank name and zero ifindex", `[{"ifname":"","ifindex":3},{"ifname":"x","ifindex":0}]`, map[string]int{}},
	}
	for _, tc := range cases {
		got, err := parseIPLinkJSON(tc.out)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Fatalf("%s: expected %s=%d, got %v", tc.name, k, v, got)
			}
		}
	}
	if _, err := parseIPLinkJSON("not json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestRawToPacket(t *testing.T) {
	ev := pktEventRaw{Saddr: 0x0201010A, Daddr: 0x0202020A, Sport: 179, Dport: 179, Proto: 6, Len: 100}
	p := rawToPacket(ev)
	if p.Proto != "tcp" || p.SrcPort != 179 || p.DstPort != 179 || p.Len != 100 {
		t.Fatalf("unexpected: %+v", p)
	}
	if p.SrcIP != "10.1.1.2" {
		t.Fatalf("expected 10.1.1.2, got %s", p.SrcIP)
	}
	if p.Summary != "10.1.1.2:179 -> 10.2.2.2:179" {
		t.Fatalf("unexpected summary: %q", p.Summary)
	}
}

func TestDecodePktEvent(t *testing.T) {
	raw := make([]byte, pktEventSize)
	binary.LittleEndian.PutUint32(raw[0:], 32)          // ifindex
	binary.LittleEndian.PutUint64(raw[8:], 0xDEADBEEF)  // netns_ino (skip 4-byte pad)
	binary.LittleEndian.PutUint32(raw[16:], 0x0201010A) // saddr = 10.1.1.2
	binary.LittleEndian.PutUint32(raw[20:], 0x0202020A) // daddr = 10.2.2.2
	binary.LittleEndian.PutUint16(raw[24:], 179)        // sport
	binary.LittleEndian.PutUint16(raw[26:], 179)        // dport
	raw[28] = 6                                         // proto tcp
	raw[29] = 4                                         // ipver
	binary.LittleEndian.PutUint32(raw[32:], 100)        // len (skip 2-byte padding)
	ev, err := decodePktEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Proto != 6 || ev.Sport != 179 || ev.Dport != 179 || ev.Len != 100 {
		t.Fatalf("unexpected: %+v", ev)
	}
	if ev.Ifindex != 32 || ev.NetnsIno != 0xDEADBEEF {
		t.Fatalf("unexpected identity: %+v", ev)
	}
	p := rawToPacket(ev)
	if p.Proto != "tcp" || p.SrcIP != "10.1.1.2" || p.DstIP != "10.2.2.2" {
		t.Fatalf("unexpected: %+v", p)
	}
}

// TestDecodePktEventWithPayload verifies the payload area is decoded at the
// kernel-correct offset (payload@36, layout: ifindex@0 netns@8 saddr@16
// daddr@20 sport@24 dport@26 proto@28 ipver@29 len@32 payload@36) into
// ParsedPacket.Payload. Engine level does NOT run gopacket (that happens in
// the UI layer), so we only assert the raw body bytes land at the right place.
func TestDecodePktEventWithPayload(t *testing.T) {
	raw := make([]byte, pktEventSize)
	binary.LittleEndian.PutUint32(raw[0:], 32)          // ifindex
	binary.LittleEndian.PutUint32(raw[16:], 0x0201010A) // saddr = 10.1.1.2
	binary.LittleEndian.PutUint32(raw[20:], 0x0202020A) // daddr = 10.2.2.2
	binary.LittleEndian.PutUint16(raw[24:], 179)        // sport
	binary.LittleEndian.PutUint16(raw[26:], 179)        // dport
	raw[28] = 6                                         // proto tcp
	raw[29] = 4                                         // ipver
	binary.LittleEndian.PutUint32(raw[32:], 60)         // len (header + payload)
	// Build a minimal ethernet+ip+tcp packet into payload at offset 36.
	eth := &layers.Ethernet{SrcMAC: []byte{0, 1, 2, 3, 4, 5}, DstMAC: []byte{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, SrcIP: []byte{10, 0, 0, 1}, DstIP: []byte{10, 0, 0, 2}, Protocol: layers.IPProtocolTCP}
	tcp := &layers.TCP{SrcPort: 179, DstPort: 179, SYN: true, Window: 65535}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true}
	tcp.SetNetworkLayerForChecksum(ip)
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, tcp); err != nil {
		t.Fatal(err)
	}
	body := buf.Bytes()
	copy(raw[36:], body)

	ev, err := decodePktEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	p := rawToPacket(ev)
	if len(p.Payload) < len(body) {
		t.Fatalf("expected Payload to carry %d bytes, got %d", len(body), len(p.Payload))
	}
	if !bytes.Equal(p.Payload[:len(body)], body) {
		t.Fatalf("payload decoded at wrong offset:\n got %x\nwant %x", p.Payload[:len(body)], body)
	}
}

func TestDecodePktEventRejectsWrongSize(t *testing.T) {
	if _, err := decodePktEvent(make([]byte, 10)); err == nil {
		t.Fatal("expected error for wrong size")
	}
}

// TestGroupContainerIfaces verifies container-interface grouping keeps
// containers separate even when container-netns ifindexes collide (both
// containers' eth0/mgmt are ifindex 2), and that target filters apply.
func TestGroupContainerIfaces(t *testing.T) {
	ifaces := []containerIface{
		{node: "r1", iface: "eth0", container: "clab-lab-r1", pid: 1, ifindex: 2},
		{node: "r2", iface: "eth0", container: "clab-lab-r2", pid: 2, ifindex: 2},
		{node: "r1", iface: "eth1", container: "clab-lab-r1", pid: 1, ifindex: 59},
		{node: "r2", iface: "eth1", container: "clab-lab-r2", pid: 2, ifindex: 60},
	}

	g := groupContainerIfaces(ifaces, "", "")
	if len(g) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(g))
	}
	if len(g["clab-lab-r1"]) != 2 || len(g["clab-lab-r2"]) != 2 {
		t.Fatalf("unexpected grouping: %+v", g)
	}

	g = groupContainerIfaces(ifaces, "", "eth1")
	if len(g) != 2 || len(g["clab-lab-r1"]) != 1 || g["clab-lab-r1"][0].iface != "eth1" {
		t.Fatalf("unexpected eth1-only grouping: %+v", g)
	}

	g = groupContainerIfaces(ifaces, "r1", "")
	if len(g) != 1 {
		t.Fatalf("expected only r1's container, got %d", len(g))
	}
	if _, ok := g["clab-lab-r2"]; ok {
		t.Fatal("r2's container should be filtered out")
	}
	if len(g["clab-lab-r1"]) != 2 {
		t.Fatalf("expected both r1 ifaces, got %+v", g)
	}
}
