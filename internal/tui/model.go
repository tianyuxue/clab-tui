package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tianyuxue/clab-tui/internal/capability"
	"github.com/tianyuxue/clab-tui/internal/engine"
	"github.com/tianyuxue/clab-tui/internal/tui/components"
	"github.com/tianyuxue/clab-tui/internal/tui/popup"
	"github.com/tianyuxue/clab-tui/internal/tui/tabs"
)

type tab int

const (
	tabTopology tab = iota
	tabSessions
	tabLogs
	tabOps
)

// presetTraceFilters are the quick BPF filters offered by the trace/capture
// filter selector. The "custom-filter" item routes to the search input.
var presetTraceFilters = []tabs.ActionItem{
	{Label: "BGP (tcp.179)", ID: "filter:tcp port 179"},
	{Label: "OSPF", ID: "filter:ip proto 89"},
	{Label: "ICMP", ID: "filter:icmp"},
	{Label: "ARP", ID: "filter:arp"},
	{Label: "All", ID: "filter:"},
	{Label: "Custom...", ID: "custom-filter"},
}

// Option configures a Model at construction time.
type Option func(*Model)

type Model struct {
	eng                engine.Engine
	caps               capability.Capabilities
	activeTab          tab
	curTopoLab         string
	initialLab         *engine.Lab
	width              int
	height             int
	ready              bool
	err                error
	errSeq             int
	labs               []*engine.Lab
	startupPickerShown bool
	devTree            *tabs.DevTree
	detailPane         *tabs.DetailPane
	labPicker          *tabs.LabPicker
	actionMenu         *tabs.ActionMenu
	searchInput        *tabs.SearchInput
	logView            *tabs.LogModel
	opsView            *tabs.OpsModel
	currentOpCh        <-chan engine.OutputLine
	currentLogCh       <-chan engine.OutputLine
	logCancel          context.CancelFunc
	confirm            *tabs.Confirm
	netemForm          *tabs.NetemForm
	netemNode          string
	netemIface         string
	sessionModel       *tabs.SessionModel
	sessionPicker      *tabs.SessionPicker
	whichKey           *tabs.WhichKey
	changeCh           <-chan engine.Change
	changeCancel       func()
	help               help.Model
	statusBar          *components.StatusBar
	toast              *components.Toast
	capturePane        *tabs.CapturePane
	captureCancel      context.CancelFunc
	captureStop        func() error
	captureEventCh     <-chan engine.PacketEvent
	captureNode        string
	captureIface       string
	captureFilter      string
	pendingCaptureFile bool
	traceCancel        context.CancelFunc
	traceStop          func() error
	traceEventCh       <-chan engine.PacketEvent
	traceFilter        string
	traceFilterSet     bool
	traceMode          bool
	traceSeq           int
	traceBlinkOn       bool
	tracing            bool
	pendingFilter      bool
	graph              graphLauncher
}

