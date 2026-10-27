package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

// draft is one request being edited, with the last response it produced.
// Drafts live for the session, one per endpoint or saved request, so moving
// between endpoints never throws away what was typed.
type draft struct {
	key      string
	request  project.RequestDraft
	guided   []project.GuidedField
	caseRes  *project.CaseResource
	baseline string
	label    string

	cursor  int
	offset  int
	editing *textInput
	editKey string

	exchange *project.Exchange
	body     []string
	scroll   int
	find     *textInput
	finding  bool
	headers  bool

	// conflict is the version on disk when it changed under unsaved edits.
	conflict *project.CaseResource
	deleted  bool
	// stash keeps the raw body while the body type is a form, and
	// stashForm the form fields while it is not, so cycling types loses nothing.
	stash     string
	stashForm []project.FormField
}

func encodeDraft(d project.RequestDraft) string {
	b, _ := json.Marshal(d)
	return string(b)
}

func newDraft(key string, r project.RequestDraft) *draft {
	if r.Path == nil {
		r.Path = map[string]string{}
	}
	return &draft{key: key, request: r, baseline: encodeDraft(r)}
}

func (d *draft) title() string {
	if d.caseRes != nil {
		return d.caseRes.Name
	}
	return d.label
}

// dirty is true for saved requests with edits not yet written. Scratch and
// endpoint drafts are kept for the session and never block leaving.
func (d *draft) dirty() bool {
	return d.caseRes != nil && encodeDraft(d.request) != d.baseline
}

func (d *draft) setExchange(ex project.Exchange) {
	d.exchange = &ex
	d.body = bodyLines(ex)
	d.scroll = 0
}

type rowKind int

const (
	rowURL rowKind = iota
	rowModule
	rowValue
	rowGhost
	rowAdd
	rowBody
)

type formRow struct {
	key, section, label, value, ghost, note string
	kind                                    rowKind
	required                                bool
}

var templateParam = regexp.MustCompile(`\{\{[^{}]*\}\}|\{([^{}]+)\}`)

