package tabs

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hinshun/vt10x"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// SessionMode is the interaction mode of the sessions tab (vim-style).
type SessionMode int

const (
	SessionInsert SessionMode = iota // keys go to the shell
	SessionNormal                    // keys are TUI shortcuts
)

// SessionEntry is one live session with its terminal emulator.
type SessionEntry struct {
	Handle  *engine.SessionHandle
	term    vt10x.Terminal
	pending [][]byte // output buffered until the terminal is sized
}

// SessionModel manages multiple live sessions. Each session is rendered via a
// vt10x terminal emulator fed by the engine's raw output stream.
type SessionModel struct {
	sessions []*SessionEntry
	active   int
	width    int
	height   int
	mode     SessionMode
	vp       viewport.Model
}

func NewSessionModel() *SessionModel {
	return &SessionModel{
		mode: SessionInsert,
		vp:   viewport.New(80, 20),
	}
}

func (m *SessionModel) Mode() SessionMode        { return m.mode }
func (m *SessionModel) SetMode(mode SessionMode) { m.mode = mode }
func (m *SessionModel) IsInsert() bool           { return m.mode == SessionInsert }

func (m *SessionModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.vp.Width = w
	m.vp.Height = h - 1 // reserve session bar
	if a := m.Active(); a != nil && a.term != nil {
		a.term.Resize(w, h-1)
		if a.Handle.Resize != nil {
			_ = a.Handle.Resize(w, h-1)
		}
		m.flushPending(a)
	}
}

func (m *SessionModel) Sessions() []*SessionEntry { return m.sessions }

func (m *SessionModel) Active() *SessionEntry {
	if len(m.sessions) == 0 {
		return nil
	}
	if m.active >= len(m.sessions) {
		m.active = len(m.sessions) - 1
	}
	return m.sessions[m.active]
}

func (m *SessionModel) AddSession(h *engine.SessionHandle) {
	m.sessions = append(m.sessions, &SessionEntry{
		Handle: h,
		term:   vt10x.New(vt10x.WithSize(m.width, m.height-1)),
	})
	m.active = len(m.sessions) - 1
	if h.Resize != nil {
		_ = h.Resize(m.width, m.height-1)
	}
}

func (m *SessionModel) RemoveSession(id string) bool {
	for i, s := range m.sessions {
		if s.Handle.ID == id {
			m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
			if m.active >= len(m.sessions) {
				m.active = len(m.sessions) - 1
			}
			if m.active < 0 {
				m.active = 0
			}
			return true
		}
	}
	return false
}

// SessionLabels returns the letter label for each live session (normal mode
// quick-switch). Letters are first-letter-deduped, skipping reserved keys.
// 's' is reserved too: in normal mode it opens the session picker.
func (m *SessionModel) SessionLabels() []string {
	titles := make([]string, len(m.sessions))
	for i, s := range m.sessions {
		titles[i] = s.Handle.Title
	}
	return AssignLabelsExcluded(titles, 'j', 'k', 'q', 's')
}

// FocusSession makes the session with the given ID active (no-op if unknown).
func (m *SessionModel) FocusSession(id string) {
	for i, s := range m.sessions {
		if s.Handle.ID == id {
			m.active = i
			m.resizeActive()
			return
		}
	}
}

// SessionsHandles returns all session handles (for the picker overlay).
func (m *SessionModel) SessionsHandles() []*engine.SessionHandle {
	out := make([]*engine.SessionHandle, len(m.sessions))
	for i, s := range m.sessions {
		out[i] = s.Handle
	}
	return out
}

func (m *SessionModel) resizeActive() {
	if a := m.Active(); a != nil {
		if a.term != nil {
			a.term.Resize(m.width, m.height-1)
		}
		if a.Handle.Resize != nil {
			_ = a.Handle.Resize(m.width, m.height-1)
		}
		m.flushPending(a)
	}
}

