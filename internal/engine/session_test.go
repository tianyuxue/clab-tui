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