// pathParams lists {name} placeholders, skipping {{variables}}.
func pathParams(path string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range templateParam.FindAllStringSubmatch(path, -1) {
		if m[1] != "" && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

func (d *draft) guidedFor(in string) []project.GuidedField {
	var out []project.GuidedField
	for _, g := range d.guided {
		if g.In == in {
			out = append(out, g)
		}
	}
	return out
}

func (d *draft) suggestion(in, name string) (project.GuidedField, bool) {
	for _, g := range d.guided {
		if g.In == in && g.Name == name {
			return g, true
		}
	}
	return project.GuidedField{}, false
}

func ghostText(g project.GuidedField) string {
	if ex := exampleText(g.Suggestion); ex != "" {
		return "e.g. " + ex
	}
	if g.Description != "" {
		return g.Description
	}
	if g.Required {
		return ""
	}
	return "optional"
}

func bodyLabel(kind string) string {
	switch kind {
	case "json":
		return "JSON"
	case "form":
		return "form"
	case "multipart":
		return "multipart form"
	}
	return "raw"
}

// rows lays the request out as the sections a person fills in.
func (m *Model) rows(d *draft) []formRow {
	r := d.request
	rows := []formRow{{key: "url", kind: rowURL, label: r.Method, value: r.URL}}
	if r.EndpointID == "" && len(m.moduleChoices()) > 0 {
		name := "none, use a full URL"
		if r.Module != "" {
			name = m.moduleName(r.Module)
		}
		rows = append(rows, formRow{key: "module", kind: rowModule, label: "API", value: name})
	}
	for _, name := range pathParams(r.URL) {
		row := formRow{key: "path:" + name, section: "Path", label: name, value: r.Path[name], kind: rowValue, required: true}
		if g, ok := d.suggestion("path", name); ok {
			row.ghost = ghostText(g)
		}
		rows = append(rows, row)
	}
	pairs := func(section, in, prefix string, fields []project.RequestField) {
		present := map[string]bool{}
		for i, f := range fields {
			present[strings.ToLower(f.Name)] = true
			rows = append(rows, formRow{key: prefix + ":" + strconv.Itoa(i), section: section, label: f.Name, value: f.Value, kind: rowValue})
		}
		for _, g := range d.guidedFor(in) {
			if !present[strings.ToLower(g.Name)] {
				rows = append(rows, formRow{key: "g" + prefix + ":" + g.Name, section: section, label: g.Name, ghost: ghostText(g), kind: rowGhost, required: g.Required})
			}
		}
		rows = append(rows, formRow{key: "add:" + prefix, section: section, label: "+ add", kind: rowAdd})
	}
	pairs("Query", "query", "query", r.Query)
	pairs("Headers", "header", "header", r.Headers)
	switch r.BodyType {
	case "form", "multipart":
		rows = append(rows, formRow{key: "body", section: "Body", label: bodyLabel(r.BodyType), kind: rowBody})
		present := map[string]bool{}
		for i, f := range r.Form {
			present[f.Name] = true
			value := f.Value
			if f.File != "" {
				value = "@" + m.displayPath(f.File)
			}
			rows = append(rows, formRow{key: "form:" + strconv.Itoa(i), section: "Body", label: f.Name, value: value, kind: rowValue})
		}
		for _, g := range d.guided {
			if (g.In == "form" || g.In == "file") && !present[g.Name] {
				ghost := ghostText(g)
				if g.In == "file" {
					ghost = "a file — press " + "f" + " to attach"
				}
				rows = append(rows, formRow{key: "g" + g.In + ":" + g.Name, section: "Body", label: g.Name, ghost: ghost, kind: rowGhost, required: g.Required})
			}
		}
		rows = append(rows, formRow{key: "add:form", section: "Body", label: "+ add", kind: rowAdd})
	default:
		value := ""
		switch {
		case r.BodyFile != "":
			value = "@" + m.displayPath(r.BodyFile)
		case r.Body == "":
			value = ""
		default:
			n := strings.Count(strings.TrimRight(r.Body, "\n"), "\n") + 1
			value = fmt.Sprintf("%d line%s", n, plural(n, "", "s"))
		}
		row := formRow{key: "body", section: "Body", label: bodyLabel(r.BodyType), value: value, kind: rowBody}
		if g, ok := d.suggestion("body", "body"); ok {
			row.required = g.Required
			if g.Suggestion != nil {
				row.ghost = "press " + m.keyName(actBody) + " to use the example"
			}
		}
		rows = append(rows, row)
	}
	for _, name := range sortedKeys(r.Variables) {
		rows = append(rows, formRow{key: "var:" + name, section: "Variables", label: name, value: r.Variables[name], kind: rowValue})
	}
	rows = append(rows, formRow{key: "add:var", section: "Variables", label: "+ add", kind: rowAdd})
	return rows
}

func (m *Model) currentRow() (formRow, bool) {
	if m.req == nil {
		return formRow{}, false
	}
	rows := m.rows(m.req)
	if len(rows) == 0 {
		return formRow{}, false
	}
	m.req.cursor = max(0, min(m.req.cursor, len(rows)-1))
	return rows[m.req.cursor], true
}

func (d *draft) moveCursor(id action, page int) {
	switch id {
	case actUp:
		d.cursor--
	case actDown:
		d.cursor++
	case actTop:
		d.cursor = 0
	case actBottom:
		d.cursor = 1 << 20
	case actPageDown:
		d.cursor += page
	case actPageUp:
		d.cursor -= page
	}
	d.cursor = max(0, d.cursor)
}

func (d *draft) scrollResponse(id action, page int) {
	switch id {
	case actUp:
		d.scroll--
	case actDown:
		d.scroll++
	case actTop:
		d.scroll = 0
	case actBottom:
		d.scroll = 1 << 20
	case actPageDown:
		d.scroll += page
	case actPageUp:
		d.scroll -= page
	}
	d.scroll = max(0, d.scroll)
}

// destination shows where the request will go, worked out from loaded data
// alone. The URL is the one thing on this screen that is never cut short.
func (m *Model) destination(d *draft) (string, bool) {
	r := d.request
	if r.Disconnected {
		return "This endpoint was removed from the contract. Press " + m.keyName(actEditRaw) + " to relink or detach it.", false
	}
	e, ok := m.envByID(m.data.activeEnv)
	if !ok {
		return "Choose an environment to see where this goes (" + m.keyName(actEnvironment) + ")", false
	}
	vars := map[string]string{}
	for k, v := range m.data.snapshot.Manifest.Variables {
		vars[k] = v
	}
	for k, v := range e.Variables {
		vars[k] = v
	}
	for k, v := range r.Variables {
		vars[k] = v
	}
	expand := func(s string) string {
		return templateParam.ReplaceAllStringFunc(s, func(t string) string {
			if strings.HasPrefix(t, "{{") {
				name := strings.TrimSpace(t[2 : len(t)-2])
				if v, ok := vars[name]; ok && !strings.HasPrefix(name, "secret:") {
					return v
				}
				return t
			}
			if v := r.Path[t[1:len(t)-1]]; v != "" {
				return url.PathEscape(v)
			}
			return t
		})
	}
	target := expand(r.URL)
	if !isURL(target) {
		base := ""
		if m.data.reviewEnv == e.ID {
			base = m.data.review.BaseURLs[r.Module]
		}
		if base == "" {
			base = e.BaseURLs[r.Module]
		}
		if base == "" {
			name := m.moduleName(r.Module)
			if r.Module == "" {
				return "Choose an API above, or type a full URL", false
			}
			return "No base URL for " + name + " in " + e.Name + ". Add one in Environments (" + m.keyName(actTab5) + ").", false
		}
		target = strings.TrimRight(expand(base), "/") + "/" + strings.TrimLeft(target, "/")
	}
	// Like history and confirmations, the preview leaves query values and
	// credentials out; query values are listed in the Query section below.
	// It is cut by hand rather than re-encoded so unfilled {params} read as
	// they were written.
	if u, err := url.Parse(target); err == nil && u.User != nil {
		target = strings.Replace(target, u.User.String()+"@", "", 1)
	}
	if i := strings.IndexAny(target, "?#"); i >= 0 {
		target = target[:i]
	}
	return target, true
}

func (m *Model) requestView(width, height int) []string {
	d := m.req
	crumb := quiet.Render("‹ "+tabNames[m.tab]+"  ") + bold.Render(d.title())
	switch {
	case d.deleted:
		crumb += warnStyle.Render("  deleted on disk")
	case d.conflict != nil:
		crumb += warnStyle.Render("  changed on disk")
	case d.dirty():
		crumb += warnStyle.Render("  edited")
	}
	lines := []string{" " + clip(crumb, width-2)}
	height--
	zoomed := m.zoom[requestScreen]
	if width >= 100 && !zoomed {
		left := width * 52 / 100
		right := width - left
		reqNote, respNote := "", m.paneNote(false, false)
		if m.respFocus {
			reqNote, respNote = m.paneNote(true, false), ""
		}
		form := frame(m.formLines(d, left-4, height-2, !m.respFocus), "Request", reqNote, left, height, !m.respFocus)
		body, foot := m.responseLines(d, right-4, height-2, m.respFocus)
		resp := framed(body, "Response", respNote, foot, right, height, m.respFocus)
		return append(lines, join(height, form, resp)...)
	}
	note := m.paneNote(m.respFocus, true)
	if zoomed {
		note = quiet.Render("zoomed  ") + note
	}
	if m.respFocus {
		body, foot := m.responseLines(d, width-4, height-2, true)
		return append(lines, framed(body, "Response", note, foot, width, height, true)...)
	}
	return append(lines, frame(m.formLines(d, width-4, height-2, true), "Request", note, width, height, true)...)
}

func (m *Model) formLines(d *draft, width, height int, focused bool) []string {
	rows := m.rows(d)
	d.cursor = max(0, min(d.cursor, len(rows)-1))
	labelWidth := 6
	for _, r := range rows {
		if r.kind == rowValue || r.kind == rowGhost {
			labelWidth = max(labelWidth, ansi.StringWidth(r.label))
		}
	}
	labelWidth = min(labelWidth, max(8, width/3))
	var lines []string
	cursorLine := 0
	section := ""
	for i, r := range rows {
		if r.section != section {
			section = r.section
			lines = append(lines, "")
			lines = append(lines, heading(section, width))
		}
		selected := focused && i == d.cursor
		if i == d.cursor {
			cursorLine = len(lines)
		}
		lines = append(lines, m.formRowLine(d, r, i == d.cursor, selected, labelWidth, width))
		if r.kind == rowURL {
			dest, ok := m.destination(d)
			for _, line := range wrap(dest, width-2) {
				if ok {
					// Parameters still to fill stand out in the address.
					lines = append(lines, "  "+styledPath(line, quiet))
				} else {
					lines = append(lines, "  "+warnStyle.Render(line))
				}
			}
		}
		if r.kind == rowBody && d.request.BodyType != "form" && d.request.BodyType != "multipart" {
			preview := bodyPreview(d.request, width-4)
			limit := max(3, height/3)
			for j, line := range preview {
				if j >= limit {
					lines = append(lines, "    "+quiet.Render(fmt.Sprintf("… %d more lines", len(preview)-limit)))
					break
				}
				lines = append(lines, "    "+line)
			}
		}
	}
	if len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
		cursorLine--
	}
	margin := 2
	if cursorLine < d.offset+margin {
		d.offset = cursorLine - margin
	}
	if cursorLine >= d.offset+height-margin {
		d.offset = cursorLine - height + margin + 1
	}
	d.offset = max(0, min(d.offset, len(lines)-height))
	if d.offset > 0 {
		lines = lines[d.offset:]
	}
	return lines
}

func (m *Model) formRowLine(d *draft, r formRow, current, selected bool, labelWidth, width int) string {
	gutter := " "
	if selected {
		gutter = accent.Render("▌")
	}
	editing := current && d.editing != nil
	switch r.kind {
	case rowURL:
		method := sp(pad(r.label, 7), methodStyle(r.label))
		if editing {
			return gutter + row([]span{method}, 8, selected) + d.editing.view(width-9, true)
		}
		spans := []span{method, plain(" ")}
		if r.value == "" {
			spans = append(spans, sp("enter a path or full URL", quiet))
		} else {
			spans = append(spans, pathSpans(r.value)...)
		}
		return gutter + row(spans, width-1, selected)
	case rowAdd:
		label := quiet
		if selected {
			label = lipglossPlain
		}
		if editing {
			return gutter + " " + d.editing.view(width-2, true)
		}
		return gutter + row([]span{sp(" "+r.label, label)}, width-1, selected)
	case rowBody:
		spans := []span{plain(" "), sp(r.label, accent)}
		if r.value != "" {
			spans = append(spans, plain("  "), sp(r.value, quiet))
		} else if d.request.BodyType != "form" && d.request.BodyType != "multipart" {
			spans = append(spans, plain("  "), sp("empty", quiet))
		}
		if r.required && r.value == "" {
			spans = append(spans, plain("  "), sp("required", accent))
		}
		if r.ghost != "" && r.value == "" {
			spans = append(spans, plain("  "), sp(r.ghost, quiet))
		}
		return gutter + row(spans, width-1, selected)
	}
	labelStyle := lipglossPlain
	if r.kind == rowGhost {
		labelStyle = quiet
	}
	spans := []span{plain(" "), sp(pad(r.label, labelWidth), labelStyle), plain("  ")}
	valueWidth := width - labelWidth - 4
	if editing {
		return gutter + row(spans, labelWidth+3, selected) + d.editing.view(valueWidth, true)
	}
	switch {
	case r.value != "":
		spans = append(spans, plain(r.value))
	case r.ghost != "":
		spans = append(spans, sp(r.ghost, quiet), plain("  "))
	}
	if r.required && r.value == "" {
		spans = append(spans, sp("required", accent))
	}
	return gutter + row(spans, width-1, selected)
}

// displayPath shortens a file path for display: relative inside the
// project, from ~ inside the home folder. The filename is what matters.
func (m *Model) displayPath(path string) string {
	if root, err := filepath.Abs(m.root); err == nil {
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
			return "~/" + rel
		}
	}
	return path
}

