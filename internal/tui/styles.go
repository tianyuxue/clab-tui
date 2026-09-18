package tui

import "github.com/charmbracelet/lipgloss"

var (
	AppStyle = lipgloss.NewStyle().Margin(0)

	TabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("240"))

	ActiveTabStyle = TabStyle.Copy().
			Foreground(lipgloss.Color("212")).
			Bold(true)

	Green  = lipgloss.Color("42")
	Red    = lipgloss.Color("196")
	Yellow = lipgloss.Color("214")
	Dim    = lipgloss.Color("240")

	// KeyLabelStyle highlights shortcut keys in bright red (btop style).
	KeyLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)

	TopologyPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62"))

	TabNames = []string{"Topology", "Sessions", "Node Logs", "Ops Log"}
)
