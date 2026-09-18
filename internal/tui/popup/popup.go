// Package popup provides a reusable, terminal-safe overlay mechanism.
//
// A Popup renders its own body (with borders, padding, colors). Place()
// centers the popup over a rendered frame, fully blanking the region it
// occupies so that background content (including ANSI escape sequences)
// cannot leak into the popup and corrupt its rendering.
package popup

import (
	"strings"
)

// Popup is any UI element that can be shown as an overlay on top of a frame.
// Implementations render their own body via Body(); Place() handles the
// positioning and background isolation.
type Popup interface {
	Visible() bool
	Body() string
	DesiredWidth() int
	DesiredHeight() int
}

// Anchor controls where a popup is placed within the frame.
type Anchor int

const (
	// AnchorCenter centers the popup both horizontally and vertically.
	AnchorCenter Anchor = iota
	// AnchorBottomRight pins the popup to the bottom-right corner, growing
	// upward and leftward.
	AnchorBottomRight
)

// anchored reports the popup's placement anchor, defaulting to Center.
func anchored(p Popup) Anchor {
	if a, ok := p.(interface{ Anchor() Anchor }); ok {
		return a.Anchor()
	}
	return AnchorCenter
}

// Place overlays p onto frame. The frame is a multi-line string. If the popup
// is not visible, frame is returned unchanged. The popup region is positioned
// per its anchor (default: centered both horizontally and vertically); the
// region is blanked with spaces before the body is stamped, so the body is
// never mixed with background ANSI codes.
func Place(frame string, p Popup, frameW, frameH int) string {
	if p == nil || !p.Visible() {
		return frame
	}
	body := p.Body()
	if body == "" {
		return frame
	}

	bodyLines := strings.Split(body, "\n")
	bodyH := len(bodyLines)
	if bodyH == 0 {
		return frame
	}
	bodyW := 0
	for _, l := range bodyLines {
		w := visualWidth(l)
		if w > bodyW {
			bodyW = w
		}
	}
	// The popup's desired size caps how much we draw; fall back to body size.
	h := p.DesiredHeight()
	if h <= 0 || h > bodyH {
		h = bodyH
	}
	w := p.DesiredWidth()
	if w <= 0 || w > bodyW {
		w = bodyW
	}

	frameLines := strings.Split(frame, "\n")
	frameH = len(frameLines)
	// frameW represents the available screen width (from the caller). If not
	// provided, fall back to the widest content line.
	if frameW <= 0 {
		frameW = 0
		for _, l := range frameLines {
			if wv := visualWidth(l); wv > frameW {
				frameW = wv
			}
		}
	}
	// Cap the popup width to the frame so it can never overflow the screen.
	if w > frameW {
		w = frameW
	}

	var top, left int
	switch anchored(p) {
	case AnchorBottomRight:
		top = frameH - h
		left = frameW - w
	default:
		top = (frameH - h) / 2
		left = (frameW - w) / 2
	}
	if top < 0 {
		top = 0
	}
	if left < 0 {
		left = 0
	}

	out := make([]string, frameH)
	for i := 0; i < frameH; i++ {
		line := frameLines[i]
		if i >= top && i < top+h {
			// Clear only the popup rectangle. Strip background ANSI from this
			// row so the popup body cannot inherit styles, while preserving the
			// pane content and borders outside the rectangle.
			row := padToWidth(line, frameW)
			if i-top < len(bodyLines) {
				row = placeString(row, bodyLines[i-top], left, w)
			}
			out[i] = row
		} else {
			out[i] = line
		}
	}
	return strings.Join(out, "\n")
}

func skipEscapeForOverlay(s string, i int) int {
	if i+1 >= len(s) {
		return len(s)
	}
	if s[i+1] == '[' {
		j := i + 2
		for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
			j++
		}
		if j < len(s) {
			return j + 1
		}
		return len(s)
	}
	if s[i+1] == ']' {
		j := i + 2
		for j < len(s) && s[j] != 0x07 && !(s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\') {
			j++
		}
		if j < len(s) && s[j] == 0x1b && j+1 < len(s) {
			return j + 2
		}
		return j + 1
	}
	return i + 2
}

// padToWidth pads s with spaces to exactly width cells (no ANSI handling;
// used for freshly blanked rows that contain no escape codes).
func padToWidth(s string, width int) string {
	if visualWidth(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visualWidth(s))
}

// placeString places src into dst starting at visible column start, occupying
// width visible columns. ANSI styles outside the replaced region are retained;
// styles inside it are reset before the popup body and restored afterwards.
func placeString(dst, src string, start, width int) string {
	if start >= visualWidth(dst) {
		return dst
	}
	// Right-align src within its region when it is narrower than width.
	srcVis := visualWidth(src)
	if srcVis < width {
		pad := strings.Repeat(" ", width-srcVis)
		src = pad + src
	}
	return replaceVisibleRange(dst, src, start, width)
}

func replaceVisibleRange(dst, replacement string, start, width int) string {
	end := start + width
	var prefix, suffix strings.Builder
	activeStyle := ""
	visible := 0
	hadANSI := false
	for i := 0; i < len(dst); {
		if dst[i] == 0x1b {
			next := skipEscapeForOverlay(dst, i)
			seq := dst[i:next]
			hadANSI = true
			if visible < start {
				prefix.WriteString(seq)
				if strings.HasSuffix(seq, "m") {
					if seq == "\x1b[0m" {
						activeStyle = ""
					} else {
						activeStyle = seq
					}
				}
			} else if visible >= end {
				suffix.WriteString(seq)
			}
			i = next
			continue
		}
		size := 1
		if dst[i] >= 0x80 {
			_, size = decodeRune([]byte(dst[i:]))
		}
		if visible < start {
			prefix.WriteString(dst[i : i+size])
		} else if visible >= end {
			suffix.WriteString(dst[i : i+size])
		}
		visible++
		i += size
	}
	if !hadANSI {
		return prefix.String() + replacement + suffix.String()
	}
	return prefix.String() + "\x1b[0m" + replacement + activeStyle + suffix.String()
}

// visualWidth returns the display width of s ignoring ANSI escape sequences.
// ASCII (and single-width runes) count as 1.
func visualWidth(s string) int {
	w := 0
	i := 0
	for i < len(s) {
		if s[i] == 0x1b { // ESC
			// Skip CSI: ESC [ ... final byte in @-~
			if i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
					j++
				}
				i = j + 1
				continue
			}
			// Skip OSC: ESC ] ... BEL/ST
			if i+1 < len(s) && s[i+1] == ']' {
				j := i + 2
				for j < len(s) && s[j] != 0x07 && !(s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\') {
					j++
				}
				i = j + 1
				continue
			}
			i++
			continue
		}
		// Count rune width (simple: bytes>127 => 1 for UTF-8 multibyte; this
		// is approximate but correct for our box-drawing/ASCII usage).
		if s[i] < 0x80 {
			w++
			i++
		} else {
			w++
			_, size := decodeRune([]byte(s[i:]))
			i += size
		}
	}
	return w
}

func decodeRune(b []byte) (rune, int) {
	if len(b) == 0 {
		return 0, 1
	}
	if b[0] < 0x80 {
		return rune(b[0]), 1
	}
	// Determine UTF-8 sequence length from the lead byte.
	lead := b[0]
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
	r := rune(lead & (0x7F >> uint(n)))
	for i := 1; i < n && i < len(b); i++ {
		r = r<<6 | rune(b[i]&0x3F)
	}
	return r, n
}
