package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func TestScenarioCLIResourceLifecycleAndExitPrecedence(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"42"}`)
	}))
	defer server.Close()
	root := t.TempDir()
	workspace, err := project.Init(root, "Scenarios", false)
	if err != nil {
		t.Fatal(err)
	}
	env, _ := workspace.Resolve("staging")
	env.BaseURLs["api"] = server.URL
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Source", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/"}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	s := project.Scenario{ID: uuid.NewString(), Name: "Workflow", Steps: []project.ScenarioStep{{ID: "one", CaseID: c.ID, Extracts: map[string]string{"id": "/id"}}, {ID: "two", CaseID: c.ID, Bindings: map[string]project.ScenarioBinding{"item": {StepID: "one", Extract: "id"}}}}}
	run := func(body string, args ...string) (int, string) {
		t.Helper()
		var out, diag bytes.Buffer
		args = append(args, "--project", root, "--output", "json")
		code := cli.Run(args, strings.NewReader(body), &out, &diag)
		return code, out.String()
	}
	b, _ := json.Marshal(s)
	if code, out := run(string(b), "scenario", "create", "--input", "-", "--preview"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	items, _ := project.Scenarios(root)
	if len(items) != 0 {
		t.Fatal("preview persisted")
	}
	inputPath := filepath.Join(t.TempDir(), "scenario.json")
	if err := os.WriteFile(inputPath, b, 0600); err != nil {
		t.Fatal(err)
	}
	if code, out := run("", "scenario", "create", "--input", inputPath); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	for _, args := range [][]string{{"scenario", "list"}, {"scenario", "get", s.ID}, {"scenario", "validate", s.ID}} {
		if code, out := run("", args...); code != 0 {
			t.Fatalf("%d %s", code, out)
		}
	}
	for _, name := range []string{"scenario.create", "scenario.update", "scenario.delete", "scenario.run"} {
		if code, out := run("", "input-schema", "get", name); code != 0 {
			t.Fatalf("%d %s", code, out)
		}
	}
	if code, out := run("", "case", "get", c.ID); code != 0 || !strings.Contains(out, "scenarios/") {
		t.Fatalf("case impact absent: %d %s", code, out)
	}
	if code, out := run("", "scenario", "run", s.ID); code != 4 || calls != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if code, out := run("", "scenario", "run", s.ID, "--env", "staging"); code != 4 || calls != 0 {
		t.Fatalf("%d %s", code, out)
	}
	review, _ := project.ReviewAccess(root, "staging")
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	if code, out := run("", "scenario", "run", s.ID, "--env", "staging"); code != 0 || calls != 2 || !strings.Contains(out, `"successful":true`) {
		t.Fatalf("%d %s calls=%d", code, out, calls)
	}
	saved, _ := project.GetScenario(root, s.ID)
	s.ContinueOnFailure = true
	s.Steps[0].Extracts["id"] = "/missing"
	update := map[string]any{"id": s.ID, "name": s.Name, "steps": s.Steps, "continueOnFailure": true, "revision": saved.Revision}
	b, _ = json.Marshal(update)
	if code, out := run(string(b), "scenario", "update", "--input", "-"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if code, out := run(string(b), "scenario", "update", "--input", "-"); code != 3 {
		t.Fatalf("%d %s", code, out)
	}
	if code, out := run("", "scenario", "run", s.ID, "--env", "staging"); code != 6 || !strings.Contains(out, `"outcome":"skipped"`) || !strings.Contains(out, "extraction_failed") {
		t.Fatalf("%d %s", code, out)
	}
	saved, _ = project.GetScenario(root, s.ID)
	body := `{"revision":"` + saved.Revision + `"}`
	if code, out := run(body, "scenario", "delete", s.ID, "--input", "-", "--preview"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if code, out := run(body, "scenario", "delete", s.ID, "--input", "-"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if code, out := run("", "scenario", "get", s.ID); code != 3 {
		t.Fatalf("%d %s", code, out)
	}
}
func TestScenarioCLIRejectsUnknownFeaturesAndInvalidInputTypes(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	project.Init(root, "Scenarios", false)
	for _, body := range []string{`{"id":"` + uuid.NewString() + `","name":"Invalid","steps":[]}`, `{"id":"` + uuid.NewString() + `","name":"Invalid","script":"run","steps":[]}`, `{"variables":{"number":3}}`, `{"variables":null}`} {
		name := "scenario.create"
		if strings.Contains(body, "variables") {
			name = "scenario.run"
		}
		var out, diag bytes.Buffer
		code := cli.Run([]string{"input-schema", "validate", name, "--project", root, "--input", "-", "--output", "json"}, strings.NewReader(body), &out, &diag)
		if code != 2 {
			t.Fatalf("%d %s input=%s", code, &out, body)
		}
	}
}
