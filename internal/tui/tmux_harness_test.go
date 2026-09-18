package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
)

type tmuxHarness struct {
	session    string
	stderrPath string
	closePath  string
}

func newTmuxHarness(t *testing.T, width, height int) *tmuxHarness {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skipf("tmux not installed: %v", err)
	}
	base := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, t.Name())
	name := fmt.Sprintf("clab-tui-%s-%d", base, time.Now().UnixNano())
	dir := t.TempDir()
	h := &tmuxHarness{session: name, stderrPath: filepath.Join(dir, "fixture.stderr"), closePath: filepath.Join(dir, "session-close.log")}
	t.Cleanup(func() { h.close(t) })
	return h
}

func (h *tmuxHarness) start(t *testing.T, width, height int, command ...string) {
	t.Helper()
	args := []string{"new-session", "-d", "-x", fmt.Sprint(width), "-y", fmt.Sprint(height), "-s", h.session}
	args = append(args, command...)
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("start tmux session: %v\n%s", err, out)
	}
}

func (h *tmuxHarness) sendKeys(t *testing.T, keys string) {
	t.Helper()
	if out, err := exec.Command("tmux", "send-keys", "-t", h.session, keys).CombinedOutput(); err != nil {
		t.Fatalf("send tmux keys %q: %v\n%s", keys, err, out)
	}
}

func (h *tmuxHarness) capture(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("tmux", "capture-pane", "-p", "-e", "-t", h.session).CombinedOutput()
	if err != nil {
		t.Fatalf("capture tmux pane: %v\n%s", err, out)
	}
	return string(out)
}

func (h *tmuxHarness) waitFor(t *testing.T, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(tmuxText(h.capture(t)), want) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q\nframe:\n%s", want, h.capture(t))
}

func (h *tmuxHarness) waitForAbsent(t *testing.T, unwanted string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !strings.Contains(tmuxText(h.capture(t)), unwanted) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q to disappear\nframe:\n%s", unwanted, h.capture(t))
}

func (h *tmuxHarness) waitForStderr(t *testing.T, want string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := os.ReadFile(h.stderrPath)
		if err == nil && strings.Contains(string(out), want) {
			return string(out)
		}
		time.Sleep(25 * time.Millisecond)
	}
	out, _ := os.ReadFile(h.stderrPath)
	t.Fatalf("timed out waiting for stderr %q, got %q", want, out)
	return string(out)
}

func (h *tmuxHarness) waitForClose(t *testing.T, want string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := os.ReadFile(h.closePath)
		if err == nil && strings.Contains(string(out), want) {
			return string(out)
		}
		time.Sleep(25 * time.Millisecond)
	}
	out, _ := os.ReadFile(h.closePath)
	t.Fatalf("timed out waiting for session close %q, got %q", want, out)
	return string(out)
}

func (h *tmuxHarness) resize(t *testing.T, width, height int) {
	t.Helper()
	if out, err := exec.Command("tmux", "resize-window", "-t", h.session, "-x", fmt.Sprint(width), "-y", fmt.Sprint(height)).CombinedOutput(); err != nil {
		t.Fatalf("resize tmux session: %v\n%s", err, out)
	}
}

func (h *tmuxHarness) close(t *testing.T) {
	t.Helper()
	if h.session == "" {
		return
	}
	has := exec.Command("tmux", "has-session", "-t", h.session)
	if out, err := has.CombinedOutput(); err != nil {
		if !tmuxSessionMissing(out) {
			t.Errorf("check tmux session %q during cleanup: %v\n%s", h.session, err, out)
		}
		h.session = ""
		return
	}
	if out, err := exec.Command("tmux", "kill-session", "-t", h.session).CombinedOutput(); err != nil && !tmuxSessionMissing(out) {
		t.Errorf("kill tmux session %q during cleanup: %v\n%s", h.session, err, out)
	}
	h.session = ""
}

func tmuxSessionMissing(output []byte) bool {
	text := strings.ToLower(string(output))
	return strings.Contains(text, "can't find session") || strings.Contains(text, "session not found")
}
