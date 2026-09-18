package tabs

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tianyuxue/clab-tui/internal/engine"
)

// roleColors maps role names to ANSI colors. Default (unassigned) is dim.
var roleColors = map[string]lipgloss.Color{
	"spine":  lipgloss.Color("36"), // cyan
	"leaf":   lipgloss.Color("34"), // blue
	"border": lipgloss.Color("33"), // yellow
	"fw":     lipgloss.Color("31"), // red
	"server": lipgloss.Color("32"), // green
	"host":   lipgloss.Color("32"), // green
	"core":   lipgloss.Color("35"), // magenta
}

const unassignedRole = "(no role)"

// isMgmtInterface reports whether an interface is the management interface
// (which carries the node's Node.IPv4 from inspect).
func isMgmtInterface(name string) bool {
	return strings.Contains(name, "mgmt") || name == "eth0"
}

// devEntry is one node in the tree with its neighbor summary lines.
type devEntry struct {
	node  engine.Node
	conns []connLine
}

// connLine is a single connection line: └ eth1 ──── leaf-01:eth1 UP.
type connLine struct {
	portA, portB string
	neighbor     string
	state        engine.LinkState
}

type ResourceKind string

const (
	ResourceNode      ResourceKind = "node"
	ResourceInterface ResourceKind = "interface"
	ResourceLink      ResourceKind = "link"
)

type ResourceSelection struct {
	Kind      ResourceKind
	Node      string
	Interface string
	Neighbor  string
	Port      string
}

type treeRow struct {
	selection ResourceSelection
}

type pathHit struct {
	in  int
	out int
}

func (h pathHit) total() int { return h.in + h.out }

// DevTree renders a role-grouped device tree with inline neighbor summaries.
type DevTree struct {
	groups map[string][]*devEntry
	// flat order of entries for cursor navigation (across groups).
	order          []*devEntry
	cursor         int
	rowCursor      int
	width          int
	height         int
	vp             viewport.Model
	entries        map[string]*devEntry
	expandedAll    bool
	expandedSet    map[string]bool
	searchTerm     string
	noMatch        bool
	showInterfaces bool
	// interfaceIPs maps node name → interface name → IPv4, populated
	// asynchronously by the model via SetInterfaceIPs.
	interfaceIPs map[string]map[string]string
	// pathHits 记录链路追踪命中的接口包计数（"node\0iface" → 计数）。
	pathHits     map[string]pathHit
	pathBlink    bool
	footerNotice string
}

// Search locates the first visible resource row matching term. Node-name
// matches take priority, followed by interface and link rows. The content is
// not filtered or hidden; the cursor moves to the match like vim's "/".
func (d *DevTree) Search(term string) {
	d.searchTerm = term
	d.noMatch = false
	if term == "" {
		return
	}
	rows := d.visibleRows()
	before := d.rowCursor
	for pass := 0; pass < 2; pass++ {
		for i, row := range rows {
			if pass == 0 && (row.selection.Kind != ResourceNode || !strings.Contains(strings.ToLower(row.selection.Node), strings.ToLower(term))) {
				continue
			}
			if pass == 1 && !d.rowMatches(row) {
				continue
			}
			d.selectRow(i)
			return
		}
	}
	d.noMatch = true
	d.rowCursor = before
}

// NoMatch reports whether the last search term found no match.
func (d *DevTree) NoMatch() bool { return d.noMatch }

// ClearSearch clears the active search state.
func (d *DevTree) ClearSearch() {
	d.searchTerm = ""
	d.noMatch = false
}

// SearchStatus returns the committed vim-style search footer state.
func (d *DevTree) SearchStatus() string {
	if d.searchTerm == "" || len(d.order) == 0 {
		return ""
	}
	rows := d.searchRows()
	position := 0
	for i, row := range rows {
		if row == d.rowCursor {
			position = i + 1
		}
	}
	matches := len(rows)
	if matches == 0 {
		return fmt.Sprintf("/%s  no matches", d.searchTerm)
	}
	if position == 0 {
		position = 1
	}
	return fmt.Sprintf("/%s  %d matches  [%d/%d]  n/N next/previous  Ctrl-f/Ctrl-b page", d.searchTerm, matches, position, matches)
}

