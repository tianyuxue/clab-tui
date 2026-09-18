package containerlab

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestIntegrationHelperCollectOutput(t *testing.T) {
	ch := make(chan engine.OutputLine, 3)
	ch <- engine.OutputLine{Stream: "stdout", Line: "hello"}
	ch <- engine.OutputLine{Stream: "stderr", Line: "warning"}
	ch <- engine.OutputLine{Done: true, Code: 7}
	close(ch)

	out, code := collectOutput(t, ch)
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	if out != "hello\nwarning\n" {
		t.Fatalf("output = %q, want %q", out, "hello\nwarning\n")
	}
}

func TestIntegrationHelperRunCommand(t *testing.T) {
	code := runCommand(t, func(context.Context) (<-chan engine.OutputLine, error) {
		ch := make(chan engine.OutputLine, 2)
		ch <- engine.OutputLine{Stream: "stdout", Line: "ok"}
		ch <- engine.OutputLine{Done: true, Code: 3}
		close(ch)
		return ch, nil
	})
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
}

func TestIntegrationHelperIntegrationEnabledHonorsSkip(t *testing.T) {
	t.Setenv("CLAB_TUI_SKIP_INTEGRATION", "1")
	if integrationEnabled(t) {
		t.Fatal("integrationEnabled = true when skip is set")
	}
}

func TestIntegrationHelperWaitForLab(t *testing.T) {
	e := &helperTestEngine{snapshot: &engine.Snapshot{Labs: map[string]*engine.Lab{
		"demo": {Name: "demo", Nodes: []engine.Node{{Name: "r1"}}},
	}}}

	lab := waitForLab(t, e, "demo", time.Second, func(lab *engine.Lab) bool {
		return len(lab.Nodes) == 1
	})
	if lab.Name != "demo" {
		t.Fatalf("lab = %q, want demo", lab.Name)
	}
}

func TestIntegrationHelperWaitForLabTimeout(t *testing.T) {
	if os.Getenv("CLAB_TUI_HELPER_TIMEOUT_TEST") == "1" {
		waitForLab(t, &helperTestEngine{snapshot: &engine.Snapshot{Labs: map[string]*engine.Lab{}}}, // want timeout
			"missing", 50*time.Millisecond, func(*engine.Lab) bool { return false })
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestIntegrationHelperWaitForLabTimeout")
	cmd.Env = append(os.Environ(), "CLAB_TUI_HELPER_TIMEOUT_TEST=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("waitForLab timeout subprocess succeeded")
	}
	if ctx.Err() != nil {
		t.Fatal("waitForLab timeout subprocess was not bounded")
	}
}

func TestIntegrationHelperLabNameMatching(t *testing.T) {
	if !matchesLabContainer("clab-lab-r1", "lab") {
		t.Fatal("expected lab container to match without a label")
	}
	if matchesLabContainer("clab-lab2-r1", "lab") {
		t.Fatal("lab2 container matched lab")
	}
}

func TestIntegrationHelperBestEffortOutputHonorsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, _, err := collectOutputBestEffort(t, ctx, make(chan engine.OutputLine))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
}

func TestIntegrationHelperClosedOutputIsFailure(t *testing.T) {
	ch := make(chan engine.OutputLine)
	close(ch)
	_, code, err := collectOutputBestEffort(t, context.Background(), ch)
	if err == nil {
		t.Fatal("closed output stream returned nil error")
	}
	if code == 0 {
		t.Fatal("closed output stream reported success")
	}
}

func TestIntegrationHelperBinaryUsesEnvironment(t *testing.T) {
	t.Setenv("CLAB_BIN", os.Args[0])
	if got := integrationBinary(t); got != os.Args[0] {
		t.Fatalf("integrationBinary = %q, want %q", got, os.Args[0])
	}
}

