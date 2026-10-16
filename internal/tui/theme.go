package tui

import (
	"bytes"
	"encoding/json"
	"image/color"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The palette follows jira-tui: colours are named for what they mark rather
// than for what they are, and each has a light and a dark variant so the same
// build reads on either terminal background.
type palette struct {
	// accent marks structure: frames, pane titles, column headers.
	accent color.Color
	// identity marks what a row is called: an endpoint path, a JSON key.
	identity color.Color
	// state marks where something is: safe methods, successful responses.
	state color.Color
	// note marks quieter cells read only once the eye is on the row.
	note                     color.Color
	success, warn, danger    color.Color
	selectionBG, selectionFG color.Color
}

func paletteFor(dark bool) palette {
	pick, c := lipgloss.LightDark(dark), lipgloss.Color
	return palette{
		accent:      pick(c("#1a6fd4"), c("#4aa3ff")),
		identity:    pick(c("#7b3fb5"), c("#c58af9")),
		state:       pick(c("#0f7b6c"), c("#5bd6c0")),
		note:        pick(c("#5c6773"), c("#8a94a6")),
		success:     pick(c("#1a7f37"), c("#56d364")),
		warn:        pick(c("#9a6700"), c("#e3b341")),
		danger:      pick(c("#cf222e"), c("#ff7b72")),
		selectionBG: pick(c("#1a6fd4"), c("#1f6feb")),
		selectionFG: c("#ffffff"),
	}
}

var (
	accentStyle, mutedStyle, stateStyle, identityStyle lipgloss.Style
	successStyle, warnStyle, dangerStyle               lipgloss.Style
	borderStyle, titleStyle, hintStyle, headerStyle    lipgloss.Style
	selectStyle, statusStyle, errorStyle, pendingStyle lipgloss.Style
)

func init() { applyTheme(true) }

// applyTheme rebuilds the styles once the terminal reports its background;
// until then the dark variant is assumed, as most terminals are.
func applyTheme(dark bool) {
	p := paletteFor(dark)
	style := lipgloss.NewStyle
	accentStyle = style().Foreground(p.accent)
	mutedStyle = style().Foreground(p.note)
	stateStyle = style().Foreground(p.state)
	identityStyle = style().Foreground(p.identity)
	successStyle = style().Foreground(p.success)
	warnStyle = style().Foreground(p.warn)
	dangerStyle = style().Foreground(p.danger)
	borderStyle = style().Foreground(p.accent)
	titleStyle = style().Foreground(p.accent).Bold(true)
	hintStyle = style().Foreground(p.note)
	headerStyle = style().Foreground(p.accent).Bold(true)
	// A solid band survives terminal themes with their own reverse-video.
	selectStyle = style().Foreground(p.selectionFG).Background(p.selectionBG)
	statusStyle = style().Faint(true)
	errorStyle = style().Foreground(p.danger).Bold(true)
	// Pending keys are bright: the next keypress will not mean what it usually does.
	pendingStyle = style().Foreground(p.accent).Bold(true)
}

// selectedMarker leads the selected row, as in jira-tui.
const selectedMarker = "▸ "

// cell measures terminal columns, not bytes, and never wraps a finished row.
func cell(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = ansi.Truncate(text, width, "…")
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

// panel draws its own frame to keep dimensions exact, even with ANSI styles,
// wide characters, tiny terminals, or titles embedded in the top rule. Only the
// focused frame is drawn in the accent colour; that is what says where a
// keypress will land when several panes are up.
func panel(body, title, hint string, width, height int, focused bool) string {
	if width < 4 || height < 3 {
		return fit(body, max(1, width), max(1, height))
	}
	border, name := mutedStyle, mutedStyle
	if focused {
		border, name = borderStyle, titleStyle
	}
	inner := width - 2
	lines := strings.Split(fit(body, inner, height-2), "\n")
	for i, line := range lines {
		lines[i] = border.Render("│") + cell(line, inner) + border.Render("│")
	}
	return border.Render("╭") + topEdge(title, hint, inner, border, name) + border.Render("╮") + "\n" +
		strings.Join(lines, "\n") + "\n" + border.Render("╰"+strings.Repeat("─", inner)+"╯")
}

// topEdge sets the name one rule clear of the left corner and the hint one
// space clear of the right. The hint is dropped rather than truncated: half a
// key in a border reads as corruption. The name wins the last columns.
func topEdge(title, hint string, width int, border, name lipgloss.Style) string {
	if title == "" || width < 6 {
		return border.Render(strings.Repeat("─", width))
	}
	title = ansi.Truncate(title, width-4, "…")
	rule := width - ansi.StringWidth(title) - 3
	if hint != "" && ansi.StringWidth(hint)+3 <= rule {
		rule -= ansi.StringWidth(hint) + 2
		return border.Render("─ ") + name.Render(title) +
			border.Render(" "+strings.Repeat("─", rule)+" ") + hintStyle.Render(hint) + border.Render(" ")
	}
	return border.Render("─ ") + name.Render(title) + border.Render(" "+strings.Repeat("─", rule))
}

func (m *Model) keyHint(action string) string {
	for _, binding := range m.bindings {
		if binding.Action == action && len(binding.Keys) > 0 {
			return binding.Keys[0]
		}
	}
	return ""
}

func (m *Model) paneHint(pane paneID) string {
	actions := map[paneID]string{endpointsPane: "endpoints", requestPane: "request", responsePane: "response", historyPane: "history", projectPane: "project", environmentsPane: "environments", casesPane: "cases", scenariosPane: "scenarios", schemasPane: "schemas"}
	return m.keyHint(actions[pane])
}

// keyLine documents one key the way the continuation menu does.
func keyLine(key, label string) string {
	return pendingStyle.Render(key) + mutedStyle.Render(" → ") + label
}

// statusLine is the bottom row: the mode, the workspace and either feedback
// or the selection position, with half-typed keys written into the right end
// as vim does.
func (m *Model) statusLine(width int) string {
	var mode string
	switch m.mode {
	case "INSERT":
		mode = stateStyle.Bold(true).Render("INSERT")
	case "SEARCH":
		mode = pendingStyle.Render("SEARCH")
	default:
		mode = pendingStyle.Render(m.mode)
	}
	left := mode + "  " + m.credentials.Sanitize(m.workspaceName())
	if m.dirty || m.requestDirty() {
		left += warnStyle.Render(" [+]")
	}
	if m.zoom {
		left += mutedStyle.Render(" [zoom]")
	}
	switch {
	case m.mode == "SEARCH":
		query := "/" + m.query
		if m.globalSearch {
			query += mutedStyle.Render("  global")
		}
		left += "  " + query
	case m.status != "":
		left += "  " + statusStyle.Render(m.credentials.Sanitize(m.status))
	default:
		if n := len(m.items()); n > 0 && m.pane != responsePane && m.pane != requestPane {
			left += "  " + statusStyle.Render(strconv.Itoa(min(m.index, n-1)+1)+"/"+strconv.Itoa(n))
		}
	}
	right := pendingStyle.Render(m.pending)
	if m.pending == "" {
		if key := m.keyHint("help"); key != "" {
			right = hintStyle.Render(key + " help")
		} else {
			right = ""
		}
	}
	room := width - ansi.StringWidth(right) - 1
	if right == "" || room < 1 {
		return cell(left, width)
	}
	return cell(left, room) + " " + right
}

func (m *Model) workspaceName() string {
	if m.draft == "" {
		return "Local API workspace"
	}
	return m.draft
}

// destinationLine says where a request would go. It wraps rather than
// truncates: the URL is the one thing on screen that must never be cut off.
func destinationLine(text string, width int) string {
	parts := strings.Split(text, " | ")
	style := stateStyle
	if len(parts) > 1 && strings.Contains(parts[1], "production") && !strings.Contains(parts[1], "nonproduction") {
		style = warnStyle.Bold(true)
	}
	if text == "Env: choose" || strings.Contains(text, "invalid") {
		style = warnStyle
	}
	head := style.Render(parts[0])
	for i := 1; i < len(parts); i++ {
		parts[i] = mutedStyle.Render(" · ") + style.Render(parts[i])
	}
	text = head + strings.Join(parts[1:], "")
	return ansi.Wrap(text, max(1, width), "")
}

var (
	jsonKey    = regexp.MustCompile(`^(\s*)("(?:[^"\\]|\\.)*")(:)(.*)$`)
	methodWord = regexp.MustCompile(`^(\s*)(GET|HEAD|OPTIONS|POST|PUT|PATCH|DELETE)(\s)`)
)

func methodStyle(method string) lipgloss.Style {
	switch method {
	case "POST":
		return successStyle.Bold(true)
	case "PUT", "PATCH":
		return warnStyle.Bold(true)
	case "DELETE":
		return dangerStyle.Bold(true)
	default:
		return stateStyle.Bold(true)
	}
}

func httpStatusStyle(status int) lipgloss.Style {
	switch {
	case status >= 400:
		return dangerStyle.Bold(true)
	case status >= 300:
		return accentStyle.Bold(true)
	default:
		return successStyle.Bold(true)
	}
}

// styledBody colours plain pane text line by line. A "> " row is the
// selection: a solid band the width of the pane when focused, and an ordinary
// row otherwise, since an unfocused pane's cursor is not where keys land.
func styledBody(body string, width int, focused bool) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if rest, ok := strings.CutPrefix(line, "> "); ok {
			if focused {
				lines[i] = selectStyle.Render(cell(selectedMarker+ansi.Strip(rest), width))
				continue
			}
			line = "  " + rest
		}
		lines[i] = styledLine(line)
	}
	return strings.Join(lines, "\n")
}

