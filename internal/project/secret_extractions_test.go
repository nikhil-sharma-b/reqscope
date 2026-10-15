package project_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func approveExtractions(t *testing.T, root, id string) {
	t.Helper()
	review, err := project.ReviewScenarioExtractions(root, "staging", id)
	if err != nil {
		t.Fatal(err)
	}
	if err := project.ApproveScenarioExtractions(root, "staging", id, review.Fingerprint); err != nil {
		t.Fatal(err)
	}
}
func secureScenario(t *testing.T, root, target string) project.ScenarioResource {
	t.Helper()
	login := scenarioCase(t, root, "/login")
	consumer := scenarioCase(t, root, "/me")
	consumer.Request.Headers = []project.RequestField{{Name: "Authorization", Value: "{{auth}}"}}
	var err error
	consumer, err = project.SaveCase(root, consumer.Case, consumer.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	return scenarioSave(t, root, []project.ScenarioStep{
		{ID: "login", CaseID: login.ID, SecretExtracts: map[string]project.SecretExtraction{"session": {Pointer: "/data/session", Destinations: []project.CredentialBinding{{Kind: "bearer", Module: "api", Destination: target + "/me", In: "header", Field: "Authorization"}}}}},
		{ID: "me", CaseID: consumer.ID, Bindings: map[string]project.ScenarioBinding{"auth": {StepID: "login", Extract: "session"}}},
	}, false)
}
func TestProtectedScenarioLoginAndNoSecretOutputs(t *testing.T) {
	const token = "newly-issued-opaque-value"
	var authenticated atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/me" {
			authenticated.Store(r.Header.Get("Authorization") == "Bearer "+token)
		}
		w.Header().Set("X-Echo", token)
		io.WriteString(w, `{"data":{"session":"`+token+`"},"echo":"`+token+`"}`)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	s := secureScenario(t, root, server.URL)
	if _, err := project.RunScenario(context.Background(), root, "staging", s.ID, project.ScenarioOptions{}); err == nil {
		t.Fatal("unapproved extraction ran")
	}
	approveExtractions(t, root, s.ID)
	credentials := project.NewCredentials(nil)
	report, err := project.RunScenario(context.Background(), root, "staging", s.ID, project.ScenarioOptions{SendOptions: project.SendOptions{Credentials: credentials}})
	if err != nil || !report.Successful || !authenticated.Load() {
		t.Fatalf("login failed: %+v %v", report, err)
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), token) || strings.Contains(report.Steps[0].Exchange.Headers.Get("X-Echo"), token) {
		t.Fatal("token leaked into report")
	}
	// Both adapters must retain the aggregate and link each actual exchange.
	page, historyErr := project.History(root, "", 100, 0)
	if historyErr != nil || page.Total != 3 {
		t.Fatalf("want aggregate and two steps, got %+v %v", page, historyErr)
	}
	aggregate, historyErr := project.GetRun(root, report.RunID)
	if historyErr != nil || aggregate.Scenario == nil || len(aggregate.Scenario.Steps) != 2 {
		t.Fatalf("aggregate unavailable: %+v %v", aggregate, historyErr)
	}
	for _, step := range aggregate.Scenario.Steps {
		recorded, e := project.GetRun(root, step.Exchange.RunID)
		if e != nil || recorded.ScenarioRunID != report.RunID || recorded.StepID != step.ID || recorded.CaseID != step.CaseID {
			t.Fatalf("step history not linked: %+v %v", recorded, e)
		}
		b, _ := json.Marshal(recorded)
		if strings.Contains(string(b), token) {
			t.Fatal("secret leaked into step history")
		}
	}
	if credentials.Sanitize(token) != "[REDACTED]" {
		t.Fatal("adapter redaction registry lost")
	}
	for _, dir := range []string{root, os.Getenv("XDG_DATA_HOME")} {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			b, err := os.ReadFile(path)
			if strings.Contains(string(b), token) {
				t.Errorf("token persisted: %s", path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// An ordinary copy of the login producer cannot read an approved pointer.
	ordinary := scenarioSave(t, root, []project.ScenarioStep{{ID: "copy", CaseID: s.Steps[0].CaseID, Extracts: map[string]string{"readback": "/data/session"}}}, false)
	report, err = project.RunScenario(context.Background(), root, "staging", ordinary.ID, project.ScenarioOptions{})
	if err == nil || report.Steps[0].Error.Code != "extraction_failed" {
		t.Fatal("ordinary extraction read protected pointer", report, err)
	}
}

func TestProtectedScenarioRejectsChangedDefinitionsAndDestinations(t *testing.T) {
	for _, change := range []string{"pointer", "binding", "case", "environment", "remove-protection", "runtime-destination"} {
		t.Run(change, func(t *testing.T) {
			var sends atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sends.Add(1)
				io.WriteString(w, `{"data":{"session":"test-session-credential"}}`)
			}))
			defer server.Close()
			root := requestWorkspace(t, server.URL)
			s := secureScenario(t, root, server.URL)
			if change == "runtime-destination" {
				consumer, _ := project.GetCase(root, s.Steps[1].CaseID)
				consumer.Request.URL = "/{{path}}"
				if _, err := project.SaveCase(root, consumer.Case, consumer.Revision, false); err != nil {
					t.Fatal(err)
				}
			}
			approveExtractions(t, root, s.ID)
			opts := project.ScenarioOptions{}
			switch change {
			case "pointer":
				extract := s.Steps[0].SecretExtracts["session"]
				extract.Pointer = "/echo"
				s.Steps[0].SecretExtracts["session"] = extract
			case "binding":
				extract := s.Steps[0].SecretExtracts["session"]
				extract.Destinations[0].Destination = server.URL + "/steal"
			case "remove-protection":
				s.Steps[0].SecretExtracts = nil
				s.Steps[0].Extracts = map[string]string{"session": "/data/session"}
			case "case":
				c, _ := project.GetCase(root, s.Steps[1].CaseID)
				c.Request.URL = "/steal"
				if _, err := project.SaveCase(root, c.Case, c.Revision, false); err != nil {
					t.Fatal(err)
				}
			case "environment":
				snapshot, _ := project.Load(root)
				env, _ := snapshot.Resolve("staging")
				env.BaseURLs["api"] = server.URL + "/changed"
				if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
					t.Fatal(err)
				}
			case "runtime-destination":
				opts.Variables = map[string]string{"path": "steal"}
			}
			if change == "pointer" || change == "binding" || change == "remove-protection" {
				if _, err := project.SaveScenario(root, s.Scenario, s.Revision, false); err != nil {
					t.Fatal(err)
				}
			}
			report, err := project.RunScenario(context.Background(), root, "staging", s.ID, opts)
			expectedSends := int32(0)
			if change == "runtime-destination" {
				expectedSends = 1
			}
			if err == nil || report.Successful || sends.Load() != expectedSends {
				t.Fatalf("changed policy executed: %+v %v sends=%d", report, err, sends.Load())
			}
		})
	}
}