func New(eng engine.Engine, labs []*engine.Lab, opts ...Option) *Model {
	m := &Model{
		eng:           eng,
		activeTab:     tabTopology,
		help:          help.New(),
		statusBar:     components.NewStatusBar(),
		toast:         components.NewToast(),
		devTree:       tabs.NewDevTree(),
		detailPane:    tabs.NewDetailPane(),
		labPicker:     tabs.NewLabPicker(),
		actionMenu:    tabs.NewActionMenu(),
		searchInput:   tabs.NewSearchInput(),
		logView:       tabs.NewLogs(),
		opsView:       tabs.NewOps(),
		confirm:       tabs.NewConfirm(),
		netemForm:     tabs.NewNetemForm(),
		sessionModel:  tabs.NewSessionModel(),
		sessionPicker: tabs.NewSessionPicker(),
		whichKey:      tabs.NewWhichKey(),
		capturePane:   tabs.NewCapturePane(),
	}
	if labs != nil {
		m.labs = labs
		m.statusBar.LabCount = len(labs)
		running := 0
		for _, l := range labs {
			if l.Status() == engine.LabStatusRunning {
				running++
			}
		}
		m.statusBar.RunningCount = running
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// WithInitialLab sets the lab to load directly at startup, skipping the lab
// picker. Used when the CLI opens a single topology file.
func WithInitialLab(lab *engine.Lab) Option {
	return func(m *Model) {
		m.initialLab = lab
	}
}

// WithCapabilities supplies the detected process privileges for proactive
// checks and degraded-mode notices.
func WithCapabilities(caps capability.Capabilities) Option {
	return func(m *Model) { m.caps = caps }
}

// requireCap shows an English toast and returns its command when req is
// unmet, or nil when the requirement is satisfied.
func (m *Model) requireCap(req capability.Requirement) tea.Cmd {
	if ok, reason := m.caps.Check(req); !ok {
		return m.toast.Show(reason)
	}
	return nil
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		subscribeEvents(m.eng),
		loadLabs(m.eng),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.dispatchSize(msg.Width, msg.Height-2)
		return m, nil

	case tea.KeyMsg:
		// Which-key is modal while open: all keys go to it exclusively.
		if m.whichKey.Visible() {
			var wkCmd tea.Cmd
			m.whichKey, wkCmd = m.whichKey.Update(msg)
			return m, wkCmd
		}
		// If the lab picker or action menu is open, keys go to it exclusively.
		if m.labPicker.Visible() {
			var lpCmd tea.Cmd
			m.labPicker, lpCmd = m.labPicker.Update(msg)
			return m, lpCmd
		}
		if m.actionMenu.Visible() {
			var amCmd tea.Cmd
			m.actionMenu, amCmd = m.actionMenu.Update(msg)
			return m, amCmd
		}
		if m.searchInput.Visible() {
			if msg.String() == "esc" {
				m.pendingFilter = false
				m.pendingCaptureFile = false
			}
			var siCmd tea.Cmd
			m.searchInput, siCmd = m.searchInput.Update(msg)
			return m, siCmd
		}
		if m.confirm.Visible() {
			var cCmd tea.Cmd
			m.confirm, cCmd = m.confirm.Update(msg)
			return m, cCmd
		}
		if m.netemForm.Visible() {
			var nfCmd tea.Cmd
			m.netemForm, nfCmd = m.netemForm.Update(msg)
			return m, nfCmd
		}
		if m.sessionPicker.Visible() {
			var spCmd tea.Cmd
			m.sessionPicker, spCmd = m.sessionPicker.Update(msg)
			return m, spCmd
		}
		// Capture pane is modal while visible: q closes/stops, c clears.
		if m.capturePane.Visible() {
			switch msg.String() {
			case "q":
				m.stopCapture()
				return m, nil
			case "c":
				m.capturePane.Clear()
				return m, nil
			case "p":
				m.capturePane.TogglePaused()
				return m, nil
			case "w":
				m.pendingCaptureFile = true
				m.searchInput.ShowCaptureFile()
				return m, nil
			}
		}
		// Sessions tab: route keys by vim-style mode. In insert mode every key
		// (incl ctrl+c, tab, arrows) is shell input; in normal mode keys are TUI
		// shortcuts.
		if m.activeTab == tabSessions && m.sessionModel.HasSessions() {
			if m.sessionModel.IsInsert() {
				// Mode switch: single ctrl+\ enters normal mode.
				if msg.String() == "ctrl+\\" {
					m.sessionModel.SetMode(tabs.SessionNormal)
					return m, nil
				}
				// Insert mode: every key (incl ctrl+c, tab, arrows, 's') is
				// shell input via the session model's KeyEncode.
				var smCmd tea.Cmd
				m.sessionModel, smCmd = m.sessionModel.Update(msg)
				return m, smCmd
			}
			// Normal mode: tab/shift+tab switch tabs; 's' opens the session
			// picker; everything else goes to the session model.
			if msg.String() == "tab" {
				m.nextTab()
				return m, m.maybeLoadTopology()
			}
			if msg.String() == "shift+tab" {
				m.prevTab()
				return m, m.maybeLoadTopology()
			}
			if msg.String() == " " {
				m.whichKey.Show(m.buildWhichKeyRoot())
				return m, nil
			}
			if msg.String() == "s" {
				m.sessionPicker.Show(m.sessionModel.SessionsHandles())
				return m, nil
			}
			// Normal mode: digits switch tabs (like the global digit shortcut).
			if len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '4' {
				m.activeTab = tab(int(msg.Runes[0] - '1'))
				return m, m.maybeLoadTopology()
			}
			var smCmd tea.Cmd
			m.sessionModel, smCmd = m.sessionModel.Update(msg)
			return m, smCmd
		}
		// Global keys handled first.
		switch {
		case key.Matches(msg, Keys.Quit) && !(m.activeTab == tabSessions && m.sessionModel.HasSessions()):
			if m.graph != nil {
				m.graph.stop()
			}
			return m, tea.Quit
		case msg.String() == "/":
			if m.activeTab == tabTopology {
				m.searchInput.Show()
			}
			return m, nil
		case msg.String() == "l":
			if m.activeTab == tabTopology {
				m.showLabMenu()
			}
			return m, nil
		case len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '4':
			// Digit shortcuts jump straight to the numbered tab.
			m.activeTab = tab(int(msg.Runes[0] - '1'))
			return m, m.maybeLoadTopology()
		case key.Matches(msg, Keys.Tab):
			m.nextTab()
			return m, m.maybeLoadTopology()
		case key.Matches(msg, Keys.ShiftTab):
			m.prevTab()
			return m, m.maybeLoadTopology()
		case key.Matches(msg, Keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		case msg.String() == " " && !m.capturePane.Visible():
			m.whichKey.Show(m.buildWhichKeyRoot())
			return m, nil
		default:
			// Forward the key to the active tab so j/k/enter/etc. work.
			return m.forwardKey(msg)
		}

	case tabs.LabPickedMsg:
		// Lab picked from the overlay: switch to topology and load it.
		m.activeTab = tabTopology
		m.curTopoLab = msg.Lab.Name
		return m, loadTopology(m.eng, msg.Lab.Name)

	case tabs.ActionSelectedMsg:
		// An action was picked from the node or lab action menu.
		m.actionMenu.Hide()
		switch {
		case msg.Item.ID == "netem-set":
			selection, ok := m.devTree.SelectedResource()
			if !ok || selection.Kind != tabs.ResourceInterface {
				return m, m.toast.Show("no interface selected")
			}
			m.netemNode, m.netemIface = selection.Node, selection.Interface
			m.netemForm.Show()
			return m, nil
		case msg.Item.ID == "netem-clear":
			selection, ok := m.devTree.SelectedResource()
			if !ok || selection.Kind != tabs.ResourceInterface {
				return m, m.toast.Show("no interface selected")
			}
			return m, m.clearNetem(selection.Node, selection.Interface)
		case msg.Item.ID == "netem-capture":
			return m, nil
		case msg.Item.ID == "netem-save":
			return m, nil
		case msg.Item.ID == "interface-capture":
			selection, ok := m.devTree.SelectedResource()
			if !ok || selection.Kind != tabs.ResourceInterface {
				return m, m.toast.Show("no interface selected")
			}
			m.captureNode = selection.Node
			m.captureIface = selection.Interface
			m.actionMenu.Show("Capture filter", presetTraceFilters)
			return m, nil
		case strings.HasPrefix(msg.Item.ID, "iface:"):
			m.captureIface = strings.TrimPrefix(msg.Item.ID, "iface:")
			m.captureNode = m.detailNodeName()
			m.actionMenu.Show("Capture filter", presetTraceFilters)
			return m, nil
		case strings.HasPrefix(msg.Item.ID, "filter:"):
			filter := strings.TrimPrefix(msg.Item.ID, "filter:")
			if m.traceMode {
				m.traceMode = false
				return m, m.startTrace(filter)
			}
			return m, m.startCapture(m.curTopoLab, m.captureNode, m.captureIface, filter)
		case msg.Item.ID == "custom-filter":
			m.pendingFilter = true
			m.searchInput.ShowCustomFilter()
			return m, nil
		}
		switch msg.Item.ID {
		case "switchlab":
			m.labPicker.Show(m.labs)
			return m, nil
		case "deploy":
			return m, m.deployCurrentLab()
		case "destroy":
			return m, m.destroyCurrentLab()
		case "redeploy":
			return m, m.redeployCurrentLab()
		case "edit":
			return m, m.editCurrentLabYAML()
		case "graph":
			return m, m.openGraphView()
		case "trace":
			return m, m.toggleTrace()
		case "filter":
			return m, m.changeTraceFilter()
		case "search":
			if m.activeTab == tabTopology {
				m.searchInput.Show()
			}
			return m, nil
		case "toggle-conns":
			m.devTree.ToggleAll()
			return m, nil
		case "session-picker":
			m.sessionPicker.Show(m.sessionModel.SessionsHandles())
			return m, nil
		case "logs-follow":
			m.logView.ToggleFollow()
			return m, nil
		case "logs-clear":
			m.logView.Clear()
			return m, nil
		case "logs-top":
			m.logView.GotoTop()
			return m, nil
		case "logs-bottom":
			m.logView.GotoBottom()
			return m, nil
		case "ops-follow":
			m.opsView.ToggleAutoFollow()
			return m, nil
		case "ops-clear":
			m.opsView.Clear()
			return m, nil
		default:
			return m, m.runNodeAction(msg.Item.ID)
		}

	case tabs.ConfirmResultMsg:
		if msg.Confirmed {
			return m, m.executeNodeAction(msg.ActionID)
		}
		return m, nil

	case tabs.SearchCommitMsg:
		// A custom BPF filter was committed from the prompt.
		if m.pendingFilter {
			m.pendingFilter = false
			filter := m.searchInput.Value()
			if m.traceMode {
				m.traceMode = false
				return m, m.startTrace(filter)
			}
			return m, m.startCapture(m.curTopoLab, m.captureNode, m.captureIface, filter)
		}
		if m.pendingCaptureFile {
			m.pendingCaptureFile = false
			path := strings.TrimSpace(msg.Term)
			if path == "" {
				return m, m.toast.Show("pcap path cannot be empty")
			}
			m.stopCapture()
			return m, m.startCaptureFile(m.curTopoLab, m.captureNode, m.captureIface, m.captureFilter, path)
		}
		// Search committed from the prompt: locate matches in the device tree.
		m.devTree.Search(msg.Term)
		return m, m.refreshDetail(m.detailNodeName())

	case tabs.NetemCommitMsg:
		return m, m.setNetem(msg.State)

	case captureEventMsg:
		if m.capturePane.Visible() {
			m.capturePane.Append(msg.ev)
			return m, m.pullCaptureEvent(m.captureEventCh)
		}
		return m, nil

	case captureStoppedMsg:
		m.stopCapture()
		return m, nil

	case traceEventMsg:
		if msg.seq != m.traceSeq {
			return m, nil
		}
		if m.tracing {
			m.devTree.IncrementPathHit(msg.ev.Node, msg.ev.Iface, msg.ev.Pkt.Direction)
			return m, m.pullTraceEvent(m.traceEventCh, msg.seq)
		}
		return m, nil

	case traceStoppedMsg:
		if msg.seq != m.traceSeq {
			return m, nil
		}
		m.tracing = false
		m.traceBlinkOn = false
		m.devTree.SetPathBlink(false)
		m.devTree.ClearPathHits()
		if m.traceCancel != nil {
			m.traceCancel()
			m.traceCancel = nil
		}
		m.traceStop = nil
		return m, nil

	case traceBlinkMsg:
		if msg.seq != m.traceSeq || !m.tracing {
			return m, nil
		}
		m.traceBlinkOn = !m.traceBlinkOn
		m.devTree.SetPathBlink(m.traceBlinkOn)
		return m, m.scheduleTraceBlink(msg.seq)

	case components.ToastExpired:
		m.toast.HandleExpired(msg)
		return m, nil

	case changeStreamReadyMsg:
		m.changeCh = msg.ch
		m.changeCancel = msg.cancel
		if msg.err != nil {
			m.err = msg.err
		}
		return m, nextChange(msg.ch)

	case engine.Change:
		cmd := m.handleChange(msg)
		return m, tea.Batch(cmd, nextChange(m.changeCh))

	case labsLoadedMsg:
		m.err = nil
		m.labs = msg.labs
		m.statusBar.LabCount = len(msg.labs)
		running := 0
		for _, l := range msg.labs {
			if l.Status() == engine.LabStatusRunning {
				running++
			}
		}
		m.statusBar.RunningCount = running
		if m.initialLab != nil {
			init := m.initialLab
			m.initialLab = nil
			m.startupPickerShown = true
			m.activeTab = tabTopology
			m.curTopoLab = init.Name
			return m, loadTopology(m.eng, m.curTopoLab)
		}
		if !m.startupPickerShown {
			m.startupPickerShown = true
			return m, m.initLabPicker()
		}
		return m, m.maybeLoadTopology()
	case topologyLoadedMsg:
		if msg.lab != nil {
			m.curTopoLab = msg.lab.Name
			m.devTree.SetTopology(msg.lab)
		}
		return m, m.refreshDetail(m.detailNodeName())

	case monitorMsg:
		if msg.node == m.detailNodeName() {
			m.detailPane.SetMonitor(msg.monitor)
		}
		return m, nil

	case interfaceIPMsg:
		if msg.node == m.detailNodeName() {
			m.devTree.SetInterfaceIPs(msg.node, msg.ips)
		}
		return m, nil

	case interfaceStatsMsg:
		if msg.node == m.detailNodeName() {
			if msg.err != nil {
				m.detailPane.SetInterfaceStatsUnavailable()
			} else {
				m.detailPane.SetInterfaceStats(msg.stats)
			}
		}
		return m, nil

	case monitorTickMsg:
		if m.activeTab == tabTopology && msg.node == m.detailNodeName() {
			return m, tea.Batch(m.collectMonitor(m.curTopoLab, msg.node), m.collectInterfaceStats(m.curTopoLab, msg.node), m.scheduleMonitorTick(msg.node))
		}
		return m, nil
	case opLineMsg:
		m.opsView.AppendLine(msg.Line)
		if msg.Line.Done {
			m.currentOpCh = nil
			return m, m.afterOperation()
		}
		return m, runOperation(m.currentOpCh)

	case operationRefreshedMsg:
		cmds := []tea.Cmd{loadLabs(m.eng)}
		if m.curTopoLab != "" {
			cmds = append(cmds, loadTopology(m.eng, m.curTopoLab))
		}
		return m, tea.Batch(cmds...)

	case sessionOutputMsg:
		m.sessionModel.Feed(msg.ID, msg.Data)
		for _, s := range m.sessionModel.Sessions() {
			if s.Handle.ID == msg.ID {
				return m, m.pullSessionOutput(s.Handle)
			}
		}
		return m, nil
	case sessionDoneMsg:
		if m.sessionModel.RemoveSession(msg.ID) {
			return m, m.toast.Show("session ended: " + msg.ID)
		}
		return m, nil

	case tabs.SessionPickedMsg:
		if msg.Session != nil {
			m.sessionModel.FocusSession(msg.Session.ID)
			m.sessionModel.SetMode(tabs.SessionInsert)
		}
		return m, nil

	case logStreamMsg:
		if !msg.Line.Done {
			m.logView.AppendLine(msg.Line.Line)
		}
		return m, m.streamLogs(msg.Ch)
	case logStreamDoneMsg:
		if m.currentLogCh == msg.Ch {
			m.currentLogCh = nil
			m.logCancel = nil
		}
		return m, nil

	case topologyReloadMsg:
		if m.curTopoLab != "" {
			return m, loadTopology(m.eng, m.curTopoLab)
		}
		return m, nil

	case graphMsg:
		if msg.err != nil {
			return m, m.toast.Show("graph: " + msg.err.Error())
		}
		return m, tea.Batch(m.toast.Show("Graph: "+msg.url), launchBrowserCmd(msg.url))

	case errMsg:
		m.errSeq++
		m.err = msg.err
		seq := m.errSeq
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return errExpiredMsg{seq: seq} })
	case errExpiredMsg:
		if msg.seq == m.errSeq {
			m.err = nil
		}
		return m, nil
	}

	return m, nil
}

