package containerlab

import (
	"context"
	"fmt"
	"regexp"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

var netemValuePattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?[A-Za-z%]+$`)
var netemIfacePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)

func buildNetemArgs(iface string, state engine.NetemState) ([]string, error) {
	if !netemIfacePattern.MatchString(iface) {
		return nil, fmt.Errorf("invalid interface name %q", iface)
	}
	if state.Delay == "" && state.Loss == "" && state.Rate == "" && state.Corruption == "" {
		return nil, fmt.Errorf("at least one netem setting is required")
	}
	if state.Jitter != "" && state.Delay == "" {
		return nil, fmt.Errorf("jitter requires delay")
	}
	for name, value := range map[string]string{
		"delay": state.Delay, "jitter": state.Jitter, "loss": state.Loss, "rate": state.Rate, "corruption": state.Corruption,
	} {
		if value != "" && !netemValuePattern.MatchString(value) {
			return nil, fmt.Errorf("invalid %s value %q", name, value)
		}
	}
	args := []string{"tc", "qdisc", "replace", "dev", iface, "root", "netem"}
	if state.Delay != "" {
		args = append(args, "delay", state.Delay)
		if state.Jitter != "" {
			args = append(args, state.Jitter)
		}
	}
	if state.Loss != "" {
		args = append(args, "loss", state.Loss)
	}
	if state.Rate != "" {
		args = append(args, "rate", state.Rate)
	}
	if state.Corruption != "" {
		args = append(args, "corrupt", state.Corruption)
	}
	return args, nil
}

func (e *ClabEngine) SetInterfaceNetem(ctx context.Context, labName, nodeName, iface string, state engine.NetemState) (<-chan engine.OutputLine, error) {
	container := e.containerForNode(ctx, labName, nodeName)
	if container == "" {
		return nil, engine.ErrNodeNotFound
	}
	args, err := buildNetemArgs(iface, state)
	if err != nil {
		return nil, err
	}
	return spawnWithLines(ctx, "docker", append([]string{"exec", container}, args...)...)
}

func (e *ClabEngine) ClearInterfaceNetem(ctx context.Context, labName, nodeName, iface string) (<-chan engine.OutputLine, error) {
	container := e.containerForNode(ctx, labName, nodeName)
	if container == "" {
		return nil, engine.ErrNodeNotFound
	}
	if !netemIfacePattern.MatchString(iface) {
		return nil, fmt.Errorf("invalid interface name %q", iface)
	}
	return spawnWithLines(ctx, "docker", "exec", container, "tc", "qdisc", "del", "dev", iface, "root")
}
