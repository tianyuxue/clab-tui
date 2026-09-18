package tabs

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tianyuxue/clab-tui/internal/tui/popup"
)

// MenuNode is one entry in the which-key tree. A node is either a group
// (Children non-empty, ActionID empty) or a leaf action (ActionID non-empty).
type MenuNode struct {
	Label     string     // display name
	DirectKey string     // existing single-key shortcut, shown as a hint; empty = none
	ActionID  string     // leaf action id (dispatched via ActionSelectedMsg); empty = group
	Children  []MenuNode // submenu for groups
}

// WhichKey is a lazyvim-style leader-key menu. It renders the current level's
// items in a bottom-right anchored popup; pressing a letter label selects the
// item directly. Groups descend, actions emit ActionSelectedMsg and close.
type WhichKey struct {
	stack   []wkLevel
	visible bool
}

type wkLevel struct {
	items  []MenuNode
	labels []string
	cursor int
	crumb  string // breadcrumb for this level, e.g. "SPC › Lab"
}

// NewWhichKey returns an empty which-key menu.
func NewWhichKey() *WhichKey {
	return &WhichKey{}
}

// Show opens the menu with the given root level items.
func (w *WhichKey) Show(root []MenuNode) {
	w.stack = []wkLevel{newWkLevel(root, "SPC")}
	w.visible = true
}

func newWkLevel(items []MenuNode, crumb string) wkLevel {
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Label
	}
	return wkLevel{
		items:  items,
		labels: AssignLabelsExcluded(names, 'j', 'k', 'q', ' '),
		cursor: 0,
		crumb:  crumb,
	}
}

func (w *WhichKey) Hide()                { w.visible = false }
func (w *WhichKey) Visible() bool        { return w.visible }
func (w *WhichKey) Depth() int           { return len(w.stack) }
func (w *WhichKey) Anchor() popup.Anchor { return popup.AnchorBottomRight }

// DesiredWidth/Height return 0 so popup.Place falls back to the rendered body
// size (the menu auto-sizes to its content).
func (w *WhichKey) DesiredWidth() int  { return 0 }
func (w *WhichKey) DesiredHeight() int { return 0 }

// labelsOfCurrent exposes the current level's labels for tests.
func (w *WhichKey) labelsOfCurrent() []string {
	if len(w.stack) == 0 {
		return nil
	}
	return w.stack[len(w.stack)-1].labels
}

func (w *WhichKey) current() *wkLevel {
	return &w.stack[len(w.stack)-1]
}

func (w *WhichKey) descend(it MenuNode) {
	crumb := w.current().crumb + " › " + it.Label
	w.stack = append(w.stack, newWkLevel(it.Children, crumb))
}

func (w *WhichKey) back() {
	if len(w.stack) > 1 {
		w.stack = w.stack[:len(w.stack)-1]
		return
	}
	w.visible = false
}

// Update handles keys while the menu is visible. It is modal: unmatched keys
// are ignored.
func (w *WhichKey) Update(msg tea.Msg) (*WhichKey, tea.Cmd) {
	if !w.visible {
		return w, nil
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return w, nil
	}
	lvl := w.current()
	switch km.String() {
	case "esc", "q":
		w.back()
		return w, nil
	case "j", "down":
		if lvl.cursor < len(lvl.items)-1 {
			lvl.cursor++
		}
		return w, nil
	case "k", "up":
		if lvl.cursor > 0 {
			lvl.cursor--
		}
		return w, nil
	case "enter":
		return w.trigger(lvl.cursor)
	}
	if len(km.Runes) == 1 {
		r := km.Runes[0]
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			if idx, ok := LabelIndex(r, lvl.labels); ok {
				return w.trigger(idx)
			}
		}
	}
	return w, nil
}

// trigger activates the item at idx in the current level. Groups descend;
// actions close the menu and emit ActionSelectedMsg.
func (w *WhichKey) trigger(idx int) (*WhichKey, tea.Cmd) {
	lvl := w.current()
	if idx < 0 || idx >= len(lvl.items) {
		return w, nil
	}
	it := lvl.items[idx]
	if it.ActionID == "" {
		if len(it.Children) > 0 {
			w.descend(it)
		}
		return w, nil
	}
	w.visible = false
	return w, func() tea.Msg {
		return ActionSelectedMsg{Item: ActionItem{Label: it.Label, ID: it.ActionID}}
	}
}

// Body renders the current level as a bordered, bottom-right popup. The menu
// letter is highlighted in red inside the label; the direct-key hint is shown
// right-aligned at the end of the row in cyan.
func (w *WhichKey) Body() string {
	lvl := w.current()

	type wkItem struct {
		line string
		dk   string
	}
	items := make([]wkItem, 0, len(lvl.items))
	for i, it := range lvl.items {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
		if i == lvl.cursor {
			prefix = "▸ "
			style = style.Bold(true).Foreground(lipgloss.Color("212"))
		}
		label := ""
		if i < len(lvl.labels) {
			label = lvl.labels[i]
		}
		dk := ""
		if it.DirectKey != "" {
			dk = directKeyStyle.Render(it.DirectKey)
		}
		items = append(items, wkItem{line: prefix + style.Render(highlightLabel(it.Label, label)), dk: dk})
	}

	crumb := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render(lvl.crumb)
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[letter] select  [Enter] ok  [Esc] back")

	wc := lipgloss.Width(crumb)
	for _, it := range items {
		v := lipgloss.Width(it.line)
		if it.dk != "" {
			v += 2 + lipgloss.Width(it.dk)
		}
		if v > wc {
			wc = v
		}
	}
	if v := lipgloss.Width(footer); v > wc {
		wc = v
	}

	var b strings.Builder
	b.WriteString(crumb + "\n")
	for _, it := range items {
		line := it.line
		if it.dk != "" {
			if pad := wc - lipgloss.Width(line) - lipgloss.Width(it.dk); pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			line += it.dk
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(footer)
	content := b.String()

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Width(wc + 2)
	return box.Render(content)
}

// directKeyStyle highlights the existing single-key shortcut in cyan.
var directKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Bold(true)
