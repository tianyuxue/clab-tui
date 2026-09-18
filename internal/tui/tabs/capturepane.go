package tabs

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// CapturePane 底部弹出面板，实时显示抓包流。
type CapturePane struct {
	lines   []string
	vp      viewport.Model
	visible bool
	paused  bool
	stats   CaptureStats
	width   int
	height  int
}

type CaptureStats struct {
	Total int
	In    int
	Out   int
	Bytes int
}

func NewCapturePane() *CapturePane {
	return &CapturePane{vp: viewport.New(80, 8)}
}

func (p *CapturePane) Visible() bool       { return p.visible }
func (p *CapturePane) Show()               { p.visible = true }
func (p *CapturePane) Hide()               { p.visible = false }
func (p *CapturePane) Paused() bool        { return p.paused }
func (p *CapturePane) TogglePaused()       { p.paused = !p.paused }
func (p *CapturePane) Stats() CaptureStats { return p.stats }
func (p *CapturePane) SetSize(w, h int) {
	p.width = w
	p.height = h
	vpW := w - 4 // 边框(2) + padding(2)
	if vpW < 10 {
		vpW = 10
	}
	p.vp.Width = vpW
	p.vp.Height = h
}

// Clear 清空包流，并把空状态文案写进 viewport，保证面板高度恒定
// （空状态与有匹配包时占同样的行数）。
func (p *CapturePane) Clear() {
	p.lines = nil
	p.stats = CaptureStats{}
	p.paused = false
	p.vp.SetContent("No packets captured yet.")
	p.vp.GotoTop()
}

// Append adds a packet event to the pane. When the raw packet body is
// available, gopacket decodes a tcpdump-style Detail line (parsing only runs
// while the pane is visible, so the cost is on demand). Long lines are
// soft-wrapped to the viewport width so the tcpdump details stay readable.
//
// The direction tag (IN/OUT) shows whether the packet was captured on the TC
// ingress or egress hook. Both tags are colored for quick visual scanning.
func (p *CapturePane) Append(ev engine.PacketEvent) {
	p.stats.Total++
	p.stats.Bytes += ev.Pkt.Len
	switch ev.Pkt.Direction {
	case "IN":
		p.stats.In++
	case "OUT":
		p.stats.Out++
	}
	proto := ev.Pkt.Proto
	if proto == "" {
		proto = "?"
	}
	dir := ev.Pkt.Direction
	dirColor := "245"
	switch dir {
	case "IN":
		dirColor = "36"
	case "OUT":
		dirColor = "214"
	}
	dirStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(dirColor)).Render(dir)
	detail := ev.Pkt.Summary
	if d := parsePacketDetail(ev.Pkt.Payload, ev.Pkt.Len); d != "" {
		detail = d
	}
	prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(ev.Node+":"+ev.Iface) +
		"  " + dirStyle +
		"  " + lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(proto)
	if ev.Pkt.Time != "" {
		prefix += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(ev.Pkt.Time)
	}
	line := prefix + "  " + detail
	p.lines = append(p.lines, line)
	if len(p.lines) > 1000 {
		p.lines = p.lines[len(p.lines)-500:]
	}
	p.vp.SetContent(wrapLines(p.lines, p.vp.Width))
	if !p.paused {
		p.vp.GotoBottom()
	}
}

// wrapLines 把每条长行按 width 软换行成多条（忽略 ANSI 码，按可视宽度）。
// 这样 viewport 里的包详情不会因超宽被横向截断。
func wrapLines(lines []string, width int) string {
	if width <= 0 {
		return strings.Join(lines, "\n")
	}
	var out []string
	for _, l := range lines {
		out = append(out, wrapOne(l, width)...)
	}
	return strings.Join(out, "\n")
}

// wrapOne 把单行按可视宽度切分成多行。ANSI 转义序列原样保留，切分不会把
// 转义序列切破（宽度只统计可见字符，字节边界按 rune 对齐）。
func wrapOne(line string, width int) []string {
	if visualWidth(line) <= width {
		return []string{line}
	}
	var out []string
	rest := line
	for visualWidth(rest) > width {
		cut := cutAtWidth(rest, width)
		if cut <= 0 {
			break
		}
		out = append(out, rest[:cut])
		rest = rest[cut:]
	}
	out = append(out, rest)
	return out
}

// cutAtWidth 返回 s 中不超过可视宽度 width 的最长前缀字节长度。
// ANSI 转义序列不占宽度且不会被切开。
func cutAtWidth(s string, width int) int {
	vis := 0
	i := 0
	for i < len(s) {
		if s[i] == 0x1b {
			next := skipEscape(s, i)
			if next <= i {
				break
			}
			i = next
			continue
		}
		if vis+1 > width {
			break
		}
		vis++
		i += runeSizeAt(s, i)
	}
	return i
}

// skipEscape 返回跳过 s[i] 处 ANSI 转义序列后的字节索引。
func skipEscape(s string, i int) int {
	if i >= len(s) || s[i] != 0x1b {
		return i
	}
	if i+1 < len(s) && s[i+1] == '[' { // CSI: ESC [ ... @-~
		j := i + 2
		for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
			j++
		}
		return j + 1
	}
	if i+1 < len(s) && s[i+1] == ']' { // OSC: ESC ] ... BEL/ST
		j := i + 2
		for j < len(s) && s[j] != 0x07 && !(s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\') {
			j++
		}
		return j + 1
	}
	return i + 1
}

// runeSizeAt 返回 s[i] 处 UTF-8 rune 的字节长度（ASCII=1，多字节按实际长度）。
func runeSizeAt(s string, i int) int {
	if i >= len(s) {
		return 1
	}
	if s[i] < 0x80 {
		return 1
	}
	lead := s[i]
	var n int
	switch {
	case lead&0xE0 == 0xC0:
		n = 2
	case lead&0xF0 == 0xE0:
		n = 3
	case lead&0xF8 == 0xF0:
		n = 4
	default:
		n = 1
	}
	if i+n > len(s) {
		return len(s) - i
	}
	return n
}

// visualWidth 返回字符串的可视显示宽度（忽略 ANSI 转义序列；单字节/多字节
// rune 均按 1 计，对 box-drawing/ASCII 场景足够准确）。
func visualWidth(s string) int {
	w := 0
	i := 0
	for i < len(s) {
		if s[i] == 0x1b {
			next := skipEscape(s, i)
			if next <= i {
				break
			}
			i = next
			continue
		}
		w++
		i += runeSizeAt(s, i)
	}
	return w
}

func (p *CapturePane) Update(msg tea.Msg) (*CapturePane, tea.Cmd) {
	var cmd tea.Cmd
	p.vp, cmd = p.vp.Update(msg)
	return p, cmd
}

var capturePaneStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("62")).
	Padding(0, 1)

// View renders the pane with a rounded border. The viewport always fills the
// panel height; the bottom hint line persists in both empty and matched
// states. The hint mentions both the close/clear keys and the IN/OUT tag
// meaning, so users can distinguish receiving and transmitting observations.
func (p *CapturePane) View() string {
	content := p.vp.View()
	pauseHint := "[p] pause"
	state := ""
	if p.paused {
		pauseHint = "[p] resume"
		state = "  PAUSED"
	}
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(fmt.Sprintf(
		"[q] close  [c] clear  %s%s  pkts %d  IN %d  OUT %d  bytes %d",
		pauseHint, state, p.stats.Total, p.stats.In, p.stats.Out, p.stats.Bytes))
	return capturePaneStyle.Render(content + "\n" + hint)
}
