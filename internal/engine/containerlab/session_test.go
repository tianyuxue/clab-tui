package containerlab

import (
	"bytes"
	"testing"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestClabEngineImplementsSessionManager(t *testing.T) {
	var _ engine.SessionManager = (*ClabEngine)(nil)
}

func TestStreamSessionRawEmitsChunks(t *testing.T) {
	input := []byte("hello\r\n\x1b[31mred\x1b[0m tail")
	output := make(chan []byte, 4)
	done := make(chan struct{})
	waited := false
	go streamSessionRaw(bytes.NewReader(input), output, done, func() error { waited = true; return nil })
	var all []byte
	for chunk := range output {
		all = append(all, chunk...)
	}
	if string(all) != string(input) {
		t.Fatalf("expected raw bytes preserved, got %q", all)
	}
	if !waited {
		t.Fatal("expected wait called after stream exhausted")
	}
}