// handleChange dispatches an engine Change to the appropriate refresh.
func (m *Model) handleChange(c engine.Change) tea.Cmd {
	switch c.Type {
	case engine.ChangeLabAdded:
		return loadLabs(m.eng)
	case engine.ChangeLabRemoved:
		// If the removed lab is the one on screen, clear the stale tree so
		// curTopoLab never points at a dead lab.
		if c.LabName == m.curTopoLab {
			m.curTopoLab = ""
			m.devTree.SetTopology(&engine.Lab{})
		}
		return loadLabs(m.eng)
	case engine.ChangeNodeAdded, engine.ChangeNodeUpdated, engine.ChangeNodeRemoved,
		engine.ChangeInterfaceAdded, engine.ChangeInterfaceUpdated, engine.ChangeInterfaceRemoved,
		engine.ChangeLinkAdded, engine.ChangeLinkUpdated, engine.ChangeLinkRemoved:
		var cmds []tea.Cmd
		// Keep lab list / status bar status fresh.
		cmds = append(cmds, loadLabs(m.eng))
		// Refresh the displayed lab's DevTree if relevant.
		if m.activeTab == tabTopology && c.LabName == m.curTopoLab {
			cmds = append(cmds, loadTopology(m.eng, c.LabName))
		}
		return tea.Batch(cmds...)
	}
	return nil
}

