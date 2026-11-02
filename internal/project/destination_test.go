package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func TestDraftDestinationKeepsMissingPathPlaceholders(t *testing.T) {
	root := requestWorkspace(t, "https://api.example.com/v1")
	file := filepath.Join(t.TempDir(), "api.yaml")
	contract := `openapi: 3.0.3
info: {title: Items, version: '1'}
paths:
  /items/{id}/tags/{tag}:
    post:
      parameters:
        - {name: id, in: path, required: true, schema: {type: integer}}
        - {name: tag, in: path, required: true, schema: {type: string}}
        - {name: q, in: query, required: true, schema: {type: string}}
      requestBody: {required: true, content: {application/json: {schema: {type: object}}}}
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
	e.BaseURLs[spec.ModuleID] = "https://api.example.com/v1"
	project.SaveEnvironment(root, e.Environment, e.Revision, false, false)

	d, _ := project.EndpointDraft(spec.Endpoints[0])
	full, path, err := project.DraftDestination(root, "staging", d, project.DefaultCredentials)
	if err != nil {
		t.Fatal(err)
	}
	if full != "https://api.example.com/v1/items/{id}/tags/{tag}" || path != "/v1/items/{id}/tags/{tag}" {
		t.Fatalf("%q %q", full, path)
	}

	d.Path["tag"] = "a b"
	d.Query = []project.RequestField{{Name: "q", Value: "x"}}
	full, path, err = project.DraftDestination(root, "staging", d, project.DefaultCredentials)
	if err != nil {
		t.Fatal(err)
	}
	if full != "https://api.example.com/v1/items/{id}/tags/a%20b?q=x" || path != "/v1/items/{id}/tags/a%20b" {
		t.Fatalf("%q %q", full, path)
	}
}
