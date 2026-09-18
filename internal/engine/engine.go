package engine

import (
	"context"
	"fmt"
	"strings"
)

// OutputLine is a single line of command output. Stream is "stdout", "stderr"
// or "session" (a live interactive PTY session). A line with Done=true is the
// terminal sentinel marking the end of the stream; it may carry an error
// message in Line. Consumers must not render Done lines as content. A stream
// that closes without Done is incomplete and must not be treated as success.
// Partial=true marks a session line that has no trailing newline yet (the
// live tail of the PTY); consumers should replace the previous partial line
// rather than append.
type OutputLine struct {
	Line    string
	Stream  string // "stdout" or "stderr"
	Partial bool   // live session tail with no trailing newline
	Done    bool
	Code    int // process exit code; set on the Done sentinel line
}

type ExecResult struct {
	Stdout string
	Stderr string
	Code   int
}

type DeployOptions struct {
	Cleanup bool
	Timeout int // seconds
}
type DestroyOptions struct {
	Cleanup bool
}
type RedeployOptions struct {
	Cleanup bool
}

type DeployOption func(*DeployOptions)
type DestroyOption func(*DestroyOptions)
type RedeployOption func(*RedeployOptions)

func WithDeployCleanup() DeployOption {
	return func(c *DeployOptions) { c.Cleanup = true }
}
func WithDestroyCleanup() DestroyOption {
	return func(c *DestroyOptions) { c.Cleanup = true }
}
func WithRedeployCleanup() RedeployOption {
	return func(c *RedeployOptions) { c.Cleanup = true }
}
func WithTimeout(seconds int) DeployOption {
	return func(c *DeployOptions) { c.Timeout = seconds }
}

// Engine is the engine abstraction consumed by upper layers. All operations
// use logical (Lab, Node) identifiers; implementations map them onto their own
// substrate (containerlab CLI, a custom forwarding engine, etc.).
type Engine interface {
	// State queries
	Snapshot() *Snapshot
	Subscribe() (<-chan Change, func())

	// Topology / detail (structure from topology file + live state)
	GetLab(ctx context.Context, labName string) (*Lab, error)
	GetNode(ctx context.Context, labName, nodeName string) (*Node, error)
	ListLabs(ctx context.Context) ([]*Lab, error)

	// Lifecycle
	Deploy(ctx context.Context, labPath string, opts ...DeployOption) (<-chan OutputLine, error)
	Destroy(ctx context.Context, labName string, opts ...DestroyOption) (<-chan OutputLine, error)
	Redeploy(ctx context.Context, labName string, opts ...RedeployOption) (<-chan OutputLine, error)
	Save(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error)

	// Node control
	StartNode(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error)
	StopNode(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error)
	RestartNode(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error)
	PauseNode(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error)
	UnpauseNode(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error)
	Exec(ctx context.Context, labName, nodeName, cmd string) (*ExecResult, error)

	// Close releases the event stream, subprocesses and goroutines.
	Close() error
}

// ---- Optional capability interfaces (type-asserted by upper layers) ----

type SessionMode int

const (
	SessionModeSSH SessionMode = iota
	SessionModeShell
	SessionModeTelnet
)

// SessionCmd describes an interactive command for embedding in the TUI
// (lazygit-style). The command runs in the foreground until the user exits.
type SessionCmd struct {
	Command []string
	Title   string
}

// LogStreamer streams a node's logs.
type LogStreamer interface {
	StreamNodeLogs(ctx context.Context, labName, nodeName string) (<-chan OutputLine, error)
}

// TerminalSessioner provides long-lived interactive sessions into a node.
type TerminalSessioner interface {
	SessionCommand(ctx context.Context, labName, nodeName string, mode SessionMode) (*SessionCmd, error)
}

type CaptureOptions struct {
	File   string
	Raw    bool
	Filter string // BPF 过滤表达式（如 "tcp port 179"）
	Count  int    // 抓包上限（0=无限）
}
type CaptureOption func(*CaptureOptions)

func WithCaptureFile(path string) CaptureOption {
	return func(c *CaptureOptions) { c.File = path }
}
func WithCaptureFilter(filter string) CaptureOption {
	return func(c *CaptureOptions) { c.Filter = filter }
}
func WithCaptureCount(n int) CaptureOption {
	return func(c *CaptureOptions) { c.Count = n }
}

// CaptureIssue describes one direction-specific attach failure. Capture may
// still proceed when another direction or interface attached successfully.
type CaptureIssue struct {
	Node      string
	Interface string
	Direction string // "ingress" or "egress"
	Err       error
}

func (i CaptureIssue) Error() string {
	return fmt.Sprintf("%s:%s %s attach failed: %v", i.Node, i.Interface, i.Direction, i.Err)
}

