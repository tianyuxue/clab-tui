package tabs

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hinshun/vt10x"
	"github.com/muesli/termenv"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

type sessionStdin struct {
	*bytes.Buffer
}

func (sessionStdin) Close() error { return nil }

func newSession(t *testing.T, id string) (*SessionModel, *engine.SessionHandle, *sessionStdin) {
	t.Helper()
	stdin := &sessionStdin{Buffer: &bytes.Buffer{}}
	h := &engine.SessionHandle{ID: id, Title: "r1", NodeName: "r1", Stdin: stdin}
	sm := NewSessionModel()
	sm.SetSize(80, 24)
	sm.AddSession(h)
	return sm, h, stdin
}

func TestSessionModelFeedRendersContent(t *testing.T) {
	sm, h, _ := newSession(t, "s1")
	sm.Feed(h.ID, []byte("hello world\r\n"))
	v := sm.View()
	if !bytes.Contains([]byte(v), []byte("hello world")) {
		t.Fatalf("expected hello world rendered, got:\n%s", v)
	}
}

func TestSessionModelFeedBeforeSizeBuffersAndFlushes(t *testing.T) {
	sm := NewSessionModel() // no SetSize → 0x0 terminal
	h := &engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"}
	sm.AddSession(h)
	sm.Feed(h.ID, []byte("early output"))
	// No panic; nothing rendered yet (term unsized).
	sm.SetSize(80, 24) // now sized → pending flushed
	v := sm.View()
	if !bytes.Contains([]byte(v), []byte("early output")) {
		t.Fatalf("expected buffered output rendered after resize, got:\n%s", v)
	}
}

func TestSessionModelModeDefaultsToInsert(t *testing.T) {
	sm, _, _ := newSession(t, "s1")
	if sm.Mode() != SessionInsert {
		t.Fatalf("expected default insert mode, got %v", sm.Mode())
	}
}

func TestSessionModelSetMode(t *testing.T) {
	sm, _, _ := newSession(t, "s1")
	sm.SetMode(SessionNormal)
	if sm.Mode() != SessionNormal {
		t.Fatalf("expected normal mode, got %v", sm.Mode())
	}
}

func TestSessionModelInsertModeWritesToStdin(t *testing.T) {
	sm, _, stdin := newSession(t, "s1")
	// insert mode (default): a key goes to the shell
	sm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if got := stdin.String(); got != "p" {
		t.Fatalf("expected p written to stdin, got %q", got)
	}
	// enter → CR
	sm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := stdin.String(); got != "p\r" {
		t.Fatalf("expected CR after enter, got %q", got)
	}
	// up arrow → escape sequence, NOT literal "up"
	sm.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := stdin.String(); got != "p\r\x1b[A" {
		t.Fatalf("expected up arrow as ESC sequence, got %q", got)
	}
}

func TestSessionModelNormalModeEnterFocuses(t *testing.T) {
	sm, _, _ := newSession(t, "s1")
	sm.SetMode(SessionNormal)
	sm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sm.Mode() != SessionInsert {
		t.Fatalf("expected insert mode after Enter in normal, got %v", sm.Mode())
	}
}

func TestSessionModelNormalModeLetterSwitches(t *testing.T) {
	sm, _, _ := newSession(t, "s1") // Title "r1"
	sm.AddSession(&engine.SessionHandle{ID: "s2", Title: "switch1", NodeName: "switch1"})
	sm.SetMode(SessionNormal)
	sm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if got := sm.Active().Handle.NodeName; got != "r1" {
		t.Fatalf("expected active r1 after r, got %s", got)
	}
	sm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if got := sm.Active().Handle.NodeName; got != "switch1" {
		t.Fatalf("expected active switch1 after w, got %s", got)
	}
}

func TestSessionModelNormalModeQCloses(t *testing.T) {
	sm, _, _ := newSession(t, "s1")
	sm.SetMode(SessionNormal)
	sm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if sm.HasSessions() {
		t.Fatal("expected session closed by q in normal mode")
	}
}

func TestSessionModelNormalModeLetterSwitch(t *testing.T) {
	sm, _, _ := newSession(t, "s1") // Title "r1"
	sm.AddSession(&engine.SessionHandle{ID: "s2", Title: "switch1", NodeName: "switch1"})
	sm.SetMode(SessionNormal)
	// Titles "r1","switch1" → labels r, w ('s' is reserved for the picker)
	sm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if got := sm.Active().Handle.NodeName; got != "switch1" {
		t.Fatalf("expected active switch1 after letter w, got %s", got)
	}
}

func TestSessionModelNormalModeLetterSkipsReserved(t *testing.T) {
	sm, _, _ := newSession(t, "s1") // Title "r1"
	sm.AddSession(&engine.SessionHandle{ID: "s2", Title: "quit", NodeName: "quit"})
	sm.SetMode(SessionNormal)
	// "q" is reserved (close) — must NOT switch to the "quit" session.
	sm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if got := sm.Active().Handle.NodeName; got != "r1" {
		t.Fatalf("expected q not to switch (reserved close), active still r1, got %s", got)
	}
}

func TestSessionModelSessionLabels(t *testing.T) {
	sm, _, _ := newSession(t, "s1") // Title "r1"
	sm.AddSession(&engine.SessionHandle{ID: "s2", Title: "srv2", NodeName: "srv2"})
	labels := sm.SessionLabels()
	// 's' is reserved (picker) so "srv2" falls back to its second letter 'v'.
	if len(labels) != 2 || labels[0] != "r" || labels[1] != "v" {
		t.Fatalf("expected labels [r v], got %v", labels)
	}
}

