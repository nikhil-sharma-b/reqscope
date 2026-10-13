package tui_test

import (
	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFocusedEndpointFuzzySearchAndNarrowSelection(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Search", false)
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\npaths:\n  /pets:\n    get: {summary: List pets, tags: [pets, pets], responses: {'200': {description: OK}}}\n    post: {summary: Create pet, responses: {'200': {description: OK}}}\n"), 0600)
	if _, err := project.ImportSpecification(root, file, false); err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 45, Height: 24})
	key(m, "g")
	key(m, "e")
	key(m, "/")
	key(m, "p")
	key(m, "t")
	if !strings.Contains(m.View().Content, "GET /pets") {
		t.Fatal(m.View().Content)
	}
	enter(m)
	key(m, "j")
	enter(m)
	if !strings.Contains(m.View().Content, "POST") {
		t.Fatal(m.View().Content)
	}
	key(m, "esc")
	key(m, "space")
	key(m, "/")
	key(m, "x")
	if !strings.Contains(m.View().Content, "No matching") {
		t.Fatal(m.View().Content)
	}
	key(m, "backspace")
	enter(m)
	key(m, "space")
	key(m, "s")
	key(m, "m")
	if strings.Contains(m.View().Content, "POST /pets") {
		t.Fatal("method filter failed", m.View().Content)
	}
	key(m, "space")
	key(m, "s")
	key(m, "t")
	if strings.Count(m.View().Content, "GET /pets") != 1 {
		t.Fatal("duplicate tag result", m.View().Content)
	}
	if !strings.Contains(m.View().Content, "api.yaml") {
		t.Fatal("source clipped", m.View().Content)
	}
	enter(m)
	if !strings.Contains(m.View().Content, "inspection") {
		t.Fatal("diagnostics lost", m.View().Content)
	}
}
