package tabs

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestSessionPickerLetterSelect(t *testing.T) {
	p := NewSessionPicker()
	p.SetSize(80, 24)
	p.Show([]*engine.SessionHandle{
		{ID: "s1", Title: "r1"},
		{ID: "s2", Title: "srv2"},
	})
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("expected command from letter s")
	}
	msg := cmd()
	sm, ok := msg.(SessionPickedMsg)
	if !ok {
		t.Fatalf("expected SessionPickedMsg, got %T", msg)
	}
	if sm.Session.ID != "s2" {
		t.Fatalf("expected s2, got %s", sm.Session.ID)
	}
}

func TestSessionPickerEmptyNotVisible(t *testing.T) {
	p := NewSessionPicker()
	p.Show(nil)
	if p.Visible() {
		t.Fatal("picker with no sessions should not be visible")
	}
}

func TestSessionPickerBodyHighlights(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	p := NewSessionPicker()
	p.SetSize(80, 24)
	p.Show([]*engine.SessionHandle{{ID: "s1", Title: "alpha"}, {ID: "s2", Title: "beta"}})
	body := p.Body()
	if !strings.Contains(body, "\x1b[38;5;196ma\x1b[0mlpha") || !strings.Contains(body, "\x1b[38;5;196mb\x1b[0meta") {
		t.Fatalf("expected highlighted session titles in body:\n%s", body)
	}
	if !strings.Contains(body, "\x1b[38;5;196m") {
		t.Fatalf("expected red highlight in body:\n%s", body)
	}
}

func TestSessionPickerEnterSelects(t *testing.T) {
	p := NewSessionPicker()
	p.SetSize(80, 24)
	p.Show([]*engine.SessionHandle{{ID: "s1", Title: "r1"}, {ID: "s2", Title: "r2"}})
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter")
	}
	msg := cmd()
	sm, ok := msg.(SessionPickedMsg)
	if !ok {
		t.Fatalf("expected SessionPickedMsg, got %T", msg)
	}
	if sm.Session.ID != "s1" {
		t.Fatalf("expected s1 (first) on Enter, got %s", sm.Session.ID)
	}
}

func TestSessionPickerEscHides(t *testing.T) {
	p := NewSessionPicker()
	p.SetSize(80, 24)
	p.Show([]*engine.SessionHandle{{ID: "s1", Title: "r1"}})
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	if p.Visible() {
		t.Fatal("expected picker hidden after esc")
	}
}
