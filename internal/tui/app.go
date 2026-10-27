// Package tui implements the interactive adapter: a list of what the project
// holds, a request editor beside the response it produced, and one line of
// keys for whatever is in front of the person.
package tui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

type screen int

const (
	browseScreen screen = iota
	requestScreen
)

type tabID int

const (
	endpointsTab tabID = iota
	savedTab
	scenariosTab
	historyTab
	envTab
	tabCount
)

var tabNames = [tabCount]string{"Endpoints", "Saved", "Scenarios", "History", "Environments"}
var tabContexts = [tabCount]string{"endpoints", "saved", "scenarios", "history", "environments"}

type flashKind int

const (
	flashInfo flashKind = iota
	flashOK
	flashErr
)

// flash is the one-line answer to the last key press. It stays until the
// next key, so it is never gone before it is read.
type flash struct {
	text string
	kind flashKind
}

// busy is a request or scenario in flight; Esc cancels it.
type busy struct {
	label  string
	cancel context.CancelFunc
}

// applyMsg carries work finished off the UI goroutine back to the model.
type applyMsg func(m *Model) tea.Cmd

type Model struct {
	root        string
	credentials *project.Credentials
	keys        *keymap
	width       int
	height      int
	data        workspace
	// credErr is set while the credential store cannot be read. The app then
	// shows only what cannot contain a secret until the person recovers.
	credErr error

	screen screen
	tab    tabID
	lists  [tabCount]listState

	schemas      bool
	methodFilter string
	groupByTag   bool
	epCache      endpointCache
	schemaCache  schemaCache
	preview      previewCache

	drafts    map[string]*draft
	req       *draft
	scratchN  int
	respFocus bool
	// zoom shows the focused pane alone, kept per screen.
	zoom [2]bool
	// detailFocus puts keys on Browse's details pane, which then scrolls.
	detailFocus  bool
	detailScroll int

	history     []project.Run
	historyNext *int
	historyMeta project.HistoryPage
	compareFrom string

	modal modal
	flash flash
	// leader is the key menu while it is open.
	leader     *leaderState
	leaderRoot *leaderGroup
	busy       *busy

	editorDone func(text string, err error) tea.Cmd

	scenarioEvents    chan tea.Msg
	scenarioApprovals chan scenarioApproval
	scenarioPaused    bool
}

func New(root string, c KeyConfig) (*Model, error) {
	return NewWithCredentials(root, c, project.DefaultCredentials)
}

func NewWithCredentials(root string, c KeyConfig, credentials *project.Credentials) (*Model, error) {
	k, err := newKeymap(c)
	if err != nil {
		return nil, err
	}
	m := &Model{root: root, credentials: credentials, keys: k, width: 80, height: 24, drafts: map[string]*draft{}, leaderRoot: leaderTree()}
	if root == "" {
		m.modal = initForm()
		return m, nil
	}
	if err := m.open(); err != nil {
		return nil, err
	}
	return m, nil
}

// open loads a project for the first time. Failures here stop startup, as a
// project that cannot be read has nothing safe to show.
func (m *Model) open() error {
	if _, err := project.Load(m.root); err != nil {
		return err
	}
	if e, err := project.RememberedEnvironment(m.root); err == nil {
		m.data.activeEnv = e.ID
	}
	m.data.gen++
	msg := loadWorkspace(m.root, m.data.activeEnv, project.Fingerprint{}, "", m.credentials, m.data.gen)
	if len(msg.errs) > 0 {
		return msg.errs[0]
	}
	m.applyLoad(msg)
	return nil
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
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))
	// The terminal's background picks the palette's light or dark variant.
	go p.Send(tea.RequestBackgroundColor())
	_, err = p.Run()
	return err
}

// Init looks for outside changes straight away, then once a second.
func (m *Model) Init() tea.Cmd { return m.poll(0) }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		applyTheme(msg.IsDark())
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
	case loadedMsg:
		m.applyLoad(msg)
		return m, m.poll(pollInterval)
	case applyMsg:
		return m, msg(m)
	case tea.PasteMsg:
		m.paste(msg.Content)
	case tea.KeyPressMsg:
		return m, m.keyPress(msg)
	case exchangeMsg:
		return m, m.finishSend(msg)
	case editorFinishedMsg:
		done := m.editorDone
		m.editorDone = nil
		if done != nil {
			return m, done(msg.text, msg.err)
		}
	case scenarioPromptMsg:
		return m, m.confirmScenarioSend(msg.request)
	case scenarioFinishedMsg:
		m.finishScenario(msg)
	}
	return m, nil
}

