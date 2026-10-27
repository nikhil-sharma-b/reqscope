package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The leader menu is the LazyVim way in: press the leader (space by default)
// and a menu lists what the next key does, grouped by topic, so every command
// can be found by reading rather than remembering. The single keys stay; the
// menu is the map to them and to the commands that have no key of their own.
// Each entry runs through runCommand, which first moves to the screen where
// the action applies.

type leaderItem struct {
	key   string
	label string
	act   action
	group *leaderGroup
}

type leaderGroup struct {
	name  string
	items []leaderItem
}

func do(key, label string, act action) leaderItem {
	return leaderItem{key: key, label: label, act: act}
}
func group(key, name string, items ...leaderItem) leaderItem {
	return leaderItem{key: key, label: "+" + name, group: &leaderGroup{name: name, items: items}}
}

func leaderTree() *leaderGroup {
	return &leaderGroup{items: []leaderItem{
		group("g", "go to",
			do("e", "endpoints", actTab1),
			do("s", "saved requests", actTab2),
			do("c", "scenarios", actTab3),
			do("h", "history", actTab4),
			do("v", "environments", actTab5),
		),
		group("r", "request",
			do("s", "send", actSend),
			do("w", "save", actSave),
			do("W", "save as new", actSaveAsNew),
			do("n", "new blank request", actNewRequest),
			do("j", "edit as JSON", actEditRaw),
			do("b", "edit body", actBody),
			do("t", "body type", actBodyType),
			do("a", "attach file", actAttach),
			do("m", "method", actMethodCycle),
			do("x", "clear", actClearRequest),
			do("k", "cancel running", actCancel),
		),
		group("o", "response",
			do("y", "copy body", actCopy),
			do("H", "toggle headers", actHeaders),
			do("/", "find", actFind),
		),
		group("a", "api contract",
			do("i", "import", actImport),
			do("v", "view contract", actContract),
			do("r", "refresh source", actRefreshSource),
			do("s", "endpoints / schemas", actSchemas),
			do("m", "method filter", actMethod),
			do("t", "group by tag", actGroup),
			do("e", "edit contract", actEditContract),
			do("o", "export contract", actExportContract),
		),
		group("c", "saved",
			do("r", "run", actRunCase),
			do("e", "details", actEditCase),
			do("d", "duplicate", actDuplicateCase),
			do("x", "delete", actDeleteCase),
			do("z", "reload from disk", actReloadCase),
			do("a", "attach run", actAttachRun),
		),
		group("s", "scenarios",
			do("r", "run", actRunScenario),
			do("n", "new", actNewScenario),
			do("v", "check", actCheckScenario),
			do("a", "approve logins", actApproveLogins),
			do("x", "delete", actDeleteScenario),
		),
		group("h", "history",
			do("p", "pin / unpin", actPin),
			do("c", "compare", actCompare),
			do("i", "compare by ID", actCompareIDs),
			do("x", "delete run", actDeleteRun),
			do("s", "retention", actRetention),
			do("P", "prune", actPrune),
		),
		group("e", "environments",
			do("e", "switch", actEnvironment),
			do("u", "use selected", actUseEnv),
			do("n", "new", actNewEnv),
			do("c", "credentials", actCredentials),
			do("a", "allow CLI", actAuthorize),
			do("x", "delete", actDeleteEnv),
		),
		group("p", "project",
			do("n", "rename", actRenameProject),
			do("v", "check files", actValidateProject),
			do("r", "reload", actReloadProject),
		),
		group("w", "panes",
			do("h", "left pane", actPaneLeft),
			do("l", "right pane", actPaneRight),
			do("z", "zoom", actZoom),
		),
		do("/", "filter list", actFilter),
		do(":", "commands", actPalette),
		do("?", "keys", actHelp),
		do("q", "quit", actQuit),
	}}
}

// leaderState is the menu while it is open: the keys typed so far and the
// group they reached.
type leaderState struct {
	path  []string
	stack []*leaderGroup
}

func (l *leaderState) current() *leaderGroup { return l.stack[len(l.stack)-1] }

func (m *Model) openLeader() {
	if m.leaderRoot == nil {
		m.leaderRoot = leaderTree()
	}
	m.leader = &leaderState{path: []string{m.keys.leader}, stack: []*leaderGroup{m.leaderRoot}}
}

func (m *Model) leaderKey(msg tea.KeyPressMsg) tea.Cmd {
	l := m.leader
	key := msg.String()
	switch key {
	case "esc":
		m.leader = nil
		return nil
	case "backspace":
		if len(l.stack) == 1 {
			m.leader = nil
			return nil
		}
		l.stack, l.path = l.stack[:len(l.stack)-1], l.path[:len(l.path)-1]
		return nil
	}
	for _, it := range l.current().items {
		if it.key != key {
			continue
		}
		if it.group != nil {
			l.stack, l.path = append(l.stack, it.group), append(l.path, key)
			return nil
		}
		m.leader = nil
		return m.runCommand(it.act)
	}
	m.leader = nil
	m.say(strings.Join(append(l.path, key), " ") + " does nothing")
	return nil
}

// whichKey draws the open menu as a framed float above the footer's right
// end, titled with the keys typed so far. Entries run down in columns.
func (m *Model) whichKey(width, height int) []string {
	g := m.leader.current()
	keyWidth, labelWidth := 1, 1
	for _, it := range g.items {
		keyWidth = max(keyWidth, ansi.StringWidth(it.key))
		labelWidth = max(labelWidth, ansi.StringWidth(it.label))
	}
	colWidth := keyWidth + 3 + labelWidth
	rowsMax := max(1, height-4)
	cols := (len(g.items) + rowsMax - 1) / rowsMax
	// Prefer two columns over one tall one when there is room for it.
	if cols == 1 && len(g.items) > 8 && 2*colWidth+3+4 <= width {
		cols = 2
	}
	perCol := (len(g.items) + cols - 1) / cols
	lines := make([]string, perCol)
	for i, it := range g.items {
		label := it.label
		if it.group != nil {
			label = accent.Render(label)
		}
		cell := keyStyle.Render(pad(it.key, keyWidth)) + quiet.Render(" → ") + pad(label, labelWidth)
		r := i % perCol
		if lines[r] != "" {
			lines[r] += "   "
		}
		lines[r] += cell
	}
	title := strings.Join(m.leader.path, " ")
	boxWidth := min(width, max(cols*colWidth+(cols-1)*3+4, ansi.StringWidth(title)+ansi.StringWidth("esc close")+10))
	note := quiet.Render("esc close")
	return frame(lines, title, note, boxWidth, len(lines)+2, true)
}

// float lays box over base with its top-left corner at x, y, keeping the
// colours of what it covers on either side.
func float(base, box []string, x, y, width int) []string {
	out := append([]string(nil), base...)
	for i, line := range box {
		row := y + i
		if row < 0 || row >= len(out) {
			continue
		}
		w := ansi.StringWidth(line)
		out[row] = ansi.Cut(out[row], 0, x) + line + ansi.Cut(out[row], x+w, width)
	}
	return out
}
