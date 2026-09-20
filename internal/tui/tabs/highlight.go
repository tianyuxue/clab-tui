package tabs

import (
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// keyLabelStyle highlights the letter used to select an item (btop-style red).
var keyLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

// highlightLabel renders name with the given label letter highlighted in red.
// If label is empty or not found, name is returned unchanged.
func highlightLabel(name, label string) string {
	before, letter, after := splitLabel(name, label)
	if letter == "" {
		return name
	}
	return before + keyLabelStyle.Render(letter) + after
}

// splitLabel splits name around the first case-insensitive occurrence of the
// label letter. When the label is absent it returns (name, "", ""). Callers use
// it to style the label text and the surrounding text separately, avoiding the
// nested-ANSI-reset problem that would otherwise drop the surrounding style.
func splitLabel(name, label string) (string, string, string) {
	if label == "" {
		return name, "", ""
	}
	for i, r := range name {
		if string(unicode.ToLower(r)) == label {
			s := string(r)
			return name[:i], s, name[i+len(s):]
		}
	}
	return name, "", ""
}