func (m *Model) say(text string) { m.flash = flash{m.credentials.Sanitize(text), flashInfo} }
func (m *Model) ok(text string)  { m.flash = flash{m.credentials.Sanitize(text), flashOK} }
func (m *Model) fail(err error) {
	if err != nil {
		m.flash = flash{m.credentials.Sanitize(sentence(err.Error())), flashErr}
	}
}

// sentence capitalises a backend message for display.
func sentence(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (m *Model) keyPress(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	m.flash = flash{}
	if key == "ctrl+c" {
		if m.busy != nil && m.busy.cancel != nil {
			return m.cancelBusy()
		}
		return tea.Quit
	}
	if m.modal != nil {
		return m.modal.update(m, msg)
	}
	if m.credErr != nil {
		return m.recoveryKey(msg)
	}
	if m.leader != nil {
		return m.leaderKey(msg)
	}
	// A field being typed in takes Esc before a running request does.
	if handled, cmd := m.inputKey(msg); handled {
		return cmd
	}
	if m.busy != nil && key == "esc" {
		return m.cancelBusy()
	}
	if key == "esc" {
		return m.back()
	}
	if key == m.keys.leader {
		m.openLeader()
		return nil
	}
	id, ok := m.keys.resolve(contexts[m.context()], key)
	if !ok {
		if msg.Text != "" {
			m.say(key + " does nothing here. Press " + m.keyName(actHelp) + " to see the keys.")
		}
		return nil
	}
	return m.do(id)
}

// context names the screen in front of the person, which picks the live keys.
func (m *Model) context() string {
	if m.screen == requestScreen && m.req != nil {
		if m.respFocus {
			return "response"
		}
		return "form"
	}
	return tabContexts[m.tab]
}

func (m *Model) keyName(id action) string {
	if k := m.keys.key(id); k != "" {
		return k
	}
	return ":"
}

func (m *Model) back() tea.Cmd {
	switch {
	case m.zoom[m.screen]:
		m.zoom[m.screen] = false
	case m.rightFocused():
		m.focusPane(false)
	case m.screen == requestScreen:
		m.screen = browseScreen
	case m.lists[m.tab].query() != "":
		m.lists[m.tab].clearFilter()
	case m.tab == historyTab && m.compareFrom != "":
		m.compareFrom = ""
		m.say("Comparison cancelled")
	}
	return nil
}

// Both screens are a left pane and a right pane: the list and its details,
// the request and its response. h and l move between them, z zooms the one
// with the keys, and Esc steps back out in the same order everywhere.
func (m *Model) rightFocused() bool {
	if m.screen == requestScreen {
		return m.respFocus
	}
	return m.detailFocus
}

func (m *Model) focusPane(right bool) {
	if m.screen == requestScreen {
		m.respFocus = right
		return
	}
	m.detailFocus = right && m.listLen() > 0
}

// paneNote is the top-edge note that names the other pane and its key: just
// the key beside a split, or the pane's name too when it is out of sight.
func (m *Model) paneNote(right, alone bool) string {
	id, name := actPaneLeft, [2]string{"List", "Request"}[m.screen]
	if !right {
		id, name = actPaneRight, [2]string{"Details", "Response"}[m.screen]
	}
	key := keyStyle.Render(m.keyName(id))
	if !alone {
		return key
	}
	return quiet.Render(name+" ") + key
}

func (m *Model) paste(text string) {
	if p, ok := m.modal.(interface{ paste(string) }); ok {
		p.paste(text)
		return
	}
	if in := m.activeInput(); in != nil {
		in.insert(text)
		m.inputChanged()
	}
}

// do runs an action in the current context.
func (m *Model) do(id action) tea.Cmd {
	switch id {
	case actQuit:
		return m.quit()
	case actHelp:
		m.modal = m.helpModal()
	case actPalette:
		m.modal = m.paletteModal()
	case actEnvironment:
		m.modal = m.envPicker()
	case actTab1, actTab2, actTab3, actTab4, actTab5:
		m.switchTab(map[action]tabID{actTab1: endpointsTab, actTab2: savedTab, actTab3: scenariosTab, actTab4: historyTab, actTab5: envTab}[id])
	case actNextTab:
		m.switchTab((m.tab + 1) % tabCount)
	case actPrevTab:
		m.switchTab((m.tab + tabCount - 1) % tabCount)
	case actUp, actDown, actTop, actBottom, actPageDown, actPageUp:
		m.navigate(id)
	case actFilter:
		m.lists[m.tab].startFilter()
	case actZoom:
		m.zoom[m.screen] = !m.zoom[m.screen]
	case actPaneLeft, actPaneRight:
		m.focusPane(id == actPaneRight)
	case actOpen:
		return m.openSelection()
	case actRenameProject:
		m.renameProject()
	case actValidateProject:
		if _, err := project.Load(m.root); err != nil {
			m.fail(err)
		} else {
			m.ok("Project files are valid")
		}
	case actReloadProject:
		m.reloadAll()
		if m.flash.kind != flashErr {
			m.ok("Project reloaded")
		}
	case actCancel:
		if m.busy != nil && m.busy.cancel != nil {
			return m.cancelBusy()
		}
		m.say("Nothing is running")
	default:
		for _, handler := range []func(action) (bool, tea.Cmd){m.endpointAction, m.caseAction, m.scenarioAction, m.historyAction, m.envAction, m.requestAction} {
			if handled, cmd := handler(id); handled {
				return cmd
			}
		}
	}
	return nil
}

// cancelBusy stops the running request or scenario. A scenario paused on a
// production question is waiting for an answer, so cancelling also collects
// its final report.
func (m *Model) cancelBusy() tea.Cmd {
	if m.busy == nil || m.busy.cancel == nil {
		return nil
	}
	m.busy.cancel()
	m.say("Cancelling…")
	if m.scenarioPaused {
		m.scenarioPaused = false
		m.modal = nil
		return m.awaitScenario()
	}
	return nil
}

// runCommand runs an action chosen from the command search, first moving to
// the screen where it applies.
func (m *Model) runCommand(id action) tea.Cmd {
	d := m.keys.def(id)
	tabs := map[scope]tabID{scopeEndpoints: endpointsTab, scopeSaved: savedTab, scopeScenarios: scenariosTab, scopeHistory: historyTab, scopeEnvs: envTab}
	here := contexts[m.context()]
	for _, s := range d.scopes {
		if slicesContains(here, s) {
			return m.do(id)
		}
	}
	switch s := d.scopes[0]; s {
	case scopeEndpoints, scopeSaved, scopeScenarios, scopeHistory, scopeEnvs:
		m.switchTab(tabs[s])
	case scopeRequest, scopeForm, scopeResponse:
		if m.req == nil {
			m.say("Open an endpoint or saved request first")
			return nil
		}
		m.screen = requestScreen
		m.respFocus = s == scopeResponse
	case scopeBrowse, scopeNav:
		m.screen = browseScreen
	}
	return m.do(id)
}

func slicesContains(list []scope, s scope) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (m *Model) switchTab(t tabID) {
	m.screen = browseScreen
	m.tab = t
	m.detailFocus, m.detailScroll = false, 0
	if t == historyTab {
		m.loadHistory()
	}
}

func (m *Model) quit() tea.Cmd {
	var dirty []string
	for _, d := range m.drafts {
		if d.dirty() {
			dirty = append(dirty, d.title())
		}
	}
	if len(dirty) == 0 {
		return tea.Quit
	}
	sort.Strings(dirty)
	lines := []string{"These saved requests have edits that are not saved:", ""}
	for _, name := range dirty {
		lines = append(lines, "  "+name)
	}
	m.modal = &confirmModal{title: "Quit without saving?", lines: lines, yes: "quit anyway", danger: true,
		onYes: func(*Model) tea.Cmd { return tea.Quit }}
	return nil
}

func (m *Model) View() tea.View {
	width, height := max(1, m.width), max(1, m.height)
	var lines []string
	switch {
	case m.credErr != nil && m.root != "":
		lines = m.recoveryView(width, height)
	case m.root == "":
		lines = block(nil, width, height)
	default:
		lines = m.workspaceView(width, height)
	}
	if m.modal != nil {
		lines = m.overlay(lines, width, height)
	}
	content := m.credentials.Sanitize(strings.Join(lines, "\n"))
	v := tea.NewView(content)
	v.AltScreen = true
	if m.root != "" && m.credErr == nil {
		v.WindowTitle = m.credentials.Sanitize(m.data.snapshot.Manifest.Name) + " — reqscope"
	}
	return v
}

func (m *Model) workspaceView(width, height int) []string {
	if width < 24 || height < 5 {
		return block([]string{clip("Enlarge the window", width)}, width, height)
	}
	footer := m.footer(width)
	bodyHeight := height - 1 - len(footer)
	var body []string
	if m.screen == requestScreen && m.req != nil {
		body = m.requestView(width, bodyHeight)
	} else {
		body = m.browseView(width, bodyHeight)
	}
	lines := []string{m.header(width)}
	lines = append(lines, block(body, width, bodyHeight)...)
	lines = append(lines, footer...)
	if m.leader != nil {
		box := m.whichKey(width-4, bodyHeight-2)
		if len(box) > 0 {
			// Inside the panes' bottom-right corner, clear of their frames.
			x := max(0, width-ansi.StringWidth(box[0])-2)
			lines = float(lines, box, x, max(1, bodyHeight-len(box)), width)
		}
	}
	return lines
}

// header names the project, the tabs with the key that reaches each, and the
// environment requests will go to. Production is the loudest thing here.
func (m *Model) header(width int) string {
	long, short := m.envBadge(false), m.envBadge(true)
	name := bold.Render(m.data.snapshot.Manifest.Name)
	tabs := func(compact bool) string {
		var parts []string
		for i := range tabCount {
			number := quiet.Render(strconv.Itoa(int(i) + 1))
			switch {
			case i == m.tab:
				parts = append(parts, tabPill.Render(strconv.Itoa(int(i)+1)+" "+tabNames[i]))
			case compact:
				parts = append(parts, number)
			default:
				parts = append(parts, number+" "+quiet.Render(tabNames[i]))
			}
		}
		return strings.Join(parts, "  ")
	}
	full, compact := tabs(false), tabs(true)
	// Full tab names matter more than the project name, which is also in
	// the window title; the environment is never dropped.
	for _, c := range [][2]string{{name + "   " + full, long}, {name + "   " + full, short}, {full, long}, {full, short}, {name + "   " + compact, long}, {compact, long}, {compact, short}} {
		if ansi.StringWidth(c[0])+ansi.StringWidth(c[1])+3 <= width {
			gap := width - ansi.StringWidth(c[0]) - ansi.StringWidth(c[1]) - 1
			return " " + c[0] + strings.Repeat(" ", gap) + c[1]
		}
	}
	room := width - ansi.StringWidth(short) - 2
	if room < 8 {
		return pad(" "+short, width)
	}
	return " " + pad(compact, room) + " " + short
}

func (m *Model) envBadge(short bool) string {
	e, ok := m.envByID(m.data.activeEnv)
	if !ok {
		if short || m.keys.key(actEnvironment) == "" {
			return warnStyle.Render("no environment")
		}
		return warnStyle.Render("no environment") + quiet.Render(" ("+m.keys.key(actEnvironment)+")")
	}
	if e.Classification == "production" {
		return prod.Render(e.Name)
	}
	return okStyle.Render("●") + " " + e.Name
}

// footer is the flash message, the running job, or the keys that work here.
func (m *Model) footer(width int) []string {
	if m.flash.text != "" {
		style, mark := quiet, ""
		switch m.flash.kind {
		case flashOK:
			style, mark = okStyle, "✓ "
		case flashErr:
			style, mark = badStyle, "✗ "
		}
		wrapped := strings.Split(ansi.Wrap(mark+m.flash.text, max(1, width-2), " /"), "\n")
		if len(wrapped) > 3 {
			wrapped = append(wrapped[:2], clip(strings.Join(wrapped[2:], " "), width-2))
		}
		for i := range wrapped {
			wrapped[i] = " " + style.Render(wrapped[i])
		}
		return wrapped
	}
	if m.leader != nil {
		return []string{pad(" "+keyStyle.Render(strings.Join(m.leader.path, " "))+quiet.Render(" …")+"   "+hint("esc", "close")+"   "+hint("backspace", "up"), width)}
	}
	if m.busy != nil {
		return []string{pad(" "+accent.Render("◌ ")+m.busy.label+"   "+hint("esc", "cancel"), width)}
	}
	right := ""
	hints := m.footerHints()
	if len(hints) == 0 {
		return []string{pad(" ", width-ansi.StringWidth(right)-1) + right + " "}
	}
	// Hints are in priority order; the first that does not fit ends the
	// line, and the keys hint always closes it.
	last := hints[len(hints)-1]
	room := width - ansi.StringWidth(right) - ansi.StringWidth(last) - 8
	line := ""
	for _, h := range hints[:len(hints)-1] {
		next := h
		if line != "" {
			next = line + "   " + h
		}
		if ansi.StringWidth(next) > room {
			break
		}
		line = next
	}
	if line != "" {
		line += "   "
	}
	line += last
	gap := width - ansi.StringWidth(line) - ansi.StringWidth(right) - 2
	if gap < 1 {
		return []string{pad(" "+line, width)}
	}
	return []string{" " + line + strings.Repeat(" ", gap) + right + " "}
}

func (m *Model) footerHints() []string {
	chain := contexts[m.context()]
	type item struct {
		order int
		text  string
	}
	var items []item
	seen := map[action]bool{}
	for _, s := range chain {
		for _, d := range m.keys.defs {
			if d.footer == 0 || seen[d.id] || len(d.keys) == 0 || !slicesContains(d.scopes, s) {
				continue
			}
			seen[d.id] = true
			label := d.label
			if d.id == actOpen {
				label = [tabCount]string{"open", "open", "edit", "view", "edit"}[m.tab]
			}

			if !m.hintApplies(d.id) {
				continue
			}
			text := hint(d.keys[0], label)
			if d.id == actHelp {
				// The key menu and the key list close the line together.
				text = hint(m.keys.leader, "menu") + "   " + text
			}
			items = append(items, item{d.footer, text})
		}
	}
	if m.screen == requestScreen {
		items = append(items, item{95, hint("esc", "back")})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].order < items[j].order })
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.text
	}
	return out
}

