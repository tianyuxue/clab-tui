package containerlab

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
	"golang.org/x/sys/unix"

	"github.com/tianyuxue/clab-tui/internal/capability"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

// pktEventRaw mirrors bpf/capture.bpf.c struct pkt_event.
//
// Memory layout: ifindex@0(4) direction@4(4) netns_ino@8(8) saddr@16(4)
// daddr@20(4) sport@24(2) dport@26(2) proto@28(1) ipver@29(1) pad(2)
// len@32(4) payload@36(128), tail-padded to 168 bytes (each ringbuf sample).
//
// direction: 1 = ingress, 2 = egress (see tc_capture_ingress/egress in BPF).
// Both directions are captured so the UI can show the complete packet path.
//
// netns_ino is actually bpf_get_netns_cookie(skb) (net->net_cookie), not the
// inode of /proc/<pid>/ns/net. It is unreliable on forwarding paths (returns
// init_net's cookie) and kept for debugging only; event attribution uses the
// per-container ring buffer + container-internal ifindex.
type pktEventRaw struct {
	Ifindex   uint32
	Direction uint32
	NetnsIno  uint64
	Saddr     uint32
	Daddr     uint32
	Sport     uint16
	Dport     uint16
	Proto     uint8
	IPVer     uint8
	Len       uint32
	Payload   [128]byte // first 128 bytes of the packet body, truncated to Len
}

// pktEventSize is the on-wire size of C struct pkt_event including alignment
// padding, also the length of every ringbuf sample.
const pktEventSize = 168

// Direction constants matching the BPF program (tc_capture_ingress/egress).
const (
	dirIngress uint32 = 1
	dirEgress  uint32 = 2
)

// directionLabel returns the short UI label for a packet direction.
func directionLabel(d uint32) string {
	switch d {
	case dirIngress:
		return "IN"
	case dirEgress:
		return "OUT"
	}
	return "?"
}

// containerIface 记录一个容器内接口（容器内 ifindex → 接口名）。
type containerIface struct {
	node      string
	iface     string
	container string
	pid       int
	ifindex   int
}

// perContainerSession 是单容器的一次抓包/追踪子会话：该容器一个独立
// BPF 对象集（含独立 events ringbuf），挂在该容器自己的接口上。
// 事件归属天然明确：本子会话读到的包只可能来自该容器。
type perContainerSession struct {
	node      string
	container string
	objs      *bpfcapObjects
	links     []link.Link
	rb        *ringbuf.Reader
	cancel    context.CancelFunc
	ifaces    map[uint32]string // 容器内 ifindex → 接口名
}

// captureSession 承载一次抓包/追踪的 eBPF 生命周期（每容器一个子会话）。
type captureSession struct {
	containers []*perContainerSession
	pcap       *capturePCAP
	done       chan struct{}
	once       sync.Once
}

type capturePCAP struct {
	mu     sync.Mutex
	file   *os.File
	writer *pcapgo.Writer
}

func newCapturePCAP(path string) (*capturePCAP, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create pcap file %q: %w", path, err)
	}
	w := pcapgo.NewWriter(f)
	if err := w.WriteFileHeader(captureFilterSnapLen, layers.LinkTypeEthernet); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write pcap header: %w", err)
	}
	return &capturePCAP{file: f, writer: w}, nil
}

func (p *capturePCAP) Write(ev pktEventRaw) error {
	if p == nil {
		return nil
	}
	length := int(ev.Len)
	if length > len(ev.Payload) {
		length = len(ev.Payload)
	}
	if length < 0 {
		length = 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.writer.WritePacket(gopacket.CaptureInfo{
		Timestamp:     time.Now(),
		CaptureLength: length,
		Length:        int(ev.Len),
	}, ev.Payload[:length])
}

func (p *capturePCAP) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.file == nil {
		return nil
	}
	err := p.file.Close()
	p.file = nil
	return err
}

func (s *captureSession) stop() error {
	var err error
	s.once.Do(func() {
		for _, sub := range s.containers {
			if sub.cancel != nil {
				sub.cancel()
			}
			for _, l := range sub.links {
				if e := l.Close(); e != nil {
					err = e
				}
			}
			if sub.rb != nil {
				_ = sub.rb.Close() // 解除 ringbuf.Read 阻塞
			}
			if sub.objs != nil {
				_ = sub.objs.Close()
			}
		}
		if s.done != nil {
			<-s.done
		}
		if s.pcap != nil {
			if e := s.pcap.Close(); e != nil {
				err = e
			}
		}
	})
	return err
}