func TestIntegrationHelperDockerNetworkNotFound(t *testing.T) {
	for _, output := range []string{
		"Error response from daemon: network clab-demo not found",
		"No such network: clab-demo",
	} {
		if !dockerNetworkNotFound([]byte(output)) {
			t.Fatalf("dockerNetworkNotFound(%q) = false", output)
		}
	}
	for _, output := range []string{
		"permission denied",
		"Cannot connect to the Docker daemon at unix:///var/run/docker.sock",
		"network inspection failed",
		"permission denied: network not found",
	} {
		if dockerNetworkNotFound([]byte(output)) {
			t.Fatalf("dockerNetworkNotFound(%q) = true", output)
		}
	}
}

func TestIntegrationHelperDestroyUsesCleanup(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "containerlab")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$DESTROY_ARGS_FILE\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	topo := filepath.Join(dir, "demo.clab.yml")
	if err := os.WriteFile(topo, []byte("name: demo\ntopology:\n  nodes: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DESTROY_ARGS_FILE", argsFile)
	e, err := New(WithBinary(bin), WithLabDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	destroyLab(t, e, "demo")
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--cleanup\n") {
		t.Fatalf("destroy args = %q, want --cleanup", args)
	}
}

type helperTestEngine struct {
	engine.Engine
	snapshot *engine.Snapshot
}

func (e *helperTestEngine) Snapshot() *engine.Snapshot { return e.snapshot }

func integrationEnabled(t *testing.T) bool {
	t.Helper()
	return os.Getenv("CLAB_TUI_SKIP_INTEGRATION") == ""
}

func integrationBinary(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("CLAB_BIN")
	if bin == "" {
		bin = "containerlab"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("containerlab binary %q unavailable: %v", bin, err)
	}
	return bin
}

func requireDocker(t *testing.T) {
	t.Helper()
	requireCommand(t, "docker")
	output, err := runDockerCommand("info")
	if err != nil {
		t.Skipf("Docker daemon unavailable: %v: %s", err, output)
	}
}

func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
}

func requireImage(t *testing.T, image string) {
	t.Helper()
	requireDocker(t)
	output, err := runDockerCommand("image", "inspect", image)
	if err != nil {
		t.Skipf("Docker image %q unavailable: %v: %s", image, err, output)
	}
}

func requireImageStrict(t *testing.T, image string) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("required integration image %q cannot be checked: docker is unavailable: %v", image, err)
	}
	if output, err := runDockerCommand("info"); err != nil {
		t.Fatalf("required integration image %q cannot be checked: Docker daemon unavailable: %v: %s", image, err, output)
	}
	if output, err := runDockerCommand("image", "inspect", image); err != nil {
		t.Fatalf("required integration image %q is unavailable: %v: %s; pull the image before running the scale suite", image, err, output)
	}
}

func requireCommand(t *testing.T, command string) {
	t.Helper()
	if _, err := exec.LookPath(command); err != nil {
		t.Skipf("command %q unavailable: %v", command, err)
	}
}

func collectOutput(t *testing.T, ch <-chan engine.OutputLine) (string, int) {
	t.Helper()
	var output strings.Builder
	for {
		line, ok := <-ch
		if !ok {
			t.Fatal("output channel closed without Done sentinel")
		}
		if line.Done {
			return output.String(), line.Code
		}
		t.Logf("%s: %s", line.Stream, line.Line)
		output.WriteString(line.Line)
		output.WriteByte('\n')
	}
}

