package tui

import (
	"context"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

// changeStreamReadyMsg carries the subscribed engine change channel. An
// optional err is set when the lazy start failed but the subscription is
// still alive.
type changeStreamReadyMsg struct {
	ch     <-chan engine.Change
	cancel func()
	err    error
}

type errMsg struct {
	err error
}

// errExpiredMsg is delivered after the errMsg display timer expires, clearing
// the error strip. seq matches the errMsg that started the timer so a stale
// expiry can't clear a newer error.
type errExpiredMsg struct {
	seq int
}

type labsLoadedMsg struct {
	labs []*engine.Lab
}

type topologyLoadedMsg struct {
	lab *engine.Lab
}

type opLineMsg struct {
	Line engine.OutputLine
}

type sessionOutputMsg struct {
	ID   string
	Data []byte
}

type sessionDoneMsg struct {
	ID string
}

type logStreamMsg struct {
	Ch   <-chan engine.OutputLine
	Line engine.OutputLine
}

type logStreamDoneMsg struct {
	Ch <-chan engine.OutputLine
}

type topologyReloadMsg struct{}

// monitorMsg carries one node monitor snapshot (or its fetch error).
type monitorMsg struct {
	node    string
	monitor *engine.NodeMonitor
	err     error
}

// monitorTickMsg schedules the next 2s monitor refresh for a node.
type monitorTickMsg struct{ node string }

// interfaceIPMsg carries one node's interface IPv4 map (or its fetch error).
type interfaceIPMsg struct {
	node string
	ips  map[string]string
	err  error
}

type interfaceStatsMsg struct {
	node  string
	stats map[string]*engine.InterfaceStats
	err   error
}

// traceEventMsg is one path-tracing packet hit. seq identifies the trace
// session so stale events from a stopped/older trace are ignored.
type traceEventMsg struct {
	ev  engine.PacketEvent
	seq int
}

// traceStoppedMsg signals that a path-tracing event stream closed.
type traceStoppedMsg struct {
	seq int
}

// traceBlinkMsg toggles the explicit render phase used for path highlights.
type traceBlinkMsg struct {
	seq int
}

// captureEventMsg is one captured packet on the capture pane.
type captureEventMsg struct{ ev engine.PacketEvent }

// captureStoppedMsg signals that a capture event stream closed.
type captureStoppedMsg struct{}

// graphMsg carries the outcome of starting the graph HTTP server.
type graphMsg struct {
	url string
	err error
}

// openBrowserFn opens url in the default browser; overridable in tests.
var openBrowserFn = func(url string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}

// launchBrowserCmd opens the graph URL in the browser, best-effort.
func launchBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		_ = openBrowserFn(url) // never surface errors; URL is already toasted
		return nil
	}
}

// subscribeEvents subscribes to the engine's change stream and triggers its
// lazy start (ListLabs). Subscribe always runs so live updates stay enabled;
// a lazy-start failure is surfaced on the message instead of killing the
// refresh loop.
func subscribeEvents(eng engine.Engine) tea.Cmd {
	return func() tea.Msg {
		if eng == nil {
			return nil
		}
		ch, cancel := eng.Subscribe()
		// Fire-and-forget the lazy start so the store populates; a failure
		// here must not disable live updates.
		if _, err := eng.ListLabs(context.Background()); err != nil {
			return changeStreamReadyMsg{ch: ch, cancel: cancel, err: err}
		}
		return changeStreamReadyMsg{ch: ch, cancel: cancel}
	}
}

// nextChange pulls the next Change from the subscribed stream as a tea.Cmd.
// Each Change emitted schedules the next pull, keeping the loop alive.
func nextChange(ch <-chan engine.Change) tea.Cmd {
	return func() tea.Msg {
		c, ok := <-ch
		if !ok {
			return nil
		}
		return c
	}
}

func loadLabs(eng engine.Engine) tea.Cmd {
	return func() tea.Msg {
		if eng == nil {
			return nil
		}
		labs, err := eng.ListLabs(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return labsLoadedMsg{labs: labs}
	}
}

func runOperation(ch <-chan engine.OutputLine) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return opLineMsg{Line: engine.OutputLine{Done: true}}
		}
		return opLineMsg{Line: line}
	}
}

// loadTopology fetches a lab (static topology + live state merged) by name.
func loadTopology(eng engine.Engine, labName string) tea.Cmd {
	return func() tea.Msg {
		if eng == nil {
			return nil
		}
		lab, err := eng.GetLab(context.Background(), labName)
		if err != nil {
			return errMsg{err}
		}
		return topologyLoadedMsg{lab: lab}
	}
}
