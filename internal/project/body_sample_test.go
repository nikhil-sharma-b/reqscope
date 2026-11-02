package project_test

import (
	"reflect"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func bodySuggestion(t *testing.T, ep project.Endpoint) any {
	t.Helper()
	_, fields := project.EndpointDraft(ep)
	for _, f := range fields {
		if f.In == "body" && f.Name == "body" {
			return f.Suggestion
		}
	}
	t.Fatalf("no body field in %+v", fields)
	return nil
}

func TestBodySuggestionFromSchema(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"name"},
		"properties": map[string]any{
			"id":      map[string]any{"type": "integer", "readOnly": true},
			"name":    map[string]any{"type": "string", "example": "Rex"},
			"age":     map[string]any{"type": "integer", "default": 3},
			"kind":    map[string]any{"type": "string", "enum": []any{"dog", "cat"}},
			"born":    map[string]any{"type": "string", "format": "date"},
			"tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"active":  map[string]any{"type": "boolean"},
			"weight":  map[string]any{"type": "number"},
			"owner":   map[string]any{"allOf": []any{map[string]any{"properties": map[string]any{"a": map[string]any{"type": "string"}}}, map[string]any{"properties": map[string]any{"b": map[string]any{"type": "integer"}}}}},
			"contact": map[string]any{"oneOf": []any{map[string]any{"type": "string", "format": "email"}, map[string]any{"type": "integer"}}},
		},
	}
	ep := project.Endpoint{Operation: map[string]any{"requestBody": map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": schema}}}}}
	want := map[string]any{
		"name": "Rex", "age": 3, "kind": "dog", "born": "2026-01-01",
		"tags": []any{"string"}, "active": false, "weight": 0.0,
		"owner":   map[string]any{"a": "string", "b": 0},
		"contact": "user@example.com",
	}
	if got := bodySuggestion(t, ep); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestBodySuggestionPrefersExamples(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}}
	media := map[string]any{"schema": schema, "examples": map[string]any{"first": map[string]any{"value": map[string]any{"x": "given"}}}}
	ep := project.Endpoint{Operation: map[string]any{"requestBody": map[string]any{"content": map[string]any{"application/json": media}}}}
	if got := bodySuggestion(t, ep); !reflect.DeepEqual(got, map[string]any{"x": "given"}) {
		t.Fatalf("%#v", got)
	}
	media["example"] = "literal"
	if got := bodySuggestion(t, ep); got != "literal" {
		t.Fatalf("%#v", got)
	}
}

func TestBodySuggestionSwagger2BodyParameter(t *testing.T) {
	param := map[string]any{"name": "pet", "in": "body", "required": true, "schema": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}}}
	ep := project.Endpoint{Operation: map[string]any{"parameters": []any{param}}}
	if got := bodySuggestion(t, ep); !reflect.DeepEqual(got, map[string]any{"name": "string"}) {
		t.Fatalf("%#v", got)
	}
}

func TestBodySampleStopsAtDepth(t *testing.T) {
	node := map[string]any{"type": "object", "properties": map[string]any{}}
	root := node
	for i := 0; i < 20; i++ {
		next := map[string]any{"type": "object", "properties": map[string]any{}}
		node["properties"].(map[string]any)["child"] = next
		node = next
	}
	if project.SchemaSample(root) == nil {
		t.Fatal("expected sample")
	}
}
