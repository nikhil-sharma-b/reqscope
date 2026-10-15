package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func TestRefreshCLIRequiresReviewedTokenAndPreservesSourceIdentity(t *testing.T) {
	root := t.TempDir()
	if _, err := project.Init(root, "Refresh", false); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "api.json")
	doc := contract("3.0.3")
	write := func() {
		t.Helper()
		b, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	s, err := project.ImportSpecification(root, file, false)
	if err != nil {
		t.Fatal(err)
	}
	run := func(body string, args ...string) (int, map[string]any) {
		t.Helper()
		var out, diag bytes.Buffer
		code := cli.Run(append([]string{"--project", root, "--output", "json"}, args...), strings.NewReader(body), &out, &diag)
		var envelope map[string]any
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatal(err, out.String(), diag.String())
		}
		return code, envelope
	}
	if code, e := run("", "input-schema", "get", "spec.refresh-apply"); code != 0 {
		t.Fatal(e)
	}
	if code, _ := run(`{"previewToken":"invalid"}`, "input-schema", "validate", "spec.refresh-apply", "--input", "-"); code == 0 {
		t.Fatal("invalid token passed schema")
	}
	doc["paths"].(map[string]any)["/new"] = map[string]any{"get": map[string]any{"responses": map[string]any{"200": map[string]any{"description": "new"}}}}
	write()
	code, e := run("", "spec", "refresh-preview", s.ID)
	if code != 0 {
		t.Fatal(e)
	}
	data := e["data"].(map[string]any)
	if len(data["added"].([]any)) != 1 {
		t.Fatal(e)
	}
	token := data["previewToken"].(string)
	current, _ := project.Specifications(root)
	if len(current) != 1 || current[0].Revision != s.Revision {
		t.Fatal("CLI preview wrote")
	}
	if code, _ := run(`{}`, "spec", "refresh-apply", s.ID, "--input", "-"); code == 0 {
		t.Fatal("CLI applied without token")
	}
	body := `{"previewToken":"` + token + `"}`
	if code, e := run(body, "spec", "refresh-apply", s.ID, "--input", "-"); code != 0 {
		t.Fatal(e)
	}
	current, _ = project.Specifications(root)
	if len(current) != 1 || current[0].ID != s.ID || current[0].ModuleID != s.ModuleID || len(current[0].Endpoints) != 3 {
		t.Fatal("CLI refresh lost source identity")
	}
	if code, _ := run(body, "spec", "refresh-apply", s.ID, "--input", "-"); code == 0 {
		t.Fatal("CLI replayed stale preview")
	}
}
