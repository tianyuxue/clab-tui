package labfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// clabFile is the subset of a containerlab topology YAML we care about.
type clabFile struct {
	Name     string `yaml:"name"`
	Topology struct {
		Nodes map[string]struct {
			Kind  string `yaml:"kind"`
			Image string `yaml:"image"`
			Group string `yaml:"group"`
		} `yaml:"nodes"`
		Links []struct {
			Endpoints []string `yaml:"endpoints"`
		} `yaml:"links"`
	} `yaml:"topology"`
}

// IsLabFile reports whether name looks like a containerlab topology file.
func IsLabFile(name string) bool {
	return strings.HasSuffix(name, ".clab.yml") || strings.HasSuffix(name, ".clab.yaml")
}

// ScanLabs scans dir for *.clab.yml / *.clab.yaml files and returns a static
// Lab (structure only, live state unknown) for each. It does not contact the
// engine, so node states are StatusUnknown.
func ScanLabs(dir string) ([]*engine.Lab, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var labs []*engine.Lab
	for _, e := range entries {
		if e.IsDir() || !IsLabFile(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		lab, err := ParseTopology(path)
		if err != nil {
			return nil, err
		}
		labs = append(labs, lab)
	}
	return labs, nil
}

// ParseTopology reads a containerlab YAML file and returns its static
// topology (nodes with roles, links). Live state fields are left unknown.
func ParseTopology(path string) (*engine.Lab, error) {
	var cf clabFile
	if err := readYAML(path, &cf); err != nil {
		return nil, err
	}

	lab := &engine.Lab{Name: cf.Name, TopoFile: path}

	names := make([]string, 0, len(cf.Topology.Nodes))
	for name := range cf.Topology.Nodes {
		names = append(names, name)
	}
	sortStrings(names)

	for _, name := range names {
		n := cf.Topology.Nodes[name]
		lab.Nodes = append(lab.Nodes, engine.Node{
			Name: name, Kind: n.Kind, Image: n.Image, Group: n.Group,
			State: engine.StatusUnknown,
		})
	}

	for _, link := range cf.Topology.Links {
		if len(link.Endpoints) != 2 {
			continue
		}
		a, epA := parseEndpoint(link.Endpoints[0])
		b, epB := parseEndpoint(link.Endpoints[1])
		if a == "" || b == "" || epA == "" || epB == "" {
			continue
		}
		lab.Links = append(lab.Links, engine.Link{
			A: a, PortA: epA, B: b, PortB: epB, State: engine.LinkUnknown,
		})
	}
	return lab, nil
}

// parseEndpoint extracts the node name and port from an endpoint like
// "srl1:e1-1", returning ("srl1", "e1-1").
func parseEndpoint(endpoint string) (string, string) {
	if i := strings.IndexByte(endpoint, ':'); i >= 0 {
		return endpoint[:i], endpoint[i+1:]
	}
	return endpoint, ""
}

func readYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// sortStrings is a tiny local sort helper to keep imports minimal.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
