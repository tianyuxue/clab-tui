package containerlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

type interfaceStatsCounters struct {
	RxBytes   uint64
	RxPackets uint64
	TxBytes   uint64
	TxPackets uint64
	At        time.Time
}

type ipLinkStatsJSON struct {
	Ifname  string `json:"ifname"`
	Stats64 struct {
		Rx struct {
			Bytes   uint64 `json:"bytes"`
			Packets uint64 `json:"packets"`
		} `json:"rx"`
		Tx struct {
			Bytes   uint64 `json:"bytes"`
			Packets uint64 `json:"packets"`
		} `json:"tx"`
	} `json:"stats64"`
}

func parseIPLinkStatsJSON(out string) (map[string]interfaceStatsCounters, error) {
	var links []ipLinkStatsJSON
	if err := json.Unmarshal([]byte(out), &links); err != nil {
		return nil, err
	}
	stats := make(map[string]interfaceStatsCounters, len(links))
	for _, link := range links {
		if link.Ifname == "" {
			continue
		}
		stats[ifaceName(link.Ifname)] = interfaceStatsCounters{
			RxBytes: link.Stats64.Rx.Bytes, RxPackets: link.Stats64.Rx.Packets,
			TxBytes: link.Stats64.Tx.Bytes, TxPackets: link.Stats64.Tx.Packets,
		}
	}
	return stats, nil
}

func parseProcNetDevStats(out string) (map[string]interfaceStatsCounters, error) {
	stats := map[string]interfaceStatsCounters{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		if name == "" || len(fields) < 10 {
			continue
		}
		nums := make([]uint64, 10)
		valid := true
		for i := range nums {
			n, err := strconv.ParseUint(fields[i], 10, 64)
			if err != nil {
				valid = false
				break
			}
			nums[i] = n
		}
		if valid {
			stats[ifaceName(name)] = interfaceStatsCounters{
				RxBytes: nums[0], RxPackets: nums[1], TxBytes: nums[8], TxPackets: nums[9],
			}
		}
	}
	if len(stats) == 0 {
		return nil, fmt.Errorf("no interface counters found")
	}
	return stats, nil
}

func interfaceStatsRate(previous, current interfaceStatsCounters) *engine.InterfaceStats {
	interval := current.At.Sub(previous.At)
	if interval <= 0 || current.At.IsZero() || previous.At.IsZero() {
		return &engine.InterfaceStats{Interval: interval, StatsAt: current.At}
	}
	seconds := interval.Seconds()
	return &engine.InterfaceStats{
		RxBps:   float64(current.RxBytes-previous.RxBytes) / seconds,
		TxBps:   float64(current.TxBytes-previous.TxBytes) / seconds,
		RxBytes: current.RxBytes, RxPackets: current.RxPackets,
		TxBytes: current.TxBytes, TxPackets: current.TxPackets,
		Interval: interval, StatsAt: current.At,
	}
}

// MonitorInterfaces samples only the selected node. JSON ip output is
// preferred; procfs provides a lightweight fallback for minimal images.
func (e *ClabEngine) MonitorInterfaces(ctx context.Context, labName, nodeName string) (map[string]*engine.InterfaceStats, error) {
	container := e.containerForNode(ctx, labName, nodeName)
	if container == "" {
		return nil, engine.ErrNodeNotFound
	}
	raw, err := e.nodeExecRaw(ctx, container, "ip", "-s", "-j", "link")
	var counters map[string]interfaceStatsCounters
	if err == nil {
		counters, err = parseIPLinkStatsJSON(raw)
	}
	if err != nil {
		raw, procErr := e.nodeExecRaw(ctx, container, "cat", "/proc/net/dev")
		if procErr != nil {
			return nil, fmt.Errorf("interface stats unavailable: %w", err)
		}
		counters, err = parseProcNetDevStats(raw)
		if err != nil {
			return nil, fmt.Errorf("interface stats unavailable: %w", err)
		}
	}

	now := time.Now()
	result := make(map[string]*engine.InterfaceStats, len(counters))
	for name, current := range counters {
		current.At = now
		key := labName + "\x00" + nodeName + "\x00" + name
		e.statsMu.Lock()
		previous, ok := e.stats[key]
		e.stats[key] = current
		e.statsMu.Unlock()
		if !ok {
			result[name] = &engine.InterfaceStats{
				RxBytes: current.RxBytes, RxPackets: current.RxPackets,
				TxBytes: current.TxBytes, TxPackets: current.TxPackets,
				StatsAt: now,
			}
			continue
		}
		result[name] = interfaceStatsRate(previous, current)
	}
	return result, nil
}
