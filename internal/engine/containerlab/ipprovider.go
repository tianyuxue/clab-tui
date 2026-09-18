package containerlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// InterfaceIPs reports per-interface IPv4 addresses by running `ip -j addr`
// inside the container. If that fails (older iproute2), falls back to plain
// `ip addr` text parsing. Unsupported kinds return an empty map (callers
// degrade gracefully).
func (e *ClabEngine) InterfaceIPs(ctx context.Context, labName, nodeName string) (map[string]string, error) {
	container := e.containerForNode(ctx, labName, nodeName)
	if container == "" {
		return nil, engine.ErrNodeNotFound
	}
	if out, err := e.nodeExecRaw(ctx, container, "ip", "-j", "addr"); err == nil {
		if ips, ok := parseIPJSON(out); ok {
			return ips, nil
		}
	}
	if out, err := e.nodeExecRaw(ctx, container, "ip", "addr"); err == nil {
		return parseIPText(out), nil
	}
	return map[string]string{}, nil // unsupported kind / command missing
}

// nodeExecRaw runs a command inside a container via `docker exec` and returns
// combined stdout/stderr. Mirrors nodeExecLines but captures output as text.
func (e *ClabEngine) nodeExecRaw(ctx context.Context, container string, args ...string) (string, error) {
	cmdArgs := append([]string{"exec", container}, args...)
	ch, err := spawnWithLines(ctx, "docker", cmdArgs...)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	code := 0
	done := false
	for line := range ch {
		if line.Done {
			done = true
			code = line.Code
			break
		}
		sb.WriteString(line.Line + "\n")
	}
	if !done {
		return sb.String(), fmt.Errorf("docker exec %v stream closed before completion", cmdArgs)
	}
	if code != 0 {
		return sb.String(), fmt.Errorf("docker exec %v failed with code %d", cmdArgs, code)
	}
	return sb.String(), nil
}

// parseIPJSON parses `ip -j addr` JSON into interface→IPv4 map.
type ipAddrJSON struct {
	Ifname   string `json:"ifname"`
	AddrInfo []struct {
		Family string `json:"family"`
		Local  string `json:"local"`
	} `json:"addr_info"`
}

func parseIPJSON(out string) (map[string]string, bool) {
	var addrs []ipAddrJSON
	if err := json.Unmarshal([]byte(out), &addrs); err != nil {
		return nil, false
	}
	ips := map[string]string{}
	for _, a := range addrs {
		for _, ai := range a.AddrInfo {
			if ai.Family == "inet" {
				ips[ifaceName(a.Ifname)] = strings.Split(ai.Local, "/")[0]
			}
		}
	}
	return ips, true
}

// ifaceName strips the veth peer suffix ("@if12") from an interface name so
// keys match the logical interface name.
func ifaceName(name string) string {
	if i := strings.Index(name, "@"); i > 0 {
		return name[:i]
	}
	return name
}

// parseIPText parses `ip addr` plain-text output into interface→IPv4 map.
func parseIPText(out string) map[string]string {
	ips := map[string]string{}
	var cur string
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if idx := strings.Index(line, ": "); idx > 0 {
			rest := line[idx+2:]
			name := rest
			if sp := strings.Index(rest, ":"); sp > 0 {
				name = rest[:sp]
			}
			cur = ifaceName(name)
		}
		if strings.HasPrefix(trimmed, "inet ") && cur != "" {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				ips[cur] = strings.Split(fields[1], "/")[0]
			}
		}
	}
	return ips
}
