package components

import (
	"testing"
)

func TestNewStatusBar(t *testing.T) {
	s := NewStatusBar()
	if s.BackendStatus != "OK" {
		t.Fatalf("expected OK, got %s", s.BackendStatus)
	}
}

func TestStatusBarView(t *testing.T) {
	s := NewStatusBar()
	s.SetSize(80)
	s.LabCount = 5
	s.RunningCount = 2
	v := s.View()
	if v == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestStatusBarError(t *testing.T) {
	s := NewStatusBar()
	s.BackendStatus = "Error: connection lost"
	s.SetSize(80)
	v := s.View()
	if v == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestStatusBarSetSize(t *testing.T) {
	s := NewStatusBar()
	s.SetSize(100)
	if s.width != 100 {
		t.Fatalf("expected width 100, got %d", s.width)
	}
}
