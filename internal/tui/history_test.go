package tui_test

import (
	"context"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
	"strings"
	"testing"
)

func TestHistoryReopenAndPinActions(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	if _, err := project.Init(root, "History", false); err != nil {
		t.Fatal(err)
	}
	ex, _ := project.Send(context.Background(), root, "local", project.RequestDraft{}, project.SendOptions{})
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"g", "h", "space", "h", "p"} {
		key(m, k)
	}
	r, err := project.GetRun(root, ex.RunID)
	if err != nil || !r.Pinned {
		t.Fatalf("%+v %v", r, err)
	}
	if !strings.Contains(m.View().Content, "failed") {
		t.Fatal(m.View().Content)
	}
	enter(m)
	if !strings.Contains(m.View().Content, "failed") {
		t.Fatal(m.View().Content)
	}
}
