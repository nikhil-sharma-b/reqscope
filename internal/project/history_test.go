package project_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func TestHistoryReopensSanitizedEvidenceAndProtectsPins(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=history-cookie-secret")
		w.Write([]byte(`{"token":"history-body-secret","ok":true}`))
	}))
	defer server.Close()
	// macOS keeps user data under the home folder rather than XDG_DATA_HOME,
	// so a temporary home keeps the test's history out of the real one.
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := requestWorkspace(t, server.URL)
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "history", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/"}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := project.SendCase(context.Background(), root, "staging", c.ID, nil, project.SendOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first.RunID == "" {
		t.Fatal("missing immutable run ID")
	}
	run, err := project.GetRun(root, first.RunID)
	if err != nil || run.CaseID != c.ID || run.Response.Status != 200 || run.InputDigest == "" || run.Outcome != "completed" {
		t.Fatalf("%+v %v", run, err)
	}
	if strings.Contains(run.Response.Body, "history-body-secret") {
		t.Fatal(run)
	}
	if err = project.ChangeRun(root, first.RunID, "pin", ""); err != nil {
		t.Fatal(err)
	}
	if err = project.ChangeRun(root, first.RunID, "delete", ""); err == nil {
		t.Fatal("deleted pin")
	}
	if err = project.SetHistoryLimits(root, project.HistoryLimits{PerCase: 1, MaxBytes: 100000}); err != nil {
		t.Fatal(err)
	}
	var latest project.Exchange
	for i := 0; i < 3; i++ {
		latest, err = project.SendCase(context.Background(), root, "staging", c.ID, nil, project.SendOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := project.History(root, c.ID, 1, 0)
	if err != nil || page.Total != 2 || page.NextOffset == nil || page.Items[0].ID != latest.RunID || page.Items[0].Response.Body != "" {
		t.Fatalf("%+v %v", page, err)
	}
	if err = project.SetHistoryLimits(root, project.HistoryLimits{}); err != nil {
		t.Fatal(err)
	}
	page, err = project.History(root, "", 50, 0)
	if err != nil || page.Total != 1 || !page.PinnedOverLimit {
		t.Fatalf("%+v %v", page, err)
	}
	if err = project.PruneHistory(root); err != nil {
		t.Fatal(err)
	}
	data := os.Getenv("XDG_DATA_HOME")
	if runtime.GOOS == "darwin" {
		data = filepath.Join(home, "Library", "Application Support")
	}
	dbs, _ := filepath.Glob(filepath.Join(data, "reqscope", "history", "*.sqlite"))
	if len(dbs) != 1 {
		t.Fatal(dbs)
	}
	raw, err := os.ReadFile(dbs[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "SQLite format 3") {
		t.Fatal("not SQLite")
	}
	for _, secret := range []string{"history-cookie-secret", "history-body-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("SQLite contains %s", secret)
		}
	}
	if err = project.ChangeRun(root, first.RunID, "unpin", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = project.GetRun(root, first.RunID); err == nil {
		t.Fatal("zero retention failed after unpin")
	}
}

func TestHistoryRetainsFailedAdHocRunAndAssociation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("done")) }))
	root := requestWorkspace(t, server.URL)
	server.Close()
	result, sendErr := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: "/"}, project.SendOptions{})
	if sendErr == nil {
		t.Fatal("expected transport failure")
	}
	r, err := project.GetRun(root, result.RunID)
	if err != nil || r.Outcome != "failed" || r.Error == "" || r.Request.Method != "GET" {
		t.Fatalf("%+v %v", r, err)
	}
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "associated", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/"}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = project.ChangeRun(root, r.ID, "associate", c.ID); err != nil {
		t.Fatal(err)
	}
	r, err = project.GetRun(root, r.ID)
	if err != nil || r.CaseID != c.ID {
		t.Fatalf("%+v %v", r, err)
	}
	if err = project.ChangeRun(root, r.ID, "associate", c.ID); err == nil {
		t.Fatal("reassociated immutable case identity")
	}
}

func TestHistoryCleanupCannotDeleteProtectedCredentials(t *testing.T) {
	const secret = "history-protected-credential"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(secret)) }))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	store := &credentialStore{values: map[string]string{}}
	vault := project.NewCredentials(store)
	binding := project.CredentialBinding{Name: "login", Kind: "bearer", Module: "api", Destination: server.URL + "/", In: "header", Field: "Authorization"}
	if err := vault.Save(root, "staging", binding, secret, false); err != nil {
		t.Fatal(err)
	}
	review, err := project.ReviewAccess(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err = project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	d := project.RequestDraft{Module: "api", Method: "GET", URL: "/", Headers: []project.RequestField{{Name: "Authorization", Value: "{{secret:login}}"}}}
	ex, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{Credentials: vault})
	if err != nil {
		t.Fatal(err)
	}
	r, err := project.GetRun(root, ex.RunID)
	if err != nil || strings.Contains(r.Response.Body, secret) || strings.Contains(r.Request.Headers.Get("Authorization"), secret) {
		t.Fatalf("%+v %v", r, err)
	}
	if err = project.SetHistoryLimits(root, project.HistoryLimits{}); err != nil {
		t.Fatal(err)
	}
	if err = project.PruneHistory(root); err != nil {
		t.Fatal(err)
	}
	if len(store.values) != 1 {
		t.Fatal("cleanup deleted protected credential")
	}
	dbs, _ := filepath.Glob(filepath.Join(os.Getenv("XDG_DATA_HOME"), "reqscope", "history", "*"))
	for _, path := range dbs {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), secret) {
			t.Fatal("SQLite artifact leaked credential")
		}
	}
	if _, err = project.Send(context.Background(), root, "staging", d, project.SendOptions{Credentials: vault}); err != nil {
		t.Fatal("credential no longer usable", err)
	}
}

func TestHistoryPreservesEvidenceAfterIncompleteResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.Header().Set("X-Evidence", "received")
		w.WriteHeader(202)
		w.Write([]byte("partial"))
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	ex, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: "/"}, project.SendOptions{})
	if err == nil {
		t.Fatal("incomplete body should fail")
	}
	r, err := project.GetRun(root, ex.RunID)
	if err != nil || r.Response.Status != 202 || r.Response.Headers.Get("X-Evidence") != "received" || r.Response.RetainedBytes != 7 || !r.Response.BodyWithheld || r.Outcome != "failed" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestHistoryDefaultsEvictOldestAndKeepBinaryMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte{0, 1, 2, 3})
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	c, err := project.SaveCase(root, project.Case{ID: uuid.NewString(), Name: "retention", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/"}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	var first, last project.Exchange
	for i := 0; i < 101; i++ {
		last, err = project.SendCase(context.Background(), root, "staging", c.ID, nil, project.SendOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = last
		}
	}
	page, err := project.History(root, c.ID, 100, 0)
	if err != nil || page.Total != 100 || page.Limits.PerCase != 100 || page.Limits.MaxBytes != 100<<20 || page.Items[0].ID != last.RunID {
		t.Fatalf("%+v %v", page, err)
	}
	if _, err = project.GetRun(root, first.RunID); err == nil {
		t.Fatal("oldest run survived default limit")
	}
	r, err := project.GetRun(root, last.RunID)
	if err != nil || !r.Response.Binary || r.Response.RetainedBytes != 4 || r.Response.Body != "" {
		t.Fatalf("%+v %v", r, err)
	}
}
