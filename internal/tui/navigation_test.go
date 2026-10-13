package tui_test

import (
	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
	"strings"
	"testing"
)

func TestCountsLeaderHelpAndPalette(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Demo", false)
	m, err := tui.New(root, tui.KeyConfig{Leader: ",", Bindings: map[string][]string{"help": {"H"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{",", "e", "l", "2", "j"} {
		key(m, k)
	}
	view := m.View().Content
	if !strings.Contains(view, "> staging") {
		t.Fatalf("count motion did not select last environment: %s", view)
	}
	key(m, "H")
	for i := 0; i < 12; i++ {
		key(m, "j")
	}
	view = m.View().Content
	if !strings.Contains(view, "H") || !strings.Contains(view, "Effective keybindings") {
		t.Fatalf("remap missing from help: %s", view)
	}
	key(m, "esc")
	key(m, ":")
	for _, k := range []string{"v", "a", "l", "i", "d"} {
		key(m, k)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.View().Content, "Project validation passed") {
		t.Fatal("palette did not execute shared validation")
	}
}
func TestConflictingBindingsRejected(t *testing.T) {
	for _, c := range []tui.KeyConfig{{Bindings: map[string][]string{"help": {"j"}}}, {Bindings: map[string][]string{"help": {"g"}}}, {Leader: "j"}, {Bindings: map[string][]string{"unknown": {"x"}}}} {
		if _, err := tui.Bindings(c); err == nil {
			t.Fatalf("accepted conflicting config: %+v", c)
		}
	}
}
func TestDirtyQuitCancelAndSave(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Demo", false)
	m, _ := tui.New(root, tui.KeyConfig{})
	for _, k := range []string{"i", "x", "esc", "q"} {
		key(m, k)
	}
	if !strings.Contains(m.View().Content, "Unsaved project name") {
		t.Fatal("quit did not protect edits")
	}
	key(m, "c")
	for _, k := range []string{"space", "p", "s"} {
		key(m, k)
	}
	s, err := project.Load(root)
	if err != nil || s.Manifest.Name != "Demox" {
		t.Fatalf("save did not persist draft: %v %+v", err, s)
	}
}
