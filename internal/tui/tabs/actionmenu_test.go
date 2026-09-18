package tabs

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ansiRe matches ANSI SGR escape sequences.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestActionMenuInit(t *testing.T) {
	m := NewActionMenu()
	if m.Visible() {
		t.Fatal("expected menu hidden by default")
	}
}

func TestActionMenuShow(t *testing.T) {
	m := NewActionMenu()
	m.Show("Actions", []ActionItem{
		{Label: "SSH", ID: "ssh"},
		{Label: "Start", ID: "start"},
		{Label: "Stop", ID: "stop"},
	})
	if !m.Visible() {
		t.Fatal("expected menu visible after Show")
	}
	if m.Selected().ID != "ssh" {
		t.Fatalf("expected first selection ssh, got %+v", m.Selected())
	}
	if !strings.Contains(m.Body(), "Actions") {
		t.Fatalf("expected title Actions in body:\n%s", m.Body())
	}
}

func TestActionMenuMove(t *testing.T) {
	m := NewActionMenu()
	m.Show("", []ActionItem{
		{Label: "A", ID: "a"},
		{Label: "B", ID: "b"},
		{Label: "C", ID: "c"},
	})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.Selected().ID != "b" {
		t.Fatalf("expected b after j, got %s", m.Selected().ID)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.Selected().ID != "c" {
		t.Fatalf("expected c after second j, got %s", m.Selected().ID)
	}
	// Bottom bound.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.Selected().ID != "c" {
		t.Fatalf("expected to stay c at bottom, got %s", m.Selected().ID)
	}
	// Move up.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.Selected().ID != "b" {
		t.Fatalf("expected b after k, got %s", m.Selected().ID)
	}
}

func TestActionMenuEnterEmitsSelection(t *testing.T) {
	m := NewActionMenu()
	m.Show("", []ActionItem{
		{Label: "SSH", ID: "ssh"},
		{Label: "Stop", ID: "stop"},
	})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if cmd == nil {
		t.Fatal("expected command from Enter")
	}
	msg := cmd()
	sel, ok := msg.(ActionSelectedMsg)
	if !ok {
		t.Fatalf("expected ActionSelectedMsg, got %T", msg)
	}
	if sel.Item.ID != "ssh" {
		t.Fatalf("expected ssh, got %s", sel.Item.ID)
	}
	// Menu hidden after selection.
	if m.Visible() {
		t.Fatal("expected menu hidden after selection")
	}
}

