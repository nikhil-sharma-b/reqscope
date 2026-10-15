package project_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func refreshDoc(t *testing.T, s project.Specification, change func(map[string]any)) {
	t.Helper()
	var doc map[string]any
	if err := project.Decode(s.Documents[filepath.Base(s.Location)], &doc); err != nil {
		t.Fatal(err)
	}
	change(doc)
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Location, raw, 0600); err != nil {
		t.Fatal(err)
	}
}
func previewRefresh(t *testing.T, root, id string) project.RefreshPreview {
	t.Helper()
	p, err := project.RefreshSpecification(root, id, "", true, project.SourceImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func applyRefresh(t *testing.T, root, id, token string) {
	t.Helper()
	if _, err := project.RefreshSpecification(root, id, token, false, project.SourceImportOptions{}); err != nil {
		t.Fatal(err)
	}
}
func endpointAt(t *testing.T, s project.Specification, path string) project.Endpoint {
	t.Helper()
	for _, ep := range s.Endpoints {
		if ep.Path == path {
			return ep
		}
	}
	t.Fatalf("missing endpoint %s", path)
	return project.Endpoint{}
}
func TestRefreshPreviewsChangesAndPreservesDisconnectedCases(t *testing.T) {
	root, s := authorFixture(t)
	pet := endpointAt(t, s, "/pets")
	original := project.Case{ID: uuid.NewString(), Name: "Saved inputs", Request: project.RequestDraft{EndpointID: pet.ID, Module: s.ModuleID, Method: "GET", URL: "/pets", Body: "user-owned", BodyType: "raw", Query: []project.RequestField{{Name: "search", Value: "retained"}}, Variables: map[string]string{"owner": "human"}}}
	saved, err := project.SaveCase(root, original, "", false)
	if err != nil {
		t.Fatal(err)
	}
	refreshDoc(t, s, func(doc map[string]any) {
		paths := doc["paths"].(map[string]any)
		delete(paths, "/pets")
		paths["/new"] = map[string]any{"get": map[string]any{"responses": map[string]any{"200": map[string]any{"description": "new"}}}}
		paths["/other"].(map[string]any)["get"].(map[string]any)["summary"] = "Backend edit"
	})
	p := previewRefresh(t, root, s.ID)
	if len(p.Added) != 1 || p.Added[0].Path != "/new" || len(p.Changed) != 1 || p.Changed[0].Path != "/other" || len(p.Removed) != 1 || p.Removed[0].ID != pet.ID || len(p.Cases) != 1 || len(p.Diagnostics) == 0 {
		t.Fatalf("bad impact: %+v", p)
	}
	before, _ := project.Specifications(root)
	unchanged, _ := project.GetCase(root, saved.ID)
	if before[0].Revision != s.Revision || unchanged.Revision != saved.Revision {
		t.Fatal("preview wrote resources")
	}
	applyRefresh(t, root, s.ID, p.Token)
	after, err := project.Specifications(root)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].ID != s.ID || after[0].ModuleID != s.ModuleID || len(after[0].DeletedEndpoints) != 1 || after[0].DeletedEndpoints[0].ID != pet.ID {
		t.Fatal("lost identity/tombstone")
	}
	disconnected, err := project.GetCase(root, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	expected := original
	expected.Request.Disconnected = true
	if !reflect.DeepEqual(disconnected.Case, expected) {
		t.Fatalf("lost case inputs: %+v", disconnected)
	}
	if _, err := project.SendCase(context.Background(), root, "local", saved.ID, nil, project.SendOptions{}); err == nil || !strings.Contains(err.Error(), "explicitly relink") {
		t.Fatalf("disconnected case send: %v", err)
	}
	// The same operation returning does not silently reconnect a saved case.
	refreshDoc(t, s, func(map[string]any) {})
	applyRefresh(t, root, s.ID, previewRefresh(t, root, s.ID).Token)
	disconnected, _ = project.GetCase(root, saved.ID)
	if !disconnected.Request.Disconnected {
		t.Fatal("implicitly reconnected returning operation")
	}
	disconnected.Request.Disconnected = false
	disconnected.Request.EndpointID = endpointAt(t, s, "/other").ID
	relinked, err := project.SaveCase(root, disconnected.Case, disconnected.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if relinked.Request.Disconnected || relinked.Request.Body != "user-owned" {
		t.Fatal("explicit relink lost inputs")
	}
	relinked.Request.EndpointID = ""
	relinked.Request.URL = "/independent"
	if _, err := project.SaveCase(root, relinked.Case, relinked.Revision, false); err != nil {
		t.Fatal("independent conversion:", err)
	}
}

func TestRefreshMergesIndependentEditsAndBlocksConflicts(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "independent", true: "conflict"}[conflict], func(t *testing.T) {
			root, s := authorFixture(t)
			edit := contractEdit(s)
			edited, err := project.EditContract(root, "schema", "update", edit, false)
			if err != nil {
				t.Fatal(err)
			}
			refreshDoc(t, s, func(doc map[string]any) {
				if conflict {
					doc["components"].(map[string]any)["schemas"].(map[string]any)["Pet"].(map[string]any)["type"] = "boolean"
				} else {
					doc["paths"].(map[string]any)["/other"].(map[string]any)["get"].(map[string]any)["summary"] = "upstream"
				}
			})
			p := previewRefresh(t, root, s.ID)
			if conflict {
				if len(p.Conflicts) != 1 || !strings.Contains(p.Conflicts[0], "/components/schemas/Pet/type") {
					t.Fatalf("missing conflict: %+v", p)
				}
				if _, err := project.RefreshSpecification(root, s.ID, p.Token, false, project.SourceImportOptions{}); err == nil {
					t.Fatal("applied unresolved conflict")
				}
				current, _ := project.Specifications(root)
				if current[0].Revision != edited.Specification.Revision {
					t.Fatal("conflict overwrote local edits")
				}
				return
			}
			if len(p.Conflicts) != 0 {
				t.Fatal(p.Conflicts)
			}
			applyRefresh(t, root, s.ID, p.Token)
			current, _ := project.Specifications(root)
			if current[0].Schemas[0].Definition.(map[string]any)["type"] != "integer" || endpointAt(t, current[0], "/other").Operation["summary"] != "upstream" {
				t.Fatal("three-way merge lost edits")
			}
			if reflect.DeepEqual(current[0].Base, s.Base) || current[0].Digest == s.Digest {
				t.Fatal("imported base not advanced")
			}
			p = previewRefresh(t, root, s.ID)
			if len(p.Conflicts)+len(p.Added)+len(p.Changed)+len(p.Removed) != 0 {
				t.Fatalf("repeat refresh changed stable merge: %+v", p)
			}
		})
	}
}

