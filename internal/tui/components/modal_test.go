package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModalNew(t *testing.T) {
	m := NewModal("Test Title", "Test message")
	if m.Title != "Test Title" {
		t.Fatalf("expected 'Test Title', got %s", m.Title)
	}
	if m.Visible {
		t.Fatal("expected hidden by default")
	}
}

func TestModalShowHide(t *testing.T) {
	m := NewModal("Title", "Msg")
	m.Show()
	if !m.Visible {
		t.Fatal("expected visible after Show")
	}
	m.Hide()
	if m.Visible {
		t.Fatal("expected hidden after Hide")
	}
}

func TestModalConfirm(t *testing.T) {
	confirmed := false
	m := NewModal("Confirm", "Sure?")
	m.OnConfirm = func() tea.Cmd {
		confirmed = true
		return nil
	}
	m.Show()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if !confirmed {
		t.Fatal("expected OnConfirm to be called")
	}
	if m.Visible {
		t.Fatal("expected modal hidden after confirm")
	}
}

func TestModalCancel(t *testing.T) {
	cancelled := false
	m := NewModal("Cancel", "Sure?")
	m.OnCancel = func() tea.Cmd {
		cancelled = true
		return nil
	}
	m.Show()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	if !cancelled {
		t.Fatal("expected OnCancel to be called")
	}
}

func TestModalView(t *testing.T) {
	m := NewModal("Title", "Msg")
	if m.View() != "" {
		t.Fatal("expected empty view when hidden")
	}
	m.Show()
	v := m.View()
	if v == "" {
		t.Fatal("expected non-empty view when visible")
	}
}
