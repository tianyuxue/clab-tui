package tabs

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

type NetemCommitMsg struct{ State engine.NetemState }

type NetemForm struct {
	values  [5]string
	cursor  int
	visible bool
	width   int
	height  int
}

func NewNetemForm() *NetemForm        { return &NetemForm{} }
func (f *NetemForm) Visible() bool    { return f.visible }
func (f *NetemForm) SetSize(w, h int) { f.width, f.height = w, h }

func (f *NetemForm) Show() {
	f.values = [5]string{}
	f.cursor = 0
	f.visible = true
}

func (f *NetemForm) Hide() { f.visible = false }

func (f *NetemForm) Update(msg tea.Msg) (*NetemForm, tea.Cmd) {
	if !f.visible {
		return f, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}
	switch key.Type {
	case tea.KeyEscape:
		f.visible = false
	case tea.KeyEnter:
		f.visible = false
		return f, func() tea.Msg { return NetemCommitMsg{State: f.state()} }
	case tea.KeyTab, tea.KeyDown:
		f.cursor = (f.cursor + 1) % len(f.values)
	case tea.KeyShiftTab, tea.KeyUp:
		f.cursor = (f.cursor + len(f.values) - 1) % len(f.values)
	case tea.KeyBackspace:
		if len(f.values[f.cursor]) > 0 {
			r := []rune(f.values[f.cursor])
			f.values[f.cursor] = string(r[:len(r)-1])
		}
	case tea.KeyRunes:
		f.values[f.cursor] += string(key.Runes)
	case tea.KeySpace:
		f.values[f.cursor] += " "
	}
	return f, nil
}

func (f *NetemForm) state() engine.NetemState {
	return engine.NetemState{Delay: f.values[0], Jitter: f.values[1], Loss: f.values[2], Rate: f.values[3], Corruption: f.values[4]}
}

func (f *NetemForm) Body() string {
	labels := []string{"Delay", "Jitter", "Loss", "Rate", "Corruption"}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render("Set Network Emulation") + "\n\n")
	for i, label := range labels {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
		if i == f.cursor {
			prefix = "▸ "
			style = style.Bold(true).Foreground(lipgloss.Color("212"))
		}
		value := f.values[i] + "▌"
		b.WriteString(prefix + style.Render(label+": "+value) + "\n")
	}
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[Tab/↑↓] field  [Enter] apply  [Esc] cancel"))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(56).
		Height(10).
		Render(b.String())
}

func (f *NetemForm) DesiredWidth() int  { return 58 }
func (f *NetemForm) DesiredHeight() int { return 14 }
