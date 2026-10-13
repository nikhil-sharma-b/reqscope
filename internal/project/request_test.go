package project_test

import (
	"context"
	"errors"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func requestWorkspace(t *testing.T, destination string) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, err := project.Init(root, "Requests", false)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = destination
	if err = project.SaveEnvironment(root, e.Environment, e.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	r, err := project.ReviewAccess(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err = project.ApproveAccess(root, r); err != nil {
		t.Fatal(err)
	}
	return root
}
func TestSendActualBytesAndHTTPErrorIsCompleted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.RequestURI() != "/v1/items?q=a&q=b" || string(b) != `{"name":"edited"}` || len(r.Header.Values("X-Item")) != 2 {
			t.Errorf("unexpected request %s %s %q %v", r.Method, r.URL.RequestURI(), b, r.Header)
		}
		w.WriteHeader(422)
		io.WriteString(w, "invalid item")
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL+"/v1")
	d := project.RequestDraft{Module: "api", Method: "POST", URL: "/items", Query: []project.RequestField{{Name: "q", Value: "a"}, {Name: "q", Value: "b"}}, Headers: []project.RequestField{{Name: "X-Item", Value: "one"}, {Name: "X-Item", Value: "two"}}, Body: `{"name":"edited"}`, BodyType: "json"}
	result, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{})
	if err != nil || result.Status != 422 || result.Body != "invalid item" {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestRedirectPolicyRejectsBeforeContactAndHonorsLimits(t *testing.T) {
	var deniedCalls int
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { deniedCalls++ }))
	defer denied.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/outside" {
			http.Redirect(w, r, denied.URL, 302)
		} else if r.URL.Path == "/loop" {
			http.Redirect(w, r, "/loop", 302)
		} else if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/done", 302)
		} else {
			io.WriteString(w, "done")
		}
	}))
	defer source.Close()
	root := requestWorkspace(t, source.URL)
	for _, interactive := range []bool{false, true} {
		_, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: "/outside"}, project.SendOptions{Interactive: interactive})
		if err == nil || !strings.Contains(err.Error(), "origin") || deniedCalls != 0 {
			t.Fatalf("%v denied calls %d", err, deniedCalls)
		}
	}
	zero := 0
	_, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: "/redirect", MaxRedirects: &zero}, project.SendOptions{})
	if err == nil {
		t.Fatal("zero redirect limit ignored")
	}
	result, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: "/redirect"}, project.SendOptions{})
	if err != nil || result.Body != "done" {
		t.Fatalf("%+v %v", result, err)
	}
	_, err = project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: "/loop"}, project.SendOptions{})
	if err == nil {
		t.Fatal("default redirect limit ignored")
	}
}
func TestLimitsCancellationAndBinary(t *testing.T) {
	started := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wait":
			started <- struct{}{}
			<-r.Context().Done()
		case "/binary":
			w.Write([]byte{0, 255, 1})
		default:
			io.WriteString(w, "abcdefgh")
		}
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	draft := project.RequestDraft{Module: "api", Method: "GET", URL: "/", MaxBodyBytes: 4}
	result, err := project.Send(context.Background(), root, "staging", draft, project.SendOptions{})
	if err != nil || result.Body != "abcd" || !result.Truncated || result.RetainedBytes != 4 {
		t.Fatalf("%+v %v", result, err)
	}
	draft.URL = "/binary"
	result, err = project.Send(context.Background(), root, "staging", draft, project.SendOptions{})
	if err != nil || !result.Binary || result.Body != "" {
		t.Fatalf("%+v %v", result, err)
	}
	draft.URL = "/wait"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := project.Send(ctx, root, "staging", draft, project.SendOptions{}); done <- err }()
	<-started
	cancel()
	err = <-done
	var pe *project.Error
	if !errors.As(err, &pe) || pe.Code != "cancelled" {
		t.Fatalf("%v", err)
	}
	draft.TimeoutMS = 20
	_, err = project.Send(context.Background(), root, "staging", draft, project.SendOptions{})
	if !errors.As(err, &pe) || pe.Code != "transport_error" {
		t.Fatalf("%v", err)
	}
}
func TestTLSAndPreflightFailures(t *testing.T) {
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted TLS contacted") }))
	defer tls.Close()
	root := requestWorkspace(t, tls.URL)
	_, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: "/"}, project.SendOptions{})
	if err == nil {
		t.Fatal("TLS verification disabled")
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	root = requestWorkspace(t, server.URL)
	for _, d := range []project.RequestDraft{
		{Module: "api", Method: "POST", URL: "/", BodyType: "json", Body: "{"},
		{Module: "api", Method: "GET", URL: "/{{missing}}"},
		{Module: "api", Method: "GET", URL: "/", Headers: []project.RequestField{{Name: "X-Test", Value: "{{missing}}"}}},
		{Module: "api", Method: "POST", URL: "/", Body: "{{missing}}"},
		{Module: "api", Method: "GET", URL: "/{id}"},
		{Module: "api", Method: "GET", URL: "/", Headers: []project.RequestField{{Name: "Authorization", Value: "Bearer ordinary"}}},
		{Module: "api", Method: "GET", URL: "/", Headers: []project.RequestField{{Name: "X-Test", Value: "evil\r\nHeader: value"}}},
	} {
		_, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{})
		if err == nil {
			t.Fatalf("accepted %+v", d)
		}
	}
	if calls != 0 {
		t.Fatalf("preflight failures made %d calls", calls)
	}
}
func TestContractInputsAndSuggestions(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = r.URL.RequestURI(); io.WriteString(w, "ok") }))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	file := filepath.Join(t.TempDir(), "api.yaml")
	contract := `openapi: 3.0.3
info: {title: Items, version: '1'}
paths:
  /items/{id}:
    get:
      parameters:
        - {name: id, in: path, required: true, description: Item identifier, schema: {type: integer}, example: 7}
        - {name: tags, in: query, required: true, style: form, explode: false, schema: {type: array, items: {type: string}}}
      responses: {'200': {description: OK}}
`
	if err := os.WriteFile(file, []byte(contract), 0600); err != nil {
		t.Fatal(err)
	}
	spec, err := project.ImportSpecification(root, file, false)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := project.Load(root)
	e, _ := s.Resolve("staging")
	e.BaseURLs[spec.ModuleID] = server.URL
	project.SaveEnvironment(root, e.Environment, e.Revision, false, false)
	r, _ := project.ReviewAccess(root, "staging")
	project.ApproveAccess(root, r)
	d, fields, err := project.ContractDraft(root, spec.Endpoints[0].ID)
	if err != nil || len(fields) != 2 || !fields[0].Required || fields[0].Description != "Item identifier" || fields[0].Suggestion == nil || len(d.Path) != 0 {
		t.Fatalf("%+v %+v %v", d, fields, err)
	}
	if _, err = project.Send(context.Background(), root, "staging", d, project.SendOptions{}); err == nil {
		t.Fatal("missing required inputs accepted")
	}
	d.Path["id"] = "edited"
	d.Query = []project.RequestField{{Name: "tags", Value: "red"}, {Name: "tags", Value: "blue"}}
	if _, err = project.Send(context.Background(), root, "staging", d, project.SendOptions{}); err == nil {
		t.Fatal("invalid integer accepted")
	}
	d.Path["id"] = "9"
	if _, err = project.Send(context.Background(), root, "staging", d, project.SendOptions{}); err != nil {
		t.Fatal(err)
	}
	if received != "/items/9?tags=red%2Cblue" {
		t.Fatal(received)
	}
}
