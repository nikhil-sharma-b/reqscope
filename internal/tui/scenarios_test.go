package tui_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
)

func scenarioKeys(m *tui.Model, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		_, cmd = m.Update(tea.KeyPressMsg{Text: k, Code: []rune(k)[0]})
	}
	return cmd
}
func scenarioTUIWorkspace(t *testing.T, url, envName string) (string, project.CaseResource, *tui.Model) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	workspace, err := project.Init(root, "Scenarios", false)
	if err != nil {
		t.Fatal(err)
	}
	env, _ := workspace.Resolve(envName)
	env.BaseURLs["api"] = url
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	if err := project.SelectEnvironment(root, env.ID); err != nil {
		t.Fatal(err)
	}
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Case", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/?q={{value}}", Variables: map[string]string{"value": "ordinary"}}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 100})
	return root, c, m
}
func scenarioTUIStart(t *testing.T, m *tui.Model) tea.Cmd {
	t.Helper()
	scenarioKeys(m, "space", "s", "l")
	cmd := scenarioKeys(m, "space", "s", "r")
	if cmd == nil {
		t.Fatal("scenario run not scheduled", m.View().Content)
	}
	return cmd
}
func TestScenarioTUIAuthorValidateRunEditDeleteAndCLIParity(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"from-response"}`)
	}))
	defer server.Close()
	root, c, m := scenarioTUIWorkspace(t, server.URL, "staging")
	scenarioKeys(m, "space", "s", "n")
	id := regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`).FindString(m.View().Content)
	if id == "" {
		t.Fatal("new scenario editor absent", m.View().Content)
	}
	s := project.Scenario{ID: id, Name: "Workflow", Steps: []project.ScenarioStep{{ID: "first", CaseID: c.ID, Overrides: map[string]string{"value": "step"}, Extracts: map[string]string{"id": "/id"}}, {ID: "second", CaseID: c.ID, Bindings: map[string]project.ScenarioBinding{"value": {StepID: "first", Extract: "id"}}}}}
	b, _ := json.Marshal(s)
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: string(b)})
	enter(m)
	saved, err := project.GetScenario(root, id)
	if err != nil || len(saved.Steps) != 2 {
		t.Fatalf("%+v %v %s", saved, err, m.View().Content)
	}
	scenarioKeys(m, "space", "s", "v")
	if !strings.Contains(m.View().Content, "Scenario valid") {
		t.Fatal(m.View().Content)
	}
	cmd := scenarioTUIStart(t, m)
	m.Update(cmd())
	if len(requests) != 2 || requests[0] != "/?q=step" || requests[1] != "/?q=from-response" || !strings.Contains(m.View().Content, "Successful: true") {
		t.Fatalf("%v %s", requests, m.View().Content)
	}
	key(m, "esc")
	review, _ := project.ReviewAccess(root, "staging")
	project.ApproveAccess(root, review)
	var out, diag bytes.Buffer
	code := cli.Run([]string{"scenario", "run", id, "--project", root, "--env", "staging", "--output", "json"}, strings.NewReader(""), &out, &diag)
	if code != 0 || len(requests) != 4 || requests[2] != requests[0] || requests[3] != requests[1] {
		t.Fatalf("%d %s %v", code, &out, requests)
	}
	scenarioKeys(m, "space", "s", "e")
	s.Name = "Renamed"
	b, _ = json.Marshal(s)
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: string(b)})
	enter(m)
	saved, err = project.GetScenario(root, id)
	if err != nil || saved.Name != "Renamed" {
		t.Fatalf("%+v %v", saved, err)
	}
	scenarioKeys(m, "space", "s", "x", "y")
	if _, err := project.GetScenario(root, id); err == nil {
		t.Fatal("TUI deletion failed")
	}
}
func TestScenarioTUIConfirmsEveryProductionSendAndCancelsRemainingSteps(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); io.WriteString(w, `{}`) }))
	defer server.Close()
	root, c, _ := scenarioTUIWorkspace(t, server.URL, "production")
	s := project.Scenario{ID: uuid.NewString(), Name: "Production", ContinueOnFailure: true, Steps: []project.ScenarioStep{{ID: "one", CaseID: c.ID}, {ID: "two", CaseID: c.ID}}}
	if _, err := project.SaveScenario(root, s, "", false); err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 100})
	for _, k := range []string{"space", "e", "l", "j", "space", "e", "s"} {
		key(m, k)
	}
	cmd := scenarioTUIStart(t, m)
	m.Update(cmd())
	if calls.Load() != 0 || !strings.Contains(m.View().Content, "Confirm production scenario send?") || strings.Contains(m.View().Content, "q=") {
		t.Fatal(m.View().Content)
	}
	_, cmd = m.Update(tea.KeyPressMsg{Text: "y", Code: 'y'})
	if cmd == nil {
		t.Fatal("approval did not resume")
	}
	m.Update(cmd())
	if calls.Load() != 1 || !strings.Contains(m.View().Content, "Confirm production scenario send?") {
		t.Fatalf("calls=%d %s", calls.Load(), m.View().Content)
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("cancel did not await report")
	}
	m.Update(cmd())
	if calls.Load() != 1 || !strings.Contains(m.View().Content, "scenario cancelled") || !strings.Contains(m.View().Content, "two: skipped") {
		t.Fatalf("calls=%d %s", calls.Load(), m.View().Content)
	}
	// Run again and approve both actual sends.
	key(m, "esc")
	cmd = scenarioTUIStart(t, m)
	m.Update(cmd())
	for i := 0; i < 2; i++ {
		_, cmd = m.Update(tea.KeyPressMsg{Text: "y", Code: 'y'})
		m.Update(cmd())
	}
	if calls.Load() != 3 || !strings.Contains(m.View().Content, "Successful: true") {
		t.Fatalf("calls=%d %s", calls.Load(), m.View().Content)
	}
}
func TestScenarioTUICancelsActiveTransport(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); close(started); <-r.Context().Done() }))
	defer server.Close()
	root, c, _ := scenarioTUIWorkspace(t, server.URL, "staging")
	s := project.Scenario{ID: uuid.NewString(), Name: "Wait", Steps: []project.ScenarioStep{{ID: "one", CaseID: c.ID}, {ID: "two", CaseID: c.ID}}}
	project.SaveScenario(root, s, "", false)
	m, _ := tui.New(root, tui.KeyConfig{})
	cmd := scenarioTUIStart(t, m)
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("transport not started")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	select {
	case msg := <-done:
		m.Update(msg)
	case <-time.After(3 * time.Second):
		t.Fatal("transport cancellation blocked")
	}
	if calls.Load() != 1 || !strings.Contains(m.View().Content, "scenario cancelled") {
		t.Fatal(m.View().Content)
	}
}
