package tui_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
)

func startImport(t *testing.T, m *tui.Model, location string) {
	t.Helper()
	press(m, "1", "i")
	typeText(m, location)
	press(m, "enter")
}

func TestImportPreviewAndInspection(t *testing.T) {
	for _, version := range []string{"2.0", "3.0.3", "3.1.0"} {
		for _, format := range []string{"json", "yaml"} {
			t.Run(version+format, func(t *testing.T) {
				root := t.TempDir()
				project.Init(root, "Pets", false)
				m := newModel(t, root, 150, 45)
				field, schema := "openapi", "components: {schemas: {Pet: {type: string, x-team: pets}}}"
				if version == "2.0" {
					field, schema = "swagger", "definitions: {Pet: {type: string, x-team: pets}}"
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
				startImport(t, m, file)
				wantScreen(t, m, "Import Pets?", "1 endpoint, 1 schema")
				if specs, _ := project.Specifications(root); len(specs) != 0 {
					t.Fatal("preview wrote")
				}
				press(m, "enter")
				wantScreen(t, m, "/pets", "Imported 1 endpoint.")
				// The wide layout previews the selection; v shows all of it.
				wantScreen(t, m, "List pets", "Responses")
				press(m, "v")
				wantScreen(t, m, "description: List pets")
				press(m, "esc", "s")
				wantScreen(t, m, "1 schema", "Pet")
				press(m, "enter")
				wantScreen(t, m, "x-team")
			})
		}
	}
}

func TestInvalidAndCancelledImport(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Pets", false)
	m := newModel(t, root, 120, 30)
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("swagger: '9.0'\n"), 0600)
	startImport(t, m, file)
	wantScreen(t, m, "Import an API", "supported versions")
	press(m, "esc")
	rejectScreen(t, m, "Import an API")
	if specs, _ := project.Specifications(root); len(specs) != 0 {
		t.Fatal("invalid import wrote")
	}
}

func TestImportRejectsChangedSourceAfterPreview(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Pets", false)
	m := newModel(t, root, 150, 35)
	dir := t.TempDir()
	file, model := filepath.Join(dir, "api.yaml"), filepath.Join(dir, "model.yaml")
	os.WriteFile(file, []byte("openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\npaths: {}\ncomponents: {schemas: {Pet: {$ref: 'model.yaml#/Pet'}}}\n"), 0600)
	startImport(t, m, file)
	wantScreen(t, m, "missing reference")
	os.WriteFile(model, []byte("Pet: {type: string, x-team: pets}\n"), 0600)
	press(m, "enter")
	wantScreen(t, m, "0 endpoints, 1 schema")
	os.WriteFile(model, []byte("Pet: {type: integer, x-team: pets}\n"), 0600)
	press(m, "enter")
	wantScreen(t, m, "The source changed since the preview")
	if specs, _ := project.Specifications(root); len(specs) != 0 {
		t.Fatal("changed preview applied")
	}
	press(m, "esc")
	if specs, _ := project.Specifications(root); len(specs) != 0 {
		t.Fatal("cancel applied")
	}
	startImport(t, m, file)
	press(m, "enter", "s", "enter")
	wantScreen(t, m, "integer")
}

func TestEndpointFilterMethodAndGrouping(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Search", false)
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\npaths:\n  /pets:\n    get: {summary: List pets, tags: [pets, pets], responses: {'200': {description: OK}}}\n    post: {summary: Create pet, responses: {'200': {description: OK}}}\n  /stores:\n    get: {summary: List stores, responses: {'200': {description: OK}}}\n"), 0600)
	if _, err := project.ImportSpecification(root, file, false); err != nil {
		t.Fatal(err)
	}
	m := newModel(t, root, 60, 24)
	press(m, "/", "p", "t")
	wantScreen(t, m, "/ pt", "/pets")
	press(m, "enter", "j", "enter")
	wantScreen(t, m, "‹ Endpoints", "POST /pets")
	press(m, "esc", "esc", "/", "x", "y", "z")
	wantScreen(t, m, "Nothing matches")
	press(m, "esc")
	wantScreen(t, m, "3 endpoints")
	press(m, "m")
	wantScreen(t, m, "GET only", "/stores")
	rejectScreen(t, m, "POST")
	press(m, "m", "m", "m", "m", "m", "m", "m", "t")
	wantScreen(t, m, "by tag", "Untagged", "pets")
	if n := strings.Count(screen(m), "List pets"); n != 1 {
		t.Fatalf("an endpoint tagged twice with one tag listed %d times:\n%s", n, screen(m))
	}
	press(m, "v")
	wantScreen(t, m, "# From", "api.yaml", "Contract inspection only")
}

