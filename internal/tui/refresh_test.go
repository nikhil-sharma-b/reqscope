package tui_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
)

func refreshKeys(m *tui.Model, seq ...string) {
	for _, k := range seq {
		key(m, k)
	}
}
func pollWorkspace(t *testing.T, m *tui.Model) {
	t.Helper()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("missing project poll")
	}
	m.Update(cmd())
}
func caseRefreshFixture(t *testing.T) (string, project.CaseResource, *tui.Model) {
	t.Helper()
	root := t.TempDir()
	if _, err := project.Init(root, "Concurrent", false); err != nil {
		t.Fatal(err)
	}
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Saved", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/original", BodyType: "raw"}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	refreshKeys(m, "space", "c", "l", "space", "c", "o")
	return root, c, m
}
func TestExternalCaseReloadKeepsOpenCleanEditorCurrent(t *testing.T) {
	root, c, m := caseRefreshFixture(t)
	refreshKeys(m, "space", "c", "e")
	c.Request.URL = "/external"
	if _, err := project.SaveCase(root, c.Case, c.Revision, false); err != nil {
		t.Fatal(err)
	}
	pollWorkspace(t, m)
	if !strings.Contains(m.View().Content, "/external") {
		t.Fatal("clean editor did not reload:", m.View().Content)
	}
	enter(m)
	current, err := project.GetCase(root, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Request.URL != "/external" {
		t.Fatal("clean editor silently overwrote external inputs:", current.Request.URL)
	}
}

func TestExternalCaseChangesRetainDirtyDraftAndRequireReconciliation(t *testing.T) {
	root, c, m := caseRefreshFixture(t)
	refreshKeys(m, "space", "r", "e")
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	local := c.Request
	local.URL = "/local-draft"
	raw, _ := json.Marshal(local)
	m.Update(tea.KeyPressMsg{Text: string(raw)})
	enter(m)
	c.Request.URL = "/external"
	external, err := project.SaveCase(root, c.Case, c.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	pollWorkspace(t, m)
	if view := m.View().Content; !strings.Contains(view, "/local-draft") || !strings.Contains(view, "Case changed externally") {
		t.Fatal(view)
	}
	refreshKeys(m, "space", "c", "s")
	current, _ := project.GetCase(root, c.ID)
	if current.Request.URL != "/external" {
		t.Fatal("dirty save overwrote concurrent edit")
	}
	refreshKeys(m, "space", "c", "m")
	if view := m.View().Content; !strings.Contains(view, "Manual case reconciliation") || !strings.Contains(view, "/external") || !strings.Contains(view, "/local-draft") {
		t.Fatal(view)
	}
	enter(m)
	refreshKeys(m, "space", "c", "s")
	current, _ = project.GetCase(root, c.ID)
	if current.Request.URL != "/local-draft" {
		t.Fatal("explicit reconciliation did not save local inputs")
	}
	// Invalid external data must not replace the usable buffer or be overwritten.
	path := filepath.Join(root, "cases", c.ID+".yaml")
	if err := os.WriteFile(path, []byte("invalid: ["), 0600); err != nil {
		t.Fatal(err)
	}
	pollWorkspace(t, m)
	if !strings.Contains(m.View().Content, "/local-draft") {
		t.Fatal("invalid external file lost buffer")
	}
	// Restore a valid external revision, then explicitly discard/reload the draft.
	b, _ := json.Marshal(external.Case)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	refreshKeys(m, "space", "c", "z")
	if !strings.Contains(m.View().Content, "/external") {
		t.Fatal("explicit reload failed", m.View().Content)
	}
}

func TestTUIRefreshConfirmsSelectedSourceAndRetainsDirtyProjectName(t *testing.T) {
	root := t.TempDir()
	if _, err := project.Init(root, "Workspace", false); err != nil {
		t.Fatal(err)
	}
	files := []string{}
	sources := []project.Specification{}
	for _, name := range []string{"First", "Second"} {
		file := filepath.Join(t.TempDir(), name+".yaml")
		raw := "openapi: 3.0.3\ninfo: {title: " + name + ", version: '1'}\npaths: {}\ncomponents: {schemas: {" + name + ": {type: string}}}\n"
		if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		s, err := project.ImportSpecification(root, file, false)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
		sources = append(sources, s)
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	refreshKeys(m, "i", "x", "esc")
	raw := "openapi: 3.0.3\ninfo: {title: Second, version: '1'}\npaths: {'/new': {get: {responses: {'200': {description: OK}}}}}\ncomponents: {schemas: {Second: {type: string}}}\n"
	if err := os.WriteFile(files[1], []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	refreshKeys(m, "space", "s", "s", "j")
	refreshKeys(m, "space", "d", "r")
	if !strings.Contains(m.View().Content, "Refresh preview") {
		t.Fatal(m.View().Content)
	}
	current, _ := project.Specifications(root)
	if current[1].Revision != sources[1].Revision {
		t.Fatal("preview wrote")
	}
	key(m, "esc")
	current, _ = project.Specifications(root)
	if current[1].Revision != sources[1].Revision {
		t.Fatal("cancel wrote")
	}
	refreshKeys(m, "space", "d", "r")
	enter(m)
	current, _ = project.Specifications(root)
	if len(current[1].Endpoints) != 1 || current[0].Revision != sources[0].Revision {
		t.Fatal("refresh targeted wrong source")
	}
	refreshKeys(m, "space", "p", "s")
	snapshot, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Manifest.Name != "Workspacex" {
		t.Fatal("refresh lost dirty project name:", snapshot.Manifest.Name)
	}
}

func TestTUIRefreshBlocksConflictsAndStaleConfirmation(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale", true: "conflict"}[conflict], func(t *testing.T) {
			root := t.TempDir()
			if _, err := project.Init(root, "Refresh", false); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "api.yaml")
			write := func(kind, version string) {
				t.Helper()
				raw := "openapi: 3.0.3\ninfo: {title: Refresh, version: '" + version + "'}\npaths: {}\ncomponents: {schemas: {Value: {type: " + kind + "}}}\n"
				if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write("string", "1")
			s, err := project.ImportSpecification(root, file, false)
			if err != nil {
				t.Fatal(err)
			}
			if conflict {
				_, err := project.EditContract(root, "schema", "update", project.ContractEdit{SourceID: s.ID, Revision: s.Revision, ImportedEdit: true, Name: "Value", Value: map[string]any{"type": "integer"}}, false)
				if err != nil {
					t.Fatal(err)
				}
				write("boolean", "2")
			} else {
				write("string", "2")
			}
			before, _ := project.Specifications(root)
			m, err := tui.New(root, tui.KeyConfig{})
			if err != nil {
				t.Fatal(err)
			}
			m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
			refreshKeys(m, "space", "d", "r")
			if !strings.Contains(m.View().Content, "Refresh preview") {
				t.Fatal(m.View().Content)
			}
			if conflict {
				if !strings.Contains(m.View().Content, "Conflict:") {
					t.Fatal("missing conflicts", m.View().Content)
				}
			} else {
				write("string", "3")
			}
			enter(m)
			after, _ := project.Specifications(root)
			if before[0].Revision != after[0].Revision {
				t.Fatal("confirmation applied conflicting/stale preview")
			}
		})
	}
}
