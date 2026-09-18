package tabs

import (
	"strings"
	"testing"

	"github.com/hinshun/vt10x"
)

func TestRenderTermRowPaletteColor(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(80, 24))
	_, _ = term.Write([]byte("\x1b[31mX"))
	row := renderTermRow(term, 0, 80)
	if !strings.Contains(row, "38;5;1") {
		t.Fatalf("expected palette fg code 38;5;1 in row, got %q", row)
	}
	if !strings.Contains(row, "X") {
		t.Fatalf("expected X in row, got %q", row)
	}
}

func TestRenderTermRowTruecolor(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(80, 24))
	_, _ = term.Write([]byte("\x1b[38;2;255;0;0mX"))
	row := renderTermRow(term, 0, 80)
	if !strings.Contains(row, "38;2;255;0;0") {
		t.Fatalf("expected truecolor fg code 38;2;255;0;0 in row, got %q", row)
	}
	if !strings.Contains(row, "X") {
		t.Fatalf("expected X in row, got %q", row)
	}
}

func TestRenderTermRowTruecolorBackground(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(80, 24))
	_, _ = term.Write([]byte("\x1b[48;2;10;128;255mX"))
	row := renderTermRow(term, 0, 80)
	if !strings.Contains(row, "48;2;10;128;255") {
		t.Fatalf("expected truecolor bg code 48;2;10;128;255 in row, got %q", row)
	}
}
