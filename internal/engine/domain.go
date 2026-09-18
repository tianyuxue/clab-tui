package engine

import "time"

// Status is a normalized node lifecycle state. Implementations map their own
// raw states onto these values (e.g. exited/dead/killed -> StatusStopped).
type Status string

const (
	StatusUnknown Status = "unknown"
	StatusCreated Status = "created"
	StatusRunning Status = "running"
	StatusPaused  Status = "paused"
	StatusStopped Status = "stopped"
)

// NormalizeStatus maps a raw state string onto a Status. Unknown values
// collapse to StatusUnknown; termination states collapse to StatusStopped.
func NormalizeStatus(raw string) Status {
	switch raw {
	case "created", "restarting":
		return StatusCreated
	case "running":
		return StatusRunning
	case "paused":
		return StatusPaused
	case "exited", "dead", "killed", "removing", "stopped":
		return StatusStopped
	default:
		return StatusUnknown
	}
}

type Node struct {
	Name          string // logical node name, e.g. "srl1"
	Container     string // engine-internal id, e.g. "clab-lab-srl1"
	Kind          string
	Image         string
	Group         string // role: spine/leaf/...
	Type          string // nodeType
	State         Status
	Status        string // raw status text, e.g. "Up 4 hours"
	IPv4          string
	IPv6          string
	StartedAt     time.Time
	Owner         string
	SubContainers []Node // multi-container nodes (e.g. SRSIM)
	Interfaces    []Interface
	Extra         map[string]string // unmodeled raw fields
}

type Interface struct {
	Name  string
	Alias string
	Type  string
	MAC   string
	MTU   int
	Index int
	State string // up / down / unknown
	Netem *NetemState
	Stats *InterfaceStats
	Extra map[string]string
}

type NetemState struct {
	Delay      string
	Jitter     string
	Loss       string
	Rate       string
	Corruption string
}

type InterfaceStats struct {
	RxBps     float64
	TxBps     float64
	RxBytes   uint64
	RxPackets uint64
	TxBytes   uint64
	TxPackets uint64
	Interval  time.Duration
	StatsAt   time.Time
}

type LinkState string

const (
	LinkUp      LinkState = "up"
	LinkDown    LinkState = "down"
	LinkUnknown LinkState = "unknown"
)

type Link struct {
	A     string
	PortA string
	B     string
	PortB string
	State LinkState
}

type Lab struct {
	Name     string
	TopoFile string
	Owner    string
	Nodes    []Node
	Links    []Link
}

type LabStatus int

const (
	LabStatusUnknown LabStatus = iota
	LabStatusRunning
	LabStatusStopped
	LabStatusPartial
	LabStatusDeploying
)

// Status derives the lab-level state from its nodes.
func (l *Lab) Status() LabStatus {
	if len(l.Nodes) == 0 {
		return LabStatusUnknown
	}
	anyRunning := false
	anyNotRunning := false
	for _, n := range l.Nodes {
		if n.State == StatusRunning {
			anyRunning = true
		} else {
			anyNotRunning = true
		}
	}
	if anyRunning && anyNotRunning {
		return LabStatusPartial
	}
	if anyRunning {
		return LabStatusRunning
	}
	return LabStatusStopped
}

type Snapshot struct {
	Labs map[string]*Lab
}

// Change types published to subscribers.
type ChangeType int

const (
	ChangeLabAdded ChangeType = iota
	ChangeLabRemoved
	ChangeNodeAdded
	ChangeNodeUpdated
	ChangeNodeRemoved
	ChangeInterfaceAdded
	ChangeInterfaceUpdated
	ChangeInterfaceRemoved
	ChangeLinkAdded
	ChangeLinkUpdated
	ChangeLinkRemoved
)

type Change struct {
	Type          ChangeType
	LabName       string
	NodeName      string
	InterfaceName string
	Link          *Link // set only for ChangeLink* change types
}

// ContainerEvent carries a normalized container lifecycle update from an
// implementation into the Store.
type ContainerEvent struct {
	LabName     string
	NodeName    string // logical name (attributes.clab-node-name)
	Container   string // engine-internal id (long name)
	ContainerID string // short id (actor_id)
	Kind        string
	Image       string
	Group       string
	Type        string
	State       Status
	Status      string
	IPv4        string
	IPv6        string
	StartedAt   time.Time
	Owner       string
	TopoFile    string
	Origin      string // "snapshot" | "live" | ""
	Action      string // raw action from the wire: "running", "stop", "die", "destroy", ...
}

// InterfaceEvent carries a normalized interface update.
type InterfaceEvent struct {
	LabName     string
	NodeName    string // logical name (resolved by implementation)
	ContainerID string // actor_id (short id)
	Name        string // ifname
	Alias       string
	Type        string
	MAC         string
	MTU         int
	Index       int
	State       string // up / down / unknown
	Action      string // "" | "delete" | "snapshot" | "create" | "update"
	Netem       *NetemState
	Stats       *InterfaceStats
	Origin      string // "netlink" | "snapshot" | ""
}

// LinkEvent carries a normalized link state update.
type LinkEvent struct {
	LabName string
	A       string
	PortA   string
	B       string
	PortB   string
	State   LinkState
}
