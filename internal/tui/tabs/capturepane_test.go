package tabs

import (
	"net"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/muesli/termenv"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

// samplePacket serializes a minimal ethernet+ip+tcp packet.
func samplePacket(t *testing.T) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: []byte{0, 1, 2, 3, 4, 5}, DstMAC: []byte{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, SrcIP: []byte{10, 0, 0, 1}, DstIP: []byte{10, 0, 0, 2}, Protocol: layers.IPProtocolTCP}
	tcp := &layers.TCP{SrcPort: 179, DstPort: 179, SYN: true, Window: 65535}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true}
	tcp.SetNetworkLayerForChecksum(ip)
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, tcp); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCapturePaneAppendAndRender(t *testing.T) {
	p := NewCapturePane()
	p.SetSize(80, 20)
	p.Append(engine.PacketEvent{Node: "r1", Iface: "e1-1", Pkt: engine.ParsedPacket{
		SrcIP: "10.0.0.1", DstIP: "10.0.0.2", Proto: "tcp", SrcPort: 179, DstPort: 179, Len: 100, Summary: "10.0.0.1:179 -> 10.0.0.2:179",
	}})
	v := p.View()
	if !strings.Contains(v, "10.0.0.1") || !strings.Contains(v, "tcp") {
		t.Fatalf("expected packet in capture pane:\n%s", v)
	}
}

func TestCapturePaneClear(t *testing.T) {
	p := NewCapturePane()
	p.SetSize(80, 20)
	p.Append(engine.PacketEvent{Node: "r1", Iface: "e1-1", Pkt: engine.ParsedPacket{SrcIP: "10.0.0.1"}})
	p.Clear()
	if len(p.lines) != 0 {
		t.Fatalf("expected empty after clear")
	}
}

func TestCapturePaneVisibleToggle(t *testing.T) {
	p := NewCapturePane()
	p.Show()
	if !p.Visible() {
		t.Fatal("expected visible after Show")
	}
	p.Hide()
	if p.Visible() {
		t.Fatal("expected hidden after Hide")
	}
}

func TestCapturePaneHasBorder(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	p := NewCapturePane()
	p.SetSize(80, 10)
	p.Show()
	p.Append(engine.PacketEvent{Node: "r1", Iface: "eth0", Pkt: engine.ParsedPacket{SrcIP: "10.0.0.1", DstIP: "10.0.0.2"}})
	v := p.View()
	if !strings.Contains(v, "╭") || !strings.Contains(v, "╰") {
		t.Fatalf("expected bordered capture pane:\n%s", v)
	}
}

// TestCapturePaneEmptyAndMatchedSameHeight verifies the empty pane occupies the
// exact same number of rows as a pane with captured packets, so the overlay
// does not resize (or leave a tiny popup) before the first match arrives.
func TestCapturePaneEmptyAndMatchedSameHeight(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	empty := NewCapturePane()
	empty.SetSize(100, 12)
	empty.Show()
	empty.Clear()
	emptyH := len(strings.Split(empty.View(), "\n"))

	full := NewCapturePane()
	full.SetSize(100, 12)
	full.Show()
	full.Append(engine.PacketEvent{Node: "r1", Iface: "eth0", Pkt: engine.ParsedPacket{
		Proto: "tcp", SrcIP: "10.0.0.1", DstIP: "10.0.0.2",
	}})
	fullH := len(strings.Split(full.View(), "\n"))

	if emptyH != fullH {
		t.Fatalf("empty pane height %d != matched pane height %d\nempty:\n%s\nfull:\n%s",
			emptyH, fullH, empty.View(), full.View())
	}
}

// TestCapturePaneHintAlwaysShown verifies the [q] close / [c] clear hints stay
// visible both before and after packets arrive.
func TestCapturePaneHintAlwaysShown(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	p := NewCapturePane()
	p.SetSize(100, 12)
	p.Show()
	p.Clear()
	if !strings.Contains(p.View(), "[q] close") || !strings.Contains(p.View(), "[c] clear") {
		t.Fatalf("expected hints in empty pane:\n%s", p.View())
	}
	p.Append(engine.PacketEvent{Node: "r1", Iface: "eth0", Pkt: engine.ParsedPacket{
		Proto: "tcp", SrcIP: "10.0.0.1", DstIP: "10.0.0.2",
	}})
	if !strings.Contains(p.View(), "[q] close") || !strings.Contains(p.View(), "[c] clear") {
		t.Fatalf("expected hints still present after packets:\n%s", p.View())
	}
}

func TestCapturePaneTracksDirectionalStats(t *testing.T) {
	p := NewCapturePane()
	p.SetSize(80, 8)
	p.Append(engine.PacketEvent{Pkt: engine.ParsedPacket{Direction: "IN", Len: 64}})
	p.Append(engine.PacketEvent{Pkt: engine.ParsedPacket{Direction: "OUT", Len: 128}})
	stats := p.Stats()
	if stats.Total != 2 || stats.In != 1 || stats.Out != 1 || stats.Bytes != 192 {
		t.Fatalf("unexpected capture stats: %+v", stats)
	}
	if !strings.Contains(p.View(), "pkts 2") || !strings.Contains(p.View(), "IN 1") || !strings.Contains(p.View(), "OUT 1") {
		t.Fatalf("expected stats in capture pane:\n%s", p.View())
	}
}

