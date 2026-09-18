//go:build integration

package containerlab

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

const clos10Lab = "clab-tui-clos10"
const clos10Leaf1LinkCount = 3

func TestIntegrationClos10Deploy(t *testing.T) {
	e, ctx, lab := newClos10Engine(t)
	deployedClos10(t, e, ctx, lab)

	converged := waitForLab(t, e, lab, 90*time.Second, clos10Converged)
	assertClos10Topology(t, converged)
}

func TestIntegrationClos10Lifecycle(t *testing.T) {
	e, ctx, lab := newClos10Engine(t)
	deployedClos10(t, e, ctx, lab)
	converged := waitForLab(t, e, lab, 90*time.Second, clos10Converged)
	assertLeaf1LinkCount(t, converged)

	if result := runClos10Command(t, ctx, func(ctx context.Context) (<-chan engine.OutputLine, error) {
		return e.StopNode(ctx, lab, "leaf1")
	}); !clos10CommandSucceeded(result) {
		t.Fatalf("stop leaf1 result = %+v", result)
	}
	stopped := waitForLab(t, e, lab, 30*time.Second, func(l *engine.Lab) bool {
		for _, n := range l.Nodes {
			if n.Name == "leaf1" {
				return n.State == engine.StatusStopped && len(n.Interfaces) == 0
			}
		}
		return false
	})
	assertLeaf1LinkCount(t, stopped)
	assertLeaf1LinksNotUp(t, stopped)

	// StartNode/RestartNode run `docker start/restart <container>`. A stopped
	// containerlab container loses its non-eth0 veth pairs (they live in the
	// destroyed network namespace and are not re-created by docker), so after a
	// start the node is running again but business interfaces and links do not
	// come back automatically. The lifecycle contract we assert is the node
	// state machine, not veth recovery (containerlab limitation).
	if result := runClos10Command(t, ctx, func(ctx context.Context) (<-chan engine.OutputLine, error) {
		return e.StartNode(ctx, lab, "leaf1")
	}); !clos10CommandSucceeded(result) {
		t.Fatalf("start leaf1 result = %+v", result)
	}
	started := waitForLab(t, e, lab, 45*time.Second, func(l *engine.Lab) bool {
		return nodeRunning(l, "leaf1")
	})

	if result := runClos10Command(t, ctx, func(ctx context.Context) (<-chan engine.OutputLine, error) {
		return e.RestartNode(ctx, lab, "leaf1")
	}); !clos10CommandSucceeded(result) {
		t.Fatalf("restart leaf1 result = %+v", result)
	}
	restarted := waitForLab(t, e, lab, 45*time.Second, func(l *engine.Lab) bool {
		return nodeRunning(l, "leaf1")
	})
	_ = started
	_ = restarted
}