// FindNext moves to the next match after the current cursor (wraps around).
func (d *DevTree) FindNext() {
	rows := d.searchRows()
	if len(rows) == 0 {
		return
	}
	position := -1
	for i, row := range rows {
		if row == d.rowCursor {
			position = i
			break
		}
	}
	d.selectRow(rows[(position+1+len(rows))%len(rows)])
}

// FindPrev moves to the previous match before the current cursor (wraps).
// Node-name matches take priority over neighbor-name matches.
func (d *DevTree) FindPrev() {
	rows := d.searchRows()
	if len(rows) == 0 {
		return
	}
	position := 0
	for i, row := range rows {
		if row == d.rowCursor {
			position = i
			break
		}
	}
	d.selectRow(rows[(position-1+len(rows))%len(rows)])
}

// moveToMatch scans forward from start (wrapping) for the first match.
// Node-name matches take priority over neighbor-name matches.
func (d *DevTree) moveToMatch(start int) {
	if len(d.order) == 0 {
		return
	}
	n := len(d.order)
	// First pass: prefer entries whose own name matches.
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		if d.nameMatches(d.order[idx]) {
			d.setNodeCursor(idx)

			return
		}
	}
	// Second pass: fall back to neighbor-name matches.
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		if d.neighborMatches(d.order[idx]) {
			d.setNodeCursor(idx)

			return
		}
	}
}

// nameMatches reports whether the entry's own node name contains the term.
func (d *DevTree) nameMatches(e *devEntry) bool {
	if d.searchTerm == "" {
		return false
	}
	term := strings.ToLower(d.searchTerm)
	if strings.Contains(strings.ToLower(e.node.Name), term) {
		return true
	}
	for _, iface := range e.node.Interfaces {
		if strings.Contains(strings.ToLower(iface.Name), term) {
			return true
		}
	}
	return false
}

// neighborMatches reports whether any neighbor name contains the term.
func (d *DevTree) neighborMatches(e *devEntry) bool {
	if d.searchTerm == "" {
		return false
	}
	term := strings.ToLower(d.searchTerm)
	for _, c := range e.conns {
		if strings.Contains(strings.ToLower(c.neighbor), term) ||
			strings.Contains(strings.ToLower(c.portA), term) ||
			strings.Contains(strings.ToLower(c.portB), term) {
			return true
		}
	}
	return false
}

// entryMatches reports whether the entry matches by name or by neighbor.
func (d *DevTree) entryMatches(e *devEntry) bool {
	return d.nameMatches(e) || d.neighborMatches(e)
}

// scrollToCursor keeps the selected entry visible in the viewport. It is
// called from View after content is rebuilt; cursorLine is the rendered line
// of the selected entry (or -1 if not found).
func (d *DevTree) scrollToCursor(cursorLine int) {
	if cursorLine < 0 || d.vp.Height <= 0 {
		return
	}
	// Content height must be known; SetContent was just called so
	// vp.TotalLineCount() is current.
	total := d.vp.TotalLineCount()
	if total <= d.vp.Height {
		d.vp.SetYOffset(0)
		return
	}
	// If cursor is outside the visible window, scroll it into the middle.
	off := d.vp.YOffset
	if cursorLine < off {
		d.vp.SetYOffset(cursorLine)
	} else if cursorLine >= off+d.vp.Height {
		newOff := cursorLine - d.vp.Height + 1
		if newOff < 0 {
			newOff = 0
		}
		d.vp.SetYOffset(newOff)
	}
}

func NewDevTree() *DevTree {
	return &DevTree{
		groups:         map[string][]*devEntry{},
		entries:        map[string]*devEntry{},
		vp:             viewport.New(60, 20),
		expandedAll:    true,
		expandedSet:    map[string]bool{},
		showInterfaces: true,
		pathHits:       map[string]pathHit{},
	}
}

// ToggleInterfaces flips whether interface (netcard) lines render. The flag
// applies globally across all nodes.
func (d *DevTree) ToggleInterfaces() {
	d.showInterfaces = !d.showInterfaces
}

// ToggleAll expands/collapses the connection summaries of every node.
func (d *DevTree) ToggleAll() {
	d.expandedAll = !d.expandedAll
	if d.expandedAll {
		// Entering expand-all: reset per-node manual state.
		d.expandedSet = map[string]bool{}
	}
}

