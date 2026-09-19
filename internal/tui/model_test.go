package tui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/tianyuxue/clab-tui/internal/capability"
	"github.com/tianyuxue/clab-tui/internal/engine"
	"github.com/tianyuxue/clab-tui/internal/tui/tabs"
)

// sessionTestStdin is a bytes.Buffer that satisfies io.WriteCloser.
type sessionTestStdin struct {
	*bytes.Buffer
}

func (sessionTestStdin) Close() error { return nil }

func TestModelInitialization(t *testing.T) {
	m := New(nil, nil)
	if m.activeTab != tabTopology {
		t.Fatalf("expected tabTopology, got %d", m.activeTab)
	}
}

func TestNilEngineFallbackDoesNotPanic(t *testing.T) {
	m := New(nil, nil) // literal nil engine.Engine
	_ = m.Init()       // must not panic
}

func TestLabMenuLimitedModeNoPanic(t *testing.T) {
	m := New(nil, nil)
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/l1.clab.yml"}}
	m.curTopoLab = "lab1"
	// l → LabMenu → Deploy in limited mode must show a toast, not panic.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "deploy"}})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected toast command in limited mode")
	}
	if !m.toast.Visible() {
		t.Fatal("expected toast visible in limited mode")
	}
}

func TestHandleChangeNodeRefreshGating(t *testing.T) {
	lab := &engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "n1", Group: "core"}}}
	f := newFakeEngine()
	f.lab = lab
	m := New(f, []*engine.Lab{lab})
	m.activeTab = tabTopology
	m.curTopoLab = "lab1"

	// Matching lab on topology tab → labs reload + topology refresh.
	if cmd := m.handleChange(engine.Change{Type: engine.ChangeNodeUpdated, LabName: "lab1", NodeName: "n1"}); cmd == nil {
		t.Fatal("expected refresh command for matching lab on topology")
	}

	// Non-matching lab → labs still reload for status freshness, but no
	// topology refresh for a lab we aren't viewing.
	cmd := m.handleChange(engine.Change{Type: engine.ChangeNodeUpdated, LabName: "other", NodeName: "n1"})
	if cmd == nil {
		t.Fatal("expected labs reload command for non-matching lab")
	}
	if _, ok := cmd().(topologyLoadedMsg); ok {
		t.Fatal("expected no topology refresh for non-matching lab")
	}
}

func TestInitReturnsCommand(t *testing.T) {
	m := New(nil, nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected non-nil command from Init")
	}
}

