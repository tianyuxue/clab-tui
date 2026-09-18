package main

import (
	"os"
	"testing"

	"github.com/tianyuxue/clab-tui/internal/labfile"
)

func TestScanLabs_FindsClabYml(t *testing.T) {
	labs, err := labfile.ScanLabs("../../testdata/simple")
	if err != nil {
		t.Fatal(err)
	}
	if len(labs) == 0 {
		t.Fatal("expected to find lab files in testdata/simple")
	}
	found := false
	for _, l := range labs {
		if l.Name == "srlinux-ceos-lab" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected srlinux-ceos-lab, got %+v", labs)
	}
}

func TestScanLabs_FindsClabYaml(t *testing.T) {
	labs, err := labfile.ScanLabs("../../testdata/subdir")
	if err != nil {
		t.Fatal(err)
	}
	if len(labs) == 0 {
		t.Fatal("expected to find .clab.yaml files in testdata/subdir")
	}
	found := false
	for _, l := range labs {
		if l.Name == "nested-lab" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected nested-lab, got %+v", labs)
	}
}

func TestScanLabs_NotRecursive(t *testing.T) {
	labs, err := labfile.ScanLabs("../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range labs {
		if l.Name == "three-node-mesh" {
			t.Fatal("expected non-recursive scan - should not find files in subdirs")
		}
	}
}

func TestScanLabs_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "clab-tui-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	labs, err := labfile.ScanLabs(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(labs) != 0 {
		t.Fatalf("expected 0 labs in empty dir, got %d", len(labs))
	}
}

func TestScanLabs_NonExistentDir(t *testing.T) {
	_, err := labfile.ScanLabs("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}
