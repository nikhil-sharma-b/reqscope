package tui_test

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaseTUISaveReopenDuplicateAndDirtyLeave(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, err := project.Init(root, "Cases", false); err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	keys := func(seq ...string) {
		for _, k := range seq {
			key(m, k)
		}
	}
	keys("space", "r", "n")
	d := project.RequestDraft{Module: "api", Method: "GET", URL: "/items", Variables: map[string]string{"name": "kept"}}
	b, _ := json.Marshal(d)
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: string(b)})
	enter(m)
	keys("space", "c", "s")
	enter(m)
	cases, err := project.Cases(root)
	if err != nil || len(cases) != 1 {
		t.Fatalf("%v %v %s", cases, err, m.View().Content)
	}
	keys("space", "c", "l", "space", "c", "d")
	cases, _ = project.Cases(root)
	if len(cases) != 2 {
		t.Fatal("duplicate failed", m.View().Content)
	}
	keys("space", "c", "o", "space", "r", "e")
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	d.URL = "/edited"
	b, _ = json.Marshal(d)
	m.Update(tea.KeyPressMsg{Text: string(b)})
	enter(m)
	keys("space", "c", "l")
	if !strings.Contains(m.View().Content, "s save | d discard | c cancel") {
		t.Fatal(m.View().Content)
	}
	keys("c")
	if !strings.Contains(m.View().Content, "/edited") {
		t.Fatal("cancel lost input", m.View().Content)
	}
	keys("space", "c", "l", "s")
	current, _ := project.GetCase(root, cases[0].ID)
	if current.Request.URL != "/edited" || current.Request.Variables["name"] != "kept" {
		t.Fatalf("%+v", current)
	}
	keys("space", "c", "o", "space", "r", "e")
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	d.URL = "/discarded"
	b, _ = json.Marshal(d)
	m.Update(tea.KeyPressMsg{Text: string(b)})
	enter(m)
	keys("q")
	if !strings.Contains(m.View().Content, "Unsaved request/case") {
		t.Fatal("quit skipped confirmation", m.View().Content)
	}
	keys("c", "space", "c", "l", "d")
	current, _ = project.GetCase(root, cases[0].ID)
	if current.Request.URL != "/edited" {
		t.Fatal("discard saved changes")
	}
}

func TestSavedCaseRunsIdenticallyThroughTUIAndCLI(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; io.WriteString(w, r.URL.Query().Get("value")) }))
	defer server.Close()
	root := t.TempDir()
	s, err := project.Init(root, "Cases", false)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = server.URL
	if err := project.SaveEnvironment(root, e.Environment, e.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	review, err := project.ReviewAccess(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	if err := project.SelectEnvironment(root, e.ID); err != nil {
		t.Fatal(err)
	}
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Runner", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/", Query: []project.RequestField{{Name: "value", Value: "{{value}}"}}, Variables: map[string]string{"value": "case input"}}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	var out, diag bytes.Buffer
	if code := cli.Run([]string{"request", "send", "--project", root, "--env", "staging", "--case", c.ID, "--output", "json"}, strings.NewReader(""), &out, &diag); code != 0 {
		t.Fatalf("%d %s %s", code, &out, &diag)
	}
	if !strings.Contains(out.String(), `"body":"case input"`) || !strings.Contains(out.String(), `"draft":false`) {
		t.Fatal(out.String())
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"space", "c", "l", "space", "c"} {
		key(m, k)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("case run did not schedule send", m.View().Content)
	}
	m.Update(cmd())
	if calls != 2 || !strings.Contains(m.View().Content, "case input") || strings.Contains(m.View().Content, "draft inputs") {
		t.Fatalf("calls=%d %s", calls, m.View().Content)
	}
}

func TestComposeAnotherEndpointStartsIndependentDraft(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, err := project.Init(root, "Cases", false); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "api.yaml")
	contract := "openapi: 3.0.3\ninfo: {title: API, version: '1'}\npaths:\n  /first:\n    get:\n      responses: {'200': {description: OK}}\n  /second:\n    get:\n      responses: {'200': {description: OK}}\n"
	if err := os.WriteFile(file, []byte(contract), 0600); err != nil {
		t.Fatal(err)
	}
	spec, err := project.ImportSpecification(root, file, false)
	if err != nil {
		t.Fatal(err)
	}
	firstID := ""
	secondID := ""
	for _, ep := range spec.Endpoints {
		if ep.Path == "/first" {
			firstID = ep.ID
		} else {
			secondID = ep.ID
		}
	}
	d, _, err := project.ContractDraft(root, firstID)
	if err != nil {
		t.Fatal(err)
	}
	source, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Original", Request: d}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"space", "c", "l", "space", "c", "o", "g", "e", "j", "space", "r", "c"} {
		key(m, k)
	}
	if !strings.Contains(m.View().Content, secondID) {
		t.Fatal("compose retained previous endpoint", m.View().Content)
	}
	enter(m)
	for _, k := range []string{"space", "c", "s"} {
		key(m, k)
	}
	enter(m)
	items, err := project.Cases(root)
	if err != nil || len(items) != 2 {
		t.Fatalf("%+v %v %s", items, err, m.View().Content)
	}
	current, _ := project.GetCase(root, source.ID)
	if current.Request.EndpointID != firstID || current.Revision != source.Revision {
		t.Fatal("compose changed saved case")
	}
}
