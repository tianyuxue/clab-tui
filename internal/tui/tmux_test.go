package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tianyuxue/clab-tui/internal/capability"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

var tmuxANSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func tmuxText(frame string) string { return tmuxANSI.ReplaceAllString(frame, "") }

func frameRowContaining(frame, want string) string {
	for _, row := range strings.Split(tmuxText(frame), "\n") {
		if strings.Contains(row, want) {
			return strings.Join(strings.Fields(row), " ")
		}
	}
	return ""
}

func waitForActiveTab(t *testing.T, h *tmuxHarness, key, label string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	want := "38;5;212m " + label
	for time.Now().Before(deadline) {
		frame := h.capture(t)
		if strings.Contains(frame, want) && strings.Contains(tmuxText(frame), label) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for active tab %s (%s)\nframe:\n%s", key, label, h.capture(t))
}

func TestTmuxFixture(t *testing.T) {
	if os.Getenv("CLAB_TUI_TMUX_FIXTURE") != "1" {
		return
	}

	stdin := &sessionTestStdin{Buffer: &bytes.Buffer{}}
	sessionOutput := make(chan []byte, 1)
	sessionOutput <- []byte("session-one\r\n")
	fake := newFakeEngine()
	lab := &engine.Lab{Name: "tmux-lab", TopoFile: "/tmp/tmux-lab.clab.yml", Nodes: []engine.Node{
		{Name: "spine1", Group: "spine", State: engine.StatusRunning, Interfaces: []engine.Interface{{Name: "e1-1", State: "up"}}},
		{Name: "leaf1", Group: "leaf", State: engine.StatusRunning, Interfaces: []engine.Interface{{Name: "e1-1", State: "up"}}},
	}}
	fake.labs = []*engine.Lab{lab}
	fake.lab = lab
	tmuxFixtureSessionCount = 0
	tmuxFixtureSession = &engine.SessionHandle{ID: "session-one", Title: "spine1", LabName: lab.Name, NodeName: "spine1", Stdin: stdin, Output: sessionOutput, Resize: func(int, int) error { return nil }}
	fixtureStderr := "trace: fixture stderr\n"
	fmt.Fprint(os.Stderr, fixtureStderr)
	if path := os.Getenv("CLAB_TUI_STDERR_FILE"); path != "" {
		if err := os.WriteFile(path, []byte(fixtureStderr), 0600); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}

	_, err := tea.NewProgram(New(fake, fake.labs, WithCapabilities(capability.Capabilities{Root: true})), tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout)).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

var tmuxFixtureSession *engine.SessionHandle
var tmuxFixtureSessionCount int

func (f *fakeEngine) OpenSession(context.Context, string, string, engine.SessionMode) (*engine.SessionHandle, error) {
	if tmuxFixtureSession == nil {
		return nil, fmt.Errorf("session fixture unavailable")
	}
	tmuxFixtureSessionCount++
	h := *tmuxFixtureSession
	h.ID = fmt.Sprintf("session-%s", map[bool]string{true: "two", false: "one"}[tmuxFixtureSessionCount > 1])
	h.Title = fmt.Sprintf("spine%d", tmuxFixtureSessionCount)
	h.Output = func() <-chan []byte {
		ch := make(chan []byte, 1)
		ch <- []byte(h.ID + "\r\n")
		return ch
	}()
	h.Close = func() error {
		path := os.Getenv("CLAB_TUI_SESSION_CLOSE_FILE")
		if path == "" {
			return nil
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = fmt.Fprintln(file, h.ID)
		return err
	}
	return &h, nil
}

func (f *fakeEngine) Capture(context.Context, string, string, string, ...engine.CaptureOption) (<-chan engine.PacketEvent, func() error, error) {
	ch := make(chan engine.PacketEvent, 1)
	ch <- engine.PacketEvent{Node: "spine1", Iface: "e1-1", Pkt: engine.ParsedPacket{Direction: "IN", Proto: "tcp", Summary: "fixture packet", Len: 64}}
	return ch, func() error { return nil }, nil
}

func TestTmuxBlackBox(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skipf("tmux not installed: %v", err)
	}
	// Direct exec arguments must preserve spaces and quotes in the binary path.
	binary := filepath.Join(t.TempDir(), "tmux fixture 'binary' with spaces.test")
	cmd := exec.Command("go", "test", "./internal/tui", "-c", "-o", binary)
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build tmux test binary: %v\n%s", err, out)
	}

	newFixture := func(t *testing.T, width, height int) *tmuxHarness {
		h := newTmuxHarness(t, width, height)
		h.start(t, width, height,
			"env",
			"CLAB_TUI_TMUX_FIXTURE=1",
			"CLAB_TUI_SESSION_CLOSE_FILE="+h.closePath,
			"CLAB_TUI_STDERR_FILE="+h.stderrPath,
			// Pin the fixture's color profile so the rendered escape
			// sequences do not depend on the CI runner's terminal.
			// CI/GITHUB_ACTIONS force color libraries to emit plain text,
			// so clear them for the fixture.
			"TERM=xterm-256color",
			"COLORTERM=",
			"NO_COLOR=",
			"CI=",
			"GITHUB_ACTIONS=",
			binary,
			"-test.run", "^TestTmuxFixture$", "-test.v")
		h.waitFor(t, "Select Lab", 5*time.Second)
		return h
	}

	t.Run("initial picker and topology", func(t *testing.T) {
		h := newFixture(t, 120, 40)
		if !strings.Contains(tmuxText(h.capture(t)), "tmux-lab") {
			t.Fatalf("expected lab picker entry:\n%s", h.capture(t))
		}
		h.sendKeys(t, "Enter")
		h.waitFor(t, "Topology", 2*time.Second)
		h.sendKeys(t, "q")
	})

	t.Run("tabs and menus", func(t *testing.T) {
		h := newFixture(t, 120, 40)
		h.sendKeys(t, "Enter")
		waitForActiveTab(t, h, "1", "Topology")
		h.sendKeys(t, "Tab")
		waitForActiveTab(t, h, "2", "Sessions")
		h.sendKeys(t, "BTab")
		waitForActiveTab(t, h, "1", "Topology")
		for _, tab := range []struct{ key, label string }{{"2", "Sessions"}, {"3", "Node Logs"}, {"4", "Ops Log"}, {"1", "Topology"}} {
			h.sendKeys(t, tab.key)
			waitForActiveTab(t, h, tab.key, tab.label)
		}
		h.sendKeys(t, "l")
		h.waitFor(t, "Lab Actions", 2*time.Second)
		h.sendKeys(t, "j")
		h.sendKeys(t, "j")
		h.waitFor(t, "Destroy", 2*time.Second)
		h.sendKeys(t, "Escape")
		h.sendKeys(t, "o")
		h.waitFor(t, "Node Actions", 2*time.Second)
		h.sendKeys(t, "Escape")
		h.sendKeys(t, "q")
	})

	t.Run("sessions and capture", func(t *testing.T) {
		h := newFixture(t, 120, 40)
		h.sendKeys(t, "Enter")
		h.waitFor(t, "Topology", 2*time.Second)
		h.sendKeys(t, "o")
		h.waitFor(t, "Node Actions", 2*time.Second)
		h.sendKeys(t, "s")
		h.waitFor(t, "session-one", 2*time.Second)
		h.sendKeys(t, "C-\\")
		h.sendKeys(t, "s")
		h.waitFor(t, "Sessions", 2*time.Second)
		h.sendKeys(t, "Escape")
		h.sendKeys(t, "1")
		h.sendKeys(t, "o")
		h.waitFor(t, "Node Actions", 2*time.Second)
		h.sendKeys(t, "s")
		h.waitFor(t, "session-two", 2*time.Second)
		h.sendKeys(t, "C-\\")
		h.sendKeys(t, "s")
		h.waitFor(t, "Sessions", 2*time.Second)
		h.sendKeys(t, "j")
		h.sendKeys(t, "Enter")
		h.waitFor(t, "session-two", 2*time.Second)
		h.sendKeys(t, "C-\\")
		h.sendKeys(t, "p")
		h.waitFor(t, "[NORMAL]", 2*time.Second)
		h.sendKeys(t, "i")
		h.waitFor(t, "session-two", 2*time.Second)
		h.sendKeys(t, "q")
		h.waitFor(t, "session-one", 2*time.Second)
		h.waitForAbsent(t, "session-two", 2*time.Second)
		h.waitForClose(t, "session-two", 2*time.Second)
		h.sendKeys(t, "1")
		h.sendKeys(t, "j")
		h.sendKeys(t, "o")
		h.waitFor(t, "Interface Actions", 2*time.Second)
		h.sendKeys(t, "Enter")
		h.waitFor(t, "Capture filter", 2*time.Second)
		h.sendKeys(t, "Enter")
		h.waitFor(t, "fixture packet", 2*time.Second)
		h.sendKeys(t, "p")
		h.waitFor(t, "PAUSED", 2*time.Second)
		h.sendKeys(t, "c")
		h.waitFor(t, "No packets captured yet", 2*time.Second)
		h.sendKeys(t, "q")
		h.waitFor(t, "Topology", 2*time.Second)
	})

	t.Run("resize and stderr isolation", func(t *testing.T) {
		h := newFixture(t, 120, 40)
		h.sendKeys(t, "Enter")
		waitForActiveTab(t, h, "1", "Topology")
		h.waitForAbsent(t, "Select Lab", 2*time.Second)
		h.waitForStderr(t, "trace: fixture stderr", 2*time.Second)
		h.waitFor(t, "State:", 2*time.Second)
		before := h.capture(t)
		beforeHelp := frameRowContaining(before, "space menu")
		beforeStatus := frameRowContaining(before, "State:")
		if beforeHelp == "" || beforeStatus == "" {
			t.Fatalf("missing stable rows before resize:\n%s", before)
		}
		stableHelp := []string{"space menu", "q quit"}
		for _, want := range stableHelp {
			if !strings.Contains(beforeHelp, want) {
				t.Fatalf("help row missing %q before resize: %q", want, beforeHelp)
			}
		}
		h.resize(t, 80, 24)
		waitForActiveTab(t, h, "1", "Topology")
		frame := h.capture(t)
		afterHelp := frameRowContaining(frame, "space menu")
		afterStatus := frameRowContaining(frame, "State:")
		if afterHelp == "" || afterStatus == "" {
			t.Fatalf("missing stable rows after resize:\n%s", frame)
		}
		for _, want := range stableHelp {
			if !strings.Contains(afterHelp, want) {
				t.Fatalf("help row lost %q after resize: %q", want, afterHelp)
			}
		}
		if strings.ReplaceAll(beforeStatus, " ", "") != strings.ReplaceAll(afterStatus, " ", "") {
			t.Fatalf("status row changed after resize: before %q, after %q", beforeStatus, afterStatus)
		}
		if strings.Contains(tmuxText(frame), "trace:") {
			t.Fatalf("stderr polluted pane:\n%s", frame)
		}
		if !strings.Contains(tmuxText(frame), "Topology") {
			t.Fatalf("stderr corrupted rendered frame:\n%s", frame)
		}
		if got := len(strings.Split(strings.TrimRight(frame, "\n"), "\n")); got < 23 {
			t.Fatalf("expected captured frame to retain the 24-row viewport, got %d rows", got)
		}
		h.sendKeys(t, "q")
	})
}

var _ io.WriteCloser = (*sessionTestStdin)(nil)
