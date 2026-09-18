package tabs

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// selectList is the shared engine behind letter-selectable popup lists. It
// handles pagination, letter-label assignment (reserving j/k/q), cursor
// movement, and rendering. The public popup types (ActionMenu, LabPicker,
// SessionPicker) wrap it.
type selectList[T any] struct {
	items    []T
	labels   []string
	page     int
	cursor   int
	visible  bool
	width    int
	height   int
	title    string
	display  func(T) string         // returns the item's label text (for letter assignment + highlight)
	render   func(T, string) string // optional full-line renderer receiving (item, labelLetter); nil uses highlightLabel
	emptyMsg string                 // body text when there are no items
	pageSize int
	emit     func(T) tea.Msg // returns the pick message for the given item
}

func newSelectList[T any](title string, display func(T) string, emit func(T) tea.Msg) *selectList[T] {
	return &selectList[T]{
		title:    title,
		display:  display,
		emit:     emit,
		pageSize: defaultPageSize,
	}
}

func (l *selectList[T]) Show(items []T) {
	l.items = items
	l.cursor = 0
	l.page = 0
	l.visible = true
	l.recomputeLabels()
}

func (l *selectList[T]) Hide()            { l.visible = false }
func (l *selectList[T]) Visible() bool    { return l.visible }
func (l *selectList[T]) SetSize(w, h int) { l.width = w; l.height = h }

// pageItems returns the items on the current page.
func (l *selectList[T]) pageItems() []T {
	pages := Paginate(l.items, l.pageSize)
	if l.page >= len(pages) {
		l.page = len(pages) - 1
	}
	if l.page < 0 {
		l.page = 0
	}
	return pages[l.page]
}

// recomputeLabels assigns letter labels to the current page's items. The
// navigation (j/k), page ([ / ]) and close (q) keys are reserved so they are
// never assigned as labels.
func (l *selectList[T]) recomputeLabels() {
	page := l.pageItems()
	names := make([]string, len(page))
	for i, it := range page {
		names[i] = l.display(it)
	}
	l.labels = AssignLabelsExcluded(names, 'j', 'k', 'q')
}

// Selected returns the item under the cursor, clamping the cursor to the
// page bounds. ok is false when there are no items.
func (l *selectList[T]) Selected() (T, bool) {
	page := l.pageItems()
	if len(page) == 0 {
		var zero T
		return zero, false
	}
	if l.cursor >= len(page) {
		l.cursor = len(page) - 1
	}
	return page[l.cursor], true
}

// Update handles key input while visible. Selection emits the item's pick
// message via the emit closure.
func (l *selectList[T]) Update(msg tea.Msg) tea.Cmd {
	if !l.visible {
		return nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		matched := false
		switch msg.String() {
		case "j", "down":
			if l.cursor < len(l.pageItems())-1 {
				l.cursor++
			}
			matched = true
		case "k", "up":
			if l.cursor > 0 {
				l.cursor--
			}
			matched = true
		case "enter", " ":
			page := l.pageItems()
			if l.cursor < len(page) {
				it := page[l.cursor]
				l.visible = false
				return func() tea.Msg { return l.emit(it) }
			}
		case "[":
			if l.page > 0 {
				l.page--
				l.cursor = 0
				l.recomputeLabels()
			}
			matched = true
		case "]":
			if l.page < len(Paginate(l.items, l.pageSize))-1 {
				l.page++
				l.cursor = 0
				l.recomputeLabels()
			}
			matched = true
		case "esc", "q":
			l.visible = false
			matched = true
		}
		if !matched && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				if idx, ok := LabelIndex(r, l.labels); ok {
					page := l.pageItems()
					if idx < len(page) {
						it := page[idx]
						l.visible = false
						return func() tea.Msg { return l.emit(it) }
					}
				}
				return nil
			}
		}
	}
	return nil
}

// Body renders the popup content with a border. No background is set so the
// terminal's own background color shows through.
func (l *selectList[T]) Body() string {
	pw := l.width * 2 / 3
	if pw < 30 {
		pw = 30
	}
	ph := l.height * 2 / 3
	if ph < 10 {
		ph = 10
	}

	var b strings.Builder
	if l.title != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render(l.title) + "\n\n")
	}
	page := l.pageItems()
	if len(page) == 0 && l.emptyMsg != "" {
		b.WriteString("  " + l.emptyMsg + "\n")
	}
	for i, it := range page {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
		if i == l.cursor {
			prefix = "▸ "
			style = style.Bold(true).Foreground(lipgloss.Color("212"))
		}
		var name string
		label := ""
		if i < len(l.labels) {
			label = l.labels[i]
		}
		if l.render != nil {
			name = l.render(it, label)
		} else {
			name = highlightLabel(l.display(it), label)
		}
		b.WriteString(prefix + style.Render(name) + "\n")
	}

	content := b.String()
	lines := strings.Count(content, "\n")

	// The hint must fit on one line inside the padded box, so truncate it to
	// the inner content width when it would otherwise wrap.
	hint := "[letter] select  [Enter] ok  [/] page  [Esc] close"
	if inner := pw - 4; lipgloss.Width(hint) > inner {
		hint = hint[:inner]
		if idx := strings.LastIndex(hint, " "); idx > 0 {
			hint = hint[:idx]
		}
	}

	// Height(ph) is the block height including the 2 padding rows, so the
	// content area holds ph-2 lines. Fill blanks so the hint lands on the last
	// content line, just above the bottom padding and border.
	for i := lines; i < ph-3; i++ {
		b.WriteString("\n")
	}
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(hint))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(pw).
		Height(ph)
	return box.Render(b.String())
}

// DesiredWidth reports the popup's total target width including borders
// (2/3 of screen, min 30) plus the 2 border columns.
func (l *selectList[T]) DesiredWidth() int {
	pw := l.width * 2 / 3
	if pw < 30 {
		pw = 30
	}
	return pw + 2
}

// DesiredHeight reports the popup's total target height including borders
// (2/3 of screen, min 10) plus the 2 border rows.
func (l *selectList[T]) DesiredHeight() int {
	ph := l.height * 2 / 3
	if ph < 10 {
		ph = 10
	}
	return ph + 2
}
