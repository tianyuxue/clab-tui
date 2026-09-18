package tabs

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConfirmInitialState(t *testing.T) {
	c := NewConfirm()
	c.Show("Confirm", "Start node r1?", "start")
	if !c.Visible() {
		t.Fatal("expected confirm visible after Show")
	}
	if got := c.Selected(); got != "confirm" {
		t.Fatalf("expected initial selection confirm, got %s", got)
	}
}

func TestConfirmToggleSelection(t *testing.T) {
	c := NewConfirm()
	c.Show("Confirm", "Stop node r1?", "stop")
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := c.Selected(); got != "cancel" {
		t.Fatalf("expected cancel after j, got %s", got)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if got := c.Selected(); got != "confirm" {
		t.Fatalf("expected confirm after k, got %s", got)
	}
}

func TestConfirmEnterEmitsResult(t *testing.T) {
	c := NewConfirm()
	c.Show("Confirm", "Restart node r1?", "restart")
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter")
	}
	msg := cmd()
	cr, ok := msg.(ConfirmResultMsg)
	if !ok {
		t.Fatalf("expected ConfirmResultMsg, got %T", msg)
	}
	if !cr.Confirmed || cr.ActionID != "restart" {
		t.Fatalf("unexpected result: %+v", cr)
	}
	if c.Visible() {
		t.Fatal("expected confirm hidden after Enter")
	}
}

func TestConfirmEnterOnCancelYieldsFalse(t *testing.T) {
	c := NewConfirm()
	c.Show("Confirm", "Stop node r1?", "stop")
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter")
	}
	msg := cmd()
	cr, ok := msg.(ConfirmResultMsg)
	if !ok {
		t.Fatalf("expected ConfirmResultMsg, got %T", msg)
	}
	if cr.Confirmed || cr.ActionID != "stop" {
		t.Fatalf("unexpected result: %+v", cr)
	}
	if c.Visible() {
		t.Fatal("expected confirm hidden after Enter")
	}
}

func TestConfirmEscCancels(t *testing.T) {
	c := NewConfirm()
	c.Show("Confirm", "Stop node r1?", "stop")
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command from Esc")
	}
	msg := cmd()
	cr, ok := msg.(ConfirmResultMsg)
	if !ok {
		t.Fatalf("expected ConfirmResultMsg, got %T", msg)
	}
	if cr.Confirmed || cr.ActionID != "stop" {
		t.Fatalf("unexpected result: %+v", cr)
	}
	if c.Visible() {
		t.Fatal("expected confirm hidden after Esc")
	}
}
