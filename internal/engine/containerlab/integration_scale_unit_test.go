//go:build integration

package containerlab

import "testing"

func TestParseClos10ListenerPID(t *testing.T) {
	pid, err := parseClos10ListenerPID("1234\n")
	if err != nil || pid != 1234 {
		t.Fatalf("parseClos10ListenerPID = %d, %v; want 1234", pid, err)
	}
	for _, stdout := range []string{"", "1234 5678\n", "not-a-pid\n", "0\n"} {
		if _, err := parseClos10ListenerPID(stdout); err == nil {
			t.Fatalf("parseClos10ListenerPID(%q) accepted invalid output", stdout)
		}
	}
}

func TestClos10TraceInspectionCommandUsesNodeSpecificTools(t *testing.T) {
	for _, node := range []string{"spine1", "leaf1", "border1"} {
		command := clos10TraceInspectionCommand(node)
		if command != "command -v ip >/dev/null && command -v tc >/dev/null" {
			t.Fatalf("tools for %s = %q, want ip/tc only", node, command)
		}
	}
	for _, node := range []string{"host1", "host2"} {
		command := clos10TraceInspectionCommand(node)
		if command != "command -v ip >/dev/null && command -v tc >/dev/null && command -v nc >/dev/null" {
			t.Fatalf("tools for %s = %q, want ip/tc/nc", node, command)
		}
	}
}
