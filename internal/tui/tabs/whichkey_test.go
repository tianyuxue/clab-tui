package tabs

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/tianyuxue/clab-tui/internal/tui/popup"
)

func TestWhichKeyDescendAndBack(t *testing.T) {
	w := NewWhichKey()
	w.Show([]MenuNode{
		{Label: "Lab", Children: []MenuNode{{Label: "Deploy", ActionID: "deploy"}}},
		{Label: "Trace", ActionID: "trace"},
	})
	if !w.Visible() {
		t.Fatal("expected menu visible after Show")
	}
	// 'l' is Lab's label -> descend.
	_, cmd := w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if cmd != nil {
		t.Fatalf("expected no command while descending into a group, got %+v", cmd)
	}
	if w.Depth() != 2 {
		t.Fatalf("expected depth 2 after descend, got %d", w.Depth())
	}
	// esc backs out one level.
	w.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if w.Depth() != 1 {
		t.Fatalf("expected depth 1 after esc, got %d", w.Depth())
	}
	// esc at root closes.
	w.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if w.Visible() {
		t.Fatal("expected menu closed after esc at root")
	}
}

func TestWhichKeyLetterTriggersAction(t *testing.T) {
	w := NewWhichKey()
	w.Show([]MenuNode{{Label: "Trace", ActionID: "trace"}})
	_, cmd := w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if cmd == nil {
		t.Fatal("expected command from letter trigger")
	}
	msg := cmd()
	am, ok := msg.(ActionSelectedMsg)
	if !ok || am.Item.ID != "trace" {
		t.Fatalf("expected ActionSelectedMsg{trace}, got %T %+v", msg, msg)
	}
	if w.Visible() {
		t.Fatal("expected menu closed after triggering an action")
	}
}

func TestWhichKeyEnterTriggersCursor(t *testing.T) {
	w := NewWhichKey()
	w.Show([]MenuNode{
		{Label: "Alpha", ActionID: "a"},
		{Label: "Beta", ActionID: "b"},
	})
	// j moves cursor to Beta, Enter triggers it.
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter")
	}
	am, ok := cmd().(ActionSelectedMsg)
	if !ok || am.Item.ID != "b" {
		t.Fatalf("expected ActionSelectedMsg{b} from Enter, got %+v", cmd())
	}
}

func TestWhichKeyLabelsExcludeNavKeys(t *testing.T) {
	w := NewWhichKey()
	w.Show([]MenuNode{
		{Label: "Jump", ActionID: "a"},
		{Label: "Keep", ActionID: "b"},
		{Label: "Quit-ish", ActionID: "c"},
		{Label: "Spacey", ActionID: "d"},
	})
	for _, l := range w.labelsOfCurrent() {
		if l == "j" || l == "k" || l == "q" || l == " " {
			t.Fatalf("navigation key %q assigned as a label: %v", l, w.labelsOfCurrent())
		}
	}
}

func TestWhichKeyBodyShowsCrumbAndDirectKey(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	w := NewWhichKey()
	w.Show([]MenuNode{
		{Label: "Lab", DirectKey: "l", Children: []MenuNode{{Label: "Deploy", ActionID: "deploy"}}},
	})
	body := w.Body()
	if !strings.Contains(body, "SPC") || !strings.Contains(body, "Lab") {
		t.Fatalf("expected root crumb and label in body:\n%s", body)
	}
	if !strings.Contains(body, "l") {
		t.Fatalf("expected direct key l in body:\n%s", body)
	}
	// Descend into Lab: crumb becomes SPC › Lab and DirectKey is gone (group
	// items in the submenu are leaf actions without direct keys).
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	body = w.Body()
	if !strings.Contains(body, "SPC › Lab") {
		t.Fatalf("expected child crumb in body:\n%s", body)
	}
	if !strings.Contains(body, "Deploy") {
		t.Fatalf("expected child item in body:\n%s", body)
	}
}

func TestWhichKeyBodyRightAlignsDirectKey(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	w := NewWhichKey()
	w.Show([]MenuNode{{Label: "Trace", DirectKey: "t", ActionID: "trace"}})
	body := w.Body()
	var directRow string
	for _, l := range strings.Split(body, "\n") {
		if strings.Contains(l, "Trace") {
			directRow = l
			break
		}
	}
	if directRow == "" {
		t.Fatal("expected a row containing the item label")
	}
	idx := strings.LastIndex(directRow, "t")
	if idx < 0 || directRow[idx+1:] != " │" {
		t.Fatalf("expected direct key right-aligned at row end, got %q", directRow)
	}
}

func TestWhichKeyImplementsPopupWithBottomRightAnchor(t *testing.T) {
	w := NewWhichKey()
	if w.Anchor() != popup.AnchorBottomRight {
		t.Fatalf("expected bottom-right anchor, got %d", w.Anchor())
	}
	if w.DesiredWidth() != 0 || w.DesiredHeight() != 0 {
		t.Fatal("expected auto-size (DesiredWidth/Height = 0) so Place uses the body size")
	}
}

func TestWhichKeyBodyFooterOnOneLine(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	w := NewWhichKey()
	w.Show([]MenuNode{
		{Label: "Trace", DirectKey: "t", ActionID: "trace"},
	})
	body := w.Body()
	found := false
	for _, l := range strings.Split(body, "\n") {
		if strings.Contains(l, "[letter] select") && strings.Contains(l, "[Esc] back") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected footer rendered on one line:\n%s", body)
	}
}
