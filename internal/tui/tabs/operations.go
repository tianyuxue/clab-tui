package tabs

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

type OpsModel struct {
	lines        []string
	vp           viewport.Model
	width        int
	height       int
	autoFollow   bool
	commandLabel string
}

func NewOps() *OpsModel {
	return &OpsModel{
		autoFollow: true,
		vp:         viewport.New(80, 20),
	}
}

func (m *OpsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.vp.Width = w
	m.vp.Height = h
}

func (m *OpsModel) AppendLine(line engine.OutputLine) {
	if line.Done {
		m.lines = append(m.lines,
			lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("--- DONE ---"))
	} else {
		style := lipgloss.NewStyle()
		if line.Stream == "stderr" {
			style = style.Foreground(lipgloss.Color("214"))
		}
		m.lines = append(m.lines, style.Render(line.Line))
	}
	if m.autoFollow {
		m.vp.GotoBottom()
	}
	m.vp.SetContent(strings.Join(m.lines, "\n"))
}

func (m *OpsModel) SetLabel(label string) {
	m.commandLabel = label
}

func (m *OpsModel) ToggleAutoFollow() { m.autoFollow = !m.autoFollow }

func (m *OpsModel) Following() bool { return m.autoFollow }

func (m *OpsModel) Clear() {
	m.lines = nil
	m.vp.SetContent("")
}

func (m *OpsModel) Update(msg tea.Msg) (*OpsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "f":
			m.autoFollow = !m.autoFollow
		case "c":
			m.Clear()
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m *OpsModel) View() string {
	if len(m.lines) == 0 {
		return padToHeight("No operations running. Select a lab and press 'd' to deploy.", m.height)
	}
	headerRows := 0
	header := ""
	if m.commandLabel != "" {
		headerRows = 1
		header = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render(m.commandLabel) + "\n"
	}
	followLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[f] follow: ON")
	copyRight := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  [c] clear")
	if !m.autoFollow {
		followLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("[f] follow: OFF")
	}
	// Reserve the header and footer rows so the viewport fits exactly within
	// the content area; otherwise the frame overflows the terminal and the
	// top tab bar scrolls off-screen.
	m.vp.Height = m.height - headerRows - 1
	return header + m.vp.View() + "\n" + followLabel + copyRight
}