// ---- 能力检测 ----

// PacketTraceCapable reports whether the capture/trace capability is usable
// in the current environment.
func (e *ClabEngine) PacketTraceCapable(ctx context.Context) engine.CapabilityResult {
	caps := capability.Detect(e.bin)
	if ok, reason := caps.Check(capability.ReqTrace); !ok {
		return engine.CapabilityResult{Available: false, Reason: reason}
	}
	if runtime.GOOS != "linux" {
		return engine.CapabilityResult{Available: false, Reason: "eBPF is Linux-only"}
	}
	return engine.CapabilityResult{Available: true}
}

// ---- 容器接口枚举（容器 netns 内） ----

// listContainerIfaces 枚举 lab 各运行中容器的接口（容器内 ifindex + pid）。
func (e *ClabEngine) listContainerIfaces(ctx context.Context, labName string) ([]containerIface, error) {
	var out []containerIface
	lab, err := e.GetLab(ctx, labName)
	if err != nil {
		return nil, err
	}
	for i := range lab.Nodes {
		node := &lab.Nodes[i]
		container := e.resolveContainerName(ctx, labName, node.Name)
		if container == "" {
			continue
		}
		pid, err := containerPid(container)
		if err != nil {
			continue // 容器未运行
		}
		ifaces, err := e.listNodeIfaces(ctx, container)
		if err != nil {
			continue
		}
		for ifname, ifidx := range ifaces {
			if ifname == "lo" {
				continue
			}
			out = append(out, containerIface{
				node: node.Name, iface: ifname, container: container,
				pid: pid, ifindex: ifidx,
			})
		}
	}
	return out, nil
}

// groupContainerIfaces 把容器接口按容器名分组，并按 target 过滤。
// 分组保证每个容器独立挂载/读取（其容器内 ifindex 可能有冲突，如各容器
// eth0/mgmt 均为 2，不能跨容器用 ifindex 直接归属）。
func groupContainerIfaces(ifaces []containerIface, targetNode, targetIface string) map[string][]containerIface {
	byContainer := map[string][]containerIface{}
	for _, ci := range ifaces {
		if targetNode != "" && ci.node != targetNode {
			continue
		}
		if targetIface != "" && ci.iface != targetIface {
			continue
		}
		byContainer[ci.container] = append(byContainer[ci.container], ci)
	}
	return byContainer
}

// resolveContainerName 解析节点的容器名：优先 store（containerForNode），
// 失败时按 containerlab 命名约定 clab-<lab>-<node> 推断并验证存在。
// 这样即使 store 尚未合并 live 容器信息（如刚 Deploy 完成的拓扑），
// 也能可靠拿到运行中的容器名。
func (e *ClabEngine) resolveContainerName(ctx context.Context, labName, nodeName string) string {
	if c := e.containerForNode(ctx, labName, nodeName); c != "" {
		return c
	}
	// 推断 clab-<lab>-<node>
	cand := "clab-" + labName + "-" + nodeName
	if pid, err := containerPid(cand); err == nil && pid > 0 {
		return cand
	}
	return ""
}

// containerPid 返回容器的进程 PID。
func containerPid(container string) (int, error) {
	out, err := runCmdOutput("docker", "inspect", "--format", "{{.State.Pid}}", container)
	if err != nil {
		return 0, err
	}
	var pid int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &pid)
	if pid == 0 {
		return 0, fmt.Errorf("container %s not running", container)
	}
	return pid, nil
}

// listNodeIfaces 用 `docker exec ip -j link` 枚举容器接口（ifname → ifindex）。
// 比读 /proc/<pid>/root/sys/class/net 更可靠（不依赖容器 rootfs 可访问性，
// srlinux 等 kind 即使 root 也无法访问 /proc/<pid>/root）。
func (e *ClabEngine) listNodeIfaces(ctx context.Context, container string) (map[string]int, error) {
	out, err := e.nodeExecRaw(ctx, container, "ip", "-j", "link")
	if err != nil {
		return nil, err
	}
	return parseIPLinkJSON(out)
}

