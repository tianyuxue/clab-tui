#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <linux/if_ether.h>
#include <linux/in.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>

// pkt_event: packet metadata extracted from __sk_buff in TC hook.
// Direction is set by the two program variants (ingress vs egress) so the
// user space can tell which way the packet was flowing on the wire.
struct pkt_event {
	__u32 ifindex;     // container-internal ifindex of the receiving iface
	__u32 direction;   // 1 = ingress, 2 = egress
	__u64 netns_ino;   // netns cookie (debug only, see pktEventRaw.NetnsIno doc)
	__u32 saddr;       // IPv4 source, network order
	__u32 daddr;       // IPv4 destination, network order
	__u16 sport;       // L4 source port
	__u16 dport;       // L4 destination port
	__u8  proto;       // IP protocol number (6=tcp 17=udp 1=icmp)
	__u8  ipver;       // 4 or 6
	__u32 len;         // packet length, including any synthesized VLAN tag
	// First bytes of the frame body. 132 bytes keeps the struct at 168 bytes
	// (36 + 132) and leaves room for the 4-byte VLAN tag synthesized below.
	__u8  payload[132];
};

#define VLAN_HLEN 4

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 20); // 1 MiB
} events SEC(".maps");

// parse_ip fills L3 fields from the network header and returns the L4 offset
// relative to the frame start, or 0 on a non-IP / truncated packet (length is
// still reported upstream). Handles in-band 802.1Q/802.1ad tags; an offloaded
// tag lives in skb metadata and is not part of skb->data, in which case the
// frame starts directly with the inner ethertype.
static __always_inline unsigned int parse_ip(void *data, void *data_end, struct pkt_event *ev) {
	struct ethhdr *eth = data;
	if ((void *)(eth + 1) > data_end)
		return 0;
	__u16 proto = eth->h_proto;
	unsigned int off = sizeof(struct ethhdr);
	if (proto == __constant_htons(ETH_P_8021Q) || proto == __constant_htons(ETH_P_8021AD)) {
		__be16 *inner = (void *)(eth + 1);
		if ((void *)(inner + 1) > data_end)
			return 0;
		proto = *inner;
		off += VLAN_HLEN;
	}
	if (proto == __constant_htons(ETH_P_IP)) {
		struct iphdr *ip = (void *)((char *)data + off);
		if ((void *)(ip + 1) > data_end)
			return 0;
		ev->ipver = 4;
		ev->saddr = ip->saddr;
		ev->daddr = ip->daddr;
		ev->proto = ip->protocol;
		return off + ip->ihl * 4;
	}
	if (proto == __constant_htons(ETH_P_IPV6)) {
		struct ipv6hdr *ip6 = (void *)((char *)data + off);
		if ((void *)(ip6 + 1) > data_end)
			return 0;
		ev->ipver = 6;
		ev->proto = ip6->nexthdr;
		return off + sizeof(struct ipv6hdr);
	}
	return 0;
}

static __always_inline void parse_l4(void *l4, void *data_end, struct pkt_event *ev) {
	if (ev->proto == IPPROTO_TCP) {
		struct tcphdr *tcp = l4;
		if ((void *)(tcp + 1) <= data_end) {
			ev->sport = bpf_ntohs(tcp->source);
			ev->dport = bpf_ntohs(tcp->dest);
		}
	} else if (ev->proto == IPPROTO_UDP) {
		struct udphdr *udp = l4;
		if ((void *)(udp + 1) <= data_end) {
			ev->sport = bpf_ntohs(udp->source);
			ev->dport = bpf_ntohs(udp->dest);
		}
	}
}

// capture is the shared body of the two TC hook variants. direction is set by
// the caller so the user space can distinguish ingress from egress.
static __always_inline int capture(struct __sk_buff *skb, __u32 direction) {
	struct pkt_event *ev = bpf_ringbuf_reserve(&events, sizeof(*ev), 0);
	if (!ev)
		return TC_ACT_OK;

	void *data = (void *)(long)skb->data;
	void *data_end = (void *)(long)skb->data_end;
	ev->ifindex = skb->ifindex;
	ev->direction = direction;
	ev->len = skb->len;

	// netns cookie: reports the netns of skb->sk (init_net on forwarding /
	// pure-receive paths where skb->sk is NULL). Kept for debugging only;
	// event attribution relies on the per-container ring buffer instead.
	ev->netns_ino = bpf_get_netns_cookie(skb);

	unsigned int l4off = parse_ip(data, data_end, ev);
	if (l4off > 0) {
		void *l4 = data + l4off;
		parse_l4(l4, data_end, ev);
	}

	// Copy the frame bytes for gopacket in user space. Use the real packet
	// length (a fixed-size request fails for short packets and leaves the
	// payload zeroed, producing 0.0.0.0 addresses).
	//
	// veth (and most NICs) offload 802.1Q tags into skb metadata by default.
	// That tag is not part of skb->data, so rebuild the 4-byte header after
	// the two MAC addresses and add it to the reported length. Without this the
	// capture loses every VLAN tag and libpcap filters such as "vlan 10" can
	// never match. skb->len excludes an offloaded tag, hence the +VLAN_HLEN.
	__u32 copy_len = skb->len;
	if (skb->vlan_present) {
		// The tag belongs after the two MAC addresses: keep bytes 0..11 and
		// copy the rest from offset 12 to payload offset 16.
		if (copy_len > sizeof(ev->payload) - VLAN_HLEN)
			copy_len = sizeof(ev->payload) - VLAN_HLEN;
		if (copy_len > 12) {
			__u32 body_len = copy_len - 12;
			bpf_skb_load_bytes(skb, 0, ev->payload, 12);
			bpf_skb_load_bytes(skb, 12, ev->payload + 16, body_len);
		} else if (copy_len > 0) {
			bpf_skb_load_bytes(skb, 0, ev->payload, copy_len);
		}
		__u16 tci = (__u16)skb->vlan_tci;
		if ((__u16)skb->vlan_proto == __constant_htons(ETH_P_8021AD)) {
			ev->payload[12] = 0x88;
			ev->payload[13] = 0xa8;
		} else {
			ev->payload[12] = 0x81;
			ev->payload[13] = 0x00;
		}
		ev->payload[14] = (__u8)(tci >> 8);
		ev->payload[15] = (__u8)(tci & 0xff);
		ev->len += VLAN_HLEN;
	} else {
		if (copy_len > sizeof(ev->payload))
			copy_len = sizeof(ev->payload);
		if (copy_len > 0)
			bpf_skb_load_bytes(skb, 0, ev->payload, copy_len);
	}

	bpf_ringbuf_submit(ev, 0);
	return TC_ACT_OK;
}

// tc_capture_ingress is attached with AttachTCXIngress.
SEC("tc")
int tc_capture_ingress(struct __sk_buff *skb) {
	return capture(skb, 1);
}

// tc_capture_egress is attached with AttachTCXEgress. Always fires for packets
// produced by the node, regardless of kind, so this is the reliable side.
SEC("tc")
int tc_capture_egress(struct __sk_buff *skb) {
	return capture(skb, 2);
}

char LICENSE[] SEC("license") = "GPL";
