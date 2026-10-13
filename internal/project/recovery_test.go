package project_test

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestInterruptedWriter(t *testing.T) {
	root := os.Getenv("REQSCOPE_TEST_WRITER_ROOT")
	if root == "" {
		return
	}
	s, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s.Manifest.Name = "Recovered"
	b, _ := yaml.Marshal(s.Manifest)
	changes := []project.Change{{Path: "reqscope.yaml", ExpectedRevision: s.Revision, Content: b}}
	for i := 0; i < 1500; i++ {
		e := project.Environment{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprint(i))).String(), Name: fmt.Sprintf("item-%d", i), Classification: "nonproduction", BaseURLs: map[string]string{}, Variables: map[string]string{}}
		b, _ := yaml.Marshal(e)
		changes = append(changes, project.Change{Path: filepath.Join("environments", e.ID+".yaml"), Content: b})
	}
	if err = project.Apply(root, changes, false); err != nil {
		t.Fatal(err)
	}
}
func TestInterruptedBatchRecoversBeforeRead(t *testing.T) {
	root := t.TempDir()
	before, err := project.Init(root, "Original", false)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestInterruptedWriter$")
	cmd.Env = append(os.Environ(), "REQSCOPE_TEST_WRITER_ROOT="+root)
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.After(10 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	observed := false
	for !observed {
		select {
		case err := <-done:
			t.Fatalf("writer exited before interruption: %v", err)
		case <-deadline:
			cmd.Process.Kill()
			<-done
			t.Fatal("journal not published")
		case <-tick.C:
			_, err := os.Stat(filepath.Join(root, ".reqscope-transaction.json"))
			observed = err == nil
		}
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-done
	reopened, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Manifest.ID != before.Manifest.ID || reopened.Manifest.Name != "Recovered" || len(reopened.Environments) != 1503 {
		t.Fatalf("partial state exposed: %s, %d environments", reopened.Manifest.Name, len(reopened.Environments))
	}
	again, err := project.Load(root)
	if err != nil || len(again.Environments) != 1503 {
		t.Fatal("recovery was not durable", err)
	}
}
