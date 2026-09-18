package containerlab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tianyuxue/clab-tui/internal/engine"
	"github.com/tianyuxue/clab-tui/internal/labfile"
)

// ClabEngine implements engine.Engine backed by the containerlab CLI.
type ClabEngine struct {
	bin    string
	labDir string

	store *engine.Store
	reg   *actorRegistry
	src   *source

	mu      sync.Mutex
	start   bool
	statsMu sync.Mutex
	stats   map[string]interfaceStatsCounters
}

type Option func(*ClabEngine)

func WithBinary(path string) Option {
	return func(e *ClabEngine) { e.bin = path }
}

// WithLabDir sets the directory scanned for .clab.yml files.
func WithLabDir(dir string) Option {
	return func(e *ClabEngine) { e.labDir = dir }
}

// New constructs a ClabEngine. The event stream is started lazily on the
// first query that needs live state (GetLab/ListLabs).
func New(opts ...Option) (*ClabEngine, error) {
	e := &ClabEngine{
		store:  engine.NewStore(),
		reg:    newActorRegistry(),
		labDir: ".",
		stats:  map[string]interfaceStatsCounters{},
	}
	for _, o := range opts {
		o(e)
	}
	if e.bin == "" {
		path, err := findBinary()
		if err != nil {
			return nil, err
		}
		e.bin = path
	}
	return e, nil
}

// NewWithFallback returns an engine.Engine, or nil if containerlab is
// unavailable (limited mode: topology from local files only). A nil engine
// keeps the TUI running without live state. labDir is the directory scanned
// for .clab.yml files and must match the TUI's lab discovery directory.
func NewWithFallback(binPath, labDir string) engine.Engine {
	e, err := New(WithBinary(binPath), WithLabDir(labDir))
	if err != nil {
		return nil
	}
	return e
}

func (e *ClabEngine) ensureStarted(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.start {
		return nil
	}
	e.src = newSource(e.bin, e.store, e.reg)
	if err := e.src.Start(ctx); err != nil {
		return err
	}
	// Seed from inspect as a fallback so the initial snapshot is populated
	// even if the event stream is not yet reporting.
	e.seedFromInspect(ctx)
	e.start = true
	return nil
}

func (e *ClabEngine) seedFromInspect(ctx context.Context) {
	lines, err := spawnWithLines(ctx, e.bin, "inspect", "--all", "--format", "json")
	if err != nil {
		return
	}
	var raw string
	for line := range lines {
		if line.Done {
			break
		}
		if line.Stream == "stdout" {
			raw += line.Line
		}
	}
	_ = applyInspectData(e.store, e.reg, []byte(raw))

	// Seed per-container interface states too, so link states can be derived
	// even before the events stream reports.
	ilines, ierr := spawnWithLines(ctx, e.bin, "inspect", "interfaces", "--format", "json")
	if ierr != nil {
		return
	}
	var iraw string
	for line := range ilines {
		if line.Done {
			break
		}
		if line.Stream == "stdout" {
			iraw += line.Line
		}
	}
	_ = applyInspectInterfaces(e.store, e.reg, []byte(iraw))
}

// Snapshot returns the live snapshot (event stream + inspect seeded).
func (e *ClabEngine) Snapshot() *engine.Snapshot {
	return e.store.Snapshot()
}

func (e *ClabEngine) Subscribe() (<-chan engine.Change, func()) {
	return e.store.Subscribe()
}

// GetLab merges static topology from .clab.yml with live node state.
func (e *ClabEngine) GetLab(ctx context.Context, labName string) (*engine.Lab, error) {
	topo, err := e.findTopoFile(labName)
	if err != nil {
		return nil, engine.ErrLabNotFound
	}
	lab, err := labfile.ParseTopology(topo)
	if err != nil {
		return nil, err
	}
	e.ensureStarted(ctx)
	live := e.store.Snapshot().Labs[labName]
	if live != nil {
		mergeLabState(lab, live)
		e.store.SetLinks(labName, lab.Links)
		// Re-read derived link states so the returned lab reflects real
		// interface-derived up/down/unknown rather than static unknown.
		if snap := e.store.Snapshot().Labs[labName]; snap != nil {
			lab.Links = snap.Links
		}
	} else {
		// Lab is not deployed: its links are genuinely down, not unknown.
		for i := range lab.Links {
			lab.Links[i].State = engine.LinkDown
		}
	}
	return lab, nil
}

func (e *ClabEngine) GetNode(ctx context.Context, labName, nodeName string) (*engine.Node, error) {
	lab, err := e.GetLab(ctx, labName)
	if err != nil {
		return nil, err
	}
	for i := range lab.Nodes {
		if lab.Nodes[i].Name == nodeName {
			return &lab.Nodes[i], nil
		}
	}
	return nil, engine.ErrNodeNotFound
}

// ListLabs returns labs found by scanning labDir for .clab.yml files.
func (e *ClabEngine) ListLabs(ctx context.Context) ([]*engine.Lab, error) {
	e.ensureStarted(ctx)
	entries, err := os.ReadDir(e.labDir)
	if err != nil {
		return nil, err
	}
	var labs []*engine.Lab
	for _, entry := range entries {
		if entry.IsDir() || !isClabFile(entry.Name()) {
			continue
		}
		lab, err := labfile.ParseTopology(filepath.Join(e.labDir, entry.Name()))
		if err != nil {
			continue
		}
		live := e.store.Snapshot().Labs[lab.Name]
		if live != nil {
			mergeLabState(lab, live)
			e.store.SetLinks(lab.Name, lab.Links)
			if snap := e.store.Snapshot().Labs[lab.Name]; snap != nil {
				lab.Links = snap.Links
			}
		} else {
			// Lab is not deployed: its links are genuinely down, not unknown.
			for i := range lab.Links {
				lab.Links[i].State = engine.LinkDown
			}
		}
		labs = append(labs, lab)
	}
	return labs, nil
}

func (e *ClabEngine) findTopoFile(labName string) (string, error) {
	entries, err := os.ReadDir(e.labDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() || !isClabFile(entry.Name()) {
			continue
		}
		lab, err := labfile.ParseTopology(filepath.Join(e.labDir, entry.Name()))
		if err != nil {
			continue
		}
		if lab.Name == labName {
			return filepath.Join(e.labDir, entry.Name()), nil
		}
	}
	return "", os.ErrNotExist
}

func (e *ClabEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.src != nil {
		if err := e.src.Stop(); err != nil {
			return err
		}
		e.src = nil
	}
	return nil
}

func isClabFile(name string) bool {
	return strings.HasSuffix(name, ".clab.yml") || strings.HasSuffix(name, ".clab.yaml")
}

// mergeLabState overlays live node state (status/IP/interfaces) onto the
// static topology lab.
func mergeLabState(static, live *engine.Lab) *engine.Lab {
	if static == nil || live == nil {
		return static
	}
	liveByNode := make(map[string]*engine.Node, len(live.Nodes))
	for i := range live.Nodes {
		liveByNode[live.Nodes[i].Name] = &live.Nodes[i]
	}
	for i := range static.Nodes {
		if ln, ok := liveByNode[static.Nodes[i].Name]; ok {
			static.Nodes[i].State = ln.State
			static.Nodes[i].Status = ln.Status
			static.Nodes[i].IPv4 = ln.IPv4
			static.Nodes[i].IPv6 = ln.IPv6
			static.Nodes[i].StartedAt = ln.StartedAt
			static.Nodes[i].Interfaces = ln.Interfaces
			static.Nodes[i].Container = ln.Container
		}
	}
	return static
}