func TestRefreshRejectsStalePreviewsAndInvalidExternalFiles(t *testing.T) {
	for _, target := range []string{"incoming", "contract", "case", "environment", "invalid-case", "invalid-source"} {
		t.Run(target, func(t *testing.T) {
			root, s := authorFixture(t)
			saved, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Original", Request: project.RequestDraft{EndpointID: s.Endpoints[0].ID, BodyType: "raw"}}, "", false)
			if err != nil {
				t.Fatal(err)
			}
			p := previewRefresh(t, root, s.ID)
			switch target {
			case "incoming":
				refreshDoc(t, s, func(doc map[string]any) { doc["info"].(map[string]any)["version"] = "2" })
			case "contract":
				if _, err := project.EditContract(root, "schema", "update", contractEdit(s), false); err != nil {
					t.Fatal(err)
				}
			case "case":
				saved.Name = "External"
				if _, err := project.SaveCase(root, saved.Case, saved.Revision, false); err != nil {
					t.Fatal(err)
				}
			case "environment":
				snapshot, _ := project.Load(root)
				env, _ := snapshot.Resolve("local")
				env.Variables["external"] = "edit"
				if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
					t.Fatal(err)
				}
			case "invalid-case":
				if err := os.WriteFile(filepath.Join(root, "cases", saved.ID+".yaml"), []byte("bad: ["), 0600); err != nil {
					t.Fatal(err)
				}
			case "invalid-source":
				if err := os.WriteFile(s.Location, []byte("bad: ["), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := project.Specifications(root)
			if _, err := project.RefreshSpecification(root, s.ID, p.Token, false, project.SourceImportOptions{}); err == nil {
				t.Fatal("accepted stale/invalid state")
			}
			after, _ := project.Specifications(root)
			if before[0].Revision != after[0].Revision {
				t.Fatal("failed refresh wrote source")
			}
		})
	}
}

func TestRefreshPreservesInputsAndReportsIncompatibilities(t *testing.T) {
	root, s := authorFixture(t)
	ep := endpointAt(t, s, "/other")
	saved, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Inputs", Request: project.RequestDraft{EndpointID: ep.ID, Query: []project.RequestField{{Name: "count", Value: "old-string"}}, BodyType: "json", Body: `{"value":"kept"}`}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	refreshDoc(t, s, func(doc map[string]any) {
		op := doc["paths"].(map[string]any)["/other"].(map[string]any)["get"].(map[string]any)
		op["parameters"] = []any{map[string]any{"name": "count", "in": "query", "required": true, "schema": map[string]any{"type": "integer"}}}
		op["requestBody"] = map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "integer"}}}}}}
	})
	p := previewRefresh(t, root, s.ID)
	if len(p.Cases) != 1 || len(p.Diagnostics) != 2 || !strings.Contains(strings.Join(p.Diagnostics, " "), "count") || !strings.Contains(strings.Join(p.Diagnostics, " "), "body.value") {
		t.Fatalf("missing input diagnostics: %+v", p)
	}
	applyRefresh(t, root, s.ID, p.Token)
	current, _ := project.GetCase(root, saved.ID)
	if !reflect.DeepEqual(current.Case, saved.Case) || current.Revision != saved.Revision {
		t.Fatal("refresh modified incompatible saved inputs")
	}
}

