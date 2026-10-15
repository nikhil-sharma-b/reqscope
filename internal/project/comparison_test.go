package project_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func comparisonRuns(t *testing.T, before, after, media string, cap int) (string, project.CaseResource, string, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", media)
		if r.URL.Path == "/before" {
			w.Header().Set("X-Trace", "first")
			w.WriteHeader(200)
			fmt.Fprint(w, before)
		} else {
			w.Header().Set("X-Trace", "second")
			w.WriteHeader(201)
			fmt.Fprint(w, after)
		}
	}))
	t.Cleanup(server.Close)
	root := requestWorkspace(t, server.URL)
	c, e := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "comparison", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/{{path}}", MaxBodyBytes: int64(cap)}}, "", false)
	if e != nil {
		t.Fatal(e)
	}
	a, e := project.SendCase(context.Background(), root, "staging", c.ID, map[string]string{"path": "before"}, project.SendOptions{})
	if e != nil {
		t.Fatal(e)
	}
	// A different environment targets the same service but remains labelled.
	s, e := project.Load(root)
	if e != nil {
		t.Fatal(e)
	}
	env, e := s.Resolve("staging")
	if e != nil {
		t.Fatal(e)
	}
	env.ID = uuid.NewString()
	env.Name = "comparison-env"
	if e = project.SaveEnvironment(root, env.Environment, "", false, false); e != nil {
		t.Fatal(e)
	}
	review, e := project.ReviewAccess(root, env.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = project.ApproveAccess(root, review); e != nil {
		t.Fatal(e)
	}
	b, e := project.SendCase(context.Background(), root, "comparison-env", c.ID, map[string]string{"path": "after"}, project.SendOptions{})
	if e != nil {
		t.Fatal(e)
	}
	return root, c, a.RunID, b.RunID
}
func TestComparisonJSONTypesPointersAndSavedIgnores(t *testing.T) {
	root, c, a, b := comparisonRuns(t, `{"nil":null,"array":[1,2],"kind":true,"gone":1,"a/b":{"~key":"old"},"order":{"b":2,"a":1},"n":9007199254740993,"decimal":1}`, `{"array":[2,1],"kind":"true","new":null,"a/b":{"~key":"new"},"order":{"a":1,"b":2},"n":9007199254740992,"decimal":1.0}`, "application/json", 0)
	result, e := project.CompareRuns(root, a, b, nil)
	if e != nil {
		t.Fatal(e)
	}
	want := map[string]string{"/nil": "removed", "/new": "added", "/array/0": "changed", "/array/1": "changed", "/kind": "changed", "/gone": "removed", "/a~1b/~0key": "changed", "/n": "changed"}
	got := map[string]string{}
	for _, d := range result.Differences {
		if d.Area == "json" {
			got[d.Path] = d.Kind
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON changes: %#v want %#v", got, want)
	}
	if result.Before.Environment != "staging" || result.After.Environment != "comparison-env" || result.Before.InputDigest == result.After.InputDigest {
		t.Fatalf("missing context: %+v", result)
	}
	old, e := project.GetRun(root, a)
	if e != nil {
		t.Fatal(e)
	}
	c.ComparisonIgnore = project.IgnoreRules{JSONPointers: []string{"/array", "/a~1b/~0key"}, Headers: []string{"X-TRACE", "Date"}}
	if _, e = project.SaveCase(root, c.Case, c.Revision, false); e != nil {
		t.Fatal(e)
	}
	result, e = project.CompareRuns(root, a, b, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range result.Differences {
		if strings.HasPrefix(d.Path, "/array") || d.Path == "/a~1b/~0key" || d.Path == "x-trace" {
			t.Fatal("ignore failed", d)
		}
	}
	reopened, e := project.GetRun(root, a)
	if e != nil || !reflect.DeepEqual(old, reopened) {
		t.Fatal("comparison changed evidence", e)
	}
	result, e = project.CompareRuns(root, a, b, &project.IgnoreRules{})
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, d := range result.Differences {
		if d.Path == "x-trace" {
			found = true
		}
	}
	if !found {
		t.Fatal("temporary rules did not replace saved rules")
	}
}

func TestComparisonTextAndUnavailableEvidence(t *testing.T) {
	for _, tt := range []struct {
		name, before, after, media, format, limitation string
		cap                                            int
	}{
		{"text", "same\nold\ntail", "same\nnew\ntail", "text/plain", "text", "", 0},
		{"binary", "\x00first", "\x00second", "application/octet-stream", "unavailable", "binary", 0},
		{"redacted", `{"token":"old-secret","ok":true}`, `{"token":"new-secret","ok":false}`, "application/json", "json", "redacted", 0},
		{"truncated", "long first response", "long second response", "text/plain", "text", "truncated", 5},
		{"withheld", `{"secret":"sensitive-first"}`, `{"secret":"sensitive-second"}`, "application/json", "unavailable", "withheld", 8},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, _, a, b := comparisonRuns(t, tt.before, tt.after, tt.media, tt.cap)
			result, e := project.CompareRuns(root, a, b, nil)
			if e != nil {
				t.Fatal(e)
			}
			if result.BodyFormat != tt.format || tt.limitation != "" && !strings.Contains(strings.Join(result.Limitations, " "), tt.limitation) {
				t.Fatalf("%+v", result)
			}
			raw, _ := json.Marshal(result)
			for _, secret := range []string{"old-secret", "new-secret", "sensitive-first", "sensitive-second"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("secret leaked")
				}
			}
			if tt.name == "text" {
				got := []project.Difference{}
				for _, d := range result.Differences {
					if d.Area == "text" {
						got = append(got, d)
					}
				}
				want := []project.Difference{{Area: "text", Path: "/2", Kind: "removed", Before: "old"}, {Area: "text", Path: "/2", Kind: "added", After: "new"}}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("text diff: %#v", got)
				}
			}
		})
	}
}