// parseIPLinkJSON 解析 `ip -j link` 输出为 ifname → ifindex。
func parseIPLinkJSON(out string) (map[string]int, error) {
	var links []struct {
		Ifname  string `json:"ifname"`
		Ifindex int    `json:"ifindex"`
	}
	if err := json.Unmarshal([]byte(out), &links); err != nil {
		return nil, err
	}
	m := map[string]int{}
	for _, l := range links {
		if l.Ifname != "" && l.Ifindex > 0 {
			m[l.Ifname] = l.Ifindex
		}
	}
	return m, nil
}

// attachInNetns attaches a TC hook to a given interface inside a container's
// network namespace. The goroutine locks its OS thread so Setns and AttachTCX
// run on the same thread (Setns is thread-local); after attach it switches
// back to the original netns. attachType is ebpf.AttachTCXIngress or
// ebpf.AttachTCXEgress so the caller can attach the same program to both
// directions of an interface.
func tcxAttachOptions(prog *ebpf.Program, ifindex int, attachType ebpf.AttachType) link.TCXOptions {
	return link.TCXOptions{
		Interface: ifindex,
		Program:   prog,
		Attach:    attachType,
		Anchor:    link.Head(),
	}
}

func attachInNetns(prog *ebpf.Program, pid, ifindex int, attachType ebpf.AttachType) (link.Link, error) {
	netnsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
	netnsFd, err := os.Open(netnsPath)
	if err != nil {
		return nil, err
	}
	defer netnsFd.Close()
	origFd, err := os.Open("/proc/self/ns/net")
	if err != nil {
		return nil, err
	}
	defer origFd.Close()

	done := make(chan struct{})
	var (
		l    link.Link
		lerr error
	)
	go func() {
		runtime.LockOSThread()
		locked := true
		defer func() {
			if locked {
				runtime.UnlockOSThread()
			}
			close(done)
		}()
		if err := unix.Setns(int(netnsFd.Fd()), unix.CLONE_NEWNET); err != nil {
			lerr = err
			return
		}
		opts := tcxAttachOptions(prog, ifindex, attachType)
		// Detect a concurrent change to the existing TCX chain. Query failure
		// is non-fatal on kernels without BPF_PROG_QUERY; the attach still uses
		// Head(), which makes capture run before pre-existing programs.
		if result, err := link.QueryPrograms(link.QueryOptions{Target: ifindex, Attach: attachType}); err == nil {
			opts.ExpectedRevision = result.Revision
		}
		l, lerr = link.AttachTCX(opts)
		if lerr == nil {
			info, err := l.Info()
			if err != nil {
				lerr = fmt.Errorf("verify TCX link: %w", err)
			} else if tcx := info.TCX(); tcx == nil {
				lerr = fmt.Errorf("verify TCX link: missing TCX metadata")
			} else if tcx.Ifindex != uint32(ifindex) || uint32(tcx.AttachType) != uint32(attachType) {
				lerr = fmt.Errorf("verify TCX link: got ifindex=%d attach_type=%d, want ifindex=%d attach_type=%d", tcx.Ifindex, tcx.AttachType, ifindex, attachType)
			}
			if lerr != nil {
				_ = l.Close()
				l = nil
			}
		}
		if err := unix.Setns(int(origFd.Fd()), unix.CLONE_NEWNET); err != nil {
			lerr = err
			locked = false // 切回失败：保持线程锁定以被终止，避免污染调度池
			if l != nil {
				_ = l.Close()
				l = nil
			}
			return
		}
	}()
	<-done
	return l, lerr
}

// ---- 主入口 ----

// TracePath 对全拓扑容器接口抓包。
func (e *ClabEngine) TracePath(ctx context.Context, labName, filter string) (<-chan engine.PacketEvent, func() error, error) {
	return e.startCapture(ctx, labName, "", "", filter, "")
}

// Capture 对单接口抓包。
func (e *ClabEngine) Capture(ctx context.Context, labName, nodeName, iface string, opts ...engine.CaptureOption) (<-chan engine.PacketEvent, func() error, error) {
	cfg := &engine.CaptureOptions{}
	for _, o := range opts {
		o(cfg)
	}
	return e.startCapture(ctx, labName, nodeName, iface, cfg.Filter, cfg.File)
}

