package project_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

const remoteContract = `{"openapi":"3.1.0","info":{"title":"Pets","version":"1"},"paths":{"/pets":{"get":{"responses":{"200":{"description":"OK"}}}}}}`

func remoteRoot(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, err := project.Init(root, "Pets", false); err != nil {
		t.Fatal(err)
	}
	return root
}
func TestRemoteImportPreservesNonCredentialQuery(t *testing.T) {
	root := remoteRoot(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("version") != "v1" {
			t.Error("version query lost")
		}
		fmt.Fprint(w, remoteContract)
	}))
	defer server.Close()
	location := server.URL + "/api.json?version=v1"
	if err := project.ApproveSourceOrigin(root, location); err != nil {
		t.Fatal(err)
	}
	s, err := project.ImportSource(root, project.SourceCandidate{Location: location}, false, "", project.SourceImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Location != location || s.Mode != "url" || len(s.Endpoints) != 1 {
		t.Fatalf("unexpected import: %+v", s)
	}
	specs, err := project.Specifications(root)
	if err != nil || len(specs) != 1 {
		t.Fatalf("%v %v", specs, err)
	}
}

func TestSwaggerDiscoveryKeepsCandidateWhenOptionalScriptFails(t *testing.T) {
	root := remoteRoot(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/docs" {
			fmt.Fprint(w, `<script>SwaggerUIBundle({url:'/api.json'})</script><script src="/analytics.js"></script>`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	if err := project.ApproveSourceOrigin(root, server.URL); err != nil {
		t.Fatal(err)
	}
	discovery, err := project.DiscoverSource(root, server.URL+"/docs", project.SourceImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Candidates) != 1 || discovery.Candidates[0].Location != server.URL+"/api.json" {
		t.Fatalf("%+v", discovery)
	}
}

func TestRemoteSourcesAndBoundedReferences(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			root := remoteRoot(t)
			hits := 0
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; fmt.Fprint(w, `Pet: {type: string}`) }))
			defer other.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/docs/index.html":
					fmt.Fprint(w, `<script src="swagger-ui-bundle.js"></script><script src="assets/initializer.js"></script>`)
				case "/docs/assets/initializer.js":
					fmt.Fprintf(w, `// url: "fake.json"
const ignored = "url: 'fake.json'";
SwaggerUIBundle({urls:[{name:'Pets',url:'../../api.%s'},{name:'Other',url:'../../other.json'}]});`, format)
				case "/api.json":
					fmt.Fprint(w, remoteContract)
				case "/api.yaml":
					fmt.Fprint(w, "openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\npaths: {}\ncomponents: {schemas: {Pet: {$ref: 'models/pet.yaml#/Pet'}}}\n")
				case "/models/pet.yaml":
					fmt.Fprint(w, "Pet: {type: string, x-owner: pets}")
				case "/denied.yaml":
					fmt.Fprintf(w, "openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\npaths: {}\ncomponents: {schemas: {Pet: {$ref: '%s/pet.yaml#/Pet'}}}", other.URL)
				case "/cycle.yaml":
					fmt.Fprint(w, "openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\npaths: {}\ncomponents: {schemas: {Pet: {$ref: 'cycle.yaml#/components/schemas/Pet'}}}")
				case "/missing.yaml":
					fmt.Fprint(w, "openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\npaths: {}\ncomponents: {schemas: {Pet: {$ref: 'absent.yaml#/Pet'}}}")
				case "/redirect":
					http.Redirect(w, r, "/api.json?token=never-print-this", http.StatusFound)
				case "/large":
					fmt.Fprint(w, strings.Repeat("x", 10*1024*1024+1))
				case "/fallback":
					fmt.Fprint(w, `<script>SwaggerUIBundle({url: window.location.href}); globalThis.exec=true;</script>`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			if _, err := project.DiscoverSource(root, server.URL+"/docs/index.html", project.SourceImportOptions{}); err == nil {
				t.Fatal("unapproved discovery allowed")
			}
			if err := project.ApproveSourceOrigin(root, server.URL); err != nil {
				t.Fatal(err)
			}
			discovery, err := project.DiscoverSource(root, server.URL+"/docs/index.html", project.SourceImportOptions{})
			if err != nil || len(discovery.Candidates) != 2 {
				t.Fatalf("%+v %v", discovery, err)
			}
			if discovery.Candidates[0].Name != "Pets" || discovery.Candidates[0].Location != server.URL+"/api."+format {
				t.Fatalf("bad relative discovery: %+v", discovery)
			}
			preview, err := project.ImportSource(root, discovery.Candidates[0], true, "", project.SourceImportOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if format == "yaml" && (len(preview.Documents) != 2 || len(preview.Schemas) != 1) {
				t.Fatalf("reference not retained: %+v", preview)
			}
			if specs, _ := project.Specifications(root); len(specs) != 0 {
				t.Fatal("preview persisted")
			}
			if _, err := project.ImportSource(root, discovery.Candidates[0], false, preview.Digest, project.SourceImportOptions{}); err != nil {
				t.Fatal(err)
			}
			before, _ := project.Load(root)
			for _, path := range []string{"/denied.yaml", "/cycle.yaml", "/missing.yaml", "/redirect", "/large"} {
				_, err := project.ImportSource(root, project.SourceCandidate{Location: server.URL + path}, false, "", project.SourceImportOptions{})
				if err == nil {
					t.Fatalf("%s accepted", path)
				}
				if strings.Contains(err.Error(), "never-print-this") {
					t.Fatal("redirect credential leaked")
				}
				after, _ := project.Load(root)
				if before.Revision != after.Revision {
					t.Fatal("failed import changed project")
				}
			}
			if hits != 0 {
				t.Fatal("denied origin contacted")
			}
			fallback, err := project.DiscoverSource(root, server.URL+"/fallback", project.SourceImportOptions{})
			if err != nil || len(fallback.Candidates) != 0 || !strings.Contains(fallback.Hint, "raw") {
				t.Fatalf("fallback missing: %+v %v", fallback, err)
			}
		})
	}
}

