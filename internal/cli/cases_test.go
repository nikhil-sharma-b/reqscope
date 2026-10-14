package cli_test

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaseCLIStdinFilePreviewAndAmbiguousNames(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, err := project.Init(root, "Cases", false); err != nil {
		t.Fatal(err)
	}
	run := func(body string, args ...string) (int, string) {
		t.Helper()
		var out, diag bytes.Buffer
		args = append(args, "--project", root, "--output", "json")
		code := cli.Run(args, strings.NewReader(body), &out, &diag)
		return code, out.String()
	}
	c := project.Case{ID: uuid.NewString(), Name: "Shared", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/"}}
	b, _ := json.Marshal(c)
	if code, out := run(string(b), "case", "create", "--input", "-", "--preview"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	items, _ := project.Cases(root)
	if len(items) != 0 {
		t.Fatal("preview wrote case")
	}
	path := filepath.Join(t.TempDir(), "case.json")
	os.WriteFile(path, b, 0600)
	if code, out := run("", "case", "create", "--input", path); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if code, out := run(`{"name":"Shared"}`, "case", "duplicate", c.ID, "--input", "-"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if code, out := run("", "case", "get", "Shared"); code != 3 || !strings.Contains(out, "ambiguous_resource") {
		t.Fatalf("%d %s", code, out)
	}
	saved, _ := project.GetCase(root, c.ID)
	update := map[string]any{"id": c.ID, "name": "Changed", "request": c.Request, "revision": saved.Revision}
	b, _ = json.Marshal(update)
	if code, out := run(string(b), "case", "update", "--input", "-"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if code, out := run(string(b), "case", "update", "--input", "-"); code != 3 || !strings.Contains(out, "revision_conflict") {
		t.Fatalf("%d %s", code, out)
	}
	current, _ := project.GetCase(root, c.ID)
	if code, out := run(`{"revision":"`+current.Revision+`"}`, "case", "delete", c.ID, "--input", "-"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	for _, name := range []string{"case.create", "case.update", "case.delete", "case.duplicate", "case.run"} {
		if code, out := run("", "input-schema", "get", name); code != 0 {
			t.Fatalf("%s %d %s", name, code, out)
		}
	}
}

func TestSavedRunRejectsNonStringVariables(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, err := project.Init(root, "Cases", false); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"input-schema", "validate", "case.run"}, {"request", "send", "--env", "staging", "--case", "any"}} {
		var out, diag bytes.Buffer
		args = append(args, "--project", root, "--output", "json", "--input", "-")
		if code := cli.Run(args, strings.NewReader("variables: {count: 123}"), &out, &diag); code != 2 || !strings.Contains(out.String(), "invalid_input") {
			t.Fatalf("%d %s %s", code, &out, &diag)
		}
	}
}

func TestCLIAuthorsAndRejectsResponseAssertions(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, err := project.Init(root, "Assertions", false); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		validation string
		code       int
	}{
		{`{"assertions":[{"kind":"status-equals","value":200},{"kind":"header-present","name":"x-test"},{"kind":"json-equals","pointer":"/nil","value":null}],"responseSchema":true}`, 0},
		{`{"assertions":[{"kind":"status-range","min":300,"max":200}]}`, 2},
		{`{"assertions":[{"kind":"json-present","pointer":"/~2"}]}`, 2},
		{`{"assertions":[{"kind":"header-equals","name":"x-test","value":12}]}`, 2},
		{`{"assertions":[{"kind":"unknown"}]}`, 2},
		{`{"responseSchema":"true"}`, 2},
		{`{"assertions":null}`, 2},
		{`{"assertions":[{"kind":"json-present","pointer":null}]}`, 2},
		{`{"assertions":[{"kind":"status-equals","value":200,"min":0}]}`, 2},
	} {
		body := `{"id":"` + uuid.NewString() + `","name":"Checks","request":{"module":"api","method":"GET","url":"/"},"validation":` + tt.validation + `}`
		var out, diag bytes.Buffer
		code := cli.Run([]string{"case", "create", "--input", "-", "--project", root, "--output", "json"}, strings.NewReader(body), &out, &diag)
		if code != tt.code {
			t.Fatalf("%s: %d %s", tt.validation, code, &out)
		}
	}
}
