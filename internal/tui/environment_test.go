package tui_test

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
	"strings"
	"testing"
)

func TestEnvironmentAuthoringApprovalAndSelection(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	project.Init(root, "Demo", false)
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"space", "e", "l", "G", "space", "e", "e"} {
		key(m, k)
	}
	s, _ := project.Load(root)
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = "https://staging.example.com/v1"
	e.Variables["ordinary"] = "value"
	b, _ := json.Marshal(e.Environment)
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: string(b)})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s, _ = project.Load(root)
	e, _ = s.Resolve("staging")
	if e.BaseURLs["api"] != "https://staging.example.com/v1" {
		t.Fatalf("edit failed %s", m.View().Content)
	}
	for _, k := range []string{"space", "e", "s", "space", "e", "a"} {
		key(m, k)
	}
	if !strings.Contains(m.View().Content, "Exact origins") {
		t.Fatal("no authorization review")
	}
	key(m, "esc")
	if _, err := project.CheckCLIAccess(root, "staging", "api", ""); err == nil {
		t.Fatal("cancel granted access")
	}
	for _, k := range []string{"space", "e", "a", "y"} {
		key(m, k)
	}
	if _, err := project.CheckCLIAccess(root, "staging", "api", ""); err != nil {
		t.Fatal(err)
	}
	reopened, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reopened.View().Content, "Env: staging") {
		t.Fatal("selection not restored")
	}
}

func TestActiveDestinationWrapsAtNarrowWidths(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, _ := project.Init(root, strings.Repeat("Long project name ", 10), false)
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = "https://staging.example.com/v1"
	if err := project.SaveEnvironment(root, e.Environment, e.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	if err := project.SelectEnvironment(root, e.ID); err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{80, 32} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		view := strings.ReplaceAll(m.View().Content, "\n", "")
		for _, text := range []string{"Env: staging", "nonproduction", "https://staging.example.com/v1"} {
			if !strings.Contains(view, text) {
				t.Fatalf("width %d missing %s: %s", width, text, m.View().Content)
			}
		}
	}
}

func TestEnvironmentDeletionUsesRegistryConfirmationAndRevision(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, err := project.Init(root, "Delete", false); err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{Leader: ","})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	for _, k := range []string{",", "e", "l", "G", ",", "e", "x"} {
		key(m, k)
	}
	if !strings.Contains(m.View().Content, "Delete environment staging") {
		t.Fatal(m.View().Content)
	}
	key(m, "esc")
	snapshot, _ := project.Load(root)
	if _, err := snapshot.Resolve("staging"); err != nil {
		t.Fatal("cancel deleted environment", err)
	}
	for _, k := range []string{",", "e", "x", "y"} {
		key(m, k)
	}
	snapshot, _ = project.Load(root)
	if _, err := snapshot.Resolve("staging"); err == nil {
		t.Fatal("environment not deleted")
	}
}