// Feed writes raw PTY bytes into the session's terminal emulator.
//
// vt10x panics when written before the terminal has a valid size (its resize
// silently refuses cols/rows < 1, leaving its internal screen nil). Feed
// buffers output until SetSize/resizeActive sizes the terminal, then flushes.
//
// Note: vt10x Write drops a trailing partial UTF-8 byte when a multi-byte
// rune is split across Write calls (see vt10x.Write). A per-session tail
// buffer to carry the split byte over is a follow-up (Task 4); latent today.
func (m *SessionModel) Feed(id string, data []byte) {
	for _, s := range m.sessions {
		if s.Handle.ID == id {
			if s.term == nil {
				return
			}
			cols, rows := s.term.Size()
			if cols < 1 || rows < 1 {
				if len(s.pending) > 100 {
					s.pending = s.pending[len(s.pending)-100:]
				}
				s.pending = append(s.pending, data)
				return
			}
			_, _ = s.term.Write(data)
			return
		}
	}
}

// flushPending writes any output buffered while the terminal was unsized.
func (m *SessionModel) flushPending(s *SessionEntry) {
	if s.term == nil || len(s.pending) == 0 {
		return
	}
	cols, rows := s.term.Size()
	if cols < 1 || rows < 1 {
		return
	}
	for _, data := range s.pending {
		_, _ = s.term.Write(data)
	}
	s.pending = nil
}

func (m *SessionModel) Update(msg tea.Msg) (*SessionModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.mode {
		case SessionInsert:
			// All keys go to the shell as encoded bytes. Mode switching is
			// handled at the model level (ctrl+\ then n).
			if a := m.Active(); a != nil && a.Handle.Stdin != nil {
				_, _ = a.Handle.Stdin.Write([]byte(KeyEncode(msg)))
			}
		case SessionNormal:
			s := msg.String()
			if s == "enter" || s == " " {
				m.mode = SessionInsert
				return m, nil
			}
			if s == "q" {
				if a := m.Active(); a != nil {
					if a.Handle.Close != nil {
						_ = a.Handle.Close()
					}
					m.RemoveSession(a.Handle.ID)
				}
				return m, nil
			}
			if len(msg.Runes) == 1 {
				r := msg.Runes[0]
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
					if idx, ok := LabelIndex(r, m.SessionLabels()); ok && idx < len(m.sessions) {
						m.active = idx
						m.resizeActive()
						return m, nil
					}
					// Unmatched letter: fall through to the viewport so
					// j/k keep scrolling the terminal output.
				}
			}
			var vpCmd tea.Cmd
			m.vp, vpCmd = m.vp.Update(msg)
			return m, vpCmd
		}
	}
	return m, nil
}

func (m *SessionModel) HasSessions() bool { return len(m.sessions) > 0 }

// View renders the session bar and the active session's terminal screen.
func (m *SessionModel) View() string {
	if len(m.sessions) == 0 {
		return padToHeight("No active sessions. Press 'a' on a node → SSH.", m.height)
	}

	var bar strings.Builder
	labels := m.SessionLabels()
	for i, s := range m.sessions {
		title := s.Handle.Title
		label := ""
		if i < len(labels) {
			label = labels[i]
		}
		display := highlightLabel(title, label)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		if i == m.active {
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
		}
		bar.WriteString(style.Render(display) + "  ")
	}
	modeHint := "INSERT"
	if m.mode == SessionNormal {
		modeHint = "NORMAL"
	}
	bar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  [" + modeHint + "]"))
	bar.WriteString("\n")

	active := m.Active()
	if active == nil || active.term == nil {
		return bar.String() + padToHeight("", m.height-1)
	}
	cols, rows := active.term.Size()

	// 会话终端光标位置（0-indexed）。insert 模式且光标可见时，在终端行内
	// 用反转色渲染光标单元格；不再输出 ANSI 光标定位（ESC[row;colH）——
	// 那会与 bubbletea 的逐行 diff 渲染冲突，把后续 status/help 行写错位置。
	var curX, curY int
	showBlockCursor := m.mode == SessionInsert && active.term.CursorVisible()
	if showBlockCursor {
		c := active.term.Cursor()
		curX, curY = c.X, c.Y
	}

	var sb strings.Builder
	for y := 0; y < rows; y++ {
		line := renderTermRow(active.term, y, cols)
		if showBlockCursor && y == curY && curX >= 0 && curX < cols {
			line = renderTermRowWithCursor(active.term, y, cols, curX)
		}
		sb.WriteString(line)
		sb.WriteString("\x1b[0m") // reset attributes after each row
		sb.WriteString("\n")
	}
	content := strings.TrimRight(sb.String(), "\n")
	m.vp.SetContent(content)
	m.vp.GotoBottom()
	view := bar.String() + m.vp.View()

	// 隐藏终端真实光标：用反转块光标替代，避免它和 TUI 文本混在一起。
	if m.mode == SessionInsert {
		if active.term.CursorVisible() {
			view += "\x1b[?25l"
		} else {
			view += "\x1b[?25l"
		}
	}
	return view
}