// CaptureWarningError reports attach failures from a partially started
// capture. The event channel and stop function remain usable in this case.
type CaptureWarningError struct {
	Issues []CaptureIssue
}

func (e *CaptureWarningError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "capture started with warnings"
	}
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		parts = append(parts, issue.Error())
	}
	return "capture started with warnings: " + strings.Join(parts, "; ")
}

// Unwrap exposes every underlying attach error to errors.As/errors.Is.
func (e *CaptureWarningError) Unwrap() []error {
	if e == nil {
		return nil
	}
	errs := make([]error, 0, len(e.Issues))
	for _, issue := range e.Issues {
		if issue.Err != nil {
			errs = append(errs, issue.Err)
		}
	}
	return errs
}

// ParsedPacket 是抓到的单个包元数据（eBPF 提取）。
type ParsedPacket struct {
	Time      string // 捕获时间，tcpdump 风格 "15:04:05.000000"
	Direction string // "IN"=ingress / "OUT"=egress / ""=unknown.
	SrcIP     string
	DstIP     string
	SrcPort   int
	DstPort   int
	Proto     string
	Len       int
	Summary   string
	Payload   []byte // 原始包体（gopacket 在 UI 层解析，见 tabs.parsePacketDetail）
}

// PacketEvent 是一次包命中（含来源节点和接口）。
type PacketEvent struct {
	Node  string
	Iface string
	Pkt   ParsedPacket
}

// PacketCapturer 对单个接口抓包，返回包事件流。opts 可含过滤条件。
// 返回事件流、停止函数、错误。
type PacketCapturer interface {
	Capture(ctx context.Context, labName, nodeName, iface string, opts ...CaptureOption) (<-chan PacketEvent, func() error, error)
}

// PacketPathTracer 对全拓扑抓包，报告包事件流。filter 为空则抓全部。
type PacketPathTracer interface {
	TracePath(ctx context.Context, labName, filter string) (<-chan PacketEvent, func() error, error)
}

// CapabilityResult 报告一个能力是否可用及原因。
type CapabilityResult struct {
	Available bool
	Reason    string
}

// CapabilityChecker 检测高级能力（eBPF/root）的可用性。
type CapabilityChecker interface {
	// PacketTraceCapable 报告链路追踪/抓包是否可用（root + BTF + Linux）。
	PacketTraceCapable(ctx context.Context) CapabilityResult
}

// NodeMonitor carries a node's resource usage snapshot (from docker stats).
type NodeMonitor struct {
	CPUPercent float64
	MemUsed    uint64 // bytes
	MemLimit   uint64 // bytes
	NetRx      uint64 // cumulative received bytes
	NetTx      uint64 // cumulative transmitted bytes
	Pids       uint64
}

// NodeMonitorProvider reports resource usage for a node.
type NodeMonitorProvider interface {
	MonitorNode(ctx context.Context, labName, nodeName string) (*NodeMonitor, error)
}

// InterfaceStatsProvider reports cumulative and interval interface counters
// for one selected node. Implementations may fall back to procfs when the
// image does not provide a JSON-capable ip command.
type InterfaceStatsProvider interface {
	MonitorInterfaces(ctx context.Context, labName, nodeName string) (map[string]*InterfaceStats, error)
}

// NetemProvider applies or clears Linux netem settings on one interface.
type NetemProvider interface {
	SetInterfaceNetem(ctx context.Context, labName, nodeName, iface string, state NetemState) (<-chan OutputLine, error)
	ClearInterfaceNetem(ctx context.Context, labName, nodeName, iface string) (<-chan OutputLine, error)
}

// InterfaceIPProvider reports per-interface IPv4 addresses of a node. The map
// keys are interface names (e.g. "e1-1", "mgmt0"). Interfaces with no IPv4 or
// kinds whose CLI is unsupported are simply absent.
type InterfaceIPProvider interface {
	InterfaceIPs(ctx context.Context, labName, nodeName string) (map[string]string, error)
}

// SessionManager lives in session.go. It is an optional capability: type-assert
// an Engine against it to manage live interactive sessions.

var (
	ErrLabNotFound     = &EngineError{Code: "lab_not_found"}
	ErrNodeNotFound    = &EngineError{Code: "node_not_found"}
	ErrNotSupported    = &EngineError{Code: "not_supported"}
	ErrOperationFailed = &EngineError{Code: "operation_failed"}
)

type EngineError struct {
	Code    string
	Message string
	Err     error
}

func (e *EngineError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e *EngineError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *EngineError) Is(target error) bool {
	if e == nil {
		return false
	}
	other, ok := target.(*EngineError)
	return ok && other.Code == e.Code
}