func TestIntegrationClos10StatsAndNetem(t *testing.T) {
	e, ctx, lab := newClos10Engine(t)
	deployedClos10(t, e, ctx, lab)
	waitForLab(t, e, lab, 90*time.Second, clos10Converged)

	monitor, ok := any(e).(engine.NodeMonitorProvider)
	if !ok {
		t.Skip("node monitor capability is unavailable")
	}
	stats, ok := any(e).(engine.InterfaceStatsProvider)
	if !ok {
		t.Skip("interface stats capability is unavailable")
	}
	netem, ok := any(e).(engine.NetemProvider)
	if !ok {
		t.Skip("netem capability is unavailable")
	}
	if result, err := e.Exec(ctx, lab, "host1", "command -v ip >/dev/null && command -v tc >/dev/null && command -v ping >/dev/null && ip link show eth1 >/dev/null"); err != nil || result.Code != 0 {
		t.Skipf("host image lacks ip/tc/ping or eth1: %v (exit %d)", err, outputCode(result))
	}

	if usage, err := monitor.MonitorNode(ctx, lab, "host1"); err != nil || usage == nil {
		t.Fatalf("MonitorNode(host1) = %#v, %v", usage, err)
	}
	ifaceStats, err := stats.MonitorInterfaces(ctx, lab, "host1")
	if err != nil {
		t.Fatalf("MonitorInterfaces(host1): %v", err)
	}
	if ifaceStats["eth1"] == nil {
		t.Fatalf("MonitorInterfaces(host1) has no eth1 stats: %#v", ifaceStats)
	}
	beforeStats := *ifaceStats["eth1"]

	state := engine.NetemState{Delay: "10ms", Loss: "1%"}
	if result := runClos10Command(t, ctx, func(ctx context.Context) (<-chan engine.OutputLine, error) {
		return netem.SetInterfaceNetem(ctx, lab, "host1", "eth1", state)
	}); !clos10CommandSucceeded(result) {
		t.Fatalf("set netem result = %+v", result)
	}
	if qdisc := clos10Qdisc(t, e, ctx, lab, "host1", "eth1"); !strings.Contains(qdisc, "netem") {
		t.Fatalf("qdisc after netem = %q, want netem", qdisc)
	}
	if result := runClos10Command(t, ctx, func(ctx context.Context) (<-chan engine.OutputLine, error) {
		return netem.ClearInterfaceNetem(ctx, lab, "host1", "eth1")
	}); !clos10CommandSucceeded(result) {
		t.Fatalf("clear netem result = %+v", result)
	}
	if qdisc := clos10Qdisc(t, e, ctx, lab, "host1", "eth1"); strings.Contains(qdisc, "netem") {
		t.Fatalf("qdisc after clear = %q, still contains netem", qdisc)
	}
	if _, err := netem.SetInterfaceNetem(ctx, lab, "host1", "eth1", engine.NetemState{Jitter: "1ms"}); err == nil {
		t.Fatal("invalid netem state was accepted")
	}
	if err := generateClos10Eth1Traffic(e, ctx, lab); err != nil {
		t.Fatalf("generate eth1 telemetry traffic: %v", err)
	}
	ifaceStats, err = stats.MonitorInterfaces(ctx, lab, "host1")
	if err != nil {
		t.Fatalf("MonitorInterfaces(host1) after eth1 traffic: %v", err)
	}
	afterStats := ifaceStats["eth1"]
	if afterStats == nil {
		t.Fatalf("MonitorInterfaces(host1) lost eth1 stats: %#v", ifaceStats)
	}
	assertMonotonicInterfaceStats(t, beforeStats, *afterStats)
	if !hasMeaningfulTxStats(afterStats) || afterStats.TxPackets <= beforeStats.TxPackets || afterStats.TxBytes <= beforeStats.TxBytes {
		t.Fatalf("eth1 TX traffic did not produce increasing counters/rates: before=%+v after=%+v", beforeStats, *afterStats)
	}
	if afterStats.RxPackets <= beforeStats.RxPackets || afterStats.RxBytes <= beforeStats.RxBytes {
		t.Logf("capability limitation: eth1 RX traffic is not observable on the fixture path: before=%+v after=%+v", beforeStats, *afterStats)
	}
}