func TestComparisonBoundsErrorsAndRuleValidation(t *testing.T) {
	before, after := map[string]any{}, map[string]any{}
	for i := 0; i < 150; i++ {
		k := fmt.Sprintf("field%03d", i)
		before[k] = "old"
		after[k] = strings.Repeat("x", 2000)
	}
	aJSON, _ := json.Marshal(before)
	bJSON, _ := json.Marshal(after)
	root, c, a, b := comparisonRuns(t, string(aJSON), string(bJSON), "application/json", 0)
	result, e := project.CompareRuns(root, a, b, nil)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(result)
	if len(result.Differences) != 100 || len(result.Limitations) == 0 || len(raw) > 300000 {
		t.Fatalf("unbounded comparison %d %d %+v", len(result.Differences), len(raw), result.Limitations)
	}
	for _, rules := range []project.IgnoreRules{{JSONPointers: []string{"bad"}}, {JSONPointers: []string{"/~2bad"}}, {Headers: []string{"bad\nheader"}}, {JSONPointers: make([]string, 101)}} {
		if _, e = project.CompareRuns(root, a, b, &rules); e == nil {
			t.Fatal("invalid rules accepted")
		}
		c.ComparisonIgnore = rules
		if _, e = project.SaveCase(root, c.Case, c.Revision, false); e == nil {
			t.Fatal("invalid saved rules accepted")
		}
	}
	for _, pair := range [][2]string{{"missing", b}, {a, "missing"}} {
		_, e = project.CompareRuns(root, pair[0], pair[1], nil)
		var pe *project.Error
		if !errors.As(e, &pe) || pe.Code != "not_found" {
			t.Fatalf("missing run: %v", e)
		}
	}
	adHoc, _ := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: "/before"}, project.SendOptions{})
	if _, e = project.CompareRuns(root, a, adHoc.RunID, nil); e == nil {
		t.Fatal("compared ad hoc run")
	}
	other, e := project.DuplicateCase(root, c.ID, "other", false)
	if e != nil {
		t.Fatal(e)
	}
	different, e := project.SendCase(context.Background(), root, "staging", other.ID, map[string]string{"path": "before"}, project.SendOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = project.CompareRuns(root, a, different.RunID, nil); e == nil {
		t.Fatal("compared different cases")
	}
	if e = project.SetHistoryLimits(root, project.HistoryLimits{}); e != nil {
		t.Fatal(e)
	}
	if _, e = project.CompareRuns(root, a, b, nil); e == nil {
		t.Fatal("compared evicted evidence")
	}
}
