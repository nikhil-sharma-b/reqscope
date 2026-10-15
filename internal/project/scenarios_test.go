package project_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
)

func scenarioCase(t *testing.T, root, path string) project.CaseResource {
	t.Helper()
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: path, Request: project.RequestDraft{Module: "api", Method: "GET", URL: path}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func scenarioSave(t *testing.T, root string, steps []project.ScenarioStep, continued bool) project.ScenarioResource {
	t.Helper()
	s, err := project.SaveScenario(root, project.Scenario{ID: uuid.NewString(), Name: "Workflow", Steps: steps, ContinueOnFailure: continued}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestScenarioBatchCannotCreateDanglingCaseReference(t *testing.T) {
	root := requestWorkspace(t, "http://localhost:12345")
	s := project.Scenario{ID: uuid.NewString(), Name: "Missing", Steps: []project.ScenarioStep{{ID: "first", CaseID: uuid.NewString()}}}
	b, _ := yaml.Marshal(s)
	if err := project.Apply(root, []project.Change{{Path: filepath.Join("scenarios", s.ID+".yaml"), Content: b}}, false); err == nil {
		t.Fatal("batch created a dangling scenario reference")
	}
}

func TestScenarioChainsEscapedPointersWithRunOnlyOverrides(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"a/b":{"~key":[9007199254740993,null]}}`)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	producer := scenarioCase(t, root, "/produce")
	consumer := scenarioCase(t, root, "/items/{{item}}?label={{label}}&nil={{nil}}")
	consumer.Request.Variables = map[string]string{"label": "source"}
	var err error
	consumer, err = project.SaveCase(root, consumer.Case, consumer.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	s := scenarioSave(t, root, []project.ScenarioStep{{ID: "produce", CaseID: producer.ID, Extracts: map[string]string{"id": "/a~1b/~0key/0", "nil": "/a~1b/~0key/1"}}, {ID: "consume", CaseID: consumer.ID, Overrides: map[string]string{"label": "step", "item": "wrong"}, Bindings: map[string]project.ScenarioBinding{"item": {StepID: "produce", Extract: "id"}, "nil": {StepID: "produce", Extract: "nil"}}}}, false)
	report, err := project.RunScenario(context.Background(), root, "staging", s.ID, project.ScenarioOptions{})
	if err != nil || !report.Successful || len(report.Steps) != 2 || len(paths) != 2 || paths[1] != "/items/9007199254740993?label=step&nil=null" {
		t.Fatalf("%+v %v %v", report, err, paths)
	}
	source, _ := project.GetCase(root, consumer.ID)
	if source.Request.Variables["label"] != "source" || source.Revision != consumer.Revision {
		t.Fatal("overrides changed source case")
	}
}

func TestScenarioFailurePoliciesAndDependentSkips(t *testing.T) {
	for _, cause := range []string{"assertion", "extraction", "incomplete", "transport"} {
		for _, continued := range []bool{false, true} {
			t.Run(cause+map[bool]string{false: "/fail-fast", true: "/continue"}[continued], func(t *testing.T) {
				var paths []string
				var pathMu sync.Mutex
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					pathMu.Lock()
					defer pathMu.Unlock()
					paths = append(paths, r.URL.Path)
					if cause == "transport" && r.URL.Path == "/producer" {
						h, _, _ := w.(http.Hijacker).Hijack()
						h.Close()
						return
					}
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"id":"value"}`)
				}))
				defer server.Close()
				root := requestWorkspace(t, server.URL)
				producer := scenarioCase(t, root, "/producer")
				if cause == "assertion" {
					project.Decode([]byte(`{"assertions":[{"kind":"status-equals","value":201}]}`), &producer.Validation)
				}
				if cause == "incomplete" {
					producer.Validation = map[string]any{"responseSchema": true}
				}
				if cause == "assertion" || cause == "incomplete" {
					if _, err := project.SaveCase(root, producer.Case, producer.Revision, false); err != nil {
						t.Fatal(err)
					}
				}
				dependent := scenarioCase(t, root, "/dependent/{{item}}")
				independent := scenarioCase(t, root, "/independent")
				pointer := "/id"
				if cause == "extraction" {
					pointer = "/missing"
				}
				s := scenarioSave(t, root, []project.ScenarioStep{{ID: "producer", CaseID: producer.ID, Extracts: map[string]string{"value": pointer}}, {ID: "dependent", CaseID: dependent.ID, Bindings: map[string]project.ScenarioBinding{"item": {StepID: "producer", Extract: "value"}}}, {ID: "independent", CaseID: independent.ID}}, continued)
				report, err := project.RunScenario(context.Background(), root, "staging", s.ID, project.ScenarioOptions{})
				pathMu.Lock()
				defer pathMu.Unlock()
				var pe *project.Error
				if !errors.As(err, &pe) || pe.Code != "scenario_failed" || report.Successful || len(report.Steps) != 3 || report.Steps[0].Outcome != "failed" || report.Steps[1].Outcome != "skipped" {
					t.Fatalf("%+v %v", report, err)
				}
				if continued {
					if len(paths) != 2 || paths[1] != "/independent" || report.Steps[2].Outcome != "passed" || !strings.Contains(report.Steps[1].Reason, "producer") {
						t.Fatalf("%+v %v", report, paths)
					}
				} else if len(paths) != 1 || report.Steps[2].Outcome != "skipped" {
					t.Fatalf("%+v %v", report, paths)
				}
			})
		}
	}
}

