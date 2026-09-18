package tabs

import "strings"

// padToHeight pads s with blank lines so the rendered view occupies exactly
// height lines. This keeps the status bar anchored to the bottom of the
// terminal even when a tab has little or no content.
func padToHeight(s string, height int) string {
	if height <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) >= height {
		return s
	}
	return s + strings.Repeat("\n", height-len(lines))
}

// padToWidthHeight pads every line of s to at least width visible columns and
// pads the result to exactly height lines. This keeps the empty state of a
// tree/pane visually aligned with its populated state (same per-line width),
// so horizontal joins and terminal line-diffs don't leave misaligned residue.
func padToWidthHeight(s string, width, height int) string {
	if width <= 0 {
		return padToHeight(s, height)
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if visualWidth(l) < width {
			lines[i] = l + strings.Repeat(" ", width-visualWidth(l))
		}
	}
	out := strings.Join(lines, "\n")
	if height > 0 && len(lines) < height {
		out += strings.Repeat("\n", height-len(lines))
	}
	return out
}
