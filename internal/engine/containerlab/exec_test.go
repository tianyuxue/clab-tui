package containerlab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestSpawnWithLinesCancellationReleasesBlockedProducer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan engine.OutputLine, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	done := make(chan struct{})
	go func() {
		readLines(ctx, strings.NewReader(strings.Repeat("line\n", 100000)), "stdout", ch, &wg)
		close(done)
	}()

	// Leave the bounded output channel unread so the reader is forced to
	// exercise its cancellation-aware send path.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("line producer did not exit after cancellation")
	}
}

func TestSpawnWithLinesCancellationDeliversFailureDone(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "sleep.sh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho ready\nwhile :; do :; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := spawnWithLines(ctx, bin)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case line := <-ch:
		if line.Line != "ready" {
			t.Fatalf("first output = %+v, want ready", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("command did not produce readiness output")
	}
	cancel()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				t.Fatal("stream closed without Done after cancellation")
			}
			if line.Done {
				if line.Code == 0 {
					t.Fatalf("canceled command reported success: %+v", line)
				}
				return
			}
		case <-deadline:
			t.Fatal("canceled command did not deliver terminal Done")
		}
	}
}
