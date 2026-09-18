package tabs

import (
	"strings"
	"testing"
)

func TestLogsViewFitsContentHeight(t *testing.T) {
	m := NewLogs()
	m.SetSize(80, 22)
	m.SetLabel("r1")
	m.AppendLine("INFO some log line")
	lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
	if len(lines) != 22 {
		t.Fatalf("LogModel.View() has %d lines, want 22 (header+footer must not overflow)", len(lines))
	}
}