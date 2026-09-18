package labfile

import (
	"os"
	"path/filepath"
	"testing"
)

const roleLab = `name: dc-core
topology:
  nodes:
    spine-01:
      kind: arista_ceos
      image: ceos:4.32.0F
      group: spine
    leaf-01:
      kind: nokia_srlinux
      image: ghcr.io/nokia/srlinux:24.7.1
      group: leaf
    fw-01:
      kind: cisco_xrd
      image: cisco-xrd:7.10.1
      group: fw
  links:
    - endpoints: ["spine-01:eth1", "leaf-01:eth1"]
`

func writeRoleFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "role.clab.yml")
	if err := os.WriteFile(path, []byte(roleLab), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseTopology_ExtractsRoleFromGroup(t *testing.T) {
	path := writeRoleFixture(t)
	topo, err := ParseTopology(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(topo.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(topo.Nodes))
	}
	roles := map[string]string{}
	for _, n := range topo.Nodes {
		roles[n.Name] = n.Group
	}
	if roles["spine-01"] != "spine" {
		t.Fatalf("expected spine-01 role spine, got %q", roles["spine-01"])
	}
	if roles["leaf-01"] != "leaf" {
		t.Fatalf("expected leaf-01 role leaf, got %q", roles["leaf-01"])
	}
	if roles["fw-01"] != "fw" {
		t.Fatalf("expected fw-01 role fw, got %q", roles["fw-01"])
	}
}

func TestParseTopology_NoGroupIsEmptyRole(t *testing.T) {
	path := writeRoleFixture(t)
	// Remove group fields
	data, _ := os.ReadFile(path)
	content := string(data)
	content = removeLinesWith(content, "group:")
	os.WriteFile(path, []byte(content), 0644)

	topo, err := ParseTopology(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range topo.Nodes {
		if n.Group != "" {
			t.Fatalf("expected empty role for %s (no group field), got %q", n.Name, n.Group)
		}
	}
}

func TestParseTopology_ExtractsEndpoints(t *testing.T) {
	path := writeRoleFixture(t)
	topo, err := ParseTopology(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(topo.Links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(topo.Links))
	}
	l := topo.Links[0]
	if l.PortA != "eth1" {
		t.Fatalf("expected PortA eth1, got %q", l.PortA)
	}
	if l.A != "spine-01" {
		t.Fatalf("expected A spine-01, got %q", l.A)
	}
	if l.PortB != "eth1" {
		t.Fatalf("expected PortB eth1, got %q", l.PortB)
	}
}

// removeLinesWith removes all lines containing substr.
func removeLinesWith(s, substr string) string {
	var out []byte
	for _, line := range splitLines(s) {
		if !contains(line, substr) {
			out = append(out, []byte(line+"\n")...)
		}
	}
	return string(out)
}

func splitLines(s string) []string {
	var lines []string
	cur := ""
	for _, c := range s {
		if c == '\n' {
			lines = append(lines, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
