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

func TestContractAuthoringCommands(t *testing.T) {
	root := t.TempDir()
	project.Init(root, "Contracts", false)
	file := filepath.Join(t.TempDir(), "api.json")
	raw, _ := json.Marshal(contract("3.0.3"))
	os.WriteFile(file, raw, 0600)
	s, err := project.ImportSpecification(root, file, false)
	if err != nil {
		t.Fatal(err)
	}
	edit := project.ContractEdit{SourceID: s.ID, Revision: s.Revision, ImportedEdit: true, Name: "NewSchema", Value: map[string]any{"type": "string", "x-owner": "team"}}
	run := func(args ...string) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(edit)
		var out, diag bytes.Buffer
		code := cli.Run(append([]string{"--project", root, "--output", "json"}, args...), strings.NewReader(string(b)), &out, &diag)
		var env map[string]any
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Fatal(err, out.String())
		}
		return code, env
	}
	for _, name := range []string{"endpoint.create", "endpoint.update", "endpoint.delete", "schema.create", "schema.update", "schema.delete"} {
		if code, env := run("input-schema", "get", name); code != 0 {
			t.Fatal(env)
		}
	}
	if code, env := run("input-schema", "validate", "schema.create", "--input", "-"); code != 0 {
		t.Fatal(env)
	}
	if code, env := run("schema", "create", "--input", "-", "--preview"); code != 0 {
		t.Fatal(env)
	}
	specs, _ := project.Specifications(root)
	if specs[0].Revision != s.Revision {
		t.Fatal("CLI preview wrote")
	}
	if code, env := run("schema", "create", "--input", "-"); code != 0 {
		t.Fatal(env)
	}
	if code, _ := run("schema", "update", "--input", "-"); code == 0 {
		t.Fatal("CLI accepted stale revision")
	}
	if code, env := run("spec", "export", s.ID); code != 0 {
		t.Fatal(env)
	} else {
		data := env["data"].(map[string]any)
		docs := data["documents"].(map[string]any)
		if !strings.Contains(docs["api.json"].(string), "NewSchema") {
			t.Fatal(env)
		}
	}
	edit.ImportedEdit = false
	if code, _ := run("input-schema", "validate", "schema.update", "--input", "-"); code == 0 {
		t.Fatal("implicit imported edit allowed")
	}
}