// hintApplies hides keys that cannot do anything right now, so the footer
// never advertises a dead end.
func (m *Model) hintApplies(id action) bool {
	empty := m.listLen() == 0
	switch id {
	case actOpen, actContract, actRunCase, actEditCase, actDuplicateCase, actDeleteCase,
		actRunScenario, actCheckScenario, actApproveLogins, actDeleteScenario,
		actPin, actCompare, actDeleteRun, actUseEnv, actCredentials, actAuthorize, actDeleteEnv:
		return !empty
	case actMethod, actGroup, actSchemas:
		return len(m.data.specs) > 0
	case actFilter:
		return !empty || m.lists[m.tab].query() != ""
	case actAttach, actBodyType:
		return false
	case actMethodCycle:
		return m.req != nil && m.req.request.EndpointID == ""
	case actRemove:
		row, ok := m.currentRow()
		return ok && (row.kind == rowValue || row.kind == rowBody && row.value != "")
	case actAdd:
		row, ok := m.currentRow()
		return ok && row.section != ""
	case actBody:
		return m.req != nil && m.req.request.BodyType != "form" && m.req.request.BodyType != "multipart"
	}
	return true
}

func (m *Model) position() string {
	if m.screen != browseScreen {
		return ""
	}
	n := m.listLen()
	if n == 0 {
		return ""
	}
	return quiet.Render(strconv.Itoa(m.lists[m.tab].cursorItem(m)+1) + "/" + strconv.Itoa(m.listCount()))
}

