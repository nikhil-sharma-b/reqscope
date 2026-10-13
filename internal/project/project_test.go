package project_test

import (
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"os"
	"path/filepath"
	"testing"
)

func TestInitializeAndDiscover(t *testing.T) {
	root := t.TempDir()
	p, err := project.Init(root, "Demo", false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Manifest.ID == "" || len(p.Environments) != 3 {
		t.Fatalf("missing identity or templates: %+v", p)
	}
	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	found, err := project.Discover(child)
	if err != nil || found != root {
		t.Fatalf("discovery: %s %v", found, err)
	}
	reopened, err := project.Load(found)
	if err != nil || reopened.Manifest.ID != p.Manifest.ID {
		t.Fatalf("reopen: %+v %v", reopened, err)
	}
}
