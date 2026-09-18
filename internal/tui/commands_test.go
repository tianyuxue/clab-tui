package tui

import (
	"testing"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestLabsLoadedMsg(t *testing.T) {
	msg := labsLoadedMsg{
		labs: []*engine.Lab{{Name: "test-lab", Nodes: []engine.Node{{Name: "a"}, {Name: "b"}, {Name: "c"}}}},
	}
	if len(msg.labs) != 1 || msg.labs[0].Name != "test-lab" {
		t.Fatalf("unexpected labs: %+v", msg.labs)
	}
}

func TestLoadLabsNilBackendReturnsNil(t *testing.T) {
	cmd := loadLabs(nil)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	msg := cmd()
	if msg != nil {
		t.Fatalf("expected nil msg for nil backend (should not clear local labs), got %T", msg)
	}
}

func TestErrMsg(t *testing.T) {
	msg := errMsg{err: nil}
	if msg.err != nil {
		t.Fatal("expected nil error")
	}
}
