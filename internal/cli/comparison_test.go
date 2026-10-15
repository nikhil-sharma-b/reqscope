package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func TestCLIComparisonAndIgnoreAuthoring(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, e := project.Init(root, "Comparison", false); e != nil {
		t.Fatal(e)
	}
	run := func(body string, args ...string) (int, string) {
		t.Helper()
		var out, diag bytes.Buffer
		code := cli.Run(append(args, "--project", root, "--output", "json"), strings.NewReader(body), &out, &diag)
		return code, out.String()
	}
	c := project.Case{ID: uuid.NewString(), Name: "compare", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/"}, ComparisonIgnore: project.IgnoreRules{JSONPointers: []string{"/incidental"}, Headers: []string{"Date"}}}
	raw, _ := json.Marshal(c)
	if code, out := run(string(raw), "case", "create", "--input", "-"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	saved, e := project.GetCase(root, c.ID)
	if e != nil || len(saved.ComparisonIgnore.JSONPointers) != 1 {
		t.Fatalf("%+v %v", saved, e)
	}
	a, _ := project.SendCase(context.Background(), root, "local", c.ID, nil, project.SendOptions{})
	b, _ := project.SendCase(context.Background(), root, "staging", c.ID, nil, project.SendOptions{})
	code, out := run("", "compare", a.RunID, b.RunID)
	if code != 0 || !strings.Contains(out, `"bodyFormat":"unavailable"`) || !strings.Contains(out, `"jsonPointers":["/incidental"]`) || !strings.Contains(out, `"area":"context"`) {
		t.Fatalf("%d %s", code, out)
	}
	if code, out = run(`{}`, "compare", a.RunID, b.RunID, "--input", "-"); code != 0 || strings.Contains(out, "incidental") {
		t.Fatalf("%d %s", code, out)
	}
	for _, body := range []string{`{"jsonPointers":["bad"]}`, `{"headers":["bad header"]}`, `{"unknown":true}`, `{"headers":"Date"}`} {
		if code, out = run(body, "compare", a.RunID, b.RunID, "--input", "-"); code != 2 {
			t.Fatalf("%d %s", code, out)
		}
	}
	if code, out = run("", "compare", a.RunID, "missing"); code != 3 || !strings.Contains(out, "missing or evicted") {
		t.Fatalf("%d %s", code, out)
	}
	if code, out = run("", "compare", a.RunID, b.RunID, "--preview"); code != 2 {
		t.Fatalf("%d %s", code, out)
	}
	if code, out = run("", "input-schema", "get", "comparison.ignore"); code != 0 || !strings.Contains(out, "jsonPointers") {
		t.Fatalf("%d %s", code, out)
	}
}
