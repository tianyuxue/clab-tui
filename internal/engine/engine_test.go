package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
)

type fakeEngine struct {
	lab *Lab
}

func (f *fakeEngine) Snapshot() *Snapshot {
	return &Snapshot{Labs: map[string]*Lab{"mini": f.lab}}
}
func (f *fakeEngine) Subscribe() (<-chan Change, func()) {
	return make(chan Change), func() {}
}
func (f *fakeEngine) GetLab(ctx context.Context, labName string) (*Lab, error) {
	if f.lab == nil || f.lab.Name != labName {
		return nil, ErrLabNotFound
	}
	return f.lab, nil
}
func (f *fakeEngine) GetNode(ctx context.Context, labName, nodeName string) (*Node, error) {
	if f.lab == nil || f.lab.Name != labName {
		return nil, ErrLabNotFound
	}
	for i := range f.lab.Nodes {
		if f.lab.Nodes[i].Name == nodeName {
			return &f.lab.Nodes[i], nil
		}
	}
	return nil, ErrNodeNotFound
}
func (f *fakeEngine) ListLabs(ctx context.Context) ([]*Lab, error) {
	if f.lab == nil {
		return nil, nil
	}
	return []*Lab{f.lab}, nil
}
func (f *fakeEngine) Deploy(ctx context.Context, labPath string, opts ...DeployOption) (<-chan OutputLine, error) {
	return nil, ErrNotSupported
}
func (f *fakeEngine) Destroy(ctx context.Context, labName string, opts ...DestroyOption) (<-chan OutputLine, error) {
	return nil, ErrNotSupported
}
func (f *fakeEngine) Redeploy(ctx context.Context, labName string, opts ...RedeployOption) (<-chan OutputLine, error) {
	return nil, ErrNotSupported
}
func (f *fakeEngine) Save(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error) {
	return nil, ErrNotSupported
}
func (f *fakeEngine) StartNode(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error) {
	return nil, ErrNotSupported
}
func (f *fakeEngine) StopNode(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error) {
	return nil, ErrNotSupported
}
func (f *fakeEngine) RestartNode(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error) {
	return nil, ErrNotSupported
}
func (f *fakeEngine) PauseNode(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error) {
	return nil, ErrNotSupported
}
func (f *fakeEngine) UnpauseNode(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error) {
	return nil, ErrNotSupported
}
func (f *fakeEngine) Exec(ctx context.Context, labName, nodeName, cmd string) (*ExecResult, error) {
	return nil, ErrNotSupported
}
func (f *fakeEngine) Close() error { return nil }

func TestEngineInterfaceContract(t *testing.T) {
	var _ Engine = (*fakeEngine)(nil) // compile-time check

	f := &fakeEngine{lab: &Lab{Name: "mini", Nodes: []Node{{Name: "r1"}}}}
	lab, err := f.GetLab(context.Background(), "mini")
	if err != nil {
		t.Fatal(err)
	}
	if lab.Name != "mini" || len(lab.Nodes) != 1 {
		t.Fatalf("unexpected lab: %+v", lab)
	}
	if _, err := f.GetLab(context.Background(), "nope"); err != ErrLabNotFound {
		t.Fatalf("expected ErrLabNotFound, got %v", err)
	}
}

func TestCaptureWarningErrorFormatsDirectionIssues(t *testing.T) {
	ingressErr := errors.New("operation not permitted")
	egressErr := errors.New("device busy")
	tests := []struct {
		name   string
		issues []CaptureIssue
		want   string
	}{
		{
			name:   "ingress only",
			issues: []CaptureIssue{{Node: "r1", Interface: "e1-1", Direction: "ingress", Err: ingressErr}},
			want:   "r1:e1-1 ingress attach failed: operation not permitted",
		},
		{
			name:   "egress only",
			issues: []CaptureIssue{{Node: "r1", Interface: "e1-1", Direction: "egress", Err: egressErr}},
			want:   "r1:e1-1 egress attach failed: device busy",
		},
		{
			name: "both directions",
			issues: []CaptureIssue{
				{Node: "r1", Interface: "e1-1", Direction: "ingress", Err: ingressErr},
				{Node: "r1", Interface: "e1-1", Direction: "egress", Err: egressErr},
			},
			want: "r1:e1-1 ingress attach failed: operation not permitted; r1:e1-1 egress attach failed: device busy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warning := &CaptureWarningError{Issues: tt.issues}
			if got := warning.Error(); got != "capture started with warnings: "+tt.want {
				t.Fatalf("Error() = %q, want %q", got, "capture started with warnings: "+tt.want)
			}
			if !errors.Is(warning, ingressErr) && tt.name != "egress only" {
				t.Fatalf("warning does not unwrap ingress error")
			}
			if tt.name == "egress only" && !errors.Is(warning, egressErr) {
				t.Fatalf("warning does not unwrap egress error")
			}
		})
	}
}

func TestEngineErrorIs(t *testing.T) {
	if !errors.Is(ErrLabNotFound, ErrLabNotFound) {
		t.Fatal("ErrLabNotFound should match itself")
	}
	derived := &EngineError{Code: "lab_not_found", Message: "no such lab"}
	if !errors.Is(derived, ErrLabNotFound) {
		t.Fatal("derived error should match ErrLabNotFound via Code")
	}
}

func TestEngineErrorWrapped(t *testing.T) {
	wrapped := fmt.Errorf("deploy failed: %w", ErrNotSupported)
	if !errors.Is(wrapped, ErrNotSupported) {
		t.Fatal("wrapped ErrNotSupported should be matched via Unwrap")
	}
	// EngineError wrapping a stdlib error matches the stdlib error too.
	inner := io.EOF
	ee := &EngineError{Code: "x", Err: inner}
	if !errors.Is(ee, io.EOF) {
		t.Fatal("EngineError wrapping io.EOF should match via Unwrap")
	}
}

func TestEngineErrorNilReceiver(t *testing.T) {
	var e *EngineError
	if errors.Is(e, ErrLabNotFound) {
		t.Fatal("nil EngineError should not match")
	}
}

func TestOptionalCapabilityAssertion(t *testing.T) {
	f := &fakeEngine{}
	if _, ok := interface{}(f).(LogStreamer); ok {
		t.Fatal("fakeEngine should not implement LogStreamer")
	}
	var _ LogStreamer = (*fakeLogStreamer)(nil)
}

type fakeLogStreamer struct{}

func (f *fakeLogStreamer) StreamNodeLogs(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error) {
	return nil, nil
}