func styledLine(line string) string {
	if strings.HasPrefix(line, "HTTP ") && len(strings.Fields(line)) > 1 {
		status, _ := strconv.Atoi(strings.Fields(line)[1])
		return httpStatusStyle(status).Render(line)
	}
	if match := jsonKey.FindStringSubmatch(line); match != nil {
		return match[1] + identityStyle.Render(match[2]) + mutedStyle.Render(match[3]) + match[4]
	}
	if strings.HasPrefix(line, "No ") || strings.HasPrefix(line, "CLI access") || strings.HasPrefix(line, "    ") {
		return mutedStyle.Render(line)
	}
	if match := methodWord.FindStringSubmatch(line); match != nil {
		line = match[1] + methodStyle(match[2]).Render(match[2]) + match[3] + line[len(match[0]):]
	}
	// Key reminders ("Enter save | Esc cancel") are quiet; they say how to
	// leave, not what is there.
	if strings.Contains(line, " | ") && (strings.Contains(line, "Esc ") || strings.Contains(line, "Enter ")) {
		return hintStyle.Render(line)
	}
	return line
}

// The editor keeps the exact draft; pretty printing is presentation-only.
func prettyDraft(text string) string {
	var out bytes.Buffer
	if json.Indent(&out, []byte(text), "", "  ") == nil {
		return out.String()
	}
	return text
}

