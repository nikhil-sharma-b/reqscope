package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLISendAndInputFailure(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"x":1}` {
			t.Errorf("body %q", b)
		}
		w.WriteHeader(500)
		io.WriteString(w, "completed")
	}))
	defer server.Close()
	root := t.TempDir()
	s, _ := project.Init(root, "Demo", false)
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = server.URL
	project.SaveEnvironment(root, e.Environment, e.Revision, false, false)
	review, _ := project.ReviewAccess(root, "staging")
	project.ApproveAccess(root, review)
	args := []string{"request", "send", "--env", "staging", "--project", root, "--input", "-", "--output", "json"}
	var out, diag bytes.Buffer
	if code := cli.Run(args, strings.NewReader(`{"module":"api","method":"POST","url":"/","bodyType":"json","body":"{\"x\":1}"}`), &out, &diag); code != 0 {
		t.Fatalf("%d %s %s", code, &out, &diag)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status":500`) {
		t.Fatal(out.String())
	}
	out.Reset()
	if code := cli.Run(args, strings.NewReader(`{"module":"api","method":"POST","url":"/","bodyType":"json","body":"{"}`), &out, &diag); code != 2 || calls != 1 {
		t.Fatalf("%d %d %s", code, calls, &out)
	}
}

func TestCLIRequestCancellationProcess(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	root := t.TempDir()
	s, err := project.Init(root, "Cancel", false)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = server.URL
	if err = project.SaveEnvironment(root, e.Environment, e.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	review, err := project.ReviewAccess(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err = project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "reqscope")
	if b, err := exec.Command("go", "build", "-o", binary, "../../cmd/reqscope").CombinedOutput(); err != nil {
		t.Fatalf("build %v %s", err, b)
	}
	cmd := exec.Command(binary, "request", "send", "--project", root, "--env", "staging", "--input", "-", "--output", "json")
	cmd.Stdin = strings.NewReader(`{"module":"api","method":"GET","url":"/"}`)
	var out, diag bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diag
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request not started")
	}
	if err = cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 130 || !strings.Contains(out.String(), `"code":"cancelled"`) {
		t.Fatalf("exit %v %s %s", err, &out, &diag)
	}
}
