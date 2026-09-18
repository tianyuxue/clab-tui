package tabs

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestLabPickerInit(t *testing.T) {
	p := NewLabPicker()
	if p.Visible() {
		t.Fatal("expected picker hidden by default")
	}
}

func TestLabPickerShow(t *testing.T) {
	p := NewLabPicker()
	p.Show([]*engine.Lab{
		{Name: "lab-a", TopoFile: "/a.clab.yml"},
		{Name: "lab-b", TopoFile: "/b.clab.yml"},
	})
	if !p.Visible() {
		t.Fatal("expected picker visible after Show")
	}
	if p.Selected().Name != "lab-a" {
		t.Fatalf("expected first selection lab-a, got %s", p.Selected().Name)
	}
}

func TestLabPickerMoveAndSelect(t *testing.T) {
	p := NewLabPicker()
	p.Show([]*engine.Lab{
		{Name: "lab-a", TopoFile: "/a.clab.yml"},
		{Name: "lab-b", TopoFile: "/b.clab.yml"},
	})
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if p.Selected().Name != "lab-b" {
		t.Fatalf("expected lab-b after move, got %s", p.Selected().Name)
	}
}

func TestLabPickerEnterEmitsSelection(t *testing.T) {
	p := NewLabPicker()
	p.Show([]*engine.Lab{
		{Name: "lab-a", TopoFile: "/a.clab.yml"},
		{Name: "lab-b", TopoFile: "/b.clab.yml"},
	})
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if cmd == nil {
		t.Fatal("expected a command from Enter")
	}
	msg := cmd()
	sel, ok := msg.(LabPickedMsg)
	if !ok {
		t.Fatalf("expected LabPickedMsg, got %T", msg)
	}
	if sel.Lab.Name != "lab-a" {
		t.Fatalf("expected lab-a picked, got %s", sel.Lab.Name)
	}
}

func TestLabPickerEscHides(t *testing.T) {
	p := NewLabPicker()
	p.Show([]*engine.Lab{{Name: "lab-a", TopoFile: "/a.clab.yml"}})
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	if p.Visible() {
		t.Fatal("expected picker hidden after esc")
	}
}

func TestLabPickerViewNonEmpty(t *testing.T) {
	p := NewLabPicker()
	if p.View() != "" {
		t.Fatal("expected empty view when hidden")
	}
	p.Show([]*engine.Lab{{Name: "lab-a", TopoFile: "/a.clab.yml"}})
	if p.View() == "" {
		t.Fatal("expected non-empty view when visible")
	}
}

func TestLabPickerBodyHasFullBorder(t *testing.T) {
	p := NewLabPicker()
	p.SetSize(116, 30)
	p.Show([]*engine.Lab{{Name: "lab-a"}, {Name: "lab-b"}})
	body := p.Body()
	lines := strings.Split(body, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected bordered body, got %d lines", len(lines))
	}
	first := strings.TrimRight(lines[0], " ")
	last := strings.TrimRight(lines[len(lines)-1], " ")
	if !strings.HasPrefix(first, "╭") || !strings.HasSuffix(first, "╮") {
		t.Fatalf("expected top border, got %q", first)
	}
	if !strings.HasPrefix(last, "╰") || !strings.HasSuffix(last, "╯") {
		t.Fatalf("expected bottom border, got %q", last)
	}
	// Desired size must cover the whole body (borders included).
	if p.DesiredHeight() < len(lines) {
		t.Fatalf("DesiredHeight %d smaller than body %d lines", p.DesiredHeight(), len(lines))
	}
}

func TestLabPickerLetterSelect(t *testing.T) {
	p := NewLabPicker()
	p.Show([]*engine.Lab{
		{Name: "alpha-lab"},
		{Name: "bravo-lab"},
	})
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if cmd == nil {
		t.Fatal("expected command from letter b")
	}
	msg := cmd()
	pm, ok := msg.(LabPickedMsg)
	if !ok {
		t.Fatalf("expected LabPickedMsg, got %T", msg)
	}
	if pm.Lab.Name != "bravo-lab" {
		t.Fatalf("expected bravo-lab, got %s", pm.Lab.Name)
	}
}

func TestLabPickerBodyStatusColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	p := NewLabPicker()
	p.SetSize(80, 24)
	p.Show([]*engine.Lab{{Name: "alpha-lab", Nodes: []engine.Node{{State: engine.StatusStopped}}}})
	body := p.Body()
	if !strings.Contains(body, "\x1b[38;5;196m●") {
		t.Fatalf("expected red status glyph in body:\n%s", body)
	}
}

func TestLabPickerPageNavigation(t *testing.T) {
	p := NewLabPicker()
	labs := make([]*engine.Lab, 12)
	for i := range labs {
		labs[i] = &engine.Lab{Name: fmt.Sprintf("lab-%02d", i)}
	}
	p.SetSize(80, 24)
	p.Show(labs)
	// Next page
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	// Selected should reflect page 2 (items 9..11)
	if p.Selected().Name != "lab-09" {
		t.Fatalf("expected lab-09 after page turn, got %s", p.Selected().Name)
	}
	// Prev page
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if p.Selected().Name != "lab-00" {
		t.Fatalf("expected lab-00 after page back, got %s", p.Selected().Name)
	}
}

func TestLabPickerBodyShowsStatusAndNodes(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	p := NewLabPicker()
	p.SetSize(80, 24)
	p.Show([]*engine.Lab{
		{Name: "alpha", Nodes: []engine.Node{{State: engine.StatusRunning}}},
		{Name: "beta", Nodes: []engine.Node{{State: engine.StatusStopped}}},
	})
	body := ansiRe.ReplaceAllString(p.Body(), "")
	if !strings.Contains(body, "alpha") || !strings.Contains(body, "1 nodes") {
		t.Fatalf("expected alpha with node count in body:\n%s", body)
	}
	if !strings.Contains(body, "beta") || !strings.Contains(body, "1 nodes") {
		t.Fatalf("expected beta with node count in body:\n%s", body)
	}
}

func TestLabPickerBodyShowsLetterAndStatus(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	p := NewLabPicker()
	p.SetSize(80, 24)
	p.Show([]*engine.Lab{{Name: "alpha", Nodes: []engine.Node{{State: engine.StatusRunning}}}, {Name: "bravo", Nodes: []engine.Node{}}})
	body := p.Body()
	if !strings.Contains(body, "1 nodes") {
		t.Fatalf("expected node count, got:\n%s", body)
	}
	if !strings.Contains(body, "\x1b[38;5;196m") {
		t.Fatalf("expected red letter highlight, got:\n%s", body)
	}
}

func TestLabPickerLetterSelectStillWorks(t *testing.T) {
	p := NewLabPicker()
	p.SetSize(80, 24)
	p.Show([]*engine.Lab{{Name: "alpha"}, {Name: "bravo"}})
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if cmd == nil {
		t.Fatal("expected command from letter b")
	}
	if msg := cmd(); msg != nil {
		pm, ok := msg.(LabPickedMsg)
		if !ok || pm.Lab.Name != "bravo" {
			t.Fatalf("expected LabPickedMsg bravo, got %T %+v", msg, pm)
		}
	}
}
