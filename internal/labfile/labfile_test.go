package labfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.clab.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

const twoNodeLab = `name: srlinux-ceos-lab
topology:
  nodes:
    srl1:
      kind: nokia_srlinux
      image: ghcr.io/nokia/srlinux:24.7.1
    ceos1:
      kind: arista_ceos
      image: ceos:4.32.0F
  links:
    - endpoints: ["srl1:e1-1", "ceos1:eth1"]
`

func TestParseTopology_ExtractsNodes(t *testing.T) {
	path := writeFixture(t, twoNodeLab)

	lab, err := ParseTopology(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lab.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(lab.Nodes))
	}
	// Nodes are returned in deterministic sorted order (ceos1 < srl1).
	if lab.Nodes[0].Name != "ceos1" {
		t.Fatalf("expected first node ceos1 (sorted), got %s", lab.Nodes[0].Name)
	}
	if lab.Nodes[1].Name != "srl1" {
		t.Fatalf("expected second node srl1, got %s", lab.Nodes[1].Name)
	}
	if lab.Nodes[1].Kind != "nokia_srlinux" {
		t.Fatalf("expected kind nokia_srlinux, got %s", lab.Nodes[1].Kind)
	}
	if lab.Nodes[0].State != engine.StatusUnknown {
		t.Fatalf("static topology nodes should be StatusUnknown, got %v", lab.Nodes[0].State)
	}
}

func TestParseTopology_ExtractsLinks(t *testing.T) {
	path := writeFixture(t, twoNodeLab)

	lab, err := ParseTopology(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lab.Links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(lab.Links))
	}
	if lab.Links[0].A != "srl1" || lab.Links[0].B != "ceos1" {
		t.Fatalf("expected srl1-ceos1 link, got %+v", lab.Links[0])
	}
	if lab.Links[0].PortA != "e1-1" || lab.Links[0].PortB != "eth1" {
		t.Fatalf("unexpected endpoints: %+v", lab.Links[0])
	}
	if lab.Links[0].State != engine.LinkUnknown {
		t.Fatalf("static links should be LinkUnknown, got %v", lab.Links[0].State)
	}
}

func TestParseTopology_FileNotFound(t *testing.T) {
	_, err := ParseTopology("/nonexistent/path.clab.yml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseTopology_InvalidYAML(t *testing.T) {
	path := writeFixture(t, "not: [valid yaml\n  broken")

	_, err := ParseTopology(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParseTopology_NoNodes(t *testing.T) {
	path := writeFixture(t, "name: empty-lab\ntopology:\n  nodes: {}\n")

	lab, err := ParseTopology(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lab.Nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(lab.Nodes))
	}
}

func TestScanLabs_FindsYmlFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.clab.yml"), []byte("name: lab-a\ntopology:\n  nodes:\n    n1: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.clab.yaml"), []byte("name: lab-b\ntopology:\n  nodes:\n    n1: {}\n    n2: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Non-lab file should be ignored
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	labs, err := ScanLabs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(labs) != 2 {
		t.Fatalf("expected 2 labs, got %d", len(labs))
	}
}

func TestScanLabs_ParsesNameAndNodes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.clab.yml"), []byte(twoNodeLab), 0644); err != nil {
		t.Fatal(err)
	}

	labs, err := ScanLabs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(labs) != 1 {
		t.Fatalf("expected 1 lab, got %d", len(labs))
	}
	if labs[0].Name != "srlinux-ceos-lab" {
		t.Fatalf("expected srlinux-ceos-lab, got %s", labs[0].Name)
	}
	if len(labs[0].Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(labs[0].Nodes))
	}
	if labs[0].TopoFile == "" {
		t.Fatal("expected TopoFile to be set")
	}
}

func TestScanLabs_EmptyDir(t *testing.T) {
	labs, err := ScanLabs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(labs) != 0 {
		t.Fatalf("expected 0 labs, got %d", len(labs))
	}
}

func TestParseTopology_DropsLinksWithMissingPort(t *testing.T) {
	path := writeFixture(t, `name: malformed
topology:
  nodes:
    r1: {kind: linux}
    r2: {kind: linux}
  links:
    - endpoints: ["r1", "r2:eth1"]    # r1 has no port -> dropped
    - endpoints: ["r1:eth1", "r2:"]   # r2 has empty port -> dropped
    - endpoints: ["r1:eth1", "r2:eth1"] # valid -> kept
`)

	lab, err := ParseTopology(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lab.Links) != 1 {
		t.Fatalf("expected 1 valid link, got %d: %+v", len(lab.Links), lab.Links)
	}
	if lab.Links[0].A != "r1" || lab.Links[0].PortA != "eth1" {
		t.Fatalf("unexpected link: %+v", lab.Links[0])
	}
}