// bodyPreview pretty-prints a JSON body for display; the draft keeps the
// exact text the person wrote.
func bodyPreview(r project.RequestDraft, width int) []string {
	if r.BodyFile != "" || r.Body == "" {
		return nil
	}
	text := r.Body
	var out bytes.Buffer
	if json.Indent(&out, []byte(text), "", "  ") == nil {
		text = out.String()
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, clip(jsonLine(line), width))
	}
	return lines
}

var jsonKey = regexp.MustCompile(`^(\s*)("(?:[^"\\]|\\.)*")(:)(.*)$`)

// jsonLine colours object keys; values keep the default colour.
func jsonLine(line string) string {
	if m := jsonKey.FindStringSubmatch(line); m != nil {
		return m[1] + accent.Render(m[2]) + quiet.Render(m[3]) + m[4]
	}
	return line
}

func statusText(code int) string {
	if text := http.StatusText(code); text != "" {
		return strconv.Itoa(code) + " " + text
	}
	return strconv.Itoa(code)
}

func bodyLines(ex project.Exchange) []string {
	if ex.Body == "" {
		return nil
	}
	text := ex.Body
	var out bytes.Buffer
	if json.Indent(&out, []byte(text), "", "  ") == nil {
		text = out.String()
	}
	return strings.Split(strings.TrimRight(text, "\n"), "\n")
}

