package tabs

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfirmResultMsg carries the result of a confirmation prompt.
type ConfirmResultMsg struct {
	Confirmed bool
	ActionID  string
}

const (
	confirmWidth  = 40
	confirmHeight = 10
)

// Confirm is a generic confirmation popup. Selection toggles between
// "confirm" and "cancel" with j/k; Enter confirms, Esc cancels.
type Confirm struct {
	title   string
	prompt  string
	action  string
	cursor  int // 0=confirm, 1=cancel
	visible bool
	width   int
	height  int
}

func NewConfirm() *Confirm {
	return &Confirm{}
}

func (c *Confirm) Visible() bool { return c.visible }

func (c *Confirm) Show(title, prompt, action string) {
	c.title = title
	c.prompt = prompt
	c.action = action
	c.cursor = 0
	c.visible = true
}

func (c *Confirm) Hide() { c.visible = false }

// Selected returns "confirm" or "cancel".
func (c *Confirm) Selected() string {
	if c.cursor == 1 {
		return "cancel"
	}
	return "confirm"
}

func (c *Confirm) SetSize(w, h int) { c.width = w; c.height = h }

func (c *Confirm) Update(msg tea.Msg) (*Confirm, tea.Cmd) {
	if !c.visible {
		return c, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if c.cursor < 1 {
				c.cursor++
			}
		case "k", "up":
			if c.cursor > 0 {
				c.cursor--
			}
		case "enter", " ":
			c.visible = false
			return c, func() tea.Msg {
				return ConfirmResultMsg{Confirmed: c.cursor == 0, ActionID: c.action}
			}
		case "esc", "q":
			c.visible = false
			return c, func() tea.Msg {
				return ConfirmResultMsg{Confirmed: false, ActionID: c.action}
			}
		}
	}
	return c, nil
}

func (c *Confirm) Body() string {
	pw := confirmWidth
	ph := confirmHeight
	var b strings.Builder
	if c.title != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render(c.title) + "\n\n")
	}
	b.WriteString(c.prompt + "\n\n")
	confirmLabel := "  confirm"
	confirmStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	cancelLabel := "  cancel"
	cancelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	if c.cursor == 0 {
		confirmLabel = "▸ confirm"
		confirmStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	} else {
		cancelLabel = "▸ cancel"
		cancelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	}
	b.WriteString(confirmStyle.Render(confirmLabel) + "\n")
	b.WriteString(cancelStyle.Render(cancelLabel) + "\n")

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(pw).
		Height(ph)
	return style.Render(b.String())
}

func (c *Confirm) DesiredWidth() int  { return confirmWidth + 2 }
func (c *Confirm) DesiredHeight() int { return confirmHeight + 2 }

// View is kept for backwards compatibility; Body is what popup.Place uses.
func (c *Confirm) View() string {
	if !c.visible {
		return ""
	}
	return c.Body()
}
