package tui_test

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
)

func TestTUISourceApprovalSelectionAndCancellation(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, err := project.Init(root, "Pets", false); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/docs" {
			fmt.Fprint(w, `<script>SwaggerUIBundle({urls:[{name:'First',url:'first.json'},{name:'Second',url:'second.json'}]})</script>`)
			return
		}
		if r.URL.Path == "/second.json" {
			fmt.Fprint(w, `{"openapi":"3.1.0","info":{"title":"Pets","version":"1"},"paths":{"/second":{"get":{"responses":{"200":{"description":"OK"}}}}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	importKeys(m, server.URL+"/docs")
	if calls != 0 || !strings.Contains(m.View().Content, "Approve specification source origin") {
		t.Fatalf("contact before approval: %d %s", calls, m.View().Content)
	}
	key(m, "esc")
	key(m, "esc")
	importKeys(m, server.URL+"/docs")
	enter(m)
	if !strings.Contains(m.View().Content, "Choose a specification") {
		t.Fatal(m.View().Content)
	}
	key(m, "j")
	enter(m)
	if !strings.Contains(m.View().Content, "Import preview") {
		t.Fatal(m.View().Content)
	}
	if specs, _ := project.Specifications(root); len(specs) != 0 {
		t.Fatal("preview wrote")
	}
	enter(m)
	if !strings.Contains(ansi.Strip(m.View().Content), "/second") {
		t.Fatal(m.View().Content)
	}
	if specs, _ := project.Specifications(root); len(specs) != 1 {
		t.Fatalf("wrong import count: %d", len(specs))
	}
}

func TestTUIInvalidDiscoveredSourceOffersRawURLFallback(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, err := project.Init(root, "Pets", false); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/docs" {
			fmt.Fprint(w, `<script>SwaggerUIBundle({url:'invalid.json'})</script>`)
			return
		}
		fmt.Fprint(w, "<html>Authentication required</html>")
	}))
	defer server.Close()
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 45})
	importKeys(m, server.URL+"/docs")
	enter(m)
	view := m.View().Content
	if !strings.Contains(view, "raw JSON/YAML URL") || !strings.Contains(view, "invalid.json") {
		t.Fatal(view)
	}
	if specs, _ := project.Specifications(root); len(specs) != 0 {
		t.Fatal("failed import wrote")
	}
}

func TestTUIAuthenticatedImportUsesActiveEnvironmentCredential(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, err := project.Init(root, "Pets", false)
	if err != nil {
		t.Fatal(err)
	}
	const secret = "tui-swagger-secret"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("missing active environment credential")
		}
		fmt.Fprint(w, `{"openapi":"3.1.0","info":{"title":"Pets","version":"1"},"paths":{}}`)
	}))
	defer server.Close()
	env, err := s.Resolve("staging")
	if err != nil {
		t.Fatal(err)
	}
	env.BaseURLs["api"] = server.URL
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	if err := project.SelectEnvironment(root, env.ID); err != nil {
		t.Fatal(err)
	}
	vault := project.NewCredentials(nil)
	if err := vault.Save(root, env.ID, project.CredentialBinding{Name: "swagger", Kind: "bearer", Module: "api", Destination: server.URL + "/api.json", In: "header", Field: "Authorization"}, secret, true); err != nil {
		t.Fatal(err)
	}
	m, err := tui.NewWithCredentials(root, tui.KeyConfig{}, vault)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	importKeys(m, server.URL+"/api.json")
	if calls != 0 {
		t.Fatal("contact before approval")
	}
	enter(m)
	if !strings.Contains(m.View().Content, "Import preview") {
		t.Fatal(m.View().Content)
	}
	if strings.Contains(m.View().Content, secret) {
		t.Fatal("credential leaked")
	}
	enter(m)
	specs, err := project.Specifications(root)
	if err != nil || len(specs) != 1 {
		t.Fatalf("%+v %v", specs, err)
	}
	if _, err := project.CheckCLIAccess(root, env.ID, "api", server.URL); err == nil {
		t.Fatal("source approval conferred request grant")
	}
}
