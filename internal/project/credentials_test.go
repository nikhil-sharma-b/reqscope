package project_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

type credentialStore struct{ values map[string]string }

func (s *credentialStore) Set(key, value string) error    { s.values[key] = value; return nil }
func (s *credentialStore) Get(key string) (string, error) { return s.values[key], nil }
func (s *credentialStore) Delete(key string) error        { delete(s.values, key); return nil }

func TestProtectedCredentialSendAndSanitize(t *testing.T) {
	const secret = "private-token-123"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("credential was not attached")
		}
		w.Header().Set("X-Echo", secret)
		w.Header().Set("Set-Cookie", "unknown-session=value")
		io.WriteString(w, `{"token":"unknown-token","echo":"`+secret+`"}`)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	vault := project.NewCredentials(&credentialStore{values: map[string]string{}})
	binding := project.CredentialBinding{Name: "login", Kind: "bearer", Module: "api", Destination: server.URL + "/", In: "header", Field: "Authorization"}
	if err := vault.Save(root, "staging", binding, secret, false); err != nil {
		t.Fatal(err)
	}
	d := project.RequestDraft{Module: "api", Method: "GET", URL: "/", Headers: []project.RequestField{{Name: "Authorization", Value: "{{secret:login}}"}}}
	if _, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{Credentials: vault}); err == nil {
		t.Fatal("binding change did not revoke grant")
	}
	review, err := project.ReviewAccess(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	result, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{Credentials: vault})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(result)
	for _, s := range []string{secret, "unknown-token", "unknown-session"} {
		if strings.Contains(string(b), s) {
			t.Fatalf("public output leaked %s: %s", s, b)
		}
	}
}

func TestCredentialBindingCannotBeMovedOrUsedInDestination(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	vault := project.NewCredentials(&credentialStore{values: map[string]string{}})
	binding := project.CredentialBinding{Name: "key", Kind: "api-key", Module: "api", Destination: server.URL + "/allowed", In: "header", Field: "X-API-Key"}
	if err := vault.Save(root, "staging", binding, "opaque-value", false); err != nil {
		t.Fatal(err)
	}
	r, _ := project.ReviewAccess(root, "staging")
	if err := project.ApproveAccess(root, r); err != nil {
		t.Fatal(err)
	}
	for _, d := range []project.RequestDraft{
		{Module: "api", Method: "GET", URL: "/other", Headers: []project.RequestField{{Name: "X-API-Key", Value: "{{secret:key}}"}}},
		{Module: "api", Method: "GET", URL: "/allowed", Headers: []project.RequestField{{Name: "X-Changed", Value: "{{secret:key}}"}}},
		{Module: "api", Method: "GET", URL: "/allowed", Query: []project.RequestField{{Name: "key", Value: "{{secret:key}}"}}},
		{Module: "api", Method: "GET", URL: "/{{secret:key}}"},
		{Module: "api", Method: "GET", URL: "/{id}", Path: map[string]string{"id": "{{secret:key}}"}},
		{Module: "api", Method: "GET", URL: "http://{{secret:key}}/allowed"},
		{Module: "api", Method: "POST", URL: "/allowed", Body: "{{secret:key}}"},
		{Module: "api", Method: "GET", URL: server.URL + "/other", Headers: []project.RequestField{{Name: "X-API-Key", Value: "{{secret:key}}"}}},
	} {
		if _, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{Credentials: vault}); err == nil {
			t.Fatalf("binding bypass accepted %+v", d)
		}
	}
	if calls != 0 {
		t.Fatal("unauthorized request contacted server")
	}
}

func TestCredentialsLifecycleAndSessionOnly(t *testing.T) {
	root := requestWorkspace(t, "https://api.example.test")
	store := &credentialStore{values: map[string]string{}}
	vault := project.NewCredentials(store)
	b := project.CredentialBinding{Name: "login", Kind: "basic", Module: "api", Destination: "https://api.example.test/", In: "header", Field: "Authorization"}
	if err := vault.Save(root, "staging", b, "user:password-one", false); err != nil {
		t.Fatal(err)
	}
	reopened := project.NewCredentials(store)
	states, err := reopened.Inspect(root, "staging")
	if err != nil || len(states) != 1 || !states[0].Configured {
		t.Fatalf("persistent state %+v %v", states, err)
	}
	if err := reopened.Save(root, "staging", b, "user:password-two", false); err != nil {
		t.Fatal(err)
	}
	if len(store.values) != 1 {
		t.Fatal("replacement retained old credential")
	}
	if err := reopened.Delete(root, "staging", "login"); err != nil {
		t.Fatal(err)
	}
	states, err = reopened.Inspect(root, "staging")
	if err != nil || len(states) != 0 || len(store.values) != 0 {
		t.Fatal("deletion did not remove credential")
	}
	if err := vault.Save(root, "staging", b, "user:session-password", true); err != nil {
		t.Fatal(err)
	}
	states, _ = vault.Inspect(root, "staging")
	if !states[0].Configured {
		t.Fatal("session credential missing")
	}
	states, _ = reopened.Inspect(root, "staging")
	if states[0].Configured {
		t.Fatal("session credential survived process lifetime")
	}
	if len(store.values) != 0 {
		t.Fatal("session credential reached OS store")
	}
}