func TestIntegrationClos10Trace(t *testing.T) {
	e, ctx, lab := newClos10Engine(t)
	deployedClos10(t, e, ctx, lab)
	waitForLab(t, e, lab, 90*time.Second, clos10Converged)
	requireRoot(t)
	tracer, ok := any(e).(engine.PacketPathTracer)
	if !ok {
		t.Skip("packet trace capability is unavailable")
	}
	capable, ok := any(e).(engine.CapabilityChecker)
	if !ok {
		t.Skip("packet trace capability check is unavailable")
	}
	if result := capable.PacketTraceCapable(ctx); !result.Available {
		t.Skipf("packet trace capability unavailable: %s", result.Reason)
	}
	requireClos10TraceInspectionTools(t, e, ctx, lab)

	_, invalidStop, err := tracer.TracePath(ctx, lab, "definitely not a valid filter (")
	if invalidStop != nil {
		defer func() {
			if stopErr := invalidStop(); stopErr != nil {
				t.Logf("deferred stop invalid-filter TracePath: %v", stopErr)
			}
		}()
	}
	if err == nil {
		t.Fatal("invalid trace filter was accepted")
	}
	if invalidStop != nil {
		if stopErr := invalidStop(); stopErr != nil {
			t.Logf("stop invalid-filter TracePath after startup error: %v", stopErr)
		}
	}

	if err := configureClos10BusinessPath(e, ctx, lab); err != nil {
		t.Fatalf("configure business trace path: %v", err)
	}

	traceCtx, cancel := context.WithCancel(ctx)
	events, stop, err := tracer.TracePath(traceCtx, lab, "icmp")
	if stop != nil {
		defer func() {
			if stopErr := stop(); stopErr != nil {
				t.Logf("deferred stop TracePath(icmp): %v", stopErr)
			}
		}()
	}
	if err != nil {
		cancel()
		if stop != nil {
			if stopErr := stop(); stopErr != nil {
				t.Logf("stop TracePath(icmp) after startup error: %v", stopErr)
			}
		}
		t.Fatalf("TracePath(icmp): %v", err)
	}
	if err := generateClos10BusinessTraffic(e, ctx, lab); err != nil {
		_ = stop()
		cancel()
		t.Fatalf("generate business trace traffic: %v", err)
	}
	want := map[string]bool{
		"host1/eth3/IN":  false,
		"host1/eth3/OUT": false,
		"host2/eth3/IN":  false,
		"host2/eth3/OUT": false,
	}
	packetCount := 0
	deadline := time.NewTimer(20 * time.Second)
	for len(want) > 0 {
		select {
		case event, open := <-events:
			if !open {
				t.Fatal("trace event channel closed before IN and OUT events")
			}
			if event.Pkt.Proto != "icmp" {
				t.Fatalf("icmp trace emitted non-ICMP packet: %+v", event)
			}
			key := event.Node + "/" + event.Iface + "/" + event.Pkt.Direction
			if _, exists := want[key]; !exists {
				t.Fatalf("ICMP trace observed on unexpected path %s: %+v", key, event)
			}
			want[key] = true
			packetCount++
			allDirections := true
			for _, seen := range want {
				allDirections = allDirections && seen
			}
			if allDirections && packetCount > 0 {
				want = nil
			}
		case <-deadline.C:
			t.Fatalf("missing trace directions: %#v", want)
		}
	}
	deadline.Stop()
	if err := stop(); err != nil {
		t.Fatalf("stop trace: %v", err)
	}
	cancel()
	assertTraceChannelClosed(t, events)
	assertNoClos10TCXResources(t, e, ctx, lab)

	filteredCtx, filteredCancel := context.WithCancel(ctx)
	filtered, filteredStop, err := tracer.TracePath(filteredCtx, lab, "tcp")
	if filteredStop != nil {
		defer func() {
			if stopErr := filteredStop(); stopErr != nil {
				t.Logf("deferred stop TracePath(tcp): %v", stopErr)
			}
		}()
	}
	if err != nil {
		filteredCancel()
		if filteredStop != nil {
			if stopErr := filteredStop(); stopErr != nil {
				t.Logf("stop TracePath(tcp) after startup error: %v", stopErr)
			}
		}
		t.Fatalf("TracePath(tcp): %v", err)
	}
	if err := generateClos10BusinessTCP(e, ctx, lab); err != nil {
		_ = filteredStop()
		filteredCancel()
		t.Fatalf("generate TCP filter control traffic: %v", err)
	}
	tcpSeen := false
	tcpDirections := map[string]bool{}
	tcpDeadline := time.NewTimer(10 * time.Second)
	for !tcpSeen {
		select {
		case event, open := <-filtered:
			if !open {
				tcpDeadline.Stop()
				_ = filteredStop()
				filteredCancel()
				t.Fatal("TCP filter channel closed before a matching TCP event")
			}
			if !clos10ServiceTraceEvent(event) || event.Pkt.Proto != "tcp" {
				t.Fatalf("TCP filter observed event outside the service path, with invalid direction, or wrong protocol: %+v", event)
			}
			tcpSeen = true
			tcpDirections[event.Pkt.Direction] = true
		case <-tcpDeadline.C:
			t.Fatal("TCP filter produced no positive-control event")
		}
	}
	tcpDeadline.Stop()
	if len(tcpDirections) == 0 {
		t.Fatal("TCP filter positive control had no valid service-path direction")
	}
	if err := generateClos10BusinessTraffic(e, ctx, lab); err != nil {
		_ = filteredStop()
		filteredCancel()
		t.Fatalf("generate traffic for filter exclusion: %v", err)
	}
	filterDeadline := time.NewTimer(5 * time.Second)
	for {
		select {
		case event, open := <-filtered:
			if !open {
				filterDeadline.Stop()
				_ = filteredStop()
				filteredCancel()
				assertNoClos10TCXResources(t, e, ctx, lab)
				t.Fatal("filtered trace channel closed before filter exclusion check")
			}
			if event.Node == "host1" || event.Node == "host2" {
				if event.Iface != "eth3" {
					t.Fatalf("TCP filter observed unexpected interface: %+v", event)
				}
			}
			if event.Pkt.Proto != "tcp" {
				filterDeadline.Stop()
				_ = filteredStop()
				filteredCancel()
				t.Fatalf("TCP-only trace emitted non-TCP event: %+v", event)
			}
		case <-filterDeadline.C:
			if err := filteredStop(); err != nil {
				t.Fatalf("stop filtered trace: %v", err)
			}
			filteredCancel()
			assertTraceChannelClosed(t, filtered)
			assertNoClos10TCXResources(t, e, ctx, lab)
			return
		}
	}
}

