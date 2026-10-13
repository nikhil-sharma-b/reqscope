package tui_test

import (
	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
	"strings"
	"testing"
)

func key(m *tui.Model, s string) {
	k := tea.Key{Text: s}
	if s == "esc" {
		k.Code = tea.KeyEscape
	} else if s == "space" {
		k.Code = tea.KeySpace
		k.Text = " "
	} else {
		k.Code = []rune(s)[0]
	}
	m.Update(tea.KeyPressMsg(k))
}
func TestModesPrefixesAndResize(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Demo", false)
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	key(m, "space")
	if !strings.Contains(m.View().Content, "More commands") {
		t.Fatal("missing continuation menu")
	}
	key(m, "esc")
	key(m, "i")
	key(m, "j")
	key(m, "esc")
	m.Update(tea.WindowSizeMsg{Width: 25, Height: 9})
	if !strings.Contains(m.View().Content, "Demoj") {
		t.Fatalf("lost edit on resize: %s", m.View().Content)
	}
}