// initLabPicker opens the lab picker at startup. Always shows even for a
// single lab, unless there are no labs.
func (m *Model) initLabPicker() tea.Cmd {
	if len(m.labs) == 0 {
		return nil
	}
	m.labPicker.Show(m.labs)
	return nil
}

// operationRefreshedMsg is emitted after a lifecycle operation completes and
// live state has been re-read from the engine.
type operationRefreshedMsg struct{}

// afterOperation re-reads live state after a lifecycle operation and triggers a
// lab/topology reload. Engines without a usable event stream (for example a
// non-root containerlab, whose `events` command needs privileges) rely on this
// to reflect node start/stop changes.
func (m *Model) afterOperation() tea.Cmd {
	return func() tea.Msg {
		if m.eng != nil {
			if r, ok := m.eng.(engine.LiveRefresher); ok {
				_ = r.RefreshLive(context.Background())
			}
		}
		return operationRefreshedMsg{}
	}
}

// maybeLoadTopology returns a command that reloads the current lab's topology
// when the topology tab is active.
func (m *Model) maybeLoadTopology() tea.Cmd {
	if m.activeTab != tabTopology || m.curTopoLab == "" {
		return nil
	}
	return loadTopology(m.eng, m.curTopoLab)
}

// buildWhichKeyRoot returns the which-key menu tree for the active tab and
// state. Leaves reuse the ActionSelectedMsg IDs handled by the model's
// ActionSelectedMsg case.
func (m *Model) buildWhichKeyRoot() []tabs.MenuNode {
	switch m.activeTab {
	case tabTopology:
		return []tabs.MenuNode{
			{Label: "Lab", DirectKey: "l", Children: []tabs.MenuNode{
				{Label: "Switch Lab", ActionID: "switchlab"},
				{Label: "Deploy", ActionID: "deploy"},
				{Label: "Destroy", ActionID: "destroy"},
				{Label: "Redeploy", ActionID: "redeploy"},
				{Label: "Edit YAML", ActionID: "edit"},
				{Label: "View Graph", ActionID: "graph"},
			}},
			{Label: "Node", DirectKey: "o", Children: []tabs.MenuNode{
				{Label: "SSH", ActionID: "ssh"},
				{Label: "Start", ActionID: "start"},
				{Label: "Stop", ActionID: "stop"},
				{Label: "Restart", ActionID: "restart"},
				{Label: "Pause", ActionID: "pause"},
				{Label: "Unpause", ActionID: "unpause"},
				{Label: "View Logs", ActionID: "logs"},
			}},
			{Label: "Trace", DirectKey: "t", ActionID: "trace"},
			{Label: "Filter", DirectKey: "f", ActionID: "filter"},
			{Label: "Search", DirectKey: "/", ActionID: "search"},
			{Label: "Toggle Details", DirectKey: "x", ActionID: "toggle-conns"},
		}
	case tabSessions:
		return []tabs.MenuNode{
			{Label: "Switch Session", DirectKey: "s", ActionID: "session-picker"},
		}
	case tabLogs:
		return []tabs.MenuNode{
			{Label: "Follow", DirectKey: "f", ActionID: "logs-follow"},
			{Label: "Clear", DirectKey: "c", ActionID: "logs-clear"},
			{Label: "Top", DirectKey: "g", ActionID: "logs-top"},
			{Label: "Bottom", DirectKey: "G", ActionID: "logs-bottom"},
		}
	case tabOps:
		return []tabs.MenuNode{
			{Label: "Follow", DirectKey: "f", ActionID: "ops-follow"},
			{Label: "Clear", DirectKey: "c", ActionID: "ops-clear"},
		}
	}
	return nil
}

// showLabMenu opens the lab actions overlay (Switch Lab / Deploy / Destroy /
// Redeploy) on the shared ActionMenu component.
func (m *Model) showLabMenu() {
	m.actionMenu.Show("Lab Actions", []tabs.ActionItem{
		{Label: "Switch Lab", ID: "switchlab"},
		{Label: "Deploy", ID: "deploy"},
		{Label: "Destroy", ID: "destroy"},
		{Label: "Redeploy", ID: "redeploy"},
		{Label: "Edit YAML", ID: "edit"},
		{Label: "View Graph", ID: "graph"},
	})
}

// currentLab resolves the lab the LabMenu actions should operate on.
func (m *Model) currentLab() *engine.Lab {
	for _, l := range m.labs {
		if l.Name == m.curTopoLab {
			return l
		}
	}
	return nil
}

func (m *Model) deployCurrentLab() tea.Cmd {
	if cmd := m.requireCap(capability.ReqClabPrivileged); cmd != nil {
		return cmd
	}
	lab := m.currentLab()
	if lab == nil {
		return m.toast.Show("no lab selected")
	}
	if m.eng == nil {
		return m.toast.Show("engine unavailable (limited mode)")
	}
	m.activeTab = tabOps
	m.opsView.SetLabel("Deploying: " + lab.Name)
	ch, err := m.eng.Deploy(context.Background(), lab.TopoFile)
	if err != nil {
		return func() tea.Msg { return errMsg{err} }
	}
	m.currentOpCh = ch
	return runOperation(ch)
}

func (m *Model) destroyCurrentLab() tea.Cmd {
	if cmd := m.requireCap(capability.ReqClabPrivileged); cmd != nil {
		return cmd
	}
	lab := m.currentLab()
	if lab == nil {
		return m.toast.Show("no lab selected")
	}
	if m.eng == nil {
		return m.toast.Show("engine unavailable (limited mode)")
	}
	m.activeTab = tabOps
	m.opsView.SetLabel("Destroying: " + lab.Name)
	ch, err := m.eng.Destroy(context.Background(), lab.Name)
	if err != nil {
		return func() tea.Msg { return errMsg{err} }
	}
	m.currentOpCh = ch
	return runOperation(ch)
}

