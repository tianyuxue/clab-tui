package tabs

import (
	"strings"
	"testing"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestOpsViewFitsContentHeight(t *testing.T) {
	m := NewOps()
	m.SetSize(80, 22)
	m.SetLabel("Deploying: lab")
	m.AppendLine(engine.OutputLine{Line: "INFO some deploy output"})
	lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
	if len(lines) != 22 {
		t.Fatalf("OpsModel.View() has %d lines, want 22 (header+footer must not overflow)", len(lines))
	}
}

func TestOpsViewFitsContentHeightNoLabel(t *testing.T) {
	m := NewOps()
	m.SetSize(80, 22)
	m.AppendLine(engine.OutputLine{Line: "INFO output"})
	lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
	if len(lines) != 22 {
		t.Fatalf("OpsModel.View() has %d lines, want 22", len(lines))
	}
}