func TestEmbeddedStaticSpecificationRefreshAndConfigResources(t *testing.T) {
	root := remoteRoot(t)
	description := "OK"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/docs" {
			fmt.Fprint(w, `<script>SwaggerUIBundle({configUrl:'config/config.json'})</script>`)
			return
		}
		if r.URL.Path == "/config/config.json" {
			fmt.Fprintf(w, `{"spec":{"openapi":"3.1.0","info":{"title":"Pets","version":"1"},"paths":{"/pets":{"get":{"responses":{"200":{"description":%q}}}}}}}`, description)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	if err := project.ApproveSourceOrigin(root, server.URL); err != nil {
		t.Fatal(err)
	}
	discovery, err := project.DiscoverSource(root, server.URL+"/docs", project.SourceImportOptions{})
	if err != nil || len(discovery.Candidates) != 1 || discovery.Candidates[0].Spec == nil {
		t.Fatalf("%+v %v", discovery, err)
	}
	preview, err := project.ImportSource(root, discovery.Candidates[0], true, "", project.SourceImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	description = "Changed"
	if _, err := project.ImportSource(root, discovery.Candidates[0], false, preview.Digest, project.SourceImportOptions{}); err == nil {
		t.Fatal("changed embedded preview applied")
	}
	if specs, _ := project.Specifications(root); len(specs) != 0 {
		t.Fatal("failed refresh wrote")
	}
}

func TestAuthenticatedSourceImportUsesProtectedBindings(t *testing.T) {
	const secret = "swagger-private-123"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("missing protected credential")
		}
		fmt.Fprint(w, remoteContract)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	vault := project.NewCredentials(&credentialStore{values: map[string]string{}})
	binding := project.CredentialBinding{Name: "swagger", Kind: "bearer", Module: "api", Destination: server.URL + "/api.json", In: "header", Field: "Authorization"}
	if err := vault.Save(root, "staging", binding, secret, false); err != nil {
		t.Fatal(err)
	}
	opts := project.SourceImportOptions{Environment: "staging", Module: "api", Credentials: vault}
	candidate := project.SourceCandidate{Location: binding.Destination}
	if _, err := project.ImportSource(root, candidate, true, "", opts); err == nil {
		t.Fatal("stale environment grant accepted")
	}
	if calls != 0 {
		t.Fatal("unapproved source contacted")
	}
	review, err := project.ReviewAccess(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	s, err := project.ImportSource(root, candidate, false, "", opts)
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bytes), secret) {
		t.Fatal("secret persisted")
	}
	// Explicit references cannot be reused at a different path.
	opts.Headers = []project.RequestField{{Name: "Authorization", Value: "{{secret:swagger}}"}}
	if _, err := project.ImportSource(root, project.SourceCandidate{Location: server.URL + "/other.json"}, true, "", opts); err == nil {
		t.Fatal("protected binding moved")
	}
	if calls != 1 {
		t.Fatalf("unexpected network calls: %d", calls)
	}
}