func TestActionMenuEscHides(t *testing.T) {
	m := NewActionMenu()
	m.Show("", []ActionItem{{Label: "A", ID: "a"}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	if m.Visible() {
		t.Fatal("expected menu hidden after esc")
	}
}

func TestActionMenuBodyHasBorders(t *testing.T) {
	m := NewActionMenu()
	m.SetSize(80, 24)
	m.Show("Actions", []ActionItem{
		{Label: "SSH", ID: "ssh"},
		{Label: "Start", ID: "start"},
	})
	body := m.Body()
	lines := strings.Split(body, "\n")
	if len(lines) < 4 {
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
	// Title present.
	foundTitle := false
	for _, l := range lines {
		if strings.Contains(l, "Actions") {
			foundTitle = true
		}
	}
	if !foundTitle {
		t.Fatal("expected title in body")
	}
	// Desired size covers the body.
	if m.DesiredHeight() < len(lines) {
		t.Fatalf("DesiredHeight %d < body %d lines", m.DesiredHeight(), len(lines))
	}
}

func TestActionMenuBodyEmptyItems(t *testing.T) {
	m := NewActionMenu()
	m.Show("Empty", nil)
	body := m.Body()
	if body == "" {
		t.Fatal("expected non-empty body even with no items")
	}
}

func TestActionMenuLetterSelect(t *testing.T) {
	m := NewActionMenu()
	m.Show("Node Actions", []ActionItem{
		{Label: "SSH", ID: "ssh"},
		{Label: "Start", ID: "start"},
		{Label: "Stop", ID: "stop"},
		{Label: "Restart", ID: "restart"},
	})
	// First letters S,S,S,R → labels s,t,o,r
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if cmd == nil {
		t.Fatal("expected command from letter o (Stop)")
	}
	msg := cmd()
	sel, ok := msg.(ActionSelectedMsg)
	if !ok {
		t.Fatalf("expected ActionSelectedMsg, got %T", msg)
	}
	if sel.Item.ID != "stop" {
		t.Fatalf("expected stop, got %s", sel.Item.ID)
	}
	if m.Visible() {
		t.Fatal("expected menu hidden after letter select")
	}
}

func TestActionMenuLetterNotMatched(t *testing.T) {
	m := NewActionMenu()
	m.Show("Lab Actions", []ActionItem{{Label: "Deploy", ID: "deploy"}})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	if cmd != nil {
		t.Fatal("expected no command for unmatched letter")
	}
	if !m.Visible() {
		t.Fatal("expected menu still visible")
	}
}

func TestActionMenuBodyHighlightsFirstLetter(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	m := NewActionMenu()
	m.Show("Node Actions", []ActionItem{
		{Label: "SSH", ID: "ssh"},
		{Label: "Restart", ID: "restart"},
	})
	body := m.Body()
	clean := ansiRe.ReplaceAllString(body, "")
	if !strings.Contains(clean, "SSH") || !strings.Contains(clean, "Restart") {
		t.Fatalf("expected item labels in body:\n%s", body)
	}
	if !strings.Contains(body, "\x1b[38;5;196m") {
		t.Fatalf("expected red (38;5;196) highlight in body:\n%s", body)
	}
}

func TestActionMenuLabelReservesNavKeys(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	m := NewActionMenu()
	m.Show("Actions", []ActionItem{
		{Label: "Jump", ID: "jump"},
		{Label: "Quit", ID: "quit"},
		{Label: "Kite", ID: "kite"},
	})
	// Reserved navigation/close keys must never be highlighted as labels.
	labelHl := regexp.MustCompile(`\x1b\[38;5;196m(.)\x1b\[0m`)
	for _, mm := range labelHl.FindAllStringSubmatch(m.Body(), -1) {
		if r := mm[1]; r == "j" || r == "k" || r == "q" {
			t.Fatalf("reserved key %q highlighted as a label:\n%s", r, m.Body())
		}
	}
	// Pressing the reserved letters must not select anything.
	for _, r := range []rune("jkq") {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if cmd != nil {
			t.Fatalf("expected no selection for reserved key %q", string(r))
		}
	}
	if m.Selected().ID != "jump" {
		t.Fatalf("expected cursor still at jump after reserved keys, got %s", m.Selected().ID)
	}
}

func TestActionMenuPageNavigation(t *testing.T) {
	items := make([]ActionItem, 12)
	for i := range items {
		items[i] = ActionItem{Label: fmt.Sprintf("Item %d", i), ID: fmt.Sprintf("item-%d", i)}
	}
	m := NewActionMenu()
	m.Show("Actions", items)
	if m.Selected().ID != "item-0" {
		t.Fatalf("expected item-0 on page 1, got %s", m.Selected().ID)
	}
	// Cursor below first position, then ] must advance page and reset cursor.
	for i := 0; i < 3; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	if m.Selected().ID != "item-9" {
		t.Fatalf("expected item-9 (page 2, cursor reset), got %s", m.Selected().ID)
	}
	// ] at last page no-op.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	if m.Selected().ID != "item-9" {
		t.Fatalf("expected stay item-9 at last page, got %s", m.Selected().ID)
	}
	// [ returns to page 1, cursor reset.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if m.Selected().ID != "item-0" {
		t.Fatalf("expected item-0 back on page 1, got %s", m.Selected().ID)
	}
	// [ at page 0 no-op.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if m.Selected().ID != "item-0" {
		t.Fatalf("expected stay item-0 at first page, got %s", m.Selected().ID)
	}
}

func TestActionMenuLetterOnSecondPage(t *testing.T) {
	items := make([]ActionItem, 12)
	for i := range items {
		items[i] = ActionItem{Label: fmt.Sprintf("Node%02d", i), ID: fmt.Sprintf("node-%02d", i)}
	}
	items[9] = ActionItem{Label: "Zebra", ID: "zebra"}
	m := NewActionMenu()
	m.Show("Actions", items)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	if cmd == nil {
		t.Fatal("expected command selecting zebra on page 2")
	}
	sel, ok := cmd().(ActionSelectedMsg)
	if !ok || sel.Item.ID != "zebra" {
		t.Fatalf("expected zebra selected, got %+v", sel)
	}
}
