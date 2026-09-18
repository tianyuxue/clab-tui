package tabs

import (
	tea "github.com/charmbracelet/bubbletea"
)

// KeyEncode converts a tea.KeyMsg into the byte sequence a terminal program
// expects. An Alt-modifier is emitted as an ESC prefix in front of the base
// encoding. Unknown keys are dropped (returned as "") rather than typed
// literally into the shell: corrupted input is worse than nothing.
func KeyEncode(k tea.KeyMsg) string {
	base := encodeBase(k)
	if k.Alt && base != "" {
		return "\x1b" + base
	}
	return base
}

// encodeBase encodes a tea.KeyMsg without its Alt modifier.
func encodeBase(k tea.KeyMsg) string {
	if k.Type == tea.KeyRunes && len(k.Runes) > 0 {
		return string(k.Runes)
	}
	switch k.Type {
	case tea.KeySpace:
		return " "
	case tea.KeyEnter:
		return "\r"
	case tea.KeyTab:
		return "\t"
	case tea.KeyShiftTab:
		return "\x1b[Z"
	case tea.KeyBackspace:
		return "\x7f"
	case tea.KeyDelete:
		return "\x1b[3~"
	case tea.KeyInsert:
		return "\x1b[2~"
	case tea.KeyHome:
		return "\x1b[H"
	case tea.KeyEnd:
		return "\x1b[F"
	case tea.KeyPgUp:
		return "\x1b[5~"
	case tea.KeyPgDown:
		return "\x1b[6~"
	case tea.KeyCtrlPgUp:
		return "\x1b[5;5~"
	case tea.KeyCtrlPgDown:
		return "\x1b[6;5~"
	case tea.KeyCtrlHome:
		return "\x1b[1;5H"
	case tea.KeyCtrlEnd:
		return "\x1b[1;5F"
	case tea.KeyUp:
		return "\x1b[A"
	case tea.KeyDown:
		return "\x1b[B"
	case tea.KeyRight:
		return "\x1b[C"
	case tea.KeyLeft:
		return "\x1b[D"
	case tea.KeyShiftUp:
		return "\x1b[1;2A"
	case tea.KeyShiftDown:
		return "\x1b[1;2B"
	case tea.KeyShiftRight:
		return "\x1b[1;2C"
	case tea.KeyShiftLeft:
		return "\x1b[1;2D"
	case tea.KeyCtrlUp:
		return "\x1b[1;5A"
	case tea.KeyCtrlDown:
		return "\x1b[1;5B"
	case tea.KeyCtrlRight:
		return "\x1b[1;5C"
	case tea.KeyCtrlLeft:
		return "\x1b[1;5D"
	case tea.KeyEsc:
		return "\x1b"
	case tea.KeyCtrlA:
		return "\x01"
	case tea.KeyCtrlB:
		return "\x02"
	case tea.KeyCtrlC:
		return "\x03"
	case tea.KeyCtrlD:
		return "\x04"
	case tea.KeyCtrlE:
		return "\x05"
	case tea.KeyCtrlF:
		return "\x06"
	case tea.KeyCtrlG:
		return "\x07"
	case tea.KeyCtrlH:
		return "\x08"
	case tea.KeyCtrlJ:
		return "\x0a"
	case tea.KeyCtrlK:
		return "\x0b"
	case tea.KeyCtrlL:
		return "\x0c"
	case tea.KeyCtrlN:
		return "\x0e"
	case tea.KeyCtrlO:
		return "\x0f"
	case tea.KeyCtrlP:
		return "\x10"
	case tea.KeyCtrlQ:
		return "\x11"
	case tea.KeyCtrlR:
		return "\x12"
	case tea.KeyCtrlS:
		return "\x13"
	case tea.KeyCtrlT:
		return "\x14"
	case tea.KeyCtrlU:
		return "\x15"
	case tea.KeyCtrlV:
		return "\x16"
	case tea.KeyCtrlW:
		return "\x17"
	case tea.KeyCtrlX:
		return "\x18"
	case tea.KeyCtrlY:
		return "\x19"
	case tea.KeyCtrlZ:
		return "\x1a"
	case tea.KeyCtrlBackslash:
		return "\x1c"
	case tea.KeyCtrlCloseBracket:
		return "\x1d"
	case tea.KeyCtrlCaret:
		return "\x1e"
	case tea.KeyCtrlUnderscore:
		return "\x1f"
	default:
		// Unknown keys are dropped rather than typed literally.
		return ""
	}
}
