package tabs

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSelectListHintAtBottom(t *testing.T) {
	sl := newSelectList("T", func(s string) string { return s }, func(s string) tea.Msg { return nil })
	sl.SetSize(80, 24)
	sl.Show([]string{"a", "b"})
	body := sl.Body()
	if !strings.Contains(body, "[Enter] ok") {
		t.Fatalf("expected hint in body:\n%s", body)
	}
	lines := strings.Split(body, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected bordered body, got %d lines", len(lines))
	}
	last := strings.TrimRight(lines[len(lines)-1], " ")
	if !strings.HasPrefix(last, "╰") || !strings.HasSuffix(last, "╯") {
		t.Fatalf("expected bottom border as last line, got %q", last)
	}
	if !strings.Contains(lines[len(lines)-3], "[Enter] ok") {
		t.Fatalf("expected hint just above bottom border, got:\n%q\n%q\n%q", lines[len(lines)-3], lines[len(lines)-2], lines[len(lines)-1])
	}
}