// ToggleSelected expands/collapses the connection summary of the node under
// the cursor only.
func (d *DevTree) ToggleSelected() {
	if len(d.order) == 0 {
		return
	}
	// Leaving "expand all" mode: snapshot every node as expanded so only the
	// selected node toggles off while the rest stay expanded.
	if d.expandedAll {
		d.expandedAll = false
		for _, e := range d.order {
			d.expandedSet[e.node.Name] = true
		}
	}
	name := d.order[d.cursor].node.Name
	d.expandedSet[name] = !d.expandedSet[name]
}

// nodeExpanded reports whether a node's connections should be shown.
func (d *DevTree) nodeExpanded(name string) bool {
	return d.expandedAll || d.expandedSet[name]
}

func (d *DevTree) SetSize(w, h int) {
	d.width = w
	d.height = h
	d.vp.Width = w
	d.vp.Height = h
}

func (d *DevTree) SetTopology(lab *engine.Lab) {
	d.groups = map[string][]*devEntry{}
	d.entries = map[string]*devEntry{}
	d.order = nil
	d.interfaceIPs = map[string]map[string]string{}

	// Preserve expand/collapse state for nodes that still exist.
	prevExpanded := d.expandedSet
	d.expandedSet = map[string]bool{}
	for _, n := range lab.Nodes {
		if prevExpanded[n.Name] {
			d.expandedSet[n.Name] = true
		}
	}
	// pathHits survive topology reloads so live trace counters aren't wiped;
	// use ClearPathHits for explicit cleanup.

	// Create entries and assign to role groups.
	for _, n := range lab.Nodes {
		e := &devEntry{node: n}
		role := n.Group
		if role == "" {
			role = unassignedRole
		}
		d.groups[role] = append(d.groups[role], e)
		d.entries[n.Name] = e
	}

	// Populate connection lines.
	for _, link := range lab.Links {
		src, okA := d.entries[link.A]
		dst, okB := d.entries[link.B]
		if !okA || !okB {
			continue
		}
		src.conns = append(src.conns, connLine{
			portA: link.PortA, portB: link.PortB,
			neighbor: link.B, state: link.State,
		})
		dst.conns = append(dst.conns, connLine{
			portA: link.PortB, portB: link.PortA,
			neighbor: link.A, state: link.State,
		})
	}

	// Deterministic group order (sorted roles).
	roles := make([]string, 0, len(d.groups))
	for r := range d.groups {
		roles = append(roles, r)
	}
	sort.Strings(roles)
	// Keep unassigned last.
	for i, r := range roles {
		if r == unassignedRole && i != len(roles)-1 {
			roles = append(append(roles[:i], roles[i+1:]...), r)
			break
		}
	}

	for _, r := range roles {
		sort.Slice(d.groups[r], func(i, j int) bool {
			return d.groups[r][i].node.Name < d.groups[r][j].node.Name
		})
		d.order = append(d.order, d.groups[r]...)
	}

	// Clamp cursor to the rebuilt order so a smaller lab can't index OOB.
	if d.cursor >= len(d.order) {
		if len(d.order) == 0 {
			d.cursor = 0
		} else {
			d.cursor = len(d.order) - 1
		}
	}
	rows := d.visibleRows()
	if d.rowCursor >= len(rows) {
		d.rowCursor = len(rows) - 1
		if d.rowCursor < 0 {
			d.rowCursor = 0
		}
	}
}

// SetInterfaceIPs sets (or replaces) the per-interface IPv4 map for one node.
// A nil ips clears that node's entry so stale IPs don't render.
func (d *DevTree) SetInterfaceIPs(node string, ips map[string]string) {
	if ips == nil {
		delete(d.interfaceIPs, node)
		return
	}
	d.interfaceIPs[node] = ips
}

// ClearInterfaceIPs resets all per-node interface IPs.
func (d *DevTree) ClearInterfaceIPs() {
	d.interfaceIPs = map[string]map[string]string{}
}

// IncrementPathHit records one trace hit and keeps ingress/egress counts
// separate for the interface and its connection rendering.
func (d *DevTree) IncrementPathHit(node, iface, direction string) {
	key := node + "\x00" + iface
	hit := d.pathHits[key]
	switch direction {
	case "IN", "ingress":
		hit.in++
	case "OUT", "egress":
		hit.out++
	}
	d.pathHits[key] = hit
}

// ClearPathHits 清空路径命中。
func (d *DevTree) ClearPathHits() {
	d.pathHits = map[string]pathHit{}
	d.pathBlink = false
}

