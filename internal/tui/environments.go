package tui

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func (m *Model) envAction(id action) (bool, tea.Cmd) {
	switch id {
	case actNewEnv:
		e := project.Resource{Environment: project.Environment{ID: uuid.NewString(), Name: "", Classification: "nonproduction", BaseURLs: map[string]string{}, Variables: map[string]string{}}}
		m.editEnv(e, true)
		return true, nil
	case actUseEnv, actDeleteEnv, actAuthorize, actCredentials:
	default:
		return false, nil
	}
	e, ok := m.selectedEnv()
	if !ok {
		m.say("No environment selected")
		return true, nil
	}
	switch id {
	case actUseEnv:
		m.useEnv(e)
	case actDeleteEnv:
		m.modal = &confirmModal{title: "Delete " + e.Name + "?", yes: "delete", danger: true,
			lines: []string{"The environment is removed from the project.", quiet.Render("Its stored credentials stay on this machine; CLI access to it is revoked.")},
			onYes: func(m *Model) tea.Cmd {
				if _, err := project.DeleteEnvironment(m.root, e.ID, e.Revision, false); err != nil {
					m.fail(err)
					return nil
				}
				if m.data.activeEnv == e.ID {
					m.data.activeEnv = ""
				}
				m.sync()
				m.ok("Deleted " + e.Name)
				return nil
			}}
	case actAuthorize:
		m.authorize(e)
	case actCredentials:
		m.modal = m.credentialList(e, false)
	}
	return true, nil
}

func (m *Model) useEnv(e project.Resource) {
	if err := project.SelectEnvironment(m.root, e.ID); err != nil {
		m.fail(err)
		return
	}
	m.data.activeEnv = e.ID
	m.sync()
	if e.Classification == "production" {
		m.say("Requests now go to " + e.Name + ". Each send asks first.")
	} else {
		m.ok("Requests now go to " + e.Name)
	}
}

func (m *Model) authorize(e project.Resource) {
	r, err := project.ReviewAccess(m.root, e.ID)
	if err != nil {
		m.fail(err)
		return
	}
	lines := []string{"Reqscope's CLI will be allowed to send requests to:", ""}
	for _, id := range sortedKeys(r.BaseURLs) {
		lines = append(lines, "  "+pad(m.moduleName(id), 20)+" "+r.BaseURLs[id])
	}
	if len(r.BaseURLs) == 0 {
		lines = append(lines, quiet.Render("  no base URLs yet"))
	}
	lines = append(lines, "", bold.Render("Exact origins"))
	for _, id := range sortedKeys(r.Origins) {
		lines = append(lines, "  "+r.Origins[id])
	}
	if len(r.Bindings) > 0 {
		lines = append(lines, "", bold.Render("Protected credentials"))
		for _, name := range sortedKeys(r.Bindings) {
			lines = append(lines, "  "+name+quiet.Render("  "+r.Bindings[name]))
		}
	}
	lines = append(lines, "", quiet.Render("Changing these settings later revokes the approval. Review "+shortID(r.Fingerprint)))
	title := "Allow the CLI to use " + e.Name + "?"
	m.modal = &confirmModal{title: title, lines: lines, yes: "allow", danger: r.Classification == "production", onYes: func(m *Model) tea.Cmd {
		if err := project.ApproveAccess(m.root, r); err != nil {
			m.fail(err)
			return nil
		}
		m.ok("The CLI can now send requests to " + e.Name)
		return nil
	}}
}

// envEditor edits an environment in place: its name, whether it is
// production, a base URL per API, and variables.
type envEditor struct {
	original project.Resource
	env      project.Environment
	isNew    bool
	cursor   int
	editing  *textInput
	editKey  string
	err      string
	offset   int
}

func (m *Model) editEnv(e project.Resource, isNew bool) {
	working := e.Environment
	working.BaseURLs = map[string]string{}
	for k, v := range e.BaseURLs {
		working.BaseURLs[k] = v
	}
	working.Variables = map[string]string{}
	for k, v := range e.Variables {
		working.Variables[k] = v
	}
	ed := &envEditor{original: e, env: working, isNew: isNew}
	if isNew {
		ed.editing, ed.editKey = newInput("", "staging, qa, local…"), "name"
	}
	m.modal = ed
}

