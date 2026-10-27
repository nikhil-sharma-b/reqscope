package tui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// textInput is a one-line editor with the usual readline movements.
type textInput struct {
	value       []rune
	pos         int
	placeholder string
	mask        bool
}

func newInput(value, placeholder string) *textInput {
	r := []rune(value)
	return &textInput{value: r, pos: len(r), placeholder: placeholder}
}

func (t *textInput) String() string { return string(t.value) }

func (t *textInput) set(value string) {
	t.value = []rune(value)
	t.pos = len(t.value)
}

func (t *textInput) insert(text string) {
	text = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(text)
	r := []rune(text)
	t.value = append(t.value[:t.pos], append(r, t.value[t.pos:]...)...)
	t.pos += len(r)
}

// update applies an editing key and reports whether it was one. Enter, Esc
// and Tab are left to the owner, which decides what they mean.
func (t *textInput) update(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "left", "ctrl+b":
		t.pos = max(0, t.pos-1)
	case "right", "ctrl+f":
		t.pos = min(len(t.value), t.pos+1)
	case "home", "ctrl+a":
		t.pos = 0
	case "end", "ctrl+e":
		t.pos = len(t.value)
	case "alt+left", "alt+b", "ctrl+left":
		t.pos = t.wordLeft()
	case "alt+right", "alt+f", "ctrl+right":
		t.pos = t.wordRight()
	case "backspace", "ctrl+h":
		if t.pos > 0 {
			t.value = append(t.value[:t.pos-1], t.value[t.pos:]...)
			t.pos--
		}
	case "delete", "ctrl+d":
		if t.pos < len(t.value) {
			t.value = append(t.value[:t.pos], t.value[t.pos+1:]...)
		}
	case "ctrl+w", "alt+backspace":
		start := t.wordLeft()
		t.value = append(t.value[:start], t.value[t.pos:]...)
		t.pos = start
	case "ctrl+u":
		t.value = t.value[t.pos:]
		t.pos = 0
	case "ctrl+k":
		t.value = t.value[:t.pos]
	case "space":
		t.insert(" ")
	default:
		if msg.Text == "" || msg.Mod&(tea.ModCtrl|tea.ModAlt) != 0 {
			return false
		}
		t.insert(msg.Text)
	}
	return true
}

func (t *textInput) wordLeft() int {
	i := t.pos
	for i > 0 && unicode.IsSpace(t.value[i-1]) {
		i--
	}
	for i > 0 && !isWordBreak(t.value[i-1]) {
		i--
	}
	return i
}

func (t *textInput) wordRight() int {
	i := t.pos
	for i < len(t.value) && isWordBreak(t.value[i]) {
		i++
	}
	for i < len(t.value) && !isWordBreak(t.value[i]) {
		i++
	}
	return i
}

func isWordBreak(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("/.:-_=&?{}[]\",", r)
}

// view renders the input in width columns, scrolling so the cursor stays in
// sight. The cursor is drawn only while the input has focus.
func (t *textInput) view(width int, focused bool) string {
	if width <= 0 {
		return ""
	}
	text := t.value
	if t.mask {
		text = []rune(strings.Repeat("•", len(text)))
	}
	if len(text) == 0 && !focused {
		return quiet.Render(clip(t.placeholder, width))
	}
	if len(text) == 0 {
		return cursorStyle.Render(" ") + quiet.Render(clip(t.placeholder, width-1))
	}
	// Scroll by runes; wide runes are rare in inputs and only cost a column.
	start := 0
	if t.pos >= width {
		start = t.pos - width + 1
	}
	var b strings.Builder
	used := 0
	for i := start; i <= len(text) && used < width; i++ {
		ch := " "
		if i < len(text) {
			ch = string(text[i])
		} else if !focused {
			break
		}
		w := max(1, ansi.StringWidth(ch))
		if used+w > width {
			break
		}
		if focused && i == t.pos {
			b.WriteString(cursorStyle.Render(ch))
		} else {
			b.WriteString(ch)
		}
		used += w
	}
	return b.String()
}
