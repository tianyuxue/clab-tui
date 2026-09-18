package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Quit     key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	Help     key.Binding
	Search   key.Binding
}

var Keys = keyMap{
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Tab:      key.NewBinding(key.WithKeys("tab", "right"), key.WithHelp("tab", "next tab")),
	ShiftTab: key.NewBinding(key.WithKeys("shift+tab", "left"), key.WithHelp("shift+tab", "prev tab")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Search:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
}

// spaceMenu is the single global shortcut hint for opening the which-key menu.
var spaceMenu = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "menu"))

// SessionInsertHelp returns the shortcut hints shown on the sessions tab while
// a live session is in insert mode, where every key is shell input.
func SessionInsertHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("ctrl+\\"), key.WithHelp("ctrl+\\", "to normal")),
	}
}
