// Package tui implements the interactive adapter and its keyboard event boundary.
package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type paneID int

const (
	projectPane paneID = iota
	environmentsPane
	endpointsPane
	requestPane
	responsePane
	historyPane
	casesPane
	scenariosPane
)

var panes = []string{"Project", "Environments", "Endpoints", "Request", "Response", "History", "Cases", "Scenarios"}
var actionPanes = map[string]paneID{"project": projectPane, "environments": environmentsPane, "endpoints": endpointsPane, "request": requestPane, "response": responsePane, "history": historyPane, "cases": casesPane, "scenarios": scenariosPane}

type Model struct {
	paletteQuery                          string
	nextAction                            string
	nextCount                             int
	root                                  string
	snapshot                              project.Snapshot
	bindings                              []Binding
	width, height, index                  int
	pane                                  paneID
	zoom                                  bool
	mode, pending, count, overlay, status string
	draft, query                          string
	dirty                                 bool
	paletteIndex                          int
	initPath, initName                    string
	initField                             int
}

func New(root string, c KeyConfig) (*Model, error) {
	b, err := Bindings(c)
	if err != nil {
		return nil, err
	}
	m := &Model{root: root, bindings: b, width: 80, height: 24, mode: "NORMAL", initPath: ".", initName: "My API"}
	if root != "" {
		s, err := project.Load(root)
		if err != nil {
			return nil, err
		}
		m.snapshot = s
		m.draft = s.Manifest.Name
	} else {
		m.overlay = "initialize"
		m.mode = "INSERT"
	}
	return m, nil
}
func Run(root, keys string, in io.Reader, out io.Writer) error {
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		found, err := project.Discover(cwd)
		if err == nil {
			root = found
		} else if e, ok := err.(*project.Error); !ok || e.Code != "not_found" {
			return err
		}
	}
	if root != "" && filepath.Base(root) == "reqscope.yaml" {
		root = filepath.Dir(root)
	}
	if keys == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		candidate := filepath.Join(dir, "reqscope", "keys.yaml")
		if _, err = os.Stat(candidate); err == nil {
			keys = candidate
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	c, err := loadKeys(keys)
	if err != nil {
		return err
	}
	m, err := New(root, c)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out)).Run()
	return err
}

type refreshMsg struct {
	snapshot project.Snapshot
	err      error
}

