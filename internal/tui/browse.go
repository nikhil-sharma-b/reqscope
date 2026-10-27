package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
)

// previewCache keeps the one preview that needs YAML rendering.
type previewCache struct {
	key   string
	lines []string
}

func (m *Model) browseView(width, height int) []string {
	title := tabNames[m.tab]
	if m.tab == endpointsTab && m.schemas {
		title = "Schemas"
	}
	zoomed := m.zoom[browseScreen]
	// One pane at a time when zoomed or narrow: whichever has the keys.
	if width < 96 || zoomed {
		note := ""
		if m.listLen() > 0 {
			note = m.paneNote(m.detailFocus, true)
		}
		if !m.detailFocus && m.listLen() > 0 {
			note = m.position() + "  " + note
		}
		if zoomed {
			note = quiet.Render("zoomed  ") + note
		}
		if m.detailFocus {
			body, foot := m.detailLines(width-4, height-2)
			return framed(body, "Details", note, foot, width, height, true)
		}
		return frame(m.listView(width-4, height-2), title, note, width, height, true)
	}
	listWidth := max(44, width*46/100)
	previewWidth := width - listWidth
	listNote, detailNote := m.position(), ""
	if m.detailFocus {
		listNote = m.paneNote(true, false) + " " + listNote
	} else if m.listLen() > 0 {
		detailNote = m.paneNote(false, false)
	}
	list := frame(m.listView(listWidth-4, height-2), title, listNote, listWidth, height, !m.detailFocus)
	body, foot := m.detailLines(previewWidth-4, height-2)
	preview := framed(body, "Details", detailNote, foot, previewWidth, height, m.detailFocus)
	return join(height, list, preview)
}

// detailLines is the preview scrolled to where the person left it, and the
// scroll position for the frame.
func (m *Model) detailLines(width, height int) ([]string, string) {
	lines := m.previewLines(width)
	m.detailScroll = max(0, min(m.detailScroll, len(lines)-height))
	return lines[m.detailScroll:], scrollPos(m.detailScroll, height, len(lines))
}

// previewLines describes the selection in full on wide screens.
func (m *Model) previewLines(width int) []string {
	if m.listLen() == 0 {
		return nil
	}
	switch m.tab {
	case endpointsTab:
		if m.schemas {
			if r, ok := m.selectedSchema(); ok {
				return m.schemaPreview(r, width)
			}
			return nil
		}
		if e, ok := m.selectedEndpoint(); ok {
			return m.endpointPreview(e, width)
		}
	case savedTab:
		if c, ok := m.selectedCase(); ok {
			return m.casePreview(c, width)
		}
	case scenariosTab:
		if s, ok := m.selectedScenario(); ok {
			return m.scenarioPreview(s, width)
		}
	case historyTab:
		if r, ok := m.selectedRun(); ok {
			return m.runPreview(r, width)
		}
	case envTab:
		if e, ok := m.selectedEnv(); ok {
			return m.envPreview(e, width)
		}
	}
	return nil
}

func wrap(text string, width int) []string {
	if text == "" {
		return nil
	}
	return strings.Split(ansi.Wrap(text, max(1, width), " /-"), "\n")
}

func section(title string) string { return accentBold.Render(title) }

// param is one contract input, read from either OpenAPI generation.
type param struct {
	name, in, kind string
	required       bool
	example        any
	description    string
}

func endpointParams(e project.Endpoint) []param {
	var out []param
	seen := map[string]int{}
	for _, raw := range []any{e.PathInputs, e.Operation["parameters"]} {
		items, _ := raw.([]any)
		for _, item := range items {
			p, _ := item.(map[string]any)
			if p == nil {
				continue
			}
			var x param
			x.name, _ = p["name"].(string)
			x.in, _ = p["in"].(string)
			x.required, _ = p["required"].(bool)
			x.description, _ = p["description"].(string)
			x.example = p["example"]
			schema, _ := p["schema"].(map[string]any)
			x.kind, _ = p["type"].(string)
			if schema != nil {
				if t, ok := schema["type"].(string); ok {
					x.kind = t
				}
				if x.example == nil {
					x.example = schema["example"]
				}
			}
			if x.in == "body" {
				continue
			}
			key := x.in + ":" + x.name
			if i, ok := seen[key]; ok {
				out[i] = x
				continue
			}
			seen[key] = len(out)
			out = append(out, x)
		}
	}
	return out
}

