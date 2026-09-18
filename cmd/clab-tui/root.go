package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tianyuxue/clab-tui/internal/capability"
	"github.com/tianyuxue/clab-tui/internal/config"
	"github.com/tianyuxue/clab-tui/internal/engine"
	clabengine "github.com/tianyuxue/clab-tui/internal/engine/containerlab"
	"github.com/tianyuxue/clab-tui/internal/labfile"
	"github.com/tianyuxue/clab-tui/internal/tui"
)

// runOptions carries the resolved startup configuration.
type runOptions struct {
	labDir     string
	initialLab *engine.Lab
}

// resolveRunOptions derives the lab directory and optional initial lab from the
// --dir / --file flags. They are mutually exclusive.
func resolveRunOptions(dir, file string) (runOptions, error) {
	if dir != "" && file != "" {
		return runOptions{}, fmt.Errorf("--dir and --file are mutually exclusive")
	}
	if file != "" {
		abs, err := filepath.Abs(file)
		if err != nil {
			return runOptions{}, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return runOptions{}, err
		}
		if info.IsDir() {
			return runOptions{}, fmt.Errorf("--file %s is a directory", file)
		}
		if !labfile.IsLabFile(abs) {
			return runOptions{}, fmt.Errorf("--file %s is not a *.clab.yml or *.clab.yaml file", file)
		}
		lab, err := labfile.ParseTopology(abs)
		if err != nil {
			return runOptions{}, err
		}
		return runOptions{labDir: filepath.Dir(abs), initialLab: lab}, nil
	}

	labDir := dir
	if labDir == "" {
		var err error
		labDir, err = os.Getwd()
		if err != nil {
			return runOptions{}, err
		}
	} else {
		abs, err := filepath.Abs(labDir)
		if err != nil {
			return runOptions{}, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return runOptions{}, err
		}
		if !info.IsDir() {
			return runOptions{}, fmt.Errorf("--dir %s is not a directory", dir)
		}
		labDir = abs
	}
	return runOptions{labDir: labDir}, nil
}

// resolveContainerlabBin returns the configured binary path, or resolves
// containerlab from PATH when no path is configured. It returns "" when the
// binary cannot be found, in which case SUID detection simply reports false.
func resolveContainerlabBin(binPath string) string {
	if binPath != "" {
		return binPath
	}
	if p, err := exec.LookPath("containerlab"); err == nil {
		return p
	}
	return ""
}

// runTUI loads config, scans labs, and runs the Bubble Tea program.
func runTUI(opts runOptions) error {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "clab-tui: warning: config error %v (using defaults)\n", err)
		cfg = nil
	}

	labs, err := labfile.ScanLabs(opts.labDir)
	if err != nil {
		return fmt.Errorf("scanning %s: %w", opts.labDir, err)
	}
	fmt.Fprintf(os.Stderr, "clab-tui: found %d lab files in %s\n", len(labs), opts.labDir)

	binPath := ""
	if cfg != nil {
		binPath = cfg.Containerlab.BinPath
	}
	binPath = resolveContainerlabBin(binPath)
	eng := clabengine.NewWithFallback(binPath, opts.labDir)
	caps := capability.Detect(binPath)

	var model *tui.Model
	if opts.initialLab != nil {
		model = tui.New(eng, labs, tui.WithInitialLab(opts.initialLab), tui.WithCapabilities(caps))
	} else {
		model = tui.New(eng, labs, tui.WithCapabilities(caps))
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// newRootCmd builds the clab-tui root command.
func newRootCmd() *cobra.Command {
	var dir string
	var file string
	cmd := &cobra.Command{
		Use:           "clab-tui",
		Short:         "terminal UI for containerlab",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := resolveRunOptions(dir, file)
			if err != nil {
				return err
			}
			return runTUI(opts)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "directory to scan for .clab.yml files (default: current dir)")
	cmd.Flags().StringVar(&file, "file", "", "open a single .clab.yml topology file directly")
	return cmd
}
