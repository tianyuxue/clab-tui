package engine

import (
	"context"
	"testing"
)

type fakeSessionManager struct{}

func (f *fakeSessionManager) OpenSession(ctx context.Context, labName, nodeName string, mode SessionMode) (*SessionHandle, error) {
	return nil, nil
}

func TestSessionManagerInterface(t *testing.T) {
	var _ SessionManager = (*fakeSessionManager)(nil)
}

func TestSessionHandleDisplayName(t *testing.T) {
	// Prefer the logical node name (shown on the topology).
	h := &SessionHandle{Title: "clab-lab-ceos1", NodeName: "ceos1"}
	if got := h.DisplayName(); got != "ceos1" {
		t.Fatalf("DisplayName = %q, want ceos1", got)
	}
	// Fall back to the container title when NodeName is unset.
	h2 := &SessionHandle{Title: "clab-lab-ceos1"}
	if got := h2.DisplayName(); got != "clab-lab-ceos1" {
		t.Fatalf("DisplayName = %q, want container title fallback", got)
	}
}

// Output is a raw byte stream (not line-parsed) so the UI can feed a terminal
// emulator directly.
func TestSessionHandleOutputIsRawBytes(t *testing.T) {
	ch := make(chan []byte, 1)
	h := &SessionHandle{Output: ch}
	ch <- []byte("echo hi\r\n\x1b[31mred\x1b[0m")
	got := <-h.Output
	if string(got) != "echo hi\r\n\x1b[31mred\x1b[0m" {
		t.Fatalf("expected raw bytes preserved, got %q", got)
	}
}
