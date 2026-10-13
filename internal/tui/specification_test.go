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

func enter(m *tui.Model) { m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})) }
func importKeys(m *tui.Model, file string) {
	key(m, "space")
	key(m, "s")
	key(m, "i")
	for _, r := range file {
		key(m, string(r))
	}
	enter(m)
}
func TestImportPreviewAndInspection(t *testing.T) {
	for _, version := range []string{"2.0", "3.0.3", "3.1.0"} {
		for _, format := range []string{"json", "yaml"} {
			t.Run(version+format, func(t *testing.T) {
				root := t.TempDir()
				project.Init(root, "Pets", false)
				m, err := tui.New(root, tui.KeyConfig{})
				if err != nil {
					t.Fatal(err)
				}
				m.Update(tea.WindowSizeMsg{Width: 150, Height: 45})
				field := "openapi"
				schema := "components: {schemas: {Pet: {type: string, x-team: pets}}}"
				if version == "2.0" {
					field = "swagger"
					schema = "definitions: {Pet: {type: string, x-team: pets}}"
				}
				document := field + ": '" + version + "'\ninfo: {title: Pets, version: '1'}\npaths: {'/pets': {get: {description: List pets, tags: [pets], responses: {'200': {description: OK}}}}}\n" + schema + "\n"
				if format == "json" {
					schemaJSON := `"components":{"schemas":{"Pet":{"type":"string","x-team":"pets"}}}`
					if version == "2.0" {
						schemaJSON = `"definitions":{"Pet":{"type":"string","x-team":"pets"}}`
					}
					document = `{"` + field + `":"` + version + `","info":{"title":"Pets","version":"1"},"paths":{"/pets":{"get":{"description":"List pets","responses":{"200":{"description":"OK"}}}}},` + schemaJSON + `}`
				}
				file := filepath.Join(t.TempDir(), "api."+format)
				os.WriteFile(file, []byte(document), 0600)
				importKeys(m, file)
				if !strings.Contains(m.View().Content, "Import preview") {
					t.Fatal(m.View().Content)
				}
				specs, _ := project.Specifications(root)
				if len(specs) != 0 {
					t.Fatal("preview wrote")
				}
				enter(m)
				if !strings.Contains(m.View().Content, "GET /pets") {
					t.Fatal(m.View().Content)
				}
				enter(m)
				if !strings.Contains(m.View().Content, "List pets") {
					t.Fatal("missing endpoint detail")
				}
				key(m, "esc")
				key(m, "space")
				key(m, "s")
				key(m, "s")
				enter(m)
				if !strings.Contains(m.View().Content, "x-team") {
					t.Fatal("schema detail missing extension")
				}
			})
		}
	}
}
func TestInvalidAndCancelledImport(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Pets", false)
	m, _ := tui.New(root, tui.KeyConfig{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("swagger: '9.0'\n"), 0600)
	importKeys(m, file)
	if !strings.Contains(m.View().Content, "supported versions") {
		t.Fatal(m.View().Content)
	}
	key(m, "esc")
	specs, _ := project.Specifications(root)
	if len(specs) != 0 {
		t.Fatal("invalid import wrote")
	}
}

func TestReferencePreviewAndChangedFiles(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Pets", false)
	m, _ := tui.New(root, tui.KeyConfig{})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 35})
	dir := t.TempDir()
	file := filepath.Join(dir, "api.yaml")
	model := filepath.Join(dir, "model.yaml")
	os.WriteFile(file, []byte("openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\npaths: {}\ncomponents: {schemas: {Pet: {$ref: 'model.yaml#/Pet'}}}\n"), 0600)
	importKeys(m, file)
	if !strings.Contains(m.View().Content, "missing reference") {
		t.Fatal(m.View().Content)
	}
	os.WriteFile(model, []byte("Pet: {type: string, x-team: pets}\n"), 0600)
	enter(m)
	if !strings.Contains(m.View().Content, "1 schemas") {
		t.Fatal(m.View().Content)
	}
	os.WriteFile(model, []byte("Pet: {type: integer, x-team: pets}\n"), 0600)
	enter(m)
	if !strings.Contains(m.View().Content, "File changed") {
		t.Fatal(m.View().Content)
	}
	specs, _ := project.Specifications(root)
	if len(specs) != 0 {
		t.Fatal("changed preview applied")
	}
	key(m, "esc")
	specs, _ = project.Specifications(root)
	if len(specs) != 0 {
		t.Fatal("cancel applied")
	}
	importKeys(m, file)
	enter(m)
	key(m, "space")
	key(m, "s")
	key(m, "s")
	enter(m)
	if !strings.Contains(m.View().Content, "integer") {
		t.Fatal(m.View().Content)
	}
}