func TestProtectedScenarioFailedExtractionSkipsDependentSends(t *testing.T) {
	for _, body := range []string{`{"other":"no-token"}`, `{"data":{"session":12}}`, `{"data":{"session":""}}`, `{"data":{"session":"partial-secret`} {
		t.Run(body, func(t *testing.T) {
			var sends atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sends.Add(1)
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, body)
			}))
			defer server.Close()
			root := requestWorkspace(t, server.URL)
			s := secureScenario(t, root, server.URL)
			s.ContinueOnFailure = true
			var err error
			s, err = project.SaveScenario(root, s.Scenario, s.Revision, false)
			if err != nil {
				t.Fatal(err)
			}
			approveExtractions(t, root, s.ID)
			report, err := project.RunScenario(context.Background(), root, "staging", s.ID, project.ScenarioOptions{})
			if err == nil || sends.Load() != 1 || report.Steps[1].Outcome != "skipped" || report.Steps[0].Error.Code != "extraction_failed" {
				t.Fatal(report, err, sends.Load())
			}
			raw, _ := json.Marshal(report)
			if strings.Contains(string(raw), "partial-secret") {
				t.Fatal("partial token leaked")
			}
		})
	}
}

func TestProtectedPointersCannotBeReadBackAsNonStringValues(t *testing.T) {
	for _, value := range []string{`null`, `123`, `""`, `{"nested":"private-value"}`, `["private-value"]`} {
		t.Run(value, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"data":{"session":`+value+`}}`) }))
			defer server.Close()
			root := requestWorkspace(t, server.URL)
			s := secureScenario(t, root, server.URL)
			approveExtractions(t, root, s.ID)
			ordinary := scenarioSave(t, root, []project.ScenarioStep{{ID: "copy", CaseID: s.Steps[0].CaseID, Extracts: map[string]string{"readback": "/data/session"}}}, false)
			report, err := project.RunScenario(context.Background(), root, "staging", ordinary.ID, project.ScenarioOptions{})
			if err == nil || report.Successful {
				t.Fatal("protected non-string field readable", report)
			}
		})
	}
}

func TestProtectedTokensRejectUnapprovedOriginsPlacementsAndRedirects(t *testing.T) {
	for _, attack := range []string{"origin", "header", "query", "body", "redirect"} {
		t.Run(attack, func(t *testing.T) {
			var stolen atomic.Int32
			evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { stolen.Add(1) }))
			defer evil.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/steal" {
					stolen.Add(1)
				}
				if attack == "redirect" && r.URL.Path == "/me" {
					http.Redirect(w, r, "/steal", http.StatusTemporaryRedirect)
					return
				}
				io.WriteString(w, `{"data":{"session":"protected-fresh-token"}}`)
			}))
			defer server.Close()
			root := requestWorkspace(t, server.URL)
			s := secureScenario(t, root, server.URL)
			consumer, _ := project.GetCase(root, s.Steps[1].CaseID)
			switch attack {
			case "origin":
				consumer.Request.URL = evil.URL + "/steal"
			case "header":
				consumer.Request.Headers = []project.RequestField{{Name: "X-Steal", Value: "{{auth}}"}}
			case "query":
				consumer.Request.Headers = nil
				consumer.Request.Query = []project.RequestField{{Name: "stolen", Value: "{{auth}}"}}
			case "body":
				consumer.Request.Headers = nil
				consumer.Request.Method = "POST"
				consumer.Request.BodyType = "json"
				consumer.Request.Body = `{"stolen":"{{auth}}"}`
			}
			if _, err := project.SaveCase(root, consumer.Case, consumer.Revision, false); err != nil {
				t.Fatal(err)
			}
			approveExtractions(t, root, s.ID)
			report, err := project.RunScenario(context.Background(), root, "staging", s.ID, project.ScenarioOptions{})
			if err == nil || report.Successful || stolen.Load() != 0 {
				t.Fatal("token reached unauthorized destination", report, err, stolen.Load())
			}
		})
	}
}

func TestProtectedAPIKeysSupportPermittedQueryAndBody(t *testing.T) {
	for _, placement := range []string{"query", "body"} {
		t.Run(placement, func(t *testing.T) {
			var authenticated atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/me" {
					if placement == "query" {
						authenticated.Store(r.URL.Query().Get("key") == "fresh-api-key")
					} else {
						b, _ := io.ReadAll(r.Body)
						authenticated.Store(string(b) == `{"key":"fresh-api-key"}`)
					}
				}
				io.WriteString(w, `{"data":{"session":"fresh-api-key"}}`)
			}))
			defer server.Close()
			root := requestWorkspace(t, server.URL)
			s := secureScenario(t, root, server.URL)
			extract := s.Steps[0].SecretExtracts["session"]
			extract.Destinations[0].Kind = "api-key"
			extract.Destinations[0].In = placement
			extract.Destinations[0].Field = ""
			consumer, _ := project.GetCase(root, s.Steps[1].CaseID)
			consumer.Request.Headers = nil
			if placement == "query" {
				extract.Destinations[0].Field = "key"
				consumer.Request.Query = []project.RequestField{{Name: "key", Value: "{{auth}}"}}
			} else {
				consumer.Request.Method = "POST"
				consumer.Request.BodyType = "json"
				consumer.Request.Body = `{"key":"{{auth}}"}`
			}
			if _, err := project.SaveCase(root, consumer.Case, consumer.Revision, false); err != nil {
				t.Fatal(err)
			}
			if _, err := project.SaveScenario(root, s.Scenario, s.Revision, false); err != nil {
				t.Fatal(err)
			}
			approveExtractions(t, root, s.ID)
			report, err := project.RunScenario(context.Background(), root, "staging", s.ID, project.ScenarioOptions{})
			if err != nil || !report.Successful || !authenticated.Load() {
				t.Fatal("permitted token placement failed", report, err)
			}
		})
	}
}
