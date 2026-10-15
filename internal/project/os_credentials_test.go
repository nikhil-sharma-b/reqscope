package project_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

// Opt in on an unlocked Linux Secret Service or macOS login keychain. The test
// creates one random Reqscope entry, uses it through the real adapter, and
// removes it. It never reads or modifies existing user credentials.
func TestOSCredentialIntegration(t *testing.T) {
	if os.Getenv("REQSCOPE_TEST_KEYRING") != "1" {
		t.Skip("set REQSCOPE_TEST_KEYRING=1 with an unlocked OS credential service")
	}
	token := uuid.NewString()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("OS credential not attached")
		}
		io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	vault := project.DefaultCredentials
	binding := project.CredentialBinding{Name: "platform-check", Kind: "bearer", Module: "api", Destination: server.URL + "/", In: "header", Field: "Authorization"}
	if err := vault.Save(root, "staging", binding, token, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := vault.Delete(root, "staging", binding.Name); err != nil {
			t.Error(err)
		}
	})
	review, err := project.ReviewAccess(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := project.ApproveAccess(root, review); err != nil {
		t.Fatal(err)
	}
	result, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "GET", URL: "/", Headers: []project.RequestField{{Name: "Authorization", Value: "{{secret:platform-check}}"}}}, project.SendOptions{Credentials: vault})
	if err != nil || result.Status != 200 {
		t.Fatal(result, err)
	}
}

type unavailableCredentialStore struct{}

func (unavailableCredentialStore) Set(string, string) error { return fmt.Errorf("service unavailable") }
func (unavailableCredentialStore) Get(string) (string, error) {
	return "", fmt.Errorf("service unavailable")
}
func (unavailableCredentialStore) Delete(string) error { return fmt.Errorf("service unavailable") }

func TestUnavailableCredentialServiceRequiresExplicitSessionChoice(t *testing.T) {
	root := requestWorkspace(t, "http://127.0.0.1:12345")
	vault := project.NewCredentials(unavailableCredentialStore{})
	binding := project.CredentialBinding{Name: "session", Kind: "bearer", Module: "api", Destination: "http://127.0.0.1:12345/", In: "header", Field: "Authorization"}
	if err := vault.Save(root, "staging", binding, "unavailable-service-test-value", false); err == nil {
		t.Fatal("persistent save silently fell back")
	}
	if err := vault.Save(root, "staging", binding, "unavailable-service-test-value", true); err != nil {
		t.Fatal(err)
	}
	states, err := vault.Inspect(root, "staging")
	if err != nil || len(states) != 1 || !states[0].Configured {
		t.Fatal(states, err)
	}
	restarted := project.NewCredentials(unavailableCredentialStore{})
	states, err = restarted.Inspect(root, "staging")
	if err != nil || len(states) != 1 || states[0].Configured {
		t.Fatal("session survived restart", states, err)
	}
}
