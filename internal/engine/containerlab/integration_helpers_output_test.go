package containerlab

import (
	"context"
	"testing"
	"time"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestCollectOutputBestEffortReturnsDoneAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := make(chan engine.OutputLine, 1)
	ch <- engine.OutputLine{Done: true, Code: 23}

	start := time.Now()
	_, code, err := collectOutputBestEffort(t, ctx, ch)
	if err != nil {
		t.Fatalf("collectOutputBestEffort error = %v", err)
	}
	if code != 23 {
		t.Fatalf("exit code = %d, want 23", code)
	}
	if elapsed := time.Since(start); elapsed >= outputDrainGrace {
		t.Fatalf("Done handling took %s, should return before grace period %s", elapsed, outputDrainGrace)
	}
}