func (m *Model) redeployCurrentLab() tea.Cmd {
	if cmd := m.requireCap(capability.ReqClabPrivileged); cmd != nil {
		return cmd
	}
	lab := m.currentLab()
	if lab == nil {
		return m.toast.Show("no lab selected")
	}
	if m.eng == nil {
		return m.toast.Show("engine unavailable (limited mode)")
	}
	m.activeTab = tabOps
	m.opsView.SetLabel("Redeploying: " + lab.Name)
	ch, err := m.eng.Redeploy(context.Background(), lab.Name)
	if err != nil {
		return func() tea.Msg { return errMsg{err} }
	}
	m.currentOpCh = ch
	return runOperation(ch)
}

// editCurrentLabYAML opens the current lab's topology file in the editor.
func (m *Model) editCurrentLabYAML() tea.Cmd {
	lab := m.currentLab()
	if lab == nil || lab.TopoFile == "" {
		return m.toast.Show("no lab selected")
	}
	return m.editYAMLCmd(lab.TopoFile)
}

// runNodeAction dispatches a node action selected from the action menu.
func (m *Model) runNodeAction(id string) tea.Cmd {
	sel := m.devTree.Selected()
	if sel == nil {
		return m.toast.Show("no node selected")
	}
	nodeName := sel.Name
	labName := m.curTopoLab
	if labName == "" {
		return m.toast.Show("no lab selected")
	}
	if m.eng == nil {
		return m.toast.Show("engine unavailable (limited mode)")
	}

	ctx := context.Background()
	switch id {
	case "start", "stop", "restart", "pause", "unpause":
		// Confirm first, then execute on ConfirmResultMsg.
		m.confirm.Show(fmt.Sprintf("%s node?", strings.ToUpper(id)), fmt.Sprintf("%s node %s in lab %s?", id, nodeName, labName), id)
		return nil
	case "ssh":
		sm, ok := m.eng.(engine.SessionManager)
		if !ok {
			return m.toast.Show("engine does not support sessions")
		}
		handle, err := sm.OpenSession(ctx, labName, nodeName, engine.SessionModeShell)
		if err != nil {
			return func() tea.Msg { return errMsg{err} }
		}
		m.sessionModel.AddSession(handle)
		m.activeTab = tabSessions
		return m.pullSessionOutput(handle)
	case "logs":
		ls, ok := m.eng.(engine.LogStreamer)
		if !ok {
			return m.toast.Show("engine does not support logs")
		}
		// Cancel any previous log stream so a second view does not leak the
		// old `docker logs -f` process.
		if m.logCancel != nil {
			m.logCancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		ch, err := ls.StreamNodeLogs(ctx, labName, nodeName)
		if err != nil {
			cancel()
			return func() tea.Msg { return errMsg{err} }
		}
		m.logCancel = cancel
		m.currentLogCh = ch
		m.activeTab = tabLogs
		m.logView.Clear()
		m.logView.SetLabel(fmt.Sprintf("Logs: %s", nodeName))
		return m.streamLogs(ch)
	case "capture":
		return m.startCaptureMenu()
	default:
		return m.toast.Show("unknown action: " + id)
	}
}

func (m *Model) setNetem(state engine.NetemState) tea.Cmd {
	np, ok := m.eng.(engine.NetemProvider)
	if !ok {
		return m.toast.Show("engine does not support netem")
	}
	ch, err := np.SetInterfaceNetem(context.Background(), m.curTopoLab, m.netemNode, m.netemIface, state)
	if err != nil {
		return func() tea.Msg { return errMsg{err} }
	}
	m.activeTab = tabOps
	m.opsView.SetLabel(fmt.Sprintf("netem: %s:%s", m.netemNode, m.netemIface))
	m.currentOpCh = ch
	return runOperation(ch)
}

func (m *Model) clearNetem(nodeName, iface string) tea.Cmd {
	np, ok := m.eng.(engine.NetemProvider)
	if !ok {
		return m.toast.Show("engine does not support netem")
	}
	ch, err := np.ClearInterfaceNetem(context.Background(), m.curTopoLab, nodeName, iface)
	if err != nil {
		return func() tea.Msg { return errMsg{err} }
	}
	m.activeTab = tabOps
	m.opsView.SetLabel(fmt.Sprintf("clear netem: %s:%s", nodeName, iface))
	m.currentOpCh = ch
	return runOperation(ch)
}

// executeNodeAction runs a confirmed node lifecycle operation.
func (m *Model) executeNodeAction(id string) tea.Cmd {
	sel := m.devTree.Selected()
	if sel == nil {
		return nil
	}
	labName := m.curTopoLab
	var ch <-chan engine.OutputLine
	var err error
	ctx := context.Background()
	switch id {
	case "start":
		ch, err = m.eng.StartNode(ctx, labName, sel.Name)
	case "stop":
		ch, err = m.eng.StopNode(ctx, labName, sel.Name)
	case "restart":
		ch, err = m.eng.RestartNode(ctx, labName, sel.Name)
	case "pause":
		ch, err = m.eng.PauseNode(ctx, labName, sel.Name)
	case "unpause":
		ch, err = m.eng.UnpauseNode(ctx, labName, sel.Name)
	}
	if err != nil {
		return func() tea.Msg { return errMsg{err} }
	}
	m.activeTab = tabOps
	title := fmt.Sprintf("%s: %s", id, sel.Name)
	m.opsView.SetLabel(title)
	m.currentOpCh = ch
	return runOperation(ch)
}

// startCaptureMenu opens the interface picker for capture on the selected node.
func (m *Model) startCaptureMenu() tea.Cmd {
	m.traceMode = false
	if m.eng == nil {
		return m.toast.Show("engine unavailable (limited mode)")
	}
	if cc, ok := m.eng.(engine.CapabilityChecker); ok {
		r := cc.PacketTraceCapable(context.Background())
		if !r.Available {
			return m.toast.Show("capture " + r.Reason)
		}
	}
	node := m.devTree.Selected()
	if node == nil {
		return m.toast.Show("no node selected")
	}
	var items []tabs.ActionItem
	for _, ifc := range node.Interfaces {
		if ifc.Name == "" {
			continue
		}
		items = append(items, tabs.ActionItem{Label: ifc.Name, ID: "iface:" + ifc.Name})
	}
	if len(items) == 0 {
		return m.toast.Show("node has no interfaces")
	}
	m.actionMenu.Show("Capture on "+node.Name, items)
	return nil
}

// startCapture starts packet capture on one interface and shows the pane.
func (m *Model) startCapture(labName, nodeName, iface, filter string) tea.Cmd {
	return m.startCaptureFile(labName, nodeName, iface, filter, "")
}

func (m *Model) startCaptureFile(labName, nodeName, iface, filter, pcapPath string) tea.Cmd {
	if cmd := m.requireCap(capability.ReqTrace); cmd != nil {
		return cmd
	}
	pc, ok := m.eng.(engine.PacketCapturer)
	if !ok {
		return m.toast.Show("engine does not support capture")
	}
	opts := []engine.CaptureOption{}
	if filter != "" {
		opts = append(opts, engine.WithCaptureFilter(filter))
	}
	if pcapPath != "" {
		opts = append(opts, engine.WithCaptureFile(pcapPath))
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch, stop, err := pc.Capture(ctx, labName, nodeName, iface, opts...)
	var warning *engine.CaptureWarningError
	partial := err != nil && errors.As(err, &warning) && ch != nil && stop != nil
	if err != nil && !partial {
		cancel()
		return func() tea.Msg { return errMsg{err} }
	}
	m.stopCapture()
	m.captureCancel = cancel
	m.captureStop = stop
	m.captureEventCh = ch
	m.captureIface = iface
	m.captureFilter = filter
	m.capturePane.Show()
	m.capturePane.Clear()
	if warning != nil {
		return tea.Batch(m.toast.Show(warning.Error()), m.pullCaptureEvent(ch))
	}
	return m.pullCaptureEvent(ch)
}

// stopCapture stops any active capture, cancels its context, and hides the
// pane. It is idempotent.
func (m *Model) stopCapture() {
	if m.captureStop != nil {
		_ = m.captureStop()
	}
	if m.captureCancel != nil {
		m.captureCancel()
	}
	m.captureStop = nil
	m.captureCancel = nil
	m.capturePane.Hide()
	m.capturePane.Clear()
}

// pullCaptureEvent pulls the next packet from the capture stream as a tea.Cmd.
func (m *Model) pullCaptureEvent(ch <-chan engine.PacketEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return captureStoppedMsg{}
		}
		return captureEventMsg{ev}
	}
}

