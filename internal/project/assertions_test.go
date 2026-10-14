package project_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func TestResponseAssertionsThroughSavedExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("X-Result", "ok")
		w.WriteHeader(422)
		io.WriteString(w, `{"nil":null,"flag":true,"n":9007199254740993,"a/b":{"~key":["yes"]}}`)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	for _, tt := range []struct {
		name, assertion, outcome string
		cap                      int64
	}{
		{"status", `{"kind":"status-equals","value":422}`, "passed", 0},
		{"range", `{"kind":"status-range","min":400,"max":499}`, "passed", 0},
		{"header presence", `{"kind":"header-present","name":"x-result"}`, "passed", 0},
		{"header equality", `{"kind":"header-equals","name":"X-RESULT","value":"ok"}`, "passed", 0},
		{"missing header", `{"kind":"header-present","name":"absent"}`, "failed", 0},
		{"null present", `{"kind":"json-present","pointer":"/nil"}`, "passed", 0},
		{"null equals", `{"kind":"json-equals","pointer":"/nil","value":null}`, "passed", 0},
		{"missing differs from null", `{"kind":"json-equals","pointer":"/missing","value":null}`, "failed", 0},
		{"boolean differs from string", `{"kind":"json-equals","pointer":"/flag","value":"true"}`, "failed", 0},
		{"large number", `{"kind":"json-equals","pointer":"/n","value":9007199254740993}`, "passed", 0},
		{"distinct large number", `{"kind":"json-equals","pointer":"/n","value":9007199254740992}`, "failed", 0},
		{"escaped pointer", `{"kind":"json-equals","pointer":"/a~1b/~0key/0","value":"yes"}`, "passed", 0},
		{"truncated", `{"kind":"json-present","pointer":"/nil"}`, "incomplete", 12},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var settings map[string]any
			if err := project.Decode([]byte(`{"assertions":[`+tt.assertion+`]}`), &settings); err != nil {
				t.Fatal(err)
			}
			c := project.Case{ID: uuid.NewString(), Name: tt.name, Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/", MaxBodyBytes: tt.cap}, Validation: settings}
			if _, err := project.SaveCase(root, c, "", false); err != nil {
				t.Fatal(err)
			}
			result, err := project.SendCase(context.Background(), root, "staging", c.ID, nil, project.SendOptions{})
			if len(result.Checks) != 1 || result.Checks[0].Outcome != tt.outcome || result.Transport != "completed" || result.Status != 422 {
				t.Fatalf("%+v %v", result, err)
			}
			if tt.outcome == "passed" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var e *project.Error
				if !errors.As(err, &e) || e.Code != "check_failed" {
					t.Fatalf("%v", err)
				}
			}
		})
	}
}

func TestResponseSchemaThroughExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"value":null}`)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	for _, tt := range []struct{ name, schema, outcome string }{
		{"valid", `{"type":"object","required":["value"],"properties":{"value":{"type":"null"}}}`, "passed"},
		{"mismatch", `{"type":"object","properties":{"value":{"type":"string"}}}`, "failed"},
		{"unsupported", `{"type":"object","properties":{"absent":{"type":"string","pattern":"abc"}}}`, "incomplete"},
		{"nullable enum", `{"type":"object","properties":{"value":{"type":"string","nullable":true,"enum":["ok"]}}}`, "failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "api.json")
			doc := `{"openapi":"3.0.3","info":{"title":"API","version":"1"},"paths":{"/":{"get":{"responses":{"200":{"description":"OK","content":{"application/json":{"schema":` + tt.schema + `}}}}}}}}`
			if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
				t.Fatal(err)
			}
			spec, err := project.ImportSpecification(root, path, false)
			if err != nil {
				t.Fatal(err)
			}
			s, _ := project.Load(root)
			e, _ := s.Resolve("staging")
			e.BaseURLs[spec.ModuleID] = server.URL
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
			c := project.Case{ID: uuid.NewString(), Name: tt.name, Request: project.RequestDraft{EndpointID: spec.Endpoints[0].ID}, Validation: map[string]any{"responseSchema": true}}
			if _, err := project.SaveCase(root, c, "", false); err != nil {
				t.Fatal(err)
			}
			result, err := project.SendCase(context.Background(), root, "staging", c.ID, nil, project.SendOptions{})
			if len(result.Checks) != 1 || result.Checks[0].Outcome != tt.outcome {
				b, _ := json.Marshal(result)
				t.Fatalf("%s %v", b, err)
			}
		})
	}
}

func TestAssertionFailuresProtectCredentials(t *testing.T) {
	const secret = "private-assertion-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Echo", secret)
		io.WriteString(w, `{"echo":"`+secret+`"}`)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	vault := project.NewCredentials(&credentialStore{values: map[string]string{}})
	if err := vault.Save(root, "staging", project.CredentialBinding{Name: "login", Kind: "bearer", Module: "api", Destination: server.URL + "/", In: "header", Field: "Authorization"}, secret, false); err != nil {
		t.Fatal(err)
	}
	review, _ := project.ReviewAccess(root, "staging")
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	project.Decode([]byte(`{"assertions":[{"kind":"json-equals","pointer":"/echo","value":"`+secret+`"},{"kind":"header-equals","name":"x-echo","value":"wrong"}]}`), &settings)
	c := project.Case{ID: uuid.NewString(), Name: "Secrets", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/", Headers: []project.RequestField{{Name: "Authorization", Value: "{{secret:login}}"}}}, Validation: settings}
	if _, err := project.SaveCase(root, c, "", false); err != nil {
		t.Fatal(err)
	}
	result, err := project.SendCase(context.Background(), root, "staging", c.ID, nil, project.SendOptions{Credentials: vault})
	b, _ := json.Marshal(result)
	if err == nil || strings.Contains(string(b)+err.Error(), secret) || len(result.Checks) != 2 || result.Checks[0].Outcome != "passed" || result.Checks[1].Outcome != "failed" {
		t.Fatalf("unexpected redaction or outcomes: %s %v", b, err)
	}
}

func TestMandatoryBodyChecksNeverPassUnavailableBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/binary" {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		if r.URL.Path == "/invalid" {
			io.WriteString(w, "not json")
			return
		}
		io.WriteString(w, `{"value":true}`)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	path := filepath.Join(t.TempDir(), "api.json")
	doc := `{"swagger":"2.0","info":{"title":"API","version":"1"},"paths":{"/":{"get":{"responses":{"default":{"description":"OK","schema":{"type":"object"}}}}}}}`
	if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	spec, err := project.ImportSpecification(root, path, false)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := project.Load(root)
	e, _ := s.Resolve("staging")
	e.BaseURLs[spec.ModuleID] = server.URL
	if err := project.SaveEnvironment(root, e.Environment, e.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	review, _ := project.ReviewAccess(root, "staging")
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		path, outcome string
		cap           int64
	}{
		{"/", "passed", 0}, {"/", "incomplete", 5}, {"/binary", "incomplete", 0}, {"/invalid", "failed", 0},
	} {
		c := project.Case{ID: uuid.NewString(), Name: "Body checks", Request: project.RequestDraft{EndpointID: spec.Endpoints[0].ID, URL: tt.path, MaxBodyBytes: tt.cap}, Validation: map[string]any{"assertions": []any{map[string]any{"kind": "json-present", "pointer": "/value"}}, "responseSchema": true}}
		if _, err := project.SaveCase(root, c, "", false); err != nil {
			t.Fatal(err)
		}
		result, err := project.SendCase(context.Background(), root, "staging", c.ID, nil, project.SendOptions{})
		if len(result.Checks) != 2 || result.Checks[0].Outcome != tt.outcome || result.Checks[1].Outcome != tt.outcome {
			t.Fatalf("%+v %v", result, err)
		}
	}
}