func (ed *envEditor) changed() bool {
	a, b := ed.original.Environment, ed.env
	if a.Name != b.Name || a.Classification != b.Classification || len(a.BaseURLs) != len(nonEmpty(b.BaseURLs)) || len(a.Variables) != len(b.Variables) {
		return true
	}
	for k, v := range a.BaseURLs {
		if b.BaseURLs[k] != v {
			return true
		}
	}
	for k, v := range a.Variables {
		if b.Variables[k] != v {
			return true
		}
	}
	return false
}

func nonEmpty(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

func (ed *envEditor) rows(m *Model) []formRow {
	production := "no"
	if ed.env.Classification == "production" {
		production = "yes, ask before every send"
	}
	rows := []formRow{
		{key: "name", label: "Name", value: ed.env.Name, ghost: "required", kind: rowValue},
		{key: "class", label: "Production", value: production, kind: rowValue},
	}
	for _, id := range m.envModules(ed.env) {
		rows = append(rows, formRow{key: "base:" + id, section: "Base URLs", label: m.moduleName(id), value: ed.env.BaseURLs[id], ghost: "not set", kind: rowValue})
	}
	rows = append(rows, formRow{key: "add:base", section: "Base URLs", label: "+ add", kind: rowAdd})
	for _, name := range sortedKeys(ed.env.Variables) {
		rows = append(rows, formRow{key: "var:" + name, section: "Variables", label: name, value: ed.env.Variables[name], kind: rowValue})
	}
	rows = append(rows, formRow{key: "add:var", section: "Variables", label: "+ add", kind: rowAdd})
	return rows
}

func (ed *envEditor) preferredWidth() int { return 84 }

func (ed *envEditor) paste(text string) {
	if ed.editing != nil {
		ed.editing.insert(text)
	}
}

func (ed *envEditor) update(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	rows := ed.rows(m)
	ed.cursor = max(0, min(ed.cursor, len(rows)-1))
	current := rows[ed.cursor]
	key := msg.String()
	if ed.editing != nil {
		switch key {
		case "enter":
			ed.commit(m, ed.editKey, ed.editing.String())
			ed.editing = nil
		case "esc":
			ed.editing = nil
		default:
			ed.editing.update(msg)
		}
		return nil
	}
	ed.err = ""
	switch key {
	case "j", "down", "tab":
		ed.cursor = min(len(rows)-1, ed.cursor+1)
	case "k", "up", "shift+tab":
		ed.cursor = max(0, ed.cursor-1)
	case "g", "home":
		ed.cursor = 0
	case "G", "end":
		ed.cursor = len(rows) - 1
	case "enter", "e":
		if current.key == "class" {
			if ed.env.Classification == "production" {
				ed.env.Classification = "nonproduction"
			} else {
				ed.env.Classification = "production"
			}
			return nil
		}
		placeholder := ""
		switch current.key {
		case "add:base":
			placeholder = "api=https://api.example.com"
		case "add:var":
			placeholder = "name=value"
		}
		if strings.HasPrefix(current.key, "base:") {
			placeholder = "https://…"
		}
		ed.editing, ed.editKey = newInput(current.value, placeholder), current.key
		if current.kind == rowAdd {
			ed.editing.set("")
		}
	case "a":
		target := "add:var"
		if current.section == "Base URLs" {
			target = "add:base"
		}
		for i, r := range rows {
			if r.key == target {
				ed.cursor = i
				ed.editing, ed.editKey = newInput("", map[string]string{"add:var": "name=value", "add:base": "api=https://api.example.com"}[target]), target
			}
		}
	case "d":
		kind, name, _ := strings.Cut(current.key, ":")
		switch kind {
		case "base":
			delete(ed.env.BaseURLs, name)
		case "var":
			delete(ed.env.Variables, name)
		}
	case "ctrl+s":
		return ed.save(m)
	case "esc":
		if ed.changed() || ed.isNew {
			m.modal = &confirmModal{title: "Discard changes?", yes: "discard", lines: []string{"Your changes to this environment are not saved."},
				onNo: func(m *Model) tea.Cmd { m.modal = ed; return nil }}
			return nil
		}
		m.modal = nil
	}
	return nil
}

func (ed *envEditor) commit(m *Model, key, value string) {
	kind, name, _ := strings.Cut(key, ":")
	switch kind {
	case "name":
		ed.env.Name = strings.TrimSpace(value)
	case "base":
		ed.env.BaseURLs[name] = strings.TrimSpace(value)
	case "var":
		ed.env.Variables[name] = value
	case "add":
		k, v := splitPair(value, "=")
		if k == "" {
			return
		}
		if name == "base" {
			// A module's display name maps back to its ID.
			for id, title := range m.data.modules {
				if title == k {
					k = id
				}
			}
			ed.env.BaseURLs[k] = v
		} else {
			ed.env.Variables[k] = v
		}
	}
}

func (ed *envEditor) save(m *Model) tea.Cmd {
	e := ed.env
	e.BaseURLs = nonEmpty(e.BaseURLs)
	if err := project.ValidateEnvironment(e); err != nil {
		ed.err = sentence(err.Error())
		return nil
	}
	revision := ed.original.Revision
	finish := func(m *Model, lower bool) tea.Cmd {
		if err := project.SaveEnvironment(m.root, e, revision, false, lower); err != nil {
			ed.err = sentence(err.Error())
			m.modal = ed
			return nil
		}
		m.modal = nil
		m.sync()
		m.ok("Saved " + e.Name)
		return nil
	}
	if err := project.SaveEnvironment(m.root, e, revision, true, false); err != nil {
		if pe, ok := err.(*project.Error); ok && pe.Code == "access_denied" {
			m.modal = &confirmModal{title: "Stop treating " + e.Name + " as production?", yes: "confirm", danger: true,
				lines: []string{"Requests will no longer ask before sending.", quiet.Render("Its CLI access approval is removed.")},
				onYes: func(m *Model) tea.Cmd { return finish(m, true) },
				onNo:  func(m *Model) tea.Cmd { m.modal = ed; return nil }}
			return nil
		}
		ed.err = sentence(err.Error())
		return nil
	}
	return finish(m, false)
}

func (ed *envEditor) content(m *Model, width, height int) (string, []string, []string) {
	rows := ed.rows(m)
	ed.cursor = max(0, min(ed.cursor, len(rows)-1))
	labelWidth := 10
	for _, r := range rows {
		if r.kind == rowValue {
			labelWidth = max(labelWidth, ansi.StringWidth(r.label))
		}
	}
	labelWidth = min(labelWidth, 24)
	var lines []string
	cursorLine := 0
	section := ""
	for i, r := range rows {
		if r.section != section {
			section = r.section
			lines = append(lines, "", " "+bold.Render(section))
		}
		if i == ed.cursor {
			cursorLine = len(lines)
		}
		selected := i == ed.cursor
		gutter := " "
		if selected {
			gutter = accent.Render("▌")
		}
		if r.kind == rowAdd {
			if selected && ed.editing != nil {
				lines = append(lines, gutter+" "+ed.editing.view(width-2, true))
				continue
			}
			lines = append(lines, gutter+row([]span{sp(" "+r.label, quiet)}, width-1, selected))
			continue
		}
		spans := []span{plain(" " + pad(r.label, labelWidth) + "  ")}
		if selected && ed.editing != nil {
			lines = append(lines, gutter+row(spans, labelWidth+3, true)+ed.editing.view(width-labelWidth-4, true))
			continue
		}
		switch {
		case r.key == "class" && ed.env.Classification == "production":
			spans = append(spans, sp(r.value, delStyle))
		case r.value != "":
			spans = append(spans, plain(r.value))
		default:
			style := quiet
			if strings.HasPrefix(r.key, "base:") {
				style = warnStyle
			}
			spans = append(spans, sp(r.ghost, style))
		}
		lines = append(lines, gutter+row(spans, width-1, selected))
	}
	if ed.err != "" {
		lines = append(lines, "")
		for _, line := range wrap(ed.err, width) {
			lines = append(lines, badStyle.Render(line))
		}
	}
	if cursorLine < ed.offset {
		ed.offset = cursorLine
	}
	if cursorLine >= ed.offset+height {
		ed.offset = cursorLine - height + 1
	}
	ed.offset = max(0, min(ed.offset, len(lines)-height))
	lines = lines[ed.offset:]
	title := "Edit " + ed.original.Name
	if ed.isNew {
		title = "New environment"
	}
	hints := []string{hint("enter", "edit"), hint("a", "add"), hint("d", "remove"), hint("ctrl+s", "save"), hint("esc", "close")}
	if ed.editing != nil {
		hints = []string{hint("enter", "done"), hint("esc", "cancel edit")}
	} else if rows[ed.cursor].key == "class" {
		hints[0] = hint("enter", "toggle")
	}
	return title, lines, hints
}

// credentialList shows which credentials an environment holds, never
// their values.
type credentialList struct {
	env      project.Resource
	states   []project.SecretState
	cursor   int
	err      string
	recovery bool
}

func (m *Model) credentialList(e project.Resource, recovery bool) modal {
	states, err := m.credentials.Inspect(m.root, e.ID)
	c := &credentialList{env: e, states: states, recovery: recovery}
	if err != nil {
		c.err = sentence(err.Error())
	}
	sort.Slice(c.states, func(i, j int) bool { return c.states[i].Name < c.states[j].Name })
	return c
}

func (c *credentialList) preferredWidth() int { return 64 }

func (c *credentialList) update(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.modal = nil
	case "j", "down":
		c.cursor = min(c.cursor+1, max(0, len(c.states)-1))
	case "k", "up":
		c.cursor = max(0, c.cursor-1)
	case "n":
		m.modal = m.credentialForm(c.env, "", c.recovery)
	case "enter":
		if len(c.states) > 0 {
			m.modal = m.credentialForm(c.env, c.states[c.cursor].Name, c.recovery)
		}
	case "d":
		if len(c.states) == 0 {
			return nil
		}
		name := c.states[c.cursor].Name
		m.modal = &confirmModal{title: "Delete " + name + "?", yes: "delete", danger: true,
			lines: []string{"The stored value is erased from this machine.", quiet.Render("The CLI loses access to this environment until you allow it again.")},
			onYes: func(m *Model) tea.Cmd {
				if err := m.credentials.Delete(m.root, c.env.ID, name); err != nil {
					m.fail(err)
					return nil
				}
				m.sync()
				m.modal = m.credentialList(c.env, c.recovery)
				m.ok("Deleted " + name)
				return nil
			},
			onNo: func(m *Model) tea.Cmd { m.modal = c; return nil }}
	}
	return nil
}

