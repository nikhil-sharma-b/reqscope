package tui

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

// A modal takes every key until it closes. Each one says, on its last line,
// exactly which keys it accepts.
type modal interface {
	update(m *Model, msg tea.KeyPressMsg) tea.Cmd
	// content returns the title, the body lines and the key hints for a box
	// at most width columns wide.
	content(m *Model, width, height int) (title string, body []string, hints []string)
	preferredWidth() int
}

func (m *Model) modalBox(width, height int) []string {
	if width < 24 || height < 7 {
		return nil
	}
	inner := min(m.modal.preferredWidth(), width-4)
	if inner < 10 {
		inner = max(1, width-2)
	}
	title, body, hints := m.modal.content(m, inner-2, max(1, height-6))
	maxBody := max(1, height-5)
	if len(body) > maxBody {
		body = body[:maxBody]
	}
	border := accent
	top := border.Render("╭─ ") + bold.Render(clip(title, inner-4)) + " "
	top += border.Render(strings.Repeat("─", max(0, inner-ansi.StringWidth(ansi.Strip(top))+1)) + "╮")
	lines := []string{top}
	side := border.Render("│")
	lines = append(lines, side+strings.Repeat(" ", inner)+side)
	for _, line := range body {
		lines = append(lines, side+" "+pad(line, inner-2)+" "+side)
	}
	pos := ""
	if s, ok := m.modal.(scrolled); ok {
		pos = s.scrollNote()
	}
	// A rule, not a blank line, parts the content from the keys, and the
	// scroll position sits in it: neither can be read as part of the body.
	if len(hints) > 0 {
		lines = append(lines, border.Render("├")+edgeNote(pos, inner, ruleStyle)+border.Render("┤"))
		lines = append(lines, side+" "+pad(strings.Join(hints, "   "), inner-2)+" "+side)
		pos = ""
	}
	lines = append(lines, border.Render("╰")+edgeNote(pos, inner, border)+border.Render("╯"))
	return lines
}

// confirmModal asks before anything that writes, sends, deletes or grants.
// Deleting and granting take y; previews of an import or change take Enter
// too, since Enter is how the person got there.
type confirmModal struct {
	// private hides the screen behind the box, for confirmations whose
	// sanitized summary is the only thing that should be on show.
	private bool
	title   string
	lines   []string
	yes     string
	enterOK bool
	danger  bool
	err     string
	onYes   func(m *Model) tea.Cmd
	onNo    func(m *Model) tea.Cmd
	extra   []extraKey
	scroll  int
	pos     string
}

type extraKey struct {
	key, label string
	run        func(m *Model) tea.Cmd
}

func (c *confirmModal) preferredWidth() int { return 76 }

func (c *confirmModal) update(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	for _, x := range c.extra {
		if key == x.key {
			m.modal = nil
			return x.run(m)
		}
	}
	switch key {
	case "y":
		if c.yes != "" {
			m.modal = nil
			return c.onYes(m)
		}
	case "enter":
		if c.yes != "" && c.enterOK {
			m.modal = nil
			return c.onYes(m)
		}
	case "esc", "n":
		m.modal = nil
		if c.onNo != nil {
			return c.onNo(m)
		}
	case "j", "down":
		c.scroll++
	case "k", "up":
		c.scroll = max(0, c.scroll-1)
	}
	return nil
}

func (c *confirmModal) content(m *Model, width, height int) (string, []string, []string) {
	var body []string
	for _, line := range c.lines {
		body = append(body, wrapStyled(line, width)...)
	}
	if c.err != "" {
		body = append(body, "")
		for _, line := range wrap(c.err, width) {
			body = append(body, badStyle.Render(line))
		}
	}
	body = scrollLines(body, &c.scroll, height, &c.pos)
	var hints []string
	if c.yes != "" {
		key := "y"
		if c.enterOK {
			key = "enter"
		}
		label := c.yes
		if c.danger {
			hints = append(hints, badStyle.Bold(true).Render(key)+" "+quiet.Render(label))
		} else {
			hints = append(hints, hint(key, label))
		}
	}
	for _, x := range c.extra {
		hints = append(hints, hint(x.key, x.label))
	}
	return c.title, body, append(hints, hint("esc", "cancel"))
}