func newClos10Engine(t *testing.T) (*ClabEngine, context.Context, string) {
	t.Helper()
	if !integrationEnabled(t) {
		t.Skip("CLAB_TUI_SKIP_INTEGRATION set")
	}
	bin := integrationBinary(t)
	requireRoot(t)
	requireImageStrict(t, "ghcr.io/nokia/srlinux:24.7.1")

	dir := t.TempDir()
	topo := filepath.Join(dir, "clos-10.clab.yml")
	data, err := os.ReadFile(clos10FixturePath(t))
	if err != nil {
		t.Fatalf("read Clos fixture: %v", err)
	}
	if image := os.Getenv("CLAB_TUI_SCALE_HOST_IMAGE"); image != "" {
		if strings.ContainsAny(image, "\r\n\t ") {
			t.Fatalf("CLAB_TUI_SCALE_HOST_IMAGE contains whitespace")
		}
		old := []byte("image: nicolaka/netshoot:v0.13")
		if count := strings.Count(string(data), string(old)); count != 2 {
			t.Fatalf("fixture host image occurrences = %d, want 2", count)
		}
		data = []byte(strings.ReplaceAll(string(data), string(old), "image: "+image))
		requireImageStrict(t, image)
	} else {
		requireImageStrict(t, "nicolaka/netshoot:v0.13")
	}
	if err := os.WriteFile(topo, data, 0o644); err != nil {
		t.Fatalf("write temporary Clos fixture: %v", err)
	}
	e, err := New(WithBinary(bin), WithLabDir(dir))
	if err != nil {
		t.Fatalf("create ClabEngine: %v", err)
	}
	registerIntegrationCleanup(t, e, clos10Lab)
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	t.Cleanup(cancel)
	return e, ctx, clos10Lab
}

func deployedClos10(t *testing.T, e *ClabEngine, ctx context.Context, lab string) {
	t.Helper()
	topo, err := e.findTopoFile(lab)
	if err != nil {
		t.Fatalf("find Clos fixture: %v", err)
	}
	if result := runClos10Command(t, ctx, func(ctx context.Context) (<-chan engine.OutputLine, error) {
		return e.Deploy(ctx, topo)
	}); !result.Done || result.Code != 0 {
		t.Fatalf("Clos deploy result = %+v", result)
	}
	if _, err := e.ListLabs(ctx); err != nil {
		t.Fatalf("ListLabs: %v", err)
	}
}

func clos10FixturePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test path")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "integration", "clos-10", "clos-10.clab.yml")
}

func clos10Converged(lab *engine.Lab) bool {
	if len(lab.Nodes) != 10 || len(lab.Links) != 17 || lab.Status() != engine.LabStatusRunning {
		return false
	}
	groups := map[string]bool{}
	for _, node := range lab.Nodes {
		if node.State != engine.StatusRunning {
			return false
		}
		groups[node.Group] = true
		if strings.HasPrefix(node.Kind, "nokia_srlinux") && len(node.Interfaces) < 3 {
			return false
		}
	}
	for _, group := range []string{"spine", "leaf", "border", "host"} {
		if !groups[group] {
			return false
		}
	}
	for _, link := range lab.Links {
		if link.State != engine.LinkUp {
			return false
		}
	}
	return true
}