func TestSourceApprovalSelectionAndCancellation(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	project.Init(root, "Pets", false)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/docs":
			fmt.Fprint(w, `<script>SwaggerUIBundle({urls:[{name:'First',url:'first.json'},{name:'Second',url:'second.json'}]})</script>`)
		case "/second.json":
			fmt.Fprint(w, `{"openapi":"3.1.0","info":{"title":"Pets","version":"1"},"paths":{"/second":{"get":{"responses":{"200":{"description":"OK"}}}}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	m := newModel(t, root)
	startImport(t, m, server.URL+"/docs")
	if calls != 0 {
		t.Fatal("contacted the origin before approval")
	}
	wantScreen(t, m, "Fetch from "+server.URL+"?")
	press(m, "esc")
	wantScreen(t, m, "Import an API", server.URL+"/docs")
	press(m, "esc")
	startImport(t, m, server.URL+"/docs")
	press(m, "enter")
	wantScreen(t, m, "Which API?", "First", "Second")
	press(m, "j", "enter")
	wantScreen(t, m, "Import Pets?")
	if specs, _ := project.Specifications(root); len(specs) != 0 {
		t.Fatal("preview wrote")
	}
	press(m, "enter")
	wantScreen(t, m, "/second")
	if specs, _ := project.Specifications(root); len(specs) != 1 {
		t.Fatalf("imported %d sources", len(specs))
	}
}

func TestInvalidDiscoveredSourceOffersRawURL(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	project.Init(root, "Pets", false)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/docs" {
			fmt.Fprint(w, `<script>SwaggerUIBundle({url:'invalid.json'})</script>`)
			return
		}
		fmt.Fprint(w, "<html>Authentication required</html>")
	}))
	defer server.Close()
	m := newModel(t, root, 150, 45)
	startImport(t, m, server.URL+"/docs")
	press(m, "enter")
	wantScreen(t, m, "raw JSON/YAML URL", "invalid.json")
	if specs, _ := project.Specifications(root); len(specs) != 0 {
		t.Fatal("failed import wrote")
	}
}

func TestAuthenticatedImportUsesActiveEnvironmentCredential(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, _ := project.Init(root, "Pets", false)
	const secret = "tui-swagger-secret"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("missing active environment credential")
		}
		fmt.Fprint(w, `{"openapi":"3.1.0","info":{"title":"Pets","version":"1"},"paths":{}}`)
	}))
	defer server.Close()
	env, _ := s.Resolve("staging")
	env.BaseURLs["api"] = server.URL
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	project.SelectEnvironment(root, env.ID)
	vault := project.NewCredentials(nil)
	if err := vault.Save(root, env.ID, project.CredentialBinding{Name: "swagger", Kind: "bearer", Module: "api", Destination: server.URL + "/api.json", In: "header", Field: "Authorization"}, secret, true); err != nil {
		t.Fatal(err)
	}
	m := newCredentialModel(t, root, vault)
	startImport(t, m, server.URL+"/api.json")
	if calls != 0 {
		t.Fatal("contacted the origin before approval")
	}
	press(m, "enter")
	wantScreen(t, m, "Import Pets?")
	rejectScreen(t, m, secret)
	press(m, "enter")
	if specs, err := project.Specifications(root); err != nil || len(specs) != 1 {
		t.Fatalf("%+v %v", specs, err)
	}
	if _, err := project.CheckCLIAccess(root, env.ID, "api", server.URL); err == nil {
		t.Fatal("source approval conferred request access")
	}
}

func TestContractEditPreviewApplyAndExport(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Contracts", false)
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: Contracts, version: '1'}\npaths: {}\n"), 0600)
	s, err := project.ImportSpecification(root, file, false)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(t, root, 160, 50)
	edit := filepath.Join(t.TempDir(), "edit.yaml")
	os.WriteFile(edit, []byte("kind: schema\naction: create\nsourceId: "+s.ID+"\nrevision: "+s.Revision+"\nimportedEdit: true\nname: Proposed\nvalue: {type: string}\n"), 0600)
	editorScript(t, `cp "`+edit+`" "$1"`)
	press(m, ":")
	typeText(m, "change an endpoint or schema")
	runEditor(t, m, send(m, "enter"))
	wantScreen(t, m, "Apply this contract change?", "create schema")
	if specs, _ := project.Specifications(root); specs[0].Revision != s.Revision {
		t.Fatal("preview wrote")
	}
	press(m, "enter")
	if specs, _ := project.Specifications(root); len(specs[0].Schemas) != 1 || specs[0].Schemas[0].Name != "Proposed" {
		t.Fatalf("not saved:\n%s", screen(m))
	}
	press(m, ":")
	typeText(m, "copy the contract")
	press(m, "enter")
	wantScreen(t, m, "Contract documents", "Proposed", "rootDocument")
}

func TestContractExportAndRefreshUseSelectedSource(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Sources", false)
	var files []string
	var sources []project.Specification
	for _, name := range []string{"First", "Second"} {
		file := filepath.Join(t.TempDir(), name+".yaml")
		os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: "+name+", version: '1'}\npaths: {}\ncomponents: {schemas: {"+name+": {type: string}}}\n"), 0600)
		s, err := project.ImportSpecification(root, file, false)
		if err != nil {
			t.Fatal(err)
		}
		files, sources = append(files, file), append(sources, s)
	}
	m := newModel(t, root, 160, 50)
	press(m, "s", "j", ":")
	typeText(m, "copy the contract")
	press(m, "enter")
	wantScreen(t, m, "Second.yaml")
	rejectScreen(t, m, "rootDocument: First.yaml")
	press(m, "esc")

	os.WriteFile(files[1], []byte("openapi: 3.0.3\ninfo: {title: Second, version: '1'}\npaths: {'/new': {get: {responses: {'200': {description: OK}}}}}\ncomponents: {schemas: {Second: {type: string}}}\n"), 0600)
	press(m, "R")
	wantScreen(t, m, "Apply the new API version?", "1 added")
	if current, _ := project.Specifications(root); current[1].Revision != sources[1].Revision {
		t.Fatal("preview wrote")
	}
	press(m, "esc")
	if current, _ := project.Specifications(root); current[1].Revision != sources[1].Revision {
		t.Fatal("cancel wrote")
	}
	press(m, "R", "enter")
	current, _ := project.Specifications(root)
	if len(current[1].Endpoints) != 1 || current[0].Revision != sources[0].Revision {
		t.Fatal("refresh targeted the wrong source")
	}
	wantScreen(t, m, "API refreshed")
}

func TestRefreshBlocksConflictsAndStalePreviews(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale", true: "conflict"}[conflict], func(t *testing.T) {
			root := t.TempDir()
			project.Init(root, "Refresh", false)
			file := filepath.Join(t.TempDir(), "api.yaml")
			write := func(kind, version string) {
				os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: Refresh, version: '"+version+"'}\npaths: {}\ncomponents: {schemas: {Value: {type: "+kind+"}}}\n"), 0600)
			}
			write("string", "1")
			s, err := project.ImportSpecification(root, file, false)
			if err != nil {
				t.Fatal(err)
			}
			if conflict {
				if _, err := project.EditContract(root, "schema", "update", project.ContractEdit{SourceID: s.ID, Revision: s.Revision, ImportedEdit: true, Name: "Value", Value: map[string]any{"type": "integer"}}, false); err != nil {
					t.Fatal(err)
				}
				write("boolean", "2")
			} else {
				write("string", "2")
			}
			before, _ := project.Specifications(root)
			m := newModel(t, root, 180, 50)
			press(m, "R")
			if conflict {
				wantScreen(t, m, "This refresh has conflicts")
				rejectScreen(t, m, "enter apply")
			} else {
				wantScreen(t, m, "Apply the new API version?")
				write("string", "3")
			}
			press(m, "enter")
			after, _ := project.Specifications(root)
			if before[0].Revision != after[0].Revision {
				t.Fatal("applied a conflicting or stale preview")
			}
		})
	}
}

func TestImportSummarisesUncheckedSchemaFeatures(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Sintio", false)
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte(`openapi: 3.0.3
info: {title: Sintio, version: '1'}
paths:
  /sessions/{id}:
    get:
      parameters: [{name: id, in: path, required: true, schema: {type: string}}]
      responses:
        '200': {description: OK, content: {application/json: {schema: {oneOf: [{$ref: '#/components/schemas/Card'}, {type: string}]}}}}
  /cards:
    get:
      responses: {'200': {description: OK}}
components:
  schemas:
    Base: {type: object, properties: {id: {type: string}}}
    Card: {allOf: [{$ref: '#/components/schemas/Base'}, {type: object}]}
    Invoice: {allOf: [{$ref: '#/components/schemas/Base'}]}
`), 0600)
	m := newModel(t, root)
	startImport(t, m, file)
	wantScreen(t, m, "Import Sintio?", "2 endpoints, 3 schemas", "Ready to import", "use allOf", "oneOf", "responses are just not checked", "v details")
	rejectScreen(t, m, "Unsupported constraint", "Notes")
	press(m, "v")
	wantScreen(t, m, "Import notes", "schema Invoice", "GET /sessions/{id}  responses/200", "esc back")
	press(m, "esc")
	wantScreen(t, m, "Import Sintio?")
	press(m, "enter")
	// One endpoint's contract lists only its own notes.
	moveTo(t, m, "/cards")
	press(m, "v")
	wantScreen(t, m, "Contract inspection only")
	rejectScreen(t, m, "Not used to check responses")
	press(m, "esc")
	moveTo(t, m, "/sessions/{id}")
	press(m, "v")
	wantScreen(t, m, "Not used to check responses: oneOf")
	rejectScreen(t, m, "Invoice")
}
