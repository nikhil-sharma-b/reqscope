package tui

import (
	"fmt"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"os"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Binding struct {
	Action      string
	Description string
	Keys        []string
}
type KeyConfig struct {
	Leader   string              `yaml:"leader"`
	Bindings map[string][]string `yaml:"bindings"`
}

func defaults() []Binding {
	return []Binding{
		{"import", "Import local specification", []string{"<leader> s i"}}, {"schemas", "Inspect schemas", []string{"<leader> s s"}},
		{"down", "Move down", []string{"j", "down"}}, {"up", "Move up", []string{"k", "up"}},
		{"first", "First item", []string{"g g"}}, {"last", "Last item", []string{"G"}},
		{"half-down", "Half page down", []string{"ctrl+d"}}, {"half-up", "Half page up", []string{"ctrl+u"}},
		{"pane-left", "Previous pane", []string{"ctrl+w h", "ctrl+w k"}}, {"pane-right", "Next pane", []string{"ctrl+w l", "ctrl+w j", "tab"}}, {"zoom", "Zoom focused pane", []string{"ctrl+w o"}},
		{"endpoints", "Endpoints pane", []string{"g e"}}, {"request", "Request pane", []string{"g q", "<leader> r i"}}, {"response", "Response pane", []string{"g r", "<leader> r r"}}, {"history", "History pane", []string{"g h", "<leader> h l"}},
		{"project", "Inspect project", []string{"<leader> p i"}}, {"validate", "Validate project", []string{"<leader> p v"}}, {"rename", "Edit project name", []string{"<leader> p n", "i"}}, {"save", "Save project name", []string{"<leader> p s"}}, {"reload", "Reload project", []string{"<leader> p r"}},
		{"environments", "Environment templates", []string{"<leader> e l"}},
		{"cases", "Cases pane", []string{"<leader> c l"}}, {"scenarios", "Scenarios pane", []string{"<leader> s l"}},
		{"method-filter", "Cycle endpoint method filter", []string{"<leader> s m"}}, {"tag-group", "Toggle endpoint tag grouping", []string{"<leader> s t"}},
		{"search", "Search focused pane", []string{"/"}}, {"endpoint-search", "Search endpoints", []string{"<leader> /"}}, {"next-match", "Next match", []string{"n"}}, {"previous-match", "Previous match", []string{"N"}},
		{"open", "Open selection or edit project name", []string{"enter"}}, {"help", "Effective keybindings", []string{"?"}}, {"palette", "Command palette", []string{"<leader> :", ":"}}, {"quit", "Quit", []string{"q", "ctrl+c"}},
	}
}
func Bindings(c KeyConfig) ([]Binding, error) {
	leader := c.Leader
	if leader == "" {
		leader = "space"
	}
	if leader == " " {
		leader = "space"
	}
	if utf8.RuneCountInString(leader) != 1 && leader != "space" {
		return nil, project.Fail("invalid_input", "leader must be one printable key or space")
	}
	if leader != "space" {
		r, _ := utf8.DecodeRuneInString(leader)
		if !unicode.IsPrint(r) || unicode.IsSpace(r) {
			return nil, project.Fail("invalid_input", "leader must be printable")
		}
	}
	reserved := map[string]bool{"esc": true, "enter": true, "tab": true, "backspace": true}
	if reserved[leader] || leader[0] >= '0' && leader[0] <= '9' {
		return nil, project.Fail("invalid_input", "leader conflicts with a reserved key")
	}
	bindings := defaults()
	known := map[string]bool{}
	sequences := map[string]string{}
	for i := range bindings {
		b := &bindings[i]
		known[b.Action] = true
		if keys, ok := c.Bindings[b.Action]; ok {
			b.Keys = keys
		}
		for j, seq := range b.Keys {
			seq = strings.ReplaceAll(seq, "<leader>", leader)
			tokens := strings.Fields(seq)
			if len(tokens) == 0 {
				return nil, project.Fail("invalid_input", "empty binding for %s", b.Action)
			}
			for _, token := range tokens {
				if token == "esc" || token[0] >= '0' && token[0] <= '9' {
					return nil, project.Fail("invalid_input", "binding %q uses a reserved key", seq)
				}
				if !validKey(token) {
					return nil, project.Fail("invalid_input", "unsupported key %q", token)
				}
			}
			seq = strings.Join(tokens, " ")
			if old, ok := sequences[seq]; ok {
				return nil, project.Fail("invalid_input", "binding %q conflicts between %s and %s", seq, old, b.Action)
			}
			sequences[seq] = b.Action
			b.Keys[j] = seq
		}
	}
	for action := range c.Bindings {
		if !known[action] {
			return nil, project.Fail("invalid_input", "unknown keybinding action %s", action)
		}
	}
	for a, action := range sequences {
		for b, other := range sequences {
			if strings.HasPrefix(b, a+" ") {
				return nil, project.Fail("invalid_input", "binding %q (%s) conflicts with prefix of %q (%s)", a, action, b, other)
			}
		}
	}
	return bindings, nil
}
func validKey(s string) bool {
	if utf8.RuneCountInString(s) == 1 {
		r, _ := utf8.DecodeRuneInString(s)
		return unicode.IsPrint(r) && !unicode.IsSpace(r)
	}
	switch s {
	case "space", "enter", "tab", "backspace", "up", "down", "left", "right", "ctrl+c", "ctrl+d", "ctrl+u", "ctrl+w":
		return true
	}
	return false
}
func loadKeys(path string) (KeyConfig, error) {
	var c KeyConfig
	if path == "" {
		return c, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = project.Decode(b, &c)
	return c, err
}
func help(bindings []Binding) string {
	var lines []string
	for _, b := range bindings {
		keys := strings.Join(b.Keys, ", ")
		if keys == "" {
			keys = "unbound"
		}
		lines = append(lines, fmt.Sprintf("%-20s %s", keys, b.Description))
	}
	return strings.Join(lines, "\n")
}
func continuations(bindings []Binding, prefix string) string {
	next := map[string]string{}
	for _, b := range bindings {
		for _, seq := range b.Keys {
			if strings.HasPrefix(seq, prefix+" ") {
				rest := strings.TrimPrefix(seq, prefix+" ")
				parts := strings.Fields(rest)
				label := b.Description
				if len(parts) > 1 {
					label = "More commands"
				}
				next[parts[0]] = label
			}
		}
	}
	var keys []string
	for key := range next {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var lines []string
	for _, key := range keys {
		lines = append(lines, key+"  "+next[key])
	}
	return strings.Join(lines, "\n")
}