func wrapStyled(line string, width int) []string {
	if ansi.StringWidth(line) <= width {
		return []string{line}
	}
	return strings.Split(ansi.Wrap(line, max(1, width), " /"), "\n")
}

// scrollLines shows the window of lines that fits, clamping the scroll, and
// sets pos to where that window is for the box's frame to show.
func scrollLines(lines []string, scroll *int, height int, pos *string) []string {
	if len(lines) <= height {
		*scroll, *pos = 0, ""
		return lines
	}
	*scroll = max(0, min(*scroll, len(lines)-height))
	*pos = scrollPos(*scroll, height, len(lines))
	return lines[*scroll : *scroll+height]
}

// scrolled is a modal whose body scrolls; its position goes in the frame.
type scrolled interface{ scrollNote() string }

func (c *confirmModal) scrollNote() string { return c.pos }
func (v *viewerModal) scrollNote() string  { return v.pos }

// promptModal asks for one line of text.
type promptModal struct {
	title  string
	note   []string
	label  string
	input  *textInput
	err    string
	submit func(m *Model, value string) (tea.Cmd, error)
}

func (p *promptModal) preferredWidth() int { return 76 }
func (p *promptModal) paste(text string)   { p.input.insert(text) }

func (p *promptModal) update(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.modal = nil
	case "enter":
		m.modal = nil
		cmd, err := p.submit(m, p.input.String())
		if err != nil {
			p.err = sentence(err.Error())
			m.modal = p
		}
		return cmd
	default:
		if p.input.update(msg) {
			p.err = ""
		}
	}
	return nil
}

func (p *promptModal) content(m *Model, width, height int) (string, []string, []string) {
	var body []string
	for _, n := range p.note {
		for _, line := range wrap(n, width) {
			body = append(body, quiet.Render(line))
		}
	}
	if len(body) > 0 {
		body = append(body, "")
	}
	body = append(body, accent.Render("› ")+p.input.view(width-2, true))
	if p.err != "" {
		body = append(body, "")
		for _, line := range strings.Split(p.err, "\n") {
			for _, w := range wrap(line, width) {
				body = append(body, badStyle.Render(w))
			}
		}
	}
	label := p.label
	if label == "" {
		label = "ok"
	}
	return p.title, body, []string{hint("enter", label), hint("esc", "cancel")}
}

// pickerModal chooses one item. With a filter, typing narrows the list and
// the arrow keys move; without one, j and k move too.
type pickItem struct {
	label  string
	detail string
	key    string
	style  lipglossStyle
}

type pickerModal struct {
	title   string
	items   []pickItem
	cursor  int
	filter  *textInput
	pick    func(m *Model, i int) tea.Cmd
	note    string
	visible []int
	width   int
}

func (p *pickerModal) preferredWidth() int {
	if p.width > 0 {
		return p.width
	}
	return 64
}

func (p *pickerModal) paste(text string) {
	if p.filter != nil {
		p.filter.insert(text)
		p.cursor = 0
	}
}

func (p *pickerModal) shown() []int {
	p.visible = p.visible[:0]
	q := ""
	if p.filter != nil {
		q = strings.ToLower(strings.TrimSpace(p.filter.String()))
	}
	for i, item := range p.items {
		if q == "" || fuzzy(strings.ToLower(item.label+" "+item.detail), q) {
			p.visible = append(p.visible, i)
		}
	}
	return p.visible
}

// fuzzy matches the query's words in order, each as a substring.
func fuzzy(text, query string) bool {
	for _, word := range strings.Fields(query) {
		i := strings.Index(text, word)
		if i < 0 {
			return false
		}
		text = text[i+len(word):]
	}
	return true
}

