package containerlab

import (
	"testing"
)

func TestParseDockerStats(t *testing.T) {
	m := parseDockerStats("0.00%|0B / 0B|0B / 0B|0")
	if m == nil {
		t.Fatal("expected non-nil")
	}
	if m.Pids != 0 || m.CPUPercent != 0 {
		t.Fatalf("expected zeros, got %+v", m)
	}

	m = parseDockerStats("12.5%|256MiB / 2GiB|1.5MB / 3.5kB|7")
	if m.CPUPercent != 12.5 {
		t.Fatalf("expected 12.5 CPU, got %f", m.CPUPercent)
	}
	if m.MemUsed != 256<<20 {
		t.Fatalf("expected 256MiB used, got %d", m.MemUsed)
	}
	if m.MemLimit != 2<<30 {
		t.Fatalf("expected 2GiB limit, got %d", m.MemLimit)
	}
	if m.NetRx != 1500000 {
		t.Fatalf("expected 1.5MB rx, got %d", m.NetRx)
	}
	if m.NetTx != 3500 {
		t.Fatalf("expected 3.5kB tx, got %d", m.NetTx)
	}
	if m.Pids != 7 {
		t.Fatalf("expected 7 pids, got %d", m.Pids)
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"", 0},
		{"0B", 0},
		{"512B", 512},
		{"1.2kB", 1200},
		{"3.5kB", 3500},
		{"10MiB", 10 << 20},
		{"1GiB", 1 << 30},
		{"1.5MB", 1500000},
		{"2.01MB", 2010000},
		{"42XB", 0}, // unknown suffix
		{"abc", 0},  // non-numeric
	}
	for _, c := range cases {
		if got := parseSize(c.in); got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
