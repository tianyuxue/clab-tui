package containerlab

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func (e *ClabEngine) Deploy(ctx context.Context, labPath string, opts ...engine.DeployOption) (<-chan engine.OutputLine, error) {
	cfg := &engine.DeployOptions{}
	for _, o := range opts {
		o(cfg)
	}
	args := []string{"deploy", "--topo", labPath}
	if cfg.Cleanup {
		args = append(args, "--cleanup")
	}
	if cfg.Timeout <= 0 {
		return spawnWithLines(ctx, e.bin, args...)
	}
	// A positive Timeout bounds the whole deploy run. The timeout context is
	// wired into the child command and released once the output stream drains
	// (not when Deploy returns, which would kill the process immediately).
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Timeout)*time.Second)
	ch, err := spawnWithLinesOnDone(timeoutCtx, cancel, e.bin, args...)
	if err != nil || ch == nil {
		cancel()
		if err == nil {
			err = fmt.Errorf("deploy output stream was nil")
		}
		return nil, err
	}
	return ch, err
}

func (e *ClabEngine) Destroy(ctx context.Context, labName string, opts ...engine.DestroyOption) (<-chan engine.OutputLine, error) {
	cfg := &engine.DestroyOptions{}
	for _, o := range opts {
		o(cfg)
	}
	labPath, err := e.resolveLabPath(ctx, labName)
	if err != nil {
		return nil, err
	}
	args := []string{"destroy", "--topo", labPath}
	if cfg.Cleanup {
		args = append(args, "--cleanup")
	}
	return spawnWithLines(ctx, e.bin, args...)
}

func (e *ClabEngine) Redeploy(ctx context.Context, labName string, opts ...engine.RedeployOption) (<-chan engine.OutputLine, error) {
	cfg := &engine.RedeployOptions{}
	for _, o := range opts {
		o(cfg)
	}
	labPath, err := e.resolveLabPath(ctx, labName)
	if err != nil {
		return nil, err
	}
	args := []string{"redeploy", "--topo", labPath}
	if cfg.Cleanup {
		args = append(args, "--cleanup")
	}
	return spawnWithLines(ctx, e.bin, args...)
}

func (e *ClabEngine) Save(ctx context.Context, labName, nodeName string) (<-chan engine.OutputLine, error) {
	labPath, err := e.resolveLabPath(ctx, labName)
	if err != nil {
		return nil, err
	}
	return spawnWithLines(ctx, e.bin, "save", "--topo", labPath, "--node", nodeName)
}

func (e *ClabEngine) resolveLabPath(ctx context.Context, labName string) (string, error) {
	path, err := e.findTopoFile(labName)
	if err != nil {
		return "", engine.ErrLabNotFound
	}
	return path, nil
}

func (e *ClabEngine) StartNode(ctx context.Context, labName, nodeName string) (<-chan engine.OutputLine, error) {
	return e.nodeExecLines(ctx, labName, nodeName, "start")
}

func (e *ClabEngine) StopNode(ctx context.Context, labName, nodeName string) (<-chan engine.OutputLine, error) {
	return e.nodeExecLines(ctx, labName, nodeName, "stop")
}

func (e *ClabEngine) RestartNode(ctx context.Context, labName, nodeName string) (<-chan engine.OutputLine, error) {
	return e.nodeExecLines(ctx, labName, nodeName, "restart")
}

func (e *ClabEngine) PauseNode(ctx context.Context, labName, nodeName string) (<-chan engine.OutputLine, error) {
	// containerlab has no pause subcommand; use the container runtime.
	return e.nodeExecLines(ctx, labName, nodeName, "pause")
}

func (e *ClabEngine) UnpauseNode(ctx context.Context, labName, nodeName string) (<-chan engine.OutputLine, error) {
	return e.nodeExecLines(ctx, labName, nodeName, "unpause")
}

func (e *ClabEngine) nodeExecLines(ctx context.Context, labName, nodeName, subcmd string) (<-chan engine.OutputLine, error) {
	container := e.containerForNode(ctx, labName, nodeName)
	if container == "" {
		return nil, engine.ErrNodeNotFound
	}
	return spawnWithLines(ctx, "docker", subcmd, container)
}

func (e *ClabEngine) containerForNode(ctx context.Context, labName, nodeName string) string {
	_ = e.ensureStarted(ctx)
	snap := e.store.Snapshot()
	lab := snap.Labs[labName]
	if lab == nil {
		return ""
	}
	for i := range lab.Nodes {
		if lab.Nodes[i].Name == nodeName {
			return lab.Nodes[i].Container
		}
	}
	return ""
}

func (e *ClabEngine) Exec(ctx context.Context, labName, nodeName, cmd string) (*engine.ExecResult, error) {
	labPath, err := e.resolveLabPath(ctx, labName)
	if err != nil {
		return nil, err
	}
	ch, err := spawnWithLines(ctx, e.bin, "exec", "--topo", labPath, "--node", nodeName, "--cmd", cmd)
	if err != nil {
		return nil, err
	}
	var stdout, stderr strings.Builder
	code := 0
	done := false
	for line := range ch {
		if line.Done {
			done = true
			code = line.Code
			if line.Line != "" && code != 0 {
				stderr.WriteString(line.Line + "\n")
			}
			continue
		}
		if line.Stream == "stderr" {
			stderr.WriteString(line.Line + "\n")
		} else {
			stdout.WriteString(line.Line + "\n")
		}
	}
	if !done {
		return &engine.ExecResult{Stdout: stdout.String(), Stderr: stderr.String(), Code: -1}, fmt.Errorf("command stream closed before completion")
	}
	return &engine.ExecResult{Stdout: stdout.String(), Stderr: stderr.String(), Code: code}, nil
}

// ---- optional capabilities ----

// StreamNodeLogs streams node logs (docker logs -f by container name).
func (e *ClabEngine) StreamNodeLogs(ctx context.Context, labName, nodeName string) (<-chan engine.OutputLine, error) {
	container := e.containerForNode(ctx, labName, nodeName)
	if container == "" {
		return nil, engine.ErrNodeNotFound
	}
	return spawnWithLines(ctx, "docker", "logs", "-f", container)
}

// SessionCommand returns an interactive command for a long-lived session into
// a node. Shell mode uses `docker exec -it <container> bash`; SSH/telnet
// modes are not supported in this version.
func (e *ClabEngine) SessionCommand(ctx context.Context, labName, nodeName string, mode engine.SessionMode) (*engine.SessionCmd, error) {
	switch mode {
	case engine.SessionModeShell:
		container := e.containerForNode(ctx, labName, nodeName)
		if container == "" {
			return nil, engine.ErrNodeNotFound
		}
		return &engine.SessionCmd{
			Command: []string{"docker", "exec", "-it", container, "bash"},
			Title:   container,
		}, nil
	default:
		return nil, engine.ErrNotSupported
	}
}