// SetPathBlink toggles the trace highlight phase. The model flips this value
// periodically while tracing so the render changes even on terminals that do
// not animate ANSI SGR blink sequences.
func (d *DevTree) SetPathBlink(on bool) { d.pathBlink = on }

// SetFooterNotice renders a transient message in the bottom interior row of
// the tree pane. Keeping it inside the pane avoids global overlays erasing the
// topology border.
func (d *DevTree) SetFooterNotice(notice string) { d.footerNotice = notice }

func (d *DevTree) Selected() *engine.Node {
	if len(d.order) == 0 {
		return nil
	}
	if d.cursor >= len(d.order) {
		d.cursor = len(d.order) - 1
	}
	return &d.order[d.cursor].node
}

func (d *DevTree) visibleRows() []treeRow {
	var rows []treeRow
	for _, role := range d.orderedRoles() {
		for _, entry := range d.groups[role] {
			rows = append(rows, treeRow{selection: ResourceSelection{Kind: ResourceNode, Node: entry.node.Name}})
			if !d.nodeExpanded(entry.node.Name) {
				continue
			}
			if d.showInterfaces {
				for _, iface := range entry.node.Interfaces {
					if iface.Name != "" {
						rows = append(rows, treeRow{selection: ResourceSelection{Kind: ResourceInterface, Node: entry.node.Name, Interface: iface.Name}})
					}
				}
			}
			for _, conn := range entry.conns {
				rows = append(rows, treeRow{selection: ResourceSelection{Kind: ResourceLink, Node: entry.node.Name, Interface: conn.portA, Neighbor: conn.neighbor, Port: conn.portB}})
			}
		}
	}
	return rows
}

func (d *DevTree) rowMatches(row treeRow) bool {
	term := strings.ToLower(d.searchTerm)
	if term == "" {
		return false
	}
	var values []string
	switch row.selection.Kind {
	case ResourceNode:
		values = []string{row.selection.Node}
	case ResourceInterface:
		values = []string{row.selection.Interface}
	case ResourceLink:
		values = []string{row.selection.Interface, row.selection.Neighbor, row.selection.Port}
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), term) {
			return true
		}
	}
	return false
}

func (d *DevTree) searchRows() []int {
	if d.searchTerm == "" {
		return nil
	}
	rows := d.visibleRows()
	matches := make([]int, 0, len(rows))
	for i, row := range rows {
		if d.rowMatches(row) {
			matches = append(matches, i)
		}
	}
	return matches
}

func (d *DevTree) selectRow(index int) {
	rows := d.visibleRows()
	if index < 0 || index >= len(rows) {
		return
	}
	d.rowCursor = index
	for i, entry := range d.order {
		if entry.node.Name == rows[index].selection.Node {
			d.cursor = i
			return
		}
	}
}

func (d *DevTree) selectedRow() (treeRow, bool) {
	rows := d.visibleRows()
	if len(rows) == 0 {
		return treeRow{}, false
	}
	if d.rowCursor < 0 {
		d.rowCursor = 0
	}
	if d.rowCursor >= len(rows) {
		d.rowCursor = len(rows) - 1
	}
	return rows[d.rowCursor], true
}

// SelectedResource returns the currently highlighted node, interface, or link.
func (d *DevTree) SelectedResource() (ResourceSelection, bool) {
	row, ok := d.selectedRow()
	if !ok {
		return ResourceSelection{}, false
	}
	return row.selection, true
}

func (d *DevTree) setNodeCursor(index int) {
	d.cursor = index
	rows := d.visibleRows()
	for i, row := range rows {
		if row.selection.Kind == ResourceNode && row.selection.Node == d.order[index].node.Name {
			d.rowCursor = i
			return
		}
	}
}

// RoleNames returns the role group names in display order.
func (d *DevTree) RoleNames() []string {
	return d.orderedRoles()
}

// Cursor returns the current selection index.
func (d *DevTree) Cursor() int { return d.cursor }

func (d *DevTree) MoveDown() {
	rows := d.visibleRows()
	if d.rowCursor < len(rows)-1 {
		d.rowCursor++
	}
	if row, ok := d.selectedRow(); ok {
		for i, entry := range d.order {
			if entry.node.Name == row.selection.Node {
				d.cursor = i
				break
			}
		}
	}
}

