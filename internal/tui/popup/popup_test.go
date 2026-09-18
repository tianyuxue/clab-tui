package popup

import (
	"strings"
	"testing"
)

// fakePopup implements Popup with a fixed body.
type fakePopup struct {
	body   string
	width  int
	height int
	on     bool
}

func (f *fakePopup) Visible() bool      { return f.on }
func (f *fakePopup) Body() string       { return f.body }
func (f *fakePopup) DesiredWidth() int  { return f.width }
func (f *fakePopup) DesiredHeight() int { return f.height }

func TestPlaceBlanksOverlayRegion(t *testing.T) {
	frame := "aaaa\nbbbb\ncccc\ndddd"
	p := &fakePopup{body: "XY\nZZ", width: 2, height: 2, on: true}
	out := Place(frame, p, 4, 4)

	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}
	// Center: top=(4-2)/2=1, left=(4-2)/2=1. Only the popup rectangle is
	// replaced; content outside it remains visible.
	if lines[1] != "bXYb" {
		t.Fatalf("expected popup row 1, got %q", lines[1])
	}
	if lines[2] != "cZZc" {
		t.Fatalf("expected popup row 2, got %q", lines[2])
	}
	// Rows outside stay intact.
	if lines[0] != "aaaa" || lines[3] != "dddd" {
		t.Fatalf("unexpected outside rows: %q %q", lines[0], lines[3])
	}
}

// TestPlaceBlocksBackgroundWithANSI ensures ANSI-colored background content
// inside the popup region cannot corrupt the popup body.
func TestPlaceBlocksBackgroundWithANSI(t *testing.T) {
	// Background rows contain ANSI color codes; the popup region must be
	// replaced with a plain body, not stamp-on-top of the codes.
	frame := "\x1b[31maaaa\x1b[0m\n\x1b[31mbbbb\x1b[0m\ncccc\ndddd"
	p := &fakePopup{body: "XY\nZZ", width: 2, height: 2, on: true}
	out := Place(frame, p, 4, 4)

	lines := strings.Split(out, "\n")
	// The popup region must be free of the red background codes, while the
	// outside columns remain visible.
	if stripANSI(lines[1]) != "bXYb" {
		t.Fatalf("popup row leaked ANSI background, got %q", lines[1])
	}
	if stripANSI(lines[2]) != "cZZc" {
		t.Fatalf("popup row leaked ANSI background, got %q", lines[2])
	}
	if !strings.Contains(lines[1], "\x1b[31m") || !strings.Contains(lines[1], "\x1b[0m") {
		t.Fatalf("expected outside ANSI style to survive popup placement: %q", lines[1])
	}
	// Rows outside the popup region keep their ANSI content untouched.
	if !strings.HasPrefix(lines[0], "\x1b[31m") || !strings.HasSuffix(lines[0], "\x1b[0m") {
		t.Fatalf("outside row 0 lost its ANSI styling: %q", lines[0])
	}
	if lines[3] != "dddd" {
		t.Fatalf("outside row 3 changed: %q", lines[3])
	}
}

func TestPlaceHiddenPopupNoop(t *testing.T) {
	frame := "abc\ndef"
	p := &fakePopup{body: "XY", width: 2, height: 1, on: false}
	if got := Place(frame, p, 3, 2); got != frame {
		t.Fatalf("expected frame unchanged when popup hidden, got %q", got)
	}
}

func TestPlaceClippedToFrame(t *testing.T) {
	frame := "a\nb"
	p := &fakePopup{body: "XYZ\nXYZ\nXYZ", width: 3, height: 3, on: true}
	out := Place(frame, p, 3, 2)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected output bounded by frame height (2), got %d", len(lines))
	}
}

func TestPlaceOpaqueRegionFullyBlanked(t *testing.T) {
	// Body shorter than its region: only the popup region is blanked, not the
	// content on either side of it.
	frame := "aaaa\nbbbb\ncccc\ndddd"
	p := &fakePopup{body: "X", width: 1, height: 1, on: true}
	out := Place(frame, p, 4, 4)
	lines := strings.Split(out, "\n")
	// top=(4-1)/2=1, left=(4-1)/2=1 -> row 1 becomes "aXaa".
	if lines[1] != "bXbb" {
		t.Fatalf("expected blanked region with body, got %q", lines[1])
	}
}

// TestPlaceFullBorder ensures a bordered popup body is not clipped: when the
// popup's Desired* includes borders, the whole body (top and bottom border)
// must appear in the output.
func TestPlaceFullBorder(t *testing.T) {
	// Simulate a bordered body: top border, 2 content rows, bottom border.
	body := "╭────╮\n│ ab │\n│ cd │\n╰────╯"
	p := &fakePopup{body: body, width: 6, height: 4, on: true}
	frame := "aaaaaa\nbbbbbb\ncccccc\ndddddd\neeeeee\nffffff"
	out := Place(frame, p, 6, 6)

	if !strings.Contains(out, "╭") || !strings.Contains(out, "╯") {
		t.Fatalf("expected full bordered popup (top and bottom), got:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	// Bottom border must be present on some line.
	foundBottom := false
	for _, l := range lines {
		if strings.Contains(l, "╰") && strings.Contains(l, "╯") {
			foundBottom = true
			break
		}
	}
	if !foundBottom {
		t.Fatalf("bottom border missing:\n%s", out)
	}
}

type anchoredPopup struct {
	fakePopup
	anchor Anchor
}

func (a *anchoredPopup) Anchor() Anchor { return a.anchor }

// TestPlaceBottomRightAnchorsToCorner ensures the popup hugs the bottom-right
// corner of the frame instead of being centered.
func TestPlaceBottomRightAnchorsToCorner(t *testing.T) {
	frame := "aaaa\nbbbb\ncccc\ndddd"
	p := &anchoredPopup{fakePopup: fakePopup{body: "XY", width: 2, height: 1, on: true}, anchor: AnchorBottomRight}
	out := Place(frame, p, 4, 4)

	lines := strings.Split(out, "\n")
	// bottom-right: top = 4-1 = 3, left = 4-2 = 2 -> row 3 becomes "ddXY".
	if lines[3] != "ddXY" {
		t.Fatalf("expected popup at bottom-right, got row %q", lines[3])
	}
	if lines[0] != "aaaa" || lines[1] != "bbbb" || lines[2] != "cccc" {
		t.Fatalf("rows above the popup must stay intact: %q %q %q", lines[0], lines[1], lines[2])
	}
}

// TestPlaceBottomRightWithANSI keeps the ANSI isolation guarantee for the new
// anchor: background codes inside the region are cleared.
func TestPlaceBottomRightWithANSI(t *testing.T) {
	frame := "\x1b[31maaaa\x1b[0m\n\x1b[31mbbbb\x1b[0m\n\x1b[31mcccc\x1b[0m\n\x1b[31mdddd\x1b[0m"
	p := &anchoredPopup{fakePopup: fakePopup{body: "XY", width: 2, height: 1, on: true}, anchor: AnchorBottomRight}
	out := Place(frame, p, 4, 4)
	lines := strings.Split(out, "\n")
	if stripANSI(lines[3]) != "ddXY" {
		t.Fatalf("popup row leaked ANSI background, got %q", stripANSI(lines[3]))
	}
	if !strings.Contains(lines[3], "\x1b[31m") {
		t.Fatalf("outside style must survive outside the popup region: %q", lines[3])
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b {
			if i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
					j++
				}
				i = j + 1
				continue
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
