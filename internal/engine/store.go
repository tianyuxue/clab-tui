package engine

import (
	"sort"
	"sync"
)

// Store maintains a normalized snapshot of labs/nodes/interfaces/links and
// broadcasts Change events to subscribers. It is safe for concurrent use.
// Implementations (EventSource) drive it via the Apply* methods.
type Store struct {
	mu      sync.RWMutex
	labs    map[string]*Lab
	subs    map[int]chan Change
	nextSub int
}

func NewStore() *Store {
	return &Store{
		labs: make(map[string]*Lab),
		subs: make(map[int]chan Change),
	}
}

// Subscribe returns a change channel and a cancel function. The channel is
// buffered (256); slow consumers silently drop events rather than block the
// store. The returned cancel must be called to release the subscription.
func (s *Store) Subscribe() (<-chan Change, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextSub
	s.nextSub++
	ch := make(chan Change, 256)
	s.subs[id] = ch
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, id)
		s.mu.Unlock()
	}
}

// Snapshot returns a deep copy of the current state, nodes and interfaces
// sorted by name for determinism.
func (s *Store) Snapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := &Snapshot{Labs: make(map[string]*Lab, len(s.labs))}
	for name, lab := range s.labs {
		cloned := cloneLab(lab)
		sort.Slice(cloned.Nodes, func(i, j int) bool { return cloned.Nodes[i].Name < cloned.Nodes[j].Name })
		for i := range cloned.Nodes {
			sort.Slice(cloned.Nodes[i].Interfaces, func(a, b int) bool {
				return cloned.Nodes[i].Interfaces[a].Name < cloned.Nodes[i].Interfaces[b].Name
			})
		}
		out.Labs[name] = cloned
	}
	return out
}

func (s *Store) broadcast(c Change) {
	for _, ch := range s.subs {
		select {
		case ch <- c:
		default:
			// slow consumer: drop
		}
	}
}

func (s *Store) node(labName, nodeName string) *Node {
	lab := s.labs[labName]
	if lab == nil {
		return nil
	}
	for i := range lab.Nodes {
		if lab.Nodes[i].Name == nodeName {
			return &lab.Nodes[i]
		}
	}
	return nil
}

// ApplyContainerEvent upserts a node. Idempotent: no Change is emitted when
// the resulting node state is unchanged.
func (s *Store) ApplyContainerEvent(ev ContainerEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	lab := s.labs[ev.LabName]
	if lab == nil {
		lab = &Lab{Name: ev.LabName, TopoFile: ev.TopoFile, Owner: ev.Owner}
		s.labs[ev.LabName] = lab
		s.broadcast(Change{Type: ChangeLabAdded, LabName: ev.LabName})
	} else {
		if ev.TopoFile != "" && lab.TopoFile == "" {
			lab.TopoFile = ev.TopoFile
		}
		if ev.Owner != "" && lab.Owner == "" {
			lab.Owner = ev.Owner
		}
	}

	node := s.node(ev.LabName, ev.NodeName)
	next := Node{
		Name: ev.NodeName, Container: ev.Container, Kind: ev.Kind,
		Image: ev.Image, Group: ev.Group, Type: ev.Type, State: ev.State,
		Status: ev.Status, IPv4: ev.IPv4, IPv6: ev.IPv6,
		StartedAt: ev.StartedAt, Owner: ev.Owner,
	}
	if node != nil {
		// Preserve interfaces on node update.
		next.Interfaces = node.Interfaces
		next.SubContainers = node.SubContainers
		next.Extra = node.Extra
	}

	if node != nil && nodeEqual(*node, next) {
		return
	}
	if node == nil {
		lab.Nodes = append(lab.Nodes, next)
		s.broadcast(Change{Type: ChangeNodeAdded, LabName: ev.LabName, NodeName: ev.NodeName})
		return
	}
	*node = next
	s.broadcast(Change{Type: ChangeNodeUpdated, LabName: ev.LabName, NodeName: ev.NodeName})
}