// overlay dims the screen and floats the modal in its centre.
func (m *Model) overlay(base []string, width, height int) []string {
	box := m.modalBox(width, height)
	if len(box) == 0 {
		return base
	}
	boxWidth := ansi.StringWidth(box[0])
	x := max(0, (width-boxWidth)/2)
	y := max(0, (height-len(box))/3)
	if c, ok := m.modal.(*confirmModal); ok && c.private {
		base = block(nil, width, len(base))
	}
	out := make([]string, len(base))
	for i, line := range base {
		plain := ansi.Strip(line)
		if i < y || i >= y+len(box) {
			out[i] = quiet.Faint(true).Render(plain)
			continue
		}
		left := ansi.Cut(plain, 0, x)
		right := ansi.Cut(plain, x+boxWidth, width)
		out[i] = quiet.Faint(true).Render(left) + box[i-y] + quiet.Faint(true).Render(right)
	}
	return out
}

// waitFor runs work off the UI goroutine while the footer says what is
// happening; the result is applied on the UI goroutine.
func (m *Model) waitFor(label string, work func() applyMsg) tea.Cmd {
	m.busy = &busy{label: label}
	return func() tea.Msg {
		apply := work()
		return applyMsg(func(m *Model) tea.Cmd {
			m.busy = nil
			return apply(m)
		})
	}
}
