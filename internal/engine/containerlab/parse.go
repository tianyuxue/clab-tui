// Golden testdata in ./testdata was captured from a real 2-node lab
// (kind: linux, debian:bookworm) on 2026-08-13 against the containerlab
// build installed at that date. Attribute key sets (clab-node-name, mgmt_ipv4,
// lab, containerlab, origin) are version-sensitive; re-capture if they drift.
//
// Origin values emitted by this package are "inspect", "snapshot", and
// "netlink" (the events stream's raw origin plus inspect seeding).
package containerlab

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// ---- raw wire structs ----

type rawEventLine struct {
	Timestamp   string            `json:"timestamp"`
	Type        string            `json:"type"`
	Action      string            `json:"action"`
	ActorID     string            `json:"actor_id"`
	ActorName   string            `json:"actor_name"`
	ActorFullID string            `json:"actor_full_id"`
	Attributes  map[string]string `json:"attributes"`
}

type rawInspectContainer struct {
	LabName     string `json:"lab_name"`
	LabPath     string `json:"labPath"`
	AbsLabPath  string `json:"absLabPath"`
	Name        string `json:"name"`
	ContainerID string `json:"container_id"`
	Image       string `json:"image"`
	Kind        string `json:"kind"`
	Group       string `json:"group"`
	State       string `json:"state"`
	Status      string `json:"status"`
	IPv4        string `json:"ipv4_address"`
	IPv6        string `json:"ipv6_address"`
	Owner       string `json:"owner"`
}

type rawInspectInterfaces struct {
	Name       string                `json:"name"`
	Interfaces []rawInspectInterface `json:"interfaces"`
}

type rawInspectInterface struct {
	Name    string `json:"name"`
	Alias   string `json:"alias"`
	MAC     string `json:"mac"`
	Ifindex int    `json:"ifindex"`
	MTU     int    `json:"mtu"`
	Type    string `json:"type"`
	State   string `json:"state"`
}

// ---- event parsing ----

// parseContainerEventLine parses a single "container" event JSON line into a
// normalized ContainerEvent.
func parseContainerEventLine(line string) (*engine.ContainerEvent, error) {
	var raw rawEventLine
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, err
	}
	if raw.Type != "container" {
		return nil, fmt.Errorf("not a container event: type=%q", raw.Type)
	}
	attrs := raw.Attributes
	// containerlab events carry an action but usually no state attribute;
	// derive the normalized state from the action.
	state := engine.NormalizeStatus(attrs["state"])
	if state == engine.StatusUnknown {
		state = stateFromAction(raw.Action)
	}
	ev := &engine.ContainerEvent{
		LabName:     attrs["containerlab"],
		NodeName:    attrs["clab-node-name"],
		Container:   attrs["clab-node-longname"],
		ContainerID: raw.ActorID,
		Kind:        attrs["clab-node-kind"],
		Image:       attrs["image"],
		Group:       attrs["clab-node-group"],
		Type:        attrs["clab-node-type"],
		State:       state,
		Status:      attrs["status"],
		IPv4:        attrs["mgmt_ipv4"],
		IPv6:        attrs["mgmt_ipv6"],
		Owner:       attrs["clab-owner"],
		TopoFile:    attrs["clab-topo-file"],
		Origin:      attrs["origin"],
		Action:      raw.Action,
	}
	if ev.NodeName == "" {
		ev.NodeName = attrs["name"]
	}
	if ev.Container == "" {
		ev.Container = raw.ActorName
	}
	if ev.LabName == "" {
		ev.LabName = attrs["lab"]
	}
	return ev, nil
}

// parseInterfaceEventLine parses a single "interface" event JSON line. The
// NodeName field must be filled in by the caller from the actor_id mapping.
func parseInterfaceEventLine(line string) (*engine.InterfaceEvent, error) {
	var raw rawEventLine
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, err
	}
	if raw.Type != "interface" {
		return nil, fmt.Errorf("not an interface event: type=%q", raw.Type)
	}
	attrs := raw.Attributes
	ev := &engine.InterfaceEvent{
		LabName:     attrs["lab"],
		ContainerID: raw.ActorID,
		Name:        attrs["ifname"],
		Alias:       attrs["alias"],
		Type:        attrs["type"],
		MAC:         attrs["mac"],
		Index:       parseInt(attrs["index"]),
		MTU:         parseInt(attrs["mtu"]),
		State:       attrs["state"],
		Action:      raw.Action,
		Origin:      attrs["origin"],
	}
	return ev, nil
}

// stateFromAction maps a containerlab event action onto a node status. The
// events stream carries an action but typically no state attribute, so the
// lifecycle transition has to be inferred from the action name.
func stateFromAction(action string) engine.Status {
	switch action {
	case "start", "restart", "unpause":
		return engine.StatusRunning
	case "die", "stop", "kill", "destroy":
		return engine.StatusStopped
	case "pause":
		return engine.StatusPaused
	}
	return engine.StatusUnknown
}

// parseInt returns 0 for empty or malformed input; that is acceptable here
// because the events stream occasionally carries non-numeric placeholder
// values for index/mtu.
func parseInt(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// ---- inspect parsing ----

// parseInspectContainers parses `containerlab inspect -f json` (a map keyed
// by lab name) into ContainerEvents.
func parseInspectContainers(data []byte) ([]engine.ContainerEvent, error) {
	var grouped map[string][]rawInspectContainer
	if err := json.Unmarshal(data, &grouped); err != nil {
		return nil, err
	}
	var out []engine.ContainerEvent
	for labName, containers := range grouped {
		for _, c := range containers {
			shortName := nodeShortName(c.Name, labName)
			ev := engine.ContainerEvent{
				LabName:     labName,
				NodeName:    shortName,
				Container:   c.Name,
				ContainerID: c.ContainerID,
				Kind:        c.Kind,
				Image:       c.Image,
				Group:       c.Group,
				State:       engine.NormalizeStatus(c.State),
				Status:      c.Status,
				IPv4:        c.IPv4,
				IPv6:        c.IPv6,
				Owner:       c.Owner,
				TopoFile:    c.AbsLabPath,
				Origin:      "inspect",
			}
			out = append(out, ev)
		}
	}
	return out, nil
}

// nodeShortName strips the `clab-<lab>-` prefix from a container long name.
func nodeShortName(longName, labName string) string {
	prefix := "clab-" + labName + "-"
	if strings.HasPrefix(longName, prefix) {
		return strings.TrimPrefix(longName, prefix)
	}
	return longName
}

// inspectInterfaceEntry groups interfaces by container long name.
type inspectInterfaceEntry struct {
	Container  string
	Interfaces []engine.Interface
}

// parseInspectInterfaces parses `containerlab inspect interfaces -f json`.
func parseInspectInterfaces(data []byte) ([]inspectInterfaceEntry, error) {
	var raw []rawInspectInterfaces
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var out []inspectInterfaceEntry
	for _, c := range raw {
		entry := inspectInterfaceEntry{Container: c.Name}
		for _, iface := range c.Interfaces {
			entry.Interfaces = append(entry.Interfaces, engine.Interface{
				Name: iface.Name, Alias: iface.Alias, Type: iface.Type,
				MAC: iface.MAC, MTU: iface.MTU, Index: iface.Ifindex,
				State: iface.State,
			})
		}
		out = append(out, entry)
	}
	return out, nil
}
