#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <linux/if_ether.h>
#include <linux/in.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/tcp.h>
#include <linux/udp.h>
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
	__u32 len;         // packet length
	__u8  payload[128]; // first 128 bytes of the packet body
};

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 20); // 1 MiB
} events SEC(".maps");

// parse_ip fills L3 fields from the network header and returns the L4 offset,
// or 0 on a non-IP / truncated packet (length is still reported upstream).
static __always_inline unsigned int parse_ip(void *data, void *data_end, struct pkt_event *ev) {
	struct ethhdr *eth = data;
	if ((void *)(eth + 1) > data_end)
		return 0;
	__u16 proto = eth->h_proto;
	if (proto == __constant_htons(ETH_P_IP)) {
		struct iphdr *ip = (void *)(eth + 1);
		if ((void *)(ip + 1) > data_end)
			return 0;
		ev->ipver = 4;
		ev->saddr = ip->saddr;
		ev->daddr = ip->daddr;
		ev->proto = ip->protocol;
		return ip->ihl * 4;
	}
	if (proto == __constant_htons(ETH_P_IPV6)) {
		struct ipv6hdr *ip6 = (void *)(eth + 1);
		if ((void *)(ip6 + 1) > data_end)
			return 0;
		ev->ipver = 6;
		ev->proto = ip6->nexthdr;
		return sizeof(struct ipv6hdr);
	}
	return 0;
}

static __always_inline void parse_l4(void *l4, void *data_end, struct pkt_event *ev) {
	if (ev->proto == IPPROTO_TCP) {
		struct tcphdr *tcp = l4;
		if ((void *)(tcp + 1) <= data_end) {
			ev->sport = tcp->source;
			ev->dport = tcp->dest;
		}
	} else if (ev->proto == IPPROTO_UDP) {
		struct udphdr *udp = l4;
		if ((void *)(udp + 1) <= data_end) {
			ev->sport = udp->source;
			ev->dport = udp->dest;
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

	// Copy up to the first 128 bytes for gopacket in user space. A fixed 128
	// byte request fails for short packets (a normal ICMP packet is often under
	// 128 bytes), leaving the payload zeroed and producing 0.0.0.0 addresses.
	__u32 copy_len = skb->len;
	if (copy_len > sizeof(ev->payload))
		copy_len = sizeof(ev->payload);
	if (copy_len > 0)
		bpf_skb_load_bytes(skb, 0, ev->payload, copy_len);

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
