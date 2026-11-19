package main

import (
	"reflect"
	"testing"
)

func TestStripSudoFlag(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"flag first", []string{"--sudo", "--dir", "/x"}, []string{"--dir", "/x"}},
		{"flag last", []string{"--dir", "/x", "--sudo"}, []string{"--dir", "/x"}},
		{"equals true", []string{"--sudo=true", "--file", "/x"}, []string{"--file", "/x"}},
		{"equals false", []string{"--sudo=false"}, []string{}},
		{"similar prefix", []string{"--sudolike", "x"}, []string{"--sudolike", "x"}},
		{"after delimiter", []string{"--dir", "--", "--sudo"}, []string{"--dir", "--", "--sudo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripSudoFlag(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("stripSudoFlag(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSudoArgv(t *testing.T) {
	got := sudoArgv("/home/u/.local/bin/clab-tui", []string{"--sudo", "--dir", "/labs"})
	want := []string{"sudo", "/home/u/.local/bin/clab-tui", "--dir", "/labs"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sudoArgv = %v, want %v", got, want)
	}
}

func TestSudoArgvKeepsDelimiterAndPositionals(t *testing.T) {
	got := sudoArgv("/usr/local/bin/clab-tui", []string{"--file", "/x", "--", "--sudo"})
	want := []string{"sudo", "/usr/local/bin/clab-tui", "--file", "/x", "--", "--sudo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sudoArgv = %v, want %v", got, want)
	}
}

func TestShouldElevate(t *testing.T) {
	tests := []struct {
		name string
		sudo bool
		euid int
		want bool
	}{
		{"requested by non-root", true, 1000, true},
		{"already root", true, 0, false},
		{"not requested", false, 1000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldElevate(tt.sudo, tt.euid); got != tt.want {
				t.Fatalf("shouldElevate(%v, %d) = %v, want %v", tt.sudo, tt.euid, got, tt.want)
			}
		})
	}
}