func TestTabSwitching(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab != tabSessions {
		t.Fatalf("expected tabSessions after first tab, got %d", m.activeTab)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab != tabLogs {
		t.Fatalf("expected tabLogs after second tab, got %d", m.activeTab)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab != tabOps {
		t.Fatalf("expected tabOps after third tab, got %d", m.activeTab)
	}
}

func TestTabWrapAround(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabOps
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab != tabTopology {
		t.Fatalf("expected wrap to tabTopology, got %d", m.activeTab)
	}
}

func TestPrevTab(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.activeTab != tabOps {
		t.Fatalf("expected tabOps on prev from first, got %d", m.activeTab)
	}
}

func TestRenderTabBar(t *testing.T) {
	bar := renderTabBar(tabTopology)
	if bar == "" {
		t.Fatal("expected non-empty tab bar")
	}
}

func TestQuitKey(t *testing.T) {
	m := New(nil, nil)
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd != nil || result != m {
	}
}

func TestChangeStreamReadyMsg(t *testing.T) {
	m := New(nil, nil)
	ch := make(chan engine.Change)
	_, cmd := m.Update(changeStreamReadyMsg{ch: ch, cancel: func() {}})
	if cmd == nil {
		t.Fatal("expected non-nil command from changeStreamReadyMsg")
	}
	close(ch)
}

func TestChangeTriggersLoadLabs(t *testing.T) {
	m := New(nil, nil)
	_, cmd := m.Update(engine.Change{Type: engine.ChangeLabRemoved, LabName: "test"})
	if cmd == nil {
		t.Fatal("expected non-nil command from engine.Change")
	}
}

func TestLabsLoadedMsgUpdatesModel(t *testing.T) {
	m := New(nil, nil)
	labs := []*engine.Lab{
		{Name: "lab1", Nodes: []engine.Node{{State: engine.StatusRunning}}},
		{Name: "lab2", Nodes: []engine.Node{{State: engine.StatusStopped}}},
	}
	result, cmd := m.Update(labsLoadedMsg{labs: labs})
	model := result.(*Model)
	if len(model.labs) != 2 {
		t.Fatalf("expected 2 labs, got %d", len(model.labs))
	}
	if model.statusBar.LabCount != 2 {
		t.Fatalf("expected LabCount 2, got %d", model.statusBar.LabCount)
	}
	if model.statusBar.RunningCount != 1 {
		t.Fatalf("expected RunningCount 1, got %d", model.statusBar.RunningCount)
	}
	if cmd != nil {
		t.Fatal("expected nil command from labsLoadedMsg")
	}
}

func TestTabSwitchToTopologyLoadsSelectedLab(t *testing.T) {
	lab := &engine.Lab{
		Name:     "srlinux-ceos-lab",
		TopoFile: "/tmp/test.clab.yml",
		Nodes: []engine.Node{
			{Name: "srl1", Kind: "nokia_srlinux"},
			{Name: "ceos1", Kind: "arista_ceos"},
		},
	}
	f := newFakeEngine()
	f.lab = lab
	m := New(f, []*engine.Lab{lab})
	m.activeTab = tabSessions
	m.curTopoLab = "srlinux-ceos-lab"

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(*Model)
	if m.activeTab != tabTopology {
		t.Fatalf("expected tabTopology, got %d", m.activeTab)
	}
	if cmd == nil {
		t.Fatal("expected non-nil command when entering topology tab")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("expected topologyLoadedMsg from command")
	}
	tlm, ok := msg.(topologyLoadedMsg)
	if !ok {
		t.Fatalf("expected topologyLoadedMsg, got %T", msg)
	}
	if len(tlm.lab.Nodes) != 2 {
		t.Fatalf("expected 2 topology nodes, got %d", len(tlm.lab.Nodes))
	}
}

func TestEnterSelectsLabAndLoadsTopology(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.clab.yml"
	content := `name: my-lab
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
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.labPicker.Show([]*engine.Lab{{Name: "my-lab", TopoFile: path}})

	// Press Enter on the picker: emits LabPickedMsg.
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected a command from Enter on the lab picker")
	}

	// Run the command to produce LabPickedMsg, then feed it back.
	pickedMsg := cmd()
	if _, ok := pickedMsg.(tabs.LabPickedMsg); !ok {
		t.Fatalf("expected LabPickedMsg from command, got %T", pickedMsg)
	}
	result, _ = m.Update(pickedMsg)
	m = result.(*Model)
	if m.activeTab != tabTopology {
		t.Fatalf("expected tabTopology after lab pick, got %d", m.activeTab)
	}
}

func TestLPressedShowsLabMenu(t *testing.T) {
	m := New(nil, []*engine.Lab{{Name: "lab1", TopoFile: "/l1.clab.yml"}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !m.actionMenu.Visible() {
		t.Fatal("expected lab menu visible after l")
	}
	if sel := m.actionMenu.Selected(); sel == nil || sel.ID != "switchlab" {
		t.Fatalf("expected first lab menu item switchlab, got %+v", sel)
	}
}

func TestLabMenuContainsSixItems(t *testing.T) {
	m := New(nil, []*engine.Lab{{Name: "lab1", TopoFile: "/l1.clab.yml"}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !m.actionMenu.Visible() {
		t.Fatal("expected lab menu visible after l")
	}

	items := m.actionMenu.Items()
	if len(items) != 6 {
		t.Fatalf("expected exactly 6 lab menu items, got %d: %v", len(items), items)
	}
	expect := []string{"switchlab", "deploy", "destroy", "redeploy", "edit", "graph"}
	for _, id := range expect {
		found := false
		for _, it := range items {
			if it.ID == id {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected %q in lab menu, got %v", id, items)
		}
	}
}

func TestLabMenuSwitchLabOpensPicker(t *testing.T) {
	m := New(nil, []*engine.Lab{{Name: "lab1", TopoFile: "/l1.clab.yml"}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !m.actionMenu.Visible() {
		t.Fatal("expected lab menu visible after l")
	}
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "switchlab"}})
	m = result.(*Model)
	if !m.labPicker.Visible() {
		t.Fatal("expected lab picker visible after switch lab")
	}
	if cmd != nil {
		t.Fatalf("expected no command from switch lab, got %+v", cmd)
	}
}

func TestLabMenuDeployCallsEngine(t *testing.T) {
	lab := &engine.Lab{Name: "lab1", TopoFile: "/l1.clab.yml"}
	f := newFakeEngine()
	m := New(f, []*engine.Lab{lab}, WithCapabilities(capability.Capabilities{ClabSUID: true}))
	m.activeTab = tabTopology
	m.curTopoLab = "lab1"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "deploy"}})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected deploy command")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("expected opLineMsg from deploy stream")
	}
	if !f.deployCalled {
		t.Fatal("expected Deploy to be called")
	}
	if f.deployPath != "/l1.clab.yml" {
		t.Fatalf("expected Deploy path /l1.clab.yml, got %q", f.deployPath)
	}
	if m.activeTab != tabOps {
		t.Fatalf("expected tabOps after deploy, got %d", m.activeTab)
	}
}

func TestLabMenuDestroyCallsEngine(t *testing.T) {
	lab := &engine.Lab{Name: "lab1", TopoFile: "/l1.clab.yml"}
	f := newFakeEngine()
	m := New(f, []*engine.Lab{lab}, WithCapabilities(capability.Capabilities{ClabSUID: true}))
	m.activeTab = tabTopology
	m.curTopoLab = "lab1"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "destroy"}})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected destroy command")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("expected opLineMsg from destroy stream")
	}
	if !f.destroyCalled {
		t.Fatal("expected Destroy to be called")
	}
	if f.destroyName != "lab1" {
		t.Fatalf("expected Destroy name lab1, got %q", f.destroyName)
	}
}

func TestLabMenuRedeployCallsEngine(t *testing.T) {
	lab := &engine.Lab{Name: "lab1", TopoFile: "/l1.clab.yml"}
	f := newFakeEngine()
	m := New(f, []*engine.Lab{lab}, WithCapabilities(capability.Capabilities{ClabSUID: true}))
	m.activeTab = tabTopology
	m.curTopoLab = "lab1"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "redeploy"}})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected redeploy command")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("expected opLineMsg from redeploy stream")
	}
	if !f.redeployCalled {
		t.Fatal("expected Redeploy to be called")
	}
	if f.redeployName != "lab1" {
		t.Fatalf("expected Redeploy name lab1, got %q", f.redeployName)
	}
}

func TestLabPickedSwitchesLab(t *testing.T) {
	lab2 := &engine.Lab{Name: "lab2", Nodes: []engine.Node{{Name: "leaf-01", Group: "leaf"}}}
	f := newFakeEngine()
	f.lab = lab2
	m := New(f, []*engine.Lab{{Name: "lab1", Nodes: []engine.Node{{Name: "spine-01", Group: "spine"}}}, lab2})
	m.labPicker = tabs.NewLabPicker()
	m.devTree = tabs.NewDevTree()

	// Select lab2 via picker message.
	result, cmd := m.Update(tabs.LabPickedMsg{Lab: lab2})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected load command after lab picked")
	}
	// Run the command: should emit topologyLoadedMsg with lab2's nodes.
	msg := cmd()
	tlm, ok := msg.(topologyLoadedMsg)
	if !ok {
		t.Fatalf("expected topologyLoadedMsg, got %T", msg)
	}
	if len(tlm.lab.Nodes) != 1 || tlm.lab.Nodes[0].Name != "leaf-01" {
		t.Fatalf("expected leaf-01 from lab2, got %+v", tlm.lab.Nodes)
	}
	// Feed back and verify devTree has role groups.
	result, _ = m.Update(msg)
	m = result.(*Model)
	if len(m.devTreeGroups()) == 0 {
		t.Fatal("expected devTree to have role groups after load")
	}
}

func TestAPressedShowsActionMenuOnTopology(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1"}}
	m.curTopoLab = "lab1"
	m.actionMenu = tabs.NewActionMenu()
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x.clab.yml"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{
		Nodes: []engine.Node{{Name: "spine-01", Group: "spine"}},
	})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if !m.actionMenu.Visible() {
		t.Fatal("expected action menu visible after a")
	}
}

func TestEnterTogglesSelectedNodeOnTopology(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{
		Nodes: []engine.Node{{Name: "spine-01", Group: "spine"}},
	})
	// Default all expanded; Enter collapses the selected node.
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.devTree.Selected() == nil {
		t.Fatal("expected a selected node")
	}
	// Action menu must NOT be open.
	if m.actionMenu.Visible() {
		t.Fatal("expected action menu closed after Enter")
	}
}

func TestActionSelectedEditYAML(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/lab.clab.yml"
	os.WriteFile(path, []byte("name: lab\ntopology:\n  nodes:\n    spine-01:\n      kind: k\n      group: spine\n"), 0644)

	lab := &engine.Lab{Name: "lab", TopoFile: path}
	m := New(newFakeEngine(), []*engine.Lab{lab})
	m.activeTab = tabTopology
	m.curTopoLab = "lab"
	m.devTree.SetTopology(lab)
	m.actionMenu = tabs.NewActionMenu()

	// Simulate menu selection: Edit YAML launches the editor without
	// switching tabs.
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{Label: "Edit YAML", ID: "edit"}})
	m = result.(*Model)
	if !m.actionMenu.Hidden() {
		t.Fatal("expected action menu hidden after selection")
	}
	if cmd == nil {
		t.Fatal("expected editYAMLCmd from Edit YAML selection")
	}
	if m.activeTab != tabTopology {
		t.Fatalf("expected to stay on topology tab, got %d", m.activeTab)
	}
}

func TestRunNodeActionStartShowsConfirm(t *testing.T) {
	f := newFakeEngine()
	lab := &engine.Lab{Name: "lab", Nodes: []engine.Node{{Name: "spine-01", Group: "spine"}}}
	m := New(f, []*engine.Lab{lab})
	m.curTopoLab = "lab"
	m.devTree.SetTopology(lab)

	cmd := m.runNodeAction("start")
	if cmd != nil {
		t.Fatal("expected nil cmd while the confirm popup is shown")
	}
	if !m.confirm.Visible() {
		t.Fatal("expected confirm popup visible after start action")
	}
	if f.startCalled {
		t.Fatal("expected no engine call before confirmation")
	}
}

func TestConfirmResultTriggersStartNode(t *testing.T) {
	f := newFakeEngine()
	lab := &engine.Lab{Name: "lab", Nodes: []engine.Node{{Name: "spine-01", Group: "spine"}}}
	m := New(f, []*engine.Lab{lab})
	m.curTopoLab = "lab"
	m.devTree.SetTopology(lab)

	m.runNodeAction("start")
	result, cmd := m.Update(tabs.ConfirmResultMsg{Confirmed: true, ActionID: "start"})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected StartNode command after confirmation")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("expected opLineMsg from StartNode stream")
	}
	if !f.startCalled {
		t.Fatal("expected StartNode to be called")
	}
	if f.startLab != "lab" || f.startNode != "spine-01" {
		t.Fatalf("expected StartNode(lab, spine-01), got (%s, %s)", f.startLab, f.startNode)
	}
	if m.activeTab != tabOps {
		t.Fatalf("expected tabOps after start, got %d", m.activeTab)
	}
}

func TestConfirmPopupReceivesKeys(t *testing.T) {
	f := newFakeEngine()
	lab := &engine.Lab{Name: "lab", Nodes: []engine.Node{{Name: "spine-01", Group: "spine"}}}
	m := New(f, []*engine.Lab{lab})
	m.curTopoLab = "lab"
	m.devTree.SetTopology(lab)

	m.runNodeAction("start")
	// j moves the cursor to cancel; the key must reach the popup, not the tree.
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = result.(*Model)
	if !m.confirm.Visible() {
		t.Fatal("expected confirm still visible after j")
	}
	if cmd != nil {
		t.Fatal("expected no command from j on the confirm popup")
	}
	// Enter on cancel emits ConfirmResultMsg{Confirmed:false}.
	result, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected ConfirmResultMsg command from Enter on confirm popup")
	}
	msg := cmd()
	crm, ok := msg.(tabs.ConfirmResultMsg)
	if !ok {
		t.Fatalf("expected ConfirmResultMsg, got %T", msg)
	}
	if crm.Confirmed {
		t.Fatal("expected cancellation (j then enter) to emit Confirmed:false")
	}
	if f.startCalled {
		t.Fatal("expected no StartNode call for cancelled confirmation")
	}
	// Feed the result back: cancelled confirmations are no-ops.
	result, cmd = m.Update(msg)
	if cmd != nil {
		t.Fatal("expected nil cmd for cancelled confirmation")
	}
}

func TestRunNodeActionLogsStreams(t *testing.T) {
	f := newFakeEngine()
	lab := &engine.Lab{Name: "lab", Nodes: []engine.Node{{Name: "spine-01", Group: "spine"}}}
	m := New(f, []*engine.Lab{lab})
	m.curTopoLab = "lab"
	m.devTree.SetTopology(lab)

	cmd := m.runNodeAction("logs")
	if cmd == nil {
		t.Fatal("expected non-nil log stream command")
	}
	if m.activeTab != tabLogs {
		t.Fatalf("expected tabLogs after logs action, got %d", m.activeTab)
	}
	if m.logView.Label() != "Logs: spine-01" {
		t.Fatalf("expected log view label set, got %q", m.logView.Label())
	}
}

func TestQuitOnSessionsTabClosesSession(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1"})
	m.sessionModel.SetMode(tabs.SessionNormal)

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = result.(*Model)
	if cmd != nil {
		t.Fatalf("expected no quit command, got %+v", cmd)
	}
	if len(m.sessionModel.Sessions()) != 0 {
		t.Fatalf("expected session closed and removed, got %d", len(m.sessionModel.Sessions()))
	}
}

func TestQuitOnSessionsTabEmptyQuits(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected quit command on empty sessions tab")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestSessionOutputMsgReschedulesPull(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1"})

	// Session bytes are fed to the terminal emulator and the output stream
	// must keep being pulled.
	result, cmd := m.Update(sessionOutputMsg{ID: "s1", Data: []byte("root@r1:~# ab\r\n\x1b[31mred\x1b[0m")})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected rescheduled pull command for live session")
	}
	if len(m.sessionModel.Sessions()) != 1 {
		t.Fatalf("expected session retained, got %d", len(m.sessionModel.Sessions()))
	}

	// An unknown session ID does not reschedule anything.
	_, cmd = m.Update(sessionOutputMsg{ID: "unknown", Data: []byte("x")})
	if cmd != nil {
		t.Fatal("expected no command for unknown session")
	}
}

func TestSessionDoneMsgRemovesSessionAndToasts(t *testing.T) {
	m := New(nil, nil)
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1"})

	result, cmd := m.Update(sessionDoneMsg{ID: "s1"})
	m = result.(*Model)
	if len(m.sessionModel.Sessions()) != 0 {
		t.Fatalf("expected session removed, got %d", len(m.sessionModel.Sessions()))
	}
	if cmd == nil {
		t.Fatal("expected toast command on session end")
	}
	if !m.toast.Visible() {
		t.Fatal("expected toast visible on session end")
	}
}

func TestLogStreamMsgRepullsOwnChannelNotCurrent(t *testing.T) {
	m := New(nil, nil)
	other := make(chan engine.OutputLine, 1)
	other <- engine.OutputLine{Line: "WRONG"}
	m.currentLogCh = other
	ch := make(chan engine.OutputLine, 1)
	ch <- engine.OutputLine{Line: "RIGHT"}

	_, cmd := m.Update(logStreamMsg{Ch: ch, Line: engine.OutputLine{Line: "a"}})
	if cmd == nil {
		t.Fatal("expected re-pull command")
	}
	msg := cmd()
	lsm, ok := msg.(logStreamMsg)
	if !ok {
		t.Fatalf("expected logStreamMsg, got %T", msg)
	}
	if lsm.Ch != ch {
		t.Fatal("expected reschedule on the message's own channel")
	}
	if lsm.Line.Line != "RIGHT" {
		t.Fatalf("expected line from msg.Ch, got %q", lsm.Line.Line)
	}
}

func TestRunNodeActionLogsTwiceCancelsFirst(t *testing.T) {
	f := newFakeEngine()
	lab := &engine.Lab{Name: "lab", Nodes: []engine.Node{{Name: "spine-01", Group: "spine"}}}
	m := New(f, []*engine.Lab{lab})
	m.curTopoLab = "lab"
	m.devTree.SetTopology(lab)

	if cmd := m.runNodeAction("logs"); cmd == nil {
		t.Fatal("expected log stream command")
	}
	if len(f.logCtxs) != 1 {
		t.Fatalf("expected 1 log stream, got %d", len(f.logCtxs))
	}
	if err := f.logCtxs[0].Err(); err != nil {
		t.Fatalf("expected first ctx not yet cancelled, got %v", err)
	}

	m.runNodeAction("logs")
	if len(f.logCtxs) != 2 {
		t.Fatalf("expected 2 log streams, got %d", len(f.logCtxs))
	}
	if err := f.logCtxs[0].Err(); err == nil {
		t.Fatal("expected first log context cancelled on second start")
	}
	if err := f.logCtxs[1].Err(); err != nil {
		t.Fatalf("expected second ctx live, got %v", err)
	}
}

func TestActionMenuIncludesPauseUnpause(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x.clab.yml"}}
	m.curTopoLab = "lab1"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if !m.actionMenu.Visible() {
		t.Fatal("expected action menu visible after a")
	}

	var ids []string
	prev := ""
	for i := 0; i < 30; i++ {
		sel := m.actionMenu.Selected()
		if sel == nil || sel.ID == prev {
			break
		}
		prev = sel.ID
		ids = append(ids, sel.ID)
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	has := func(id string) bool {
		for _, x := range ids {
			if x == id {
				return true
			}
		}
		return false
	}
	if !has("pause") {
		t.Fatalf("expected Pause in action menu, got %v", ids)
	}
	if !has("unpause") {
		t.Fatalf("expected Unpause in action menu, got %v", ids)
	}
}

func TestRenderHelpBarOmitsQuitOnSessions(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"})

	s := renderHelpBar(m, false)
	if strings.Contains(s, "quit") {
		t.Fatalf("expected quit hint hidden on sessions tab, got %q", s)
	}
	if !strings.Contains(s, "to normal") {
		t.Fatalf("expected mode-switch hint on sessions tab, got %q", s)
	}

	// No sessions → global keys (incl. quit) apply.
	m2 := New(nil, nil)
	m2.activeTab = tabSessions
	s2 := renderHelpBar(m2, false)
	if !strings.Contains(s2, "quit") {
		t.Fatalf("expected quit hint when no sessions, got %q", s2)
	}
}

func TestRenderHelpBarSessionsInsertModeShowsOnlyModeSwitch(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"})
	// insert mode (default)
	s := renderHelpBar(m, false)
	if strings.Contains(s, "picker") {
		t.Fatalf("expected no picker hint in insert mode, got %q", s)
	}
	if !strings.Contains(s, "to normal") {
		t.Fatalf("expected mode-switch hint in insert mode, got %q", s)
	}
	// normal mode shows the space menu hint and the mode switch back to insert
	m.sessionModel.SetMode(tabs.SessionNormal)
	s2 := renderHelpBar(m, false)
	if !strings.Contains(s2, "menu") {
		t.Fatalf("expected space menu hint in normal mode, got %q", s2)
	}
	if !strings.Contains(s2, "to insert") {
		t.Fatalf("expected mode-switch hint in normal mode, got %q", s2)
	}
}

func TestRenderHelpBarKeepsQuitAndShowsTraceControls(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.tracing = true
	s := renderHelpBar(m, false)
	if !strings.Contains(s, "quit") {
		t.Fatalf("expected quit hint visible while tracing, got %q", s)
	}
	if !strings.Contains(s, "trace stop") {
		t.Fatalf("expected trace stop hint while tracing, got %q", s)
	}
	if !strings.Contains(s, "change filter") {
		t.Fatalf("expected change filter hint while tracing, got %q", s)
	}
}

func TestRenderHelpBarShowsCaptureKeysWhilePaneVisible(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.capturePane.Show()
	s := renderHelpBar(m, false)
	if strings.Contains(s, "quit") {
		t.Fatalf("expected quit hint hidden while capture pane visible, got %q", s)
	}
	if !strings.Contains(s, "close") || !strings.Contains(s, "clear") {
		t.Fatalf("expected q=close / c=clear hints while capture pane visible, got %q", s)
	}
}

func TestRenderHelpBarSlimmedToSpaceAndQuit(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	s := renderHelpBar(m, false)
	if !strings.Contains(s, "menu") {
		t.Fatalf("expected space menu hint, got %q", s)
	}
	if !strings.Contains(s, "quit") {
		t.Fatalf("expected quit hint, got %q", s)
	}
	// Per-tab shortcuts are no longer listed (they live in the space menu).
	if strings.Contains(s, "trace start") || strings.Contains(s, "lab actions") {
		t.Fatalf("expected per-tab shortcuts removed from help bar, got %q", s)
	}
}

func TestRenderHelpBarHighlightsKeys(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := New(nil, nil)
	s := renderHelpBar(m, false)
	if !strings.Contains(s, "\x1b[1;38;5;196m") {
		t.Fatalf("expected red-highlighted keys in help bar, got %q", s)
	}
}

func TestSessionsTabLiveRoutesAllKeysToSession(t *testing.T) {
	stdin := &sessionTestStdin{Buffer: &bytes.Buffer{}}
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", Stdin: stdin})

	// 'd' must reach the shell, not the deploy hotkey.
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = result.(*Model)
	if cmd != nil {
		t.Fatalf("expected no command from char key on live session, got %+v", cmd)
	}
	if got := stdin.String(); got != "d" {
		t.Fatalf("expected 'd' written to session stdin, got %q", got)
	}
	if m.activeTab != tabSessions {
		t.Fatalf("expected to stay on sessions tab, got %d", m.activeTab)
	}
}

func TestSessionsTabLiveTabStaysInShell(t *testing.T) {
	stdin := &sessionTestStdin{Buffer: &bytes.Buffer{}}
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", Stdin: stdin})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(*Model)
	if cmd != nil {
		t.Fatalf("expected no tab-switch command, got %+v", cmd)
	}
	if m.activeTab != tabSessions {
		t.Fatalf("expected to stay on sessions tab in insert mode, got %d", m.activeTab)
	}
	if got := stdin.String(); got != "\t" {
		t.Fatalf("expected tab encoded to session stdin, got %q", got)
	}
}

func TestSessionsTabLiveShiftTabStaysInShell(t *testing.T) {
	stdin := &sessionTestStdin{Buffer: &bytes.Buffer{}}
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", Stdin: stdin})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(*Model)
	if cmd != nil {
		t.Fatalf("expected no command from shift+tab on live session, got %+v", cmd)
	}
	if m.activeTab != tabSessions {
		t.Fatalf("expected to stay on sessions tab in insert mode, got %d", m.activeTab)
	}
	if got := stdin.String(); got != "\x1b[Z" {
		t.Fatalf("expected shift+tab encoded to session stdin, got %q", got)
	}
}

func TestSessionsTabLiveRightArrowStaysInShell(t *testing.T) {
	stdin := &sessionTestStdin{Buffer: &bytes.Buffer{}}
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", Stdin: stdin})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = result.(*Model)
	if cmd != nil {
		t.Fatalf("expected no command from right arrow on live session, got %+v", cmd)
	}
	if m.activeTab != tabSessions {
		t.Fatalf("expected to stay on sessions tab, got %d", m.activeTab)
	}
	if got := stdin.String(); got != "\x1b[C" {
		t.Fatalf("expected right arrow encoded to session stdin, got %q", got)
	}
}

func TestSessionsTabLiveLeftArrowStaysInShell(t *testing.T) {
	stdin := &sessionTestStdin{Buffer: &bytes.Buffer{}}
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", Stdin: stdin})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = result.(*Model)
	if cmd != nil {
		t.Fatalf("expected no command from left arrow on live session, got %+v", cmd)
	}
	if m.activeTab != tabSessions {
		t.Fatalf("expected to stay on sessions tab, got %d", m.activeTab)
	}
	if got := stdin.String(); got != "\x1b[D" {
		t.Fatalf("expected left arrow encoded to session stdin, got %q", got)
	}
}

func TestSessionInsertModeRoutesAllKeysToShell(t *testing.T) {
	stdin := &sessionTestStdin{Buffer: &bytes.Buffer{}}
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1", Stdin: stdin})

	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("q")},
		{Type: tea.KeyTab},
		{Type: tea.KeyUp},
	} {
		m.Update(k)
	}
	if !m.sessionModel.HasSessions() {
		t.Fatal("q should NOT close the session in insert mode")
	}
	got := stdin.String()
	if !bytes.Contains([]byte(got), []byte("q")) || !bytes.Contains([]byte(got), []byte("\x1b[A")) {
		t.Fatalf("expected q and up-arrow encoded to stdin, got %q", got)
	}
}

func TestSessionNormalModeQCloses(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1", Close: nil})
	m.sessionModel.SetMode(tabs.SessionNormal)

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m.sessionModel.HasSessions() {
		t.Fatal("expected session closed by q in normal mode")
	}
}

func TestSessionNormalModeEnterFocuses(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"})
	m.sessionModel.SetMode(tabs.SessionNormal)

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.sessionModel.Mode() != tabs.SessionInsert {
		t.Fatal("expected insert mode after Enter in normal mode")
	}
}

func TestSessionNormalModeSpaceOpensWhichKey(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"})
	m.sessionModel.SetMode(tabs.SessionNormal)

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !m.whichKey.Visible() {
		t.Fatal("expected which-key menu open after space in sessions normal mode")
	}
	if m.sessionModel.Mode() != tabs.SessionNormal {
		t.Fatal("expected to stay in normal mode (space opens menu, does not enter insert)")
	}
}

func TestSessionInsertModeCtrlCNoneGoesToShell(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	stdin := &sessionTestStdin{Buffer: &bytes.Buffer{}}
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1", Stdin: stdin})

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if got := stdin.String(); got != "\x03" {
		t.Fatalf("expected ctrl+c to shell in insert mode, got %q", got)
	}
}

func TestSessionNormalModeTabSwitchesTab(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"})
	m.sessionModel.SetMode(tabs.SessionNormal)

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab == tabSessions {
		t.Fatal("expected tab to switch away from sessions in normal mode")
	}
}

func TestSessionsTabEmptyKeepsGlobalKeys(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions

	// 'd' on the empty sessions tab must not panic.
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = result.(*Model)
	if cmd != nil {
		t.Fatalf("expected no command for d on empty sessions, got %+v", cmd)
	}
	// Tab still switches tabs away from the empty sessions tab.
	m.activeTab = tabSessions
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(*Model)
	if m.activeTab != tabLogs {
		t.Fatalf("expected tab to advance to Logs from empty sessions, got %d", m.activeTab)
	}
}

func TestSearchCommitMovesCursor(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/lab.clab.yml"
	os.WriteFile(path, []byte(`name: lab
topology:
  nodes:
    spine-01:
      kind: k
      group: spine
    leaf-01:
      kind: k
      group: leaf
    host-05:
      kind: k
      group: host
`), 0644)

	m := New(nil, []*engine.Lab{{Name: "lab", TopoFile: path}})
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
			{Name: "host-05", Group: "host"},
		},
	})

	// Open search, type term, commit.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	// Type 'host'
	for _, r := range "host" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(string(r))})
	}
	// Enter commits
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(*Model)

	if m.devTree.Selected().Name != "host-05" {
		t.Fatalf("expected cursor on host-05 after search, got %s", m.devTree.Selected().Name)
	}
}

// TestSearchCommitViaKeyStream simulates real key stream: /, type, Enter.
func TestSearchCommitViaKeyStream(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
			{Name: "host-05", Group: "host"},
		},
	})
	// Simulate real Enter KeyMsg (Type=KeyEnter, String="enter").
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("host")})
	// Check searchInput received the text.
	if m.searchInput.Value() != "host" {
		t.Fatalf("expected searchInput value host, got %q", m.searchInput.Value())
	}
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(*Model)
	if m.devTree.Selected() == nil || m.devTree.Selected().Name != "host-05" {
		t.Fatalf("expected host-05, got %+v", m.devTree.Selected())
	}
}

// TestSearchCommitFullLoop simulates the complete tea loop: Enter returns a
// command that emits SearchCommitMsg, which must be fed back to Update.
func TestSearchCommitFullLoop(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine"},
			{Name: "leaf-01", Group: "leaf"},
			{Name: "host-05", Group: "host"},
		},
	})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("spine")})

	// Enter returns a command.
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected command from Enter in search input")
	}
	// Execute the command: it produces SearchCommitMsg.
	msg := cmd()
	if msg == nil {
		t.Fatal("expected SearchCommitMsg from command")
	}
	// Feed it back to Update (like the tea loop does).
	result, _ = m.Update(msg)
	m = result.(*Model)

	if m.devTree.Selected() == nil || m.devTree.Selected().Name != "spine-01" {
		t.Fatalf("expected spine-01 after full loop, got %+v", m.devTree.Selected())
	}
}

// TestSearchCommitRefreshesDetailPane: a search landing on a different node
// must rewire the detail pane to the new selection.
func TestSearchCommitRefreshesDetailPane(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.detailPane.SetSize(30, 20)
	m.devTree.SetTopology(&engine.Lab{
		Nodes: []engine.Node{
			{Name: "spine-01", Group: "spine", Container: "clab-lab-spine-01"},
			{Name: "leaf-01", Group: "leaf", Container: "clab-lab-leaf-01"},
			{Name: "host-05", Group: "host", Container: "clab-lab-host-05"},
		},
	})
	// Roles sort alphabetically (host first), so the initial selection is
	// host-05. Wire the detail pane to it like a real topology load.
	m.refreshDetail(m.detailNodeName())
	if got := m.renderActiveTab(); !strings.Contains(got, "clab-lab-host-05") {
		t.Fatalf("expected detail pane on initial selection before search:\n%s", got)
	}

	result, _ := m.Update(tabs.SearchCommitMsg{Term: "spine"})
	m = result.(*Model)
	if m.devTree.Selected() == nil || m.devTree.Selected().Name != "spine-01" {
		t.Fatalf("expected cursor on spine-01 after search, got %+v", m.devTree.Selected())
	}
	// Container names only render in the detail pane (never the tree), so
	// their presence proves the pane re-rendered for the new selection.
	got := m.renderActiveTab()
	if !strings.Contains(got, "clab-lab-spine-01") {
		t.Fatalf("expected detail pane refreshed to spine-01 after search:\n%s", got)
	}
	if strings.Contains(got, "clab-lab-host-05") {
		t.Fatalf("expected stale initial detail gone after search:\n%s", got)
	}
}

func TestNoMatchSearchShowsFooterStatus(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{
		Nodes: []engine.Node{{Name: "spine-01", Group: "spine"}},
	})
	// Simulate SearchCommitMsg with no match.
	result, _ := m.Update(tabs.SearchCommitMsg{Term: "zzz"})
	m = result.(*Model)
	// The no-match state renders in the topology footer only, not as a toast.
	if m.toast.Visible() {
		t.Fatal("no-match should not show a toast")
	}
	if got := m.topologyNotice(); !strings.Contains(got, "no match") {
		t.Fatalf("expected no-match status in topology footer, got %q", got)
	}
}

func TestToastShownAboveStatusBar(t *testing.T) {
	m := New(nil, nil, WithCapabilities(capability.Capabilities{ClabSUID: true}))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.toast.Show("No match for \"zzz\"")
	v := m.View()
	// The toast text must appear in the frame, and NOT inside the tree content.
	if !strings.Contains(v, "No match") {
		t.Fatalf("expected toast text in view:\n%s", v)
	}
}

func TestSubscribeEventsSurvivesListLabsError(t *testing.T) {
	f := newFakeEngine()
	f.listErr = errors.New("boom")
	cmd := subscribeEvents(f)
	msg := cmd()
	csrm, ok := msg.(changeStreamReadyMsg)
	if !ok {
		t.Fatalf("expected changeStreamReadyMsg even on ListLabs error, got %T", msg)
	}
	if csrm.err == nil {
		t.Fatal("expected err to be carried on changeStreamReadyMsg")
	}
	if csrm.ch == nil {
		t.Fatal("expected subscription channel despite ListLabs error")
	}

	// Feeding the message back must still establish the subscription and
	// schedule the next change pull.
	m := New(nil, nil)
	result, cmd := m.Update(msg)
	model := result.(*Model)
	if model.changeCh != csrm.ch {
		t.Fatal("expected changeCh to be set despite ListLabs error")
	}
	if model.err == nil {
		t.Fatal("expected m.err to be set from carried error")
	}
	if cmd == nil {
		t.Fatal("expected nextChange command from changeStreamReadyMsg")
	}
}

func TestErrorShownAboveStatusBar(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.err = errors.New("something broke")
	v := m.View()
	if !strings.Contains(v, "something broke") {
		t.Fatalf("expected error text in view:\n%s", v)
	}
	if got := len(strings.Split(v, "\n")); got > 24 {
		t.Fatalf("view height %d exceeds terminal height 24", got)
	}
}

// fakeEngine is a minimal engine.Engine implementation for tests. Unimplemented
// methods panic through the embedded interface.
type fakeEngine struct {
	engine.Engine  // embed interface for unimplemented methods
	labs           []*engine.Lab
	lab            *engine.Lab
	subCh          chan engine.Change
	cancel         func()
	listErr        error
	startCalled    bool
	startLab       string
	startNode      string
	deployCalled   bool
	deployPath     string
	destroyCalled  bool
	destroyName    string
	redeployCalled bool
	redeployName   string
	logCtxs        []context.Context
}

func newFakeEngine() *fakeEngine {
	ch := make(chan engine.Change, 256)
	return &fakeEngine{subCh: ch, cancel: func() {}}
}

func (f *fakeEngine) ListLabs(ctx context.Context) ([]*engine.Lab, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.labs, nil
}
func (f *fakeEngine) GetLab(ctx context.Context, name string) (*engine.Lab, error) {
	if f.lab != nil && f.lab.Name == name {
		return f.lab, nil
	}
	return nil, engine.ErrLabNotFound
}
func (f *fakeEngine) Subscribe() (<-chan engine.Change, func()) {
	return f.subCh, f.cancel
}

func (f *fakeEngine) Deploy(ctx context.Context, labPath string, opts ...engine.DeployOption) (<-chan engine.OutputLine, error) {
	f.deployCalled = true
	f.deployPath = labPath
	ch := make(chan engine.OutputLine, 1)
	ch <- engine.OutputLine{Done: true}
	return ch, nil
}

func (f *fakeEngine) Destroy(ctx context.Context, labName string, opts ...engine.DestroyOption) (<-chan engine.OutputLine, error) {
	f.destroyCalled = true
	f.destroyName = labName
	ch := make(chan engine.OutputLine, 1)
	ch <- engine.OutputLine{Done: true}
	return ch, nil
}

func (f *fakeEngine) Redeploy(ctx context.Context, labName string, opts ...engine.RedeployOption) (<-chan engine.OutputLine, error) {
	f.redeployCalled = true
	f.redeployName = labName
	ch := make(chan engine.OutputLine, 1)
	ch <- engine.OutputLine{Done: true}
	return ch, nil
}

func (f *fakeEngine) StartNode(ctx context.Context, labName, nodeName string) (<-chan engine.OutputLine, error) {
	f.startCalled = true
	f.startLab = labName
	f.startNode = nodeName
	ch := make(chan engine.OutputLine, 1)
	ch <- engine.OutputLine{Done: true}
	return ch, nil
}

func (f *fakeEngine) StreamNodeLogs(ctx context.Context, labName, nodeName string) (<-chan engine.OutputLine, error) {
	f.logCtxs = append(f.logCtxs, ctx)
	ch := make(chan engine.OutputLine, 1)
	ch <- engine.OutputLine{Done: true}
	return ch, nil
}

// refreshFakeEngine records LiveRefresher calls so tests can assert that a
// completed operation re-reads live state.
type refreshFakeEngine struct {
	*fakeEngine
	refreshed int
}

func (f *refreshFakeEngine) RefreshLive(context.Context) error {
	f.refreshed++
	return nil
}

func TestOpCompletionRefreshesLiveState(t *testing.T) {
	fe := &refreshFakeEngine{fakeEngine: newFakeEngine()}
	m := New(fe, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	_, cmd := m.Update(opLineMsg{Line: engine.OutputLine{Done: true}})
	if cmd == nil {
		t.Fatal("expected a command after operation completion")
	}
	msg := cmd()
	if fe.refreshed != 1 {
		t.Fatalf("RefreshLive called %d times, want 1", fe.refreshed)
	}
	if _, ok := msg.(operationRefreshedMsg); !ok {
		t.Fatalf("expected operationRefreshedMsg, got %T", msg)
	}
}

func TestDigitSwitchesTab(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	mm := result.(*Model)
	if mm.activeTab != tabTopology {
		t.Fatalf("expected tabTopology after '1', got %d", mm.activeTab)
	}
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	mm = result.(*Model)
	if mm.activeTab != tabSessions {
		t.Fatalf("expected tabSessions after '2', got %d", mm.activeTab)
	}
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	mm = result.(*Model)
	if mm.activeTab != tabLogs {
		t.Fatalf("expected tabLogs after '3', got %d", mm.activeTab)
	}
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	mm = result.(*Model)
	if mm.activeTab != tabOps {
		t.Fatalf("expected tabOps after '4', got %d", mm.activeTab)
	}
	mm.activeTab = tabTopology
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	mm = result.(*Model)
	if mm.activeTab != tabTopology {
		t.Fatalf("expected no change on '6', got %d", mm.activeTab)
	}
}

func TestCtrlBackslashSingleEntersNormal(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlBackslash})
	if m.sessionModel.Mode() != tabs.SessionNormal {
		t.Fatal("expected normal mode after single ctrl+\\")
	}
}

func TestSessionPickerShownOnS(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"})
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s2", Title: "srv2", NodeName: "srv2"})
	m.sessionModel.SetMode(tabs.SessionNormal)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !m.sessionPicker.Visible() {
		t.Fatal("expected session picker visible after s in normal mode")
	}
}

func TestSessionPickedMsgFocusesSession(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"})
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s2", Title: "srv2", NodeName: "srv2"})
	m.Update(tabs.SessionPickedMsg{Session: &engine.SessionHandle{ID: "s2", Title: "srv2", NodeName: "srv2"}})
	if got := m.sessionModel.Active().Handle.NodeName; got != "srv2" {
		t.Fatalf("expected active srv2 after SessionPickedMsg, got %s", got)
	}
}

func TestSessionPickerHiddenInInsertMode(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	stdin := &sessionTestStdin{Buffer: &bytes.Buffer{}}
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1", Stdin: stdin})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.sessionPicker.Visible() {
		t.Fatal("expected no picker in insert mode")
	}
	if got := stdin.String(); got != "s" {
		t.Fatalf("expected s written to shell in insert mode, got %q", got)
	}
}

func TestRenderTabBarShowsDigits(t *testing.T) {
	bar := renderTabBar(tabTopology)
	for _, d := range []string{"1", "2", "3", "4"} {
		if !strings.Contains(bar, d) {
			t.Fatalf("expected digit %s in tab bar, got %q", d, bar)
		}
	}
}

func TestRenderTabBarActiveHighlight(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	bar := renderTabBar(tabTopology)
	// The red digit's trailing reset must not strip the active style from the
	// name: " Topology" must stay highlighted, not fall back to default.
	if strings.Contains(bar, "\x1b[0m Topology") {
		t.Fatalf("active tab name lost its highlight to the digit's reset: %q", bar)
	}
	// The active tab is highlighted via foreground color + bold only; it must
	// not carry a background fill.
	if !strings.Contains(bar, "38;5;212") {
		t.Fatalf("expected active tab foreground highlight, got %q", bar)
	}
	if strings.Contains(bar, "48;5;236") {
		t.Fatalf("active tab must not carry a background highlight, got %q", bar)
	}
	if inactive := renderTabBar(tabLogs); strings.Contains(inactive, "38;5;212m Topology") {
		t.Fatalf("inactive Topology tab must not carry the active foreground highlight, got %q", inactive)
	}
}

func TestDigitBoundariesIgnored(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabLogs
	for _, r := range []rune{'0', '6', '9'} {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if m.activeTab != tabLogs {
			t.Fatalf("digit %c must not switch tabs, got %d", r, m.activeTab)
		}
	}
}

func TestSessionNormalModeDigitSwitchesTab(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"})
	m.sessionModel.SetMode(tabs.SessionNormal)

	// '2' → Sessions tab
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if m.activeTab != tabSessions {
		t.Fatalf("expected tabSessions after '2' in normal mode, got %d", m.activeTab)
	}
	// '3' → Logs tab
	m.activeTab = tabSessions
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if m.activeTab != tabLogs {
		t.Fatalf("expected tabLogs after '3' in normal mode, got %d", m.activeTab)
	}
}

func TestSessionInsertModeDigitGoesToShell(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	stdin := &sessionTestStdin{Buffer: &bytes.Buffer{}}
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1", Stdin: stdin})
	// insert mode (default): '2' is shell input
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if m.activeTab != tabSessions {
		t.Fatalf("expected no tab switch in insert mode, got %d", m.activeTab)
	}
	if got := stdin.String(); got != "2" {
		t.Fatalf("expected '2' written to shell in insert mode, got %q", got)
	}
}

func TestSessionPickedMsgEntersInsertMode(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"})
	m.sessionModel.SetMode(tabs.SessionNormal)

	m.Update(tabs.SessionPickedMsg{Session: &engine.SessionHandle{ID: "s1", Title: "r1", NodeName: "r1"}})
	if m.sessionModel.Mode() != tabs.SessionInsert {
		t.Fatal("expected insert mode after SessionPickedMsg")
	}
}

func TestSessionTabHeightMatchesOtherTabs(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30}) // content area = 25

	// Topology tab height (baseline)
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{
		Nodes: []engine.Node{{Name: "r1", Group: "core"}},
	})
	topoLines := len(strings.Split(m.renderActiveTab(), "\n"))

	// Sessions with one session
	m.activeTab = tabSessions
	m.sessionModel.AddSession(&engine.SessionHandle{ID: "s1", Title: "clab-e2e-r1", NodeName: "r1"})
	m.sessionModel.Feed("s1", []byte("hello\r\n"))
	sessLines := len(strings.Split(m.renderActiveTab(), "\n"))

	if sessLines != topoLines {
		t.Fatalf("sessions tab height %d != topology height %d", sessLines, topoLines)
	}
}

func TestTabEnumHasNoLabList(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabSessions
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	mm := result.(*Model)
	if mm.activeTab != tabTopology {
		t.Fatalf("expected tabTopology after '1', got %d", mm.activeTab)
	}
}

func TestTabNamesHasFourEntries(t *testing.T) {
	if len(TabNames) != 4 {
		t.Fatalf("expected 4 tab names, got %d: %v", len(TabNames), TabNames)
	}
	if TabNames[0] != "Topology" {
		t.Fatalf("expected Topology first, got %q", TabNames[0])
	}
}

func TestStartupShowsLabPicker(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	labs := []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	// Simulate the first labsLoadedMsg — must open the picker.
	result, _ := m.Update(labsLoadedMsg{labs: labs})
	mm := result.(*Model)
	if !mm.labPicker.Visible() {
		t.Fatal("expected lab picker visible on first labs load")
	}
	// Dismiss the picker the way a user would (selecting or closing it).
	m.labPicker.Hide()
	// A subsequent labsLoadedMsg must NOT re-open it.
	result, _ = m.Update(labsLoadedMsg{labs: labs})
	mm = result.(*Model)
	if mm.labPicker.Visible() {
		t.Fatal("expected picker NOT re-shown on later labs load")
	}
}

func TestLabActionContainsEditYAML(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	m.showLabMenu()
	items := m.actionMenu.Items()
	found := false
	for _, it := range items {
		if it.ID == "edit" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Edit YAML in lab action menu, got %+v", items)
	}
}

func TestNodeActionHasNoEditYAML(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "r1"}}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	for _, it := range m.actionMenu.Items() {
		if it.ID == "edit" {
			t.Fatal("expected no Edit YAML in node action menu")
		}
	}
}

func TestLabActionEditYAMLCmdNonNil(t *testing.T) {
	m := New(nil, nil)
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	cmd := m.editCurrentLabYAML()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from editCurrentLabYAML")
	}
}

func TestTopologyLayoutHasDetailPane(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "n1"}}})
	v := m.renderActiveTab()
	// The detail pane's placeholder only renders from the pane itself, never
	// from the tree — its presence proves the split-pane layout is active.
	if !strings.Contains(v, "Select a node") {
		t.Fatalf("expected detail pane content in topology view:\n%s", v)
	}
}

func TestTopologyLayoutHasLeftAndRightBorders(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "n1"}}})
	v := m.renderActiveTab()
	if strings.Count(v, "╭") < 2 || strings.Count(v, "╰") < 2 {
		t.Fatalf("expected borders around both topology panes:\n%s", v)
	}
}

func TestViewOmitsLabsStatusBar(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if strings.Contains(m.View(), "labs:") {
		t.Fatalf("expected labs status bar to be removed:\n%s", m.View())
	}
}

func TestTopologySearchFooterKeepsPaneBordersAligned(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "leaf-01"}}})
	withoutSearch := strings.Split(m.renderActiveTab(), "\n")
	m.devTree.Search("leaf")
	withSearch := strings.Split(m.renderActiveTab(), "\n")
	if len(withSearch) != len(withoutSearch) {
		t.Fatalf("search changed topology height from %d to %d", len(withoutSearch), len(withSearch))
	}
	if strings.LastIndex(withSearch[len(withSearch)-1], "╰") < 0 {
		t.Fatalf("expected aligned bottom borders after search:\n%s", strings.Join(withSearch, "\n"))
	}
}

func TestKeyTTogglesTrace(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	// nil engine → capability check returns unavailable → toast, no panic
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected toast command from trace toggle in limited mode")
	}
	if !m.toast.Visible() {
		t.Fatal("expected toast visible in limited mode")
	}
}

func TestNodeMenuDoesNotHaveCaptureItem(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "r1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	for _, it := range m.actionMenu.Items() {
		if it.ID == "capture" {
			t.Fatal("Capture should be an interface action")
		}
	}
}

func TestInterfaceMenuHasCaptureItem(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "r1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	selection, ok := m.devTree.SelectedResource()
	if !ok || selection.Kind != tabs.ResourceInterface || selection.Interface != "eth0" {
		t.Fatalf("expected eth0 interface selection, got %+v", selection)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	for _, it := range m.actionMenu.Items() {
		if it.ID == "interface-capture" {
			return
		}
	}
	t.Fatal("expected Capture in interface action menu")
}

func TestModelSearchNMovesMixedNodeInterfaceResults(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "eth1", Interfaces: []engine.Interface{{Name: "eth1"}}}}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("eth1")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected search commit command")
	}
	result, _ := m.Update(cmd())
	m = result.(*Model)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = result.(*Model)
	selection, ok := m.devTree.SelectedResource()
	if !ok || selection.Kind != tabs.ResourceInterface || selection.Interface != "eth1" {
		t.Fatalf("expected n to select interface result, got %+v", selection)
	}
}

func TestKeyITogglesInterfaces(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "n1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})
	if !strings.Contains(m.renderActiveTab(), "eth0") {
		t.Fatalf("expected eth0 visible by default")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if strings.Contains(m.renderActiveTab(), "eth0") {
		t.Fatalf("expected eth0 hidden after i")
	}
}

type traceEngine struct {
	engine.Engine // embed interface for unimplemented methods
}

func (e *traceEngine) PacketTraceCapable(ctx context.Context) engine.CapabilityResult {
	return engine.CapabilityResult{Available: true}
}

func (e *traceEngine) Capture(ctx context.Context, labName, nodeName, iface string, opts ...engine.CaptureOption) (<-chan engine.PacketEvent, func() error, error) {
	ch := make(chan engine.PacketEvent, 1)
	ch <- engine.PacketEvent{Node: nodeName, Iface: iface, Pkt: engine.ParsedPacket{Proto: "tcp", Summary: "pkt"}}
	close(ch)
	return ch, func() error { return nil }, nil
}

func (e *traceEngine) TracePath(ctx context.Context, labName, filter string) (<-chan engine.PacketEvent, func() error, error) {
	ch := make(chan engine.PacketEvent, 1)
	ch <- engine.PacketEvent{Node: "r1", Iface: "eth0", Pkt: engine.ParsedPacket{Proto: "tcp", Summary: "pkt"}}
	close(ch)
	return ch, func() error { return nil }, nil
}

type partialCaptureEngine struct{ traceEngine }

func (e *partialCaptureEngine) Capture(ctx context.Context, labName, nodeName, iface string, opts ...engine.CaptureOption) (<-chan engine.PacketEvent, func() error, error) {
	ch := make(chan engine.PacketEvent, 1)
	ch <- engine.PacketEvent{Node: nodeName, Iface: iface, Pkt: engine.ParsedPacket{Direction: "OUT", Proto: "icmp", Summary: "packet"}}
	close(ch)
	warning := &engine.CaptureWarningError{Issues: []engine.CaptureIssue{{
		Node: nodeName, Interface: iface, Direction: "ingress", Err: errors.New("operation not permitted"),
	}}}
	return ch, func() error { return nil }, warning
}

func TestCaptureFlowAppendsToPane(t *testing.T) {
	m := New(&traceEngine{}, nil, WithCapabilities(capability.Capabilities{Root: true}))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "r1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})

	// a → capture → iface:eth0 → filter: → startCapture
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	result, _ := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "capture"}})
	m = result.(*Model)
	if !m.actionMenu.Visible() {
		t.Fatal("expected iface menu visible after capture")
	}
	result, _ = m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "iface:eth0"}})
	m = result.(*Model)
	if !m.actionMenu.Visible() {
		t.Fatal("expected filter menu visible after iface pick")
	}
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "filter:"}})
	m = result.(*Model)
	if !m.capturePane.Visible() {
		t.Fatal("expected capture pane visible after start")
	}
	if cmd == nil {
		t.Fatal("expected pullCaptureEvent command after start")
	}
	// Simulate the runtime delivering the buffered packet event.
	result, _ = m.Update(cmd())
	m = result.(*Model)
	if !strings.Contains(m.View(), "r1:eth0") {
		t.Fatalf("expected captured packet in view:\n%s", m.View())
	}
}

func TestCaptureFlowKeepsPartialSessionAndShowsAttachWarning(t *testing.T) {
	m := New(&partialCaptureEngine{}, nil, WithCapabilities(capability.Capabilities{Root: true}))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "r1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	result, _ := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "capture"}})
	m = result.(*Model)
	result, _ = m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "iface:eth0"}})
	m = result.(*Model)
	result, _ = m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "filter:"}})
	m = result.(*Model)
	if !m.capturePane.Visible() {
		t.Fatal("expected partial capture pane to remain visible")
	}
	if !m.toast.Visible() || !strings.Contains(m.toast.Message(), "r1:eth0 ingress attach failed: operation not permitted") {
		t.Fatalf("expected direction-specific attach warning, got %q", m.toast.Message())
	}
}

func TestTraceFlowIncrementsPathHits(t *testing.T) {
	m := New(&traceEngine{}, nil, WithCapabilities(capability.Capabilities{Root: true}))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "r1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})

	// t → filter menu → filter: → startTrace
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if !m.actionMenu.Visible() {
		t.Fatal("expected filter menu visible after t")
	}
	result, _ := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "filter:icmp"}})
	m = result.(*Model)
	if !m.tracing {
		t.Fatal("expected tracing started")
	}
	// t again stops tracing
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if m.tracing {
		t.Fatal("expected tracing stopped after second t")
	}
}

func TestTraceFilterRememberedAfterAll(t *testing.T) {
	m := New(&traceEngine{}, nil, WithCapabilities(capability.Capabilities{Root: true}))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "r1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})

	// t → filter menu → choose "All" (filter:)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if !m.actionMenu.Visible() {
		t.Fatal("expected filter menu visible after t")
	}
	result, _ := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "filter:"}})
	m = result.(*Model)
	if !m.tracing {
		t.Fatal("expected tracing started with All filter")
	}

	// t stops, then t again must start without re-prompting.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if m.tracing {
		t.Fatal("expected tracing stopped after t")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if m.actionMenu.Visible() {
		t.Fatal("expected no filter menu on re-trace (All remembered)")
	}
	if !m.tracing {
		t.Fatal("expected tracing restarted on re-trace")
	}
}

func TestCaptureModeDoesNotLeakIntoTrace(t *testing.T) {
	m := New(&traceEngine{}, nil, WithCapabilities(capability.Capabilities{Root: true}))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "r1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})

	// a → capture → iface:eth0 → filter: → startCapture
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	result, _ := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "capture"}})
	m = result.(*Model)
	result, _ = m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "iface:eth0"}})
	m = result.(*Model)
	result, _ = m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "filter:"}})
	m = result.(*Model)
	if !m.capturePane.Visible() {
		t.Fatal("expected capture started")
	}
	if m.captureIface == "" {
		t.Fatal("expected captureIface set by capture flow")
	}

	// t → filter menu → filter:icmp must route to startTrace, not startCapture.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if !m.actionMenu.Visible() {
		t.Fatal("expected filter menu visible after t")
	}
	result, _ = m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "filter:icmp"}})
	m = result.(*Model)
	if !m.tracing {
		t.Fatal("expected tracing started, not capture")
	}
}

// captureCloseEngine records whether the capture stop func was invoked.
type captureCloseEngine struct {
	traceEngine
	stopCalled bool
}

func (e *captureCloseEngine) Capture(ctx context.Context, labName, nodeName, iface string, opts ...engine.CaptureOption) (<-chan engine.PacketEvent, func() error, error) {
	ch := make(chan engine.PacketEvent, 1)
	ch <- engine.PacketEvent{Node: nodeName, Iface: iface, Pkt: engine.ParsedPacket{Proto: "tcp", Summary: "pkt"}}
	close(ch)
	return ch, func() error { e.stopCalled = true; return nil }, nil
}

// startCaptureForTest drives the UI through a → Capture → iface → filter to
// reach a running capture, mirroring TestCaptureFlowAppendsToPane.
func startCaptureForTest(m *Model) (tea.Cmd, *Model) {
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	result, _ := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "capture"}})
	m = result.(*Model)
	result, _ = m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "iface:eth0"}})
	m = result.(*Model)
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "filter:"}})
	return cmd, result.(*Model)
}

func TestCapturePaneQStopsAndHides(t *testing.T) {
	eng := &captureCloseEngine{}
	m := New(eng, nil, WithCapabilities(capability.Capabilities{Root: true}))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "r1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})

	_, m = startCaptureForTest(m)
	if !m.capturePane.Visible() || m.captureStop == nil {
		t.Fatal("expected capture running with pane visible")
	}

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = result.(*Model)
	if cmd != nil {
		t.Fatal("expected no quit command on capture q")
	}
	if m.capturePane.Visible() {
		t.Fatal("expected pane hidden after q")
	}
	if m.captureStop != nil || m.captureCancel != nil {
		t.Fatal("expected capture state cleared after q")
	}
	if !eng.stopCalled {
		t.Fatal("expected engine capture stop invoked on q")
	}
}

func TestCapturePaneCClears(t *testing.T) {
	m := New(&traceEngine{}, nil, WithCapabilities(capability.Capabilities{Root: true}))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "r1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})

	cmd, m := startCaptureForTest(m)
	result, _ := m.Update(cmd())
	m = result.(*Model)
	if !strings.Contains(m.capturePane.View(), "r1:eth0") {
		t.Fatalf("expected packet in pane:\n%s", m.capturePane.View())
	}
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = result.(*Model)
	if strings.Contains(m.capturePane.View(), "r1:eth0") {
		t.Fatalf("expected pane cleared after c:\n%s", m.capturePane.View())
	}
}

// TestQClosesCaptureWithoutStoppingTrace verifies that q closes only the
// capture pane; the trace remains active and can be stopped with t on topology.
func TestQClosesCaptureWithoutStoppingTrace(t *testing.T) {
	m := New(&traceEngine{}, nil, WithCapabilities(capability.Capabilities{Root: true}))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	m.devTree.SetTopology(&engine.Lab{Name: "lab1", Nodes: []engine.Node{{Name: "r1", Interfaces: []engine.Interface{{Name: "eth0", State: "up"}}}}})

	// Start a trace.
	m.tracing = true
	m.traceSeq = 1
	m.traceStop = func() error { return nil }
	m.devTree.IncrementPathHit("r1", "eth0", "IN")
	if !strings.Contains(m.devTree.View(), "IN 1") {
		t.Fatalf("expected path hits visible before close:\n%s", m.devTree.View())
	}

	// Open capture on top of the running trace.
	_, m = startCaptureForTest(m)
	if !m.capturePane.Visible() {
		t.Fatal("expected capture pane visible")
	}

	// q closes the capture pane but leaves the trace active.
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = result.(*Model)
	if m.capturePane.Visible() {
		t.Fatal("expected capture pane hidden after q")
	}
	if !m.tracing {
		t.Fatal("expected trace to remain active after q closed capture pane")
	}
	if !strings.Contains(m.devTree.View(), "IN 1") {
		t.Fatalf("expected path hits preserved after q:\n%s", m.devTree.View())
	}
}

func TestStaleTraceStoppedDoesNotKillNewTrace(t *testing.T) {
	m := New(&traceEngine{}, nil)
	m.traceSeq = 5 // a newer trace is already active
	m.tracing = true
	// stale stop from an older trace (seq 3) must be ignored.
	result, _ := m.Update(traceStoppedMsg{seq: 3})
	m = result.(*Model)
	if !m.tracing {
		t.Fatal("stale traceStoppedMsg killed the active trace")
	}
}

func TestErrMsgTimesOut(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	_, cmd := m.Update(errMsg{err: errors.New("boom")})
	if cmd == nil {
		t.Fatal("expected timer cmd from errMsg")
	}
	// 执行 cmd 得到 errExpiredMsg，更新后 m.err 清空
	msg := cmd()
	result, _ := m.Update(msg)
	mm := result.(*Model)
	if mm.err != nil {
		t.Fatalf("expected err cleared after expiry, got %v", mm.err)
	}
}

func TestQQuitsWhileTracing(t *testing.T) {
	// q keeps its global quit behavior while tracing.
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.tracing = true
	m.traceStop = func() error { return nil }
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	mm := result.(*Model)
	if !mm.tracing {
		t.Fatal("expected trace to remain active after q")
	}
	if cmd == nil {
		t.Fatal("expected quit command from q")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
}

// TestQQuitsFromAnyTabWhileTracing verifies q remains the global quit key.
func TestQQuitsFromAnyTabWhileTracing(t *testing.T) {
	for _, tabID := range []tab{tabLogs, tabOps} {
		m := New(nil, nil)
		m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		m.activeTab = tabID
		m.tracing = true
		m.traceStop = func() error { return nil }
		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		mm := result.(*Model)
		if !mm.tracing {
			t.Fatalf("expected trace to remain active after q on tab %d", tabID)
		}
		if cmd == nil {
			t.Fatalf("expected quit command on q while tracing on tab %d", tabID)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("expected tea.QuitMsg on tab %d, got %T", tabID, cmd())
		}
	}
}

func TestFChangesTraceFilterFromTopology(t *testing.T) {
	m := New(&traceEngine{}, nil)
	m.activeTab = tabTopology
	m.tracing = true
	m.traceStop = func() error { return nil }
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = result.(*Model)
	if cmd != nil {
		t.Fatal("expected filter picker without a command")
	}
	if m.tracing {
		t.Fatal("expected current trace to stop before changing filter")
	}
	if !m.actionMenu.Visible() {
		t.Fatal("expected trace filter picker to open")
	}
}

// TestTraceStoppedMsgClearsPathHits verifies a natural trace-stream close
// clears path hits, matching q-stop behavior.
func TestTraceStoppedMsgClearsPathHits(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "r1", Interfaces: []engine.Interface{{Name: "eth0"}}}}})
	m.tracing = true
	m.traceSeq = 1
	m.devTree.IncrementPathHit("r1", "eth0", "IN")
	if !strings.Contains(m.devTree.View(), "IN 1") {
		t.Fatalf("expected path hits before stop:\n%s", m.devTree.View())
	}
	m.Update(traceStoppedMsg{seq: 1})
	if m.tracing {
		t.Fatal("expected tracing false after traceStoppedMsg")
	}
	if strings.Contains(m.devTree.View(), "IN 1") {
		t.Fatal("expected path hits cleared after traceStoppedMsg")
	}
}

func TestErrSeqGuardIgnoresStaleExpiry(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	_, cmd1 := m.Update(errMsg{err: errors.New("first")})
	_, cmd2 := m.Update(errMsg{err: errors.New("second")})
	// A stale expiry (cmd1) must NOT clear the newer error.
	result, _ := m.Update(cmd1())
	mm := result.(*Model)
	if mm.err == nil {
		t.Fatal("stale expiry cleared the newer error")
	}
	// The current error's expiry clears it.
	result, _ = m.Update(cmd2())
	mm = result.(*Model)
	if mm.err != nil {
		t.Fatalf("expected current error cleared, got %v", mm.err)
	}
}

func TestTabOrderAndNames(t *testing.T) {
	if len(TabNames) != 4 {
		t.Fatalf("expected 4 tabs, got %d", len(TabNames))
	}
	want := []string{"Topology", "Sessions", "Node Logs", "Ops Log"}
	for i, w := range want {
		if TabNames[i] != w {
			t.Fatalf("TabNames[%d]=%q, want %q", i, TabNames[i], w)
		}
	}
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	mm := result.(*Model)
	if mm.activeTab != tabSessions {
		t.Fatalf("expected tabSessions after '2', got %d", mm.activeTab)
	}
}

func TestSpaceOpensWhichKeyOnTopology(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !m.whichKey.Visible() {
		t.Fatal("expected which-key menu open after space on topology")
	}
}

func TestWhichKeyMenuDeployRoutesThroughActionSelected(t *testing.T) {
	lab := &engine.Lab{Name: "lab1", TopoFile: "/l1.clab.yml"}
	f := newFakeEngine()
	m := New(f, []*engine.Lab{lab}, WithCapabilities(capability.Capabilities{ClabSUID: true}))
	m.activeTab = tabTopology
	m.curTopoLab = "lab1"
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// space -> root menu; 'l' descends into Lab; 'd' triggers Deploy.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !m.whichKey.Visible() {
		t.Fatal("expected which-key menu open after space")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if m.whichKey.Depth() != 2 {
		t.Fatalf("expected depth 2 inside Lab group, got %d", m.whichKey.Depth())
	}
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = result.(*Model)
	if cmd == nil {
		t.Fatal("expected command from Deploy trigger")
	}
	if f.deployCalled {
		t.Fatal("expected no engine call before ActionSelectedMsg round-trip")
	}
	msg := cmd()
	am, ok := msg.(tabs.ActionSelectedMsg)
	if !ok || am.Item.ID != "deploy" {
		t.Fatalf("expected ActionSelectedMsg{deploy}, got %T %+v", msg, msg)
	}
	result, _ = m.Update(am)
	m = result.(*Model)
	if !f.deployCalled {
		t.Fatal("expected Deploy called after ActionSelectedMsg round-trip")
	}
	if m.activeTab != tabOps {
		t.Fatalf("expected tabOps after deploy, got %d", m.activeTab)
	}
}

func TestWhichKeyModalConsumesKeys(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "n1", Group: "core"}}})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	// 'j' must move the which-key cursor, never reach the devTree or other keys.
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = result.(*Model)
	if !m.whichKey.Visible() {
		t.Fatal("expected which-key still visible after j")
	}
	if cmd != nil {
		t.Fatalf("expected no command while which-key open, got %+v", cmd)
	}
}

func TestSpaceNoOpWhileCaptureVisible(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.capturePane.Show()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if m.whichKey.Visible() {
		t.Fatal("expected space to be ignored while capture pane is visible")
	}
}

func TestWhichKeyViewRendered(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	v := m.View()
	if !strings.Contains(v, "SPC") {
		t.Fatalf("expected which-key body in view:\n%s", v)
	}
}

func TestWhichKeyNotClippedByTracingStrip(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabLogs
	m.tracing = true
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !m.whichKey.Visible() {
		t.Fatal("expected which-key menu open on logs tab")
	}
	v := m.View()
	if !strings.Contains(v, "[Esc] back") {
		t.Fatalf("expected which-key footer visible despite tracing strip:\n%s", v)
	}
	if !strings.Contains(v, "╰") || !strings.Contains(v, "╯") {
		t.Fatalf("expected which-key bottom border visible despite tracing strip:\n%s", v)
	}
}

func TestSpaceDoesNotToggleNodeAnymore(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.devTree.SetTopology(&engine.Lab{Nodes: []engine.Node{{Name: "spine-01", Group: "spine"}}})
	// Space now opens the menu instead of toggling the selected node.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !m.whichKey.Visible() {
		t.Fatal("expected which-key menu open after space on a selected node")
	}
}

func TestWhichKeyTraceAction(t *testing.T) {
	f := newFakeEngine()
	m := New(f, nil, WithCapabilities(capability.Capabilities{Root: true}))
	m.activeTab = tabTopology
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "trace"}})
	m = result.(*Model)
	// First trace with no filter set opens the filter menu and returns no cmd.
	if cmd != nil {
		t.Fatalf("expected no command from first trace (filter menu opens), got %+v", cmd)
	}
	if !m.actionMenu.Visible() {
		t.Fatal("expected trace filter menu visible on first trace")
	}
}

func TestWhichKeySearchActionOpensSearchInput(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "search"}})
	m = result.(*Model)
	if !m.searchInput.Visible() {
		t.Fatal("expected search input visible after search action")
	}
	if cmd != nil {
		t.Fatalf("expected no command, got %+v", cmd)
	}
}

func TestWhichKeyLogsFollowAction(t *testing.T) {
	m := New(nil, nil)
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "logs-follow"}})
	m = result.(*Model)
	if cmd != nil {
		t.Fatalf("expected no command from logs-follow, got %+v", cmd)
	}
	// Follow defaults ON; after toggle it is OFF.
	if m.logView.Following() {
		t.Fatal("expected follow toggled off after logs-follow action")
	}
}

func TestWhichKeyOpsFollowAction(t *testing.T) {
	m := New(nil, nil)
	result, cmd := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "ops-follow"}})
	m = result.(*Model)
	if cmd != nil {
		t.Fatalf("expected no command from ops-follow, got %+v", cmd)
	}
	if m.opsView.Following() {
		t.Fatal("expected auto-follow toggled off after ops-follow action")
	}
}

// fakeGraphLauncher is a stub graphLauncher for model tests.
type fakeGraphLauncher struct {
	topo    string
	url     string
	err     error
	stopped bool
}

func (f *fakeGraphLauncher) start(topoPath string) (string, error) {
	f.topo = topoPath
	return f.url, f.err
}

func (f *fakeGraphLauncher) stop() { f.stopped = true }

func TestLabMenuHasViewGraph(t *testing.T) {
	m := New(nil, nil)
	m.showLabMenu()
	found := false
	for _, it := range m.actionMenu.Items() {
		if it.ID == "graph" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected View Graph in lab action menu, got %+v", m.actionMenu.Items())
	}
}

func TestWhichKeyLabGroupHasGraph(t *testing.T) {
	m := New(nil, nil)
	m.activeTab = tabTopology
	root := m.buildWhichKeyRoot()
	var labGroup tabs.MenuNode
	for _, n := range root {
		if n.Label == "Lab" {
			labGroup = n
		}
	}
	found := false
	for _, c := range labGroup.Children {
		if c.ActionID == "graph" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected graph action in which-key Lab group, got %+v", labGroup.Children)
	}
}

func TestOpenGraphViewNoLab(t *testing.T) {
	m := New(nil, nil)
	cmd := m.openGraphView()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from openGraphView with no lab")
	}
	if got := m.toast.Message(); got != "no lab selected" {
		t.Fatalf("toast message = %q, want %q", got, "no lab selected")
	}
}

func TestOpenGraphViewSuccessRoutes(t *testing.T) {
	m := New(nil, nil)
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	fake := &fakeGraphLauncher{url: "http://localhost:50080"}
	m.graph = fake

	cmd := m.openGraphView()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	gm, ok := msg.(graphMsg)
	if !ok {
		t.Fatalf("expected graphMsg, got %T", msg)
	}
	if gm.url != "http://localhost:50080" || gm.err != nil {
		t.Fatalf("graphMsg = %+v", gm)
	}
	if fake.topo != "/x/lab1.clab.yml" {
		t.Fatalf("launcher topo = %q, want /x/lab1.clab.yml", fake.topo)
	}

	result, cmd2 := m.Update(tabs.ActionSelectedMsg{Item: tabs.ActionItem{ID: "graph"}})
	if cmd2 == nil {
		t.Fatalf("expected cmd from ActionSelectedMsg(graph), got %v", cmd2)
	}
	_ = result
}

func TestQuitStopsGraphServer(t *testing.T) {
	m := New(nil, nil)
	fake := &fakeGraphLauncher{}
	m.graph = fake
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !fake.stopped {
		t.Fatal("expected graph server stopped on quit")
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg from quit cmd, got %T", cmd())
	}
	_ = result
}

func TestGraphMsgHandlerShowsURL(t *testing.T) {
	prev := openBrowserFn
	openBrowserFn = func(string) error { return nil }
	defer func() { openBrowserFn = prev }()

	m := New(nil, nil)
	m.Update(graphMsg{url: "http://localhost:50080"})
	if got := m.toast.Message(); !strings.Contains(got, "localhost:50080") {
		t.Fatalf("expected toast containing URL, got %q", got)
	}
}

func TestNewWithInitialLabSetsField(t *testing.T) {
	lab := &engine.Lab{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}
	m := New(nil, []*engine.Lab{lab}, WithInitialLab(lab))
	if m.initialLab == nil || m.initialLab.Name != "lab1" {
		t.Fatalf("initialLab = %+v, want lab1", m.initialLab)
	}
}

func TestLabsLoadedWithInitialLabLoadsDirectly(t *testing.T) {
	lab := &engine.Lab{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}
	m := New(nil, []*engine.Lab{lab}, WithInitialLab(lab))
	result, cmd := m.Update(labsLoadedMsg{labs: []*engine.Lab{lab}})
	mm := result.(*Model)
	if mm.curTopoLab != "lab1" {
		t.Fatalf("curTopoLab = %q, want lab1", mm.curTopoLab)
	}
	if mm.activeTab != tabTopology {
		t.Fatalf("activeTab = %d, want tabTopology", mm.activeTab)
	}
	if mm.initialLab != nil {
		t.Fatal("initialLab must be cleared after first load")
	}
	if !mm.startupPickerShown {
		t.Fatal("startupPickerShown must be set so the picker is not shown later")
	}
	if cmd == nil {
		t.Fatal("expected a loadTopology cmd")
	}
}

func TestLabsLoadedWithoutInitialLabShowsPicker(t *testing.T) {
	lab := &engine.Lab{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}
	m := New(nil, []*engine.Lab{lab})
	result, _ := m.Update(labsLoadedMsg{labs: []*engine.Lab{lab}})
	mm := result.(*Model)
	if mm.curTopoLab != "" {
		t.Fatalf("curTopoLab = %q, want empty", mm.curTopoLab)
	}
	if !mm.labPicker.Visible() {
		t.Fatal("expected lab picker to show without initial lab")
	}
}

func TestInitialLabClearedAfterFirstLabsLoaded(t *testing.T) {
	lab := &engine.Lab{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}
	m := New(nil, []*engine.Lab{lab}, WithInitialLab(lab))
	m.Update(labsLoadedMsg{labs: []*engine.Lab{lab}})
	m.Update(labsLoadedMsg{labs: []*engine.Lab{lab, {Name: "lab2", TopoFile: "/x/lab2.clab.yml"}}})
	if m.curTopoLab != "lab1" {
		t.Fatalf("curTopoLab = %q after second refresh, want lab1", m.curTopoLab)
	}
}

func TestOpsTabFrameKeepsTabBar(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.activeTab = tabOps
	m.opsView.SetLabel("Deploying: lab")
	m.opsView.AppendLine(engine.OutputLine{Line: "INFO some deploy output"})
	lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
	if len(lines) > 24 {
		t.Fatalf("frame has %d lines > 24; the top rows (tab bar) scroll off-screen", len(lines))
	}
	if !strings.Contains(lines[0], "Topology") {
		t.Fatalf("tab bar missing from first frame row: %q", lines[0])
	}
}

func TestLogsTabFrameKeepsTabBar(t *testing.T) {
	m := New(nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.activeTab = tabLogs
	m.logView.SetLabel("r1")
	m.logView.AppendLine("INFO some log line")
	lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
	if len(lines) > 24 {
		t.Fatalf("frame has %d lines > 24; the top rows (tab bar) scroll off-screen", len(lines))
	}
	if !strings.Contains(lines[0], "Topology") {
		t.Fatalf("tab bar missing from first frame row: %q", lines[0])
	}
}

func TestDeployBlockedWithoutClabPrivilege(t *testing.T) {
	m := New(nil, nil, WithCapabilities(capability.Capabilities{}))
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	cmd := m.deployCurrentLab()
	if cmd == nil {
		t.Fatal("expected a toast cmd")
	}
	if got := m.toast.Message(); got == "" || !strings.Contains(got, "containerlab") {
		t.Fatalf("expected English toast mentioning containerlab, got %q", got)
	}
	if m.activeTab == tabOps {
		t.Fatal("deploy must not switch to ops tab when blocked")
	}
}

func TestDeployAllowedWithClabSUID(t *testing.T) {
	m := New(nil, nil, WithCapabilities(capability.Capabilities{ClabSUID: true}))
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	cmd := m.deployCurrentLab()
	// With a nil engine the deploy path should return a toast "engine unavailable (limited mode)".
	// The key assertion: it must NOT be the capability-denial toast.
	if strings.Contains(m.toast.Message(), "containerlab needs root") {
		t.Fatalf("SUID should pass the capability check, got %q", m.toast.Message())
	}
	_ = cmd
}

func TestTraceBlockedWithoutTraceCaps(t *testing.T) {
	m := New(nil, nil, WithCapabilities(capability.Capabilities{}))
	m.curTopoLab = "lab1"
	cmd := m.toggleTrace()
	if cmd == nil {
		t.Fatal("expected toast cmd")
	}
	if got := m.toast.Message(); !strings.Contains(got, "tracing") {
		t.Fatalf("expected English tracing toast, got %q", got)
	}
}

func TestCapDenialToastVisibleInLimitedMode(t *testing.T) {
	m := New(nil, nil, WithCapabilities(capability.Capabilities{}))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.activeTab = tabTopology
	m.labs = []*engine.Lab{{Name: "lab1", TopoFile: "/x/lab1.clab.yml"}}
	m.curTopoLab = "lab1"
	cmd := m.deployCurrentLab()
	if cmd == nil {
		t.Fatal("expected a toast cmd")
	}
	// The capability-denial toast must render on the right side of the status bar
	// (truncated to fit), taking priority over the limited-mode hint.
	frame := tmuxANSI.ReplaceAllString(m.View(), "")
	if !strings.Contains(frame, "containerlab needs root") {
		t.Fatalf("denial toast missing from status bar:\n%s", frame)
	}
	// Once the toast expires, the limited-mode hint shows on the right instead.
	m.toast.Hide()
	frame = tmuxANSI.ReplaceAllString(m.View(), "")
	if !strings.Contains(frame, "limited mode") {
		t.Fatalf("limited-mode hint missing from status bar:\n%s", frame)
	}
	if strings.Contains(frame, "limited mode: run sudo") {
		t.Fatalf("long limited-mode notice should not render:\n%s", frame)
	}
}

func TestStartTraceBlockedWithoutTraceCaps(t *testing.T) {
	m := New(nil, nil, WithCapabilities(capability.Capabilities{}))
	m.curTopoLab = "lab1"
	cmd := m.startTrace("icmp")
	if cmd == nil {
		t.Fatal("expected toast cmd")
	}
	if got := m.toast.Message(); !strings.Contains(got, "packet tracing needs root") {
		t.Fatalf("expected capability-denial toast, got %q", got)
	}
}