// responseLines is the response as it fits the pane, and the body's scroll
// position for the frame.
func (m *Model) responseLines(d *draft, width, height int, focused bool) ([]string, string) {
	ex := d.exchange
	if ex == nil {
		if m.busy != nil {
			return []string{"", quiet.Render("Waiting for the response…")}, ""
		}
		return []string{"", quiet.Render("Nothing sent yet."), "", hint(m.keyName(actSend), "send this request")}, ""
	}
	var head []string
	if ex.Status != 0 {
		head = append(head, statusBadge(ex.Status, statusText(ex.Status))+"  "+quiet.Render(fmt.Sprintf("%d ms   %s", ex.DurationMS, humanBytes(ex.RetainedBytes))))
	} else {
		head = append(head, badBadge.Render("no response"))
	}
	if ex.Method != "" {
		head = append(head, quiet.Render(clip(ex.Method+" "+ex.Destination, width)))
	}
	if ex.CaseID != "" && !ex.Draft {
		head = append(head, quiet.Render("Sent as saved"))
	} else if ex.CaseID != "" {
		head = append(head, warnStyle.Render("Sent with unsaved edits"))
	}
	for _, c := range ex.Checks {
		mark, style := "✓", okStyle
		switch c.Outcome {
		case "failed":
			mark, style = "✗", badStyle
		case "passed":
		default:
			mark, style = "•", quiet
		}
		text := c.Kind + " " + c.Outcome
		if c.Message != "" {
			text += "  " + c.Message
		}
		head = append(head, style.Render(mark+" ")+clip(text, width-2))
	}
	if len(ex.Headers) > 0 {
		if d.headers {
			head = append(head, "", heading("Headers", width))
			for _, name := range sortedKeys(ex.Headers) {
				for _, v := range ex.Headers[name] {
					head = append(head, clip(accent.Render(name)+quiet.Render(": ")+v, width))
				}
			}
		} else {
			head = append(head, quiet.Render(fmt.Sprintf("%d headers  ", len(ex.Headers)))+hint(m.keyName(actHeaders), "show"))
		}
	}
	head = append(head, "", heading("Body", width))
	switch {
	case ex.BodyWithheld:
		head = append(head, warnStyle.Render("Body withheld: it held authentication data that could not be safely redacted."))
	case ex.Binary:
		head = append(head, quiet.Render("Binary body; nothing to show or copy."))
	case len(d.body) == 0:
		head = append(head, quiet.Render("Empty body"))
	}
	body := d.body
	if d.find != nil {
		q := strings.ToLower(d.find.String())
		var hits []string
		for _, line := range body {
			if q == "" || strings.Contains(strings.ToLower(line), q) {
				hits = append(hits, line)
			}
		}
		body = hits
		findLine := accent.Render("/ ") + d.find.view(width-20, d.finding) + "  " + quiet.Render(fmt.Sprintf("%d line%s", len(hits), plural(len(hits), "", "s")))
		head = append([]string{findLine}, head...)
	}
	if ex.Truncated {
		body = append(append([]string(nil), body...), "", warnStyle.Render("Truncated: the rest of the body was not kept, so checks on it are incomplete."))
	}
	room := max(1, height-len(head))
	d.scroll = max(0, min(d.scroll, len(body)-room))
	lines := head
	for i := d.scroll; i < len(body) && len(lines) < height; i++ {
		lines = append(lines, clip(jsonLine(body[i]), width))
	}
	return lines, scrollPos(d.scroll, room, len(body))
}

// inputKey feeds keys to whichever text field is open on the screen.
func (m *Model) inputKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	key := msg.String()
	if m.screen == browseScreen {
		l := &m.lists[m.tab]
		if !l.filtering {
			return false, nil
		}
		switch key {
		case "enter":
			l.filtering = false
			if l.query() == "" {
				l.clearFilter()
			}
		case "esc":
			l.clearFilter()
		case "up", "ctrl+p":
			m.navigate(actUp)
		case "down", "ctrl+n":
			m.navigate(actDown)
		default:
			if l.filter.update(msg) {
				m.inputChanged()
			}
		}
		return true, nil
	}
	d := m.req
	if d == nil {
		return false, nil
	}
	if d.editing != nil {
		switch key {
		case "enter":
			value := d.editing.String()
			d.editing = nil
			m.commitEdit(d, d.editKey, value)
		case "esc":
			d.editing = nil
		default:
			d.editing.update(msg)
		}
		return true, nil
	}
	if d.finding {
		switch key {
		case "enter":
			d.finding = false
			if d.find.String() == "" {
				d.find = nil
			}
		case "esc":
			d.find, d.finding = nil, false
		default:
			if d.find.update(msg) {
				d.scroll = 0
			}
		}
		return true, nil
	}
	if key == "esc" && d.find != nil {
		d.find = nil
		return true, nil
	}
	return false, nil
}

func (m *Model) activeInput() *textInput {
	if m.screen == browseScreen {
		if l := &m.lists[m.tab]; l.filtering {
			return l.filter
		}
		return nil
	}
	if d := m.req; d != nil {
		if d.editing != nil {
			return d.editing
		}
		if d.finding {
			return d.find
		}
	}
	return nil
}

func (m *Model) inputChanged() {
	if m.screen == browseScreen {
		l := &m.lists[m.tab]
		l.cursor, l.offset = 0, 0
		m.skipHeader(1)
	} else if m.req != nil && m.req.finding {
		m.req.scroll = 0
	}
}

func (m *Model) requestAction(id action) (bool, tea.Cmd) {
	if m.req == nil {
		switch id {
		case actSend, actSave, actEditRaw, actEdit, actAdd, actRemove, actMethodCycle,
			actBody, actBodyType, actAttach, actFind, actCopy, actHeaders, actClearRequest, actSaveAsNew, actReloadCase:
			m.say("Open an endpoint or saved request first")
			return true, nil
		}
		return false, nil
	}
	d := m.req
	switch id {
	case actSend:
		return true, m.send()
	case actSave:
		return true, m.save(d, false)
	case actSaveAsNew:
		return true, m.save(d, true)
	case actEditRaw:
		return true, m.editRaw(d)
	case actEdit:
		return true, m.editRow(d)
	case actAdd:
		row, _ := m.currentRow()
		prefix := map[string]string{"Query": "query", "Headers": "header", "Body": "form", "Variables": "var"}[row.section]
		if prefix == "" || prefix == "form" && d.request.BodyType != "form" && d.request.BodyType != "multipart" {
			prefix = "query"
		}
		for i, r := range m.rows(d) {
			if r.key == "add:"+prefix {
				d.cursor = i
				return true, m.editRow(d)
			}
		}
	case actRemove:
		m.removeRow(d)
	case actMethodCycle:
		if d.request.EndpointID != "" {
			m.say("This request follows the contract, so its method is fixed")
			return true, nil
		}
		methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
		next := 0
		for i, method := range methods {
			if strings.EqualFold(method, d.request.Method) {
				next = (i + 1) % len(methods)
			}
		}
		d.request.Method = methods[next]
	case actBody:
		return true, m.editBody(d)
	case actBodyType:
		m.cycleBodyType(d)
	case actAttach:
		m.attachFile(d, "")
	case actFind:
		if d.exchange == nil {
			m.say("Nothing to search yet")
			return true, nil
		}
		if d.find == nil {
			d.find = newInput("", "find in the body")
		}
		d.finding = true
	case actCopy:
		switch {
		case d.exchange == nil:
			m.say("Nothing to copy yet")
		case d.exchange.Binary || d.exchange.BodyWithheld:
			m.say("This body cannot be copied")
		default:
			m.ok("Copied the response body")
			return true, tea.SetClipboard(d.exchange.Body)
		}
	case actHeaders:
		d.headers = !d.headers
	case actClearRequest:
		fresh := project.RequestDraft{Method: "GET", BodyType: "raw", Module: d.request.Module, Path: map[string]string{}}
		if d.request.EndpointID != "" {
			if e, ok := m.data.endpoints[d.request.EndpointID]; ok {
				fresh, _ = project.EndpointDraft(*e)
			}
		}
		d.request = fresh
		m.say("Request cleared")
	case actReloadCase:
		if d.caseRes == nil {
			m.say("This is not a saved request")
			return true, nil
		}
		current, err := project.GetCase(m.root, d.caseRes.ID)
		if err != nil {
			m.fail(err)
			return true, nil
		}
		m.loadCase(d, current)
		m.ok("Reloaded “" + current.Name + "” from disk")
	default:
		return false, nil
	}
	return true, nil
}

