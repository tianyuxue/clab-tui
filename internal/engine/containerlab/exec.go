package containerlab

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func findBinary() (string, error) {
	return exec.LookPath("containerlab")
}

// spawnWithLines runs a command and streams its stdout/stderr as OutputLine.
func spawnWithLines(ctx context.Context, bin string, args ...string) (<-chan engine.OutputLine, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	return streamLines(ctx, cmd)
}

func spawnWithLinesOnDone(ctx context.Context, onDone func(), bin string, args ...string) (<-chan engine.OutputLine, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	return streamLinesOnDone(ctx, cmd, onDone)
}

func streamLines(ctx context.Context, cmd *exec.Cmd) (<-chan engine.OutputLine, error) {
	return streamLinesOnDone(ctx, cmd, nil)
}

func streamLinesOnDone(ctx context.Context, cmd *exec.Cmd, onDone func()) (<-chan engine.OutputLine, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	ch := make(chan engine.OutputLine, 100)

	if err := cmd.Start(); err != nil {
		close(ch)
		return nil, err
	}

	go func() {
		defer close(ch)
		defer func() {
			if onDone != nil {
				onDone()
			}
		}()
		var wg sync.WaitGroup

		wg.Add(2)
		go readLines(ctx, stdout, "stdout", ch, &wg)
		go readLines(ctx, stderr, "stderr", ch, &wg)

		wg.Wait()

		err := cmd.Wait()
		if err != nil {
			code := 0
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				code = exitErr.ExitCode()
			}
			sendTerminal(ch, engine.OutputLine{
				Line: err.Error(), Stream: "stderr", Done: true, Code: code,
			})
			return
		}
		sendTerminal(ch, engine.OutputLine{Done: true, Code: 0})
	}()

	return ch, nil
}

func readLines(ctx context.Context, r io.Reader, stream string, ch chan<- engine.OutputLine, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\n\r")
		if !sendLine(ctx, ch, engine.OutputLine{Line: line, Stream: stream}) {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		sendLine(ctx, ch, engine.OutputLine{Line: "scanner error: " + err.Error(), Stream: "stderr"})
	}
}

func sendLine(ctx context.Context, ch chan<- engine.OutputLine, line engine.OutputLine) bool {
	select {
	case ch <- line:
		return true
	case <-ctx.Done():
		return false
	}
}

const terminalDeliveryTimeout = time.Second

// sendTerminal ignores command cancellation so the exit result is not lost.
// If the consumer stopped reading, the bounded timeout lets the producer
// close; consumers treat a close without Done as an incomplete command.
func sendTerminal(ch chan<- engine.OutputLine, line engine.OutputLine) bool {
	timer := time.NewTimer(terminalDeliveryTimeout)
	defer timer.Stop()
	select {
	case ch <- line:
		return true
	case <-timer.C:
		return false
	}
}
