package tui_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
)

type tuiCredentialStore struct {
	unavailable bool
	values      map[string]string
}

func (s *tuiCredentialStore) Set(k, v string) error {
	if s.unavailable {
		return errors.New("unavailable store")
	}
	s.values[k] = v
	return nil
}
func (s *tuiCredentialStore) Get(k string) (string, error) { return s.values[k], nil }
func (s *tuiCredentialStore) Delete(k string) error        { delete(s.values, k); return nil }

func TestMaskedCredentialEntryReplacementDeletionAndSessionChoice(t *testing.T) {
	for _, session := range []bool{false, true} {
		t.Run(map[bool]string{false: "OS-store", true: "unavailable-session"}[session], func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			root := t.TempDir()
			s, _ := project.Init(root, "Credentials", false)
			e, _ := s.Resolve("staging")
			e.BaseURLs["api"] = "https://api.example.test"
			project.SaveEnvironment(root, e.Environment, e.Revision, false, false)
			store := &tuiCredentialStore{unavailable: session, values: map[string]string{}}
			vault := project.NewCredentials(store)
			m, err := tui.NewWithCredentials(root, tui.KeyConfig{}, vault)
			if err != nil {
				t.Fatal(err)
			}
			m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			enter := func(value string) {
				for _, k := range []string{"space", "e", "k"} {
					key(m, k)
				}
				for i, text := range []string{"login", "bearer", "api", "https://api.example.test/", "header", "Authorization", value} {
					m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
					m.Update(tea.KeyPressMsg{Text: text})
					if i == 6 {
						view := m.View().Content
						if strings.Contains(view, value) || !strings.Contains(view, "****") {
							t.Fatalf("unmasked entry: %s", view)
						}
					}
					key(m, "enter")
				}
			}
			enter("entry-private-value")
			if session {
				if !strings.Contains(m.View().Content, "session-only") {
					t.Fatal("no explicit session choice")
				}
				states, _ := vault.Inspect(root, "staging")
				if len(states) != 0 {
					t.Fatal("unavailable OS save silently fell back")
				}
				key(m, "y")
			}
			states, _ := vault.Inspect(root, "staging")
			if len(states) != 1 || !states[0].Configured {
				t.Fatal("credential not saved")
			}
			review, _ := project.ReviewAccess(root, "staging")
			project.ApproveAccess(root, review)
			enter("replacement-private-value")
			if session {
				key(m, "y")
			}
			if _, err := project.CheckCLIAccess(root, "staging", "api", ""); err == nil {
				t.Fatal("replacement retained CLI grant")
			}
			for _, k := range []string{"space", "e", "i"} {
				key(m, k)
			}
			if strings.Contains(m.View().Content, "private-value") || !strings.Contains(m.View().Content, "configured") {
				t.Fatal("unsafe inspection")
			}
			key(m, "esc")
			for _, k := range []string{"space", "e", "d"} {
				key(m, k)
			}
			m.Update(tea.KeyPressMsg{Text: "login"})
			key(m, "enter")
			key(m, "y")
			states, _ = vault.Inspect(root, "staging")
			if len(states) != 0 || len(store.values) != 0 {
				t.Fatal("credential not deleted")
			}
		})
	}
}
