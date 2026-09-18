package engine

import "testing"

func TestLabStatusEmpty(t *testing.T) {
	lab := &Lab{}
	if got := lab.Status(); got != LabStatusUnknown {
		t.Fatalf("expected Unknown, got %v", got)
	}
}

func TestLabStatusAllRunning(t *testing.T) {
	lab := &Lab{Nodes: []Node{{State: StatusRunning}, {State: StatusRunning}}}
	if got := lab.Status(); got != LabStatusRunning {
		t.Fatalf("expected Running, got %v", got)
	}
}

func TestLabStatusAllStopped(t *testing.T) {
	lab := &Lab{Nodes: []Node{{State: StatusStopped}, {State: StatusCreated}}}
	if got := lab.Status(); got != LabStatusStopped {
		t.Fatalf("expected Stopped, got %v", got)
	}
}

func TestLabStatusMixed(t *testing.T) {
	lab := &Lab{Nodes: []Node{{State: StatusRunning}, {State: StatusStopped}}}
	if got := lab.Status(); got != LabStatusPartial {
		t.Fatalf("expected Partial, got %v", got)
	}
}