// startCapture loads an independent eBPF object set per target container (each
// with its own events ringbuf), attaches the capture program to both the
// ingress and egress TCX hooks of every container interface, and spawns one
// reader goroutine per container. Event attribution is unambiguous: a packet
// read from a container's ringbuf can only come from that container; the
// interface name is resolved by the container-internal ifindex. We do not
// rely on the netns cookie or the host ifindex for attribution (the cookie is
// unreliable on forwarding paths, and eth0/mgmt share container-internal
// ifindex 2 across containers).
//
// Both directions are attached so the UI can distinguish packets observed on
// the receiving and transmitting sides of each interface.
//
// targetNode/targetIface non-empty: attach only the named interface (single
// interface capture). filter uses full libpcap/tcpdump syntax.
func (e *ClabEngine) startCapture(ctx context.Context, labName, targetNode, targetIface, filter, pcapPath string) (<-chan engine.PacketEvent, func() error, error) {
	bpfFilter, err := compileCaptureFilter(filter)
	if err != nil {
		return nil, nil, err
	}
	var pcapFile *capturePCAP
	if pcapPath != "" {
		pcapFile, err = newCapturePCAP(pcapPath)
		if err != nil {
			return nil, nil, err
		}
	}
	ifaces, err := e.listContainerIfaces(ctx, labName)
	if err != nil {
		_ = pcapFile.Close()
		return nil, nil, err
	}
	if len(ifaces) == 0 {
		_ = pcapFile.Close()
		return nil, nil, fmt.Errorf("no running container interfaces found")
	}
	byContainer := groupContainerIfaces(ifaces, targetNode, targetIface)
	if len(byContainer) == 0 {
		_ = pcapFile.Close()
		return nil, nil, fmt.Errorf("no interfaces matched the target")
	}

	sess := &captureSession{pcap: pcapFile, done: make(chan struct{})}
	out := make(chan engine.PacketEvent, 256)
	var wg sync.WaitGroup
	var attachIssues []engine.CaptureIssue

	for _, cis := range byContainer {
		objs := &bpfcapObjects{}
		if err := loadBpfcapObjects(objs, nil); err != nil {
			// Stay silent: stderr would corrupt the TUI renderer. The caller
			// reports a generic error if no container ends up attached.
			_ = err
			continue
		}
		sub := &perContainerSession{
			node: cis[0].node, container: cis[0].container, objs: objs,
			ifaces: map[uint32]string{},
		}
		for _, ci := range cis {
			// Attach both directions of every interface. The UI tags each
			// event IN/OUT so packet direction is unambiguous.
			if l, err := attachInNetns(objs.TcCaptureIngress, ci.pid, ci.ifindex, ebpf.AttachTCXIngress); err == nil {
				sub.links = append(sub.links, l)
			} else {
				attachIssues = append(attachIssues, engine.CaptureIssue{
					Node: cis[0].node, Interface: ci.iface, Direction: "ingress", Err: err,
				})
			}
			if l, err := attachInNetns(objs.TcCaptureEgress, ci.pid, ci.ifindex, ebpf.AttachTCXEgress); err == nil {
				sub.links = append(sub.links, l)
			} else {
				attachIssues = append(attachIssues, engine.CaptureIssue{
					Node: cis[0].node, Interface: ci.iface, Direction: "egress", Err: err,
				})
			}
			sub.ifaces[uint32(ci.ifindex)] = ci.iface
		}
		if len(sub.links) == 0 {
			_ = objs.Close()
			continue // no hook attached for this container, skip
		}
		rb, err := ringbuf.NewReader(objs.Events)
		if err != nil {
			_ = objs.Close()
			continue
		}
		sub.rb = rb
		subCtx, cancel := context.WithCancel(ctx)
		sub.cancel = cancel
		sess.containers = append(sess.containers, sub)

		wg.Add(1)
		go func(sub *perContainerSession) {
			defer wg.Done()
			defer rb.Close()
			defer objs.Close()
			for {
				select {
				case <-subCtx.Done():
					return
				default:
				}
				record, err := rb.Read()
				if err != nil {
					if subCtx.Err() != nil {
						return
					}
					continue
				}
				ev, err := decodePktEvent(record.RawSample)
				if err != nil {
					continue
				}
				pkt := rawToPacket(ev)
				if !captureFilterMatches(bpfFilter, ev.Payload[:], int(ev.Len)) {
					continue
				}
				iface, ok := sub.ifaces[ev.Ifindex]
				if !ok {
					continue // event for an interface we did not attach
				}
				_ = sess.pcap.Write(ev)
				select {
				case out <- engine.PacketEvent{Node: sub.node, Iface: iface, Pkt: pkt}:
				case <-subCtx.Done():
					return
				}
			}
		}(sub)
	}
	if len(sess.containers) == 0 {
		_ = sess.pcap.Close()
		if len(attachIssues) > 0 {
			return nil, nil, fmt.Errorf("capture failed: no direction attached: %w", &engine.CaptureWarningError{Issues: attachIssues})
		}
		return nil, nil, fmt.Errorf("no container attached (need root)")
	}
	go func() {
		wg.Wait()
		close(out)
		close(sess.done)
	}()
	if len(attachIssues) > 0 {
		return out, sess.stop, &engine.CaptureWarningError{Issues: attachIssues}
	}
	return out, sess.stop, nil
}

