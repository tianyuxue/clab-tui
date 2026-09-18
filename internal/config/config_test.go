package config

import (
	"os"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Theme != "default" {
		t.Fatalf("expected default theme, got %s", c.Theme)
	}
	if c.Containerlab.Timeout != 300 {
		t.Fatalf("expected default timeout 300, got %d", c.Containerlab.Timeout)
	}
}

func TestPath(t *testing.T) {
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", "/tmp/clab-tui-test-xdg")
	defer func() {
		if oldXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", oldXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
		os.RemoveAll("/tmp/clab-tui-test-xdg")
	}()
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if path != "/tmp/clab-tui-test-xdg/clab-tui/config.yaml" {
		t.Fatalf("unexpected path: %s", path)
	}
}

func TestLoad_NotExist(t *testing.T) {
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", "/tmp/clab-tui-test-load")
	defer func() {
		if oldXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", oldXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
		os.RemoveAll("/tmp/clab-tui-test-load")
	}()
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "default" {
		t.Fatalf("expected default config, got %+v", cfg)
	}
}

func TestSaveAndLoad(t *testing.T) {
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", "/tmp/clab-tui-test-save")
	defer func() {
		if oldXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", oldXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
		os.RemoveAll("/tmp/clab-tui-test-save")
	}()
	cfg := Default()
	cfg.Containerlab.BinPath = "/usr/local/bin/containerlab"
	if err := Save(&cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Containerlab.BinPath != "/usr/local/bin/containerlab" {
		t.Fatalf("expected saved bin_path, got %s", loaded.Containerlab.BinPath)
	}
}
