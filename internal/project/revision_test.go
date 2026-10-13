package project_test

import (
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
	"testing"
)

func TestRevisionConflictAndPreview(t *testing.T) {
	root := t.TempDir()
	s, err := project.Init(root, "Before", false)
	if err != nil {
		t.Fatal(err)
	}
	m := s.Manifest
	m.Name = "After"
	b, _ := yaml.Marshal(m)
	c := []project.Change{{Path: "reqscope.yaml", ExpectedRevision: s.Revision, Content: b}}
	if err = project.Apply(root, c, true); err != nil {
		t.Fatal(err)
	}
	current, _ := project.Load(root)
	if current.Manifest.Name != "Before" {
		t.Fatal("preview wrote changes")
	}
	if err = project.Apply(root, c, false); err != nil {
		t.Fatal(err)
	}
	if err = project.Apply(root, c, false); err == nil {
		t.Fatal("stale write accepted")
	}
	current, _ = project.Load(root)
	if current.Manifest.Name != "After" || current.Manifest.ID != s.Manifest.ID {
		t.Fatal("update lost name or identity")
	}
}
