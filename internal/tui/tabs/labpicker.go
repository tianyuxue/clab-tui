package tabs

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// LabPickedMsg is emitted when the user selects a lab from the overlay.
type LabPickedMsg struct {
	Lab *engine.Lab
}

// LabPicker is an overlay popup that lists labs for switching. It implements
// popup.Popup so any overlay host can render it.
type LabPicker struct {
	sl *selectList[*engine.Lab]
}

func NewLabPicker() *LabPicker {
	sl := newSelectList("Select Lab", func(l *engine.Lab) string { return l.Name },
		func(l *engine.Lab) tea.Msg { return LabPickedMsg{Lab: l} })
	sl.render = func(l *engine.Lab, label string) string {
		statusChar, color := labStatusIndicator(l)
		status := lipgloss.NewStyle().Foreground(color).Render(statusChar)
		return status + " " + highlightLabel(l.Name, label) + " " + fmt.Sprintf("%d nodes", len(l.Nodes))
	}
	return &LabPicker{sl: sl}
}

// labStatusIndicator returns the status glyph and color for a lab.
func labStatusIndicator(l *engine.Lab) (string, lipgloss.Color) {
	switch l.Status() {
	case engine.LabStatusRunning:
		return "●", lipgloss.Color("42")
	case engine.LabStatusStopped:
		return "●", lipgloss.Color("196")
	case engine.LabStatusPartial:
		return "◐", lipgloss.Color("214")
	default:
		return "○", lipgloss.Color("240")
	}
}

func (p *LabPicker) Visible() bool { return p.sl.Visible() }

func (p *LabPicker) Show(labs []*engine.Lab) { p.sl.Show(labs) }

func (p *LabPicker) Hide() { p.sl.Hide() }

func (p *LabPicker) SetSize(w, h int) { p.sl.SetSize(w, h) }

func (p *LabPicker) Selected() *engine.Lab {
	lab, ok := p.sl.Selected()
	if !ok {
		return nil
	}
	return lab
}

func (p *LabPicker) Update(msg tea.Msg) (*LabPicker, tea.Cmd) {
	return p, p.sl.Update(msg)
}

func (p *LabPicker) Body() string { return p.sl.Body() }

func (p *LabPicker) DesiredWidth() int { return p.sl.DesiredWidth() }

func (p *LabPicker) DesiredHeight() int { return p.sl.DesiredHeight() }

// View is kept for backwards compatibility; Body is what popup.Place uses.
func (p *LabPicker) View() string {
	if !p.Visible() {
		return ""
	}
	return p.Body()
}
