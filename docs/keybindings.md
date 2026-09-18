# Key bindings

**English** | [简体中文](zh-CN/keybindings.md)

clab-tui is fully keyboard driven. Press `?` to expand the help bar, or `space` to open
the which-key menu, which lists every action for the current tab.

## Global

| Key | Action |
|-----|--------|
| `q` / `ctrl+c` | Quit (stops the graph server if running) |
| `tab` / `right` | Next tab |
| `shift+tab` / `left` | Previous tab |
| `1`–`4` | Jump to Topology / Sessions / Node Logs / Ops Log |
| `space` | Open the which-key menu |
| `?` | Toggle the expanded help bar |
| `/` | Search (Topology tab) |
| `l` | Lab actions (Topology tab) |
| `o` | Node or interface actions (Topology tab) |

## Topology tab

| Key | Action |
|-----|--------|
| `j` / `k` | Move down / up |
| `g` / `G` | Jump to top / bottom |
| `ctrl+f` / `ctrl+b` | Page down / up |
| `enter` | Toggle the selected node's connections |
| `x` | Toggle all connection summaries |
| `i` | Toggle interface lines |
| `n` / `N` | Next / previous search match |
| `t` | Start / stop the path trace |
| `f` | Change the trace filter |
| `o` | Node/interface actions |
| `l` | Lab actions |

## Which-key menu (space)

The menu is modal: press the highlighted letter to choose, `j`/`k` to move, `Enter` to
confirm a group or action, and `Esc`/`q` to go back.

- **Topology → Lab**: Switch Lab, Deploy, Destroy, Redeploy, Edit YAML, View Graph.
- **Topology → Node**: SSH, Start, Stop, Restart, Pause, Unpause, View Logs.
- **Topology**: Trace, Filter, Search.
- **Sessions**: Switch Session.
- **Node Logs**: Follow, Clear, Top, Bottom.
- **Ops Log**: Follow, Clear.

## Sessions tab

Sessions have two modes.

**Insert mode (default)** — every key is sent to the active shell, including
`ctrl+c` and `tab`. Press `ctrl+\` to leave.

**Normal mode**:

| Key | Action |
|-----|--------|
| `enter` or `space` | Return to insert mode |
| `s` | Open the session picker |
| label letter (`a`–`z`) | Switch to that session |
| `q` | Close the active session |
| `j` / `k` | Scroll the viewport |
| `tab` / `shift+tab` | Next / previous tab |
| `space` | Open the which-key menu |

## Node Logs tab

| Key | Action |
|-----|--------|
| `f` | Toggle follow |
| `c` | Clear |
| `g` / `G` | Top / bottom |

## Ops Log tab

| Key | Action |
|-----|--------|
| `f` | Toggle follow |
| `c` | Clear |

## Capture pane

The capture pane is an overlay on the Topology tab, shown while a capture is active.

| Key | Action |
|-----|--------|
| `q` | Close (stops the capture) |
| `c` | Clear |
| `p` | Pause / resume |
| `w` | Save the current packets to a pcap file |

## Lists and forms

- **Lab picker / action menu / session picker**: `j`/`k` move, `enter`/`space` select,
  `[`/`]` page, `esc`/`q` close, letter jumps to an item.
- **Search input**: `enter` commits, `esc` cancels; arrow keys and `home`/`end` edit.
  The same input is reused for custom packet filters and the pcap save path.
- **Netem form**: `tab`/`down` next field, `shift+tab`/`up` previous field, `enter`
  applies, `esc` cancels. Fields: Delay, Jitter, Loss, Rate, Corruption.