func (p *pickerModal) update(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	shown := p.shown()
	key := msg.String()
	nav := map[string]int{"up": -1, "down": 1, "ctrl+p": -1, "ctrl+n": 1, "pgup": -10, "pgdown": 10}
	if p.filter == nil {
		nav["k"], nav["j"] = -1, 1
	}
	if d, ok := nav[key]; ok {
		p.cursor = max(0, min(len(shown)-1, p.cursor+d))
		return nil
	}
	switch key {
	case "esc":
		m.modal = nil
	case "enter":
		if len(shown) == 0 {
			return nil
		}
		m.modal = nil
		return p.pick(m, shown[min(p.cursor, len(shown)-1)])
	default:
		if p.filter != nil && p.filter.update(msg) {
			p.cursor = 0
		}
	}
	return nil
}

func (p *pickerModal) content(m *Model, width, height int) (string, []string, []string) {
	shown := p.shown()
	var body []string
	if p.note != "" {
		body = append(body, quiet.Render(p.note), "")
	}
	if p.filter != nil {
		body = append(body, accent.Render("› ")+p.filter.view(width-2, true), "")
	}
	rows := max(1, height-len(body))
	p.cursor = max(0, min(p.cursor, len(shown)-1))
	start := max(0, min(p.cursor-rows/2, len(shown)-rows))
	if len(shown) == 0 {
		body = append(body, quiet.Render("Nothing matches."))
	}
	for i := start; i < len(shown) && i < start+rows; i++ {
		item := p.items[shown[i]]
		keyWidth := 0
		if item.key != "" {
			keyWidth = 9
		}
		spans := []span{sp(item.label, item.style)}
		if item.detail != "" {
			spans = append(spans, plain("  "), sp(item.detail, quiet))
		}
		gutter := " "
		if i == p.cursor {
			gutter = accent.Render("▌")
		}
		line := gutter + row(spans, width-keyWidth-2, i == p.cursor)
		if keyWidth > 0 {
			k := pad(item.key, keyWidth)
			if i == p.cursor {
				line += selBG.Render(" ") + accentBold.Background(selectionColor).Render(k)
			} else {
				line += " " + accentBold.Render(k)
			}
		} else if i == p.cursor {
			line += selBG.Render(" ")
		} else {
			line += " "
		}
		body = append(body, line)
	}
	nav := "↑↓"
	if p.filter == nil {
		nav = "j/k"
	}
	return p.title, body, []string{hint("enter", "choose"), hint(nav, "move"), hint("esc", "close")}
}

// viewerModal shows long read-only text with scrolling.
type viewerModal struct {
	title    string
	lines    []string
	scroll   int
	pos      string
	copyText string
	extra    []extraKey
	width    int
	// back is the modal to return to on close, when opened from one.
	back modal
}

func (v *viewerModal) preferredWidth() int {
	if v.width > 0 {
		return v.width
	}
	return 100
}

func (v *viewerModal) update(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	page := max(1, m.height-10)
	for _, x := range v.extra {
		if msg.String() == x.key {
			m.modal = nil
			return x.run(m)
		}
	}
	switch msg.String() {
	case "esc", "q", "enter":
		m.modal = v.back
	case "j", "down":
		v.scroll++
	case "k", "up":
		v.scroll = max(0, v.scroll-1)
	case "ctrl+d", "pgdown", "space":
		v.scroll += page
	case "ctrl+u", "pgup":
		v.scroll = max(0, v.scroll-page)
	case "g", "home":
		v.scroll = 0
	case "G", "end":
		v.scroll = len(v.lines)
	case "y":
		if v.copyText != "" {
			m.ok("Copied")
			return tea.SetClipboard(v.copyText)
		}
	}
	return nil
}

func (v *viewerModal) content(m *Model, width, height int) (string, []string, []string) {
	var body []string
	for _, line := range v.lines {
		body = append(body, wrapStyled(line, width)...)
	}
	body = scrollLines(body, &v.scroll, height, &v.pos)
	hints := []string{}
	if len(v.lines) > height {
		hints = append(hints, hint("j/k", "scroll"))
	}
	if v.copyText != "" {
		hints = append(hints, hint("y", "copy"))
	}
	for _, x := range v.extra {
		hints = append(hints, hint(x.key, x.label))
	}
	if v.back != nil {
		return v.title, body, append(hints, hint("esc", "back"))
	}
	return v.title, body, append(hints, hint("esc", "close"))
}