func (d *DevTree) MoveUp() {
	if d.rowCursor > 0 {
		d.rowCursor--
	}
	if row, ok := d.selectedRow(); ok {
		for i, entry := range d.order {
			if entry.node.Name == row.selection.Node {
				d.cursor = i
				break
			}
		}
	}
}

func (d *DevTree) Update(msg tea.Msg) (*DevTree, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			d.MoveDown()
			d.vp.LineDown(1)
		case "k", "up":
			d.MoveUp()
			d.vp.LineUp(1)
		case "g":
			d.cursor = 0
			d.rowCursor = 0
			d.vp.GotoTop()
		case "G":
			d.cursor = len(d.order) - 1
			d.rowCursor = len(d.visibleRows()) - 1
			if d.rowCursor < 0 {
				d.rowCursor = 0
			}
			d.vp.GotoBottom()
		case "n":
			d.FindNext()
		case "N":
			d.FindPrev()
		case "ctrl+f":
			d.vp.PageDown()
		case "ctrl+b":
			d.vp.PageUp()
		}
	}
	var cmd tea.Cmd
	d.vp, cmd = d.vp.Update(msg)
	return d, cmd
}

func (d *DevTree) View() string {
	if len(d.order) == 0 {
		if d.width == 0 {
			d.width = 60
		}
		return d.applyFooter(padToWidthHeight("No devices in this lab.", d.width, d.height))
	}
	if d.width == 0 {
		d.width = 60
	}
	selected, _ := d.selectedRow()

	var b strings.Builder
	// cursorLine tracks the rendered line index of the selected entry.
	cursorLine := -1
	line := 0
	for _, role := range d.orderedRoles() {
		entries := d.groups[role]
		color := roleColors[role]
		if role == unassignedRole {
			color = lipgloss.Color("240")
		}
		header := lipgloss.NewStyle().
			Bold(true).
			Foreground(color).
			Render(fmt.Sprintf("▓▓ %s (%d)", strings.ToUpper(role), len(entries)))
		b.WriteString(header + "\n")
		line++

		for _, e := range entries {
			isSel := selected.selection.Kind == ResourceNode && selected.selection.Node == e.node.Name
			prefix := "  "
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
			if isSel {
				prefix = "▸ "
				style = style.Bold(true).Foreground(lipgloss.Color("212"))
			}
			// Status dot.
			statusDot := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("●") // running
			switch e.node.State {
			case engine.StatusStopped:
				statusDot = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("●") // red
			case engine.StatusPaused:
				statusDot = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◐") // yellow/half
			case engine.StatusUnknown:
				statusDot = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("○") // grey/empty
			}
			b.WriteString(prefix + statusDot + " " + style.Render(highlightSearch(e.node.Name, d.searchTerm)) + "\n")
			if isSel {
				cursorLine = line
			}
			line++
			if !d.nodeExpanded(e.node.Name) {
				continue
			}
			// Interface lines (tree-style, ┃ guide).
			for _, iface := range e.node.Interfaces {
				if !d.showInterfaces || iface.Name == "" {
					continue
				}
				ifState := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("UP")
				if iface.State == "down" {
					ifState = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("DOWN")
				} else if iface.State == "unknown" {
					ifState = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("UNKNOWN")
				}
				mac := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(iface.MAC)
				ip := ""
				if isMgmtInterface(iface.Name) && e.node.IPv4 != "" {
					// Management IP is available non-invasively from inspect.
					ip = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(e.node.IPv4)
				} else if m, ok := d.interfaceIPs[e.node.Name]; ok {
					// Business-port IPs are collected via docker exec (selected node only).
					if v, ok2 := m[iface.Name]; ok2 && v != "" {
						ip = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(v)
					}
				}
				ifaceSelected := selected.selection.Kind == ResourceInterface && selected.selection.Node == e.node.Name && selected.selection.Interface == iface.Name
				ifaceNameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
				ifacePrefix := "  ┃ "
				if ifaceSelected {
					ifacePrefix = "▸ ┃ "
					ifaceNameStyle = ifaceNameStyle.Bold(true).Foreground(lipgloss.Color("212"))
				}
				parts := []string{
					ifaceNameStyle.Render(highlightSearch(iface.Name, d.searchTerm)),
				}
				if iface.MAC != "" {
					parts = append(parts, mac)
				}
				if ip != "" {
					parts = append(parts, ip)
				}
				parts = append(parts, ifState)
				hitKey := e.node.Name + "\x00" + iface.Name
				if hit := d.pathHits[hitKey]; hit.total() > 0 {
					inText := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render(fmt.Sprintf("IN %d", hit.in))
					outText := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render(fmt.Sprintf("OUT %d", hit.out))
					parts = append(parts, "("+inText+" "+outText+")")
				}
				ifLine := ifacePrefix + strings.Join(parts, "  ")
				b.WriteString(ifLine + "\n")
				if ifaceSelected {
					cursorLine = line
				}
				line++
			}
			for _, c := range e.conns {
				linkSelected := selected.selection.Kind == ResourceLink && selected.selection.Node == e.node.Name && selected.selection.Interface == c.portA && selected.selection.Neighbor == c.neighbor
				lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("36"))
				var upStyle string
				switch c.state {
				case engine.LinkUp:
					upStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("UP")
				case engine.LinkDown:
					upStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("DOWN")
				default:
					upStyle = lineStyle.Render("UNKNOWN")
				}
				aHit := d.pathHits[e.node.Name+"\x00"+c.portA].total() > 0
				bHit := d.pathHits[c.neighbor+"\x00"+c.portB].total() > 0
				aStyle := lineStyle
				bStyle := lineStyle
				switch {
				case aHit && bHit:
					if d.pathBlink {
						lineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)
					}
					aStyle, bStyle = lineStyle, lineStyle
					label := string(c.state)
					if label == "" {
						label = "UNKNOWN"
					}
					upStyle = lineStyle.Render(strings.ToUpper(label))
				case aHit:
					if d.pathBlink {
						aStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)
					}
				case bHit:
					if d.pathBlink {
						bStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)
					}
				}
				connPrefix := "  └ "
				if linkSelected {
					connPrefix = "▸ └ "
				}
				conn := fmt.Sprintf("%s%s ──── %s:%s  %s",
					connPrefix,
					aStyle.Render(highlightSearch(c.portA, d.searchTerm)),
					bStyle.Render(highlightSearch(c.neighbor, d.searchTerm)),
					bStyle.Render(highlightSearch(c.portB, d.searchTerm)),
					upStyle,
				)
				b.WriteString(conn + "\n")
				if linkSelected {
					cursorLine = line
				}
				line++
			}
		}
	}

	d.vp.SetContent(b.String())
	d.scrollToCursor(cursorLine)
	return d.applyFooter(d.vp.View())
}