// editRow starts editing the selected field, or does what the row is for.
func (m *Model) editRow(d *draft) tea.Cmd {
	row, ok := m.currentRow()
	if !ok {
		return nil
	}
	switch row.kind {
	case rowModule:
		options := append([]string{""}, m.moduleChoices()...)
		for i, id := range options {
			if id == d.request.Module {
				d.request.Module = options[(i+1)%len(options)]
				break
			}
		}
		return nil
	case rowBody:
		if d.request.BodyType == "form" || d.request.BodyType == "multipart" {
			m.say("Press " + m.keyName(actBodyType) + " to change the body type, or " + m.keyName(actAdd) + " to add a field")
			return nil
		}
		return m.editBody(d)
	}
	if strings.HasPrefix(row.key, "gfile:") {
		m.attachFile(d, strings.TrimPrefix(row.key, "gfile:"))
		return nil
	}
	value, placeholder := row.value, ""
	switch {
	case row.kind == rowGhost:
		if g, ok := d.suggestion(strings.TrimPrefix(strings.SplitN(row.key, ":", 2)[0], "g"), row.label); ok {
			value = exampleText(g.Suggestion)
		}
	case row.key == "add:header":
		placeholder = "Name: value"
	case row.kind == rowAdd:
		placeholder = "name=value"
	case row.key == "url":
		placeholder = "/path or https://…"
	}
	d.editing = newInput(value, placeholder)
	d.editKey = row.key
	return nil
}

// splitPair reads "name=value" or "Name: value".
func splitPair(text, sep string) (string, string) {
	if i := strings.Index(text, sep); i >= 0 {
		return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+len(sep):])
	}
	if i := strings.IndexAny(text, "=:"); i >= 0 {
		return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+1:])
	}
	return strings.TrimSpace(text), ""
}

func (m *Model) commitEdit(d *draft, key, value string) {
	r := &d.request
	kind, rest, _ := strings.Cut(key, ":")
	index, _ := strconv.Atoi(rest)
	switch kind {
	case "url":
		r.URL = strings.TrimSpace(value)
	case "path":
		if r.Path == nil {
			r.Path = map[string]string{}
		}
		r.Path[rest] = value
	case "query":
		if index < len(r.Query) {
			r.Query[index].Value = value
		}
	case "header":
		if index < len(r.Headers) {
			r.Headers[index].Value = value
		}
	case "form":
		if index < len(r.Form) {
			if r.Form[index].File != "" {
				r.Form[index].File = strings.TrimPrefix(value, "@")
			} else {
				r.Form[index].Value = value
			}
		}
	case "var":
		r.Variables[rest] = value
	case "gquery":
		r.Query = append(r.Query, project.RequestField{Name: rest, Value: value})
	case "gheader":
		r.Headers = append(r.Headers, project.RequestField{Name: rest, Value: value})
	case "gform":
		r.Form = append(r.Form, project.FormField{Name: rest, Value: value})
	case "add":
		if strings.TrimSpace(value) == "" {
			return
		}
		switch rest {
		case "query":
			name, v := splitPair(value, "=")
			r.Query = append(r.Query, project.RequestField{Name: name, Value: v})
		case "header":
			name, v := splitPair(value, ":")
			r.Headers = append(r.Headers, project.RequestField{Name: name, Value: v})
		case "form":
			name, v := splitPair(value, "=")
			f := project.FormField{Name: name, Value: v}
			if strings.HasPrefix(v, "@") {
				f = project.FormField{Name: name, File: strings.TrimPrefix(v, "@")}
			}
			r.Form = append(r.Form, f)
		case "var":
			name, v := splitPair(value, "=")
			if r.Variables == nil {
				r.Variables = map[string]string{}
			}
			r.Variables[name] = v
		}
	}
}

func (m *Model) removeRow(d *draft) {
	row, ok := m.currentRow()
	if !ok {
		return
	}
	r := &d.request
	kind, rest, _ := strings.Cut(row.key, ":")
	index, _ := strconv.Atoi(rest)
	switch kind {
	case "query":
		r.Query = append(r.Query[:index], r.Query[index+1:]...)
	case "header":
		r.Headers = append(r.Headers[:index], r.Headers[index+1:]...)
	case "form":
		r.Form = append(r.Form[:index], r.Form[index+1:]...)
	case "var":
		delete(r.Variables, rest)
	case "path":
		delete(r.Path, rest)
	case "body":
		if r.BodyFile != "" {
			r.BodyFile = ""
		} else if r.BodyType != "form" && r.BodyType != "multipart" {
			r.Body = ""
		}
	default:
		m.say("Nothing to remove here")
	}
}

