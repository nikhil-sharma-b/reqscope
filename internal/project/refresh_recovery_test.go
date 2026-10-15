package project_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func TestInterruptedRefreshWriter(t *testing.T) {
	root := os.Getenv("REQSCOPE_TEST_REFRESH_ROOT")
	if root == "" {
		return
	}
	_, err := project.RefreshSpecification(root, os.Getenv("REQSCOPE_TEST_REFRESH_SOURCE"), os.Getenv("REQSCOPE_TEST_REFRESH_TOKEN"), false, project.SourceImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInterruptedRefreshRecoversContractAndCasesTogether(t *testing.T) {
	root, s := authorFixture(t)
	ep := endpointAt(t, s, "/pets")
	changes := []project.Change{}
	const count = 800
	for i := 0; i < count; i++ {
		c := project.Case{ID: uuid.NewString(), Name: fmt.Sprintf("Case %04d", i), Request: project.RequestDraft{EndpointID: ep.ID, Body: "retained", BodyType: "raw"}}
		raw, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		changes = append(changes, project.Change{Path: "cases/" + c.ID + ".yaml", Content: raw})
	}
	if err := project.Apply(root, changes, false); err != nil {
		t.Fatal(err)
	}
	refreshDoc(t, s, func(doc map[string]any) { delete(doc["paths"].(map[string]any), "/pets") })
	p := previewRefresh(t, root, s.ID)
	cmd := exec.Command(os.Args[0], "-test.run=^TestInterruptedRefreshWriter$")
	cmd.Env = append(os.Environ(), "REQSCOPE_TEST_REFRESH_ROOT="+root, "REQSCOPE_TEST_REFRESH_SOURCE="+s.ID, "REQSCOPE_TEST_REFRESH_TOKEN="+p.Token)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	interrupted := false
	for !interrupted {
		select {
		case err := <-done:
			t.Fatalf("refresh exited before interruption: %v", err)
		case <-deadline.C:
			cmd.Process.Kill()
			<-done
			t.Fatal("refresh journal not published")
		case <-tick.C:
			if _, err := os.Stat(filepath.Join(root, ".reqscope-transaction.json")); err == nil {
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				<-done
				interrupted = true
			}
		}
	}
	// Public reads must finish recovery before exposing either resource collection.
	specs, err := project.Specifications(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, ep := range specs[0].Endpoints {
		if ep.Path == "/pets" {
			t.Fatal("old contract exposed after recovery")
		}
	}
	if len(specs[0].DeletedEndpoints) != 1 {
		t.Fatal("tombstone not recovered")
	}
	items, err := project.Cases(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != count {
		t.Fatalf("recovered %d cases", len(items))
	}
	for _, c := range items {
		if !c.Request.Disconnected || c.Request.Body != "retained" {
			t.Fatal("partial refresh exposed", c.ID)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".reqscope-transaction.json")); !os.IsNotExist(err) {
		t.Fatal("journal not retired", err)
	}
}
