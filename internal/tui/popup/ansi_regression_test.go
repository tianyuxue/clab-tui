package popup

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func buildConsistentBody() string {
	var b strings.Builder
	b.WriteString("Header\n\n")
	b.WriteString("▸ item-01\n")
	b.WriteString("  item-02\n")
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(30).
		Height(8)
	return style.Render(b.String())
}

// TestPlaceConsistentBodyBordersIntact verifies a real lipgloss-bordered body
// renders with all four borders (left │, right │, corners) when placed onto an
// ANSI-colored background frame.
func TestPlaceConsistentBodyBordersIntact(t *testing.T) {
	body := buildConsistentBody()
	bodyLines := strings.Split(body, "\n")
	bh := len(bodyLines)
	bw := 0
	for _, l := range bodyLines {
		if w := visualWidth(l); w > bw {
			bw = w
		}
	}
	p := &fakePopup{body: body, width: bw, height: bh, on: true}

	frameH := bh + 6
	frameW := bw + 10
	var frame strings.Builder
	for i := 0; i < frameH; i++ {
		frame.WriteString(strings.Repeat("\x1b[31mF\x1b[0m", frameW))
		if i < frameH-1 {
			frame.WriteString("\n")
		}
	}
	out := Place(frame.String(), p, frameW, frameH)
	outLines := strings.Split(out, "\n")
	top := (frameH - bh) / 2

	for i := 0; i < bh; i++ {
		s := stripANSI(outLines[top+i])
		// Top row: ╭ left, ╮ right (both must be present somewhere in row).
		if i == 0 {
			if !strings.ContainsRune(s, '╭') || !strings.ContainsRune(s, '╮') {
				t.Fatalf("top border broken: %q", s)
			}
			continue
		}
		// Bottom row: ╰ left, ╯ right.
		if i == bh-1 {
			if !strings.ContainsRune(s, '╰') || !strings.ContainsRune(s, '╯') {
				t.Fatalf("bottom border broken: %q", s)
			}
			continue
		}
		// Middle rows: │ both sides.
		if strings.Count(s, "│") < 2 {
			t.Fatalf("side borders broken row %d: %q", i, s)
		}
	}
	// Content preserved: Header text somewhere.
	found := false
	for _, l := range outLines {
		if strings.Contains(stripANSI(l), "Header") {
			found = true
		}
	}
	if !found {
		t.Fatal("popup content (Header) missing")
	}
}
