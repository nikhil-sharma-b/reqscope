package tui_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
)

func scenarioWorkspace(t *testing.T, url, envName string) (string, project.CaseResource) {
	t.Helper()
	root := workspace(t, url, envName, true)
	c := saveCase(t, root, "Case", project.RequestDraft{Module: "api", Method: "GET", URL: "/?q={{value}}", Variables: map[string]string{"value": "ordinary"}})
	return root, c
}

func TestScenarioAuthorCheckRunEditDeleteAndCLIParity(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"from-response"}`)
	}))
	defer server.Close()
	root, c := scenarioWorkspace(t, server.URL, "staging")
	m := newModel(t, root, 160, 50)
	steps := "[{id: first, caseId: " + c.ID + ", overrides: {value: step}, extracts: {id: /id}}, {id: second, caseId: " + c.ID + ", bindings: {value: {stepId: first, extract: id}}}]"
	edit := filepath.Join(t.TempDir(), "steps.sed")
	os.WriteFile(edit, []byte("s|^name: .*|name: Workflow|\ns|^steps: \\[\\]|steps: "+steps+"|\n"), 0600)
	captured := filepath.Join(t.TempDir(), "template")
	editorScript(t, `cp "$1" "`+captured+`"; sed -f "`+edit+`" "$1" > "$1.new" && mv "$1.new" "$1"`)
	press(m, "3")
	runEditor(t, m, send(m, "n"))
	template, _ := os.ReadFile(captured)
	if !strings.Contains(string(template), c.ID) || !strings.Contains(string(template), "Case") {
		t.Fatalf("the template does not list saved requests:\n%s", template)
	}
	id := regexp.MustCompile(`(?m)^id: (\S+)`).FindStringSubmatch(string(template))[1]
	saved, err := project.GetScenario(root, id)
	if err != nil || len(saved.Steps) != 2 {
		t.Fatalf("%+v %v\n%s", saved, err, screen(m))
	}
	wantScreen(t, m, "Saved “Workflow”", "2 steps")
	press(m, "v")
	wantScreen(t, m, "“Workflow” is valid")
	press(m, "r")
	if len(requests) != 2 || requests[0] != "/?q=step" || requests[1] != "/?q=from-response" {
		t.Fatalf("%v", requests)
	}
	wantScreen(t, m, "Passed", "first", "second", "200 OK")
	press(m, "esc")
	review, _ := project.ReviewAccess(root, "staging")
	project.ApproveAccess(root, review)
	var out, diag bytes.Buffer
	if code := cli.Run([]string{"scenario", "run", id, "--project", root, "--env", "staging", "--output", "json"}, strings.NewReader(""), &out, &diag); code != 0 || len(requests) != 4 || requests[2] != requests[0] || requests[3] != requests[1] {
		t.Fatalf("%d %s %v", code, &out, requests)
	}
	editorScript(t, `sed 's|^name: .*|name: Renamed|' "$1" > "$1.new" && mv "$1.new" "$1"`)
	runEditor(t, m, send(m, "enter"))
	if saved, err := project.GetScenario(root, id); err != nil || saved.Name != "Renamed" {
		t.Fatalf("%+v %v", saved, err)
	}
	press(m, "d")
	wantScreen(t, m, "Delete “Renamed”?")
	press(m, "y")
	if _, err := project.GetScenario(root, id); err == nil {
		t.Fatal("not deleted")
	}
}

func productionScenario(t *testing.T, handler http.HandlerFunc, continueOnFailure bool) (string, *tui.Model) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	root, c := scenarioWorkspace(t, server.URL, "production")
	s := project.Scenario{ID: uuid.NewString(), Name: "Production", ContinueOnFailure: continueOnFailure, Steps: []project.ScenarioStep{{ID: "one", CaseID: c.ID}, {ID: "two", CaseID: c.ID}}}
	if _, err := project.SaveScenario(root, s, "", false); err != nil {
		t.Fatal(err)
	}
	m := newModel(t, root, 160, 50)
	// Production is never reselected on reopen; choose it explicitly.
	press(m, "e")
	moveTo(t, m, "production")
	press(m, "enter", "3")
	return root, m
}

func TestScenarioAsksBeforeEveryProductionSend(t *testing.T) {
	var calls atomic.Int32
	_, m := productionScenario(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); io.WriteString(w, `{}`) }, true)
	press(m, "r")
	wantScreen(t, m, "Send to production?", "paused before this step")
	rejectScreen(t, m, "q=")
	if calls.Load() != 0 {
		t.Fatal("sent before confirmation")
	}
	press(m, "y")
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
	wantScreen(t, m, "Send to production?")
	press(m, "esc")
	if calls.Load() != 1 {
		t.Fatal("cancelling sent the next step")
	}
	wantScreen(t, m, "Failed", "skipped", "scenario cancelled")
	press(m, "esc", "r", "y", "y")
	if calls.Load() != 3 {
		t.Fatalf("calls=%d", calls.Load())
	}
	wantScreen(t, m, "Passed")
}

func TestScenarioCancelsActiveTransport(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); close(started); <-r.Context().Done() }))
	defer server.Close()
	root, c := scenarioWorkspace(t, server.URL, "staging")
	project.SaveScenario(root, project.Scenario{ID: uuid.NewString(), Name: "Wait", Steps: []project.ScenarioStep{{ID: "one", CaseID: c.ID}, {ID: "two", CaseID: c.ID}}}, "", false)
	m := newModel(t, root)
	press(m, "3")
	cmd := send(m, "r")
	done := make(chan any, 1)
	go func() { done <- cmd() }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("transport never started")
	}
	if send(m, "n") != nil {
		t.Fatal("started editing during a run")
	}
	wantScreen(t, m, "Wait for the running request")
	press(m, "esc")
	select {
	case msg := <-done:
		m.Update(msg)
	case <-time.After(3 * time.Second):
		t.Fatal("cancelling did not stop the transport")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
	wantScreen(t, m, "cancelled")
}

func TestScenarioAsksBeforeFollowingAProductionRedirect(t *testing.T) {
	var calls atomic.Int32
	_, m := productionScenario(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/redirected?token=sensitive", http.StatusFound)
			return
		}
		io.WriteString(w, `{}`)
	}, false)
	press(m, "r", "y")
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
	wantScreen(t, m, "Send to production?", "/redirected")
	rejectScreen(t, m, "token=")
	press(m, "esc")
	if calls.Load() != 1 {
		t.Fatal("cancelled redirect was followed")
	}
	wantScreen(t, m, "scenario cancelled")
}

func TestScenarioLoginApprovalForTheCLI(t *testing.T) {
	const token = "tui-approved-fresh-session"
	var authenticated atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/me" {
			authenticated.Store(r.Header.Get("Authorization") == "Bearer "+token)
		}
		io.WriteString(w, `{"custom":"`+token+`","echo":"`+token+`"}`)
	}))
	defer server.Close()
	root, login := scenarioWorkspace(t, server.URL, "staging")
	login.Request.URL = "/login"
	login, err := project.SaveCase(root, login.Case, login.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	consumer := saveCase(t, root, "Authenticated", project.RequestDraft{Module: "api", Method: "GET", URL: "/me", Headers: []project.RequestField{{Name: "Authorization", Value: "{{auth}}"}}})
	s, err := project.SaveScenario(root, project.Scenario{ID: uuid.NewString(), Name: "Login", Steps: []project.ScenarioStep{
		{ID: "login", CaseID: login.ID, SecretExtracts: map[string]project.SecretExtraction{"session": {Pointer: "/custom", Destinations: []project.CredentialBinding{{Kind: "bearer", Module: "api", Destination: server.URL + "/me", In: "header", Field: "Authorization"}}}}},
		{ID: "me", CaseID: consumer.ID, Bindings: map[string]project.ScenarioBinding{"auth": {StepID: "login", Extract: "session"}}},
	}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	access, _ := project.ReviewAccess(root, "staging")
	project.ApproveAccess(root, access)
	runCLI := func(expected int) {
		t.Helper()
		var out, diag bytes.Buffer
		code := cli.Run([]string{"scenario", "run", s.ID, "--project", root, "--env", "staging", "--output", "json"}, strings.NewReader(""), &out, &diag)
		if code != expected || strings.Contains(out.String()+diag.String(), token) {
			t.Fatalf("code %d: %s %s", code, &out, &diag)
		}
	}
	runCLI(4)
	m := newModel(t, root, 160, 50)
	press(m, "3", "a")
	wantScreen(t, m, "Approve protected logins?", "/custom", server.URL+"/me", "Authorization")
	press(m, "esc")
	runCLI(4)
	press(m, "a", "y")
	runCLI(0)
	if !authenticated.Load() {
		t.Fatal("the CLI did not attach the approved token")
	}
	press(m, "r")
	wantScreen(t, m, "Passed")
	rejectScreen(t, m, token)
}

func TestSharedRequestNamesItsScenariosBeforeSaving(t *testing.T) {
	root, c := scenarioWorkspace(t, "http://localhost:12345", "staging")
	if _, err := project.SaveScenario(root, project.Scenario{ID: uuid.NewString(), Name: "Affected", Steps: []project.ScenarioStep{{ID: "one", CaseID: c.ID}}}, "", false); err != nil {
		t.Fatal(err)
	}
	m := newModel(t, root, 160, 50)
	press(m, "2")
	wantScreen(t, m, "Used by", "Affected")
	press(m, "enter", "enter", "ctrl+u")
	typeText(m, "/changed")
	press(m, "enter", "ctrl+s")
	wantScreen(t, m, "Change a shared request?", "Affected")
	if before, _ := project.GetCase(root, c.ID); before.Request.URL == "/changed" {
		t.Fatal("saved before confirming")
	}
	press(m, "y")
	if after, _ := project.GetCase(root, c.ID); after.Request.URL != "/changed" {
		t.Fatal("confirmed save did not write")
	}
}

func TestHistoryPinViewAndCompare(t *testing.T) {
	root := workspace(t, "http://127.0.0.1:1", "staging", false)
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "comparison", ComparisonIgnore: project.IgnoreRules{JSONPointers: []string{"/ignored"}, Headers: []string{"Date"}}, Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/"}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := project.SendCase(context.Background(), root, "local", c.ID, nil, project.SendOptions{})
	b, _ := project.SendCase(context.Background(), root, "staging", c.ID, nil, project.SendOptions{})
	m := newModel(t, root, 160, 50)
	press(m, "4")
	wantScreen(t, m, "2 runs", "failed")
	press(m, "p")
	wantScreen(t, m, "Pinned", "◆")
	run, _ := project.GetRun(root, b.RunID)
	if !run.Pinned {
		t.Fatal("not pinned")
	}
	press(m, "enter")
	wantScreen(t, m, "Run "+b.RunID[:8], "failed")
	press(m, "esc", "c", "j")
	wantScreen(t, m, "comparing from here")
	press(m, "c")
	wantScreen(t, m, "Comparison", "Before", "After")
	press(m, "esc", ":")
	typeText(m, "compare two runs by id")
	press(m, "enter", "ctrl+u")
	typeText(m, a.RunID+" missing")
	press(m, "enter")
	wantScreen(t, m, "missing or evicted")
	press(m, "ctrl+u")
	typeText(m, a.RunID+" "+b.RunID)
	press(m, "enter")
	wantScreen(t, m, "Comparison")
	for i := 0; i < 25; i++ {
		press(m, "j")
	}
	view := screen(m)
	if !strings.Contains(view, "/ignored") && !strings.Contains(view, "unavailable") {
		t.Fatal(view)
	}
}

func TestScenarioReportReopensFromHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	defer server.Close()
	root, c := scenarioWorkspace(t, server.URL, "staging")
	s, err := project.SaveScenario(root, project.Scenario{ID: uuid.NewString(), Name: "Workflow", Steps: []project.ScenarioStep{{ID: "one", CaseID: c.ID}}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := project.RunScenario(context.Background(), root, "staging", s.ID, project.ScenarioOptions{}); err != nil {
		t.Fatal(err)
	}
	m := newModel(t, root)
	press(m, "4")
	wantScreen(t, m, "scenario", "Workflow")
	press(m, "enter")
	wantScreen(t, m, "Passed", "one", "passed")
}

func TestCtrlCDuringAProductionPromptStopsTheScenario(t *testing.T) {
	var calls atomic.Int32
	_, m := productionScenario(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); io.WriteString(w, `{}`) }, true)
	press(m, "r")
	wantScreen(t, m, "Send to production?")
	press(m, "ctrl+c")
	if calls.Load() != 0 {
		t.Fatal("sent after cancelling")
	}
	wantScreen(t, m, "Failed", "scenario cancelled")
	rejectScreen(t, m, "Running Production")
	// Nothing is left running: a new run starts normally.
	press(m, "esc", "r")
	wantScreen(t, m, "Send to production?")
}
