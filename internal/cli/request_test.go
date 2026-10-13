package cli_test

import (
	"bytes"
	"encoding/json"
	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
