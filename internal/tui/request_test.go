package tui_test

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

// workspace creates a project whose named environment points at url and is
// selected, with CLI access approved when approve is set.
func workspace(t *testing.T, url, envName string, approve bool) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, err := project.Init(root, "Workspace", false)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := s.Resolve(envName)
	e.BaseURLs["api"] = url
	if err := project.SaveEnvironment(root, e.Environment, e.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	if approve {
		review, _ := project.ReviewAccess(root, envName)
		if err := project.ApproveAccess(root, review); err != nil {
			t.Fatal(err)
		}
	}
	if err := project.SelectEnvironment(root, e.ID); err != nil {
		t.Fatal(err)
	}
	return root
}

func saveCase(t *testing.T, root, name string, d project.RequestDraft) project.CaseResource {
	t.Helper()
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: name, Request: d}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestProductionSendAsksFirstAndHidesTheDraft(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; io.WriteString(w, "searchable response") }))
	defer server.Close()
	root := workspace(t, server.URL, "staging", false)
	s, _ := project.Load(root)
	prod, _ := s.Resolve("production")
	prod.BaseURLs["api"] = server.URL
	project.SaveEnvironment(root, prod.Environment, prod.Revision, false, false)
	m := newModel(t, root)
	// Switch through the environment picker: production is listed last.
	press(m, "e")
	moveTo(t, m, "production")
	press(m, "enter")
	wantScreen(t, m, "Requests now go to production")
	press(m, "n")
	typeText(m, "/?token=ordinary")
	press(m, "enter", "s")
	wantScreen(t, m, "Send to production?", "GET  "+server.URL)
	rejectScreen(t, m, "token=")
	press(m, "esc")
	wantScreen(t, m, "Not sent")
	if calls != 0 {
		t.Fatal("cancelling contacted the server")
	}
	press(m, "s", "y")
	if calls != 1 {
		t.Fatalf("confirmed send made %d calls", calls)
	}
	wantScreen(t, m, "200 OK", "searchable response")
}

func TestSendMatchesTheCLIByteForByte(t *testing.T) {
	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received <- r.Method + " " + r.URL.RequestURI() + " " + strings.Join(r.Header.Values("X-Item"), ",") + " " + string(b)
		io.WriteString(w, "one\ntwo")
	}))
	defer server.Close()
	root := workspace(t, server.URL+"/v1", "staging", true)
	d := project.RequestDraft{Module: "api", Method: "POST", URL: "/items", BodyType: "json", Body: `{"name":"edited"}`, Query: []project.RequestField{{Name: "q", Value: "one"}, {Name: "q", Value: "two"}}, Headers: []project.RequestField{{Name: "X-Item", Value: "a"}, {Name: "X-Item", Value: "b"}}}
	b, _ := json.Marshal(d)
	var out, diag bytes.Buffer
	if code := cli.Run([]string{"request", "send", "--env", "staging", "--project", root, "--input", "-", "--output", "json"}, bytes.NewReader(b), &out, &diag); code != 0 {
		t.Fatalf("%d %s", code, &diag)
	}
	saveCase(t, root, "Parity", d)
	// A narrow terminal shows one pane; the response takes over after a send.
	m := newModel(t, root, 90, 30)
	press(m, "2", "enter", "s")
	if first, second := <-received, <-received; first != second || first != `POST /v1/items?q=one&q=two a,b {"name":"edited"}` {
		t.Fatalf("%q != %q", first, second)
	}
	if !hasLine(m, "one") || !hasLine(m, "two") {
		t.Fatalf("response body missing:\n%s", screen(m))
	}
	press(m, "/")
	typeText(m, "two")
	press(m, "enter")
	if hasLine(m, "one") || !hasLine(m, "two") {
		t.Fatalf("find did not narrow the body:\n%s", screen(m))
	}
	if cmd := send(m, "y"); cmd == nil {
		t.Fatal("copy did not reach the clipboard")
	}
}

