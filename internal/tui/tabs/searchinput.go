package tabs

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SearchCommitMsg is emitted when the user commits a search term (Enter).
type SearchCommitMsg struct {
	Term string
}

// SearchInput is a vim-style search prompt overlay. It implements popup.Popup.
// The user types a term, Enter commits (emits SearchCommitMsg), Esc cancels.
type SearchInput struct {
	value        string
	cursor       int
	visible      bool
	customFilter bool
	captureFile  bool
	width        int
	height       int
}

func NewSearchInput() *SearchInput {
	return &SearchInput{}
}

func (s *SearchInput) Visible() bool { return s.visible }

func (s *SearchInput) Show() {
	s.value = ""
	s.cursor = 0
	s.visible = true
	s.customFilter = false
	s.captureFile = false
}

// ShowCustomFilter opens the input with libpcap/tcpdump filter guidance.
func (s *SearchInput) ShowCustomFilter() {
	s.value = ""
	s.cursor = 0
	s.visible = true
	s.customFilter = true
	s.captureFile = false
}

// ShowCaptureFile opens the input for a pcap output path.
func (s *SearchInput) ShowCaptureFile() {
	s.value = ""
	s.cursor = 0
	s.visible = true
	s.customFilter = false
	s.captureFile = true
}

func (s *SearchInput) Hide() { s.visible = false }

func (s *SearchInput) SetSize(w, h int) { s.width = w; s.height = h }

func (s *SearchInput) Value() string { return s.value }

// InlineText renders the normal search value for the topology footer.
func (s *SearchInput) InlineText() string {
	value := []rune(s.value)
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor > len(value) {
		s.cursor = len(value)
	}
	return "/" + string(value[:s.cursor]) + "▌" + string(value[s.cursor:])
}

func (s *SearchInput) Update(msg tea.Msg) (*SearchInput, tea.Cmd) {
	if !s.visible {
		return s, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			s.visible = false
			term := s.value
			return s, func() tea.Msg { return SearchCommitMsg{Term: term} }
		case tea.KeyBackspace:
			if s.cursor > 0 {
				r := []rune(s.value)
				r = append(r[:s.cursor-1], r[s.cursor:]...)
				s.value = string(r)
				s.cursor--
			}
		case tea.KeyDelete:
			r := []rune(s.value)
			if s.cursor < len(r) {
				r = append(r[:s.cursor], r[s.cursor+1:]...)
				s.value = string(r)
			}
		case tea.KeyLeft:
			if s.cursor > 0 {
				s.cursor--
			}
		case tea.KeyRight:
			if s.cursor < len([]rune(s.value)) {
				s.cursor++
			}
		case tea.KeyHome:
			s.cursor = 0
		case tea.KeyEnd:
			s.cursor = len([]rune(s.value))
		case tea.KeyEscape:
			s.visible = false
		case tea.KeySpace:
			s.insert([]rune{' '})
		case tea.KeyRunes:
			s.insert(msg.Runes)
		}
	}
	return s, nil
}

func (s *SearchInput) insert(runes []rune) {
	if len(runes) == 0 {
		return
	}
	value := []rune(s.value)
	value = append(value[:s.cursor], append(runes, value[s.cursor:]...)...)
	s.value = string(value)
	s.cursor += len(runes)
}

// Body renders the search prompt with a border.
func (s *SearchInput) Body() string {
	pw := s.width * 2 / 3
	if pw < 40 {
		pw = 40
	}

	labelText := "Search"
	height := 7
	var description string
	if s.captureFile {
		labelText = "Save Capture"
		height = 9
		description = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(
			"Write matching packets to a pcap file.\n" +
				"The file is replaced when capture restarts.")
	} else if s.customFilter {
		labelText = "Custom Packet Filter"
		height = 12
		description = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(
			"Applied using libpcap tcpdump syntax.\n" +
				"Examples: host 10.0.0.1, src host 10.0.0.2, tcp port 179\n" +
				"Reference: https://www.tcpdump.org/manpages/pcap-filter.7.html")
	}
	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render(labelText)
	var b strings.Builder
	b.WriteString(label + "\n\n")
	if description != "" {
		b.WriteString(description + "\n\n")
	}
	prompt := "/ "
	action := "[Enter] search  [Esc] cancel"
	if s.captureFile {
		prompt = "File path: "
		action = "[Enter] save  [Esc] cancel"
	} else if s.customFilter {
		prompt = "Filter: "
		action = "[Enter] apply  [Esc] cancel"
	}
	value := []rune(s.value)
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor > len(value) {
		s.cursor = len(value)
	}
	valueWithCursor := string(value[:s.cursor]) + "▌" + string(value[s.cursor:])
	b.WriteString("  " + prompt + lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(valueWithCursor) + "\n")
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(action))

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(pw).
		Height(height)
	return style.Render(b.String())
}

// DesiredWidth reports the total width including borders (2/3 of screen).
func (s *SearchInput) DesiredWidth() int {
	pw := s.width * 2 / 3
	if pw < 40 {
		pw = 40
	}
	return pw + 2
}

// DesiredHeight reports the total height including borders.
func (s *SearchInput) DesiredHeight() int {
	if s.customFilter {
		return 12 + 2
	}
	if s.captureFile {
		return 9 + 2
	}
	return 7 + 2
}

// View is kept for backwards compatibility; Body is what popup.Place uses.
func (s *SearchInput) View() string {
	if !s.visible {
		return ""
	}
	return s.Body()
}
