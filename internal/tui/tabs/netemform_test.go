package tabs

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNetemFormEditsAndCommits(t *testing.T) {
	f := NewNetemForm()
	f.SetSize(80, 24)
	f.Show()
	for _, r := range []rune("10ms") {
		f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	f.Update(tea.KeyMsg{Type: tea.KeyTab})
	f.Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range []rune("1%") {
		f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected netem submit command")
	}
	msg, ok := cmd().(NetemCommitMsg)
	if !ok {
		t.Fatalf("expected NetemCommitMsg, got %T", cmd())
	}
	if msg.State.Delay != "10ms" || msg.State.Loss != "1%" {
		t.Fatalf("unexpected netem state: %+v", msg.State)
	}
}

func TestNetemFormBodyShowsFields(t *testing.T) {
	f := NewNetemForm()
	f.SetSize(80, 24)
	f.Show()
	body := f.Body()
	for _, want := range []string{"Set Network Emulation", "Delay", "Jitter", "Loss", "Rate", "Corruption", "[Enter] apply"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "╰") {
		t.Fatalf("expected complete bottom border in body:\n%s", body)
	}
}