func TestSecretQueryAndBodyAndBasicBytes(t *testing.T) {
	for _, placement := range []string{"query", "body", "header"} {
		t.Run(placement, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch placement {
				case "query":
					if r.URL.Query().Get("api_key") != "a+b&c" {
						t.Error("query value changed")
					}
				case "body":
					var body map[string]string
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["credential"] != "a+b&c" {
						t.Errorf("body %+v %v", body, err)
					}
				case "header":
					if r.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" {
						t.Error("basic encoding incorrect")
					}
				}
				io.WriteString(w, "a+b&c")
			}))
			defer server.Close()
			root := requestWorkspace(t, server.URL)
			vault := project.NewCredentials(&credentialStore{values: map[string]string{}})
			b := project.CredentialBinding{Name: "auth", Kind: "api-key", Module: "api", Destination: server.URL + "/", In: placement}
			d := project.RequestDraft{Module: "api", Method: "POST", URL: "/"}
			value := "a+b&c"
			switch placement {
			case "query":
				b.Field = "api_key"
				d.Query = []project.RequestField{{Name: "api_key", Value: "{{secret:auth}}"}}
			case "body":
				d.BodyType = "json"
				d.Body = `{"credential":"{{secret:auth}}"}`
			case "header":
				b.Kind = "basic"
				b.Field = "Authorization"
				value = "user:pass"
				d.Headers = []project.RequestField{{Name: "Authorization", Value: "{{secret:auth}}"}}
			}
			if err := vault.Save(root, "staging", b, value, false); err != nil {
				t.Fatal(err)
			}
			r, _ := project.ReviewAccess(root, "staging")
			project.ApproveAccess(root, r)
			result, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{Credentials: vault})
			if err != nil {
				t.Fatal(err)
			}
			if placement != "header" && strings.Contains(result.Body, value) {
				t.Fatal("response leak")
			}
		})
	}
}

func TestCredentialRedirectCannotEscapePathOrOrigin(t *testing.T) {
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect destination contacted") }))
	defer denied.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/outside" {
			http.Redirect(w, r, denied.URL, 302)
		} else if r.URL.Path == "/same" {
			http.Redirect(w, r, "/unbound", 307)
		} else {
			t.Error("unbound redirect contacted")
		}
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	vault := project.NewCredentials(&credentialStore{values: map[string]string{}})
	for _, target := range []string{"/outside", "/same"} {
		b := project.CredentialBinding{Name: "key", Kind: "api-key", Module: "api", Destination: server.URL + target, In: "header", Field: "X-API-Key"}
		if err := vault.Save(root, "staging", b, "private-redirect-value", false); err != nil {
			t.Fatal(err)
		}
		r, _ := project.ReviewAccess(root, "staging")
		project.ApproveAccess(root, r)
		_, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: target, Headers: []project.RequestField{{Name: "X-API-Key", Value: "{{secret:key}}"}}}, project.SendOptions{Credentials: vault})
		if err == nil || strings.Contains(err.Error(), "private-redirect-value") {
			t.Fatalf("redirect error %v", err)
		}
	}
}

func TestJSONCredentialValuePreservesEscaping(t *testing.T) {
	const value = `quoted"credential\value`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["credential"] != value {
			t.Errorf("invalid expanded JSON: %+v %v", body, err)
		}
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	vault := project.NewCredentials(&credentialStore{values: map[string]string{}})
	b := project.CredentialBinding{Name: "key", Kind: "api-key", Module: "api", Destination: server.URL + "/", In: "body"}
	if err := vault.Save(root, "staging", b, value, false); err != nil {
		t.Fatal(err)
	}
	r, _ := project.ReviewAccess(root, "staging")
	project.ApproveAccess(root, r)
	_, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: "json", Body: `{"credential":"{{secret:key}}"}`}, project.SendOptions{Credentials: vault})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTruncatedAndFormAuthenticationPositionsAreSanitized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/form" {
			w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
			io.WriteString(w, "access_token=unregistered-form-token&message=ok")
		} else {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"access_token":"unregistered-truncated-token","padding":"`+strings.Repeat("x", 200)+`"}`)
		}
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	for _, target := range []string{"/json", "/form"} {
		result, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: target, MaxBodyBytes: 80}, project.SendOptions{})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(result)
		if strings.Contains(string(raw), "unregistered-") {
			t.Fatalf("authentication position leaked %s", raw)
		}
	}
}

func TestXMLAuthenticationPositionsAndTruncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		io.WriteString(w, `<response access_token="attribute-secret"><access_token>element-secret</access_token><message>visible</message></response>`)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	for _, capBytes := range []int64{1024, 100} {
		result, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: "/", MaxBodyBytes: capBytes}, project.SendOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(result.Body, "attribute-secret") || strings.Contains(result.Body, "element-secret") {
			t.Fatalf("XML authentication leaked %s", result.Body)
		}
		if capBytes == 1024 && !strings.Contains(result.Body, "visible") {
			t.Fatal("safe XML content lost")
		}
	}
}
