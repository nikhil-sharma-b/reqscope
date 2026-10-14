package project_test

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCaseReopenDuplicateAndRevisionConflict(t *testing.T) {
	root := requestWorkspace(t, "http://localhost:12345")
	c := project.Case{ID: uuid.NewString(), Name: "Example", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/items", Variables: map[string]string{"item": "original"}, Headers: []project.RequestField{{Name: "Authorization", Value: "{{secret:token}}"}}}}
	saved, err := project.SaveCase(root, c, "", false)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := project.GetCase(root, c.ID)
	if err != nil || reopened.Request.Variables["item"] != "original" || reopened.Request.Headers[0].Value != "{{secret:token}}" {
		t.Fatalf("%+v %v", reopened, err)
	}
	copy, err := project.DuplicateCase(root, c.ID, "Copy", false)
	if err != nil || copy.ID == c.ID || copy.Request.Headers[0].Value != "{{secret:token}}" {
		t.Fatalf("%+v %v", copy, err)
	}
	c.Name = "Edited"
	if _, err = project.SaveCase(root, c, saved.Revision, false); err != nil {
		t.Fatal(err)
	}
	if _, err = project.SaveCase(root, c, saved.Revision, false); err == nil {
		t.Fatal("stale save accepted")
	}
	if _, err = project.DeleteCase(root, c.ID, saved.Revision, false); err == nil {
		t.Fatal("stale deletion accepted")
	}
	current, _ := project.GetCase(root, c.ID)
	if _, err = project.DeleteCase(root, c.ID, current.Revision, true); err != nil {
		t.Fatal(err)
	}
	if _, err = project.GetCase(root, c.ID); err != nil {
		t.Fatal("preview deleted case")
	}
	if _, err = project.DeleteCase(root, c.ID, current.Revision, false); err != nil {
		t.Fatal(err)
	}
	if _, err = project.GetCase(root, c.ID); err == nil {
		t.Fatal("deleted case still present")
	}
}

func TestCaseRunResolutionAndDraftParity(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; io.WriteString(w, r.URL.Query().Get("item")) }))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	s, _ := project.Load(root)
	s.Manifest.Variables = map[string]string{"item": "project", "default": "default"}
	b, _ := yaml.Marshal(s.Manifest)
	if err := project.Apply(root, []project.Change{{Path: "reqscope.yaml", ExpectedRevision: s.Revision, Content: b}}, false); err != nil {
		t.Fatal(err)
	}
	e, _ := s.Resolve("staging")
	e.Variables["item"] = "environment"
	if err := project.SaveEnvironment(root, e.Environment, e.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	d := project.RequestDraft{Module: "api", Method: "GET", URL: "/", Query: []project.RequestField{{Name: "item", Value: "{{item}}-{{default}}"}}, Variables: map[string]string{"item": "case"}}
	saved, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Run", Request: d}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := project.SendCase(context.Background(), root, "staging", saved.ID, map[string]string{"item": "override"}, project.SendOptions{})
	if err != nil || result.Body != "override-default" || result.CaseID != saved.ID || result.Draft {
		t.Fatalf("%+v %v", result, err)
	}
	draft, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{Variables: map[string]string{"item": "override"}})
	if err != nil || draft.Body != result.Body || !draft.Draft {
		t.Fatalf("%+v %v", draft, err)
	}
	current, _ := project.GetCase(root, saved.ID)
	if current.Request.Variables["item"] != "case" || d.Variables["item"] != "case" {
		t.Fatal("run changed saved or caller inputs")
	}
	_, err = project.SendCase(context.Background(), root, "staging", saved.ID, map[string]string{"item": "{{missing}}"}, project.SendOptions{})
	if err == nil || calls != 2 {
		t.Fatalf("undefined variable sent: %v calls=%d", err, calls)
	}
}

func TestCasesKeepEndpointSourceIdentityAndRejectDanglingDeletion(t *testing.T) {
	root := requestWorkspace(t, "http://localhost:12345")
	file := filepath.Join(t.TempDir(), "api.yaml")
	if err := os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: Cases, version: '1'}\npaths:\n  /items:\n    get:\n      responses: {'200': {description: OK}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec, err := project.ImportSpecification(root, file, false)
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := project.ContractDraft(root, spec.Endpoints[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	d.Variables = map[string]string{"value": "original"}
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Endpoint case", Request: d}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := project.DuplicateCase(root, c.ID, "Copy", false)
	if err != nil || copy.Request.EndpointID != spec.Endpoints[0].ID || copy.Request.Module != spec.ModuleID {
		t.Fatalf("%+v %v", copy, err)
	}
	scenarioPath := filepath.Join(root, "scenarios", uuid.NewString()+".yaml")
	if err := os.WriteFile(scenarioPath, []byte("steps:\n  - caseId: "+c.ID+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := project.DeleteCase(root, c.ID, c.Revision, false)
	if err == nil || len(report.References) != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	if _, err := project.DeleteCase(root, copy.ID, copy.Revision, false); err != nil {
		t.Fatal("unrelated scenario prevented deletion", err)
	}
	if err := project.Apply(root, []project.Change{{Path: "cases/" + c.ID + ".yaml", ExpectedRevision: c.Revision, Delete: true}}, false); err == nil {
		t.Fatal("batch deletion bypassed references")
	}
}

func TestConcurrentCaseWritersOnlyOneSucceeds(t *testing.T) {
	root := requestWorkspace(t, "http://localhost:12345")
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Initial", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/"}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, name := range []string{"First", "Second"} {
		go func(name string) {
			next := c.Case
			next.Name = name
			_, err := project.SaveCase(root, next, c.Revision, false)
			results <- err
		}(name)
	}
	successes := 0
	conflicts := 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else {
			var e *project.Error
			if errors.As(err, &e) && e.Code == "revision_conflict" {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}
