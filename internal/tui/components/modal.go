package components

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("62")).
	Padding(1, 2).
	Background(lipgloss.Color("235"))

type Modal struct {
	Title     string
	Message   string
	Visible   bool
	OnConfirm func() tea.Cmd
	OnCancel  func() tea.Cmd
}

func NewModal(title, msg string) *Modal {
	return &Modal{
		Title:   title,
		Message: msg,
		Visible: false,
	}
}

func (m *Modal) SetSize(w, h int) {}

func (m *Modal) Show() {
	m.Visible = true
}

func (m *Modal) Hide() {
	m.Visible = false
}

func (m *Modal) Update(msg tea.Msg) (*Modal, tea.Cmd) {
	if !m.Visible {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "y":
			m.Visible = false
			if m.OnConfirm != nil {
				return m, m.OnConfirm()
			}
			return m, nil
		case "esc", "n", "q":
			m.Visible = false
			if m.OnCancel != nil {
				return m, m.OnCancel()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m *Modal) View() string {
	if !m.Visible {
		return ""
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Render(m.Title),
		"",
		m.Message,
		"",
		"[Enter] Confirm  [Esc] Cancel",
	)
	return modalStyle.Render(content)
}
