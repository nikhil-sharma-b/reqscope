package cli_test

import (
	"context"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"testing"
)

func TestCLIHistoryOperations(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	specCommand(t, root, "project", "init", "History")
	ex, _ := project.Send(context.Background(), root, "local", project.RequestDraft{}, project.SendOptions{})
	code, e := specCommand(t, root, "history", "list", "--limit", "1")
	if code != 0 {
		t.Fatal(e)
	}
	data := e["data"].(map[string]any)
	if data["total"] != float64(1) {
		t.Fatal(data)
	}
	for _, action := range []string{"get", "pin"} {
		if code, e = specCommand(t, root, "history", action, ex.RunID); code != 0 {
			t.Fatal(e)
		}
	}
	if code, _ = specCommand(t, root, "history", "delete", ex.RunID); code == 0 {
		t.Fatal("deleted pin")
	}
	for _, action := range []string{"unpin", "delete"} {
		if code, e = specCommand(t, root, "history", action, ex.RunID); code != 0 {
			t.Fatal(e)
		}
	}
	if code, e = specCommand(t, root, "history", "prune"); code != 0 {
		t.Fatal(e)
	}
	if code, _ = specCommand(t, root, "history", "list", "--limit", "0"); code != 2 {
		t.Fatal(code)
	}
}