func (m *Model) poll() tea.Cmd {
	if m.root == "" {
		return nil
	}
	root := m.root
	return tea.Tick(time.Second, func(time.Time) tea.Msg { s, err := project.Load(root); return refreshMsg{s, err} })
}
func (m *Model) Init() tea.Cmd { return m.poll() }
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else if m.dirty && msg.snapshot.Revision != m.snapshot.Revision {
			m.status = "Project changed externally. Draft retained; discard/reload or reconcile before saving."
			m.snapshot.Environments = msg.snapshot.Environments
		} else if !m.dirty {
			m.snapshot = msg.snapshot
			m.draft = msg.snapshot.Manifest.Name
		}
		return m, m.poll()
	case tea.WindowSizeMsg:
		m.width = max(1, msg.Width)
		m.height = max(1, msg.Height)
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}
func (m *Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == " " {
		key = "space"
	}
	if m.overlay == "initialize" {
		if key == "esc" || key == "ctrl+c" {
			return m, tea.Quit
		}
		if key == "tab" {
			m.initField = 1 - m.initField
			return m, nil
		}
		if key == "enter" {
			s, err := project.Init(m.initPath, m.initName, false)
			if err != nil {
				m.status = err.Error()
				return m, nil
			}
			m.root = m.initPath
			m.snapshot = s
			m.draft = s.Manifest.Name
			m.overlay = ""
			m.mode = "NORMAL"
			m.status = "Project initialized"
			return m, m.poll()
		}
		if m.initField == 0 {
			m.initPath = edit(m.initPath, msg)
		} else {
			m.initName = edit(m.initName, msg)
		}
		return m, nil
	}
	if key == "esc" {
		m.pending = ""
		m.count = ""
		m.mode = "NORMAL"
		m.overlay = ""
		return m, nil
	}
	if m.overlay == "confirm-leave" {
		switch key {
		case "s":
			if !m.save() {
				return m, nil
			}
		case "d":
			m.dirty = false
			m.reload()
		case "c":
			m.overlay = ""
			m.nextAction = ""
			return m, nil
		default:
			return m, nil
		}
		m.overlay = ""
		action, n := m.nextAction, m.nextCount
		m.nextAction = ""
		return m.action(action, n)
	}
	if m.overlay == "confirm-quit" {
		switch key {
		case "s":
			if m.save() {
				return m, tea.Quit
			}
		case "d":
			return m, tea.Quit
		case "c":
			m.overlay = ""
		}
		return m, nil
	}
	if m.overlay == "help" {
		if key == "j" || key == "down" {
			m.paletteIndex++
		}
		if key == "k" || key == "up" {
			m.paletteIndex = max(0, m.paletteIndex-1)
		}
		return m, nil
	}
	if m.overlay == "palette" {
		switch key {
		case "down", "ctrl+d":
			m.paletteIndex++
		case "up", "ctrl+u":
			m.paletteIndex = max(0, m.paletteIndex-1)
		case "enter":
			actions := m.paletteActions()
			if len(actions) > 0 {
				b := actions[min(m.paletteIndex, len(actions)-1)]
				m.overlay = ""
				m.mode = "NORMAL"
				return m.action(b.Action, 1)
			}
		default:
			m.paletteQuery = edit(m.paletteQuery, msg)
			m.paletteIndex = 0
		}
		return m, nil
	}
	if m.mode == "INSERT" {
		if key == "enter" {
			m.mode = "NORMAL"
			return m, nil
		}
		m.draft = edit(m.draft, msg)
		m.dirty = m.draft != m.snapshot.Manifest.Name
		return m, nil
	}
	if m.mode == "SEARCH" {
		if key == "enter" {
			m.mode = "NORMAL"
			return m, nil
		}
		m.query = edit(m.query, msg)
		m.index = 0
		return m, nil
	}
	if m.pending == "" && len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		if len(m.count) < 4 {
			m.count += key
		}
		return m, nil
	}
	sequence := key
	if m.pending != "" {
		sequence = m.pending + " " + key
	}
	prefix := false
	for _, b := range m.bindings {
		for _, seq := range b.Keys {
			if seq == sequence {
				n := 1
				if m.count != "" {
					n, _ = strconv.Atoi(m.count)
					n = max(1, n)
				}
				m.pending = ""
				m.count = ""
				return m.action(b.Action, n)
			}
			if strings.HasPrefix(seq, sequence+" ") {
				prefix = true
			}
		}
	}
	if prefix {
		m.pending = sequence
	} else {
		m.pending = ""
		m.count = ""
		m.status = "Unknown binding: " + sequence
	}
	return m, nil
}
func edit(s string, msg tea.KeyPressMsg) string {
	switch msg.String() {
	case "backspace":
		if s != "" {
			_, n := utf8.DecodeLastRuneInString(s)
			return s[:len(s)-n]
		}
	case "space":
		return s + " "
	default:
		if msg.Text != "" && !strings.Contains(msg.Text, "\n") {
			return s + msg.Text
		}
	}
	return s
}
func (m *Model) action(action string, n int) (tea.Model, tea.Cmd) {
	if m.dirty && m.pane == projectPane {
		switch action {
		case "pane-left", "pane-right", "environments", "endpoints", "request", "response", "history", "cases", "scenarios", "endpoint-search":
			m.overlay = "confirm-leave"
			m.nextAction = action
			m.nextCount = n
			return m, nil
		}
	}
	if pane, ok := actionPanes[action]; ok {
		m.pane = pane
		m.index = 0
		m.query = ""
		return m, nil
	}
	switch action {
	case "quit":
		if m.dirty {
			m.overlay = "confirm-quit"
		} else {
			return m, tea.Quit
		}
	case "down":
		m.index = min(max(0, len(m.items())-1), m.index+n)
	case "up":
		m.index = max(0, m.index-n)
	case "first":
		m.index = 0
	case "last":
		m.index = max(0, len(m.items())-1)
	case "half-down":
		m.index = min(max(0, len(m.items())-1), m.index+n*max(1, m.height/2))
	case "half-up":
		m.index = max(0, m.index-n*max(1, m.height/2))
	case "pane-left":
		m.pane = (m.pane + paneID(len(panes)) - 1) % paneID(len(panes))
		m.index = 0
	case "pane-right":
		m.pane = (m.pane + 1) % paneID(len(panes))
		m.index = 0
	case "zoom":
		m.zoom = !m.zoom
	case "rename":
		m.query = ""
		m.pane = projectPane
		m.mode = "INSERT"
	case "save":
		m.save()
	case "reload":
		if m.dirty {
			m.status = "Unsaved name retained. Save it or use the palette's discard action."
		} else {
			m.reload()
		}
	case "validate":
		_, err := project.Load(m.root)
		if err != nil {
			m.status = err.Error()
		} else {
			m.status = "Project validation passed"
		}
	case "endpoint-search":
		m.pane = endpointsPane
		m.mode = "SEARCH"
		m.query = ""
		m.index = 0
	case "search":
		m.mode = "SEARCH"
		m.query = ""
		m.index = 0
	case "next-match":
		items := m.items()
		if len(items) > 0 {
			m.index = (m.index + n) % len(items)
		}
	case "previous-match":
		items := m.items()
		if len(items) > 0 {
			m.index = (m.index - n%len(items) + len(items)) % len(items)
		}
	case "open":
		if m.pane == projectPane {
			m.mode = "INSERT"
		} else if m.pane == environmentsPane {
			items := m.environmentItems()
			if len(items) > 0 {
				e := items[min(m.index, len(items)-1)]
				state := "unconfigured"
				if len(e.BaseURLs) > 0 {
					state = fmt.Sprintf("%d configured destinations", len(e.BaseURLs))
				}
				m.status = e.Name + ": " + e.Classification + ", " + state + "; CLI access denied"
			}
		}
	case "help":
		m.overlay = "help"
		m.paletteIndex = 0
	case "palette":
		m.overlay = "palette"
		m.mode = "INSERT"
		m.paletteQuery = ""
		m.paletteIndex = 0
	case "discard":
		m.dirty = false
		m.reload()
	}
	return m, nil
}
func (m *Model) save() bool {
	if !m.dirty {
		m.status = "No unsaved changes"
		return true
	}
	manifest := m.snapshot.Manifest
	manifest.Name = m.draft
	b, err := yaml.Marshal(manifest)
	if err == nil {
		err = project.Apply(m.root, []project.Change{{Path: "reqscope.yaml", ExpectedRevision: m.snapshot.Revision, Content: b}}, false)
	}
	if err != nil {
		m.status = err.Error()
		return false
	}
	m.dirty = false
	m.reload()
	m.status = "Project name saved"
	return true
}
func (m *Model) reload() {
	s, err := project.Load(m.root)
	if err != nil {
		m.status = err.Error()
		return
	}
	m.snapshot = s
	m.draft = s.Manifest.Name
	m.status = "Project reloaded"
}
func (m *Model) environmentItems() []project.Resource {
	var items []project.Resource
	for _, e := range m.snapshot.Environments {
		if strings.Contains(strings.ToLower(e.Name), strings.ToLower(m.query)) {
			items = append(items, e)
		}
	}
	return items
}
func (m *Model) items() []string {
	if m.pane == environmentsPane {
		var items []string
		for _, e := range m.environmentItems() {
			items = append(items, e.Name+" | "+e.Classification)
		}
		return items
	}
	if m.pane == projectPane {
		var result []string
		for _, line := range []string{"Name: " + m.draft, "ID: " + m.snapshot.Manifest.ID, "Format: 1", "Revision: " + m.snapshot.Revision} {
			if strings.Contains(strings.ToLower(line), strings.ToLower(m.query)) {
				result = append(result, line)
			}
		}
		return result
	}
	return nil
}
func (m *Model) paletteActions() []Binding {
	actions := append([]Binding{}, m.bindings...)
	actions = append(actions, Binding{Action: "discard", Description: "Discard name edits and reload"})
	var result []Binding
	for _, b := range actions {
		if strings.Contains(strings.ToLower(b.Description), strings.ToLower(m.paletteQuery)) {
			result = append(result, b)
		}
	}
	return result
}
func fit(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
func (m *Model) paneView(pane paneID, width, height int) string {
	title := panes[pane]
	if pane == m.pane {
		title = "> " + title
	}
	var body string
	switch pane {
	case projectPane:
		items := m.items()
		if len(items) == 0 {
			body = "No matching project fields"
		}
		start := max(0, m.index-max(1, height-2)+1)
		for i := start; i < len(items); i++ {
			mark := "  "
			if i == m.index {
				mark = "> "
			}
			body += mark + items[i] + "\n"
		}
		body += "\n" + m.root
	case environmentsPane:
		items := m.environmentItems()
		if len(items) == 0 {
			body = "No matching environments"
		}
		start := max(0, m.index-max(1, height-3)+1)
		for i := start; i < len(items); i++ {
			mark := "  "
			if i == m.index {
				mark = "> "
			}
			body += mark + items[i].Name + " | " + items[i].Classification + "\n"
		}
		body += "\nNo CLI grants configured"
	case endpointsPane:
		body = "No specifications imported"
	case requestPane:
		body = "No request selected"
	case responsePane:
		body = "No response recorded"
	case historyPane:
		body = "No runs recorded"
	case casesPane:
		body = "No cases saved"
	case scenariosPane:
		body = "No scenarios saved"
	}
	return fit(title+"\n"+body, max(1, width), max(1, height))
}
func (m *Model) View() tea.View {
	width, height := m.width, m.height
	dirty := ""
	if m.dirty {
		dirty = " *unsaved"
	}
	header := fmt.Sprintf("Reqscope | %s%s | %s | %s", m.draft, dirty, m.mode, panes[m.pane])
	contentHeight := max(1, height-3)
	var content string
	if width < 90 || m.zoom {
		content = m.paneView(m.pane, width, contentHeight)
	} else {
		left := width / 3
		right := width - left - 3
		other := requestPane
		if m.pane != endpointsPane {
			other = m.pane
		}
		rightView := m.paneView(other, right, contentHeight)
		if m.pane == endpointsPane || m.pane == requestPane || m.pane == responsePane {
			top := max(1, contentHeight/2)
			bottom := max(1, contentHeight-top)
			rightView = m.paneView(requestPane, right, top) + "\n" + m.paneView(responsePane, right, bottom)
		}
		content = lipgloss.JoinHorizontal(lipgloss.Top, m.paneView(endpointsPane, left, contentHeight), " | ", rightView)
	}
	footer := m.status
	if m.mode == "SEARCH" {
		footer = "/" + m.query
	}
	if m.pending != "" {
		footer = m.pending
	}
	base := fit(header+"\n"+content+"\n"+footer+"\n? help | normal/insert | pane switches", width, height)
	var overlay string
	switch m.overlay {
	case "initialize":
		field := "Path"
		if m.initField == 1 {
			field = "Name"
		}
		overlay = "Initialize project\nPath: " + m.initPath + "\nName: " + m.initName + "\nEditing: " + field + "\nTab switch | Enter create | Esc quit\n" + m.status
	case "help":
		lines := strings.Split(help(m.bindings), "\n")
		start := min(m.paletteIndex, max(0, len(lines)-1))
		overlay = "Effective bindings | Esc close\n" + strings.Join(lines[start:], "\n")
	case "palette":
		overlay = "Command palette: " + m.paletteQuery + "\n"
		actions := m.paletteActions()
		if len(actions) == 0 {
			overlay += "No matching commands"
		}
		selected := min(m.paletteIndex, max(0, len(actions)-1))
		start := max(0, selected-max(1, height-6)+1)
		for i := start; i < len(actions); i++ {
			mark := "  "
			if i == selected {
				mark = "> "
			}
			overlay += mark + actions[i].Description + "\n"
		}
	case "confirm-leave":
		overlay = "Unsaved project name\ns save | d discard | c cancel"
	case "confirm-quit":
		overlay = "Unsaved project name\ns save | d discard | c cancel"
	}
	if m.pending != "" {
		overlay = m.pending + "\n" + continuations(m.bindings, m.pending) + "\nEsc cancel"
	}
	if overlay != "" {
		ow := max(1, min(width, 64))
		oh := max(1, min(height, len(strings.Split(overlay, "\n"))+2))
		style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(max(1, ow-2))
		box := fit(style.Render(fit(overlay, max(1, ow-2), max(1, oh-2))), ow, oh)
		base = lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(box).X(max(0, (width-ow)/2)).Y(max(0, (height-oh)/2)).Z(1)).Render()
	}
	v := tea.NewView(base)
	v.AltScreen = true
	return v
}
