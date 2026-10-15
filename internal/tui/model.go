// Package tui implements the interactive adapter and its keyboard event boundary.
package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
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
	schemasPane
)

var panes = []string{"Project", "Environments", "Endpoints", "Request", "Response", "History", "Cases", "Scenarios", "Schemas"}
var actionPanes = map[string]paneID{"project": projectPane, "environments": environmentsPane, "endpoints": endpointsPane, "request": requestPane, "response": responsePane, "history": historyPane, "cases": casesPane, "scenarios": scenariosPane}

type Model struct {
	savedScenarios    []project.ScenarioResource
	scenarioDraft     project.ScenarioResource
	scenarioText      string
	scenarioReport    project.ScenarioReport
	scenarioEvents    chan tea.Msg
	scenarioApprovals chan scenarioApproval

	credentials       *project.Credentials
	credentialEntry   [7]string
	credentialField   int
	historyPage       project.HistoryPage
	historyOffset     int
	historyText       string
	savedCases        []project.CaseResource
	caseDraft         project.CaseResource
	caseDelete        project.CaseResource
	caseText          string
	caseTextBaseline  string
	caseMetadataDirty bool
	requestBaseline   string
	requestText       string
	requestFields     []project.GuidedField
	uploadName        string
	uploadDir         string
	uploadEntries     []os.DirEntry
	uploadIndex       int
	prepared          project.PreparedRequest
	requestCancel     context.CancelFunc
	exchange          project.Exchange
	activeEnvironment string
	environmentDraft  project.Resource
	environmentText   string
	accessReview      project.AccessReview
	specs             []project.Specification
	importPath        string
	importPreview     project.Specification
	sourceCandidate   project.SourceCandidate
	sourceDiscovery   project.SourceDiscovery
	detail            string
	endpointQuery     string
	methodFilter      string
	groupTags         bool
	globalSearch      bool

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
	return NewWithCredentials(root, c, project.DefaultCredentials)
}

