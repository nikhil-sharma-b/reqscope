package tui_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHistoryReopenAndPinActions(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, err := project.Init(root, "History", false); err != nil {
		t.Fatal(err)
	}
	ex, _ := project.Send(context.Background(), root, "local", project.RequestDraft{}, project.SendOptions{})
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"g", "h", "space", "h", "p"} {
		key(m, k)
	}
	r, err := project.GetRun(root, ex.RunID)
	if err != nil || !r.Pinned {
		t.Fatalf("%+v %v", r, err)
	}
	if !strings.Contains(m.View().Content, "failed") {
		t.Fatal(m.View().Content)
	}
	enter(m)
	if !strings.Contains(m.View().Content, "failed") {
		t.Fatal(m.View().Content)
	}
}

func TestScenarioReportReopensFromHistory(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	defer server.Close()
	root := t.TempDir()
	s, err := project.Init(root, "Scenario history", false)
	if err != nil {
		t.Fatal(err)
	}
	env, _ := s.Resolve("staging")
	env.BaseURLs["api"] = server.URL
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	review, _ := project.ReviewAccess(root, env.ID)
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Step", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/"}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := project.SaveScenario(root, project.Scenario{ID: uuid.NewString(), Name: "Workflow", Steps: []project.ScenarioStep{{ID: "one", CaseID: c.ID}}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := project.RunScenario(context.Background(), root, env.ID, scenario.ID, project.ScenarioOptions{}); err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"g", "h", "enter"} {
		key(m, k)
	}
	if !strings.Contains(m.View().Content, "Scenario report") || !strings.Contains(m.View().Content, "one: passed") {
		t.Fatal(m.View().Content)
	}
}