func runCommand(t *testing.T, command func(context.Context) (<-chan engine.OutputLine, error)) int {
	t.Helper()
	ch, err := command(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, code := collectOutput(t, ch)
	return code
}

func destroyLab(t *testing.T, e *ClabEngine, lab string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ch, err := e.Destroy(ctx, lab, engine.WithDestroyCleanup())
	if err != nil {
		t.Logf("cleanup destroy %q: %v", lab, err)
		return
	}
	output, code, outputErr := collectOutputBestEffort(t, ctx, ch)
	if outputErr != nil {
		t.Logf("cleanup destroy %q output: %v", lab, outputErr)
	}
	if code != 0 {
		t.Logf("cleanup destroy %q exited %d:\n%s", lab, code, output)
	}
}

func registerIntegrationCleanup(t *testing.T, e *ClabEngine, lab string) {
	t.Helper()
	t.Cleanup(func() {
		destroyLab(t, e, lab)
		assertNoLabResidue(t, lab)
		if err := e.Close(); err != nil {
			t.Logf("cleanup close engine %q: %v", lab, err)
		}
	})
}

func collectOutputBestEffort(t *testing.T, ctx context.Context, ch <-chan engine.OutputLine) (string, int, error) {
	t.Helper()
	var output strings.Builder
	for {
		select {
		case <-ctx.Done():
			return drainOutputAfterCancellation(t, ch, &output, ctx.Err())
		case line, ok := <-ch:
			if !ok {
				return output.String(), -1, fmt.Errorf("output channel closed without Done sentinel")
			}
			if line.Done {
				return output.String(), line.Code, nil
			}
			t.Logf("%s: %s", line.Stream, line.Line)
			output.WriteString(line.Line)
			output.WriteByte('\n')
		}
	}
}

const outputDrainGrace = 100 * time.Millisecond

func drainOutputAfterCancellation(t *testing.T, ch <-chan engine.OutputLine, output *strings.Builder, cause error) (string, int, error) {
	t.Helper()
	grace := time.NewTimer(outputDrainGrace)
	defer grace.Stop()
	code := -1
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return output.String(), code, fmt.Errorf("output drain canceled: %w", cause)
			}
			if line.Done {
				return output.String(), line.Code, nil
			}
			t.Logf("%s: %s", line.Stream, line.Line)
			output.WriteString(line.Line)
			output.WriteByte('\n')
		case <-grace.C:
			return output.String(), code, fmt.Errorf("output drain canceled after %s grace: %w", outputDrainGrace, cause)
		}
	}
}

func waitForLab(t *testing.T, e engine.Engine, lab string, timeout time.Duration, predicate func(*engine.Lab) bool) *engine.Lab {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot := e.Snapshot()
		if snapshot != nil {
			if current := snapshot.Labs[lab]; current != nil && predicate(current) {
				return current
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("lab %q did not converge within %s", lab, timeout)
		}
	}
}

func assertNoLabResidue(t *testing.T, lab string) {
	t.Helper()
	prefix := "clab-" + lab + "-"
	output, err := runDockerCommand("ps", "-a", "--filter", "name=^/"+prefix, "--format", "{{.Names}}")
	if err != nil {
		t.Fatalf("inspect Docker containers for lab %q: %v: %s", lab, err, output)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		if matchesLabContainer(line, lab) {
			t.Fatalf("Docker container remains for lab %q: %s", lab, line)
		}
	}

	network := "clab-" + lab
	if output, err := runDockerCommand("network", "inspect", network); err == nil {
		t.Fatalf("Docker network %q remains: %s", network, strings.TrimSpace(string(output)))
	} else if !dockerNetworkNotFound(output) {
		t.Fatalf("inspect Docker network %q: %v: %s", network, err, output)
	}
}

const dockerCommandTimeout = 10 * time.Second

func runDockerCommand(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerCommandTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return output, fmt.Errorf("docker %s timed out after %s: %w", strings.Join(args, " "), dockerCommandTimeout, ctx.Err())
		}
		return output, fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}

func matchesLabContainer(name, lab string) bool {
	return strings.HasPrefix(name, "clab-"+lab+"-")
}

func dockerNetworkNotFound(output []byte) bool {
	message := strings.ToLower(string(output))
	return strings.Contains(message, "no such network") ||
		(strings.Contains(message, "error response from daemon: network ") && strings.Contains(message, " not found"))
}
