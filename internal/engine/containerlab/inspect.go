package containerlab

import (
	"github.com/tianyuxue/clab-tui/internal/engine"
)

// applyInspectData feeds `containerlab inspect -f json` output into the store.
// It seeds the actor→node mapping (with long names) so subsequent interface
// data can resolve containers to logical nodes.
func applyInspectData(store *engine.Store, reg *actorRegistry, data []byte) error {
	events, err := parseInspectContainers(data)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if reg != nil {
			reg.Set(ev.ContainerID, ev.LabName, ev.NodeName, ev.Container)
		}
		store.ApplyContainerEvent(ev)
	}
	return nil
}

// applyInspectInterfaces feeds `containerlab inspect interfaces -f json`
// output into the store, resolving container long names to logical nodes via
// the actor registry. Interfaces for unknown containers are skipped.
func applyInspectInterfaces(store *engine.Store, reg *actorRegistry, data []byte) error {
	entries, err := parseInspectInterfaces(data)
	if err != nil {
		return err
	}
	for _, e := range entries {
		labName, nodeName := "", ""
		if reg != nil {
			labName, nodeName = reg.LookupByLongName(e.Container)
		}
		if nodeName == "" {
			continue
		}
		for _, iface := range e.Interfaces {
			store.ApplyInterfaceEvent(engine.InterfaceEvent{
				LabName: labName, NodeName: nodeName,
				Name: iface.Name, Alias: iface.Alias, Type: iface.Type,
				MAC: iface.MAC, MTU: iface.MTU, Index: iface.Index,
				State: iface.State, Origin: "inspect",
			})
		}
	}
	return nil
}