// endpointRow is one endpoint label split back into the cells it was built from.
type endpointRow struct{ tag, method, path, summary, source string }

func parseEndpoint(label string) endpointRow {
	var r endpointRow
	if strings.HasPrefix(label, "[") {
		if end := strings.Index(label, "] "); end > 0 {
			r.tag, label = label[1:end], label[end+2:]
		}
	}
	parts := strings.Split(label, " | ")
	r.method, r.path, _ = strings.Cut(parts[0], " ")
	if len(parts) > 1 {
		r.summary = parts[1]
	}
	if len(parts) > 2 {
		r.source = parts[2]
	}
	return r
}

type column struct {
	title        string
	min, natural int
	flex         bool
	style        func(endpointRow) lipgloss.Style
	value        func(endpointRow) string
}

const columnGap = 1

// endpointTable lays endpoints out as jira-tui lays out work items: a bold
// header, right-aligned row numbers, each column in its own colour, and the
// selection as a full-width band. Columns that cannot fit their minimum are
// dropped from the right; dropping beats a column of ellipses.
func endpointTable(labels []string, selected, width, height int, focused bool) string {
	rows := make([]endpointRow, len(labels))
	tagged := false
	for i, label := range labels {
		rows[i] = parseEndpoint(label)
		tagged = tagged || rows[i].tag != ""
	}
	plain := func(endpointRow) lipgloss.Style { return lipgloss.NewStyle() }
	cols := []column{
		{title: "METHOD", min: 6, value: func(r endpointRow) string { return r.method }, style: func(r endpointRow) lipgloss.Style { return methodStyle(r.method) }},
		{title: "PATH", min: 8, value: func(r endpointRow) string { return r.path }, style: func(endpointRow) lipgloss.Style { return identityStyle }},
		{title: "SUMMARY", min: 10, flex: true, value: func(r endpointRow) string { return r.summary }, style: plain},
		{title: "SOURCE", min: 8, natural: 24, value: func(r endpointRow) string { return r.source }, style: func(endpointRow) lipgloss.Style { return mutedStyle }},
	}
	if tagged {
		tag := column{title: "TAG", min: 4, natural: 16, value: func(r endpointRow) string { return r.tag }, style: func(endpointRow) lipgloss.Style { return accentStyle }}
		cols = append([]column{tag}, cols...)
	}
	numWidth := len(strconv.Itoa(len(rows))) + columnGap
	widths := tableLayout(cols, rows, width-ansi.StringWidth(selectedMarker)-numWidth)

	selected = max(0, min(selected, len(rows)-1))
	visible := max(1, height-1)
	// When the source column did not fit, the selected endpoint keeps its
	// source on the line beneath it: which file an endpoint came from must
	// stay readable at any width.
	var sourceLines []string
	if focused && widths[len(widths)-1] == 0 && rows[selected].source != "" {
		indent := strings.Repeat(" ", ansi.StringWidth(selectedMarker)+numWidth)
		for _, line := range strings.Split(ansi.Wrap(rows[selected].source, max(1, width-len(indent)), "/"), "\n") {
			sourceLines = append(sourceLines, mutedStyle.Render(cell(indent+line, width)))
		}
		visible = max(1, visible-len(sourceLines))
	}
	start := max(0, selected-visible+1)
	number := func(text string) string {
		return strings.Repeat(" ", max(0, numWidth-columnGap-len(text))) + text + strings.Repeat(" ", columnGap)
	}
	render := func(r endpointRow, styled bool) string {
		var b strings.Builder
		for i, c := range cols {
			if widths[i] == 0 {
				continue
			}
			if b.Len() > 0 {
				b.WriteString(strings.Repeat(" ", columnGap))
			}
			text := cell(c.value(r), widths[i])
			if styled {
				text = c.style(r).Render(text)
			}
			b.WriteString(text)
		}
		return b.String()
	}
	header := make([]string, 0, len(cols))
	for i, c := range cols {
		if widths[i] > 0 {
			header = append(header, cell(c.title, widths[i]))
		}
	}
	lines := []string{headerStyle.Render(cell("  "+number("#")+strings.Join(header, strings.Repeat(" ", columnGap)), width))}
	for i := start; i < len(rows) && len(lines) < height; i++ {
		if i == selected && focused {
			lines = append(lines, selectStyle.Render(cell(selectedMarker+number(strconv.Itoa(i+1))+render(rows[i], false), width)))
			lines = append(lines, sourceLines...)
			continue
		}
		lines = append(lines, "  "+mutedStyle.Render(number(strconv.Itoa(i+1)))+render(rows[i], true))
	}
	return strings.Join(lines, "\n")
}

