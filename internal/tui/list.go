package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

// listState is the cursor, scroll position and filter of one tab's list.
type listState struct {
	cursor, offset int
	filter         *textInput
	filtering      bool
	// page is the number of rows that fit, measured at the last draw.
	page int
}

func (l *listState) query() string {
	if l.filter == nil {
		return ""
	}
	return strings.TrimSpace(l.filter.String())
}

func (l *listState) startFilter() {
	if l.filter == nil {
		l.filter = newInput("", "type to filter")
	}
	l.filtering = true
}

func (l *listState) clearFilter() {
	l.filter, l.filtering = nil, false
	l.cursor, l.offset = 0, 0
}

// cursorItem counts selectable rows before the cursor, for the n/total readout.
func (l *listState) cursorItem(m *Model) int {
	src := m.source()
	n := 0
	for i := 0; i < min(l.cursor, src.n); i++ {
		if !src.header(i) {
			n++
		}
	}
	return n
}

// source describes the current tab's rows without building them all: rows
// are rendered one at a time, and only the ones on screen.
type source struct {
	n      int
	header func(i int) bool
	render func(i, width int, selected bool) string
	empty  []string
}

func noHeaders(int) bool { return false }

func (m *Model) source() source {
	switch m.tab {
	case endpointsTab:
		if m.schemas {
			return m.schemaSource()
		}
		return m.endpointSource()
	case savedTab:
		return m.caseSource()
	case scenariosTab:
		return m.scenarioSource()
	case historyTab:
		return m.historySource()
	}
	return m.envSource()
}

func (m *Model) listLen() int { return m.source().n }

func (m *Model) listCount() int {
	src := m.source()
	n := 0
	for i := range src.n {
		if !src.header(i) {
			n++
		}
	}
	return n
}

func (m *Model) clampLists() {
	saved := m.tab
	for t := range tabCount {
		m.tab = t
		l := &m.lists[t]
		n := m.listLen()
		l.cursor = max(0, min(l.cursor, n-1))
		m.skipHeader(1)
	}
	m.tab = saved
}

// skipHeader moves the cursor off a group heading in the given direction.
func (m *Model) skipHeader(dir int) {
	src := m.source()
	l := &m.lists[m.tab]
	if src.n == 0 {
		l.cursor = 0
		return
	}
	for l.cursor >= 0 && l.cursor < src.n && src.header(l.cursor) {
		l.cursor += dir
	}
	if l.cursor < 0 || l.cursor >= src.n {
		l.cursor = max(0, min(l.cursor, src.n-1))
		for l.cursor >= 0 && l.cursor < src.n && src.header(l.cursor) {
			l.cursor -= dir
		}
		l.cursor = max(0, min(l.cursor, src.n-1))
	}
}

func (m *Model) navigate(id action) {
	if m.screen == requestScreen && m.req != nil {
		if m.respFocus {
			m.req.scrollResponse(id, m.pageSize())
		} else {
			m.req.moveCursor(id, m.pageSize())
		}
		return
	}
	l := &m.lists[m.tab]
	page := max(1, l.page-1)
	if m.detailFocus {
		m.detailScroll += map[action]int{actUp: -1, actDown: 1, actPageDown: page, actPageUp: -page, actTop: -1 << 30, actBottom: 1 << 30}[id]
		m.detailScroll = max(0, m.detailScroll)
		return
	}
	// A new selection starts its details from the top.
	m.detailScroll = 0
	n := m.listLen()
	dir := 1
	switch id {
	case actUp:
		l.cursor--
		dir = -1
	case actDown:
		l.cursor++
	case actTop:
		l.cursor = 0
	case actBottom:
		l.cursor = n - 1
		dir = -1
	case actPageDown:
		l.cursor += page
	case actPageUp:
		l.cursor -= page
		dir = -1
	}
	l.cursor = max(0, min(l.cursor, n-1))
	m.skipHeader(dir)
	if m.tab == historyTab && l.cursor >= n-1 {
		m.loadMoreHistory()
	}
}

func (m *Model) pageSize() int { return max(1, m.height-6) }

