package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLabFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("name: "+name+"\ntopology:\n  nodes:\n    r1:\n      kind: linux\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveRunOptionsFile(t *testing.T) {
	dir := t.TempDir()
	file := writeLabFile(t, dir, "lab1.clab.yml")
	opts, err := resolveRunOptions("", file)
	if err != nil {
		t.Fatal(err)
	}
	if opts.labDir != dir {
		t.Fatalf("labDir = %q, want %q", opts.labDir, dir)
	}
	if opts.initialLab == nil {
		t.Fatal("expected initialLab to be set")
	}
	if opts.initialLab.Name != "lab1.clab.yml" {
		t.Fatalf("initialLab.Name = %q, want %q", opts.initialLab.Name, "lab1.clab.yml")
	}
	if opts.initialLab.TopoFile != file {
		t.Fatalf("initialLab.TopoFile = %q, want %q", opts.initialLab.TopoFile, file)
	}
}

func TestResolveRunOptionsDir(t *testing.T) {
	dir := t.TempDir()
	writeLabFile(t, dir, "lab1.clab.yml")
	opts, err := resolveRunOptions(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.labDir != dir {
		t.Fatalf("labDir = %q, want %q", opts.labDir, dir)
	}
	if opts.initialLab != nil {
		t.Fatal("expected initialLab to be nil for --dir")
	}
}

func TestResolveRunOptionsDefaultUsesCwd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	opts, err := resolveRunOptions("", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.labDir != dir {
		t.Fatalf("labDir = %q, want cwd %q", opts.labDir, dir)
	}
	if opts.initialLab != nil {
		t.Fatal("expected initialLab to be nil for default")
	}
}

func TestResolveRunOptionsMutuallyExclusive(t *testing.T) {
	if _, err := resolveRunOptions("/tmp", "/tmp/lab.clab.yml"); err == nil {
		t.Fatal("expected error when both --dir and --file given")
	}
}

func TestResolveRunOptionsFileMissing(t *testing.T) {
	if _, err := resolveRunOptions("", "/nonexistent/lab.clab.yml"); err == nil {
		t.Fatal("expected error for missing --file")
	}
}

func TestResolveRunOptionsFileNotLab(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRunOptions("", path); err == nil {
		t.Fatal("expected error for non-lab --file")
	}
}

func TestResolveRunOptionsFileIsDir(t *testing.T) {
	dir := t.TempDir()
	if _, err := resolveRunOptions("", dir); err == nil {
		t.Fatal("expected error when --file is a directory")
	}
}

func TestResolveRunOptionsDirMissing(t *testing.T) {
	if _, err := resolveRunOptions("/nonexistent/dir", ""); err == nil {
		t.Fatal("expected error for missing --dir")
	}
}

func TestRootCmdRegistersDirAndFileFlags(t *testing.T) {
	cmd := newRootCmd()
	dir := cmd.Flags().Lookup("dir")
	if dir == nil {
		t.Fatal("expected --dir flag")
	}
	file := cmd.Flags().Lookup("file")
	if file == nil {
		t.Fatal("expected --file flag")
	}
}

func TestRootCmdDirFileMutuallyExclusive(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--dir", "/tmp", "--file", "/tmp/lab.clab.yml"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when both --dir and --file given")
	}
}

func TestResolveContainerlabBinReturnsConfiguredPath(t *testing.T) {
	if got := resolveContainerlabBin("/custom/clab"); got != "/custom/clab" {
		t.Fatalf("resolveContainerlabBin = %q, want configured path unchanged", got)
	}
}

func TestResolveContainerlabBinFallsBackToPATH(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "containerlab")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	got := resolveContainerlabBin("")
	if got != bin {
		t.Fatalf("resolveContainerlabBin(\"\") = %q, want PATH-resolved %q", got, bin)
	}
}

func TestResolveContainerlabBinEmptyWhenNotFound(t *testing.T) {
	t.Setenv("PATH", filepath.Join(t.TempDir(), "nonexistent"))
	if got := resolveContainerlabBin(""); got != "" {
		t.Fatalf("resolveContainerlabBin(\"\") = %q, want empty when not on PATH", got)
	}
}

func TestRootCmdVersionFlag(t *testing.T) {
	old := version
	version = "v9.9.9-test"
	t.Cleanup(func() { version = old })

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "v9.9.9-test") {
		t.Fatalf("version output = %q, want it to contain %q", buf.String(), "v9.9.9-test")
	}
}
