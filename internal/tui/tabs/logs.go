package tabs

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type LogModel struct {
	lines  []string
	vp     viewport.Model
	width  int
	height int
	follow bool
	label  string
}

func NewLogs() *LogModel {
	return &LogModel{
		follow: true,
		vp:     viewport.New(80, 20),
	}
}

func (m *LogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.vp.Width = w
	m.vp.Height = h
}

func (m *LogModel) AppendLine(line string) {
	m.lines = append(m.lines, line)
	if len(m.lines) > 10000 {
		m.lines = m.lines[len(m.lines)-5000:]
	}
	if m.follow {
		m.vp.GotoBottom()
	}
	m.vp.SetContent(strings.Join(m.lines, "\n"))
}

func (m *LogModel) SetLabel(label string) { m.label = label }

func (m *LogModel) Label() string { return m.label }

func (m *LogModel) ToggleFollow() { m.follow = !m.follow }

func (m *LogModel) Following() bool { return m.follow }

func (m *LogModel) GotoTop() { m.vp.GotoTop() }

func (m *LogModel) GotoBottom() { m.vp.GotoBottom() }

func (m *LogModel) Clear() {
	m.lines = nil
	m.vp.SetContent("")
}

func (m *LogModel) Update(msg tea.Msg) (*LogModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "f":
			m.follow = !m.follow
		case "c":
			m.Clear()
		case "g":
			m.vp.GotoTop()
		case "G":
			m.vp.GotoBottom()
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m *LogModel) View() string {
	if len(m.lines) == 0 {
		return padToHeight("No logs. Select a container and press 'l' to view logs.", m.height)
	}
	headerRows := 0
	header := ""
	if m.label != "" {
		headerRows = 1
		header = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render(m.label) + "\n"
	}
	followLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[f] follow: ON")
	copyRight := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  [c] clear  [g] top  [G] bottom")
	if !m.follow {
		followLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("[f] follow: OFF")
	}
	// Reserve the header and footer rows so the viewport fits exactly within
	// the content area; otherwise the frame overflows the terminal and the
	// top tab bar scrolls off-screen.
	m.vp.Height = m.height - headerRows - 1
	return header + m.vp.View() + "\n" + followLabel + copyRight
}