func (m *Model) cycleBodyType(d *draft) {
	r := &d.request
	order := []string{"raw", "json", "form", "multipart"}
	current := r.BodyType
	if current == "" {
		current = "raw"
	}
	next := "raw"
	for i, kind := range order {
		if kind == current {
			next = order[(i+1)%len(order)]
		}
	}
	formNow, formNext := current == "form" || current == "multipart", next == "form" || next == "multipart"
	if !formNow && formNext {
		d.stash, r.Body, r.BodyFile = r.Body, "", ""
		r.Form, d.stashForm = d.stashForm, nil
	}
	if formNow && !formNext {
		r.Body, d.stash = d.stash, ""
		d.stashForm, r.Form = r.Form, nil
	}
	r.BodyType = next
	switch next {
	case "json":
		if r.ContentType == "" || r.ContentType == "application/x-www-form-urlencoded" || r.ContentType == "multipart/form-data" {
			r.ContentType = "application/json"
		}
	case "form":
		r.ContentType = "application/x-www-form-urlencoded"
	case "multipart":
		r.ContentType = "multipart/form-data"
	case "raw":
		if r.ContentType == "multipart/form-data" || r.ContentType == "application/x-www-form-urlencoded" {
			r.ContentType = ""
		}
	}
	m.say("Body type: " + bodyLabel(next))
}

func (m *Model) openEndpoint(e project.EndpointMatch) {
	key := "endpoint:" + e.ID
	d := m.drafts[key]
	if d == nil {
		r, guided := project.EndpointDraft(e.Endpoint)
		d = newDraft(key, r)
		d.guided = guided
		d.label = e.Method + " " + e.Path
		m.drafts[key] = d
	}
	m.show(d)
}

func (m *Model) newRequest() {
	m.scratchN++
	r := project.RequestDraft{Method: "GET", BodyType: "raw", Path: map[string]string{}}
	if choices := m.moduleChoices(); len(choices) > 0 {
		r.Module = choices[0]
	}
	d := newDraft("new:"+strconv.Itoa(m.scratchN), r)
	d.label = "New request"
	m.drafts[d.key] = d
	m.show(d)
	d.editing, d.editKey = newInput("", "/path or https://…"), "url"
}

func (m *Model) show(d *draft) {
	m.req = d
	m.screen = requestScreen
	m.respFocus = false
}

func (m *Model) openCase(c project.CaseResource) *draft {
	key := "case:" + c.ID
	d := m.drafts[key]
	if d == nil {
		d = newDraft(key, c.Request)
		m.loadCase(d, c)
		m.drafts[key] = d
	}
	m.show(d)
	return d
}

func (m *Model) loadCase(d *draft, c project.CaseResource) {
	d.request = project.CloneDraft(c.Request)
	if d.request.Path == nil {
		d.request.Path = map[string]string{}
	}
	d.baseline = encodeDraft(d.request)
	d.caseRes = &c
	d.conflict, d.deleted = nil, false
	d.guided = nil
	if e, ok := m.data.endpoints[c.Request.EndpointID]; ok {
		_, d.guided = project.EndpointDraft(*e)
	}
}

// reconcileDrafts follows saved requests changed outside the app: clean
// drafts take the new version, edited ones keep their edits and are flagged.
func (m *Model) reconcileDrafts() {
	byID := map[string]project.CaseResource{}
	for _, c := range m.data.cases {
		byID[c.ID] = c
	}
	for _, d := range m.drafts {
		if d.caseRes == nil {
			continue
		}
		current, ok := byID[d.caseRes.ID]
		switch {
		case !ok && !d.deleted:
			d.deleted = true
			if d.dirty() {
				m.say("“" + d.title() + "” was deleted on disk. Your edits are kept; saving creates it again.")
			}
		case ok && current.Revision != d.caseRes.Revision:
			if d.dirty() {
				copy := current
				d.conflict = &copy
				m.say("“" + d.title() + "” changed on disk. Your edits are kept; saving asks how to combine them.")
			} else {
				m.loadCase(d, current)
			}
		case ok:
			d.deleted = false
		}
	}
}

// send prepares off the UI goroutine, asks before production, then sends.
func (m *Model) send() tea.Cmd {
	d := m.req
	if m.busy != nil {
		m.say("Wait for the running request, or press esc to cancel it")
		return nil
	}
	if d.caseRes != nil {
		c := d.caseRes.Case
		c.Request = d.request
		if err := project.ValidateCase(c); err != nil {
			m.fail(err)
			return nil
		}
	}
	if m.data.activeEnv == "" {
		m.modal = m.envPicker()
		if p, ok := m.modal.(*pickerModal); ok {
			p.note = "Choose where to send requests, then press " + m.keyName(actSend) + " again."
		}
		return nil
	}
	request := project.CloneDraft(d.request)
	root, env, creds, key := m.root, m.data.activeEnv, m.credentials, d.key
	return m.waitFor("Preparing the request…", func() applyMsg {
		p, err := project.PrepareRequestWithCredentials(root, env, request, true, creds)
		return func(m *Model) tea.Cmd {
			if err != nil {
				m.fail(err)
				return nil
			}
			if p.Classification == "production" {
				m.modal = &confirmModal{title: "Send to production?", yes: "send", danger: true, private: true,
					lines: []string{methodStyle(p.Method).Render(p.Method) + "  " + p.URL, "", quiet.Render("This environment is marked production.")},
					onYes: func(m *Model) tea.Cmd { return m.startSend(key, request, p.Fingerprint) },
					onNo:  func(m *Model) tea.Cmd { m.say("Not sent"); return nil }}
				return nil
			}
			return m.startSend(key, request, "")
		}
	})
}

type exchangeMsg struct {
	key    string
	result project.Exchange
	err    error
	cancel context.CancelFunc
}

func (m *Model) startSend(key string, request project.RequestDraft, confirmation string) tea.Cmd {
	d := m.drafts[key]
	if d == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	method, _ := m.caseRoute(request)
	m.busy = &busy{label: "Sending " + method + " request…", cancel: cancel}
	caseID, draftInputs := "", true
	var validation map[string]any
	if d.caseRes != nil {
		caseID, validation, draftInputs = d.caseRes.ID, d.caseRes.Validation, d.dirty()
	}
	root, env, creds := m.root, m.data.activeEnv, m.credentials
	return func() tea.Msg {
		result, err := project.Send(ctx, root, env, request, project.SendOptions{CaseID: caseID, Draft: draftInputs, Validation: validation, Interactive: true, Confirmation: confirmation, Credentials: creds})
		result.CaseID, result.Draft = caseID, draftInputs
		return exchangeMsg{key, result, err, cancel}
	}
}