func TestCapturePanePauseStopsAutoScroll(t *testing.T) {
	p := NewCapturePane()
	p.SetSize(80, 3)
	for i := 0; i < 10; i++ {
		p.Append(engine.PacketEvent{Pkt: engine.ParsedPacket{Summary: "packet"}})
	}
	p.TogglePaused()
	before := p.vp.YOffset
	p.Append(engine.PacketEvent{Pkt: engine.ParsedPacket{Summary: "new packet"}})
	if p.vp.YOffset != before {
		t.Fatalf("paused capture scrolled from %d to %d", before, p.vp.YOffset)
	}
	if !strings.Contains(p.View(), "PAUSED") {
		t.Fatalf("expected paused indicator:\n%s", p.View())
	}
}

// TestWrapLinesLongLinesWrapToWidth verifies long tcpdump-style lines are soft
// wrapped to the viewport width so no packet detail is horizontally clipped.
func TestWrapLinesLongLinesWrapToWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	// A long detail that must be split across several visual rows.
	long := "00:01:02:03:04:05 > 06:07:08:09:0a:0b, ethertype IPv4 (0x0800), length 60: 10.0.0.1.179 > 10.0.0.2.179: Flags [S], seq 0, win 65535, length 0"
	out := wrapLines([]string{long}, 50)
	for _, l := range strings.Split(out, "\n") {
		if visualWidth(l) > 50 {
			t.Fatalf("wrapped line wider than 50: %q (vis=%d)", l, visualWidth(l))
		}
	}
	if visualWidth(strings.Join(strings.Split(out, "\n"), "")) != visualWidth(long) {
		t.Fatalf("wrapping lost content:\n%s", out)
	}
}

// TestWrapLinesPreservesANSI verifies ANSI codes survive wrapping intact and
// that the visible width of wrapped output matches the original.
func TestWrapLinesPreservesANSI(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("r1:eth0") +
		"  " + lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render("tcp") +
		"  " + strings.Repeat("x", 90)
	original := visualWidth(styled)
	out := wrapLines([]string{styled}, 40)
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI codes preserved in wrapped output:\n%s", out)
	}
	if visualWidth(strings.Join(strings.Split(out, "\n"), "")) != original {
		t.Fatalf("wrapping lost content: orig vis=%d, got %d", original, visualWidth(strings.Join(strings.Split(out, "\n"), "")))
	}
}

func TestParsePacketDetailEmptyForShortPayload(t *testing.T) {
	if got := parsePacketDetail(nil, 0); got != "" {
		t.Fatalf("expected empty detail for nil payload, got %q", got)
	}
	if got := parsePacketDetail([]byte{0, 1, 2}, 3); got != "" {
		t.Fatalf("expected empty detail for <14-byte payload, got %q", got)
	}
}

func TestParsePacketDetailDecodesEthernetTCP(t *testing.T) {
	body := samplePacket(t)
	detail := parsePacketDetail(body, len(body))
	if detail == "" {
		t.Fatal("expected non-empty detail from ethernet/tcp payload")
	}
	if !strings.Contains(detail, "ethertype IPv4 (0x0800)") {
		t.Fatalf("expected ethertype in detail, got %q", detail)
	}
	if !strings.Contains(detail, "Flags [S]") {
		t.Fatalf("expected TCP flags in detail, got %q", detail)
	}
	if !strings.Contains(detail, "10.0.0.1.179 > 10.0.0.2.179") {
		t.Fatalf("expected ip.port direction in detail, got %q", detail)
	}
}

// TestParsePacketDetailDecodesEthernetUDP verifies the UDP branch of the
// tcpdump-style formatter (ip.port > ip.port: length N).
func TestParsePacketDetailDecodesEthernetUDP(t *testing.T) {
	eth := &layers.Ethernet{SrcMAC: []byte{0, 1, 2, 3, 4, 5}, DstMAC: []byte{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, SrcIP: []byte{10, 0, 0, 1}, DstIP: []byte{10, 0, 0, 2}, Protocol: layers.IPProtocolUDP}
	udp := &layers.UDP{SrcPort: 5353, DstPort: 5353}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true}
	udp.SetNetworkLayerForChecksum(ip)
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, udp); err != nil {
		t.Fatal(err)
	}
	body := buf.Bytes()
	detail := parsePacketDetail(body, len(body))
	if detail == "" {
		t.Fatal("expected non-empty detail from ethernet/udp payload")
	}
	if !strings.Contains(detail, "10.0.0.1.5353 > 10.0.0.2.5353") {
		t.Fatalf("expected udp direction in detail, got %q", detail)
	}
	if !strings.Contains(detail, "length") {
		t.Fatalf("expected udp length in detail, got %q", detail)
	}
}

