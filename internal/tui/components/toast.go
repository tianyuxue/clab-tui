// Package components holds reusable TUI widgets.
package components

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// toastExpiredMsg is emitted when a toast's auto-hide timer fires. It is
// unexported but sent through the tea loop; the owning model calls
// HandleExpired on it.
type toastExpiredMsg struct{}

// Toast is a transient status message that appears and auto-hides after a
// short duration. It is useful for non-blocking feedback (e.g. "No match
// found", "Saved") without permanently occupying layout space.
//
// Usage (in a Bubble Tea model):
//
//	toast := components.NewToast()
//
//	// show:
//	cmd := toast.Show("No match found")   // returns a tea.Cmd
//	// feed the returned cmd into tea.Batch(...)
//
//	// in Update:
//	case components.ToastExpired:
//	    toast.HandleExpired(msg)
//
// The toast renders as a single-line strip; place it where the UI needs it
// (e.g. directly above the status bar).
type Toast struct {
	message string
	visible bool
	// ExpireAfter controls the auto-hide duration. Default 3s.
	ExpireAfter time.Duration
}

// ToastExpired is the message type emitted when a toast auto-hides. Exported
// so the model can pattern-match it in Update.
type ToastExpired = toastExpiredMsg

// NewToast returns a hidden toast with a 3-second default duration.
func NewToast() *Toast {
	return &Toast{
		ExpireAfter: 3 * time.Second,
	}
}

// Show makes the toast visible with the given message and returns a tea.Cmd
// that fires toastExpiredMsg after ExpireAfter. Replace any prior timer.
func (t *Toast) Show(message string) tea.Cmd {
	t.message = message
	t.visible = true
	return t.expireCmd()
}

// Visible reports whether the toast is currently shown.
func (t *Toast) Visible() bool { return t.visible }

// Message returns the current toast text.
func (t *Toast) Message() string { return t.message }

// Hide makes the toast invisible.
func (t *Toast) Hide() {
	t.visible = false
}

// HandleExpired hides the toast in response to a toastExpiredMsg.
func (t *Toast) HandleExpired(msg tea.Msg) {
	switch msg.(type) {
	case toastExpiredMsg:
		t.visible = false
	}
}

// expireCmd returns a command that fires toastExpiredMsg after ExpireAfter.
func (t *Toast) expireCmd() tea.Cmd {
	d := t.ExpireAfter
	if d <= 0 {
		d = 3 * time.Second
	}
	return tea.Tick(d, func(time.Time) tea.Msg {
		return toastExpiredMsg{}
	})
}