// toggleTrace starts path tracing (picking a filter on first use) or stops it.
func (m *Model) toggleTrace() tea.Cmd {
	if m.tracing {
		m.stopTrace()
		return m.toast.Show("trace stopped")
	}
	if cmd := m.requireCap(capability.ReqTrace); cmd != nil {
		return cmd
	}
	if m.eng == nil {
		return m.toast.Show("engine unavailable (limited mode)")
	}
	if cc, ok := m.eng.(engine.CapabilityChecker); ok {
		r := cc.PacketTraceCapable(context.Background())
		if !r.Available {
			return m.toast.Show("trace " + r.Reason)
		}
	}
	if !m.traceFilterSet {
		m.traceMode = true
		m.actionMenu.Show("Trace filter", presetTraceFilters)
		return nil
	}
	return m.startTrace(m.traceFilter)
}

// changeTraceFilter opens the filter picker from the topology tab. If a trace
// is active, stop it first; selecting a new filter starts a fresh trace.
func (m *Model) changeTraceFilter() tea.Cmd {
	if m.tracing {
		m.stopTrace()
	}
	m.traceMode = true
	m.actionMenu.Show("Trace filter", presetTraceFilters)
	return nil
}

// stopTrace stops a running path trace and clears its packet-hit counters from
// the device tree. It is idempotent.
func (m *Model) stopTrace() {
	if m.traceStop != nil {
		_ = m.traceStop()
	}
	m.tracing = false
	m.traceStop = nil
	m.traceCancel = nil
	m.traceBlinkOn = false
	m.devTree.SetPathBlink(false)
	m.devTree.ClearPathHits()
}

// startTrace starts whole-topology path tracing with the given BPF filter.
func (m *Model) startTrace(filter string) tea.Cmd {
	if cmd := m.requireCap(capability.ReqTrace); cmd != nil {
		return cmd
	}
	pt, ok := m.eng.(engine.PacketPathTracer)
	if !ok {
		return m.toast.Show("engine does not support path tracing")
	}
	m.traceFilter = filter
	m.traceFilterSet = true
	m.devTree.ClearPathHits()
	ctx, cancel := context.WithCancel(context.Background())
	ch, stop, err := pt.TracePath(ctx, m.curTopoLab, filter)
	if err != nil {
		cancel()
		return func() tea.Msg { return errMsg{err} }
	}
	m.traceSeq++
	seq := m.traceSeq
	m.traceBlinkOn = true
	m.devTree.SetPathBlink(true)
	m.tracing = true
	m.traceCancel = cancel
	m.traceStop = stop
	m.traceEventCh = ch
	return tea.Batch(m.toast.Show("tracing... (t to stop)"), m.pullTraceEvent(ch, seq), m.scheduleTraceBlink(seq))
}

func (m *Model) scheduleTraceBlink(seq int) tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg {
		return traceBlinkMsg{seq: seq}
	})
}

// pullTraceEvent pulls the next packet from the trace stream as a tea.Cmd.
func (m *Model) pullTraceEvent(ch <-chan engine.PacketEvent, seq int) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return traceStoppedMsg{seq: seq}
		}
		return traceEventMsg{ev: ev, seq: seq}
	}
}

// pullSessionOutput pulls the next line from a session's output stream.
func (m *Model) pullSessionOutput(h *engine.SessionHandle) tea.Cmd {
	return func() tea.Msg {
		chunk, ok := <-h.Output
		if !ok {
			return sessionDoneMsg{ID: h.ID}
		}
		return sessionOutputMsg{ID: h.ID, Data: chunk}
	}
}

// editYAMLCmd opens the topology file in the user's editor; reload on exit.
func (m *Model) editYAMLCmd(path string) tea.Cmd {
	parts := strings.Fields(os.Getenv("EDITOR"))
	if len(parts) == 0 {
		parts = []string{"vi"}
	}
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return topologyReloadMsg{}
	})
}

// openGraphView starts the browser graph server for the current lab.
func (m *Model) openGraphView() tea.Cmd {
	lab := m.currentLab()
	if lab == nil || lab.TopoFile == "" {
		return m.toast.Show("no lab selected")
	}
	if m.graph == nil {
		m.graph = newGraphServer()
	}
	return func() tea.Msg {
		url, err := m.graph.start(lab.TopoFile)
		return graphMsg{url: url, err: err}
	}
}

// streamLogs pulls the next line from a node log stream. The channel is
// captured so rescheduling always re-pulls the same stream.
func (m *Model) streamLogs(ch <-chan engine.OutputLine) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return logStreamDoneMsg{Ch: ch}
		}
		return logStreamMsg{Ch: ch, Line: line}
	}
}