// tableLayout gives each column what its content needs, capped by its
// natural width, drops columns from the right until the minimums fit, and
// hands the remaining width to the flexible column so the right edge is flush.
func tableLayout(cols []column, rows []endpointRow, width int) []int {
	widths := make([]int, len(cols))
	shown := len(cols)
	minimum := func(n int) int {
		total := columnGap * (n - 1)
		for _, c := range cols[:n] {
			total += c.min
		}
		return total
	}
	natural := make([]int, len(cols))
	for i, c := range cols {
		w := ansi.StringWidth(c.title)
		for _, r := range rows {
			w = max(w, ansi.StringWidth(c.value(r)))
		}
		if c.natural > 0 {
			w = min(w, c.natural)
		}
		natural[i] = max(w, c.min)
	}
	// A trailing column that would squeeze the ones before it below their
	// content is dropped instead: a path cut short is worse than no source.
	needed := func(n int) int {
		total := columnGap * (n - 1)
		for i, c := range cols[:n] {
			if c.flex {
				total += c.min
			} else {
				total += natural[i]
			}
		}
		return total
	}
	for shown > 1 && (minimum(shown) > width || shown > 2 && needed(shown) > width && !cols[shown-1].flex) {
		shown--
	}
	flex := -1
	for i, c := range cols[:shown] {
		widths[i] = natural[i]
		if c.flex {
			flex = i
		}
	}
	if flex < 0 {
		flex = shown - 1
	}
	used := columnGap * (shown - 1)
	for i := range shown {
		if i != flex {
			used += widths[i]
		}
	}
	widths[flex] = max(width-used, 0)
	// Too narrow for every column's content: take from the widest fixed
	// columns, never below their minimum, until the flexible one has its own.
	for widths[flex] < cols[flex].min {
		widest := -1
		for i := range shown {
			if i != flex && widths[i] > cols[i].min && (widest < 0 || widths[i] > widths[widest]) {
				widest = i
			}
		}
		if widest < 0 {
			break
		}
		widths[widest]--
		widths[flex]++
	}
	return widths
}

// whichKey is the continuation menu for a pending prefix, as jira-tui draws
// it: a small framed float in the bottom right, titled with the keys typed so
// far, that leaves what was being read where it was.
func (m *Model) whichKey(width, height int) string {
	if m.pending == "" {
		return ""
	}
	next := continuations(m.bindings, m.pending)
	keyWidth, labelWidth := 0, 0
	for _, c := range next {
		keyWidth = max(keyWidth, ansi.StringWidth(c.key))
		labelWidth = max(labelWidth, ansi.StringWidth(c.label))
	}
	rows := make([]string, 0, len(next))
	for _, c := range next {
		label := c.label
		if c.group {
			label = mutedStyle.Render(label)
		}
		rows = append(rows, " "+pendingStyle.Render(cell(c.key, keyWidth))+mutedStyle.Render(" → ")+label)
	}
	if len(rows) == 0 {
		rows = append(rows, mutedStyle.Render(" no continuations"))
	}
	boxWidth := min(width-4, max(keyWidth+labelWidth+6, ansi.StringWidth(m.pending)+ansi.StringWidth("esc cancel")+9))
	boxHeight := min(height-3, len(rows)+2)
	if boxWidth < 4 || boxHeight < 3 {
		return ""
	}
	return panel(strings.Join(rows, "\n"), m.pending, "esc cancel", boxWidth, boxHeight, true)
}
