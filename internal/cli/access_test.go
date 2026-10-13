package cli_test

import (
	"bytes"
	"encoding/json"
	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"strings"
	"testing"
)

func TestCLIRequiresEnvironmentAndCannotLowerProduction(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, err := project.Init(root, "Demo", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"environment", "check-access", "--module", "api"}, {"environment", "check-access", "--env", "staging", "--module", "api"}} {
		var out, diag bytes.Buffer
		args = append(args, "--project", root, "--output", "json")
		if code := cli.Run(args, strings.NewReader(""), &out, &diag); code != 4 || !strings.Contains(out.String(), `"code":"access_denied"`) {
			t.Fatalf("%d %s %s", code, &out, &diag)
		}
	}
	e, _ := s.Resolve("production")
	e.Classification = "nonproduction"
	input := map[string]any{"id": e.ID, "name": e.Name, "classification": e.Classification, "baseURLs": e.BaseURLs, "variables": e.Variables, "revision": e.Revision}
	b, _ := json.Marshal(input)
	var out, diag bytes.Buffer
	if code := cli.Run([]string{"environment", "update", "--input", "-", "--project", root, "--output", "json"}, strings.NewReader(string(b)), &out, &diag); code != 4 {
		t.Fatalf("%d %s %s", code, &out, &diag)
	}
}
