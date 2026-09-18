package tabs

import (
	"fmt"
	"strings"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// parsePacketDetail 用 gopacket 解码 eBPF 抓到的原始以太网包体，返回
// tcpdump -ennl 风格的单行详情。payload 是 ParsedPacket.Payload（内核只
// 复制包体前 128 字节，其后为全零填充），pktLen 是真实包长（ev.Len），
// 用于截断尾部零填充。返回空串表示无法解析（不是以太网帧）。
func parsePacketDetail(payload []byte, pktLen int) string {
	if pktLen < 14 || len(payload) < 14 {
		return ""
	}
	data := payload
	if pktLen < len(data) {
		data = data[:pktLen]
	}
	p := gopacket.NewPacket(data, layers.LayerTypeEthernet,
		gopacket.DecodeOptions{Lazy: true, NoCopy: true})

	var b strings.Builder
	if eth := p.Layer(layers.LayerTypeEthernet); eth != nil {
		ethL := eth.(*layers.Ethernet)
		fmt.Fprintf(&b, "%s > %s, ethertype %s (0x%04x), length %d: ",
			ethL.SrcMAC, ethL.DstMAC, etherTypeName(ethL.EthernetType),
			uint16(ethL.EthernetType), pktLen)
	} else {
		return ""
	}
	if vlan := p.Layer(layers.LayerTypeDot1Q); vlan != nil {
		fmt.Fprintf(&b, "vlan %d, ", vlan.(*layers.Dot1Q).VLANIdentifier)
	}

	// 网络层
	if ip := p.Layer(layers.LayerTypeIPv4); ip != nil {
		ipL := ip.(*layers.IPv4)
		writeIPSummary(&b, p, ipL.SrcIP.String(), ipL.DstIP.String(),
			ipL.Protocol.String(), ipL.Protocol, pktLen, int(ipL.IHL)*4)
	} else if ip := p.Layer(layers.LayerTypeIPv6); ip != nil {
		ipL := ip.(*layers.IPv6)
		writeIPSummary(&b, p, ipL.SrcIP.String(), ipL.DstIP.String(),
			ipL.NextHeader.String(), ipL.NextHeader, pktLen, 40)
	} else if arp := p.Layer(layers.LayerTypeARP); arp != nil {
		arpL := arp.(*layers.ARP)
		fmt.Fprintf(&b, "ARP %s: %s > %s (%s)",
			arpOpName(arpL.Operation), ip4Bytes(arpL.SourceProtAddress),
			ip4Bytes(arpL.DstProtAddress), etherTypeName(arpL.Protocol))
	}
	return b.String()
}

// writeIPSummary 追加网络层 + 传输层的 tcpdump 风格摘要。
// networkHeaderLen is the L3 header length in bytes (IPv4 IHL or 40 for
// IPv6's base header). It keeps TCP payload length correct for both versions.
func writeIPSummary(b *strings.Builder, p gopacket.Packet, srcIP, dstIP, protoName string, proto layers.IPProtocol, pktLen, networkHeaderLen int) {
	switch proto {
	case layers.IPProtocolTCP:
		if tcp := p.Layer(layers.LayerTypeTCP); tcp != nil {
			tcpL := tcp.(*layers.TCP)
			// 端口转义用点分隔，如 tcpdump 的 ip.port
			fmt.Fprintf(b, "%s.%d > %s.%d: Flags [%s], seq %d",
				srcIP, tcpL.SrcPort, dstIP, tcpL.DstPort,
				tcpFlagString(tcpL), tcpL.Seq)
			if tcpL.ACK {
				fmt.Fprintf(b, ", ack %d", tcpL.Ack)
			}
			fmt.Fprintf(b, ", win %d, length %d",
				tcpL.Window, payloadLen(pktLen, 14, networkHeaderLen, int(tcpL.DataOffset)*4))
			return
		}
	case layers.IPProtocolUDP:
		if udp := p.Layer(layers.LayerTypeUDP); udp != nil {
			udpL := udp.(*layers.UDP)
			fmt.Fprintf(b, "%s.%d > %s.%d: length %d",
				srcIP, udpL.SrcPort, dstIP, udpL.DstPort, udpL.Length)
			return
		}
	case layers.IPProtocolICMPv4:
		if icmp := p.Layer(layers.LayerTypeICMPv4); icmp != nil {
			icmpL := icmp.(*layers.ICMPv4)
			fmt.Fprintf(b, "%s > %s: ICMP %s", srcIP, dstIP, icmpTypeName(icmpL.TypeCode))
			return
		}
	case layers.IPProtocolICMPv6:
		if icmp := p.Layer(layers.LayerTypeICMPv6); icmp != nil {
			icmpL := icmp.(*layers.ICMPv6)
			fmt.Fprintf(b, "%s > %s: ICMPv6 %s", srcIP, dstIP, icmpv6TypeName(icmpL.TypeCode))
			return
		}
	}
	// 无法解析传输层时回退到裸 IP 摘要
	fmt.Fprintf(b, "%s > %s: %s", srcIP, dstIP, protoName)
}

// payloadLen 计算 L4 payload 长度（pktLen 减去各层首部）。
func payloadLen(pktLen int, hdrs ...int) int {
	n := pktLen
	for _, h := range hdrs {
		n -= h
	}
	if n < 0 {
		return 0
	}
	return n
}

// tcpFlagString 生成 tcpdump 风格的 TCP 标志串，如 [S.] / [.] / [S]。
func tcpFlagString(tcp *layers.TCP) string {
	var flags strings.Builder
	if tcp.SYN {
		flags.WriteByte('S')
	}
	if tcp.FIN {
		flags.WriteByte('F')
	}
	if tcp.RST {
		flags.WriteByte('R')
	}
	if tcp.PSH {
		flags.WriteByte('P')
	}
	if tcp.ACK {
		flags.WriteByte('.')
	}
	if tcp.URG {
		flags.WriteByte('U')
	}
	if tcp.ECE {
		flags.WriteByte('E')
	}
	if tcp.CWR {
		flags.WriteByte('W')
	}
	if flags.Len() == 0 {
		return "none"
	}
	return flags.String()
}

func etherTypeName(et layers.EthernetType) string {
	switch et {
	case layers.EthernetTypeIPv4:
		return "IPv4"
	case layers.EthernetTypeIPv6:
		return "IPv6"
	case layers.EthernetTypeARP:
		return "ARP"
	case layers.EthernetTypeDot1Q:
		return "802.1Q"
	default:
		return fmt.Sprintf("0x%04x", uint16(et))
	}
}

func arpOpName(op uint16) string {
	switch op {
	case 1:
		return "who-has"
	case 2:
		return "is-at"
	default:
		return fmt.Sprintf("op-%d", op)
	}
}

func icmpTypeName(tc layers.ICMPv4TypeCode) string {
	switch tc.Type() {
	case 8:
		return "echo request"
	case 0:
		return "echo reply"
	case 3:
		return "destination unreachable"
	case 5:
		return "redirect"
	case 11:
		return "time exceeded"
	default:
		return fmt.Sprintf("type %d code %d", tc.Type(), tc.Code())
	}
}

func icmpv6TypeName(tc layers.ICMPv6TypeCode) string {
	switch tc.Type() {
	case layers.ICMPv6TypeEchoRequest:
		return "echo request"
	case layers.ICMPv6TypeEchoReply:
		return "echo reply"
	case layers.ICMPv6TypeNeighborSolicitation:
		return "neighbor solicitation"
	case layers.ICMPv6TypeNeighborAdvertisement:
		return "neighbor advertisement"
	default:
		return tc.String()
	}
}

// ip4Bytes 把 ARP 载荷里的 4 字节协议地址格式化为主机点分形式。
func ip4Bytes(b []byte) string {
	if len(b) < 4 {
		return "?"
	}
	return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
}