// formModal is a short form typed straight into: Tab or Enter moves on,
// and Enter on the last field submits.
type formField struct {
	label   string
	input   *textInput
	choices []string
	names   []string
	choice  int
	note    string
}

func (f *formField) value() string {
	if f.choices != nil {
		return f.choices[f.choice]
	}
	return f.input.String()
}

type formModal struct {
	title  string
	note   []string
	fields []*formField
	cursor int
	err    string
	submit func(m *Model, f *formModal) (tea.Cmd, error)
	cancel func(m *Model) tea.Cmd
	label  string
}

func (f *formModal) preferredWidth() int { return 76 }

func (f *formModal) paste(text string) {
	if in := f.fields[f.cursor].input; in != nil {
		in.insert(text)
	}
}

func (f *formModal) update(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	field := f.fields[f.cursor]
	switch msg.String() {
	case "esc":
		m.modal = nil
		if f.cancel != nil {
			return f.cancel(m)
		}
	case "tab", "down":
		f.cursor = (f.cursor + 1) % len(f.fields)
	case "shift+tab", "up":
		f.cursor = (f.cursor + len(f.fields) - 1) % len(f.fields)
	case "enter":
		if f.cursor < len(f.fields)-1 {
			f.cursor++
			return nil
		}
		m.modal = nil
		cmd, err := f.submit(m, f)
		if err != nil {
			f.err = sentence(err.Error())
			m.modal = f
		}
		return cmd
	case "left", "right", "space":
		if field.choices != nil {
			d := 1
			if msg.String() == "left" {
				d = len(field.choices) - 1
			}
			field.choice = (field.choice + d) % len(field.choices)
			return nil
		}
		field.input.update(msg)
	default:
		if field.input != nil && field.input.update(msg) {
			f.err = ""
		}
	}
	return nil
}

func (f *formModal) content(m *Model, width, height int) (string, []string, []string) {
	var body []string
	for _, n := range f.note {
		for _, line := range wrap(n, width) {
			body = append(body, quiet.Render(line))
		}
	}
	if len(body) > 0 {
		body = append(body, "")
	}
	labelWidth := 0
	for _, field := range f.fields {
		labelWidth = max(labelWidth, ansi.StringWidth(field.label))
	}
	for i, field := range f.fields {
		focused := i == f.cursor
		label := quiet.Render(pad(field.label, labelWidth))
		if focused {
			label = accentBold.Render(pad(field.label, labelWidth))
		}
		var value string
		if field.choices != nil {
			var parts []string
			for j, c := range field.choices {
				name := c
				if field.names != nil {
					name = field.names[j]
				}
				switch {
				case j == field.choice && focused:
					parts = append(parts, accentBold.Render("‹"+name+"›"))
				case j == field.choice:
					parts = append(parts, bold.Render(name))
				default:
					parts = append(parts, quiet.Render(name))
				}
			}
			value = strings.Join(parts, " ")
		} else {
			value = field.input.view(width-labelWidth-2, focused)
		}
		body = append(body, label+"  "+value)
		if focused && field.note != "" {
			body = append(body, strings.Repeat(" ", labelWidth+2)+quiet.Render(clip(field.note, width-labelWidth-2)))
		}
	}
	if f.err != "" {
		body = append(body, "")
		for _, line := range wrap(f.err, width) {
			body = append(body, badStyle.Render(line))
		}
	}
	label := f.label
	if label == "" {
		label = "save"
	}
	hints := []string{hint("enter", "next"), hint("tab", "next field")}
	if f.cursor == len(f.fields)-1 {
		hints[0] = hint("enter", label)
	}
	if f.fields[f.cursor].choices != nil {
		hints = append(hints, hint("←→", "choose"))
	}
	return f.title, body, append(hints, hint("esc", "cancel"))
}

