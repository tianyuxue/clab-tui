package tabs

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKeyEncode(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyMsg
		want string
	}{
		{"rune h", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")}, "h"},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, "\r"},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, "\t"},
		{"backspace", tea.KeyMsg{Type: tea.KeyBackspace}, "\x7f"},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, "\x1b[A"},
		{"down", tea.KeyMsg{Type: tea.KeyDown}, "\x1b[B"},
		{"right", tea.KeyMsg{Type: tea.KeyRight}, "\x1b[C"},
		{"left", tea.KeyMsg{Type: tea.KeyLeft}, "\x1b[D"},
		{"alt+h", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h"), Alt: true}, "\x1bh"},
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}, "\x03"},
		{"ctrl+d", tea.KeyMsg{Type: tea.KeyCtrlD}, "\x04"},
		{"ctrl+\\", tea.KeyMsg{Type: tea.KeyCtrlBackslash}, "\x1c"},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, "\x1b"},
		{"pgup", tea.KeyMsg{Type: tea.KeyPgUp}, "\x1b[5~"},
		{"space", tea.KeyMsg{Type: tea.KeySpace}, " "},
		{"alt+right", tea.KeyMsg{Type: tea.KeyRight, Alt: true}, "\x1b\x1b[C"},
		{"alt+backspace", tea.KeyMsg{Type: tea.KeyBackspace, Alt: true}, "\x1b\x7f"},
		{"shift+tab", tea.KeyMsg{Type: tea.KeyShiftTab}, "\x1b[Z"},
		{"delete", tea.KeyMsg{Type: tea.KeyDelete}, "\x1b[3~"},
		{"insert", tea.KeyMsg{Type: tea.KeyInsert}, "\x1b[2~"},
		{"home", tea.KeyMsg{Type: tea.KeyHome}, "\x1b[H"},
		{"end", tea.KeyMsg{Type: tea.KeyEnd}, "\x1b[F"},
		{"pgdown", tea.KeyMsg{Type: tea.KeyPgDown}, "\x1b[6~"},
		{"shift+up", tea.KeyMsg{Type: tea.KeyShiftUp}, "\x1b[1;2A"},
		{"shift+down", tea.KeyMsg{Type: tea.KeyShiftDown}, "\x1b[1;2B"},
		{"shift+right", tea.KeyMsg{Type: tea.KeyShiftRight}, "\x1b[1;2C"},
		{"shift+left", tea.KeyMsg{Type: tea.KeyShiftLeft}, "\x1b[1;2D"},
		{"ctrl+up", tea.KeyMsg{Type: tea.KeyCtrlUp}, "\x1b[1;5A"},
		{"ctrl+down", tea.KeyMsg{Type: tea.KeyCtrlDown}, "\x1b[1;5B"},
		{"ctrl+right", tea.KeyMsg{Type: tea.KeyCtrlRight}, "\x1b[1;5C"},
		{"ctrl+left", tea.KeyMsg{Type: tea.KeyCtrlLeft}, "\x1b[1;5D"},
		{"ctrl+j", tea.KeyMsg{Type: tea.KeyCtrlJ}, "\x0a"},
		{"ctrl+]", tea.KeyMsg{Type: tea.KeyCtrlCloseBracket}, "\x1d"},
		{"ctrl+^", tea.KeyMsg{Type: tea.KeyCtrlCaret}, "\x1e"},
		{"ctrl+_", tea.KeyMsg{Type: tea.KeyCtrlUnderscore}, "\x1f"},
		{"ctrl+pgup", tea.KeyMsg{Type: tea.KeyCtrlPgUp}, "\x1b[5;5~"},
		{"ctrl+pgdown", tea.KeyMsg{Type: tea.KeyCtrlPgDown}, "\x1b[6;5~"},
		{"unknown f1 dropped", tea.KeyMsg{Type: tea.KeyF1}, ""},
		{"unknown type dropped", tea.KeyMsg{Type: tea.KeyType(1000)}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := KeyEncode(tc.key); got != tc.want {
				t.Fatalf("KeyEncode(%v)=%q, want %q", tc.key, got, tc.want)
			}
		})
	}
}