// forwardKey dispatches a key press to the active tab's model so its own
// keybindings (j/k navigation, Enter selection, etc.) take effect.
func (m *Model) forwardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.activeTab {
	case tabTopology:
		// x toggles all connection summaries; enter toggles the selected
		// node's connections; o opens the action menu.
		prev := m.detailNodeName()
		switch msg.String() {
		case "x":
			m.devTree.ToggleAll()
		case "i":
			m.devTree.ToggleInterfaces()
			return m, nil
		case "enter":
			if selection, ok := m.devTree.SelectedResource(); ok && selection.Kind == tabs.ResourceNode {
				m.devTree.ToggleSelected()
			}
		case "o":
			lab := m.currentLab()
			if lab == nil {
				return m, m.toast.Show("no lab selected")
			}
			selection, selected := m.devTree.SelectedResource()
			if selected && selection.Kind == tabs.ResourceInterface {
				m.actionMenu.Show("Interface Actions", []tabs.ActionItem{
					{Label: "Packet Capture", ID: "interface-capture"},
					{Label: "Set Netem", ID: "netem-set"},
					{Label: "Clear Netem", ID: "netem-clear"},
				})
				return m, nil
			}
			m.actionMenu.Show("Node Actions", []tabs.ActionItem{
				{Label: "SSH", ID: "ssh"},
				{Label: "Start", ID: "start"},
				{Label: "Stop", ID: "stop"},
				{Label: "Restart", ID: "restart"},
				{Label: "Pause", ID: "pause"},
				{Label: "Unpause", ID: "unpause"},
				{Label: "View Logs", ID: "logs"},
			})
			return m, nil
		case "t":
			return m, m.toggleTrace()
		case "f":
			return m, m.changeTraceFilter()
		default:
			m.devTree, cmd = m.devTree.Update(msg)
		}
		if cur := m.detailNodeName(); cur != prev {
			return m, tea.Batch(cmd, m.refreshDetail(cur))
		}
		return m, cmd
	case tabLogs:
		m.logView, cmd = m.logView.Update(msg)
	case tabOps:
		m.opsView, cmd = m.opsView.Update(msg)
	case tabSessions:
		m.sessionModel, cmd = m.sessionModel.Update(msg)
	}
	return m, cmd
}

// detailNodeName returns the currently selected node's name ("" if none).
func (m *Model) detailNodeName() string {
	if n := m.devTree.Selected(); n != nil {
		return n.Name
	}
	return ""
}

// refreshDetail updates the detail pane for the selected node and starts the
// periodic monitor refresh.
func (m *Model) refreshDetail(nodeName string) tea.Cmd {
	m.detailPane.SetNode(m.devTree.Selected())
	if nodeName == "" {
		m.detailPane.SetMonitor(nil)
		m.detailPane.SetInterfaceStatsUnavailable()
		m.devTree.ClearInterfaceIPs()
		return nil
	}
	m.devTree.ClearInterfaceIPs() // clear stale IPs until fresh data arrives
	return tea.Batch(
		m.collectMonitor(m.curTopoLab, nodeName),
		m.collectInterfaceStats(m.curTopoLab, nodeName),
		m.collectInterfaceIPs(m.curTopoLab, nodeName),
		m.scheduleMonitorTick(nodeName),
	)
}

func (m *Model) collectInterfaceStats(labName, nodeName string) tea.Cmd {
	return func() tea.Msg {
		sp, ok := m.eng.(engine.InterfaceStatsProvider)
		if !ok || labName == "" || nodeName == "" {
			return interfaceStatsMsg{node: nodeName, err: engine.ErrNotSupported}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		stats, err := sp.MonitorInterfaces(ctx, labName, nodeName)
		return interfaceStatsMsg{node: nodeName, stats: stats, err: err}
	}
}

// collectMonitor fetches one NodeMonitor snapshot for the node. labName is
// captured at scheduling time (on the main loop), never read from m inside the
// goroutine, so there is no data race with Update mutating m on the main loop.
func (m *Model) collectMonitor(labName, nodeName string) tea.Cmd {
	return func() tea.Msg {
		mp, ok := m.eng.(engine.NodeMonitorProvider)
		if !ok {
			return nil
		}
		if labName == "" || nodeName == "" {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		mon, err := mp.MonitorNode(ctx, labName, nodeName)
		if err != nil {
			return monitorMsg{node: nodeName, monitor: nil, err: err}
		}
		return monitorMsg{node: nodeName, monitor: mon}
	}
}

// scheduleMonitorTick schedules the next 2s monitor refresh for a node.
func (m *Model) scheduleMonitorTick(nodeName string) tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return monitorTickMsg{node: nodeName}
	})
}

// collectInterfaceIPs fetches the selected node's interface IPs asynchronously.
// labName/nodeName are captured at scheduling time, never read from m inside
// the goroutine, so there is no data race with Update mutating m.
func (m *Model) collectInterfaceIPs(labName, nodeName string) tea.Cmd {
	return func() tea.Msg {
		ip, ok := m.eng.(engine.InterfaceIPProvider)
		if !ok {
			return nil
		}
		if labName == "" || nodeName == "" {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ips, err := ip.InterfaceIPs(ctx, labName, nodeName)
		if err != nil {
			return interfaceIPMsg{node: nodeName, ips: nil, err: err}
		}
		return interfaceIPMsg{node: nodeName, ips: ips}
	}
}

func (m *Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	tabBar := renderTabBar(m.activeTab)
	content := m.renderActiveTab()
	if m.labPicker.Visible() {
		content = popup.Place(content, m.labPicker, m.width, m.height-2)
	}
	if m.actionMenu.Visible() {
		content = popup.Place(content, m.actionMenu, m.width, m.height-2)
	}
	if m.searchInput.Visible() && (m.pendingFilter || m.pendingCaptureFile || m.activeTab != tabTopology) {
		content = popup.Place(content, m.searchInput, m.width, m.height-2)
	}
	if m.netemForm.Visible() {
		content = popup.Place(content, m.netemForm, m.width, m.height-2)
	}
	if m.confirm.Visible() {
		content = popup.Place(content, m.confirm, m.width, m.height-2)
	}
	if m.sessionPicker.Visible() {
		content = popup.Place(content, m.sessionPicker, m.width, m.height-2)
	}
	// 底部叠加层：toast/err/capturePane/tracing 统一覆盖 content 底部
	// （自底向上：toast → err → capturePane → tracing），保证总高度不变，
	// help 栏始终贴底、无空行。
	var bottom []string
	if m.activeTab == tabTopology {
		if m.capturePane.Visible() {
			bottom = append(bottom, m.capturePane.View())
		}
	}
	if len(bottom) > 0 {
		content = overlayBottom(content, strings.Join(bottom, "\n"))
	}
	// Which-key 最后叠加，确保它绘制在底部 strip 之上而不被截断。
	if m.whichKey.Visible() {
		content = popup.Place(content, m.whichKey, m.width, m.height-2)
	}

	frame := AppStyle.Render(
		tabBar + "\n" +
			content + "\n" +
			m.renderStatusBar(),
	)
	return strings.TrimRight(frame, "\n")
}

func (m *Model) topologyNotice() string {
	if m.searchInput.Visible() && !m.pendingFilter && !m.pendingCaptureFile {
		return m.searchInput.InlineText()
	}
	if status := m.devTree.SearchStatus(); status != "" {
		return status
	}
	return ""
}

// overlayBottom 把 strip 覆盖到 content 底部：从 content 底部截断与 strip
// 相同行数，再把 strip 追加到末尾，使总行数保持不变。
func overlayBottom(content, strip string) string {
	stripLines := strings.Split(strip, "\n")
	n := len(stripLines)
	contentLines := strings.Split(content, "\n")
	if len(contentLines) > n {
		content = strings.Join(contentLines[:len(contentLines)-n], "\n")
	} else {
		content = ""
	}
	if content != "" {
		content += "\n"
	}
	return content + strip
}

// renderHelpBar renders the bottom hint line. It is deliberately minimal:
// the space menu is the discoverability surface, so only space + quit (plus
// modal-specific hints while a capture is open or a session is live) appear.
func renderHelpBar(m *Model, showAll bool) string {
	var bindings []key.Binding
	switch {
	case m.capturePane.Visible():
		pauseDesc := "pause"
		if m.capturePane.Paused() {
			pauseDesc = "resume"
		}
		bindings = []key.Binding{
			key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "close")),
			key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear")),
			key.NewBinding(key.WithKeys("p"), key.WithHelp("p", pauseDesc)),
			key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "save pcap")),
		}
	case m.activeTab == tabSessions && m.sessionModel.HasSessions() && m.sessionModel.IsInsert():
		bindings = SessionInsertHelp()
	default:
		bindings = []key.Binding{spaceMenu}
		if m.activeTab == tabTopology && m.tracing {
			bindings = append(bindings,
				key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "trace stop")),
				key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "change filter")),
			)
		}
		if m.activeTab == tabSessions && m.sessionModel.HasSessions() {
			bindings = append(bindings,
				key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "to insert")))
		} else {
			bindings = append(bindings, Keys.Quit)
		}
	}
	if showAll {
		bindings = append(bindings, Keys.ShiftTab, Keys.Search, Keys.Help)
	}
	var parts []string
	for _, b := range bindings {
		h := b.Help()
		if h.Key == "" {
			continue
		}
		parts = append(parts, KeyLabelStyle.Render(h.Key)+" "+h.Desc)
	}
	return strings.Join(parts, "  •  ")
}