func initForm() *formModal {
	f := &formModal{title: "Create a Reqscope project", label: "create",
		note: []string{"No project here yet. Reqscope keeps endpoints, saved requests and environments in a folder you can commit."},
		fields: []*formField{
			{label: "Folder", input: newInput(".", "."), note: "Where reqscope.yaml goes"},
			{label: "Name", input: newInput("My API", "My API")},
		}}
	f.cancel = func(*Model) tea.Cmd { return tea.Quit }
	f.submit = func(m *Model, f *formModal) (tea.Cmd, error) {
		path := strings.TrimSpace(f.fields[0].value())
		if _, err := project.Init(path, f.fields[1].value(), false); err != nil {
			return nil, err
		}
		m.root = path
		if err := m.open(); err != nil {
			m.root = ""
			return nil, err
		}
		m.ok("Project created. Press " + m.keyName(actImport) + " to import an API.")
		// Init ran before there was a project, so polling starts here.
		return m.poll(0), nil
	}
	return f
}

func (m *Model) paletteModal() modal {
	var items []pickItem
	var ids []action
	here := contexts[m.context()]
	defs := append([]actionDef(nil), m.keys.defs...)
	rank := func(d actionDef) int {
		for i, s := range here {
			if slicesContains(d.scopes, s) && s != scopeGlobal && s != scopeNav {
				return i
			}
		}
		if d.scopes[0] == scopeGlobal {
			return 90
		}
		return 50
	}
	sort.SliceStable(defs, func(i, j int) bool { return rank(defs[i]) < rank(defs[j]) })
	for _, d := range defs {
		switch d.id {
		case actPalette, actUp, actDown, actTop, actBottom, actPageUp, actPageDown, actNextTab, actPrevTab:
			continue
		}
		key := ""
		if len(d.keys) > 0 {
			key = d.keys[0]
		}
		where := scopeNames[d.scopes[0]]
		if d.scopes[0] == scopeGlobal || d.scopes[0] == scopeCommand || d.scopes[0] == scopeBrowse {
			where = ""
		}
		items = append(items, pickItem{label: d.desc, detail: where, key: key, style: lipglossPlain})
		ids = append(ids, d.id)
	}
	return &pickerModal{title: "Commands", items: items, filter: newInput("", "type to search"), width: 84,
		pick: func(m *Model, i int) tea.Cmd { return m.runCommand(ids[i]) }}
}

func (m *Model) helpModal() modal {
	var chain []scope
	for _, s := range contexts[m.context()] {
		if s != scopeNav {
			chain = append(chain, s)
		}
	}
	chain = append(chain, scopeNav)
	var lines []string
	for _, s := range chain {
		var rows []string
		for _, d := range m.keys.defs {
			if !slicesContains(d.scopes, s) || len(d.keys) == 0 {
				continue
			}
			rows = append(rows, "  "+accentBold.Render(pad(strings.Join(d.keys, " "), 16))+" "+d.desc)
		}
		if s == scopeGlobal {
			rows = append(rows, "  "+accentBold.Render(pad(m.keys.leader, 16))+" Open the key menu: every command, grouped")
			rows = append(rows, "  "+accentBold.Render(pad("esc", 16))+" Go back, close or cancel")
			rows = append(rows, "  "+accentBold.Render(pad("ctrl+c", 16))+" Cancel what is running, or quit")
		}
		if len(rows) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, bold.Render(scopeNames[s]))
		lines = append(lines, rows...)
	}
	lines = append(lines, "", quiet.Render("Everything else is in the command search: press "+m.keyName(actPalette)+"."))
	return &viewerModal{title: "Keys", lines: lines, width: 76}
}

// envPicker switches environments from anywhere.
func (m *Model) envPicker() modal {
	envs := m.environments()
	if len(envs) == 0 {
		m.say("No environments yet. Create one in Environments (" + m.keyName(actTab5) + ").")
		return nil
	}
	items := make([]pickItem, len(envs))
	cursor := 0
	for i, e := range envs {
		item := pickItem{label: e.Name, style: lipglossPlain}
		if e.Classification == "production" {
			item.style, item.detail = delStyle, "asks before each send"
		}
		if e.ID == m.data.activeEnv {
			item.detail = strings.TrimSpace("in use  " + item.detail)
			cursor = i
		}
		items[i] = item
	}
	return &pickerModal{title: "Send requests to", items: items, cursor: cursor, width: 48,
		pick: func(m *Model, i int) tea.Cmd { m.useEnv(envs[i]); return nil }}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