func NewWithCredentials(root string, c KeyConfig, credentials *project.Credentials) (*Model, error) {
	b, err := Bindings(c)
	if err != nil {
		return nil, err
	}
	m := &Model{credentials: credentials, root: root, bindings: b, width: 80, height: 24, mode: "NORMAL", initPath: ".", initName: "My API"}
	if root != "" {
		s, err := project.Load(root)
		if err != nil {
			return nil, err
		}
		m.snapshot = s
		m.specs, err = project.Specifications(root)
		if err != nil {
			return nil, err
		}
		m.savedScenarios, err = project.Scenarios(root)
		if err != nil {
			return nil, err
		}
		m.savedCases, err = project.Cases(root)
		if err != nil {
			return nil, err
		}
		m.draft = s.Manifest.Name
		if e, err := project.RememberedEnvironment(root); err == nil {
			m.activeEnvironment = e.ID
		}
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
	case scenarioPromptMsg:
		m.prepared = msg.request
		m.overlay = "scenario-confirm"
		m.mode = "NORMAL"
		return m, nil
	case scenarioFinishedMsg:
		if m.requestCancel != nil {
			m.requestCancel()
			m.requestCancel = nil
		}
		m.scenarioEvents = nil
		m.scenarioApprovals = nil
		m.scenarioReport = msg.report
		m.overlay = "scenario-report"
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.status = "Scenario completed"
		}
		return m, nil
	case editorFinishedMsg:
		m.finishExternalEditor(msg)
		return m, nil
	case exchangeMsg:
		if m.requestCancel != nil {
			m.requestCancel()
			m.requestCancel = nil
		}
		if msg.result.RunID != "" || msg.result.Status != 0 {
			m.exchange = msg.result
			m.refreshHistory()
			m.pane = responsePane
			m.query = ""
			m.index = 0
		}
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.status = "HTTP exchange completed"
		}
		return m, nil
	case refreshMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else if m.dirty && msg.snapshot.Revision != m.snapshot.Revision {
			m.status = "Project changed externally. Draft retained; discard/reload or reconcile before saving."
			m.snapshot.Environments = msg.snapshot.Environments
		} else if !m.dirty {
			m.snapshot = msg.snapshot
			m.savedScenarios, _ = project.Scenarios(m.root)
			if cases, err := project.Cases(m.root); err == nil {
				m.savedCases = cases
				if m.caseDraft.ID != "" {
					for _, current := range cases {
						if current.ID == m.caseDraft.ID && current.Revision != m.caseDraft.Revision {
							if m.requestDirty() {
								m.status = "Case changed externally. Draft retained; reconcile or discard before saving."
							} else {
								m.loadCaseDraft(current)
							}
						}
					}
				}
			} else {
				m.status = err.Error()
			}
			if specs, err := project.Specifications(m.root); err == nil {
				m.specs = specs
			} else {
				m.status = err.Error()
			}
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
	if handled, cmd := m.scenarioKey(msg); handled {
		return m, cmd
	}
	if m.historyKey(msg) {
		return m, nil
	}
	if handled, cmd := m.caseKey(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.requestKey(msg); handled {
		return m, cmd
	}
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
	if m.credentialUpdate(msg) {
		return m, nil
	}
	if m.environmentUpdate(msg) {
		return m, nil
	}
	if m.sourceImportKey(msg) {
		return m, nil
	}
	if m.overlay == "import" {
		if key == "esc" {
			m.overlay = ""
			m.mode = "NORMAL"
			return m, nil
		}
		if key == "enter" {
			if strings.HasPrefix(m.importPath, "http://") || strings.HasPrefix(m.importPath, "https://") {
				if _, err := project.Origin(m.importPath); err != nil {
					m.status = err.Error()
					return m, nil
				}
				m.overlay = "source-approve"
				return m, nil
			}
			preview, err := project.ImportSpecification(m.root, m.importPath, true)
			if err != nil {
				m.status = err.Error()
				return m, nil
			}
			m.importPreview = preview
			m.overlay = "import-confirm"
			return m, nil
		}
		m.importPath = edit(m.importPath, msg)
		return m, nil
	}
	if m.overlay == "import-confirm" {
		if key == "esc" {
			m.overlay = ""
			m.mode = "NORMAL"
			return m, nil
		}
		if key == "enter" {
			var next project.Specification
			var err error
			if m.importPreview.Mode == "url" {
				next, err = project.ImportSource(m.root, m.sourceCandidate, true, "", m.sourceOptions())
			} else {
				next, err = project.ImportSpecification(m.root, m.importPath, true)
			}
			if err != nil {
				m.status = err.Error()
				return m, nil
			}
			if next.Digest != m.importPreview.Digest {
				m.importPreview = next
				m.status = "File changed; review updated preview"
				return m, nil
			}
			if m.importPreview.Mode == "url" {
				_, err = project.ImportSource(m.root, m.sourceCandidate, false, m.importPreview.Digest, m.sourceOptions())
			} else {
				_, err = project.ImportReviewedSpecification(m.root, m.importPath, m.importPreview.Digest)
			}
			if err != nil {
				m.status = err.Error()
				return m, nil
			}
			m.reload()
			m.overlay = ""
			m.mode = "NORMAL"
			m.pane = endpointsPane
			m.status = "Specification imported"
		}
		return m, nil
	}
	if m.overlay == "detail" {
		if key == "esc" {
			m.overlay = ""
		}
		if key == "j" || key == "down" {
			m.paletteIndex++
		}
		if key == "k" || key == "up" {
			m.paletteIndex = max(0, m.paletteIndex-1)
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
		if m.globalSearch {
			m.endpointQuery = edit(m.endpointQuery, msg)
		} else {
			m.query = edit(m.query, msg)
		}
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
	if m.requestDirty() {
		leaving := false
		switch action {
		case "quit", "reload", "discard", "request-reset", "case-open", "case-run", "request-new", "request-compose", "pane-left", "pane-right", "project", "environments", "endpoints", "request", "response", "history", "cases", "scenarios", "schemas", "endpoint-search", "rename", "import":
			leaving = true
		case "open":
			leaving = m.pane == casesPane
		}
		if leaving {
			m.overlay = "case-leave"
			m.nextAction = action
			m.nextCount = n
			return m, nil
		}
	}
	if m.historyAction(action) {
		return m, nil
	}
	if handled, cmd := m.scenarioAction(action); handled {
		return m, cmd
	}
	if handled, cmd := m.caseAction(action); handled {
		return m, cmd
	}
	if handled, cmd := m.requestAction(action); handled {
		return m, cmd
	}
	if m.credentialAction(action) {
		return m, nil
	}
	if m.environmentAction(action) {
		return m, nil
	}
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
	case "import":
		m.overlay = "import"
		m.mode = "INSERT"
		m.importPath = ""
	case "schemas":
		m.pane = schemasPane
		m.index = 0
		m.query = ""
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
		m.query = ""
		m.pane = (m.pane + paneID(len(panes)) - 1) % paneID(len(panes))
		if m.pane == historyPane {
			m.refreshHistory()
		}
		m.index = 0
	case "pane-right":
		m.query = ""
		m.pane = (m.pane + 1) % paneID(len(panes))
		if m.pane == historyPane {
			m.refreshHistory()
		}
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
	case "method-filter":
		methods := []string{"", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE"}
		for i, method := range methods {
			if method == m.methodFilter {
				m.methodFilter = methods[(i+1)%len(methods)]
				break
			}
		}
		m.index = 0
		m.status = "Method: " + m.methodFilter
	case "tag-group":
		m.groupTags = !m.groupTags
		m.index = 0
	case "endpoint-search":
		m.globalSearch = true
		m.endpointQuery = ""
		m.pane = endpointsPane
		m.mode = "SEARCH"
		m.query = ""
		m.index = 0
	case "search":
		m.globalSearch = false
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
		} else if m.pane == endpointsPane || m.pane == schemasPane {
			details := m.contractDetails()
			if len(details) > 0 {
				m.detail = details[min(m.index, len(details)-1)]
				m.paletteIndex = 0
				m.overlay = "detail"
			}
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
	m.savedScenarios, _ = project.Scenarios(m.root)
	if cases, err := project.Cases(m.root); err == nil {
		m.savedCases = cases
	} else {
		m.status = err.Error()
		return
	}
	specs, err := project.Specifications(m.root)
	if err != nil {
		m.status = err.Error()
		return
	}
	m.specs = specs
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
	if m.pane == historyPane {
		return m.historyLines()
	}
	if m.pane == scenariosPane {
		labels := []string{}
		for _, s := range m.scenarioItems() {
			labels = append(labels, s.Name)
		}
		return labels
	}
	if m.pane == casesPane {
		labels := []string{}
		for _, c := range m.caseItems() {
			labels = append(labels, c.Name+" | "+c.Request.EndpointID)
		}
		return labels
	}
	if m.pane == responsePane {
		return m.responseLines()
	}
	if m.pane == endpointsPane || m.pane == schemasPane {
		labels, _ := m.contractItems()
		return labels
	}
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
	if pane == endpointsPane {
		if m.methodFilter != "" {
			title += " " + m.methodFilter
		}
		if m.groupTags {
			title += " by tag"
		}
	}
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
		body += "\nCLI access requires TUI approval"
	case endpointsPane, schemasPane:
		labels, _ := m.contractItems()
		if len(labels) == 0 {
			body = "No matching imported resources"
		}
		start := min(m.index, max(0, len(labels)-1))
		for i := start; i < len(labels); i++ {
			mark := "  "
			if i == m.index {
				mark = "> "
			}
			if i == m.index && pane == endpointsPane {
				body += ansi.Wrap(mark+labels[i], max(1, width), "") + "\n"
			} else {
				body += mark + labels[i] + "\n"
			}
		}
	case requestPane:
		body = m.requestText
		if m.overlay == "request-confirm" {
			body = "Review the sanitized destination in the confirmation"
		}
		if m.requestText != "" {
			label := "Draft"
			if m.caseDraft.ID != "" {
				label = "Case: " + m.caseDraft.Name
			}
			if m.requestDirty() {
				label += " | unsaved"
			}
			body = label + "\n" + body
		}
		if body == "" {
			body = "No request selected | request new or compose"
		}
	case responsePane:
		lines := m.responseLines()
		start := min(m.index, max(0, len(lines)-1))
		body = strings.Join(lines[start:], "\n")
	case historyPane:
		body = strings.Join(m.historyLines(), "\n")
	case casesPane:
		body = "No cases saved"
		if items := m.caseItems(); len(items) > 0 {
			body = ""
			for i, c := range items {
				mark := "  "
				if i == m.index {
					mark = "> "
				}
				body += mark + c.Name + " | endpoint " + c.Request.EndpointID + "\n"
			}
		}
	case scenariosPane:
		body = "No scenarios saved"
		if items := m.scenarioItems(); len(items) > 0 {
			body = ""
			for i, s := range items {
				mark := "  "
				if i == m.index {
					mark = "> "
				}
				body += mark + s.Name + "\n"
			}
		}
	}
	return fit(m.credentials.Sanitize(title+"\n"+body), max(1, width), max(1, height))
}
func (m *Model) View() tea.View {
	if err := m.credentials.Refresh(m.root); err != nil {
		return m.credentialRecoveryView()
	}
	width, height := m.width, m.height
	dirty := ""
	if m.dirty || m.requestDirty() {
		dirty = " *unsaved"
	}
	header := fmt.Sprintf("Reqscope | %s%s | %s | %s", m.draft, dirty, m.mode, panes[m.pane])
	active := ansi.Wrap(m.credentials.Sanitize(m.activeDestination()), max(1, width), "")
	activeHeight := len(strings.Split(active, "\n"))
	contentHeight := max(1, height-3-activeHeight)
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
		if m.globalSearch {
			footer = "Endpoints /" + m.endpointQuery
		}
	}
	if m.pending != "" {
		footer = m.pending
	}
	base := fit(m.credentials.Sanitize(header+"\n"+active+"\n"+content+"\n"+footer+"\n? help | normal/insert | pane switches"), width, height)
	overlay := m.environmentOverlay()
	if m.overlay == "history-settings" {
		overlay = "History retention JSON | Enter save | Esc cancel\n" + m.historyText + "\n" + m.status
	}
	if credentialOverlay := m.credentialOverlay(); credentialOverlay != "" {
		overlay = credentialOverlay
	}
	if requestOverlay := m.requestOverlay(); requestOverlay != "" {
		overlay = requestOverlay
	}
	if caseOverlay := m.caseOverlay(); caseOverlay != "" {
		overlay = caseOverlay
	}
	if scenarioOverlay := m.scenarioOverlay(); scenarioOverlay != "" {
		overlay = scenarioOverlay
	}
	if sourceOverlay := m.sourceImportOverlay(); sourceOverlay != "" {
		overlay = sourceOverlay
	}
	switch m.overlay {
	case "import":
		overlay = "Import specification file or Swagger/raw URL\nSource: " + m.importPath + "\nEnter preview | Esc cancel\n" + m.status
	case "import-confirm":
		overlay = fmt.Sprintf("Import preview\n%s\n%d endpoints, %d schemas\n%s\nEnter apply | Esc cancel\n%s", m.importPath, len(m.importPreview.Endpoints), len(m.importPreview.Schemas), strings.Join(m.importPreview.Diagnostics, "\n"), m.status)
	case "detail":
		lines := strings.Split(m.detail, "\n")
		start := min(m.paletteIndex, max(0, len(lines)-1))
		overlay = "Contract inspection | j/k scroll | Esc close\n" + strings.Join(lines[start:], "\n")
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
		overlay = m.credentials.Sanitize(overlay)
		ow := max(1, min(width, 64))
		if strings.HasPrefix(m.overlay, "environment-") || strings.HasPrefix(m.overlay, "credential-") || strings.HasPrefix(m.overlay, "source-") || strings.HasPrefix(m.overlay, "import") {
			ow = width
			overlay = ansi.Wrap(overlay, max(1, ow-4), "")
		}
		oh := max(1, min(height, len(strings.Split(overlay, "\n"))+2))
		style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(max(1, ow-2))
		box := fit(style.Render(fit(overlay, max(1, ow-2), max(1, oh-2))), ow, oh)
		base = lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(box).X(max(0, (width-ow)/2)).Y(max(0, (height-oh)/2)).Z(1)).Render()
	}
	v := tea.NewView(base)
	v.AltScreen = true
	return v
}

func (m *Model) contractItems() ([]string, []string) {
	labels := []string{}
	details := []string{}
	for _, s := range m.specs {
		if m.pane == schemasPane {
			for _, schema := range s.Schemas {
				b, _ := yaml.Marshal(schema)
				text := string(b) + "Source: " + s.Location + "\n" + strings.Join(s.Diagnostics, "\n")
				if strings.Contains(strings.ToLower(text), strings.ToLower(m.query)) {
					labels = append(labels, schema.Name)
					details = append(details, text)
				}
			}

		}
	}
	if m.pane != schemasPane {
		focusedQuery := ""
		if m.pane == endpointsPane {
			focusedQuery = m.query
		}
		query := m.endpointQuery
		allowed := map[string]bool{}
		if focusedQuery != "" {
			query = focusedQuery
			for _, match := range project.SearchEndpoints(m.specs, project.EndpointQuery{Query: m.endpointQuery}) {
				allowed[match.ID] = true
			}
		}
		for _, match := range project.SearchEndpoints(m.specs, project.EndpointQuery{Query: query, Method: m.methodFilter, GroupByTag: m.groupTags}) {
			if focusedQuery != "" && !allowed[match.ID] {
				continue
			}
			b, _ := yaml.Marshal(match.Endpoint)
			text := string(b) + "Source: " + match.Source + "\n"
			for _, source := range m.specs {
				if source.ID == match.SourceID {
					text += strings.Join(source.Diagnostics, "\n")
					break
				}
			}
			label := match.Method + " " + match.Path + " | " + match.Summary + " | " + match.Source + " | module " + match.ModuleID
			if match.Group != "" {
				label = "[" + match.Group + "] " + label
			}
			labels = append(labels, label)
			details = append(details, text)
		}
	}

	return labels, details
}
func (m *Model) contractDetails() []string { _, details := m.contractItems(); return details }