func (m *Model) finishSend(msg exchangeMsg) tea.Cmd {
	msg.cancel()
	m.busy = nil
	d := m.drafts[msg.key]
	if d != nil && (msg.result.RunID != "" || msg.result.Status != 0) {
		d.setExchange(msg.result)
		if m.width < 100 && m.req == d {
			m.respFocus = true
		}
	}
	ex := msg.result
	if ex.Status == 0 {
		m.fail(msg.err)
		return nil
	}
	text := fmt.Sprintf("%s in %d ms", statusText(ex.Status), ex.DurationMS)
	failed := 0
	for _, c := range ex.Checks {
		if c.Outcome == "failed" {
			failed++
		}
	}
	switch {
	case failed > 0:
		m.fail(fmt.Errorf("%s, %d check%s failed", text, failed, plural(failed, "", "s")))
	case msg.err != nil:
		m.fail(fmt.Errorf("%s: %w", text, msg.err))
	default:
		m.ok(text)
	}
	return nil
}

// save writes a saved request. A new one is named first; one used by
// scenarios says so before it changes under them.
func (m *Model) save(d *draft, asNew bool) tea.Cmd {
	if d.caseRes == nil || asNew {
		name := d.label
		if d.caseRes != nil {
			name = d.caseRes.Name + " copy"
		}
		if e, ok := m.data.endpoints[d.request.EndpointID]; ok {
			if summary, _ := e.Operation["summary"].(string); summary != "" && d.caseRes == nil {
				name = summary
			}
		}
		m.modal = &promptModal{title: "Save request", label: "save", input: newInput(name, "Name"),
			note: []string{"Saved requests are kept in the project and can be run from scenarios and the CLI."},
			submit: func(m *Model, name string) (tea.Cmd, error) {
				name = strings.TrimSpace(name)
				if name == "" {
					return nil, project.Fail("invalid_input", "give it a name")
				}
				c := project.Case{ID: uuid.NewString(), Name: name, Request: project.CloneDraft(d.request)}
				if d.caseRes != nil {
					c.Validation, c.ComparisonIgnore = d.caseRes.Validation, d.caseRes.ComparisonIgnore
				}
				saved, err := project.SaveCase(m.root, c, "", false)
				if err != nil {
					return nil, err
				}
				m.sync()
				nd := newDraft("case:"+saved.ID, saved.Request)
				m.loadCase(nd, saved)
				nd.exchange, nd.body = d.exchange, d.body
				m.drafts[nd.key] = nd
				m.show(nd)
				m.ok("Saved “" + name + "”")
				return nil, nil
			}}
		return nil
	}
	return m.saveCase(d, d.caseRes.Case, false)
}

func (m *Model) saveCase(d *draft, c project.Case, confirmed bool) tea.Cmd {
	c.Request = project.CloneDraft(d.request)
	if !confirmed {
		refs, err := project.CaseReferences(m.root, c.ID)
		if err != nil {
			m.fail(err)
			return nil
		}
		if len(refs) > 0 {
			lines := []string{"Scenarios that run this request will use the new version:", ""}
			for _, name := range m.scenarioNames(refs) {
				lines = append(lines, "  "+name)
			}
			m.modal = &confirmModal{title: "Change a shared request?", lines: lines, yes: "save",
				onYes: func(m *Model) tea.Cmd { return m.saveCase(d, c, true) }}
			return nil
		}
	}
	revision := ""
	if !d.deleted {
		revision = d.caseRes.Revision
	}
	saved, err := project.SaveCase(m.root, c, revision, false)
	if err != nil {
		if pe, ok := err.(*project.Error); ok && pe.Code == "revision_conflict" {
			m.reconcile(d)
			return nil
		}
		m.fail(err)
		return nil
	}
	m.loadCase(d, saved)
	m.sync()
	m.ok("Saved “" + saved.Name + "”")
	return nil
}

// reconcile asks how to combine edits with a version that changed on disk.
func (m *Model) reconcile(d *draft) {
	current, err := project.GetCase(m.root, d.caseRes.ID)
	if err != nil {
		m.modal = &confirmModal{title: "Deleted on disk", lines: []string{"“" + d.title() + "” was deleted outside Reqscope.", "Save it again with your edits?"}, yes: "save again",
			onYes: func(m *Model) tea.Cmd { d.deleted = true; return m.saveCase(d, d.caseRes.Case, true) }}
		return
	}
	theirs, ours := m.summarise(current.Request), m.summarise(d.request)
	lines := []string{"“" + d.title() + "” changed on disk while you were editing it.", "", bold.Render("On disk")}
	lines = append(lines, theirs...)
	lines = append(lines, "", bold.Render("Yours"))
	lines = append(lines, ours...)
	m.modal = &confirmModal{title: "Changed on disk", lines: lines,
		extra: []extraKey{
			{"o", "keep mine", func(m *Model) tea.Cmd {
				d.caseRes.Revision, d.deleted = current.Revision, false
				return m.saveCase(d, current.Case, true)
			}},
			{"t", "take theirs", func(m *Model) tea.Cmd {
				m.loadCase(d, current)
				m.ok("Loaded the version on disk")
				return nil
			}},
		}}
}

func (m *Model) summarise(r project.RequestDraft) []string {
	method, path := m.caseRoute(r)
	lines := []string{"  " + methodStyle(method).Render(method) + " " + path}
	for _, f := range r.Query {
		lines = append(lines, quiet.Render("  ?"+f.Name+"="+f.Value))
	}
	for _, f := range r.Headers {
		lines = append(lines, quiet.Render("  "+f.Name+": "+f.Value))
	}
	if r.Body != "" {
		lines = append(lines, quiet.Render("  body: "+clip(strings.ReplaceAll(r.Body, "\n", " "), 60)))
	}
	return lines
}

// editRaw opens the whole request as JSON for anything the form leaves out:
// timeouts, redirects, limits and endpoint links.
func (m *Model) editRaw(d *draft) tea.Cmd {
	b, _ := json.MarshalIndent(d.request, "", "  ")
	return m.editText(string(b)+"\n", "json", func(m *Model, text string) (tea.Cmd, error) {
		var r project.RequestDraft
		if err := project.Decode([]byte(text), &r); err != nil {
			return nil, err
		}
		if r.Path == nil {
			r.Path = map[string]string{}
		}
		d.request = r
		m.ok("Request updated")
		return nil, nil
	})
}

