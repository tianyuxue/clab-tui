package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

type StatusBar struct {
	LabCount        int
	RunningCount    int
	BackendStatus   string
	ContainerlabVer string
	width           int
}

func NewStatusBar() *StatusBar {
	return &StatusBar{
		BackendStatus: "OK",
	}
}

func (s *StatusBar) SetSize(w int) {
	s.width = w
}

func (s *StatusBar) View() string {
	if s.width == 0 {
		s.width = 80
	}

	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("254")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)

	left := fmt.Sprintf(" labs: %d ", s.LabCount)

	statusColor := lipgloss.Color("42")
	if s.BackendStatus != "OK" {
		statusColor = lipgloss.Color("196")
	}
	status := lipgloss.NewStyle().Foreground(statusColor).Render("\u25cf")
	center := fmt.Sprintf(" %s %s ", status, s.BackendStatus)

	right := ""
	if s.ContainerlabVer != "" {
		right = fmt.Sprintf(" clab %s ", s.ContainerlabVer)
	} else {
		right = fmt.Sprintf(" running: %d ", s.RunningCount)
	}

	// Account for the 2 chars added by Padding(0,1).
	inner := s.width - 2
	spacer := inner - lipgloss.Width(left) - lipgloss.Width(center) - lipgloss.Width(right)
	if spacer < 1 {
		spacer = 1
	}

	return style.Render(left + center + fmt.Sprintf("%*s", spacer, "") + right)
}