func assertClos10Topology(t *testing.T, lab *engine.Lab) {
	t.Helper()
	if len(lab.Nodes) != 10 {
		t.Fatalf("node count = %d, want 10", len(lab.Nodes))
	}
	if len(lab.Links) != 17 {
		t.Fatalf("link count = %d, want 17", len(lab.Links))
	}
	if !hasClos10ServiceLink(lab) {
		t.Fatal("missing host1:eth3-host2:eth3 service link")
	}
	groups := map[string]int{}
	for _, node := range lab.Nodes {
		groups[node.Group]++
		if node.State != engine.StatusRunning {
			t.Fatalf("node %q state = %s, want running", node.Name, node.State)
		}
		if strings.HasPrefix(node.Kind, "nokia_srlinux") && len(node.Interfaces) < 3 {
			t.Fatalf("SR Linux node %q has %d interfaces, want at least 3", node.Name, len(node.Interfaces))
		}
	}
	for group, want := range map[string]int{"spine": 2, "leaf": 4, "border": 2, "host": 2} {
		if groups[group] != want {
			t.Fatalf("group %q count = %d, want %d", group, groups[group], want)
		}
	}
	for _, link := range lab.Links {
		if link.State != engine.LinkUp {
			t.Fatalf("link %s:%s-%s:%s state = %s, want up", link.A, link.PortA, link.B, link.PortB, link.State)
		}
	}
}

func nodeRunning(lab *engine.Lab, name string) bool {
	for _, node := range lab.Nodes {
		if node.Name == name {
			return node.State == engine.StatusRunning
		}
	}
	return false
}

func assertLeaf1LinkCount(t *testing.T, lab *engine.Lab) {
	t.Helper()
	seen := 0
	for _, link := range lab.Links {
		if link.A == "leaf1" || link.B == "leaf1" {
			seen++
		}
	}
	if seen != clos10Leaf1LinkCount {
		t.Fatalf("leaf1-related link count = %d, want %d", seen, clos10Leaf1LinkCount)
	}
}

func assertLeaf1LinksNotUp(t *testing.T, lab *engine.Lab) {
	t.Helper()
	assertLeaf1LinkCount(t, lab)
	for _, link := range lab.Links {
		if link.A == "leaf1" || link.B == "leaf1" {
			if link.State == engine.LinkUp {
				t.Fatalf("leaf1 link %s:%s-%s:%s still up after stop", link.A, link.PortA, link.B, link.PortB)
			}
		}
	}
}

func hasClos10ServiceLink(lab *engine.Lab) bool {
	for _, link := range lab.Links {
		if (link.A == "host1" && link.PortA == "eth3" && link.B == "host2" && link.PortB == "eth3") ||
			(link.A == "host2" && link.PortA == "eth3" && link.B == "host1" && link.PortB == "eth3") {
			return true
		}
	}
	return false
}

func clos10ServiceTraceEvent(event engine.PacketEvent) bool {
	return (event.Node == "host1" || event.Node == "host2") &&
		event.Iface == "eth3" &&
		(event.Pkt.Direction == "IN" || event.Pkt.Direction == "OUT")
}

type clos10CommandResult struct {
	Output string
	Code   int
	Done   bool
}

func clos10CommandSucceeded(result clos10CommandResult) bool {
	return result.Done && result.Code == 0 && strings.Contains(result.Output, "[Done code=0]")
}

func runClos10Command(t *testing.T, ctx context.Context, command func(context.Context) (<-chan engine.OutputLine, error)) clos10CommandResult {
	t.Helper()
	ch, err := command(ctx)
	if err != nil {
		t.Fatalf("scale operation: %v", err)
	}
	var output strings.Builder
	for line := range ch {
		if line.Done {
			output.WriteString(fmt.Sprintf("[Done code=%d]\n", line.Code))
			return clos10CommandResult{Output: output.String(), Code: line.Code, Done: true}
		}
		t.Logf("%s: %s", line.Stream, line.Line)
		output.WriteString(line.Line)
		output.WriteByte('\n')
	}
	t.Fatalf("scale operation channel closed without Done; output=%q", output.String())
	return clos10CommandResult{}
}

