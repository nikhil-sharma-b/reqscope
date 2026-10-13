package cli_test

import (
	"bytes"
	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"strings"
	"testing"
)

func TestJSONInitInspectAndNoninteractive(t *testing.T) {
	root := t.TempDir()
	var out, diag bytes.Buffer
	code := cli.Run([]string{"project", "init", "--project", root, "--input", "-", "--output", "json"}, strings.NewReader(`{"name":"Demo"}`), &out, &diag)
	if code != 0 || !strings.Contains(out.String(), `"schemaVersion":1`) {
		t.Fatalf("%d %s %s", code, &out, &diag)
	}
	out.Reset()
	code = cli.Run([]string{"project", "inspect", "--project", root, "--output", "json"}, strings.NewReader(""), &out, &diag)
	if code != 0 || !strings.Contains(out.String(), `"name":"Demo"`) {
		t.Fatalf("%d %s", code, &out)
	}
	out.Reset()
	code = cli.Run(nil, strings.NewReader(""), &out, &diag)
	if code != 0 || !strings.Contains(out.String(), "Usage:") {
		t.Fatal("no arguments did not print usage")
	}
}

func TestInvalidInputsKeepJSONEnvelope(t *testing.T) {
	for _, tc := range []struct {
		input string
		args  []string
	}{
		{"", []string{"--unknown", "value", "--output", "json"}},
		{"name: 123\n", []string{"input-schema", "validate", "project.init", "--input", "-", "--output", "json"}},
		{"name: One\n---\nname: Two\n", []string{"input-schema", "validate", "project.init", "--input", "-", "--output", "json"}},
		{"name: '   '\n", []string{"input-schema", "validate", "project.init", "--input", "-", "--output", "json"}},
	} {
		var out, diag bytes.Buffer
		if code := cli.Run(tc.args, strings.NewReader(tc.input), &out, &diag); code != 2 || !strings.Contains(out.String(), `"success":false`) {
			t.Fatalf("invalid input: %d %s %s", code, &out, &diag)
		}
	}
}
func TestMissingExplicitProjectIsNotFound(t *testing.T) {
	var out, diag bytes.Buffer
	root := t.TempDir() + "/missing"
	code := cli.Run([]string{"project", "inspect", "--project", root, "--output", "json"}, strings.NewReader(""), &out, &diag)
	if code != 3 || !strings.Contains(out.String(), `"code":"not_found"`) {
		t.Fatalf("missing project: %d %s %s", code, &out, &diag)
	}
}