func TestRunningRequestCanBeCancelled(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	root := workspace(t, server.URL, "staging", false)
	saveCase(t, root, "Slow", project.RequestDraft{Module: "api", Method: "GET", URL: "/"})
	m := newModel(t, root)
	press(m, "2", "enter")
	prepare := send(m, "s")
	_, transport := m.Update(prepare())
	done := make(chan any, 1)
	go func() { done <- transport() }()
	<-started
	wantScreen(t, m, "Sending GET request", "esc cancel")
	press(m, "esc")
	m.Update(<-done)
	wantScreen(t, m, "request cancelled")
}

func TestUploadReferenceMatchesTheCLI(t *testing.T) {
	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received <- r.Header.Get("Content-Type") + "\n" + string(b)
	}))
	defer server.Close()
	root := workspace(t, server.URL, "staging", true)
	path := filepath.Join(root, "a-upload.bin")
	os.WriteFile(path, []byte{0, 255, 1}, 0600)
	d := project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: "multipart", Form: []project.FormField{{Name: "tag", Value: "one"}, {Name: "tag", Value: "two"}}}
	saveCase(t, root, "Upload", d)
	m := newModel(t, root)
	press(m, "2", "enter", "f")
	wantScreen(t, m, "Attach a file", "Which form field")
	press(m, "enter")
	wantScreen(t, m, "Choose a file", "a-upload.bin")
	moveTo(t, m, "a-upload.bin")
	press(m, "enter")
	wantScreen(t, m, "Attached a-upload.bin", "file    @a-upload.bin")
	press(m, "s")
	d.Form = append(d.Form, project.FormField{Name: "file", File: path})
	b, _ := json.Marshal(d)
	var out, diag bytes.Buffer
	if code := cli.Run([]string{"request", "send", "--project", root, "--env", "staging", "--input", "-", "--output", "json"}, bytes.NewReader(b), &out, &diag); code != 0 {
		t.Fatalf("%d %s %s", code, &out, &diag)
	}
	if a, b := <-received, <-received; a != b {
		t.Fatalf("TUI and CLI sent different bytes:\n%q\n%q", a, b)
	}
}

func TestFormBuildsTheRequestAndSavesIt(t *testing.T) {
	root := workspace(t, "https://api.example.test/v1", "staging", false)
	m := newModel(t, root)
	press(m, "n")
	wantScreen(t, m, "New request", "/path or https://")
	typeText(m, "/items/{id}")
	press(m, "enter")
	wantScreen(t, m, "Path", "id", "required", "https://api.example.test/v1/items/{id}")
	press(m, "j", "j")
	if !selected(m, "required") {
		t.Fatalf("path input not selected:\n%s", screen(m))
	}
	press(m, "enter")
	typeText(m, "42")
	press(m, "enter", "a")
	typeText(m, "q=one")
	press(m, "enter", "a")
	typeText(m, "q=two")
	press(m, "enter")
	wantScreen(t, m, "https://api.example.test/v1/items/42")
	rejectScreen(t, m, "q=one")
	press(m, "m", "m")
	wantScreen(t, m, "PUT")
	press(m, "ctrl+s")
	wantScreen(t, m, "Save request")
	press(m, "ctrl+u")
	typeText(m, "Items")
	press(m, "enter")
	wantScreen(t, m, "Saved “Items”")
	cases, _ := project.Cases(root)
	if len(cases) != 1 {
		t.Fatalf("%d saved requests", len(cases))
	}
	r := cases[0].Request
	if r.Method != "PUT" || r.URL != "/items/{id}" || r.Path["id"] != "42" || r.Module != "api" || len(r.Query) != 2 || r.Query[1].Value != "two" {
		t.Fatalf("%+v", r)
	}
}