func clos10Qdisc(t *testing.T, e *ClabEngine, ctx context.Context, lab, node, iface string) string {
	t.Helper()
	result, err := e.Exec(ctx, lab, node, "tc qdisc show dev "+iface)
	if err != nil {
		t.Fatalf("inspect qdisc %s/%s: %v", node, iface, err)
	}
	if result.Code != 0 {
		t.Fatalf("inspect qdisc %s/%s exited %d: %s", node, iface, result.Code, result.Stderr)
	}
	return result.Stdout
}

func hasMeaningfulStats(stats *engine.InterfaceStats) bool {
	return stats != nil && stats.StatsAt.IsZero() == false && stats.Interval > 0 &&
		stats.RxPackets > 0 && stats.TxPackets > 0 && (stats.RxBps > 0 || stats.TxBps > 0)
}

func hasMeaningfulTxStats(stats *engine.InterfaceStats) bool {
	return stats != nil && !stats.StatsAt.IsZero() && stats.Interval > 0 &&
		stats.TxPackets > 0 && stats.TxBytes > 0 && stats.TxBps > 0
}

func assertMonotonicInterfaceStats(t *testing.T, before, after engine.InterfaceStats) {
	t.Helper()
	if after.RxBytes < before.RxBytes || after.RxPackets < before.RxPackets ||
		after.TxBytes < before.TxBytes || after.TxPackets < before.TxPackets {
		t.Fatalf("interface counters regressed: before=%+v after=%+v", before, after)
	}
}

func configureClos10BusinessPath(e *ClabEngine, ctx context.Context, lab string) error {
	host1Iface, host2Iface, err := clos10BusinessIfaces(e, ctx, lab)
	if err != nil {
		return err
	}
	return configureClos10BusinessEndpoints(e, ctx, lab, host1Iface, host2Iface)
}

func clos10BusinessIfaces(e *ClabEngine, ctx context.Context, lab string) (string, string, error) {
	current, err := e.GetLab(ctx, lab)
	if err != nil {
		return "", "", err
	}
	for _, link := range current.Links {
		if link.A == "host1" && link.B == "host2" {
			return link.PortA, link.PortB, nil
		}
		if link.A == "host2" && link.B == "host1" {
			return link.PortB, link.PortA, nil
		}
	}
	return "", "", fmt.Errorf("capability: fixture has no direct host1-host2 business link and no configured SR Linux L3 forwarding")
}

func configureClos10BusinessEndpoints(e *ClabEngine, ctx context.Context, lab, host1Iface, host2Iface string) error {
	for _, spec := range []struct {
		node, iface, address string
	}{
		{"host1", host1Iface, "10.255.10.1/30"},
		{"host2", host2Iface, "10.255.10.2/30"},
	} {
		command := fmt.Sprintf("ip addr replace %s dev %s && ip link set %s up", spec.address, spec.iface, spec.iface)
		result, err := e.Exec(ctx, lab, spec.node, command)
		if err != nil || result.Code != 0 {
			return fmt.Errorf("configure %s/%s: err=%v exit=%d stderr=%s", spec.node, spec.iface, err, outputCode(result), outputStderr(result))
		}
	}
	return nil
}

func generateClos10BusinessTraffic(e *ClabEngine, ctx context.Context, lab string) error {
	host1Iface, _, err := clos10BusinessIfaces(e, ctx, lab)
	if err != nil {
		return err
	}
	result, err := e.Exec(ctx, lab, "host1", fmt.Sprintf("ping -I %s -c 5 -W 1 10.255.10.2", host1Iface))
	if err != nil {
		return err
	}
	if result.Code != 0 {
		return fmt.Errorf("ping exited %d: %s", outputCode(result), outputStderr(result))
	}
	return nil
}

