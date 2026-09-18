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
	if label == "" {
		return name
	}
	for i, r := range name {
		if string(unicode.ToLower(r)) == label {
			return name[:i] + keyLabelStyle.Render(string(r)) + name[i+len(string(r)):]
		}
	}
	return name
}