func TestSecretBearingSourceLocationsAreRejectedBeforeFetching(t *testing.T) {
	const secret = "never-store-swagger-token"
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/docs" {
			fmt.Fprint(w, `<script src="/initializer.js?token=unregistered-private-token"></script>`)
			return
		}
		fmt.Fprint(w, remoteContract)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	vault := project.NewCredentials(&credentialStore{values: map[string]string{}})
	if err := vault.Save(root, "staging", project.CredentialBinding{Name: "swagger", Kind: "bearer", Module: "api", Destination: server.URL + "/api.json", In: "header", Field: "Authorization"}, secret, false); err != nil {
		t.Fatal(err)
	}
	if err := project.ApproveSourceOrigin(root, server.URL); err != nil {
		t.Fatal(err)
	}
	opts := project.SourceImportOptions{Credentials: vault}
	raw := server.URL + "/api.json?version=" + secret
	discovery, err := project.DiscoverSource(root, raw, opts)
	if err == nil {
		t.Fatal("secret-bearing URL accepted")
	}
	b, _ := json.Marshal(discovery)
	if strings.Contains(string(b), secret) || strings.Contains(err.Error(), secret) {
		t.Fatal("secret exposed by discovery")
	}
	if _, err := project.ImportSource(root, project.SourceCandidate{Location: raw}, false, "", opts); err == nil {
		t.Fatal("secret-bearing metadata stored")
	}
	if hits != 0 {
		t.Fatal("secret-bearing URL fetched")
	}
	discovery, err = project.DiscoverSource(root, server.URL+"/docs", opts)
	if err == nil {
		t.Fatal("credential query resource accepted")
	}
	b, _ = json.Marshal(discovery)
	if strings.Contains(string(b), "unregistered-private-token") || strings.Contains(err.Error(), "unregistered-private-token") {
		t.Fatal("rejected resource credential exposed")
	}
}

func TestSeveralEmbeddedSpecificationsRemainSelectable(t *testing.T) {
	root := remoteRoot(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<script>SwaggerUIBundle({spec:{openapi:'3.1.0',info:{title:'First',version:'1'},paths:{}}}); SwaggerUIBundle({spec:{openapi:'3.1.0',info:{title:'Second',version:'1'},paths:{}}});</script>`)
	}))
	defer server.Close()
	if err := project.ApproveSourceOrigin(root, server.URL); err != nil {
		t.Fatal(err)
	}
	discovery, err := project.DiscoverSource(root, server.URL, project.SourceImportOptions{})
	if err != nil || len(discovery.Candidates) != 2 {
		t.Fatalf("%+v %v", discovery, err)
	}
	if discovery.Candidates[0].Name == discovery.Candidates[1].Name {
		t.Fatal("embedded choices are ambiguous")
	}
	s, err := project.ImportSource(root, discovery.Candidates[1], true, "", project.SourceImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Base["info"].(map[string]any)["title"] != "Second" {
		t.Fatal("wrong embedded contract imported")
	}
}
