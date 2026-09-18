package tabs

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSearchInputInit(t *testing.T) {
	s := NewSearchInput()
	if s.Visible() {
		t.Fatal("expected hidden by default")
	}
}

func TestSearchInputShowAndType(t *testing.T) {
	s := NewSearchInput()
	s.Show()
	if !s.Visible() {
		t.Fatal("expected visible after Show")
	}
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if s.Value() != "lea" {
		t.Fatalf("expected value lea, got %q", s.Value())
	}
}

func TestSearchInputBackspace(t *testing.T) {
	s := NewSearchInput()
	s.Show()
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	s.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if s.Value() != "" {
		t.Fatalf("expected empty after backspace, got %q", s.Value())
	}
}

func TestSearchInputEnterEmitsMsg(t *testing.T) {
	s := NewSearchInput()
	s.Show()
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter")
	}
	msg := cmd()
	sm, ok := msg.(SearchCommitMsg)
	if !ok {
		t.Fatalf("expected SearchCommitMsg, got %T", msg)
	}
	if sm.Term != "x" {
		t.Fatalf("expected term x, got %q", sm.Term)
	}
	if s.Visible() {
		t.Fatal("expected input hidden after Enter")
	}
}

func TestSearchInputEscHides(t *testing.T) {
	s := NewSearchInput()
	s.Show()
	s.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if s.Visible() {
		t.Fatal("expected hidden after esc")
	}
}

func TestSearchInputBody(t *testing.T) {
	s := NewSearchInput()
	s.SetSize(80, 24)
	s.Show()
	body := s.Body()
	if body == "" {
		t.Fatal("expected non-empty body")
	}
	if s.DesiredHeight() < 3 {
		t.Fatal("expected reasonable height")
	}
}

func TestSearchInputCustomFilterBody(t *testing.T) {
	s := NewSearchInput()
	s.SetSize(100, 30)
	s.ShowCustomFilter()
	body := s.Body()
	for _, want := range []string{
		"Custom Packet Filter",
		"Applied using libpcap tcpdump syntax.",
		"Reference: https://www.tcpdump.org/manpages/pcap-filter.7.html",
		"[Enter] apply  [Esc] cancel",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("custom filter body missing %q:\n%s", want, body)
		}
	}
	if s.DesiredHeight() <= 9 {
		t.Fatalf("expected custom filter body to be taller than search body, got %d", s.DesiredHeight())
	}
	s.Show()
	if strings.Contains(s.Body(), "Custom Packet Filter") {
		t.Fatal("normal search mode should not use custom filter title")
	}
}

func TestSearchInputCaptureFileBody(t *testing.T) {
	s := NewSearchInput()
	s.SetSize(100, 30)
	s.ShowCaptureFile()
	body := s.Body()
	for _, want := range []string{
		"Save Capture",
		"Write matching packets to a pcap file.",
		"File path:",
		"[Enter] save  [Esc] cancel",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("capture file body missing %q:\n%s", want, body)
		}
	}
}

func TestSearchInputMultiRunes(t *testing.T) {
	s := NewSearchInput()
	s.Show()
	// Bubble Tea may send multiple runes in one KeyMsg.
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("host")})
	if s.Value() != "host" {
		t.Fatalf("expected host, got %q", s.Value())
	}
	// Enter commits.
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter")
	}
	if msg := cmd(); msg.(SearchCommitMsg).Term != "host" {
		t.Fatalf("expected host, got %q", msg.(SearchCommitMsg).Term)
	}
}

func TestSearchInputSupportsSpacesAndCursorEditing(t *testing.T) {
	s := NewSearchInput()
	s.ShowCustomFilter()
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab")})
	s.Update(tea.KeyMsg{Type: tea.KeySpace})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("cd")})
	if s.Value() != "ab cd" {
		t.Fatalf("expected spaces to be inserted, got %q", s.Value())
	}

	s.Update(tea.KeyMsg{Type: tea.KeyLeft})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if s.Value() != "ab cXd" {
		t.Fatalf("expected left arrow to move cursor before insertion, got %q", s.Value())
	}

	s.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if s.Value() != "ab cX" {
		t.Fatalf("expected delete to remove the character after cursor, got %q", s.Value())
	}
}

func TestSearchInputInlineTextShowsCursor(t *testing.T) {
	s := NewSearchInput()
	s.Show()
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("leaf")})
	if got := s.InlineText(); got != "/leaf▌" {
		t.Fatalf("inline text = %q, want /leaf cursor", got)
	}
}
