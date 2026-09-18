package tabs

import (
	"strings"
	"testing"
)

func TestPadToHeight(t *testing.T) {
	cases := []struct {
		in     string
		height int
	}{
		{"hello", 5},
		{"", 3},
		{"a\nb", 4},
		{"already long enough", 2},
		{"x", 0},
		{"x", -1},
	}
	for _, c := range cases {
		out := padToHeight(c.in, c.height)
		if c.height > 0 {
			if got := len(strings.Split(out, "\n")); got != c.height {
				t.Fatalf("padToHeight(%q, %d) => %d lines, want %d", c.in, c.height, got, c.height)
			}
		} else {
			if out != c.in {
				t.Fatalf("padToHeight(%q, %d) should return input unchanged", c.in, c.height)
			}
		}
	}
}

func TestLogsEmptyStateFillsHeight(t *testing.T) {
	m := NewLogs()
	m.SetSize(80, 30)
	v := m.View()
	if got := len(strings.Split(v, "\n")); got != 30 {
		t.Fatalf("expected 30 lines for empty logs view, got %d", got)
	}
}

func TestOpsEmptyStateFillsHeight(t *testing.T) {
	m := NewOps()
	m.SetSize(80, 30)
	v := m.View()
	if got := len(strings.Split(v, "\n")); got != 30 {
		t.Fatalf("expected 30 lines for empty ops view, got %d", got)
	}
}

func TestTopologyEmptyStateFillsHeight(t *testing.T) {
	m := NewDevTree()
	m.SetSize(80, 30)
	v := m.View()
	if got := len(strings.Split(v, "\n")); got != 30 {
		t.Fatalf("expected 30 lines for empty topology view, got %d", got)
	}
}