func highlightSearch(text, term string) string {
	if term == "" {
		return text
	}
	lowerText := strings.ToLower(text)
	lowerTerm := strings.ToLower(term)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("226")).Bold(true)
	var b strings.Builder
	for offset := 0; offset < len(text); {
		rel := strings.Index(lowerText[offset:], lowerTerm)
		if rel < 0 {
			b.WriteString(text[offset:])
			break
		}
		start := offset + rel
		b.WriteString(text[offset:start])
		end := start + len(term)
		b.WriteString(style.Render(text[start:end]))
		offset = end
	}
	return b.String()
}

func (d *DevTree) applyFooter(view string) string {
	if d.footerNotice == "" {
		return view
	}
	view = strings.TrimRight(view, "\n")
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		return view
	}
	height := d.height
	if height <= 0 {
		height = len(lines)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", d.width))
	}
	color := lipgloss.Color("255")
	notice := d.footerNotice
	if visualWidth(notice) > d.width {
		runes := []rune(notice)
		limit := d.width - 1
		if limit < 1 {
			limit = 1
		}
		if len(runes) > limit {
			notice = string(runes[:limit]) + "…"
		}
	}
	footer := lipgloss.NewStyle().Bold(true).Foreground(color).Render(notice)
	if visualWidth(footer) < d.width {
		footer += strings.Repeat(" ", d.width-visualWidth(footer))
	}
	lines[len(lines)-1] = footer
	return strings.Join(lines, "\n")
}

func (d *DevTree) orderedRoles() []string {
	roles := make([]string, 0, len(d.groups))
	for r := range d.groups {
		roles = append(roles, r)
	}
	sort.Strings(roles)
	for i, r := range roles {
		if r == unassignedRole && i != len(roles)-1 {
			roles = append(append(roles[:i], roles[i+1:]...), r)
			break
		}
	}
	return roles
}