func TestSavedRequestEditsAreProtectedUntilSaved(t *testing.T) {
	root := workspace(t, "https://api.example.test", "staging", false)
	c := saveCase(t, root, "Items", project.RequestDraft{Module: "api", Method: "GET", URL: "/items", Variables: map[string]string{"name": "kept"}})
	m := newModel(t, root)
	press(m, "2", "enter", "enter", "ctrl+u")
	typeText(m, "/edited")
	press(m, "enter")
	wantScreen(t, m, "Items  edited")
	// Drafts outlive leaving the screen.
	press(m, "esc")
	wantScreen(t, m, "edited")
	press(m, "q")
	wantScreen(t, m, "Quit without saving?", "Items")
	press(m, "esc", "enter")
	wantScreen(t, m, "/edited")
	press(m, "ctrl+s")
	current, _ := project.GetCase(root, c.ID)
	if current.Request.URL != "/edited" || current.Request.Variables["name"] != "kept" {
		t.Fatalf("%+v", current.Request)
	}
	press(m, "esc", "c")
	if cases, _ := project.Cases(root); len(cases) != 2 {
		t.Fatalf("duplicate made %d", len(cases))
	}
	press(m, "g", "enter", "enter", "ctrl+u")
	typeText(m, "/discarded")
	press(m, "enter")
	if cmd := send(m, "q"); cmd != nil {
		t.Fatal("quit skipped the unsaved-edits question")
	}
	if cmd := send(m, "y"); cmd == nil {
		t.Fatal("confirmed quit did not exit")
	}
	if current, _ := project.GetCase(root, c.ID); current.Request.URL != "/edited" {
		t.Fatal("quitting saved discarded edits")
	}
}

func TestSavedRequestRunsLikeTheCLIAndReportsChecks(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; io.WriteString(w, r.URL.Query().Get("value")) }))
	defer server.Close()
	root := workspace(t, server.URL, "staging", true)
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Runner",
		Validation: map[string]any{"assertions": []any{map[string]any{"kind": "status-equals", "value": 201}}},
		Request:    project.RequestDraft{Module: "api", Method: "GET", URL: "/", Query: []project.RequestField{{Name: "value", Value: "{{value}}"}}, Variables: map[string]string{"value": "case input"}}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	var out, diag bytes.Buffer
	if code := cli.Run([]string{"request", "send", "--project", root, "--env", "staging", "--case", c.ID, "--output", "json"}, strings.NewReader(""), &out, &diag); code != 6 || !strings.Contains(out.String(), `"body":"case input"`) {
		t.Fatalf("%d %s %s", code, &out, &diag)
	}
	m := newModel(t, root)
	press(m, "2", "r")
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
	wantScreen(t, m, "case input", "status-equals failed", "1 check failed", "Sent as saved")
}

func TestOpeningAnotherEndpointKeepsEachDraft(t *testing.T) {
	root := workspace(t, "https://api.example.test", "staging", false)
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: API, version: '1'}\npaths:\n  /first:\n    get:\n      summary: First thing\n      parameters: [{name: limit, in: query, schema: {type: integer, example: 5}}]\n      responses: {'200': {description: OK}}\n  /second:\n    get:\n      responses: {'200': {description: OK}}\n"), 0600)
	if _, err := project.ImportSpecification(root, file, false); err != nil {
		t.Fatal(err)
	}
	m := newModel(t, root)
	press(m, "enter")
	wantScreen(t, m, "GET /first", "limit", "e.g. 5")
	moveTo(t, m, "limit")
	press(m, "enter")
	// The suggestion is offered as the starting value.
	wantScreen(t, m, "5")
	press(m, "ctrl+u")
	typeText(m, "9")
	press(m, "enter", "esc", "j", "enter")
	wantScreen(t, m, "GET /second")
	rejectScreen(t, m, "limit")
	press(m, "esc", "k", "enter")
	wantScreen(t, m, "GET /first", "limit")
	if !strings.Contains(screen(m), "9") {
		t.Fatalf("the first endpoint's edits were lost:\n%s", screen(m))
	}
	press(m, "ctrl+s", "enter")
	cases, _ := project.Cases(root)
	if len(cases) != 1 || cases[0].Name != "First thing" || cases[0].Request.Query[0].Value != "9" {
		t.Fatalf("%+v", cases)
	}
}
