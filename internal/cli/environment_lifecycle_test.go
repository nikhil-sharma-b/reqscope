package cli_test

import (
	"bytes"
	"encoding/json"
	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"strings"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func TestEnvironmentVariablesAndDeletion(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, err := project.Init(root, "Lifecycle", false)
	if err != nil {
		t.Fatal(err)
	}
	env, _ := s.Resolve("staging")
	run := func(want int, body string, args ...string) map[string]any {
		t.Helper()
		code, result := acceptanceCLI(t, root, body, args...)
		if code != want {
			t.Fatalf("%v: %d %+v", args, code, result)
		}
		return result
	}
	body := func(value, revision string) string {
		b, _ := json.Marshal(map[string]string{"name": "host", "value": value, "revision": revision})
		return string(b)
	}
	run(0, body("localhost", env.Revision), "variables", "create", "--env", env.ID, "--input", "-", "--preview")
	run(3, "", "variables", "get", "host", "--env", env.ID)
	run(0, body("localhost", env.Revision), "variables", "create", "--env", env.ID, "--input", "-")
	run(3, body("other", env.Revision), "variables", "update", "--env", env.ID, "--input", "-")
	current, _ := project.Load(root)
	env, _ = current.Resolve(env.ID)
	run(0, "", "variables", "get", "host", "--env", env.ID)
	run(0, "", "variables", "list", "--env", env.ID)
	run(2, body("other", env.Revision), "variables", "create", "--env", env.ID, "--input", "-")
	run(0, body("other", env.Revision), "variables", "update", "--env", env.ID, "--input", "-")
	current, _ = project.Load(root)
	env, _ = current.Resolve(env.ID)
	run(0, `{"revision":"`+env.Revision+`"}`, "variables", "delete", "host", "--env", env.ID, "--input", "-")
	current, _ = project.Load(root)
	env, _ = current.Resolve(env.ID)
	deletion := `{"revision":"` + env.Revision + `"}`
	run(0, deletion, "environment", "delete", env.ID, "--input", "-", "--preview")
	run(0, "", "environment", "get", env.ID)
	run(0, deletion, "environment", "delete", env.ID, "--input", "-")
	run(3, "", "environment", "get", env.ID)
	for _, name := range []string{"environment.delete", "variables.create", "variables.update", "variables.delete"} {
		run(0, "", "input-schema", "get", name)
	}
}

func acceptanceCLI(t *testing.T, root, body string, args ...string) (int, map[string]any) {
	t.Helper()
	var out, diag bytes.Buffer
	code := cli.Run(append(args, "--project", root, "--output", "json"), strings.NewReader(body), &out, &diag)
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid envelope: %v %s %s", err, &out, &diag)
	}
	return code, result
}
