package engine

import (
	"context"
	"io"
)

// SessionHandle is a live interactive session into a node. It stays connected
// in the background; output continues to flow into Output even when the UI is
// not focused on it.
type SessionHandle struct {
	ID       string
	Title    string
	LabName  string
	NodeName string
	Stdin    io.WriteCloser // write input to the session
	Output   <-chan []byte  // raw PTY output bytes for terminal emulation
	Resize   func(w, h int) error
	Close    func() error
}

// DisplayName returns the human-facing label for the session. It prefers the
// logical node name (the name shown on the topology) and falls back to Title
// (the engine container name) when NodeName is unset.
func (h *SessionHandle) DisplayName() string {
	if h.NodeName != "" {
		return h.NodeName
	}
	return h.Title
}

// SessionManager manages multiple live interactive sessions (multi-tab). Each
// session is an independent PTY; sessions keep running in the background and
// their output is buffered.
type SessionManager interface {
	OpenSession(ctx context.Context, labName, nodeName string, mode SessionMode) (*SessionHandle, error)
}
