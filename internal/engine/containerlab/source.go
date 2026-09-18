package containerlab

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// actorRegistry maps actor_id (container short id) to its lab, logical node
// name, and container long name. Container events populate it; interface
// events resolve through it.
type actorRegistry struct {
	mu sync.RWMutex
	m  map[string]struct{ lab, node, longName string }
}

func newActorRegistry() *actorRegistry {
	return &actorRegistry{m: make(map[string]struct{ lab, node, longName string })}
}

func (r *actorRegistry) Set(actorID, lab, node, longName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if actorID == "" {
		return
	}
	r.m[actorID] = struct{ lab, node, longName string }{lab, node, longName}
}

func (r *actorRegistry) Lab(actorID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.m[actorID].lab
}

func (r *actorRegistry) Node(actorID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.m[actorID].node
}

func (r *actorRegistry) Delete(actorID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, actorID)
}

// LookupByLongName returns (lab, node) for a container long name, or empty
// strings if not found. It scans entries because the key is the short id.
func (r *actorRegistry) LookupByLongName(longName string) (string, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.m {
		if v.longName == longName {
			return v.lab, v.node
		}
	}
	return "", ""
}

// handleRawEventLine parses one JSON line and applies it to the store,
// resolving interface events to their logical node via the actor registry.
func handleRawEventLine(store *engine.Store, reg *actorRegistry, line string) error {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &probe); err != nil {
		return err
	}
	switch probe.Type {
	case "container":
		ev, err := parseContainerEventLine(line)
		if err != nil {
			return err
		}
		// Skip exec artifacts (e.g. "exec_create: ip -j addr" emitted by our
		// own docker exec calls) — they are not container lifecycle changes
		// and must not overwrite the node's real state.
		if isExecAction(ev.Action) {
			return nil
		}
		reg.Set(ev.ContainerID, ev.LabName, ev.NodeName, ev.Container)
		switch ev.Action {
		case "destroy", "remove", "delete":
			store.RemoveNode(ev.LabName, ev.NodeName)
			reg.Delete(ev.ContainerID)
			return nil
		}
		store.ApplyContainerEvent(*ev)
		// When a node stops, its network namespace is gone; clear stale
		// interfaces so links/lines reflect reality.
		if ev.State == engine.StatusStopped {
			store.ClearNodeInterfaces(ev.LabName, ev.NodeName)
		}
		return nil
	case "interface":
		ev, err := parseInterfaceEventLine(line)
		if err != nil {
			return err
		}
		if ev.LabName == "" {
			ev.LabName = reg.Lab(ev.ContainerID)
		}
		ev.NodeName = reg.Node(ev.ContainerID)
		if ev.NodeName != "" {
			if ev.Action == "delete" {
				store.RemoveInterface(ev.LabName, ev.NodeName, ev.Name)
				return nil
			}
			store.ApplyInterfaceEvent(*ev)
		}
		return nil
	default:
		return nil
	}
}

// isExecAction reports whether an event action is a docker exec artifact
// rather than a container lifecycle transition.
func isExecAction(action string) bool {
	return strings.HasPrefix(action, "exec_") || strings.HasPrefix(action, "exec:")
}

// source is the event stream runner for the containerlab CLI.
type source struct {
	bin    string
	store  *engine.Store
	reg    *actorRegistry
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func newSource(bin string, store *engine.Store, reg *actorRegistry) *source {
	return &source{bin: bin, store: store, reg: reg}
}

// Start spawns `containerlab events --format json --initial-state` and feeds
// parsed events into the store until the context is cancelled.
func (s *source) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return nil // already started
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.cancel = cancel
	s.done = done
	s.mu.Unlock()

	cmd := exec.CommandContext(ctx, s.bin, "events", "--format", "json", "--initial-state")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		s.mu.Lock()
		s.cancel = nil
		s.done = nil
		s.mu.Unlock()
		return err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		s.mu.Lock()
		s.cancel = nil
		s.done = nil
		s.mu.Unlock()
		return err
	}

	go func() {
		defer func() {
			s.mu.Lock()
			if s.done == done {
				s.cancel = nil
				s.done = nil
			}
			s.mu.Unlock()
			close(done)
		}()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			_ = handleRawEventLine(s.store, s.reg, line)
		}
		_ = cmd.Wait()
	}()

	return nil
}

// Stop cancels the event stream and waits for it to exit. Safe to call
// before Start or after a failed Start (returns immediately).
const sourceStopTimeout = 5 * time.Second

func (s *source) Stop() error {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()
	if cancel == nil || done == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		s.mu.Lock()
		if s.done == done {
			s.cancel = nil
			s.done = nil
		}
		s.mu.Unlock()
		return nil
	case <-time.After(sourceStopTimeout):
		return fmt.Errorf("containerlab events did not stop within %s", sourceStopTimeout)
	}
}