// ApplyInterfaceEvent upserts an interface on an existing node. Unknown nodes
// are ignored (implementation must resolve node names before calling).
func (s *Store) ApplyInterfaceEvent(ev InterfaceEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	node := s.node(ev.LabName, ev.NodeName)
	if node == nil {
		return
	}
	// A stopped container has no network namespace; ignore late netlink
	// interface events so they can't resurrect stale interfaces.
	if node.State == StatusStopped {
		return
	}
	idx := -1
	for i := range node.Interfaces {
		if node.Interfaces[i].Name == ev.Name {
			idx = i
			break
		}
	}
	// veths are created with a transient host-side name (e.g. clab-*) and then
	// renamed into the container keeping the same ifindex (clab-* -> e1-1, or
	// docker eth0 -> mgmt0). Treat a same-ifindex name change as a rename so
	// the transient name doesn't linger as a bogus interface.
	if idx < 0 && ev.Index > 0 {
		for i := range node.Interfaces {
			if node.Interfaces[i].Index == ev.Index {
				idx = i
				break
			}
		}
	}
	next := Interface{
		Name: ev.Name, Alias: ev.Alias, Type: ev.Type, MAC: ev.MAC,
		MTU: ev.MTU, Index: ev.Index, State: ev.State,
		Netem: ev.Netem, Stats: ev.Stats,
	}
	if idx >= 0 && ifaceEqual(node.Interfaces[idx], next) {
		return
	}
	if idx < 0 {
		node.Interfaces = append(node.Interfaces, next)
		s.broadcast(Change{Type: ChangeInterfaceAdded, LabName: ev.LabName, NodeName: ev.NodeName, InterfaceName: ev.Name})
		s.recomputeAllLinksLocked(ev.LabName)
		return
	}
	node.Interfaces[idx] = next
	s.broadcast(Change{Type: ChangeInterfaceUpdated, LabName: ev.LabName, NodeName: ev.NodeName, InterfaceName: ev.Name})
	s.recomputeAllLinksLocked(ev.LabName)
}

// SetLinks stores the static link structure for a lab (from the topology
// file). Link state is derived from both endpoint interfaces.
func (s *Store) SetLinks(labName string, links []Link) {
	s.mu.Lock()
	defer s.mu.Unlock()

	lab := s.labs[labName]
	if lab == nil {
		lab = &Lab{Name: labName}
		s.labs[labName] = lab
	}
	copied := make([]Link, len(links))
	copy(copied, links)
	lab.Links = copied
	s.recomputeAllLinksLocked(labName)
}

func (s *Store) recomputeAllLinksLocked(labName string) {
	lab := s.labs[labName]
	if lab == nil {
		return
	}
	for i := range lab.Links {
		lab.Links[i].State = s.deriveLinkState(lab, lab.Links[i])
	}
}

func (s *Store) deriveLinkState(lab *Lab, l Link) LinkState {
	left := s.ifaceState(lab, l.A, l.PortA)
	right := s.ifaceState(lab, l.B, l.PortB)
	switch {
	case left == "up" && right == "up":
		return LinkUp
	case left == "" || right == "":
		return LinkUnknown
	case left == "unknown" || right == "unknown":
		return LinkUnknown
	default:
		return LinkDown
	}
}

func (s *Store) ifaceState(lab *Lab, nodeName, port string) string {
	for i := range lab.Nodes {
		if lab.Nodes[i].Name != nodeName {
			continue
		}
		for _, iface := range lab.Nodes[i].Interfaces {
			if iface.Name == port {
				return iface.State
			}
		}
	}
	return ""
}

// ClearNodeInterfaces removes all interfaces from a node (e.g. when the
// container stops and its network namespace is gone). Emits
// ChangeInterfaceRemoved.
func (s *Store) ClearNodeInterfaces(labName, nodeName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	node := s.node(labName, nodeName)
	if node == nil {
		return
	}
	if len(node.Interfaces) == 0 {
		return
	}
	node.Interfaces = nil
	s.recomputeAllLinksLocked(labName)
	s.broadcast(Change{Type: ChangeInterfaceRemoved, LabName: labName, NodeName: nodeName})
}

