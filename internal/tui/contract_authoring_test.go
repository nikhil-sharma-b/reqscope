package tui_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
)

func TestContractAuthoringPreviewApplyAndExport(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Contracts", false)
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: Contracts, version: '1'}\npaths: {}\n"), 0600)
	s, err := project.ImportSpecification(root, file, false)
	if err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	key(m, "space")
	key(m, "d")
	key(m, "a")
	if !strings.Contains(m.View().Content, "Local contract edit JSON") {
		t.Fatal(m.View().Content)
	}
	// Replace the editable JSON buffer through ordinary input.
	for i := 0; i < 1000; i++ {
		m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	}
	edit := map[string]any{"kind": "schema", "action": "create", "sourceId": s.ID, "revision": s.Revision, "importedEdit": true, "name": "Proposed", "value": map[string]any{"type": "string"}}
	b, _ := json.Marshal(edit)
	for _, r := range string(b) {
		key(m, string(r))
	}
	enter(m)
	if !strings.Contains(m.View().Content, "Local contract edit preview") {
		t.Fatal(m.View().Content)
	}
	specs, _ := project.Specifications(root)
	if specs[0].Revision != s.Revision {
		t.Fatal("TUI preview wrote")
	}
	enter(m)
	specs, _ = project.Specifications(root)
	if len(specs[0].Schemas) != 1 || specs[0].Schemas[0].Name != "Proposed" {
		t.Fatal("TUI did not save", m.View().Content)
	}
	key(m, "space")
	key(m, "d")
	key(m, "o")
	if !strings.Contains(m.View().Content, "Proposed") || !strings.Contains(m.View().Content, "rootDocument") {
		t.Fatal("missing export", m.View().Content)
	}
}

func TestContractExportUsesSelectedSource(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Sources", false)
	for _, name := range []string{"First", "Second"} {
		file := filepath.Join(t.TempDir(), name+".yaml")
		os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: "+name+", version: '1'}\npaths: {}\ncomponents: {schemas: {"+name+": {type: string}}}\n"), 0600)
		if _, err := project.ImportSpecification(root, file, false); err != nil {
			t.Fatal(err)
		}
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	key(m, "space")
	key(m, "s")
	key(m, "s")
	key(m, "j")
	key(m, "space")
	key(m, "d")
	key(m, "o")
	view := m.View().Content
	if !strings.Contains(view, "Second.yaml") || strings.Contains(view, "rootDocument: First.yaml") {
		t.Fatal("exported wrong source", view)
	}
}
