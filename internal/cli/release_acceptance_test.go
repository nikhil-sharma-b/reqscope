package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

// The same real project flows through discovery, CLI authoring, protected
// execution, SQLite evidence, comparison and refresh. Approvals use the shared
// interactive boundary; no CLI approval bypass is introduced for the fixture.
func TestReleaseAcceptanceLocalWorkflow(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	const token = "acceptance-issued-session-credential"
	var calls, sequence atomic.Int32
	var refreshed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docs":
			fmt.Fprint(w, `<script>SwaggerUIBundle({url:'openapi.json'})</script>`)
		case "/openapi.json":
			description := "Inspect account profile"
			if refreshed.Load() {
				description = "Inspect refreshed account profile"
			}
			fmt.Fprintf(w, `{"openapi":"3.0.3","info":{"title":"Accounts","version":"1"},"paths":{"/login":{"post":{"responses":{"200":{"description":"OK"}}}},"/me":{"get":{"description":%q,"responses":{"200":{"description":"OK"}}}}}}`, description)
		case "/login":
			calls.Add(1)
			fmt.Fprintf(w, `{"session":%q}`, token)
		case "/me":
			calls.Add(1)
			if r.Header.Get("Authorization") != "Bearer "+token {
				w.WriteHeader(401)
				fmt.Fprint(w, `{"error":"missing authentication"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"name":"Ada","sequence":%d}`, sequence.Add(1))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	run := func(want int, body string, args ...string) map[string]any {
		t.Helper()
		code, result := acceptanceCLI(t, root, body, args...)
		if code != want {
			t.Fatalf("%v: %d %+v", args, code, result)
		}
		b, _ := json.Marshal(result)
		if strings.Contains(string(b), token) {
			t.Fatal("credential in CLI output")
		}
		return result
	}
	encode := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	run(0, "", "project", "init", "Acceptance")
	run(4, "", "spec", "import", "--url", server.URL+"/docs")
	if err := project.ApproveSourceOrigin(root, server.URL); err != nil {
		t.Fatal(err)
	}
	run(0, "", "spec", "import", "--url", server.URL+"/docs")
	for _, query := range []string{"/me", "account profile"} {
		result := run(0, "", "endpoint", "search", "--query", query, "--limit", "1")
		if result["data"].(map[string]any)["total"] != float64(1) {
			t.Fatal(result)
		}
	}
	specs, err := project.Specifications(root)
	if err != nil || len(specs) != 1 {
		t.Fatal(specs, err)
	}
	source := specs[0]
	var me project.Endpoint
	for _, ep := range source.Endpoints {
		if ep.Path == "/me" {
			me = ep
		}
	}
	workspace, _ := project.Load(root)
	env, _ := workspace.Resolve("staging")
	env.BaseURLs[me.ModuleID] = server.URL
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	// Session-only credentials are deliberately provisioned through the TUI's
	// shared boundary. The CLI can use them in this process, but cannot read them.
	credentials := project.DefaultCredentials
	binding := project.CredentialBinding{Name: "acceptance", Kind: "bearer", Module: me.ModuleID, Destination: server.URL + "/me", In: "header", Field: "Authorization"}
	if err := credentials.Save(root, "staging", binding, token, true); err != nil {
		t.Fatal(err)
	}
	defer credentials.Delete(root, "staging", binding.Name)
	c := project.Case{ID: uuid.NewString(), Name: "Profile", Request: project.RequestDraft{EndpointID: me.ID, Headers: []project.RequestField{{Name: "Authorization", Value: "{{secret:acceptance}}"}}}, ComparisonIgnore: project.IgnoreRules{JSONPointers: []string{"/sequence"}, Headers: []string{"Date"}}}
	project.Decode([]byte(`{"assertions":[{"kind":"status-equals","value":200}]}`), &c.Validation)
	run(0, encode(c), "case", "create", "--input", "-")
	run(4, "", "request", "send", "--case", c.ID, "--env", "staging")
	review, err := project.ReviewAccess(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i := 0; i < 2; i++ {
		result := run(0, "", "request", "send", "--case", c.ID, "--env", "staging")
		ids = append(ids, result["data"].(map[string]any)["runId"].(string))
	}
	comparison := run(0, "", "compare", ids[0], ids[1])["data"].(map[string]any)
	if len(comparison["differences"].([]any)) != 0 {
		t.Fatal(comparison)
	}
	login := project.Case{ID: uuid.NewString(), Name: "Login", Request: project.RequestDraft{Module: me.ModuleID, Method: "POST", URL: "/login"}}
	consumer := c
	consumer.ID = uuid.NewString()
	consumer.Name = "Scenario profile"
	consumer.Request.Headers = []project.RequestField{{Name: "Authorization", Value: "{{auth}}"}}
	for _, item := range []project.Case{login, consumer} {
		run(0, encode(item), "case", "create", "--input", "-")
	}
	scenario := project.Scenario{ID: uuid.NewString(), Name: "Secure workflow", Steps: []project.ScenarioStep{
		{ID: "login", CaseID: login.ID, SecretExtracts: map[string]project.SecretExtraction{"session": {Pointer: "/session", Destinations: []project.CredentialBinding{binding}}}},
		{ID: "profile", CaseID: consumer.ID, Bindings: map[string]project.ScenarioBinding{"auth": {StepID: "login", Extract: "session"}}},
	}}
	run(0, encode(scenario), "scenario", "create", "--input", "-")
	run(0, "", "scenario", "validate", scenario.ID)
	approval, err := project.ReviewScenarioExtractions(root, "staging", scenario.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := project.ApproveScenarioExtractions(root, "staging", scenario.ID, approval.Fingerprint); err != nil {
		t.Fatal(err)
	}
	report := run(0, "", "scenario", "run", scenario.ID, "--env", "staging")["data"].(map[string]any)
	runID := report["runId"].(string)
	run(0, "", "history", "get", runID)
	run(0, "", "history", "pin", runID)
	page := run(0, "", "history", "list", "--limit", "1")["data"].(map[string]any)
	if page["nextOffset"] == nil {
		t.Fatal("history did not page")
	}
	before := calls.Load()
	run(4, "", "request", "send", "--case", c.ID, "--env", "production")
	run(4, `{"module":"`+me.ModuleID+`","method":"GET","url":"http://127.0.0.1:1/stolen"}`, "request", "send", "--env", "staging", "--input", "-")
	workspace, _ = project.Load(root)
	env, _ = workspace.Resolve("staging")
	env.BaseURLs[me.ModuleID] = server.URL + "/changed"
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	run(4, "", "request", "send", "--case", c.ID, "--env", "staging")
	if calls.Load() != before {
		t.Fatal("denied request reached backend")
	}
	refreshed.Store(true)
	preview := run(0, "", "spec", "refresh-preview", source.ID)["data"].(map[string]any)
	run(0, encode(map[string]any{"previewToken": preview["previewToken"]}), "spec", "refresh-apply", source.ID, "--input", "-")
	saved, err := project.GetCase(root, c.ID)
	if err != nil || saved.Request.Headers[0].Value != "{{secret:acceptance}}" {
		t.Fatal("refresh lost saved input", err)
	}
	run(0, "", "project", "validate")
	for _, dir := range []string{root, os.Getenv("XDG_DATA_HOME")} {
		if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			b, err := os.ReadFile(path)
			if strings.Contains(string(b), token) {
				t.Errorf("credential persisted in %s", path)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}