// RemoveInterface removes a single interface from a node (e.g. on a netlink
// interface delete event). No-op when the interface does not exist. Emits
// ChangeInterfaceRemoved.
func (s *Store) RemoveInterface(labName, nodeName, ifaceName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	node := s.node(labName, nodeName)
	if node == nil {
		return
	}
	for i := range node.Interfaces {
		if node.Interfaces[i].Name == ifaceName {
			node.Interfaces = append(node.Interfaces[:i], node.Interfaces[i+1:]...)
			s.recomputeAllLinksLocked(labName)
			s.broadcast(Change{Type: ChangeInterfaceRemoved, LabName: labName, NodeName: nodeName, InterfaceName: ifaceName})
			return
		}
	}
}

// RemoveLab removes an entire lab (lab:deleted / destroy). Emits
// ChangeLabRemoved.
func (s *Store) RemoveLab(labName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.labs[labName]; !ok {
		return
	}
	delete(s.labs, labName)
	s.broadcast(Change{Type: ChangeLabRemoved, LabName: labName})
}

// RemoveNode removes a node and all its interfaces from a lab. Emits
// ChangeNodeRemoved.
func (s *Store) RemoveNode(labName, nodeName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lab := s.labs[labName]
	if lab == nil {
		return
	}
	for i := range lab.Nodes {
		if lab.Nodes[i].Name == nodeName {
			lab.Nodes = append(lab.Nodes[:i], lab.Nodes[i+1:]...)
			s.broadcast(Change{Type: ChangeNodeRemoved, LabName: labName, NodeName: nodeName})
			s.recomputeAllLinksLocked(labName)
			return
		}
	}
}

// Reset clears all state (engine switch / event stream restart).
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.labs = make(map[string]*Lab)
}

func cloneLab(l *Lab) *Lab {
	out := &Lab{Name: l.Name, TopoFile: l.TopoFile, Owner: l.Owner}
	out.Nodes = make([]Node, len(l.Nodes))
	for i := range l.Nodes {
		out.Nodes[i] = cloneNode(l.Nodes[i])
	}
	out.Links = make([]Link, len(l.Links))
	copy(out.Links, l.Links)
	return out
}

func cloneNode(n Node) Node {
	out := n
	out.Extra = cloneStringMap(n.Extra)
	out.Interfaces = make([]Interface, len(n.Interfaces))
	for i := range n.Interfaces {
		out.Interfaces[i] = cloneInterface(n.Interfaces[i])
	}
	out.SubContainers = make([]Node, len(n.SubContainers))
	for i := range n.SubContainers {
		out.SubContainers[i] = cloneNode(n.SubContainers[i])
	}
	return out
}

func cloneInterface(f Interface) Interface {
	out := f
	out.Extra = cloneStringMap(f.Extra)
	if f.Netem != nil {
		nm := *f.Netem
		out.Netem = &nm
	}
	if f.Stats != nil {
		st := *f.Stats
		out.Stats = &st
	}
	return out
}

func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func nodeEqual(a, b Node) bool {
	return a.Name == b.Name && a.Container == b.Container && a.Kind == b.Kind &&
		a.Image == b.Image && a.Group == b.Group && a.Type == b.Type &&
		a.State == b.State && a.Status == b.Status && a.IPv4 == b.IPv4 &&
		a.IPv6 == b.IPv6 && a.Owner == b.Owner &&
		a.StartedAt.Equal(b.StartedAt)
}

func ifaceEqual(a, b Interface) bool {
	return a.Name == b.Name && a.Alias == b.Alias && a.Type == b.Type &&
		a.MAC == b.MAC && a.MTU == b.MTU && a.Index == b.Index &&
		a.State == b.State && netemEqual(a.Netem, b.Netem) && statsEqual(a.Stats, b.Stats)
}

// netemEqual compares NetemState values; nil equals nil.
func netemEqual(a, b *NetemState) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// statsEqual compares InterfaceStats values; nil equals nil.
func statsEqual(a, b *InterfaceStats) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