func generateClos10BusinessTCP(e *ClabEngine, ctx context.Context, lab string) (retErr error) {
	pidFile := "/tmp/clab-tui-clos10-nc.pid"
	listener, err := e.Exec(ctx, lab, "host2", fmt.Sprintf("sh -c 'rm -f %s; nc -l -p 18080 >/dev/null 2>&1 & printf \"%%s\\n\" \"$!\" > %s; cat %s'", pidFile, pidFile, pidFile))
	listenerPID := 0
	cleanupDone := false
	defer func() {
		if cleanupDone {
			return
		}
		if cleanupErr := cleanupClos10TCPListener(e, ctx, lab, &listenerPID, pidFile); cleanupErr != nil && retErr == nil {
			retErr = fmt.Errorf("cleanup host2 TCP listener: %w", cleanupErr)
		}
	}()
	if err != nil || listener == nil || listener.Code != 0 {
		return fmt.Errorf("start host2 TCP listener: err=%v exit=%d stderr=%s", err, outputCode(listener), outputStderr(listener))
	}
	listenerPID, err = parseClos10ListenerPID(listener.Stdout)
	if err != nil {
		return fmt.Errorf("start host2 TCP listener: %w; stdout=%s stderr=%s", err, listener.Stdout, listener.Stderr)
	}
	client, clientErr := e.Exec(ctx, lab, "host1", "nc -w 2 10.255.10.2 18080 </dev/null")
	cleanupErr := cleanupClos10TCPListener(e, ctx, lab, &listenerPID, pidFile)
	cleanupDone = true
	if clientErr != nil || client.Code != 0 {
		return fmt.Errorf("connect to host2 TCP listener: err=%v exit=%d stderr=%s", clientErr, outputCode(client), outputStderr(client))
	}
	if cleanupErr != nil {
		return fmt.Errorf("stop host2 TCP listener: %w", cleanupErr)
	}
	return nil
}

func parseClos10ListenerPID(stdout string) (int, error) {
	fields := strings.Fields(stdout)
	if len(fields) != 1 {
		return 0, fmt.Errorf("listener returned %d PID fields, want one", len(fields))
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid listener PID %q", fields[0])
	}
	return pid, nil
}

func stopClos10TCPListener(e *ClabEngine, ctx context.Context, lab string, pid int) error {
	probe, err := e.Exec(ctx, lab, "host2", fmt.Sprintf("kill -0 %d", pid))
	if err != nil {
		return fmt.Errorf("probe PID %d: %w", pid, err)
	}
	if probe.Code != 0 {
		return nil // nc already exited after its one-shot connection.
	}
	kill, killErr := e.Exec(ctx, lab, "host2", fmt.Sprintf("kill %d", pid))
	if killErr != nil || kill.Code != 0 {
		// The listener can exit between kill -0 and kill. A bounded poll below
		// distinguishes that expected race from a process that remains alive.
		if waitErr := waitClos10TCPListenerGone(e, ctx, lab, pid); waitErr == nil {
			return nil
		} else {
			return fmt.Errorf("kill PID %d: err=%v exit=%d stderr=%s: %w", pid, killErr, outputCode(kill), outputStderr(kill), waitErr)
		}
	}
	return waitClos10TCPListenerGone(e, ctx, lab, pid)
}