func (c *credentialList) content(m *Model, width, height int) (string, []string, []string) {
	body := []string{quiet.Render("Values are kept in your system keychain and are never shown."), ""}
	if len(c.states) == 0 {
		body = append(body, "No credentials yet.")
	}
	for i, s := range c.states {
		state := sp("stored", okStyle)
		if !s.Configured {
			state = sp("missing", warnStyle)
		}
		body = append(body, row([]span{plain(pad(s.Name, width-10)), state}, width, i == c.cursor))
	}
	if c.err != "" {
		body = append(body, "", badStyle.Render(c.err))
	}
	name := c.env.Name
	if c.recovery {
		name = shortID(c.env.ID)
	}
	hints := []string{hint("n", "add")}
	if len(c.states) > 0 {
		hints = append(hints, hint("enter", "replace"), hint("d", "delete"))
	}
	return "Credentials for " + name, body, append(hints, hint("esc", "close"))
}

// credentialForm adds or replaces one credential. In recovery it shows no
// project text, since unreadable secrets cannot be redacted from it.
func (m *Model) credentialForm(e project.Resource, name string, recovery bool) modal {
	kinds := []string{"bearer", "api-key", "basic"}
	module := &formField{label: "API", input: newInput("", "module name")}
	if len(m.data.moduleList) > 0 && !recovery {
		module = &formField{label: "API", choices: m.data.moduleList}
		for _, id := range m.data.moduleList {
			module.names = append(module.names, m.moduleName(id))
		}
	}
	destination := ""
	if !recovery && len(m.data.moduleList) > 0 {
		if base := e.BaseURLs[m.data.moduleList[0]]; base != "" {
			destination = strings.TrimRight(base, "/") + "/"
		}
	}
	nameField := &formField{label: "Name", input: newInput(name, "login"), note: "Requests refer to it as {{secret:name}}"}
	value := &formField{label: "Value", input: newInput("", ""), note: "For basic, type username:password"}
	value.input.mask = true
	f := &formModal{title: "Credential for " + map[bool]string{false: e.Name, true: shortID(e.ID)}[recovery], label: "save",
		note: []string{"Stored in your system keychain. Existing values are never shown; saving replaces them."},
		fields: []*formField{
			nameField,
			{label: "Type", choices: kinds, names: []string{"bearer token", "API key", "basic auth"}},
			module,
			{label: "Sent to", input: newInput(destination, "https://api.example.com/"), note: "The exact URL prefix the credential may go to"},
			{label: "Placed in", choices: []string{"header", "query", "body"}},
			{label: "Field", input: newInput("Authorization", "Authorization"), note: "Header or parameter name; leave empty for a body"},
			value,
		}}
	f.cancel = func(m *Model) tea.Cmd { m.modal = m.credentialList(e, recovery); return nil }
	f.submit = func(m *Model, f *formModal) (tea.Cmd, error) {
		b := project.CredentialBinding{Name: strings.TrimSpace(f.fields[0].value()), Kind: f.fields[1].value(), Module: strings.TrimSpace(f.fields[2].value()), Destination: strings.TrimSpace(f.fields[3].value()), In: f.fields[4].value(), Field: strings.TrimSpace(f.fields[5].value())}
		secret := f.fields[6].value()
		done := func(m *Model) {
			m.sync()
			if recovery {
				m.credErr = m.credentials.Refresh(m.root)
			}
			m.modal = m.credentialList(e, recovery)
			m.ok("Saved " + b.Name + ". Allow CLI access again if the CLI needs it.")
		}
		err := m.credentials.Save(m.root, e.ID, b, secret, false)
		if pe, ok := err.(*project.Error); ok && pe.Code == "credential_unavailable" {
			m.modal = &confirmModal{title: "Keychain unavailable", yes: "keep for this session",
				lines: []string{"Your system keychain is locked or unavailable.", "Keep this credential in memory until Reqscope quits?"},
				onYes: func(m *Model) tea.Cmd {
					if err := m.credentials.Save(m.root, e.ID, b, secret, true); err != nil {
						m.fail(err)
						return nil
					}
					done(m)
					return nil
				},
				onNo: func(m *Model) tea.Cmd { m.say("Credential not saved"); return nil }}
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		done(m)
		return nil, nil
	}
	return f
}

// recoveryView is shown while stored credentials cannot be read. It shows
// environment IDs only: any other project text could hold a secret that,
// unread, cannot be redacted.
func (m *Model) recoveryView(width, height int) []string {
	lines := []string{
		"",
		" " + badStyle.Bold(true).Render("Reqscope cannot read your stored credentials."),
		" " + quiet.Render("Unlock your system keychain and press r to retry,"),
		" " + quiet.Render("or replace a credential for this session only."),
		"",
		" " + bold.Render("Environments"),
	}
	envs := m.data.snapshot.Environments
	l := &m.lists[envTab]
	l.cursor = max(0, min(l.cursor, len(envs)-1))
	for i, e := range envs {
		mark := "  "
		if e.ID == m.data.activeEnv {
			mark = "● "
		}
		gutter := " "
		if i == l.cursor {
			gutter = accent.Render("▌")
		}
		lines = append(lines, gutter+row([]span{plain(mark + e.ID)}, width-1, i == l.cursor))
	}
	active := "none"
	if m.data.activeEnv != "" {
		active = m.data.activeEnv
	}
	lines = append(lines, "", " "+quiet.Render("Requests go to ")+active)
	lines = block(lines, width, height-1)
	return append(lines, " "+strings.Join([]string{hint("j/k", "move"), hint("u", "use"), hint("c", "credentials"), hint("r", "retry"), hint("q", "quit")}, "   "))
}

func (m *Model) recoveryKey(msg tea.KeyPressMsg) tea.Cmd {
	envs := m.data.snapshot.Environments
	l := &m.lists[envTab]
	switch msg.String() {
	case "q":
		return tea.Quit
	case "j", "down":
		l.cursor = min(l.cursor+1, max(0, len(envs)-1))
	case "k", "up":
		l.cursor = max(0, l.cursor-1)
	case "g":
		l.cursor = 0
	case "G":
		l.cursor = max(0, len(envs)-1)
	case "u", "enter":
		if len(envs) > 0 {
			e := envs[l.cursor]
			if err := project.SelectEnvironment(m.root, e.ID); err != nil {
				m.fail(err)
				return nil
			}
			m.data.activeEnv = e.ID
		}
	case "c":
		if len(envs) > 0 {
			m.modal = m.credentialList(envs[l.cursor], true)
		}
	case "r":
		m.credErr = m.credentials.Refresh(m.root)
		if m.credErr == nil {
			m.reloadAll()
			m.ok("Credentials readable again")
		}
	}
	return nil
}