// listView draws the filter line and the visible rows, keeping the cursor
// two rows clear of either edge where it can.
func (m *Model) listView(width, height int) []string {
	l := &m.lists[m.tab]
	src := m.source()
	lines := []string{m.listHeading(width, src)}
	rows := max(1, height-1)
	l.page = rows
	if src.n == 0 {
		if l.query() == "" && !l.filtering && m.methodFilter == "" {
			lines[0] = ""
		}
		lines = append(lines, "")
		for _, line := range src.empty {
			lines = append(lines, "  "+line)
		}
		return lines
	}
	l.cursor = max(0, min(l.cursor, src.n-1))
	margin := min(2, rows/4)
	if l.cursor < l.offset+margin {
		l.offset = l.cursor - margin
	}
	if l.cursor >= l.offset+rows-margin {
		l.offset = l.cursor - rows + margin + 1
	}
	l.offset = max(0, min(l.offset, src.n-rows))
	for i := l.offset; i < min(src.n, l.offset+rows); i++ {
		selected := i == l.cursor && !src.header(i)
		gutter := " "
		if selected {
			gutter = accent.Render("▌")
		}
		lines = append(lines, gutter+src.render(i, width-1, selected))
	}
	return lines
}

// listHeading is the filter as it is typed, or what the list holds.
func (m *Model) listHeading(width int, src source) string {
	l := &m.lists[m.tab]
	count := m.listCount()
	var chips []string
	if m.tab == endpointsTab && !m.schemas {
		if m.methodFilter != "" {
			chips = append(chips, methodStyle(m.methodFilter).Render(m.methodFilter)+quiet.Render(" only"))
		}
		if m.groupByTag {
			chips = append(chips, quiet.Render("by tag"))
		}
	}
	right := strings.Join(chips, "   ")
	var left string
	switch {
	case l.filtering:
		left = accentBold.Render("/") + " " + l.filter.view(max(1, width-ansi.StringWidth(right)-16), true) + "  " + quiet.Render(fmt.Sprintf("%d match%s", count, plural(count, "", "es")))
	case l.query() != "":
		left = accent.Render("/") + " " + l.query() + "  " + quiet.Render(fmt.Sprintf("%d of %d", count, m.unfilteredCount()))
	default:
		left = quiet.Render(m.listNoun(count))
	}
	return " " + pad(left, max(1, width-ansi.StringWidth(right)-2)) + right
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (m *Model) listNoun(n int) string {
	nouns := [tabCount][2]string{{"endpoint", "endpoints"}, {"saved request", "saved requests"}, {"scenario", "scenarios"}, {"run", "runs"}, {"environment", "environments"}}
	noun := nouns[m.tab]
	if m.tab == endpointsTab && m.schemas {
		noun = [2]string{"schema", "schemas"}
	}
	text := strconv.Itoa(n) + " " + noun[1]
	if n == 1 {
		text = "1 " + noun[0]
	}
	if m.tab == historyTab && m.historyMeta.Total > n {
		text = fmt.Sprintf("%d of %d runs", n, m.historyMeta.Total)
	}
	return text
}

func (m *Model) unfilteredCount() int {
	switch m.tab {
	case endpointsTab:
		if m.schemas {
			n := 0
			for _, s := range m.data.specs {
				n += len(s.Schemas)
			}
			return n
		}
		n := 0
		for _, s := range m.data.specs {
			n += len(s.Endpoints)
		}
		return n
	case savedTab:
		return len(m.data.cases)
	case scenariosTab:
		return len(m.data.scenarios)
	case historyTab:
		return len(m.history)
	}
	return len(m.data.snapshot.Environments)
}

func matches(query string, fields ...string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

// endpointCache holds the filtered endpoint rows until the query, the method
// filter, grouping or the specifications change. Filtering runs once per
// keystroke, never once per frame.
type endpointCache struct {
	valid     bool
	specGen   int
	query     string
	method    string
	group     bool
	matches   []project.EndpointMatch
	rows      []int // index into matches, or -1 for a group heading
	headings  map[int]string
	pathWidth int
}

func (m *Model) endpoints() *endpointCache {
	c := &m.epCache
	q := m.lists[endpointsTab].query()
	if c.valid && c.specGen == m.data.specGen && c.query == q && c.method == m.methodFilter && c.group == m.groupByTag {
		return c
	}
	*c = endpointCache{valid: true, specGen: m.data.specGen, query: q, method: m.methodFilter, group: m.groupByTag, headings: map[int]string{}}
	c.matches = project.SearchEndpoints(m.data.specs, project.EndpointQuery{Query: q, Method: m.methodFilter, GroupByTag: m.groupByTag})
	last := "\x00"
	for i, match := range c.matches {
		if m.groupByTag && match.Group != last {
			c.headings[len(c.rows)] = match.Group
			c.rows = append(c.rows, -1)
			last = match.Group
		}
		c.rows = append(c.rows, i)
		c.pathWidth = max(c.pathWidth, ansi.StringWidth(match.Path))
	}
	return c
}

func (m *Model) selectedEndpoint() (project.EndpointMatch, bool) {
	c := m.endpoints()
	l := m.lists[endpointsTab]
	if l.cursor < 0 || l.cursor >= len(c.rows) || c.rows[l.cursor] < 0 {
		return project.EndpointMatch{}, false
	}
	return c.matches[c.rows[l.cursor]], true
}

func (m *Model) endpointSource() source {
	c := m.endpoints()
	src := source{n: len(c.rows), header: func(i int) bool { return c.rows[i] < 0 }}
	src.render = func(i, width int, selected bool) string {
		if c.rows[i] < 0 {
			return row([]span{sp(c.headings[i], bold)}, width, false)
		}
		e := c.matches[c.rows[i]]
		pathWidth := min(c.pathWidth, max(12, width*55/100))
		path := e.Path
		if ansi.StringWidth(path) > pathWidth {
			path = ansi.Truncate(path, pathWidth, "…")
		}
		spans := []span{sp(pad(e.Method, 7), methodStyle(e.Method)), plain(" ")}
		spans = append(spans, pathSpans(path)...)
		spans = append(spans, plain(strings.Repeat(" ", pathWidth-ansi.StringWidth(path)+2)), sp(e.Summary, quiet))
		return row(spans, width, selected)
	}
	switch {
	case len(m.data.specs) == 0:
		src.empty = []string{"No API imported yet.", "", hint(m.keyName(actImport), "import an OpenAPI file or a Swagger URL")}
	default:
		src.empty = m.noMatch()
	}
	return src
}

func (m *Model) noMatch() []string {
	if q := m.lists[m.tab].query(); q != "" {
		return []string{"Nothing matches “" + q + "”.", "", hint("esc", "clear the filter")}
	}
	if m.tab == endpointsTab && m.methodFilter != "" {
		return []string{"No " + m.methodFilter + " endpoints.", "", hint(m.keyName(actMethod), "show another method")}
	}
	return []string{"Nothing here yet."}
}

func pathSpans(path string) []span {
	locs := pathParam.FindAllStringIndex(path, -1)
	var out []span
	last := 0
	for _, loc := range locs {
		out = append(out, plain(path[last:loc[0]]), sp(path[loc[0]:loc[1]], accent))
		last = loc[1]
	}
	return append(out, plain(path[last:]))
}

type schemaRow struct {
	spec   *project.Specification
	schema project.Schema
}

type schemaCache struct {
	valid   bool
	specGen int
	query   string
	rows    []schemaRow
}

func (m *Model) schemaRows() []schemaRow {
	c := &m.schemaCache
	q := m.lists[endpointsTab].query()
	if c.valid && c.specGen == m.data.specGen && c.query == q {
		return c.rows
	}
	*c = schemaCache{valid: true, specGen: m.data.specGen, query: q}
	for i := range m.data.specs {
		s := &m.data.specs[i]
		for _, schema := range s.Schemas {
			if matches(q, schema.Name) {
				c.rows = append(c.rows, schemaRow{s, schema})
			}
		}
	}
	return c.rows
}

func (m *Model) schemaSource() source {
	rows := m.schemaRows()
	src := source{n: len(rows), header: noHeaders}
	src.render = func(i, width int, selected bool) string {
		r := rows[i]
		return row([]span{sp(r.schema.Name, bold), plain("  "), sp(m.moduleName(r.spec.ModuleID), quiet)}, width, selected)
	}
	src.empty = []string{"No schemas."}
	if len(m.data.specs) == 0 {
		src.empty = []string{"No API imported yet.", "", hint(m.keyName(actImport), "import an OpenAPI file or a Swagger URL")}
	} else if m.lists[endpointsTab].query() != "" {
		src.empty = m.noMatch()
	}
	return src
}

func (m *Model) filteredCases() []project.CaseResource {
	q := m.lists[savedTab].query()
	var out []project.CaseResource
	for _, c := range m.data.cases {
		if matches(q, c.Name, c.Request.URL, c.Request.Method) {
			out = append(out, c)
		}
	}
	return out
}

func (m *Model) caseSource() source {
	items := m.filteredCases()
	src := source{n: len(items), header: noHeaders}
	src.render = func(i, width int, selected bool) string {
		c := items[i]
		method, path := m.caseRoute(c.Request)
		spans := []span{sp(pad(method, 7), methodStyle(method)), plain(" "), sp(c.Name, bold), plain("  "), sp(path, quiet)}
		if d := m.drafts["case:"+c.ID]; d != nil && d.dirty() {
			spans = append(spans[:4], append([]span{sp("edited  ", warnStyle)}, spans[4:]...)...)
		}
		return row(spans, width, selected)
	}
	src.empty = m.noMatch()
	if len(m.data.cases) == 0 {
		src.empty = []string{"No saved requests yet.", "", "Open an endpoint, fill it in, then press " + accentBold.Render(m.keyName(actSave)) + " to keep it here."}
	}
	return src
}

// caseRoute shows a saved request by the contract endpoint it follows.
func (m *Model) caseRoute(d project.RequestDraft) (string, string) {
	method, path := d.Method, d.URL
	if e, ok := m.data.endpoints[d.EndpointID]; ok {
		method = e.Method
		if path == "" {
			path = e.Path
		}
	}
	if method == "" {
		method = "GET"
	}
	return strings.ToUpper(method), path
}

func (m *Model) filteredScenarios() []project.ScenarioResource {
	q := m.lists[scenariosTab].query()
	var out []project.ScenarioResource
	for _, s := range m.data.scenarios {
		if matches(q, s.Name) {
			out = append(out, s)
		}
	}
	return out
}

func (m *Model) scenarioSource() source {
	items := m.filteredScenarios()
	src := source{n: len(items), header: noHeaders}
	src.render = func(i, width int, selected bool) string {
		s := items[i]
		steps := fmt.Sprintf("%d step%s", len(s.Steps), plural(len(s.Steps), "", "s"))
		return row([]span{sp(s.Name, bold), plain("  "), sp(steps, quiet)}, width, selected)
	}
	src.empty = m.noMatch()
	if len(m.data.scenarios) == 0 {
		src.empty = []string{"No scenarios yet.", "", hint(m.keyName(actNewScenario), "chain saved requests into a scenario")}
	}
	return src
}

func (m *Model) filteredRuns() []project.Run {
	q := m.lists[historyTab].query()
	if q == "" {
		return m.history
	}
	var out []project.Run
	for _, r := range m.history {
		if matches(q, r.ID, r.Response.Method, r.Response.Destination, r.Outcome, strconv.Itoa(r.Response.Status)) {
			out = append(out, r)
		}
	}
	return out
}

func runTime(t time.Time) string {
	t = t.Local()
	if y, mo, d := t.Date(); y == time.Now().Year() && mo == time.Now().Month() && d == time.Now().Day() {
		return t.Format("15:04:05")
	}
	return t.Format("Jan 02 15:04")
}

func (m *Model) historySource() source {
	items := m.filteredRuns()
	src := source{n: len(items), header: noHeaders}
	src.render = func(i, width int, selected bool) string {
		r := items[i]
		pin := "  "
		if r.Pinned {
			pin = "◆ "
		}
		mark := r.ID == m.compareFrom
		spans := []span{sp(pin, accent), sp(pad(runTime(r.StartedAt), 12), quiet), plain(" ")}
		if r.ScenarioID != "" {
			spans = append(spans, sp(pad(r.Outcome, 7), outcomeStyle(r.Outcome)), plain(" "), sp("scenario  ", quiet), plain(m.scenarioName(r.ScenarioID)))
		} else {
			status := "—"
			if r.Response.Status != 0 {
				status = strconv.Itoa(r.Response.Status)
			}
			spans = append(spans, sp(pad(status, 4), statusStyle(r.Response.Status)), sp(pad(r.Response.Method, 7), methodStyle(r.Response.Method)), plain(" "), plain(shortURL(r.Response.Destination)))
			if r.Response.Status == 0 {
				spans = append(spans, plain("  "), sp(r.Outcome, outcomeStyle(r.Outcome)))
			}
		}
		if mark {
			spans = append(spans, plain("  "), sp("comparing from here", accentBold))
		}
		return row(spans, width, selected)
	}
	src.empty = m.noMatch()
	if len(m.history) == 0 {
		src.empty = []string{"No runs yet.", "", "Requests you send are kept here."}
	}
	return src
}

func outcomeStyle(outcome string) lipglossStyle {
	switch outcome {
	case "passed", "succeeded", "success", "completed":
		return okStyle
	case "cancelled", "skipped":
		return quiet
	}
	return badStyle
}

// shortURL drops the scheme; the host and path are what tell runs apart.
func shortURL(raw string) string {
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	return raw
}

func (m *Model) scenarioName(id string) string {
	for _, s := range m.data.scenarios {
		if s.ID == id {
			return s.Name
		}
	}
	return id
}

func (m *Model) filteredEnvs() []project.Resource {
	q := m.lists[envTab].query()
	var out []project.Resource
	for _, e := range m.environments() {
		if matches(q, e.Name, e.Classification) {
			out = append(out, e)
		}
	}
	return out
}

func (m *Model) envSource() source {
	items := m.filteredEnvs()
	nameWidth := 0
	for _, e := range items {
		nameWidth = max(nameWidth, ansi.StringWidth(e.Name))
	}
	src := source{n: len(items), header: noHeaders}
	src.render = func(i, width int, selected bool) string {
		e := items[i]
		mark := sp("  ", quiet)
		if e.ID == m.data.activeEnv {
			mark = sp("● ", okStyle)
		}
		name := sp(pad(e.Name, nameWidth), bold)
		if e.Classification == "production" {
			name.style = delStyle
		}
		set := 0
		for _, u := range e.BaseURLs {
			if u != "" {
				set++
			}
		}
		urls := sp("no base URL", warnStyle)
		if set > 0 {
			urls = sp(fmt.Sprintf("%d base URL%s", set, plural(set, "", "s")), quiet)
		}
		return row([]span{mark, name, plain("  "), urls}, width, selected)
	}
	src.empty = m.noMatch()
	if len(m.data.snapshot.Environments) == 0 {
		src.empty = []string{"No environments.", "", hint(m.keyName(actNewEnv), "create one")}
	}
	return src
}

func (m *Model) selectedCase() (project.CaseResource, bool) {
	items := m.filteredCases()
	l := m.lists[savedTab]
	if len(items) == 0 {
		return project.CaseResource{}, false
	}
	return items[min(l.cursor, len(items)-1)], true
}

func (m *Model) selectedScenario() (project.ScenarioResource, bool) {
	items := m.filteredScenarios()
	if len(items) == 0 {
		return project.ScenarioResource{}, false
	}
	return items[min(m.lists[scenariosTab].cursor, len(items)-1)], true
}

func (m *Model) selectedRun() (project.Run, bool) {
	items := m.filteredRuns()
	if len(items) == 0 {
		return project.Run{}, false
	}
	return items[min(m.lists[historyTab].cursor, len(items)-1)], true
}

func (m *Model) selectedEnv() (project.Resource, bool) {
	items := m.filteredEnvs()
	if len(items) == 0 {
		return project.Resource{}, false
	}
	return items[min(m.lists[envTab].cursor, len(items)-1)], true
}

func (m *Model) selectedSchema() (schemaRow, bool) {
	rows := m.schemaRows()
	if len(rows) == 0 {
		return schemaRow{}, false
	}
	return rows[min(m.lists[endpointsTab].cursor, len(rows)-1)], true
}