// ---- 解析 ----

// decodePktEvent parses a raw ringbuf sample into pktEventRaw, using explicit
// offsets to match the C struct layout (padding between ifindex/direction and
// between ipver/len must be skipped).
func decodePktEvent(b []byte) (pktEventRaw, error) {
	var ev pktEventRaw
	if len(b) != pktEventSize {
		return ev, fmt.Errorf("bad event size %d, want %d", len(b), pktEventSize)
	}
	ev.Ifindex = binary.LittleEndian.Uint32(b[0:4])
	ev.Direction = binary.LittleEndian.Uint32(b[4:8])
	ev.NetnsIno = binary.LittleEndian.Uint64(b[8:16])
	ev.Saddr = binary.LittleEndian.Uint32(b[16:20])
	ev.Daddr = binary.LittleEndian.Uint32(b[20:24])
	ev.Sport = binary.LittleEndian.Uint16(b[24:26])
	ev.Dport = binary.LittleEndian.Uint16(b[26:28])
	ev.Proto = b[28]
	ev.IPVer = b[29]
	ev.Len = binary.LittleEndian.Uint32(b[32:36])
	copy(ev.Payload[:], b[36:164])
	return ev, nil
}

// rawToPacket converts a raw event to a user-space ParsedPacket. gopacket
// parsing is deliberately done in the UI layer (tabs.parsePacketDetail) on
// demand; here we only carry the raw packet body and the direction label so
// the UI can show whether the packet was captured on ingress or egress.
func rawToPacket(ev pktEventRaw) engine.ParsedPacket {
	proto := "?"
	switch ev.Proto {
	case 1:
		proto = "icmp"
	case 6:
		proto = "tcp"
	case 17:
		proto = "udp"
	}
	src := ip4Str(ev.Saddr)
	dst := ip4Str(ev.Daddr)
	return engine.ParsedPacket{
		Time:      time.Now().Format("15:04:05.000000"),
		Direction: directionLabel(ev.Direction),
		SrcIP:     src,
		DstIP:     dst,
		SrcPort:   int(ev.Sport),
		DstPort:   int(ev.Dport),
		Proto:     proto,
		Len:       int(ev.Len),
		Summary:   fmt.Sprintf("%s:%d -> %s:%d", src, ev.Sport, dst, ev.Dport),
		Payload:   ev.Payload[:],
	}
}

func ip4Str(ip uint32) string {
	return net.IP{byte(ip), byte(ip >> 8), byte(ip >> 16), byte(ip >> 24)}.String()
}

const captureFilterSnapLen = 128

// compileCaptureFilter compiles a complete tcpdump/libpcap expression for an
// Ethernet frame. Empty input intentionally means "match everything".
func compileCaptureFilter(expression string) (*pcap.BPF, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, nil
	}
	filter, err := pcap.NewBPF(layers.LinkTypeEthernet, captureFilterSnapLen, expression)
	if err != nil {
		return nil, fmt.Errorf("invalid capture filter %q: %w", expression, err)
	}
	return filter, nil
}

// captureFilterMatches evaluates a compiled libpcap filter against the raw
// Ethernet sample copied by eBPF. The BPF snapshot may be shorter than the
// actual packet, so CaptureInfo preserves the full length while data is
// truncated to the available bytes.
func captureFilterMatches(filter *pcap.BPF, payload []byte, packetLen int) bool {
	if filter == nil {
		return true
	}
	if packetLen < 0 {
		return false
	}
	sampleLen := len(payload)
	if packetLen < sampleLen {
		sampleLen = packetLen
	}
	if sampleLen <= 0 {
		return false
	}
	return filter.Matches(gopacket.CaptureInfo{
		CaptureLength: sampleLen,
		Length:        packetLen,
	}, payload[:sampleLen])
}

// runCmdOutput 运行命令返回 stdout。
func runCmdOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
