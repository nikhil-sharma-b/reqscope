package project_test

import (
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
	"os"
	"path/filepath"
	"testing"
)

func TestDestinationApproval(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, err := project.Init(root, "Demo", false)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = "https://{{host}}/v1"
	e.Variables["host"] = "staging.example.com"
	b, _ := yaml.Marshal(e.Environment)
	if err := project.Apply(root, []project.Change{{Path: filepath.Join("environments", e.ID+".yaml"), ExpectedRevision: e.Revision, Content: b}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := project.CheckCLIAccess(root, "staging", "api", ""); err == nil {
		t.Fatal("default allowed")
	}
	review, err := project.ReviewAccess(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	if _, err := project.CheckCLIAccess(root, "staging", "api", "https://staging.example.com:443/v1/items"); err != nil {
		t.Fatal(err)
	}
	if _, err := project.CheckCLIAccess(root, "", "api", ""); err == nil {
		t.Fatal("implicit environment")
	}
	if _, err := project.CheckCLIAccess(root, "production", "api", ""); err == nil {
		t.Fatal("production allowed")
	}
	if _, err := project.CheckCLIAccess(root, "staging", "api", "https://staging.example.com.evil/v1"); err == nil {
		t.Fatal("foreign origin")
	}
	s, _ = project.Load(root)
	e, _ = s.Resolve(e.ID)
	e.Name = "QA"
	b, _ = yaml.Marshal(e.Environment)
	if err := project.Apply(root, []project.Change{{Path: filepath.Join("environments", e.ID+".yaml"), ExpectedRevision: e.Revision, Content: b}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := project.CheckCLIAccess(root, "QA", "api", ""); err != nil {
		t.Fatal(err)
	}
	s, _ = project.Load(root)
	e, _ = s.Resolve(e.ID)
	e.Variables["host"] = "other.example.com"
	b, _ = yaml.Marshal(e.Environment)
	if err := project.Apply(root, []project.Change{{Path: filepath.Join("environments", e.ID+".yaml"), ExpectedRevision: e.Revision, Content: b}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := project.CheckCLIAccess(root, "QA", "api", ""); err == nil {
		t.Fatal("stale approval")
	}
}

func TestProductionProtectionAndLocalSelection(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, err := project.Init(root, "Demo", false)
	if err != nil {
		t.Fatal(err)
	}
	prod, _ := s.Resolve("production")
	prod.Name = "Harmless name"
	prod.Classification = "nonproduction"
	if err := project.SaveEnvironment(root, prod.Environment, prod.Revision, false, false); err == nil {
		t.Fatal("CLI lowered protection")
	}
	b, _ := yaml.Marshal(prod.Environment)
	if err := project.Apply(root, []project.Change{{Path: filepath.Join("environments", prod.ID+".yaml"), ExpectedRevision: prod.Revision, Content: b}}, false); err == nil {
		t.Fatal("Apply lowered protection")
	}
	if err := project.SaveEnvironment(root, prod.Environment, prod.Revision, false, true); err != nil {
		t.Fatal(err)
	}
	s, _ = project.Load(root)
	e, _ := s.Resolve(prod.ID)
	if e.Classification != "nonproduction" {
		t.Fatal("confirmed lowering failed")
	}
	stage, _ := s.Resolve("staging")
	if err := project.SelectEnvironment(root, stage.ID); err != nil {
		t.Fatal(err)
	}
	remembered, err := project.RememberedEnvironment(root)
	if err != nil || remembered.ID != stage.ID {
		t.Fatalf("selection %v %v", remembered, err)
	}
	// Raising classification invalidates selection independently of the name.
	stage.Classification = "production"
	stage.BaseURLs["api"] = "https://example.com"
	if err := project.SaveEnvironment(root, stage.Environment, stage.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	if err := project.SelectEnvironment(root, stage.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := project.RememberedEnvironment(root); err == nil {
		t.Fatal("restored production")
	}
	r, err := project.ReviewAccess(root, stage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := project.ApproveAccess(root, r); err != nil {
		t.Fatal(err)
	}
	if _, err := project.CheckCLIAccess(root, stage.ID, "api", ""); err != nil {
		t.Fatal("deliberate production grant", err)
	}
	copied := t.TempDir()
	if err := os.MkdirAll(filepath.Join(copied, "environments"), 0700); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(root, "reqscope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(copied, "reqscope.yaml"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(root, "environments", "*.yaml"))
	for _, file := range files {
		data, _ := os.ReadFile(file)
		if err := os.WriteFile(filepath.Join(copied, "environments", filepath.Base(file)), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := project.CheckCLIAccess(copied, stage.ID, "api", ""); err == nil {
		t.Fatal("copied grant")
	}
	s, _ = project.Load(root)
	stage, _ = s.Resolve(stage.ID)
	stage.Classification = "nonproduction"
	if err := project.SaveEnvironment(root, stage.Environment, stage.Revision, false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := project.CheckCLIAccess(root, stage.ID, "api", ""); err == nil {
		t.Fatal("classification retained grant")
	}
}

func TestPromotionProtectsAgainstImmediateExternalDowngrade(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, _ := project.Init(root, "Demo", false)
	e, _ := s.Resolve("staging")
	e.Classification = "production"
	if err := project.SaveEnvironment(root, e.Environment, e.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	e.Classification = "nonproduction"
	b, _ := yaml.Marshal(e.Environment)
	if err := os.WriteFile(filepath.Join(root, "environments", e.ID+".yaml"), b, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	e, err = s.Resolve(e.ID)
	if err != nil || e.Classification != "production" {
		t.Fatalf("lost protection %v %v", e, err)
	}
}