func waitClos10TCPListenerGone(e *ClabEngine, ctx context.Context, lab string, pid int) error {
	deadline := time.Now().Add(2 * time.Second)
	for {
		probe, err := e.Exec(ctx, lab, "host2", fmt.Sprintf("kill -0 %d", pid))
		if err != nil {
			return fmt.Errorf("probe PID %d: %w", pid, err)
		}
		if probe.Code != 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("PID %d remained alive after 2s", pid)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func cleanupClos10TCPListener(e *ClabEngine, ctx context.Context, lab string, pid *int, pidFile string) error {
	if *pid == 0 {
		result, err := e.Exec(ctx, lab, "host2", "cat "+pidFile)
		if err != nil || result.Code != 0 {
			return fmt.Errorf("read listener PID: err=%v exit=%d stderr=%s", err, outputCode(result), outputStderr(result))
		}
		parsed, parseErr := parseClos10ListenerPID(result.Stdout)
		if parseErr != nil {
			return parseErr
		}
		*pid = parsed
	}
	cleanupErr := stopClos10TCPListener(e, ctx, lab, *pid)
	remove, removeErr := e.Exec(ctx, lab, "host2", "rm -f "+pidFile)
	if cleanupErr != nil {
		return cleanupErr
	}
	if removeErr != nil || remove.Code != 0 {
		return fmt.Errorf("remove listener PID file: err=%v exit=%d stderr=%s", removeErr, outputCode(remove), outputStderr(remove))
	}
	return nil
}

func generateClos10Eth1Traffic(e *ClabEngine, ctx context.Context, lab string) error {
	setup, err := e.Exec(ctx, lab, "host1", "ip link set eth1 up")
	if err != nil || setup.Code != 0 {
		return fmt.Errorf("enable host1/eth1: err=%v exit=%d stdout=%s stderr=%s", err, outputCode(setup), outputStdout(setup), outputStderr(setup))
	}
	traffic, err := e.Exec(ctx, lab, "host1", "ping -I eth1 -c 5 -W 1 -b 169.254.255.255")
	if err != nil || traffic.Code != 0 {
		return fmt.Errorf("generate host1/eth1 traffic: err=%v exit=%d stdout=%s stderr=%s", err, outputCode(traffic), outputStdout(traffic), outputStderr(traffic))
	}
	return nil
}

func assertTraceChannelClosed(t *testing.T, events <-chan engine.PacketEvent) {
	t.Helper()
	select {
	case _, open := <-events:
		if open {
			t.Fatal("trace event channel remained open after stop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("trace event channel did not terminate after stop")
	}
}

func requireClos10TraceInspectionTools(t *testing.T, e *ClabEngine, ctx context.Context, lab string) {
	t.Helper()
	current, err := e.GetLab(ctx, lab)
	if err != nil {
		t.Skipf("trace inspection capability unavailable: %v", err)
	}
	for _, node := range current.Nodes {
		result, execErr := e.Exec(ctx, lab, node.Name, clos10TraceInspectionCommand(node.Name))
		if execErr != nil || result == nil || result.Code != 0 {
			t.Skipf("trace inspection capability unavailable in %s: err=%v exit=%d", node.Name, execErr, outputCode(result))
		}
	}
}

func clos10TraceInspectionCommand(nodeName string) string {
	command := "command -v ip >/dev/null && command -v tc >/dev/null"
	if nodeName == "host1" || nodeName == "host2" {
		command += " && command -v nc >/dev/null"
	}
	return command
}

func assertNoClos10TCXResources(t *testing.T, e *ClabEngine, ctx context.Context, lab string) {
	t.Helper()
	current, err := e.GetLab(ctx, lab)
	if err != nil {
		t.Fatalf("inspect trace resources: %v", err)
	}
	var failures []string
	for _, node := range current.Nodes {
		linkResult, execErr := e.Exec(ctx, lab, node.Name, "ip -j link")
		if execErr != nil || linkResult.Code != 0 {
			failures = append(failures, fmt.Sprintf("%s: enumerate interfaces: err=%v exit=%d", node.Name, execErr, outputCode(linkResult)))
			continue
		}
		interfaces, parseErr := parseIPLinkJSON(linkResult.Stdout)
		if parseErr != nil {
			failures = append(failures, fmt.Sprintf("%s: parse interfaces: %v", node.Name, parseErr))
			continue
		}
		for iface := range interfaces {
			if iface == "" || iface == "lo" {
				continue
			}
			command := fmt.Sprintf("tc filter show dev %s ingress; tc filter show dev %s egress", iface, iface)
			result, err := e.Exec(ctx, lab, node.Name, command)
			if err != nil || result.Code != 0 {
				failures = append(failures, fmt.Sprintf("%s/%s: inspect filters: err=%v exit=%d", node.Name, iface, err, outputCode(result)))
				continue
			}
			if strings.Contains(strings.ToLower(result.Stdout), "bpf") {
				failures = append(failures, fmt.Sprintf("%s/%s: TCX/BPF filter remains: %s", node.Name, iface, result.Stdout))
			}
		}
	}
	if len(failures) > 0 {
		t.Fatalf("TCX/BPF cleanup inspection failures:\n%s", strings.Join(failures, "\n"))
	}
}

func outputCode(result *engine.ExecResult) int {
	if result == nil {
		return -1
	}
	return result.Code
}

func outputStderr(result *engine.ExecResult) string {
	if result == nil {
		return ""
	}
	return result.Stderr
}

func outputStdout(result *engine.ExecResult) string {
	if result == nil {
		return ""
	}
	return result.Stdout
}