func TestScenarioSnapshotsLaterCasesDuringConcurrentEdit(t *testing.T) {
	var root string
	var later project.CaseResource
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/first" {
			later.Request.URL = "/edited"
			later.Validation = map[string]any{"responseSchema": true}
			if _, err := project.SaveCase(root, later.Case, later.Revision, false); err != nil {
				t.Error(err)
			}
		}
		io.WriteString(w, `{}`)
	}))
	defer server.Close()
	root = requestWorkspace(t, server.URL)
	first := scenarioCase(t, root, "/first")
	later = scenarioCase(t, root, "/original")
	s := scenarioSave(t, root, []project.ScenarioStep{{ID: "first", CaseID: first.ID}, {ID: "later", CaseID: later.ID}}, false)
	report, err := project.RunScenario(context.Background(), root, "staging", s.ID, project.ScenarioOptions{})
	if err != nil || !report.Successful || len(paths) != 2 || paths[1] != "/original" {
		t.Fatalf("%+v %v %v", report, err, paths)
	}
	edited, _ := project.GetCase(root, later.ID)
	if edited.Request.URL != "/edited" {
		t.Fatal("concurrent edit did not occur")
	}
}

func TestScenarioCancellationStopsActiveTransportAndFurtherSteps(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); close(started); <-r.Context().Done() }))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	c := scenarioCase(t, root, "/wait")
	s := scenarioSave(t, root, []project.ScenarioStep{{ID: "first", CaseID: c.ID}, {ID: "later", CaseID: c.ID}}, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		report, err := project.RunScenario(ctx, root, "staging", s.ID, project.ScenarioOptions{})
		if report.Successful || len(report.Steps) != 2 || report.Steps[1].Outcome != "skipped" {
			done <- errors.New("incorrect cancellation report")
			return
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("transport not started")
	}
	cancel()
	select {
	case err := <-done:
		var pe *project.Error
		if !errors.As(err, &pe) || pe.Code != "cancelled" || calls.Load() != 1 {
			t.Fatalf("%v calls=%d", err, calls.Load())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation blocked")
	}
}

func TestScenarioCRUDReferencesAndUnsupportedFeatures(t *testing.T) {
	root := requestWorkspace(t, "http://localhost:12345")
	c := scenarioCase(t, root, "/")
	s := project.Scenario{ID: uuid.NewString(), Name: "Workflow", Steps: []project.ScenarioStep{{ID: "step", CaseID: c.ID}}}
	if _, err := project.SaveScenario(root, s, "", true); err != nil {
		t.Fatal(err)
	}
	items, _ := project.Scenarios(root)
	if len(items) != 0 {
		t.Fatal("preview wrote scenario")
	}
	saved, err := project.SaveScenario(root, s, "", false)
	if err != nil {
		t.Fatal(err)
	}
	source, _ := project.GetCase(root, c.ID)
	if len(source.References) != 1 {
		t.Fatal("affected scenarios absent")
	}
	if _, err := project.DeleteCase(root, c.ID, c.Revision, false); err == nil {
		t.Fatal("referenced case deleted")
	}
	s.Name = "Changed"
	updated, err := project.SaveScenario(root, s, saved.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := project.SaveScenario(root, s, saved.Revision, false); err == nil {
		t.Fatal("stale scenario update accepted")
	}
	if _, err := project.CheckScenario(root, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := project.DeleteScenario(root, s.ID, updated.Revision, true); err != nil {
		t.Fatal(err)
	}
	if _, err := project.GetScenario(root, s.ID); err != nil {
		t.Fatal("preview deleted scenario")
	}
	if _, err := project.DeleteScenario(root, s.ID, updated.Revision, false); err != nil {
		t.Fatal(err)
	}
	if _, err := project.GetScenario(root, s.ID); err == nil {
		t.Fatal("deleted scenario found")
	}
	for _, feature := range []string{"scenarioId", "loop", "script"} {
		var bad project.Scenario
		if err := project.Decode([]byte(`{"id":"`+s.ID+`","name":"Bad","steps":[{"id":"one","caseId":"`+c.ID+`","`+feature+`":"unsupported"}]}`), &bad); err == nil {
			t.Fatal("unsupported feature accepted", feature)
		}
	}
	for _, steps := range [][]project.ScenarioStep{{{ID: "same", CaseID: c.ID}, {ID: "same", CaseID: c.ID}}, {{ID: "first", CaseID: c.ID, Bindings: map[string]project.ScenarioBinding{"v": {StepID: "later", Extract: "x"}}}, {ID: "later", CaseID: c.ID, Extracts: map[string]string{"x": "/id"}}}, {{ID: "step", CaseID: c.ID, Extracts: map[string]string{"x": "/~2"}}}} {
		s.Steps = steps
		if err := project.ValidateScenario(s); err == nil {
			t.Fatal("invalid scenario accepted")
		}
	}
}

func TestScenarioSnapshotsContractConstructionAndResponseChecks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("contract method changed")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":1}`)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	path := filepath.Join(t.TempDir(), "api.json")
	doc := `{"openapi":"3.0.3","info":{"title":"API","version":"1"},"paths":{"/":{"get":{"responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"object"}}}}}}}}}`
	if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	spec, err := project.ImportSpecification(root, path, false)
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := project.Load(root)
	env, _ := workspace.Resolve("staging")
	env.BaseURLs[spec.ModuleID] = server.URL
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	review, _ := project.ReviewAccess(root, "staging")
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "Contract", Request: project.RequestDraft{EndpointID: spec.Endpoints[0].ID}, Validation: map[string]any{"responseSchema": true}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	s := scenarioSave(t, root, []project.ScenarioStep{{ID: "first", CaseID: c.ID}, {ID: "second", CaseID: c.ID}}, false)
	run, err := project.SnapshotScenario(root, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	spec.Endpoints[0].Method = "POST"
	spec.Endpoints[0].Operation["responses"] = map[string]any{}
	b, _ := yaml.Marshal(spec)
	if err := project.Apply(root, []project.Change{{Path: filepath.Join("specs", spec.ID+".yaml"), ExpectedRevision: spec.Revision, Content: b}}, false); err != nil {
		t.Fatal(err)
	}
	report, err := run.Run(context.Background(), root, "staging", project.ScenarioOptions{})
	if err != nil || !report.Successful || report.Steps[0].ContractDigest == "" || report.Steps[0].Exchange.Checks[0].Outcome != "passed" {
		t.Fatalf("%+v %v", report, err)
	}
}

func TestScenarioReportsAndOrdinaryExtractsAreSanitized(t *testing.T) {
	const secret = "session-sensitive"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"token":"`+secret+`"}`)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	c := scenarioCase(t, root, "/")
	s := scenarioSave(t, root, []project.ScenarioStep{{ID: "first", CaseID: c.ID, Extracts: map[string]string{"token": "/token"}}}, false)
	report, err := project.RunScenario(context.Background(), root, "staging", s.ID, project.ScenarioOptions{})
	b, _ := json.Marshal(report)
	if err == nil || report.Successful || strings.Contains(string(b), secret) {
		t.Fatalf("%s %v", b, err)
	}
}

func TestScenarioProductionConfirmationFailureEndsContinuedRun(t *testing.T) {
	calls := 0
	confirmations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; io.WriteString(w, `{}`) }))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	workspace, _ := project.Load(root)
	env, _ := workspace.Resolve("production")
	env.BaseURLs["api"] = server.URL
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	c := scenarioCase(t, root, "/")
	s := scenarioSave(t, root, []project.ScenarioStep{{ID: "one", CaseID: c.ID}, {ID: "two", CaseID: c.ID}}, true)
	report, err := project.RunScenario(context.Background(), root, "production", s.ID, project.ScenarioOptions{SendOptions: project.SendOptions{Interactive: true}, Confirm: func(context.Context, project.PreparedRequest) (string, error) {
		confirmations++
		return "", project.Fail("access_denied", "confirmation unavailable")
	}})
	if err == nil || report.Successful || confirmations != 1 || calls != 0 || report.Steps[1].Outcome != "skipped" {
		t.Fatalf("%+v %v confirmations=%d calls=%d", report, err, confirmations, calls)
	}
}

func TestScenarioRedirectConfirmationFailureEndsContinuedRun(t *testing.T) {
	calls := 0
	confirmations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/done", http.StatusFound)
		} else {
			io.WriteString(w, `{}`)
		}
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	workspace, _ := project.Load(root)
	env, _ := workspace.Resolve("production")
	env.BaseURLs["api"] = server.URL
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	c := scenarioCase(t, root, "/")
	s := scenarioSave(t, root, []project.ScenarioStep{{ID: "one", CaseID: c.ID}, {ID: "two", CaseID: c.ID}}, true)
	report, err := project.RunScenario(context.Background(), root, "production", s.ID, project.ScenarioOptions{SendOptions: project.SendOptions{Interactive: true}, Confirm: func(_ context.Context, p project.PreparedRequest) (string, error) {
		confirmations++
		if confirmations == 2 {
			return "", project.Fail("access_denied", "confirmation unavailable")
		}
		return p.Fingerprint, nil
	}})
	if err == nil || report.Successful || confirmations != 2 || calls != 1 || report.Steps[1].Outcome != "skipped" {
		t.Fatalf("%+v %v confirmations=%d calls=%d", report, err, confirmations, calls)
	}
}

func TestFailedScenarioHistoryCanBeReopenedPinnedAndBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	c := scenarioCase(t, root, "/")
	steps := []project.ScenarioStep{{ID: "producer", CaseID: c.ID, Extracts: map[string]string{"missing": "/missing"}}}
	for i := 0; i < 55; i++ {
		steps = append(steps, project.ScenarioStep{ID: fmt.Sprintf("step-%d", i), CaseID: c.ID})
	}
	scenario := scenarioSave(t, root, steps, false)
	report, err := project.RunScenario(context.Background(), root, "staging", scenario.ID, project.ScenarioOptions{})
	if err == nil || report.Successful || report.Steps[1].Outcome != "skipped" {
		t.Fatal(report, err)
	}
	stored, err := project.GetRun(root, report.RunID)
	if err != nil || stored.Scenario == nil || stored.Outcome != "failed" || len(stored.Scenario.Steps) != 56 {
		t.Fatal(stored, err)
	}
	skipped, err := project.GetRun(root, report.Steps[1].Exchange.RunID)
	if err != nil || skipped.Outcome != "skipped" || skipped.ScenarioRunID != report.RunID || skipped.Response.Transport != "not-started" {
		t.Fatal("skip evidence missing", skipped, err)
	}
	preview := project.PreviewRun(stored, 4096)
	if len(preview.Scenario.Steps) != 50 || !preview.Scenario.StepsTruncated || preview.Scenario.TotalSteps != 56 || preview.Scenario.Steps[0].Exchange.Body != "" {
		t.Fatal(preview)
	}
	if len(stored.Scenario.Steps) != 56 || stored.Scenario.StepsTruncated {
		t.Fatal("preview mutated stored report")
	}
	if err := project.ChangeRun(root, report.RunID, "associate", c.ID); err == nil {
		t.Fatal("aggregate associated with case")
	}
	if err := project.ChangeRun(root, report.RunID, "pin", ""); err != nil {
		t.Fatal(err)
	}
	if err := project.SetHistoryLimits(root, project.HistoryLimits{PerCase: 0, MaxBytes: 0}); err != nil {
		t.Fatal(err)
	}
	stored, err = project.GetRun(root, report.RunID)
	if err != nil || !stored.Pinned || stored.Scenario == nil {
		t.Fatal("pinned report evicted", err)
	}
	page, err := project.History(root, "", 1, 0)
	if err != nil || page.Total != 1 || !page.PinnedOverLimit || page.Items[0].Scenario != nil || page.Items[0].ScenarioID != scenario.ID {
		t.Fatal(page, err)
	}
}
