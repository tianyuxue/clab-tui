package tabs

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

// DetailPane shows information about the currently selected node: identity,
// uptime, image, and live resource usage.
type DetailPane struct {
	node        *engine.Node
	monitor     *engine.NodeMonitor
	stats       map[string]*engine.InterfaceStats
	statsLoaded bool
	statsError  bool
	width       int
	height      int
}

func NewDetailPane() *DetailPane {
	return &DetailPane{}
}

func (p *DetailPane) SetNode(n *engine.Node)           { p.node = n }
func (p *DetailPane) SetMonitor(m *engine.NodeMonitor) { p.monitor = m }
func (p *DetailPane) SetInterfaceStats(s map[string]*engine.InterfaceStats) {
	p.stats = s
	p.statsLoaded = true
	p.statsError = false
}
func (p *DetailPane) SetInterfaceStatsUnavailable() {
	p.stats = nil
	p.statsLoaded = true
	p.statsError = true
}
func (p *DetailPane) SetSize(w, h int) { p.width = w; p.height = h }

// View renders the detail pane. Returns a padded placeholder when no node is
// selected.
func (p *DetailPane) View() string {
	if p.node == nil {
		return padToWidthHeight("Select a node to view details.", p.width, p.height)
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render(p.node.Name) + "\n\n")
	b.WriteString("  Container: " + p.node.Container + "\n")
	b.WriteString("  Image:     " + p.node.Image + "\n")
	b.WriteString("  State:     " + stateText(p.node.State) + "\n")
	if !p.node.StartedAt.IsZero() {
		b.WriteString("  Uptime:    " + formatUptime(p.node.StartedAt) + "\n")
	}
	b.WriteString("\n")
	if p.monitor != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Resources") + "\n")
		b.WriteString(fmt.Sprintf("  CPU:   %.1f%%\n", p.monitor.CPUPercent))
		b.WriteString(fmt.Sprintf("  Mem:   %s / %s\n", bytesHuman(p.monitor.MemUsed), bytesHuman(p.monitor.MemLimit)))
		b.WriteString(fmt.Sprintf("  Net:   ↓ %s  ↑ %s\n", bytesHuman(p.monitor.NetRx), bytesHuman(p.monitor.NetTx)))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  (collecting…)\n"))
	}
	b.WriteString("\n")
	if len(p.node.Interfaces) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Interfaces") + "\n")
		for _, ifc := range p.node.Interfaces {
			if ifc.Name == "" {
				continue
			}
			state := "up"
			color := lipgloss.Color("42")
			if ifc.State == "down" {
				state = "down"
				color = lipgloss.Color("196")
			} else if ifc.State == "unknown" {
				state = "unknown"
				color = lipgloss.Color("240")
			}
			b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
				lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(ifc.Name),
				lipgloss.NewStyle().Foreground(color).Render(state),
				lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(ifc.MAC)))
			if stats, ok := p.stats[ifc.Name]; ok {
				b.WriteString(fmt.Sprintf("    RX: %s  %d pkts  %s/s\n",
					bytesHuman(stats.RxBytes), stats.RxPackets, bytesHuman(uint64(stats.RxBps/8))))
				b.WriteString(fmt.Sprintf("    TX: %s  %d pkts  %s/s\n",
					bytesHuman(stats.TxBytes), stats.TxPackets, bytesHuman(uint64(stats.TxBps/8))))
			}
		}
		if p.statsLoaded && p.statsError {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  stats unavailable\n"))
		}
	}
	// Wrap long values to the fixed inner width before truncating vertically.
	// The outer topology layout owns the border; this pane always returns a
	// stable inner rectangle for it.
	content := lipgloss.NewStyle().Width(p.width).Render(b.String())
	return padToWidthHeight(truncateToHeight(content, p.height), p.width, p.height)
}

// truncateToHeight caps s to at most height lines.
func truncateToHeight(s string, height int) string {
	if height <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		return strings.Join(lines[:height], "\n")
	}
	return s
}

// stateText renders a node state with a color matching the tree status dots.
func stateText(s engine.Status) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	switch s {
	case engine.StatusStopped:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	case engine.StatusPaused:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	case engine.StatusUnknown:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	}
	return style.Render(string(s))
}

// formatUptime renders a StartedAt as "Xh Ym" duration.
func formatUptime(start time.Time) string {
	d := time.Since(start)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

// bytesHuman formats bytes as a human-readable size.
func bytesHuman(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}
