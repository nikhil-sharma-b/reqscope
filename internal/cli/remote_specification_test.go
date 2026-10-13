package cli_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func TestCLIURLImportApprovalSelectionAndFallback(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if code, e := specCommand(t, root, "project", "init", "Pets"); code != 0 {
		t.Fatal(e)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/docs":
			fmt.Fprint(w, `<script>SwaggerUIBundle({urls:[{name:'Pets',url:'api.json'},{name:'Other',url:'other.json'}]})</script>`)
		case "/api.json":
			fmt.Fprint(w, `{"openapi":"3.1.0","info":{"title":"Pets","version":"1"},"paths":{"/pets":{"get":{"responses":{"200":{"description":"OK"}}}}}}`)
		case "/fallback":
			fmt.Fprint(w, `<script>SwaggerUIBundle({url: computedURL()})</script>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	if code, e := specCommand(t, root, "spec", "import", "--url", server.URL+"/docs", "--preview"); code != 4 {
		t.Fatalf("unapproved: %d %+v", code, e)
	}
	if calls != 0 {
		t.Fatal("unapproved preview fetched")
	}
	if err := project.ApproveSourceOrigin(root, server.URL); err != nil {
		t.Fatal(err)
	}
	if code, e := specCommand(t, root, "spec", "import", "--url", server.URL+"/docs"); code != 2 || len(e["data"].(map[string]any)["candidates"].([]any)) != 2 {
		t.Fatalf("selection: %d %+v", code, e)
	}
	if code, e := specCommand(t, root, "spec", "import", "--url", server.URL+"/docs", "--select", "Pets", "--preview"); code != 0 {
		t.Fatal(e)
	}
	if specs, _ := project.Specifications(root); len(specs) != 0 {
		t.Fatal("preview wrote")
	}
	if code, e := specCommand(t, root, "spec", "import", "--url", server.URL+"/docs", "--select", "Pets"); code != 0 {
		t.Fatal(e)
	}
	if code, e := specCommand(t, root, "spec", "import", "--url", server.URL+"/fallback"); code != 2 || e["data"].(map[string]any)["hint"] == "" {
		t.Fatalf("fallback: %d %+v", code, e)
	}
	if code, e := specCommand(t, root, "spec", "import", "--url", server.URL+"/api.json"); code != 0 {
		t.Fatal(e)
	}
	before := calls
	if code, e := specCommand(t, root, "project", "validate"); code != 0 {
		t.Fatal(e)
	}
	if code, e := specCommand(t, root, "spec", "inspect"); code != 0 {
		t.Fatal(e)
	}
	if calls != before {
		t.Fatal("inspection/validation fetched")
	}
	if specs, _ := project.Specifications(root); len(specs) != 2 {
		t.Fatalf("import count %d", len(specs))
	}
}
