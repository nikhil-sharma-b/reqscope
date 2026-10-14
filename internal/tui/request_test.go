package tui_test

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"encoding/json"
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

func TestProductionSendConfirmationAndCancellation(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; io.WriteString(w, "searchable response") }))
	defer server.Close()
	root := t.TempDir()
	s, _ := project.Init(root, "Demo", false)
	e, _ := s.Resolve("production")
	e.BaseURLs["api"] = server.URL
	project.SaveEnvironment(root, e.Environment, e.Revision, false, false)
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	// Select production through the public environment keyboard boundary.
	for _, k := range []string{"space", "e", "l", "j", "space", "e", "s", "space", "r", "n"} {
		key(m, k)
	}
	d := project.RequestDraft{Module: "api", Method: "GET", URL: "/?token=ordinary", BodyType: "raw"}
	b, _ := json.Marshal(d)
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: string(b)})
	enter(m)
	for _, k := range []string{"space", "r", "s"} {
		key(m, k)
	}
	if !strings.Contains(m.View().Content, "Confirm production") || strings.Contains(m.View().Content, "token=") {
		t.Fatal(m.View().Content)
	}
	key(m, "esc")
	if calls != 0 {
		t.Fatal("cancel contacted server")
	}
	for _, k := range []string{"space", "r", "s"} {
		key(m, k)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Text: "y", Code: 'y'})
	if cmd == nil {
		t.Fatal("confirmation did not schedule send")
	}
	m.Update(cmd())
	if calls != 1 || !strings.Contains(m.View().Content, "searchable response") {
		t.Fatalf("calls %d %s", calls, m.View().Content)
	}
}

func TestTUIAndCLIConstructIdenticalRequests(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received <- r.Method + " " + r.URL.RequestURI() + " " + strings.Join(r.Header.Values("X-Item"), ",") + " " + string(b)
		io.WriteString(w, "one\ntwo")
	}))
	defer server.Close()
	root := t.TempDir()
	s, _ := project.Init(root, "Parity", false)
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = server.URL + "/v1"
	project.SaveEnvironment(root, e.Environment, e.Revision, false, false)
	review, _ := project.ReviewAccess(root, "staging")
	project.ApproveAccess(root, review)
	project.SelectEnvironment(root, "staging")
	d := project.RequestDraft{Module: "api", Method: "POST", URL: "/items", BodyType: "json", Body: `{"name":"edited"}`, Query: []project.RequestField{{Name: "q", Value: "one"}, {Name: "q", Value: "two"}}, Headers: []project.RequestField{{Name: "X-Item", Value: "a"}, {Name: "X-Item", Value: "b"}}}
	b, _ := json.Marshal(d)
	var out, diag bytes.Buffer
	if code := cli.Run([]string{"request", "send", "--env", "staging", "--project", root, "--input", "-", "--output", "json"}, strings.NewReader(string(b)), &out, &diag); code != 0 {
		t.Fatalf("%d %s", code, &diag)
	}
	m, _ := tui.New(root, tui.KeyConfig{})
	for _, k := range []string{"space", "r", "n"} {
		key(m, k)
	}
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: string(b)})
	enter(m)
	for _, k := range []string{"space", "r"} {
		key(m, k)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if cmd == nil {
		t.Fatal("send not scheduled")
	}
	m.Update(cmd())
	first, second := <-received, <-received
	if first != second || first != `POST /v1/items?q=one&q=two a,b {"name":"edited"}` {
		t.Fatalf("%q != %q", first, second)
	}
	key(m, "/")
	m.Update(tea.KeyPressMsg{Text: "two"})
	enter(m)
	if strings.Contains(m.View().Content, "one") || !strings.Contains(m.View().Content, "two") {
		t.Fatal(m.View().Content)
	}
	for _, k := range []string{"space", "r"} {
		key(m, k)
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("copy command missing")
	}
}
func TestTUIRunningRequestCanBeCancelled(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	root := t.TempDir()
	s, _ := project.Init(root, "Cancel", false)
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = server.URL
	project.SaveEnvironment(root, e.Environment, e.Revision, false, false)
	project.SelectEnvironment(root, "staging")
	m, _ := tui.New(root, tui.KeyConfig{})
	for _, k := range []string{"space", "r", "n"} {
		key(m, k)
	}
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: `{"module":"api","method":"GET","url":"/"}`})
	enter(m)
	for _, k := range []string{"space", "r"} {
		key(m, k)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	if cmd == nil {
		t.Fatal("send missing")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	<-started
	key(m, "esc")
	m.Update(<-done)
	if !strings.Contains(m.View().Content, "request cancelled") {
		t.Fatal(m.View().Content)
	}
}

func TestTUISelectsUploadReferenceAndCLIParity(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received <- r.Header.Get("Content-Type") + "\n" + string(b)
	}))
	defer server.Close()
	root := t.TempDir()
	s, _ := project.Init(root, "Uploads", false)
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = server.URL
	project.SaveEnvironment(root, e.Environment, e.Revision, false, false)
	review, _ := project.ReviewAccess(root, "staging")
	project.ApproveAccess(root, review)
	project.SelectEnvironment(root, "staging")
	path := filepath.Join(root, "a-upload.bin")
	os.WriteFile(path, []byte{0, 255, 1}, 0600)
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	d := project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: "multipart", Form: []project.FormField{{Name: "tag", Value: "one"}, {Name: "tag", Value: "two"}}}
	b, _ := json.Marshal(d)
	for _, k := range []string{"space", "r", "n"} {
		key(m, k)
	}
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: string(b)})
	enter(m)
	for _, k := range []string{"space", "r", "f"} {
		key(m, k)
	}
	m.Update(tea.KeyPressMsg{Text: "file"})
	enter(m)
	if !strings.Contains(m.View().Content, "a-upload.bin") {
		t.Fatal(m.View().Content)
	}
	key(m, "j")
	enter(m)
	for _, k := range []string{"space", "r"} {
		key(m, k)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if cmd == nil {
		t.Fatal(m.View().Content)
	}
	m.Update(cmd())
	d.Form = append(d.Form, project.FormField{Name: "file", File: path})
	b, _ = json.Marshal(d)
	var out, diag bytes.Buffer
	if code := cli.Run([]string{"request", "send", "--project", root, "--env", "staging", "--input", "-", "--output", "json"}, bytes.NewReader(b), &out, &diag); code != 0 {
		t.Fatalf("%d %s %s", code, &out, &diag)
	}
	if a, b := <-received, <-received; a != b {
		t.Fatalf("TUI/CLI bytes differ %q %q", a, b)
	}
}
