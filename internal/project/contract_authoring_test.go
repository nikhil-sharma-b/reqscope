package project_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func authorFixture(t *testing.T) (string, project.Specification) {
	t.Helper()
	root := t.TempDir()
	if _, err := project.Init(root, "Contracts", false); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "api.yaml")
	doc := `openapi: 3.0.3
info: {title: Contracts, version: '1'}
x-owner: {team: pets}
paths:
  /pets:
    x-note: preserved
    get:
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pet'}
  /other:
    get: {responses: {'200': {description: OK}}}
components:
  schemas:
    Pet: {type: string, x-extra: kept}
`
	if err := os.WriteFile(file, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := project.ImportSpecification(root, file, false)
	if err != nil {
		t.Fatal(err)
	}
	return root, s
}
func contractEdit(s project.Specification) project.ContractEdit {
	return project.ContractEdit{SourceID: s.ID, Revision: s.Revision, ImportedEdit: true, Name: "Pet", Value: map[string]any{"type": "integer", "x-extra": "kept"}}
}
func TestContractEditsPreserveImportAndIdentity(t *testing.T) {
	root, s := authorFixture(t)
	edit := contractEdit(s)
	preview, err := project.EditContract(root, "schema", "update", edit, true)
	if err != nil {
		t.Fatal(err)
	}
	loaded, _ := project.Specifications(root)
	if loaded[0].Revision != s.Revision {
		t.Fatal("preview wrote source")
	}
	result, err := project.EditContract(root, "schema", "update", edit, false)
	if err != nil {
		t.Fatal(err)
	}
	next := result.Specification
	if !reflect.DeepEqual(s.Documents, next.Documents) || !reflect.DeepEqual(s.Base, next.Base) || s.Digest != next.Digest || s.Location != next.Location {
		t.Fatal("imported base or source identity changed")
	}
	if next.Schemas[0].ID != s.Schemas[0].ID || next.Endpoints[0].ID != s.Endpoints[0].ID {
		t.Fatal("stable IDs changed")
	}
	if preview.Specification.Schemas[0].ID != next.Schemas[0].ID {
		t.Fatal("preview IDs differ")
	}
	exported, err := project.ExportContract(root, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	docs := exported["documents"].(map[string]string)
	var doc map[string]any
	if err = project.Decode([]byte(docs["api.yaml"]), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["x-owner"].(map[string]any)["team"] != "pets" || !strings.Contains(docs["api.yaml"], "$ref:") || !strings.Contains(docs["api.yaml"], "x-note: preserved") {
		t.Fatal("extensions or references lost")
	}
	if _, err = project.EditContract(root, "schema", "update", edit, false); err == nil {
		t.Fatal("accepted stale revision")
	}
}
func TestContractDeletionImpactAndAtomicReassignment(t *testing.T) {
	root, s := authorFixture(t)
	var pet, other project.Endpoint
	for _, e := range s.Endpoints {
		if e.Path == "/pets" {
			pet = e
		} else {
			other = e
		}
	}
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Pet case", Request: project.RequestDraft{EndpointID: pet.ID}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := project.SaveScenario(root, project.Scenario{ID: uuid.NewString(), Name: "Pet flow", Steps: []project.ScenarioStep{{ID: "one", CaseID: c.ID}}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	edit := project.ContractEdit{SourceID: s.ID, Revision: s.Revision, ImportedEdit: true, Path: "/pets", Method: "GET"}
	preview, err := project.EditContract(root, "endpoint", "delete", edit, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Cases) != 1 || len(preview.Scenarios) != 1 || preview.Scenarios[0].ID != flow.ID {
		t.Fatal("missing impact")
	}
	if _, err = project.EditContract(root, "endpoint", "delete", edit, false); err == nil {
		t.Fatal("deleted referenced endpoint")
	}
	edit.ReassignTo = other.ID
	result, err := project.EditContract(root, "endpoint", "delete", edit, false)
	if err != nil {
		t.Fatal(err)
	}
	current, err := project.GetCase(root, c.ID)
	if err != nil || current.Request.EndpointID != other.ID {
		t.Fatal("case not reassigned", err)
	}
	if len(result.Specification.DeletedEndpoints) != 1 || result.Specification.DeletedEndpoints[0].ID != pet.ID {
		t.Fatal("missing tombstone")
	}
}
func TestContractInvalidAndProtectedEditsDoNotWrite(t *testing.T) {
	root, s := authorFixture(t)
	before, err := os.ReadFile(filepath.Join(root, "specs", s.ID+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	edits := []struct {
		kind, action string
		edit         project.ContractEdit
	}{
		{"schema", "delete", contractEdit(s)},
		{"endpoint", "update", project.ContractEdit{SourceID: s.ID, Revision: s.Revision, ImportedEdit: true, Path: "/pets", Method: "GET", Value: map[string]any{"responses": map[string]any{"200": map[string]any{"description": "OK"}}, "servers": []any{map[string]any{"url": "https://changed.example"}}}}},
		{"endpoint", "update", project.ContractEdit{SourceID: s.ID, Revision: s.Revision, ImportedEdit: true, Path: "/pets", Method: "GET", Value: map[string]any{"responses": map[string]any{"200": map[string]any{"description": "OK"}}, "security": []any{}}}},
		{"schema", "update", project.ContractEdit{SourceID: s.ID, Revision: s.Revision, Name: "Pet", Value: map[string]any{"type": "integer"}}},
		{"endpoint", "update", project.ContractEdit{SourceID: s.ID, Revision: s.Revision, ImportedEdit: true, Path: "/pets", Method: "GET", Value: map[string]any{"summary": "invalid"}}},
	}
	for _, tc := range edits {
		if _, err := project.EditContract(root, tc.kind, tc.action, tc.edit, false); err == nil {
			t.Fatalf("accepted invalid edit %+v", tc)
		}
		after, _ := os.ReadFile(filepath.Join(root, "specs", s.ID+".yaml"))
		if !bytes.Equal(before, after) {
			t.Fatal("invalid edit wrote source")
		}
	}
	// Export does not include unrelated private or project resources.
	os.MkdirAll(filepath.Join(root, ".local"), 0700)
	os.WriteFile(filepath.Join(root, ".local", "history"), []byte("private-response-secret"), 0600)
	export, err := project.ExportContract(root, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(export)
	if strings.Contains(string(b), "private-response-secret") || strings.Contains(string(b), "affectedCases") {
		t.Fatal("export leaked non-contract state")
	}
}
func TestContractCreateUpdateDelete(t *testing.T) {
	root, s := authorFixture(t)
	for _, kind := range []string{"endpoint", "schema"} {
		edit := project.ContractEdit{SourceID: s.ID, Revision: s.Revision, ImportedEdit: true}
		if kind == "schema" {
			edit.Name = "New"
			edit.Value = map[string]any{"type": "string"}
		} else {
			edit.Path = "/new"
			edit.Method = "POST"
			edit.Value = map[string]any{"responses": map[string]any{"201": map[string]any{"description": "Created"}}}
		}
		for _, action := range []string{"create", "update", "delete"} {
			result, err := project.EditContract(root, kind, action, edit, false)
			if err != nil {
				t.Fatalf("%s %s: %v", kind, action, err)
			}
			s = result.Specification
			edit.Revision = s.Revision
		}
	}
}

func TestContractCredentialValuesCannotBePersistedOrExported(t *testing.T) {
	root, s := authorFixture(t)
	c := project.NewCredentials(&credentialStore{values: map[string]string{}})
	binding := project.CredentialBinding{Name: "login", Kind: "bearer", Module: s.ModuleID, Destination: "https://example.test/", In: "header", Field: "Authorization"}
	const secret = "contract-private-secret"
	snapshot, _ := project.Load(root)
	env, _ := snapshot.Resolve("staging")
	env.BaseURLs[s.ModuleID] = "https://example.test/"
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(root, "staging", binding, secret, true); err != nil {
		t.Fatal(err)
	}
	edit := contractEdit(s)
	edit.Value = map[string]any{"type": "string", "example": secret}
	if _, err := project.EditContract(root, "schema", "update", edit, false, c); err == nil {
		t.Fatal("persisted credential")
	}
	loaded, _ := project.Specifications(root)
	if loaded[0].Revision != s.Revision {
		t.Fatal("rejected credential edit wrote source")
	}
	// Simulate a pre-existing contract containing an echoed local credential.
	file := filepath.Join(t.TempDir(), "echo.yaml")
	os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: Echo, version: '1'}\npaths: {}\nx-example: "+secret+"\n"), 0600)
	echoed, err := project.ImportSpecification(root, file, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = project.ExportContract(root, echoed.ID, c); err == nil {
		t.Fatal("exported credential")
	}
	for _, tail := range []string{"paths: {}\n# " + secret + "\n", "paths: {'/x': {get: {responses: {200: {description: " + secret + "}}}}}\n"} {
		os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: Echo, version: '1'}\n"+tail), 0600)
		unsafe, err := project.ImportSpecification(root, file, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = project.ExportContract(root, unsafe.ID, c); err == nil {
			t.Fatal("export leaked comment or numeric-key credential")
		}
		edit := project.ContractEdit{SourceID: unsafe.ID, Revision: unsafe.Revision, ImportedEdit: true, Name: "New", Value: map[string]any{"type": "string"}}
		if _, err = project.EditContract(root, "schema", "create", edit, false, c); err == nil {
			t.Fatal("edit retained protected credential")
		}
	}

}

func TestContractEditsRetainedReferenceDocuments(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "References", false)
	dir := t.TempDir()
	main := filepath.Join(dir, "api.yaml")
	os.WriteFile(main, []byte("openapi: 3.0.3\ninfo: {title: References, version: '1'}\npaths:\n  /pets:\n    get: {responses: {'200': {description: OK, content: {application/json: {schema: {$ref: 'models.yaml#/components/schemas/Pet'}}}}}}\n"), 0600)
	os.WriteFile(filepath.Join(dir, "models.yaml"), []byte("components:\n  schemas:\n    Pet: {type: string, x-owner: team}\n"), 0600)
	s, err := project.ImportSpecification(root, main, false)
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(dir)
	edit := contractEdit(s)
	edit.Document = "models.yaml"
	result, err := project.EditContract(root, "schema", "update", edit, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(s.Documents["api.yaml"], result.Specification.LocalDocuments["api.yaml"]) {
		t.Fatal("rewrote referencing document")
	}
	if !strings.Contains(string(result.Specification.LocalDocuments["api.yaml"]), "models.yaml#/components/schemas/Pet") {
		t.Fatal("lost external reference")
	}
	edit.Revision = result.Specification.Revision
	if _, err = project.EditContract(root, "schema", "delete", edit, false); err == nil {
		t.Fatal("allowed dangling cross-document reference")
	}
}
