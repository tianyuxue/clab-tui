package tabs

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// SessionPickedMsg is emitted when the user picks a session from the picker.
type SessionPickedMsg struct {
	Session *engine.SessionHandle
}

// SessionPicker is an overlay popup listing live sessions for quick switching.
// It implements popup.Popup.
type SessionPicker struct {
	sl *selectList[*engine.SessionHandle]
}

func NewSessionPicker() *SessionPicker {
	return &SessionPicker{
		sl: newSelectList("Sessions", func(s *engine.SessionHandle) string { return s.Title },
			func(s *engine.SessionHandle) tea.Msg { return SessionPickedMsg{Session: s} }),
	}
}

func (p *SessionPicker) Visible() bool { return p.sl.Visible() }

func (p *SessionPicker) Show(sessions []*engine.SessionHandle) {
	if len(sessions) == 0 {
		p.Hide()
		return
	}
	p.sl.Show(sessions)
}

func (p *SessionPicker) Hide() { p.sl.Hide() }

func (p *SessionPicker) SetSize(w, h int) { p.sl.SetSize(w, h) }

func (p *SessionPicker) Update(msg tea.Msg) (*SessionPicker, tea.Cmd) {
	return p, p.sl.Update(msg)
}

func (p *SessionPicker) Body() string { return p.sl.Body() }

func (p *SessionPicker) DesiredWidth() int { return p.sl.DesiredWidth() }

func (p *SessionPicker) DesiredHeight() int { return p.sl.DesiredHeight() }

// View is kept for backwards compatibility; Body is what popup.Place uses.
func (p *SessionPicker) View() string {
	if !p.Visible() {
		return ""
	}
	return p.Body()
}
