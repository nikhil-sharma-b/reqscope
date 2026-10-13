package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

type protectedStore struct {
	values      map[string]string
	unavailable bool
}

func (s *protectedStore) Set(k, v string) error {
	if s.unavailable {
		return errors.New("store error containing private-store-value")
	}
	s.values[k] = v
	return nil
}
func (s *protectedStore) Get(k string) (string, error) {
	if s.unavailable {
		return "", errors.New("store error containing private-store-value")
	}
	return s.values[k], nil
}
func (s *protectedStore) Delete(k string) error { delete(s.values, k); return nil }

func TestCLIProtectedOutputAndInspection(t *testing.T) {
	const value = `private"store\value`
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != value {
			t.Error("missing protected value")
		}
		b, _ := json.Marshal(map[string]string{"echo": value, "password": "unknown-password"})
		w.Write(b)
	}))
	defer server.Close()
	root := t.TempDir()
	snap, err := project.Init(root, "Protected", false)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := snap.Resolve("staging")
	e.BaseURLs["api"] = server.URL
	if err = project.SaveEnvironment(root, e.Environment, e.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	store := &protectedStore{values: map[string]string{}}
	vault := project.NewCredentials(store)
	b := project.CredentialBinding{Name: "key", Kind: "api-key", Module: "api", Destination: server.URL + "/", In: "header", Field: "X-API-Key"}
	if err = vault.Save(root, "staging", b, value, false); err != nil {
		t.Fatal(err)
	}
	review, _ := project.ReviewAccess(root, "staging")
	project.ApproveAccess(root, review)
	run := func(args []string, input string) (int, string, string) {
		var out, diag bytes.Buffer
		args = append(args, "--project", root, "--output", "json")
		code := cli.RunWithCredentials(args, strings.NewReader(input), &out, &diag, vault)
		raw := out.String() + diag.String()
		var decoded any
		json.Unmarshal(out.Bytes(), &decoded)
		decodedJSON, _ := json.Marshal(decoded)
		escaped, _ := json.Marshal(value)
		if strings.Contains(raw, string(escaped[1:len(escaped)-1])) || strings.Contains(raw, value) || strings.Contains(raw, "unknown-password") {
			t.Fatalf("secret leaked: %s %s", raw, decodedJSON)
		}
		return code, out.String(), diag.String()
	}
	code, out, _ := run([]string{"request", "send", "--env", "staging", "--input", "-"}, `{"module":"api","method":"GET","url":"/","headers":[{"name":"X-API-Key","value":"{{secret:key}}"}]}`)
	if code != 0 || !strings.Contains(out, "REDACTED") {
		t.Fatalf("%d %s", code, out)
	}
	for _, action := range []string{"list", "inspect"} {
		code, out, _ = run([]string{"secret", action, "--env", "staging"}, "")
		if code != 0 || !strings.Contains(out, `"configured":true`) || strings.Contains(out, "destination") || strings.Contains(out, "X-Api-Key") {
			t.Fatalf("inspection %d %s", code, out)
		}
	}
	for _, group := range []string{"secret", "access"} {
		for _, action := range []string{"get", "set", "replace", "delete", "bind", "approve"} {
			code, out, _ = run([]string{group, action, "--env", "staging"}, "")
			if code != 4 || !strings.Contains(out, "interactively") {
				t.Fatalf("write exposed %d %s", code, out)
			}
		}
	}
	code, out, _ = run([]string{"project", "inspect"}, "")
	if code != 0 || strings.Contains(out, "X-Api-Key") || strings.Contains(out, "grants") {
		t.Fatalf("project exports local state: %s", out)
	}
	// Input validation previews must sanitize values even when JSON escaping differs.
	code, out, _ = run([]string{"input-schema", "validate", "project.init", "--input", "-"}, `{"name":`+string(mustJSON(value))+`}`)
	if code != 0 || !strings.Contains(out, "REDACTED") {
		t.Fatalf("preview %d %s", code, out)
	}
	store.unavailable = true
	code, out, diag := run([]string{"secret", "inspect", "--env", "staging"}, "")
	if code != 0 || !strings.Contains(out, `"configured":false`) {
		t.Fatalf("missing state %d %s %s", code, out, diag)
	}
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestCLIUnavailableStoreHasNoPlaintextFallback(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, _ := project.Init(root, "Unavailable", false)
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = "https://example.test"
	project.SaveEnvironment(root, e.Environment, e.Revision, false, false)
	vault := project.NewCredentials(&protectedStore{values: map[string]string{}, unavailable: true})
	b := project.CredentialBinding{Name: "key", Kind: "bearer", Module: "api", Destination: "https://example.test/", In: "header", Field: "Authorization"}
	err := vault.Save(root, "staging", b, "private-store-value", false)
	if err == nil || strings.Contains(err.Error(), "private-store-value") || !strings.Contains(err.Error(), "session-only") {
		t.Fatalf("unsafe store failure %v", err)
	}
	states, _ := vault.Inspect(root, "staging")
	if len(states) != 0 {
		t.Fatal("failed OS save silently configured credential")
	}
	var out, diag bytes.Buffer
	cli.RunWithCredentials([]string{"unknown-private-store-value", "inspect", "--project", root, "--output", "json"}, strings.NewReader(""), &out, &diag, vault)
	if strings.Contains(out.String()+diag.String(), "private-store-value") {
		t.Fatal("stderr or stdout leaked known value")
	}
	if err = vault.Save(root, "staging", b, "private-store-value", true); err != nil {
		t.Fatal(err)
	}
}

func TestUnavailableExistingStoreKeepsTUIRecoveryReachable(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, _ := project.Init(root, "Recovery", false)
	e, _ := s.Resolve("staging")
	e.BaseURLs["api"] = "https://example.test"
	project.SaveEnvironment(root, e.Environment, e.Revision, false, false)
	store := &protectedStore{values: map[string]string{}}
	vault := project.NewCredentials(store)
	b := project.CredentialBinding{Name: "key", Kind: "bearer", Module: "api", Destination: "https://example.test/", In: "header", Field: "Authorization"}
	if err := vault.Save(root, "staging", b, "private-store-value", false); err != nil {
		t.Fatal(err)
	}
	store.unavailable = true
	var out, diag bytes.Buffer
	code := cli.RunWithCredentials([]string{"tui", "--project", root, "--output", "json"}, strings.NewReader(""), &out, &diag, vault)
	if code != 2 || !strings.Contains(out.String(), "requires terminal") {
		t.Fatalf("TUI dispatcher blocked recovery: %d %s %s", code, &out, &diag)
	}
}