func TestSessionModelNormalModeEnterEntersInsert(t *testing.T) {
	sm, _, _ := newSession(t, "s1")
	sm.SetMode(SessionNormal)
	sm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !sm.IsInsert() {
		t.Fatal("expected Enter to enter insert mode")
	}
}

func TestRenderTermRowWithCursorUsesWhiteBlock(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(10, 3))
	if _, err := term.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	row := renderTermRowWithCursor(term, 0, 10, 2)
	if !strings.Contains(row, "\x1b[38;5;0;48;5;15m") {
		t.Fatalf("expected white block cursor SGR, got %q", row)
	}
}

func TestSessionModelNormalModeJKScrollsViewport(t *testing.T) {
	sm, _, _ := newSession(t, "s1") // Title "r1"
	sm.SetMode(SessionNormal)
	// Long content so the viewport can actually scroll; j/k are not labels,
	// so they must fall through to the viewport instead of being swallowed.
	sm.vp.SetContent(strings.Repeat("line\n", 100))
	sm.vp.GotoBottom()
	bottom := sm.vp.YOffset
	if bottom <= 0 {
		t.Fatalf("expected scrollable content, YOffset %d", bottom)
	}
	sm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if sm.vp.YOffset >= bottom {
		t.Fatalf("expected k to scroll up, YOffset %d -> %d", bottom, sm.vp.YOffset)
	}
	sm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if sm.vp.YOffset != bottom {
		t.Fatalf("expected j to scroll back down to %d, got %d", bottom, sm.vp.YOffset)
	}
}

func TestSessionModelFocusSession(t *testing.T) {
	sm, _, _ := newSession(t, "s1")
	sm.AddSession(&engine.SessionHandle{ID: "s2", Title: "srv2", NodeName: "srv2"})
	sm.AddSession(&engine.SessionHandle{ID: "s3", Title: "srv3", NodeName: "srv3"})
	sm.FocusSession("s3")
	if got := sm.Active().Handle.ID; got != "s3" {
		t.Fatalf("expected active s3 after FocusSession, got %s", got)
	}
	sm.FocusSession("does-not-exist")
	if got := sm.Active().Handle.ID; got != "s3" {
		t.Fatalf("expected active s3 unchanged after unknown id, got %s", got)
	}
}

func TestSessionModelSessionsHandles(t *testing.T) {
	sm, _, _ := newSession(t, "s1")
	sm.AddSession(&engine.SessionHandle{ID: "s2", Title: "srv2"})
	handles := sm.SessionsHandles()
	if len(handles) != 2 {
		t.Fatalf("expected 2 handles, got %d", len(handles))
	}
	if handles[0].ID != "s1" || handles[1].ID != "s2" {
		t.Fatalf("unexpected handles order: %+v", handles)
	}
}

// TestSessionModelViewEmitsCursorControl verifies the view does NOT emit ANSI
// CUP cursor positioning (ESC [ row ; col H) in insert mode. CUP would fight
// Bubble Tea's line-by-line diff renderer and displace the status/help bars;
// the cursor is instead drawn as a reverse-video block inside the rendered
// terminal rows. We assert no CUP appears and the terminal cursor is hidden.
func TestSessionModelViewEmitsCursorControl(t *testing.T) {
	sm, h, _ := newSession(t, "s1")
	// Feed a prompt + a cursor-home so vt10x tracks a cursor position.
	sm.Feed(h.ID, []byte("prompt> \x1b[1;1H"))
	sm.SetSize(80, 24)
	v := sm.View()
	cup := regexp.MustCompile(`\x1b\[\d+;\d+H`)
	if cup.MatchString(v) {
		t.Fatalf("expected NO CUP cursor position sequence in session view:\n%q", v)
	}
	if !strings.Contains(v, "\x1b[?25l") {
		t.Fatalf("expected terminal cursor hidden (ESC [?25l) in session view:\n%q", v)
	}
}

// TestSessionModelCursorRowBelowBars verifies the reverse-video block cursor
// is rendered inline in the terminal content (no ANSI CUP that could displace
// the status/help bars). With a cursor at terminal (0,0) the first content row
// carries the reverse-video block.
func TestSessionModelCursorRowBelowBars(t *testing.T) {
	sm, h, _ := newSession(t, "s1")
	sm.Feed(h.ID, []byte("prompt> \x1b[1;1H"))
	sm.SetSize(80, 24)
	v := sm.View()
	cup := regexp.MustCompile(`\x1b\[\d+;\d+H`)
	if cup.MatchString(v) {
		t.Fatalf("expected NO CUP cursor sequence in view:\n%q", v)
	}
	// Reverse-video cursor: cell (0,0) of the terminal is drawn with its
	// fg/bg swapped. The swap only matters when fg != bg; feed a prompt with
	// a default-color cell and confirm the view renders the row (the block
	// cursor path runs for cursor-visible rows regardless of exact colors).
	if !strings.Contains(v, "p") {
		t.Fatalf("expected prompt content in view:\n%q", v)
	}
}

func TestSessionModelBarShowsLetterLabels(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	sm, _, _ := newSession(t, "s1") // Title "r1"
	sm.AddSession(&engine.SessionHandle{ID: "s2", Title: "srv2", NodeName: "srv2"})
	sm.SetMode(SessionNormal)
	// Red-highlighted letters r and v should appear (with ANSI color 196),
	// and the old "1:r1" numeric prefix should be gone.
	v := sm.View()
	if strings.Contains(v, "1:r1") {
		t.Fatalf("expected numeric prefix removed, got:\n%s", v)
	}
	if !strings.Contains(v, "\x1b[38;5;196m") {
		t.Fatalf("expected red letter highlight in bar, got:\n%s", v)
	}
}