func (m *Model) editBody(d *draft) tea.Cmd {
	r := d.request
	if r.BodyFile != "" || r.BodyType != "raw" && r.BodyType != "json" && r.BodyType != "" {
		m.say("The body editor works on raw and JSON bodies. Press " + m.keyName(actBodyType) + " to change the body type.")
		return nil
	}
	if m.credentials.Sanitize(r.Body) != r.Body {
		m.fail(fmt.Errorf("this body contains a stored credential; replace it with {{secret:name}} before editing it outside Reqscope"))
		return nil
	}
	previous := encodeDraft(d.request)
	ext := "txt"
	if r.BodyType == "json" {
		ext = "json"
	}
	// An empty body starts from the contract's example, so the editor opens
	// on something to change rather than a blank file.
	initial, example := r.Body, false
	if g, ok := d.suggestion("body", "body"); ok && initial == "" && g.Suggestion != nil {
		if s, ok := g.Suggestion.(string); ok {
			initial = s
		} else {
			b, _ := json.MarshalIndent(g.Suggestion, "", "  ")
			initial = string(b) + "\n"
		}
		example = true
	}
	return m.openEditor(initial, ext, func(text string, err error) tea.Cmd {
		switch {
		case err != nil:
			m.say("The editor closed with an error; the body is unchanged")
		case encodeDraft(d.request) != previous:
			m.say("The request changed while the editor was open; kept the current version")
		case text == d.request.Body:
			m.say("No changes to the body")
		default:
			d.request.Body = text
			switch {
			case d.request.BodyType == "json" && !json.Valid([]byte(text)):
				m.fail(fmt.Errorf("the body is not valid JSON yet; press %s to fix it before sending", m.keyName(actBody)))
			case example && text == initial:
				m.ok("Body set from the contract example")
			default:
				m.ok("Body updated")
			}
		}
		return nil
	})
}

// attachFile picks a local file for a multipart field or a raw body.
func (m *Model) attachFile(d *draft, field string) {
	switch d.request.BodyType {
	case "multipart", "raw", "":
	default:
		m.say("Files attach to multipart or raw bodies. Press " + m.keyName(actBodyType) + " to change the body type.")
		return
	}
	choose := func(field string) {
		dir, _ := filepath.Abs(m.root)
		m.modal = newFileModal(dir, func(m *Model, path string) {
			r := &d.request
			if r.BodyType != "multipart" {
				r.BodyType, r.BodyFile, r.Body = "raw", path, ""
				m.ok("The body is now " + filepath.Base(path))
				return
			}
			f := project.FormField{Name: field, File: path}
			for i := range r.Form {
				if r.Form[i].Name == field && r.Form[i].File != "" {
					r.Form[i] = f
					m.ok("Attached " + filepath.Base(path))
					return
				}
			}
			r.Form = append(r.Form, f)
			m.ok("Attached " + filepath.Base(path))
		})
	}
	if d.request.BodyType != "multipart" || field != "" {
		choose(field)
		return
	}
	for _, g := range d.guided {
		if g.In == "file" {
			field = g.Name
			break
		}
	}
	if field == "" {
		field = "file"
	}
	m.modal = &promptModal{title: "Attach a file", label: "choose file", input: newInput(field, "field name"), note: []string{"Which form field should carry the file?"},
		submit: func(m *Model, name string) (tea.Cmd, error) {
			if strings.TrimSpace(name) == "" {
				return nil, project.Fail("invalid_input", "enter a field name")
			}
			choose(strings.TrimSpace(name))
			return nil, nil
		}}
}

// fileModal browses directories for an upload.
type fileModal struct {
	dir     string
	entries []os.DirEntry
	cursor  int
	err     string
	pick    func(m *Model, path string)
}

func newFileModal(dir string, pick func(m *Model, path string)) *fileModal {
	f := &fileModal{dir: dir, pick: pick}
	f.load()
	return f
}

func (f *fileModal) load() {
	entries, err := os.ReadDir(f.dir)
	f.err = ""
	if err != nil {
		f.err = "Cannot read this folder"
		entries = nil
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].IsDir() && !entries[j].IsDir() })
	f.entries, f.cursor = entries, 0
}

func (f *fileModal) preferredWidth() int { return 76 }

func (f *fileModal) update(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.modal = nil
	case "j", "down":
		f.cursor = min(f.cursor+1, max(0, len(f.entries)-1))
	case "k", "up":
		f.cursor = max(0, f.cursor-1)
	case "h", "backspace", "left":
		f.dir = filepath.Dir(f.dir)
		f.load()
	case "enter", "l", "right":
		if len(f.entries) == 0 {
			return nil
		}
		path := filepath.Join(f.dir, f.entries[f.cursor].Name())
		info, err := os.Stat(path)
		if err != nil {
			f.err = "Cannot read that file"
			return nil
		}
		if info.IsDir() {
			f.dir = path
			f.load()
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0444 == 0 {
			f.err = "Choose a readable file"
			return nil
		}
		m.modal = nil
		f.pick(m, path)
	}
	return nil
}

func (f *fileModal) content(m *Model, width, height int) (string, []string, []string) {
	body := []string{quiet.Render(clip(f.dir, width)), ""}
	rows := max(1, height-3)
	start := max(0, min(f.cursor-rows/2, len(f.entries)-rows))
	for i := start; i < len(f.entries) && i < start+rows; i++ {
		name := f.entries[i].Name()
		style := lipglossPlain
		if f.entries[i].IsDir() {
			name += "/"
			style = accent
		}
		gutter := " "
		if i == f.cursor {
			gutter = accent.Render("▌")
		}
		body = append(body, gutter+row([]span{sp(name, style)}, width-1, i == f.cursor))
	}
	if len(f.entries) == 0 {
		body = append(body, quiet.Render("Empty folder"))
	}
	if f.err != "" {
		body = append(body, "", badStyle.Render(f.err))
	}
	return "Choose a file", body, []string{hint("enter", "open or choose"), hint("h", "up a folder"), hint("esc", "cancel")}
}
