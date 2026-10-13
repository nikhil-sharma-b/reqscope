package project_test

import (
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestMalformedAndUnsupportedProjects(t *testing.T) {
	for _, content := range []string{"[broken", "formatVersion: 999\nid: bad\nname: Demo\n"} {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "reqscope.yaml"), []byte(content), 0644)
		if _, err := project.Load(root); err == nil {
			t.Fatal("invalid project accepted")
		}
	}
}
func TestBatchValidationPreventsPartialWrites(t *testing.T) {
	root := t.TempDir()
	s, _ := project.Init(root, "Original", false)
	m := s.Manifest
	m.Name = "Changed"
	b, _ := yaml.Marshal(m)
	err := project.Apply(root, []project.Change{{Path: "reqscope.yaml", ExpectedRevision: s.Revision, Content: b}, {Path: "../escape.yaml", Content: []byte("invalid")}}, false)
	if err == nil {
		t.Fatal("invalid batch accepted")
	}
	s, _ = project.Load(root)
	if s.Manifest.Name != "Original" {
		t.Fatal("partial batch exposed")
	}
}
func TestConcurrentRevisionsHaveOneWinner(t *testing.T) {
	root := t.TempDir()
	s, _ := project.Init(root, "Original", false)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"First", "Second"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			m := s.Manifest
			m.Name = name
			b, _ := yaml.Marshal(m)
			results <- project.Apply(root, []project.Change{{Path: "reqscope.yaml", ExpectedRevision: s.Revision, Content: b}}, false)
		}(name)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d concurrent writers succeeded", wins)
	}
}
func TestAmbiguousNamesRequireID(t *testing.T) {
	root := t.TempDir()
	s, _ := project.Init(root, "Demo", false)
	a, b := s.Environments[0], s.Environments[1]
	b.Name = a.Name
	raw, _ := yaml.Marshal(b.Environment)
	if err := project.Apply(root, []project.Change{{Path: filepath.Join("environments", b.ID+".yaml"), ExpectedRevision: b.Revision, Content: raw}}, false); err != nil {
		t.Fatal(err)
	}
	s, _ = project.Load(root)
	if _, err := s.Resolve(a.Name); err == nil {
		t.Fatal("ambiguous name resolved")
	}
	if got, err := s.Resolve(a.ID); err != nil || got.ID != a.ID {
		t.Fatal("UUID did not resolve")
	}
}

func TestImmutableIDAndPreviewWithoutDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	if _, err := project.Init(root, "Preview", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("initialization preview created files")
	}
	s, err := project.Init(root, "Real", false)
	if err != nil {
		t.Fatal(err)
	}
	m := s.Manifest
	m.ID = "00000000-0000-0000-0000-000000000001"
	b, _ := yaml.Marshal(m)
	if err = project.Apply(root, []project.Change{{Path: "reqscope.yaml", ExpectedRevision: s.Revision, Content: b}}, false); err == nil {
		t.Fatal("project identity was mutable")
	}
}
func TestSymlinkResourcesCannotEscapeProject(t *testing.T) {
	root := t.TempDir()
	s, err := project.Init(root, "Demo", false)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err = os.Remove(filepath.Join(root, "environments", s.Environments[0].ID+".yaml")); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(outside, "resource.yaml"), filepath.Join(root, "environments", s.Environments[0].ID+".yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err = project.Load(root); err == nil {
		t.Fatal("symlink resource loaded")
	}
}