func exampleText(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func (m *Model) endpointPreview(e project.EndpointMatch, width int) []string {
	lines := []string{methodStyle(e.Method).Render(e.Method) + "  " + styledPath(e.Path, bold)}
	if e.Summary != "" {
		lines = append(lines, wrap(e.Summary, width)...)
	}
	if desc, _ := e.Operation["description"].(string); desc != "" && desc != e.Summary {
		for _, line := range wrap(strings.TrimSpace(desc), width) {
			lines = append(lines, quiet.Render(line))
		}
	}
	if params := endpointParams(e.Endpoint); len(params) > 0 {
		lines = append(lines, "", section("Parameters"))
		nameWidth := 0
		for _, p := range params {
			nameWidth = max(nameWidth, ansi.StringWidth(p.name))
		}
		nameWidth = min(nameWidth, 24)
		for _, p := range params {
			spans := []span{plain(pad(p.name, nameWidth)), plain("  "), sp(pad(p.in, 6), quiet), plain(" "), sp(pad(p.kind, 8), quiet)}
			if p.required {
				spans = append(spans, plain(" "), sp("required", accent))
			}
			if ex := exampleText(p.example); ex != "" {
				spans = append(spans, plain("  "), sp("e.g. "+ex, quiet))
			}
			lines = append(lines, row(spans, width, false))
		}
	}
	if body := requestBody(e.Endpoint); body != "" {
		lines = append(lines, "", section("Body"), body)
	}
	if responses, _ := e.Operation["responses"].(map[string]any); len(responses) > 0 {
		lines = append(lines, "", section("Responses"))
		codes := make([]string, 0, len(responses))
		for code := range responses {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		for _, code := range codes {
			r, _ := responses[code].(map[string]any)
			desc, _ := r["description"].(string)
			n, _ := strconv.Atoi(code)
			style := statusStyle(n)
			if n == 0 {
				style = quiet
			}
			lines = append(lines, row([]span{sp(pad(code, 8), style), plain(desc)}, width, false))
		}
	}
	lines = append(lines, "")
	if len(e.Tags) > 0 {
		lines = append(lines, quiet.Render("Tags       ")+strings.Join(e.Tags, ", "))
	}
	if id, _ := e.Operation["operationId"].(string); id != "" {
		lines = append(lines, quiet.Render("Operation  ")+id)
	}
	lines = append(lines, quiet.Render("From       ")+quiet.Render(m.moduleName(e.ModuleID)+"  "+filepath.Base(e.Source)))
	return lines
}

func requestBody(e project.Endpoint) string {
	if rb, ok := e.Operation["requestBody"].(map[string]any); ok {
		content, _ := rb["content"].(map[string]any)
		var types []string
		for t := range content {
			types = append(types, t)
		}
		sort.Strings(types)
		text := strings.Join(types, ", ")
		if required, _ := rb["required"].(bool); required {
			text += accent.Render("  required")
		}
		return text
	}
	if items, ok := e.Operation["parameters"].([]any); ok {
		for _, item := range items {
			if p, _ := item.(map[string]any); p != nil && p["in"] == "body" {
				return "application/json"
			}
		}
	}
	return ""
}

// toYAML writes YAML with two-space indentation, as people write it.
func toYAML(v any) []byte {
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	_ = enc.Encode(v)
	_ = enc.Close()
	return []byte(b.String())
}

func (m *Model) schemaYAML(r schemaRow) []string {
	key := fmt.Sprintf("%d|%s|%s", m.data.specGen, r.spec.ID, r.schema.ID)
	if m.preview.key != key {
		b := toYAML(r.schema.Definition)
		m.preview = previewCache{key, strings.Split(strings.TrimRight(m.credentials.Sanitize(string(b)), "\n"), "\n")}
	}
	return m.preview.lines
}

func (m *Model) schemaPreview(r schemaRow, width int) []string {
	lines := []string{bold.Render(r.schema.Name), quiet.Render(m.moduleName(r.spec.ModuleID)), ""}
	for _, line := range m.schemaYAML(r) {
		lines = append(lines, yamlLine(line))
	}
	return lines
}

// yamlLine colours a key the way JSON keys are coloured in responses.
func yamlLine(line string) string {
	trimmed := strings.TrimLeft(line, " -")
	if i := strings.Index(trimmed, ":"); i > 0 && !strings.ContainsAny(trimmed[:i], " \"'") {
		indent := line[:len(line)-len(trimmed)]
		return indent + accent.Render(trimmed[:i]) + quiet.Render(":") + trimmed[i+1:]
	}
	return line
}

func (m *Model) casePreview(c project.CaseResource, width int) []string {
	method, path := m.caseRoute(c.Request)
	lines := []string{bold.Render(c.Name), methodStyle(method).Render(method) + "  " + styledPath(path, lipglossPlain)}
	if c.Request.Module != "" {
		lines = append(lines, quiet.Render("API  ")+m.moduleName(c.Request.Module))
	}
	if d := m.drafts["case:"+c.ID]; d != nil && d.dirty() {
		lines = append(lines, warnStyle.Render("Edited and not saved yet"))
	}
	lines = append(lines, "")
	lines = append(lines, fieldLines("Query", c.Request.Query, "=", width)...)
	lines = append(lines, fieldLines("Headers", c.Request.Headers, ": ", width)...)
	if c.Request.Body != "" || c.Request.BodyFile != "" || len(c.Request.Form) > 0 {
		lines = append(lines, section("Body")+"  "+quiet.Render(bodyLabel(c.Request.BodyType)))
		for i, line := range bodyPreview(c.Request, width) {
			if i >= 8 {
				break
			}
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}
	if assertions, ok := c.Validation["assertions"].([]any); ok && len(assertions) > 0 {
		lines = append(lines, quiet.Render("Checks     ")+fmt.Sprintf("%d", len(assertions)))
	}
	if names := m.caseUsers(c.ID); len(names) > 0 {
		lines = append(lines, quiet.Render("Used by    ")+strings.Join(names, ", "))
	}
	return lines
}

func fieldLines(title string, fields []project.RequestField, sep string, width int) []string {
	if len(fields) == 0 {
		return nil
	}
	lines := []string{section(title)}
	for _, f := range fields {
		lines = append(lines, clip("  "+f.Name+quiet.Render(sep)+f.Value, width))
	}
	return append(lines, "")
}

// caseUsers names the scenarios with a step that runs the saved request.
func (m *Model) caseUsers(id string) []string {
	var out []string
	for _, s := range m.data.scenarios {
		for _, step := range s.Steps {
			if step.CaseID == id {
				out = append(out, s.Name)
				break
			}
		}
	}
	return out
}

// scenarioNames turns reference paths into the scenario names people chose.
func (m *Model) scenarioNames(refs []string) []string {
	var out []string
	for _, ref := range refs {
		id := strings.TrimSuffix(filepath.Base(ref), ".yaml")
		out = append(out, m.scenarioName(id))
	}
	return out
}

func (m *Model) scenarioPreview(s project.ScenarioResource, width int) []string {
	mode := "Stops at the first failed step"
	if s.ContinueOnFailure {
		mode = "Runs every step, even after a failure"
	}
	lines := []string{bold.Render(s.Name), quiet.Render(mode), "", section("Steps")}
	for i, step := range s.Steps {
		name := step.CaseID
		for _, c := range m.data.cases {
			if c.ID == step.CaseID {
				name = c.Name
			}
		}
		lines = append(lines, row([]span{sp(fmt.Sprintf("%2d  ", i+1), quiet), sp(step.ID, bold), plain("  "), plain(name)}, width, false))
		var notes []string
		for name, b := range step.Bindings {
			notes = append(notes, "uses "+b.StepID+"."+b.Extract+" as "+name)
		}
		for name := range step.Extracts {
			notes = append(notes, "keeps "+name)
		}
		for name := range step.SecretExtracts {
			notes = append(notes, "keeps protected "+name)
		}
		sort.Strings(notes)
		for _, n := range notes {
			lines = append(lines, quiet.Render("      "+clip(n, width-6)))
		}
	}
	if len(s.Steps) == 0 {
		lines = append(lines, quiet.Render("No steps yet."))
	}
	return lines
}

func (m *Model) runPreview(r project.Run, width int) []string {
	if r.ScenarioID != "" {
		lines := []string{bold.Render(m.scenarioName(r.ScenarioID)), outcomeStyle(r.Outcome).Render(r.Outcome), quiet.Render(r.StartedAt.Local().Format("Jan 02 15:04:05") + "  " + m.envName(r.Environment))}
		if r.Error != "" {
			lines = append(lines, "", badStyle.Render(clip(r.Error, width)))
		}
		return lines
	}
	ex := r.Response
	lines := []string{}
	if ex.Status != 0 {
		lines = append(lines, statusBadge(ex.Status, statusText(ex.Status))+"  "+quiet.Render(fmt.Sprintf("%d ms  %s", ex.DurationMS, humanBytes(ex.RetainedBytes))))
	} else {
		lines = append(lines, outcomeStyle(r.Outcome).Render(r.Outcome))
	}
	lines = append(lines, methodStyle(ex.Method).Render(ex.Method)+"  "+clip(ex.Destination, width-len(ex.Method)-2))
	lines = append(lines, quiet.Render(r.StartedAt.Local().Format("Jan 02 15:04:05")+"  "+m.envName(r.Environment)))
	if r.Pinned {
		lines = append(lines, accent.Render("◆ pinned"))
	}
	if r.Error != "" {
		lines = append(lines, "")
		for _, line := range wrap(r.Error, width) {
			lines = append(lines, badStyle.Render(line))
		}
	}
	if ex.Body != "" && !ex.Binary && !ex.BodyWithheld {
		lines = append(lines, "")
		for i, line := range bodyLines(ex) {
			if i >= 40 {
				break
			}
			lines = append(lines, jsonLine(line))
		}
	}
	return lines
}

func (m *Model) envName(id string) string {
	if e, ok := m.envByID(id); ok {
		return e.Name
	}
	return id
}

func (m *Model) envPreview(e project.Resource, width int) []string {
	title := bold.Render(e.Name)
	if e.Classification == "production" {
		title = delStyle.Render(e.Name) + quiet.Render("  production: every send asks first")
	}
	lines := []string{title}
	if e.ID == m.data.activeEnv {
		lines = append(lines, okStyle.Render("● requests go here"))
	} else {
		lines = append(lines, quiet.Render("Not in use. Press ")+accentBold.Render(m.keyName(actUseEnv))+quiet.Render(" to send requests here."))
	}
	lines = append(lines, "", section("Base URLs"))
	modules := m.envModules(e.Environment)
	if len(modules) == 0 {
		lines = append(lines, quiet.Render("None yet. Press enter to add one."))
	}
	nameWidth := 0
	for _, id := range modules {
		nameWidth = max(nameWidth, ansi.StringWidth(m.moduleName(id)))
	}
	nameWidth = min(nameWidth, 24)
	for _, id := range modules {
		value := e.BaseURLs[id]
		cell := sp(value, lipglossPlain)
		if value == "" {
			cell = sp("not set", warnStyle)
		}
		lines = append(lines, row([]span{plain("  " + pad(m.moduleName(id), nameWidth) + "  "), cell}, width, false))
	}
	if len(e.Variables) > 0 {
		lines = append(lines, "", section("Variables"))
		names := make([]string, 0, len(e.Variables))
		for name := range e.Variables {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			lines = append(lines, clip("  "+name+quiet.Render(" = ")+e.Variables[name], width))
		}
	}
	return lines
}

// envModules lists every API the environment should have a base URL for:
// the imported ones first, then any others it names.
func (m *Model) envModules(e project.Environment) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range m.data.moduleList {
		seen[id] = true
		out = append(out, id)
	}
	var extra []string
	for id := range e.BaseURLs {
		if !seen[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	return append(out, extra...)
}

func (m *Model) openSelection() tea.Cmd {
	switch m.tab {
	case endpointsTab:
		if m.schemas {
			if r, ok := m.selectedSchema(); ok {
				m.modal = m.contractViewer(r.schema.Name, r.schema, r.spec)
			}
			return nil
		}
		if e, ok := m.selectedEndpoint(); ok {
			m.openEndpoint(e)
		}
	case savedTab:
		if c, ok := m.selectedCase(); ok {
			m.openCase(c)
		}
	case scenariosTab:
		if s, ok := m.selectedScenario(); ok {
			return m.editScenario(s)
		}
	case historyTab:
		if r, ok := m.selectedRun(); ok {
			m.viewRun(r)
		}
	case envTab:
		if e, ok := m.selectedEnv(); ok {
			m.editEnv(e, false)
		}
	}
	return nil
}

func (m *Model) endpointAction(id action) (bool, tea.Cmd) {
	switch id {
	case actImport:
		m.startImport("")
	case actContract:
		if m.schemas {
			if r, ok := m.selectedSchema(); ok {
				m.modal = m.contractViewer(r.schema.Name, r.schema, r.spec)
			}
		} else if e, ok := m.selectedEndpoint(); ok {
			m.modal = m.contractViewer(e.Method+" "+e.Path, e.Endpoint, m.specByID(e.SourceID))
		}
	case actNewRequest:
		m.newRequest()
	case actMethod:
		methods := []string{"", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
		for i, method := range methods {
			if method == m.methodFilter {
				m.methodFilter = methods[(i+1)%len(methods)]
				break
			}
		}
		m.lists[endpointsTab].cursor, m.lists[endpointsTab].offset = 0, 0
		m.skipHeader(1)
	case actGroup:
		m.groupByTag = !m.groupByTag
		m.lists[endpointsTab].cursor, m.lists[endpointsTab].offset = 0, 0
		m.skipHeader(1)
	case actSchemas:
		m.schemas = !m.schemas
		m.lists[endpointsTab] = listState{}
		m.skipHeader(1)
	case actRefreshSource:
		return true, m.refreshSource()
	case actEditContract:
		return true, m.editContract()
	case actExportContract:
		return true, m.exportContract()
	default:
		return false, nil
	}
	return true, nil
}

func (m *Model) specByID(id string) *project.Specification {
	for i := range m.data.specs {
		if m.data.specs[i].ID == id {
			return &m.data.specs[i]
		}
	}
	return nil
}

// selectedSource is the specification behind the selection, or the first.
func (m *Model) selectedSource() *project.Specification {
	if len(m.data.specs) == 0 {
		return nil
	}
	if m.tab == endpointsTab {
		if m.schemas {
			if r, ok := m.selectedSchema(); ok {
				return r.spec
			}
		} else if e, ok := m.selectedEndpoint(); ok {
			if s := m.specByID(e.SourceID); s != nil {
				return s
			}
		}
	}
	return &m.data.specs[0]
}

func (m *Model) contractViewer(title string, value any, spec *project.Specification) modal {
	b := toYAML(value)
	text := string(b)
	if spec != nil {
		var notes []string
		switch v := value.(type) {
		case project.Endpoint:
			notes = relevantNotes(spec.Diagnostics, "/paths/"+v.Path+"/"+strings.ToLower(v.Method)+"/", "operation: "+v.Path+" "+v.Method)
		case project.Schema:
			notes = relevantNotes(spec.Diagnostics, "/schemas/"+v.Name+"/", "/definitions/"+v.Name+"/")
		default:
			notes = spec.Diagnostics
		}
		text += "\n# From " + spec.Location + "\n"
		for _, d := range notes {
			if path, ok := strings.CutPrefix(d, unsupportedNote); ok {
				i := strings.LastIndex(path, "/")
				d = "Not used to check responses: " + path[i+1:] + " at " + path[:i]
			}
			text += "# " + d + "\n"
		}
	}
	text = m.credentials.Sanitize(text)
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if strings.HasPrefix(line, "#") {
			lines = append(lines, quiet.Render(line))
		} else {
			lines = append(lines, yamlLine(line))
		}
	}
	return &viewerModal{title: title, lines: lines, copyText: text}
}

func (m *Model) sourceOptions(location string) project.SourceImportOptions {
	opts := project.SourceImportOptions{Credentials: m.credentials, Interactive: true}
	if m.data.activeEnv != "" {
		if review, err := project.ReviewAccess(m.root, m.data.activeEnv); err == nil {
			origin, _ := project.Origin(location)
			for module, target := range review.Origins {
				if target == origin {
					opts.Environment = m.data.activeEnv
					opts.Module = module
					break
				}
			}
		}
	}
	return opts
}

func isURL(s string) bool { return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") }

// startImport asks for a file or URL. URLs need the origin approved before
// anything is fetched; files are previewed straight away.
func (m *Model) startImport(value string) {
	p := &promptModal{title: "Import an API", label: "preview", input: newInput(value, "./openapi.yaml or https://api.example.com/docs"),
		note: []string{"A path to an OpenAPI or Swagger file, or the URL of a Swagger UI page or raw spec."}}
	p.submit = func(m *Model, value string) (tea.Cmd, error) {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, project.Fail("invalid_input", "enter a file path or URL")
		}
		if isURL(value) {
			origin, err := project.Origin(value)
			if err != nil {
				return nil, err
			}
			m.modal = &confirmModal{title: "Fetch from " + origin + "?", yes: "allow and fetch", enterOK: true,
				lines: []string{
					"Reqscope will download the API description from:",
					"  " + bold.Render(origin),
					"",
					quiet.Render("The CLI may import from this origin later too."),
					quiet.Render("Stored credentials for it in the active environment may be sent."),
				},
				onYes: func(m *Model) tea.Cmd {
					if err := project.ApproveSourceOrigin(m.root, value); err != nil {
						m.startImport(value)
						m.modal.(*promptModal).err = sentence(err.Error())
						return nil
					}
					return m.discover(value)
				},
				onNo: func(m *Model) tea.Cmd { m.startImport(value); return nil },
			}
			return nil, nil
		}
		preview, err := project.ImportSpecification(m.root, value, true)
		if err != nil {
			return nil, err
		}
		m.confirmImport(preview, value, nil)
		return nil, nil
	}
	m.modal = p
}

func (m *Model) discover(location string) tea.Cmd {
	root, opts := m.root, m.sourceOptions(location)
	return m.waitFor("Looking for the API description…", func() applyMsg {
		discovery, err := project.DiscoverSource(root, location, opts)
		return func(m *Model) tea.Cmd {
			if err != nil {
				detail := sentence(err.Error()) + ". Paste the raw JSON/YAML URL instead."
				for _, c := range discovery.Candidates {
					detail += "\nFound: " + c.Name + "  " + c.Location
				}
				for _, r := range discovery.Resources {
					detail += "\nRead: " + r
				}
				m.startImport(location)
				m.modal.(*promptModal).err = detail
				return nil
			}
			switch len(discovery.Candidates) {
			case 0:
				m.startImport(location)
				m.modal.(*promptModal).err = sentence(discovery.Hint)
				return nil
			case 1:
				return m.previewSource(discovery.Candidates[0], location)
			}
			items := make([]pickItem, len(discovery.Candidates))
			for i, c := range discovery.Candidates {
				items[i] = pickItem{label: c.Name, detail: c.Location}
			}
			m.modal = &pickerModal{title: "Which API?", items: items, pick: func(m *Model, i int) tea.Cmd {
				return m.previewSource(discovery.Candidates[i], location)
			}}
			return nil
		}
	})
}

func (m *Model) previewSource(c project.SourceCandidate, location string) tea.Cmd {
	root, opts := m.root, m.sourceOptions(c.Location)
	return m.waitFor("Reading "+c.Location+"…", func() applyMsg {
		preview, err := project.ImportSource(root, c, true, "", opts)
		return func(m *Model) tea.Cmd {
			if err != nil {
				m.startImport(location)
				m.modal.(*promptModal).err = "Could not read " + c.Location + ": " + err.Error() + ". Paste the raw JSON/YAML URL instead."
				return nil
			}
			m.confirmImport(preview, location, &c)
			return nil
		}
	})
}

// confirmImport shows what an import would add. Applying previews again and
// refuses if the source changed in between, so what was reviewed is what lands.
func (m *Model) confirmImport(preview project.Specification, location string, candidate *project.SourceCandidate) {
	title := "Import this API?"
	if info, ok := preview.Base["info"].(map[string]any); ok {
		if t, ok := info["title"].(string); ok && t != "" {
			title = "Import " + t + "?"
		}
	}
	lines := []string{
		bold.Render(fmt.Sprintf("%d endpoint%s", len(preview.Endpoints), plural(len(preview.Endpoints), "", "s"))) + quiet.Render(fmt.Sprintf(", %d schema%s", len(preview.Schemas), plural(len(preview.Schemas), "", "s"))),
		quiet.Render(preview.Location),
	}
	summary, details := importNotes(preview.Diagnostics)
	lines = append(lines, "")
	lines = append(lines, summary...)
	c := &confirmModal{title: title, lines: lines, yes: "import", enterOK: true}
	if len(details) > 0 {
		c.extra = []extraKey{{"v", "details", func(m *Model) tea.Cmd {
			m.modal = &viewerModal{title: "Import notes", lines: details, back: c, width: 100}
			return nil
		}}}
	}
	m.modal = c
	c.onYes = func(m *Model) tea.Cmd {
		var next project.Specification
		var err error
		opts := m.sourceOptions(location)
		if candidate != nil {
			next, err = project.ImportSource(m.root, *candidate, true, "", opts)
		} else {
			next, err = project.ImportSpecification(m.root, location, true)
		}
		if err == nil && next.Digest != preview.Digest {
			m.confirmImport(next, location, candidate)
			m.modal.(*confirmModal).err = "The source changed since the preview. Review it again."
			return nil
		}
		if err == nil {
			if candidate != nil {
				_, err = project.ImportSource(m.root, *candidate, false, preview.Digest, opts)
			} else {
				_, err = project.ImportReviewedSpecification(m.root, location, preview.Digest)
			}
		}
		if err != nil {
			m.fail(err)
			return nil
		}
		m.sync()
		m.screen, m.tab, m.schemas = browseScreen, endpointsTab, false
		m.lists[endpointsTab] = listState{}
		m.ok(fmt.Sprintf("Imported %d endpoint%s. Set a base URL for this API in Environments (%s).", len(preview.Endpoints), plural(len(preview.Endpoints), "", "s"), m.keyName(actTab5)))
		return nil
	}
}

const (
	unsupportedNote = "Unsupported constraint or operation "
	inspectionNote  = "Contract inspection only; response schema validation is not implemented"
)

// importNotes condenses the importer's notes. Most say a schema uses a
// feature such as allOf that Reqscope shows but does not check responses
// against; a large API can have hundreds, so they are counted, not listed.
// Anything else is listed, as it may need attention.
func importNotes(diagnostics []string) (summary, details []string) {
	counts := map[string]int{}
	total := 0
	var other []string
	for _, d := range diagnostics {
		switch {
		case d == inspectionNote:
		case strings.HasPrefix(d, unsupportedNote):
			path := strings.TrimPrefix(d, unsupportedNote)
			keyword := path[strings.LastIndex(path, "/")+1:]
			counts[keyword]++
			total++
			details = append(details, accent.Render(pad(keyword, 14))+" "+notePlace(path[:strings.LastIndex(path, "/")]))
		default:
			other = append(other, d)
		}
	}
	summary = []string{okStyle.Render("Ready to import.")}
	if total > 0 {
		var parts []string
		for _, k := range sortedKeys(counts) {
			parts = append(parts, fmt.Sprintf("%s (%d)", k, counts[k]))
		}
		used := strings.Join(parts, ", ")
		if n := len(parts); n > 1 {
			used = strings.Join(parts[:n-1], ", ") + " or " + parts[n-1]
		}
		summary = append(summary,
			quiet.Render(fmt.Sprintf("%d schema part%s use%s %s.", total, plural(total, "", "s"), plural(total, "s", ""), used)),
			quiet.Render("They display normally; responses are just not checked against them."))
	}
	for i, d := range other {
		if i == 3 {
			summary = append(summary, quiet.Render(fmt.Sprintf("…and %d more", len(other)-3)))
			break
		}
		summary = append(summary, warnStyle.Render(d))
	}
	for _, d := range other {
		details = append(details, warnStyle.Render(d))
	}
	if len(details) > 0 {
		details = append([]string{quiet.Render("Responses are not checked against these parts of the contract."), ""}, details...)
	}
	return summary, details
}

// notePlace turns a JSON Pointer into the API's own terms: an endpoint or
// a schema name, then where inside it.
func notePlace(pointer string) string {
	if rest, ok := strings.CutPrefix(pointer, "/paths/"); ok {
		for _, method := range []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"} {
			if i := strings.Index(rest+"/", "/"+method+"/"); i >= 0 {
				inside := strings.TrimPrefix(rest[min(len(rest), i+len(method)+1):], "/")
				return methodStyle(method).Render(strings.ToUpper(method)) + " " + rest[:i] + quiet.Render("  "+inside)
			}
		}
	}
	for _, prefix := range []string{"/components/schemas/", "/definitions/"} {
		if rest, ok := strings.CutPrefix(pointer, prefix); ok {
			name, inside, _ := strings.Cut(rest, "/")
			return "schema " + bold.Render(name) + quiet.Render("  "+inside)
		}
	}
	return quiet.Render(pointer)
}

// relevantNotes keeps the importer notes that concern one endpoint or schema.
func relevantNotes(diagnostics []string, prefixes ...string) []string {
	var out []string
	for _, d := range diagnostics {
		if d == inspectionNote {
			out = append(out, d)
			continue
		}
		for _, p := range prefixes {
			if strings.Contains(d, p) {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

func (m *Model) refreshSource() tea.Cmd {
	source := m.selectedSource()
	if source == nil {
		m.say("Import an API first")
		return nil
	}
	root, id, opts := m.root, source.ID, m.sourceOptions(source.Location)
	return m.waitFor("Reading "+source.Location+"…", func() applyMsg {
		preview, err := project.RefreshSpecification(root, id, "", true, opts)
		return func(m *Model) tea.Cmd {
			if err != nil {
				m.fail(err)
				return nil
			}
			m.confirmRefresh(preview, source.Location, opts)
			return nil
		}
	})
}

func (m *Model) confirmRefresh(p project.RefreshPreview, location string, opts project.SourceImportOptions) {
	lines := []string{quiet.Render(location), "",
		fmt.Sprintf("%s added, %s changed, %s removed", okStyle.Render(strconv.Itoa(len(p.Added))), warnStyle.Render(strconv.Itoa(len(p.Changed))), badStyle.Render(strconv.Itoa(len(p.Removed))))}
	for _, group := range []struct {
		name  string
		style lipglossStyle
		eps   []project.Endpoint
	}{{"+", okStyle, p.Added}, {"~", warnStyle, p.Changed}, {"−", badStyle, p.Removed}} {
		for _, e := range group.eps {
			lines = append(lines, "  "+group.style.Render(group.name)+" "+methodStyle(e.Method).Render(pad(e.Method, 7))+" "+e.Path)
		}
	}
	if len(p.Cases) > 0 {
		lines = append(lines, "", "Saved requests affected")
		for _, c := range p.Cases {
			lines = append(lines, "  "+c.Name)
		}
	}
	for _, d := range p.Diagnostics {
		lines = append(lines, quiet.Render(d))
	}
	c := &confirmModal{title: "Apply the new API version?", lines: lines, yes: "apply", enterOK: true}
	if len(p.Conflicts) > 0 {
		c.title = "This refresh has conflicts"
		c.lines = append(c.lines, "", badStyle.Render("Local contract edits conflict with the new version:"))
		for _, conflict := range p.Conflicts {
			c.lines = append(c.lines, badStyle.Render("  "+conflict))
		}
		c.lines = append(c.lines, "", quiet.Render("Resolve these before refreshing."))
		c.yes = ""
	}
	c.onYes = func(m *Model) tea.Cmd {
		if _, err := project.RefreshSpecification(m.root, p.SourceID, p.Token, false, opts); err != nil {
			m.fail(err)
			return nil
		}
		m.sync()
		m.ok("API refreshed. Saved requests keep their inputs.")
		return nil
	}
	m.modal = c
}

type contractDraft struct {
	Kind                 string `json:"kind" yaml:"kind"`
	Action               string `json:"action" yaml:"action"`
	project.ContractEdit `yaml:",inline"`
}

func (m *Model) editContract() tea.Cmd {
	source := m.selectedSource()
	if source == nil {
		m.say("Import an API first")
		return nil
	}
	d := contractDraft{Kind: "endpoint", Action: "create", ContractEdit: project.ContractEdit{SourceID: source.ID, Revision: source.Revision, Path: "/new", Method: "GET", Value: map[string]any{"responses": map[string]any{"200": map[string]any{"description": "OK"}}}}}
	if m.schemas {
		d.Kind, d.Path, d.Method, d.Name, d.Value = "schema", "", "", "NewSchema", map[string]any{"type": "object"}
	}
	b := toYAML(d)
	text := "# Change the contract locally; the source file is not touched.\n# kind: endpoint or schema. action: create, update or delete.\n# Set importedEdit: true to change something that came from the import.\n" + string(b)
	return m.editText(text, "yaml", func(m *Model, text string) (tea.Cmd, error) {
		var d contractDraft
		if err := project.Decode([]byte(text), &d); err != nil {
			return nil, err
		}
		impact, err := project.EditContract(m.root, d.Kind, d.Action, d.ContractEdit, true, m.credentials)
		if err != nil {
			return nil, err
		}
		lines := []string{fmt.Sprintf("%s %s", d.Action, d.Kind), ""}
		if len(impact.Cases)+len(impact.Scenarios) == 0 {
			lines = append(lines, quiet.Render("No saved requests or scenarios are affected."))
		}
		for _, c := range impact.Cases {
			lines = append(lines, "Saved request  "+c.Name)
		}
		for _, s := range impact.Scenarios {
			lines = append(lines, "Scenario       "+s.Name)
		}
		m.modal = &confirmModal{title: "Apply this contract change?", lines: lines, yes: "apply", enterOK: true, onYes: func(m *Model) tea.Cmd {
			if _, err := project.EditContract(m.root, d.Kind, d.Action, d.ContractEdit, false, m.credentials); err != nil {
				m.fail(err)
				return nil
			}
			m.sync()
			m.ok("Contract changed")
			return nil
		}}
		return nil, nil
	})
}

func (m *Model) exportContract() tea.Cmd {
	source := m.selectedSource()
	if source == nil {
		m.say("Import an API first")
		return nil
	}
	data, err := project.ExportContract(m.root, source.ID, m.credentials)
	if err != nil {
		m.fail(err)
		return nil
	}
	b := toYAML(data)
	text := m.credentials.Sanitize(string(b))
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		lines = append(lines, yamlLine(line))
	}
	m.modal = &viewerModal{title: "Contract documents", lines: lines, copyText: text}
	m.ok("Copied the contract documents. Saved requests and scenarios are not part of OpenAPI.")
	return tea.SetClipboard(text)
}
