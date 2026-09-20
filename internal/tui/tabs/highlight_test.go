package tabs

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestSplitLabel(t *testing.T) {
	cases := []struct {
		name, label string
		before, mid string
		after       string
	}{
		{"SSH", "s", "", "S", "SH"},
		{"clab-lab-r1", "l", "c", "l", "ab-lab-r1"},
		{"r1", "", "r1", "", ""},
		{"r1", "z", "r1", "", ""},
		{"srv2", "v", "sr", "v", "2"},
	}
	for _, c := range cases {
		b, m, a := splitLabel(c.name, c.label)
		if b != c.before || m != c.mid || a != c.after {
			t.Fatalf("splitLabel(%q,%q)=(%q,%q,%q), want (%q,%q,%q)",
				c.name, c.label, b, m, a, c.before, c.mid, c.after)
		}
	}
}

func TestHighlightLabelEmptyLabel(t *testing.T) {
	if got := highlightLabel("SSH", ""); got != "SSH" {
		t.Fatalf("empty label: got %q, want SSH", got)
	}
}

func TestHighlightLabelNotInName(t *testing.T) {
	if got := highlightLabel("SSH", "z"); got != "SSH" {
		t.Fatalf("missing label: got %q, want SSH unchanged", got)
	}
}

func TestHighlightLabelFirstLetter(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	out := highlightLabel("SSH", "s")
	if !strings.HasPrefix(out, "\x1b[38;5;196mS\x1b[0mSH") {
		t.Fatalf("expected first letter highlighted, got %q", out)
	}
}

func TestHighlightLabelMultibyteOffset(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	out := highlightLabel("λalpha", "a")
	// λ is 2 bytes; the highlighted a must immediately follow it.
	if !strings.HasPrefix(out, "λ") {
		t.Fatalf("expected λ prefix preserved, got %q", out)
	}
	rest := out[len("λ"):]
	if !strings.HasPrefix(rest, "\x1b[38;5;196m") {
		t.Fatalf("expected red highlight at correct byte offset after λ, got %q", rest)
	}
	body := strings.TrimPrefix(rest, "\x1b[38;5;196m")
	if body != "a\x1b[0mlpha" {
		t.Fatalf("expected highlighted a followed by reset and lpha, got %q", body)
	}
}