func TestParsePacketDetailDecodesVLAN(t *testing.T) {
	eth := &layers.Ethernet{SrcMAC: []byte{0, 1, 2, 3, 4, 5}, DstMAC: []byte{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeDot1Q}
	vlan := &layers.Dot1Q{VLANIdentifier: 100, Type: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, SrcIP: []byte{10, 0, 0, 1}, DstIP: []byte{10, 0, 0, 2}, Protocol: layers.IPProtocolUDP}
	udp := &layers.UDP{SrcPort: 179, DstPort: 179}
	udp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true}, eth, vlan, ip, udp); err != nil {
		t.Fatal(err)
	}
	detail := parsePacketDetail(buf.Bytes(), len(buf.Bytes()))
	if !strings.Contains(detail, "vlan 100") || !strings.Contains(detail, "10.0.0.1.179 > 10.0.0.2.179") {
		t.Fatalf("expected VLAN and UDP details, got %q", detail)
	}
}

func TestParsePacketDetailDecodesIPv6ICMP(t *testing.T) {
	eth := &layers.Ethernet{SrcMAC: []byte{0, 1, 2, 3, 4, 5}, DstMAC: []byte{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv6}
	ip := &layers.IPv6{Version: 6, HopLimit: 64, SrcIP: net.ParseIP("2001:db8::1").To16(), DstIP: net.ParseIP("2001:db8::2").To16(), NextHeader: layers.IPProtocolICMPv6}
	icmp := &layers.ICMPv6{TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeEchoRequest, 0)}
	icmp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true}, eth, ip, icmp); err != nil {
		t.Fatal(err)
	}
	detail := parsePacketDetail(buf.Bytes(), len(buf.Bytes()))
	if !strings.Contains(detail, "2001:db8::1 > 2001:db8::2") || !strings.Contains(detail, "ICMPv6 echo request") {
		t.Fatalf("expected IPv6 ICMP details, got %q", detail)
	}
}

// TestParsePacketDetailDecodesARP verifies the ARP branch of the formatter.
func TestParsePacketDetailDecodesARP(t *testing.T) {
	eth := &layers.Ethernet{SrcMAC: []byte{0, 1, 2, 3, 4, 5}, DstMAC: []byte{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeARP}
	arp := &layers.ARP{
		AddrType: layers.LinkTypeEthernet, Protocol: layers.EthernetTypeIPv4,
		HwAddressSize: 6, ProtAddressSize: 4,
		Operation:         layers.ARPRequest,
		SourceHwAddress:   []byte{0, 1, 2, 3, 4, 5},
		SourceProtAddress: []byte{10, 0, 0, 1},
		DstHwAddress:      []byte{6, 7, 8, 9, 10, 11},
		DstProtAddress:    []byte{10, 0, 0, 2},
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true}
	if err := gopacket.SerializeLayers(buf, opts, eth, arp); err != nil {
		t.Fatal(err)
	}
	body := buf.Bytes()
	detail := parsePacketDetail(body, len(body))
	if detail == "" {
		t.Fatal("expected non-empty detail from arp payload")
	}
	if !strings.Contains(detail, "ARP") || !strings.Contains(detail, "who-has") {
		t.Fatalf("expected ARP who-has in detail, got %q", detail)
	}
}

// TestParsePacketDetailTruncatesToPktLen verifies trailing zero padding beyond
// the real packet length is ignored (kernel copies only the first 128 bytes).
func TestParsePacketDetailTruncatesToPktLen(t *testing.T) {
	body := samplePacket(t)
	padded := append([]byte{}, body...)
	padded = append(padded, make([]byte, 128-len(body))...)
	detail := parsePacketDetail(padded, len(body))
	if detail == "" || !strings.Contains(detail, "Flags [S]") {
		t.Fatalf("expected TCP detail from padded payload, got %q", detail)
	}
}

func TestCapturePaneAppendShowsDetailWhenPayloadPresent(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	body := samplePacket(t)
	p := NewCapturePane()
	p.SetSize(80, 20)
	p.Append(engine.PacketEvent{Node: "r1", Iface: "e1-1", Pkt: engine.ParsedPacket{
		Proto: "tcp", Len: len(body), Payload: body, Summary: "plain summary",
	}})
	v := p.View()
	if strings.Contains(v, "plain summary") {
		t.Fatalf("expected gopacket detail to replace plain summary:\n%s", v)
	}
	if !strings.Contains(v, "Flags [S]") {
		t.Fatalf("expected TCP flags rendered in pane:\n%s", v)
	}
}

// TestCapturePaneWidthIncludesBorderAndPadding ensures the viewport width is
// reduced by border(2)+padding(2) so the rendered pane never overflows.
func TestCapturePaneWidthIncludesBorderAndPadding(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	p := NewCapturePane()
	p.SetSize(80, 10)
	p.Show()
	p.Append(engine.PacketEvent{Node: "r1", Iface: "e1-1", Pkt: engine.ParsedPacket{
		Proto: "tcp", Summary: strings.Repeat("x", 400),
	}})
	for _, line := range strings.Split(p.View(), "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Fatalf("line width %d exceeds pane width 80:\n%q", w, line)
		}
	}
}
