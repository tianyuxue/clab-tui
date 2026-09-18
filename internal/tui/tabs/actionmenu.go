package tabs

import tea "github.com/charmbracelet/bubbletea"

// ActionItem is a single selectable entry in an ActionMenu.
type ActionItem struct {
	Label string
	ID    string
}

// ActionSelectedMsg is emitted when the user picks an item from an ActionMenu.
type ActionSelectedMsg struct {
	Item ActionItem
}

// ActionMenu is a generic overlay action menu. It implements popup.Popup so it
// can be shown via popup.Place over the main view. Keyboard: j/k navigate,
// Enter selects (emits ActionSelectedMsg), letter keys select by label, Esc
// cancels.
type ActionMenu struct {
	sl *selectList[ActionItem]
}

func NewActionMenu() *ActionMenu {
	sl := newSelectList("", func(it ActionItem) string { return it.Label },
		func(it ActionItem) tea.Msg { return ActionSelectedMsg{Item: it} })
	sl.emptyMsg = "(no actions)"
	return &ActionMenu{sl: sl}
}

func (m *ActionMenu) Visible() bool { return m.sl.Visible() }

func (m *ActionMenu) Show(title string, items []ActionItem) {
	m.sl.title = title
	m.sl.Show(items)
}

func (m *ActionMenu) Hide() { m.sl.Hide() }

// Items returns a copy of the current menu items.
func (m *ActionMenu) Items() []ActionItem {
	out := make([]ActionItem, len(m.sl.items))
	copy(out, m.sl.items)
	return out
}

// Hidden reports whether the menu is currently hidden (inverse of Visible).
func (m *ActionMenu) Hidden() bool { return !m.sl.Visible() }

func (m *ActionMenu) SetSize(w, h int) { m.sl.SetSize(w, h) }

func (m *ActionMenu) Selected() *ActionItem {
	it, ok := m.sl.Selected()
	if !ok {
		return nil
	}
	return &it
}

func (m *ActionMenu) Update(msg tea.Msg) (*ActionMenu, tea.Cmd) {
	return m, m.sl.Update(msg)
}

func (m *ActionMenu) Body() string { return m.sl.Body() }

func (m *ActionMenu) DesiredWidth() int { return m.sl.DesiredWidth() }

func (m *ActionMenu) DesiredHeight() int { return m.sl.DesiredHeight() }

// View is kept for backwards compatibility; Body is what popup.Place uses.
func (m *ActionMenu) View() string {
	if !m.Visible() {
		return ""
	}
	return m.Body()
}
