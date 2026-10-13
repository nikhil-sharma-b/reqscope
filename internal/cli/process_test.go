package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIProcess(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "reqscope")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/reqscope")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, b)
	}
	root := filepath.Join(t.TempDir(), "project")
	var projectID string
	run := func(input string, want int, args ...string) map[string]any {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Stdin = strings.NewReader(input)
		var out, diag bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &diag
		err := cmd.Run()
		code := 0
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				code = e.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if code != want {
			t.Fatalf("%v: exit %d want %d: %s %s", args, code, want, &out, &diag)
		}
		var result map[string]any
		decoder := json.NewDecoder(&out)
		if err := decoder.Decode(&result); err != nil {
			t.Fatalf("invalid envelope: %v %s", err, &out)
		}
		if decoder.More() {
			t.Fatal("extra JSON output")
		}
		return result
	}
	r := run("name: Process\n", 0, "project", "init", "--project", root, "--input", "-", "--output", "json")
	data := r["data"].(map[string]any)
	projectID = data["project"].(map[string]any)["id"].(string)
	r = run("", 0, "--project", root, "project", "inspect", "--output", "json")
	data = r["data"].(map[string]any)
	if data["project"].(map[string]any)["id"] != projectID {
		t.Fatal("ID changed on reopen")
	}
	revision := data["revision"].(string)
	run(`{"name":"Renamed","revision":"`+revision+`"}`, 0, "project", "update", "--project", root, "--input", "-", "--preview", "--output", "json")
	r = run("", 0, "--project", root, "project", "inspect", "--output", "json")
	if r["data"].(map[string]any)["project"].(map[string]any)["name"] != "Process" {
		t.Fatal("preview persisted changes")
	}
	run(`{"name":"Renamed","revision":"`+revision+`"}`, 0, "project", "update", "--project", root, "--input", "-", "--output", "json")
	run(`{"name":"Stale","revision":"`+revision+`"}`, 3, "project", "update", "--project", root, "--input", "-", "--output", "json")
	run(`{"name":"Bad","extra":true}`, 2, "input-schema", "validate", "project.init", "--input", "-", "--output", "json")
	run("", 2, "tui", "--output", "json")
	child := filepath.Join(root, "nested")
	os.Mkdir(child, 0755)
	cmd := exec.Command(binary, "project", "validate", "--output", "json")
	cmd.Dir = child
	if b, err := cmd.CombinedOutput(); err != nil || !bytes.Contains(b, []byte(projectID)) {
		t.Fatalf("ancestor discovery: %v %s", err, b)
	}
	cmd = exec.Command(binary)
	cmd.Stdin = strings.NewReader("")
	b, err := cmd.CombinedOutput()
	if err != nil || !bytes.Contains(b, []byte("Usage:")) {
		t.Fatalf("noninteractive no args: %v %s", err, b)
	}
}
