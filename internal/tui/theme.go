package tui

import (
	"image/color"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The palette borrows its loud colours from the subject: HTTP methods use the
// hues Swagger UI made familiar, and response codes reuse them by class.
// Everything else is quiet so those colours carry meaning. A single violet
// marks where keys land: the focused title, key names and the cursor.
type palette struct {
	quiet, rule, border, accent         color.Color
	get, post, put, patch, del, neutral color.Color
	selection, badgeText                color.Color
}

func paletteFor(dark bool) palette {
	pick, c := lipgloss.LightDark(dark), lipgloss.Color
	return palette{
		quiet:     pick(c("#667080"), c("#8b93a1")),
		rule:      pick(c("#d5dae1"), c("#3a404a")),
		border:    pick(c("#aab2bf"), c("#4f5866")),
		accent:    pick(c("#6f42c1"), c("#b392f0")),
		get:       pick(c("#1f6fc5"), c("#61affe")),
		post:      pick(c("#1a8a55"), c("#49cc90")),
		put:       pick(c("#b26a00"), c("#fca130")),
		patch:     pick(c("#0f8a78"), c("#50e3c2")),
		del:       pick(c("#c62828"), c("#f93e3e")),
		neutral:   pick(c("#6b7480"), c("#9aa4b2")),
		selection: pick(c("#e6e9ef"), c("#2a2f3a")),
		badgeText: pick(c("#ffffff"), c("#14161a")),
	}
}

var (
	quiet, ruleStyle, accent, accentBold, bold    lipgloss.Style
	okStyle, warnStyle, badStyle, infoStyle       lipgloss.Style
	getStyle, postStyle, putStyle, patchStyle     lipgloss.Style
	delStyle, neutralStyle, selBG                 lipgloss.Style
	okBadge, warnBadge, badBadge, infoBadge, prod lipgloss.Style
	cursorStyle, borderStyle, tabPill, keyStyle   lipgloss.Style
	selectionColor                                color.Color
)

// lipglossStyle lets files that only pass styles around skip the import.
type lipglossStyle = lipgloss.Style

// lipglossPlain is the terminal's own colours.
var lipglossPlain = lipgloss.NewStyle()

func init() { applyTheme(true) }

// applyTheme rebuilds the styles once the terminal reports its background.
// Until then the dark variant is assumed, as most terminals are.
func applyTheme(dark bool) {
	p := paletteFor(dark)
	s := lipgloss.NewStyle
	quiet = s().Foreground(p.quiet)
	ruleStyle = s().Foreground(p.rule)
	accent = s().Foreground(p.accent)
	accentBold = s().Foreground(p.accent).Bold(true)
	bold = s().Bold(true)
	getStyle = s().Foreground(p.get).Bold(true)
	postStyle = s().Foreground(p.post).Bold(true)
	putStyle = s().Foreground(p.put).Bold(true)
	patchStyle = s().Foreground(p.patch).Bold(true)
	delStyle = s().Foreground(p.del).Bold(true)
	neutralStyle = s().Foreground(p.neutral).Bold(true)
	okStyle = s().Foreground(p.post)
	infoStyle = s().Foreground(p.get)
	warnStyle = s().Foreground(p.put)
	badStyle = s().Foreground(p.del)
	selectionColor = p.selection
	selBG = s().Background(p.selection)
	badge := func(bg color.Color) lipgloss.Style {
		return s().Background(bg).Foreground(p.badgeText).Bold(true).Padding(0, 1)
	}
	okBadge, infoBadge, warnBadge, badBadge = badge(p.post), badge(p.get), badge(p.put), badge(p.del)
	prod = badge(p.del)
	cursorStyle = s().Reverse(true)
	borderStyle = s().Foreground(p.border)
	tabPill = s().Background(p.accent).Foreground(p.badgeText).Bold(true).Padding(0, 1)
	keyStyle = s().Foreground(p.accent).Bold(true)
}

func methodStyle(method string) lipgloss.Style {
	switch strings.ToUpper(method) {
	case "GET":
		return getStyle
	case "POST":
		return postStyle
	case "PUT":
		return putStyle
	case "PATCH":
		return patchStyle
	case "DELETE":
		return delStyle
	}
	return neutralStyle
}

// statusBadge is the loudest thing on screen: the answer to "did it work?".
func statusBadge(code int, text string) string {
	switch {
	case code >= 500:
		return badBadge.Render(text)
	case code >= 400:
		return warnBadge.Render(text)
	case code >= 300:
		return infoBadge.Render(text)
	case code >= 200:
		return okBadge.Render(text)
	}
	return badBadge.Render(text)
}

func statusStyle(code int) lipgloss.Style {
	switch {
	case code >= 500 || code == 0:
		return badStyle
	case code >= 400:
		return warnStyle
	case code >= 300:
		return infoStyle
	}
	return okStyle
}

var pathParam = regexp.MustCompile(`\{[^{}]+\}`)

// styledPath sets template parameters in the accent colour: they are the
// parts of a path a person has to fill in.
func styledPath(path string, base lipgloss.Style) string {
	locs := pathParam.FindAllStringIndex(path, -1)
	if len(locs) == 0 {
		return base.Render(path)
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		b.WriteString(base.Render(path[last:loc[0]]))
		b.WriteString(accent.Render(path[loc[0]:loc[1]]))
		last = loc[1]
	}
	b.WriteString(base.Render(path[last:]))
	return b.String()
}

// pad truncates or pads to exactly width terminal columns.
func pad(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(text) > width {
		text = ansi.Truncate(text, width, "…")
	}
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

// clip truncates without padding.
func clip(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(text) > width {
		return ansi.Truncate(text, width, "…")
	}
	return text
}

// block forces lines into an exact width × height rectangle.
func block(lines []string, width, height int) []string {
	out := make([]string, height)
	for i := range out {
		if i < len(lines) {
			out[i] = pad(lines[i], width)
		} else {
			out[i] = strings.Repeat(" ", max(0, width))
		}
	}
	return out
}

// beside joins two rectangles with a quiet vertical rule.
func beside(left, right []string, lw, rw, height int) []string {
	l, r := block(left, lw, height), block(right, rw, height)
	sep := ruleStyle.Render("│")
	out := make([]string, height)
	for i := range out {
		out[i] = l[i] + sep + r[i]
	}
	return out
}

func rule(width int) string { return ruleStyle.Render(strings.Repeat("─", max(0, width))) }

// heading opens a section inside a pane: the name in the accent colour and a
// quiet rule to the edge, so sections read as separate blocks at a glance.
func heading(title string, width int) string {
	t := accentBold.Render(title)
	return t + " " + rule(width-ansi.StringWidth(title)-1)
}

// frame draws a rounded box of exactly width × height around lines, with a
// title set into the top edge and an optional note at its right end. The
// focused frame is drawn in the accent colour: that is where keys land.
func frame(lines []string, title, note string, width, height int, focused bool) []string {
	return framed(lines, title, note, "", width, height, focused)
}

// framed is frame with a foot: a note set into the bottom edge, used for
// the scroll position so it never takes a line that content could use.
func framed(lines []string, title, note, foot string, width, height int, focused bool) []string {
	if width < 6 || height < 3 {
		return block(lines, width, height)
	}
	edge, name := borderStyle, quiet.Bold(true)
	if focused {
		edge, name = accent, accentBold
	}
	inner := width - 2
	title = clip(title, max(1, inner-4))
	top := edge.Render("╭─ ") + name.Render(title) + " "
	used := 4 + ansi.StringWidth(title)
	if note != "" && used+ansi.StringWidth(note)+4 <= inner+1 {
		fill := inner - used - ansi.StringWidth(note) - 2
		top += edge.Render(strings.Repeat("─", fill)) + " " + note + " " + edge.Render("─╮")
	} else {
		top += edge.Render(strings.Repeat("─", max(0, inner-used+1)) + "╮")
	}
	body := block(lines, inner-2, height-2)
	out := []string{top}
	side := edge.Render("│")
	for _, line := range body {
		out = append(out, side+" "+line+" "+side)
	}
	return append(out, edge.Render("╰")+edgeNote(foot, inner, edge)+edge.Render("╯"))
}

// edgeNote is a horizontal edge width columns wide with note set near its
// right end, or a plain edge when the note does not fit.
func edgeNote(note string, width int, edge lipgloss.Style) string {
	w := ansi.StringWidth(note)
	if note == "" || w+6 > width {
		return edge.Render(strings.Repeat("─", width))
	}
	return edge.Render(strings.Repeat("─", width-w-3)) + " " + note + " " + edge.Render("─")
}

// scrollPos says which lines of a scrolled view are on show, with arrows for
// the directions there is more. It is empty when everything fits.
func scrollPos(scroll, height, total int) string {
	if total <= height || height <= 0 {
		return ""
	}
	arrows := ""
	if scroll > 0 {
		arrows += "↑"
	}
	if scroll+height < total {
		arrows += "↓"
	}
	last := min(total, scroll+height)
	return keyStyle.Render(arrows) + " " + quiet.Render(strconv.Itoa(scroll+1)+"–"+strconv.Itoa(last)+" of "+strconv.Itoa(total))
}

// join lays rectangles of equal height side by side.
func join(height int, parts ...[]string) []string {
	out := make([]string, height)
	for _, p := range parts {
		for i := range out {
			if i < len(p) {
				out[i] += p[i]
			}
		}
	}
	return out
}

// hint renders one key and what it does, as the footer and modals show them.
func hint(key, label string) string {
	return keyStyle.Render(key) + " " + quiet.Render(label)
}

// span is a run of text in one style. Rows are built from spans so the
// selection band can be laid under every span: a style rendered inside another
// would reset the band's background part way along the row.
type span struct {
	text  string
	style lipgloss.Style
}

func sp(text string, style lipgloss.Style) span { return span{text, style} }
func plain(text string) span                    { return span{text, lipgloss.NewStyle()} }

// row renders spans into exactly width columns, truncating the last visible
// span with an ellipsis. A selected row carries the band across its width.
func row(spans []span, width int, selected bool) string {
	var b strings.Builder
	used := 0
	for _, s := range spans {
		if used >= width {
			break
		}
		text := s.text
		w := ansi.StringWidth(text)
		if used+w > width {
			text = ansi.Truncate(text, width-used, "…")
			w = ansi.StringWidth(text)
		}
		if text == "" {
			continue
		}
		style := s.style
		if selected {
			style = style.Background(selectionColor)
		}
		b.WriteString(style.Render(text))
		used += w
	}
	if used < width {
		fill := strings.Repeat(" ", width-used)
		if selected {
			fill = selBG.Render(fill)
		}
		b.WriteString(fill)
	}
	return b.String()
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
	case n >= 1<<10:
		return strconv.FormatFloat(float64(n)/(1<<10), 'f', 1, 64) + " kB"
	}
	return strconv.Itoa(n) + " B"
}
