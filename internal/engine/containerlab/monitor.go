package containerlab

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// MonitorNode reports a node's CPU/mem/network usage via `docker stats
// --no-stream`. NetRx/NetTx are cumulative bytes since container start; rate
// is computed by the caller from successive samples.
func (e *ClabEngine) MonitorNode(ctx context.Context, labName, nodeName string) (*engine.NodeMonitor, error) {
	container := e.containerForNode(ctx, labName, nodeName)
	if container == "" {
		return nil, engine.ErrNodeNotFound
	}
	ch, err := spawnWithLines(ctx, "docker", "stats", "--no-stream",
		"--format", "{{.CPUPerc}}|{{.MemUsage}}|{{.NetIO}}|{{.PIDs}}", container)
	if err != nil {
		return nil, err
	}
	var line string
	code := 0
	done := false
	for l := range ch {
		if l.Done {
			done = true
			code = l.Code
			break
		}
		line = l.Line
	}
	if !done {
		return nil, fmt.Errorf("docker stats stream closed before completion")
	}
	if code != 0 {
		return nil, fmt.Errorf("docker stats failed: %s", line)
	}
	return parseDockerStats(line), nil
}

// parseDockerStats parses `docker stats --no-stream` one-line output in the
// format "CPU%|MemUsed / MemLimit|Rx / Tx|PIDs".
func parseDockerStats(line string) *engine.NodeMonitor {
	m := &engine.NodeMonitor{}
	parts := strings.Split(line, "|")
	if len(parts) >= 4 {
		m.CPUPercent, _ = strconv.ParseFloat(strings.TrimSuffix(parts[0], "%"), 64)
		memUsed, memLimit := splitPair(parts[1])
		m.MemUsed = parseSize(memUsed)
		m.MemLimit = parseSize(memLimit)
		rx, tx := splitPair(parts[2])
		m.NetRx = parseSize(rx)
		m.NetTx = parseSize(tx)
		m.Pids, _ = strconv.ParseUint(strings.TrimSpace(parts[3]), 10, 64)
	}
	return m
}

func splitPair(s string) (a, b string) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(s), ""
}

// parseSize parses docker sizes like "10MiB", "1.2kB", "512B" into bytes.
func parseSize(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	mult := uint64(1)
	switch {
	case strings.HasSuffix(s, "KiB"):
		mult = 1 << 10
		s = strings.TrimSuffix(s, "KiB")
	case strings.HasSuffix(s, "MiB"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "MiB")
	case strings.HasSuffix(s, "GiB"):
		mult = 1 << 30
		s = strings.TrimSuffix(s, "GiB")
	case strings.HasSuffix(s, "TiB"):
		mult = 1 << 40
		s = strings.TrimSuffix(s, "TiB")
	case strings.HasSuffix(s, "kB"):
		mult = 1000
		s = strings.TrimSuffix(s, "kB")
	case strings.HasSuffix(s, "MB"):
		mult = 1000 * 1000
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "GB"):
		mult = 1000 * 1000 * 1000
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return uint64(math.Round(f * float64(mult)))
}