func TestRefreshRevokesAffectedApprovalsWithoutChangingBindings(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root, s := authorFixture(t)
	snapshot, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	env, err := snapshot.Resolve("staging")
	if err != nil {
		t.Fatal(err)
	}
	env.BaseURLs[s.ModuleID] = "https://approved.example.test"
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	review, err := project.ReviewAccess(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	refreshDoc(t, s, func(doc map[string]any) {
		doc["servers"] = []any{map[string]any{"url": "https://new.example.test"}}
		doc["security"] = []any{map[string]any{"newBinding": []any{}}}
	})
	p := previewRefresh(t, root, s.ID)
	if _, err := project.CheckCLIAccess(root, "staging", s.ModuleID, "https://approved.example.test"); err != nil {
		t.Fatal("preview revoked approval", err)
	}
	applyRefresh(t, root, s.ID, p.Token)
	if _, err := project.CheckCLIAccess(root, "staging", s.ModuleID, "https://approved.example.test"); err == nil {
		t.Fatal("contract destination/security change retained approval")
	}
	after, err := project.ReviewAccess(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.BaseURLs, review.BaseURLs) || !reflect.DeepEqual(after.Bindings, review.Bindings) {
		t.Fatal("refresh changed protected configuration")
	}
	if _, err := project.CheckCLIAccess(root, "staging", s.ModuleID, "https://new.example.test"); err == nil {
		t.Fatal("refresh granted new destination")
	}
}

func TestRefreshRemoteEmbeddedSourceAndStaleIncoming(t *testing.T) {
	root := remoteRoot(t)
	var description atomic.Value
	description.Store("original")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<script>SwaggerUIBundle({spec:{"openapi":"3.0.3","info":{"title":"Embedded","version":"1"},"paths":{"/pets":{"get":{"responses":{"200":{"description":%q}}}}}}})</script>`, description.Load().(string))
	}))
	defer server.Close()
	if err := project.ApproveSourceOrigin(root, server.URL); err != nil {
		t.Fatal(err)
	}
	discovery, err := project.DiscoverSource(root, server.URL, project.SourceImportOptions{})
	if err != nil || len(discovery.Candidates) != 1 {
		t.Fatalf("discovery: %+v %v", discovery, err)
	}
	s, err := project.ImportSource(root, discovery.Candidates[0], false, "", project.SourceImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	description.Store("changed")
	p := previewRefresh(t, root, s.ID)
	if len(p.Changed) != 1 {
		t.Fatalf("embedded refresh impact: %+v", p)
	}
	description.Store("changed-again")
	if _, err := project.RefreshSpecification(root, s.ID, p.Token, false, project.SourceImportOptions{}); err == nil {
		t.Fatal("accepted stale remote preview")
	}
	applyRefresh(t, root, s.ID, previewRefresh(t, root, s.ID).Token)
	current, err := project.Specifications(root)
	if err != nil {
		t.Fatal(err)
	}
	response := current[0].Endpoints[0].Operation["responses"].(map[string]any)["200"].(map[string]any)
	if response["description"] != "changed-again" || current[0].Endpoints[0].ID != s.Endpoints[0].ID {
		t.Fatal("embedded refresh lost content or identity")
	}
}

func TestRefreshRejectsProtectedCredentialsBeforePersistence(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root, s := authorFixture(t)
	vault := project.NewCredentials(&credentialStore{values: map[string]string{}})
	snapshot, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	env, err := snapshot.Resolve("staging")
	if err != nil {
		t.Fatal(err)
	}
	env.BaseURLs[s.ModuleID] = "https://example.test/"
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	const secret = "refresh-private-credential"
	binding := project.CredentialBinding{Name: "login", Kind: "bearer", Module: s.ModuleID, Destination: "https://example.test/", In: "header", Field: "Authorization"}
	if err := vault.Save(root, "staging", binding, secret, true); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"openapi: 3.0.3\ninfo: {title: Secret, version: '1'}\npaths: {}\n# " + secret + "\n",
		`{"openapi":"3.0.3","info":{"title":"Secret","version":"1"},"paths":{},"x-note":"refresh-private-\u0063redential"}`,
	} {
		if err := os.WriteFile(s.Location, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := project.RefreshSpecification(root, s.ID, "", true, project.SourceImportOptions{Credentials: vault}); err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("credential not safely rejected: %v", err)
		}
		current, err := project.Specifications(root)
		if err != nil {
			t.Fatal(err)
		}
		if current[0].Revision != s.Revision {
			t.Fatal("credential refresh wrote source")
		}
	}
}
