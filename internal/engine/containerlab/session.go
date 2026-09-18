package containerlab

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/creack/pty"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// sessionCounter generates unique session IDs.
var sessionCounter struct {
	mu sync.Mutex
	n  int
}

func nextSessionID() string {
	sessionCounter.mu.Lock()
	defer sessionCounter.mu.Unlock()
	sessionCounter.n++
	return fmt.Sprintf("s%d", sessionCounter.n)
}

// OpenSession opens a live PTY session into a node via docker exec.
func (e *ClabEngine) OpenSession(ctx context.Context, labName, nodeName string, mode engine.SessionMode) (*engine.SessionHandle, error) {
	switch mode {
	case engine.SessionModeShell:
	default:
		return nil, engine.ErrNotSupported
	}
	container := e.containerForNode(ctx, labName, nodeName)
	if container == "" {
		return nil, engine.ErrNodeNotFound
	}

	cmd := exec.CommandContext(ctx, "docker", "exec", "-it", container, "bash")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	output := make(chan []byte, 128)
	done := make(chan struct{})
	var closeOnce sync.Once
	var closeErr error
	handle := &engine.SessionHandle{
		ID:       nextSessionID(),
		Title:    container,
		LabName:  labName,
		NodeName: nodeName,
		Stdin:    ptmx,
		Output:   output,
		Resize: func(w, h int) error {
			return pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
		},
		Close: func() error {
			closeOnce.Do(func() {
				close(done)
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				closeErr = ptmx.Close()
			})
			return closeErr
		},
	}

	go streamSessionRaw(ptmx, output, done, cmd.Wait)

	return handle, nil
}

// streamSessionRaw reads raw bytes from r and emits them as chunks so the UI's
// terminal emulator can render them. wait reports the process exit status after
// the stream is exhausted (used only to keep the process reaped).
func streamSessionRaw(r io.Reader, output chan<- []byte, done <-chan struct{}, wait func() error) {
	defer close(output)
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			select {
			case output <- chunk:
			case <-done:
				return
			}
		}
		if err != nil {
			break
		}
	}
	_ = wait()
}

var _ engine.SessionManager = (*ClabEngine)(nil)