// renderTermRow renders one vt10x row as an ANSI string, changing colors only
// when they differ from the previous cell.
//
// Glyph.Mode fidelity (bold/underline/italic/etc.) is not rendered: vt10x
// stores these in an unexported attr bitmask with no public constants, so
// mapping them to SGR attributes would require hardcoding internal bits.
// Mode fidelity is a follow-up.
func renderTermRow(t vt10x.View, y, cols int) string {
	var sb strings.Builder
	var curFg, curBg vt10x.Color = vt10x.DefaultFG, vt10x.DefaultBG
	for x := 0; x < cols; x++ {
		g := t.Cell(x, y)
		ch := g.Char
		if ch == 0 {
			ch = ' '
		}
		fg, bg := g.FG, g.BG
		if fg != curFg || bg != curBg {
			sb.WriteString(colorSeq(fg, bg))
			curFg, curBg = fg, bg
		}
		sb.WriteRune(ch)
	}
	return sb.String()
}

// renderTermRowWithCursor renders one row with the cell at curX drawn as an
// inverted (reverse video) block cursor. Reversing fg/bg gives a visible block
// without emitting ANSI cursor positioning, which would fight Bubble Tea's
// line-by-line diff renderer.
func renderTermRowWithCursor(t vt10x.View, y, cols, curX int) string {
	var sb strings.Builder
	var curFg, curBg vt10x.Color = vt10x.DefaultFG, vt10x.DefaultBG
	for x := 0; x < cols; x++ {
		g := t.Cell(x, y)
		ch := g.Char
		if ch == 0 {
			ch = ' '
		}
		fg, bg := g.FG, g.BG
		if x == curX {
			// Reverse video: swap fg/bg so the cursor cell stands out.
			fg, bg = bg, fg
		}
		if fg != curFg || bg != curBg {
			sb.WriteString(colorSeq(fg, bg))
			curFg, curBg = fg, bg
		}
		sb.WriteRune(ch)
	}
	return sb.String()
}

// colorSeq emits the SGR sequence for a (fg, bg) pair.
func colorSeq(fg, bg vt10x.Color) string {
	var codes []string
	if fg != vt10x.DefaultFG {
		codes = append(codes, sgrColor(true, fg))
	}
	if bg != vt10x.DefaultBG {
		codes = append(codes, sgrColor(false, bg))
	}
	if len(codes) == 0 {
		return "\x1b[0m"
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// sgrColor emits the SGR color code for one vt10x.Color. Palette colors are
// [0,256) and map onto 256-color codes (38;5;N / 48;5;N). Truecolor values are
// packed by vt10x as r<<16|g<<8|b; those with n >= 65536 are decoded and
// emitted as 38;2;r;g;b / 48;2;r;g;b. (A truecolor with red == 0 packs below
// 65536 and is indistinguishable from a palette index — an ambiguity inherent
// to vt10x's packing.)
func sgrColor(fg bool, c vt10x.Color) string {
	n := uint32(c)
	if n >= 65536 { // truecolor r<<16|g<<8|b
		prefix := "38;2"
		if !fg {
			prefix = "48;2"
		}
		return fmt.Sprintf("%s;%d;%d;%d", prefix, byte(n>>16), byte(n>>8), byte(n))
	}
	prefix := "38;5"
	if !fg {
		prefix = "48;5"
	}
	return fmt.Sprintf("%s;%d", prefix, n)
}
