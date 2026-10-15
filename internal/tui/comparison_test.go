package tui_test

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
	"strings"
	"testing"
)

func TestTUIComparisonArbitraryIDsAndSavedIgnores(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, e := project.Init(root, "Comparison", false); e != nil {
		t.Fatal(e)
	}
	c, e := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "comparison", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/"}}, "", false)
	if e != nil {
		t.Fatal(e)
	}
	a, _ := project.SendCase(context.Background(), root, "local", c.ID, nil, project.SendOptions{})
	b, _ := project.SendCase(context.Background(), root, "staging", c.ID, nil, project.SendOptions{})
	m, e := tui.New(root, tui.KeyConfig{})
	if e != nil {
		t.Fatal(e)
	}
	keys := func(seq ...string) {
		for _, k := range seq {
			key(m, k)
		}
	}
	keys("space", "c", "l", "space", "c", "o", "space", "c", "e")
	c.ComparisonIgnore = project.IgnoreRules{JSONPointers: []string{"/ignored"}, Headers: []string{"Date"}}
	raw, _ := json.Marshal(c.Case)
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: string(raw)})
	enter(m)
	saved, e := project.GetCase(root, c.ID)
	if e != nil || len(saved.ComparisonIgnore.JSONPointers) != 1 {
		t.Fatalf("%+v %v %s", saved, e, m.View().Content)
	}
	keys("g", "h", "space", "h", "d")
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: a.RunID + " missing"})
	enter(m)
	if !strings.Contains(m.View().Content, "missing or evicted") {
		t.Fatal(m.View().Content)
	}
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: a.RunID + " " + b.RunID})
	enter(m)
	if !strings.Contains(m.View().Content, "Comparison | j/k scroll") {
		t.Fatal(m.View().Content)
	}
	// The report remains keyboard-scrollable beyond the first screen.
	for i := 0; i < 25; i++ {
		key(m, "j")
	}
	view := m.View().Content
	if !strings.Contains(view, "/ignored") && !strings.Contains(view, "unavailable") {
		t.Fatal(view)
	}
	keys("esc", "space", "h", "d", "esc")
	if strings.Contains(m.View().Content, "Enter compare") {
		t.Fatal("cancel left overlay open")
	}
}