// statusRight returns the transient/status hint text and its color to show on
// the right side of the status bar. Priority: toast > error > tracing >
// limited mode.
func (m *Model) statusRight() (string, lipgloss.Color) {
	if m.toast.Visible() {
		return m.toast.Message(), lipgloss.Color("196")
	}
	if m.err != nil {
		return "ERROR: " + m.err.Error(), lipgloss.Color("196")
	}
	if m.tracing {
		return "● TRACING", lipgloss.Color("42")
	}
	if _, reason := m.caps.Check(capability.ReqClabPrivileged); reason != "" {
		return "limited mode", lipgloss.Color("214")
	}
	return "", ""
}

// renderStatusBar composes a tmux-style bottom line: shortcut hints on the
// left, status hints (toast/tracing/limited mode/errors) on the right.
func (m *Model) renderStatusBar() string {
	left := renderHelpBar(m, m.help.ShowAll)
	text, color := m.statusRight()
	if text == "" {
		return helpBarStyle.Render(left)
	}
	inner := m.width - 2
	leftW := lipgloss.Width(left)
	if leftW > inner {
		leftW = inner
	}
	limit := inner - leftW - 1
	if limit < 1 {
		limit = 1
	}
	trunc := truncateRunes(text, limit)
	right := lipgloss.NewStyle().Foreground(color).Bold(true).Render(trunc)
	rightW := lipgloss.Width(right)
	pad := inner - leftW - rightW
	if pad < 1 {
		pad = 1
	}
	return helpBarStyle.Render(left + strings.Repeat(" ", pad) + right)
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit < 2 {
		return string(runes[:1])
	}
	return string(runes[:limit-1]) + "…"
}

var (
	helpBarStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Padding(0, 1)
)

func (m *Model) nextTab() {
	if m.activeTab < tabOps {
		m.activeTab++
	} else {
		m.activeTab = tabTopology
	}
}

func (m *Model) prevTab() {
	if m.activeTab > tabTopology {
		m.activeTab--
	} else {
		m.activeTab = tabOps
	}
}

func renderTabBar(active tab) string {
	var bar string
	for i, name := range TabNames {
		st := TabStyle
		if tab(i) == active {
			st = ActiveTabStyle
		}
		// Style the digit and the name separately so the red digit's trailing
		// reset cannot strip the tab style from the name.
		num := fmt.Sprintf("%d", i+1)
		bar += st.UnsetPadding().Render(KeyLabelStyle.Render(num)) +
			st.UnsetPadding().Render(" "+name) +
			"  "
	}
	return bar
}

func (m *Model) renderActiveTab() string {
	switch m.activeTab {
	case tabTopology:
		m.devTree.SetFooterNotice(m.topologyNotice())
		if m.width <= 0 {
			return lipgloss.JoinHorizontal(lipgloss.Left,
				TopologyPaneStyle.Render(m.devTree.View()),
				TopologyPaneStyle.Render(m.detailPane.View()))
		}
		leftWidth := m.width * 2 / 3
		rightWidth := m.width - leftWidth
		paneHeight := maxInt(m.height-4, 1)
		treeStyle := TopologyPaneStyle.Copy().Width(maxInt(leftWidth-2, 1)).Height(paneHeight)
		detailStyle := TopologyPaneStyle.Copy().Width(maxInt(rightWidth-2, 1)).Height(paneHeight)
		tree := treeStyle.Render(strings.TrimRight(m.devTree.View(), "\n"))
		detail := detailStyle.Render(strings.TrimRight(m.detailPane.View(), "\n"))
		return lipgloss.JoinHorizontal(lipgloss.Left, tree, detail)
	case tabLogs:
		return m.logView.View()
	case tabOps:
		return m.opsView.View()
	case tabSessions:
		return m.sessionModel.View()
	default:
		return "Unknown tab"
	}
}

func (m *Model) dispatchSize(w, h int) {
	leftWidth := w * 2 / 3
	rightWidth := w - leftWidth
	innerHeight := h - 2
	if innerHeight < 1 {
		innerHeight = 1
	}
	leftInnerWidth := leftWidth - 2
	rightInnerWidth := rightWidth - 2
	if leftInnerWidth < 1 {
		leftInnerWidth = 1
	}
	if rightInnerWidth < 1 {
		rightInnerWidth = 1
	}
	m.devTree.SetSize(leftInnerWidth, innerHeight)
	m.detailPane.SetSize(rightInnerWidth, innerHeight)
	m.labPicker.SetSize(w, h)
	m.actionMenu.SetSize(w, h)
	m.searchInput.SetSize(w, h)
	m.netemForm.SetSize(w, h)
	m.logView.SetSize(w, h)
	m.opsView.SetSize(w, h)
	m.confirm.SetSize(w, h)
	m.sessionModel.SetSize(w, h)
	m.sessionPicker.SetSize(w, h)
	m.capturePane.SetSize(w, h/3)
	m.statusBar.SetSize(w)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// devTreeGroups returns the role-group names currently in the device tree.
// Exposed for tests.
func (m *Model) devTreeGroups() []string {
	return m.devTree.RoleNames()
}
