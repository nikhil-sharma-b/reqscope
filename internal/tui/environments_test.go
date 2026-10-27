package tui_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func TestEnvironmentEditSelectAndAllowCLI(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	project.Init(root, "Demo", false)
	m := newModel(t, root)
	press(m, "5")
	moveTo(t, m, "staging")
	press(m, "enter")
	wantScreen(t, m, "Edit staging", "Base URLs", "Variables")
	moveTo(t, m, "+ add")
	press(m, "enter")
	typeText(m, "api=https://staging.example.com/v1")
	press(m, "enter", "G", "enter")
	typeText(m, "ordinary=value")
	press(m, "enter", "ctrl+s")
	wantScreen(t, m, "Saved staging")
	s, _ := project.Load(root)
	e, _ := s.Resolve("staging")
	if e.BaseURLs["api"] != "https://staging.example.com/v1" || e.Variables["ordinary"] != "value" {
		t.Fatalf("%+v", e)
	}
	press(m, "u")
	wantScreen(t, m, "Requests now go to staging", "● staging")
	press(m, "a")
	wantScreen(t, m, "Allow the CLI to use staging?", "Exact origins", "https://staging.example.com")
	press(m, "esc")
	if _, err := project.CheckCLIAccess(root, "staging", "api", ""); err == nil {
		t.Fatal("cancelling granted access")
	}
	press(m, "a", "y")
	if _, err := project.CheckCLIAccess(root, "staging", "api", ""); err != nil {
		t.Fatal(err)
	}
	reopened := newModel(t, root)
	wantScreen(t, reopened, "● staging")
}

func TestEnvironmentStaysVisibleAtNarrowWidths(t *testing.T) {
	root := workspace(t, "https://staging.example.com/v1", "staging", false)
	m := newModel(t, root)
	for _, width := range []int{120, 80, 32} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		header := strings.SplitN(screen(m), "\n", 2)[0]
		if !strings.Contains(header, "staging") {
			t.Fatalf("width %d header lacks the environment: %q", width, header)
		}
	}
}

func TestEnvironmentDeleteAsksFirst(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	project.Init(root, "Delete", false)
	m := newModel(t, root, 60, 20)
	press(m, "5")
	moveTo(t, m, "staging")
	press(m, "d")
	wantScreen(t, m, "Delete staging?")
	press(m, "esc")
	if s, _ := project.Load(root); len(s.Environments) != 3 {
		t.Fatal("cancel deleted")
	}
	press(m, "d", "enter")
	if s, _ := project.Load(root); len(s.Environments) != 3 {
		t.Fatal("enter confirmed a delete; only y should")
	}
	press(m, "d", "y")
	s, _ := project.Load(root)
	if _, err := s.Resolve("staging"); err == nil {
		t.Fatal("not deleted")
	}
}

func TestLoweringProductionAsksFirst(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	project.Init(root, "Lower", false)
	m := newModel(t, root)
	press(m, "5")
	moveTo(t, m, "production")
	press(m, "enter")
	wantScreen(t, m, "Production", "yes, ask before every send")
	moveTo(t, m, "Production")
	press(m, "enter", "ctrl+s")
	wantScreen(t, m, "Stop treating production as production?")
	press(m, "esc")
	wantScreen(t, m, "Edit production")
	if s, _ := project.Load(root); s.Environments[1].Classification != "production" {
		t.Fatal("cancel lowered the classification")
	}
	press(m, "ctrl+s", "y")
	s, _ := project.Load(root)
	e, _ := s.Resolve("production")
	if e.Classification != "nonproduction" {
		t.Fatalf("%+v", e)
	}
}

func TestImportedAPIsNeedABaseURL(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	project.Init(root, "Pets", false)
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: Petstore, version: '1'}\npaths: {'/pets': {get: {responses: {'200': {description: OK}}}}}\n"), 0600)
	if _, err := project.ImportSpecification(root, file, false); err != nil {
		t.Fatal(err)
	}
	m := newModel(t, root)
	press(m, "e", "j", "enter", "enter")
	wantScreen(t, m, "No base URL for Petstore in staging", "Environments (5)")
	press(m, "5")
	moveTo(t, m, "staging")
	wantScreen(t, m, "Petstore  not set")
	press(m, "enter")
	moveTo(t, m, "Petstore")
	press(m, "enter")
	typeText(m, "https://pets.example.com")
	press(m, "enter", "ctrl+s", "1", "enter")
	wantScreen(t, m, "https://pets.example.com/pets")
}

func TestMaskedCredentialsReplaceDeleteAndSessionFallback(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "keychain", true: "session"}[unavailable], func(t *testing.T) {
			root := workspace(t, "https://api.example.test", "staging", false)
			store := &memoryStore{unavailable: unavailable, values: map[string]string{}}
			vault := project.NewCredentials(store)
			m := newCredentialModel(t, root, vault)
			add := func(value string, replace bool) {
				t.Helper()
				if replace {
					press(m, "enter")
				} else {
					press(m, "n")
					typeText(m, "login")
				}
				wantScreen(t, m, "Credential for staging")
				press(m, "tab", "tab")
				typeText(m, "api")
				press(m, "tab")
				typeText(m, "https://api.example.test/")
				press(m, "tab", "tab", "tab")
				typeText(m, value)
				wantScreen(t, m, "•••")
				rejectScreen(t, m, value)
				press(m, "enter")
				if unavailable {
					wantScreen(t, m, "Keychain unavailable")
					if states, _ := vault.Inspect(root, "staging"); len(states) != 0 && !replace {
						t.Fatal("an unavailable keychain silently fell back to memory")
					}
					press(m, "y")
				}
			}
			press(m, "5")
			moveTo(t, m, "staging")
			press(m, "c")
			wantScreen(t, m, "Credentials for staging", "No credentials yet")
			add("entry-private-value", false)
			if states, _ := vault.Inspect(root, "staging"); len(states) != 1 || !states[0].Configured {
				t.Fatalf("not saved: %+v", states)
			}
			wantScreen(t, m, "login", "stored")
			rejectScreen(t, m, "private-value")
			review, _ := project.ReviewAccess(root, "staging")
			project.ApproveAccess(root, review)
			add("replacement-private-value", true)
			if _, err := project.CheckCLIAccess(root, "staging", "api", ""); err == nil {
				t.Fatal("replacing a credential kept the CLI grant")
			}
			press(m, "d", "y")
			if states, _ := vault.Inspect(root, "staging"); len(states) != 0 || len(store.values) != 0 {
				t.Fatal("not deleted")
			}
		})
	}
}

func TestUnreadableKeychainShowsOnlyIdentitiesUntilRecovered(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s, _ := project.Init(root, "private-value-from-unreadable-store", false)
	for _, name := range []string{"staging", "production"} {
		e, _ := s.Resolve(name)
		e.BaseURLs["api"] = "https://example.test"
		project.SaveEnvironment(root, e.Environment, e.Revision, false, false)
	}
	store := &memoryStore{values: map[string]string{}}
	b := project.CredentialBinding{Name: "login", Kind: "bearer", Module: "api", Destination: "https://example.test/", In: "header", Field: "Authorization"}
	if err := project.NewCredentials(store).Save(root, "production", b, "private-value-from-unreadable-store", false); err != nil {
		t.Fatal(err)
	}
	store.unavailable = true
	vault := project.NewCredentials(store)
	m := newCredentialModel(t, root, vault)
	production, _ := s.Resolve("production")
	wantScreen(t, m, "cannot read your stored credentials", production.ID)
	rejectScreen(t, m, "private-value-from-unreadable-store")
	moveTo(t, m, production.ID)
	press(m, "u")
	wantScreen(t, m, "Requests go to "+production.ID)
	press(m, "c", "n")
	typeText(m, "login")
	press(m, "tab", "tab")
	typeText(m, "api")
	press(m, "tab")
	typeText(m, "https://example.test/")
	press(m, "tab", "tab", "tab")
	typeText(m, "session-replacement")
	press(m, "enter", "y")
	if states, err := vault.Inspect(root, "production"); err != nil || len(states) != 1 || !states[0].Configured {
		t.Fatalf("%+v %v", states, err)
	}
	press(m, "esc")
	wantScreen(t, m, "1 Endpoints")
}

func TestExternalChangesReloadCleanDraftsAndKeepEditedOnes(t *testing.T) {
	root := workspace(t, "https://api.example.test", "staging", false)
	c := saveCase(t, root, "Saved", project.RequestDraft{Module: "api", Method: "GET", URL: "/original", BodyType: "raw"})
	m := newModel(t, root)
	press(m, "2", "enter")
	c.Request.URL = "/external"
	external, err := project.SaveCase(root, c.Case, c.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	poll(t, m)
	wantScreen(t, m, "/external")
	rejectScreen(t, m, "changed on disk")

	press(m, "enter", "ctrl+u")
	typeText(m, "/local-draft")
	press(m, "enter")
	external.Request.URL = "/external-two"
	external, err = project.SaveCase(root, external.Case, external.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	poll(t, m)
	wantScreen(t, m, "/local-draft", "changed on disk")
	press(m, "ctrl+s")
	wantScreen(t, m, "Changed on disk", "/external-two", "/local-draft", "o keep mine", "t take theirs")
	if current, _ := project.GetCase(root, c.ID); current.Request.URL != "/external-two" {
		t.Fatal("saving overwrote the version on disk before asking")
	}
	press(m, "o")
	if current, _ := project.GetCase(root, c.ID); current.Request.URL != "/local-draft" {
		t.Fatal("keep mine did not save")
	}

	// Invalid outside edits never replace a usable draft.
	path := filepath.Join(root, "cases", c.ID+".yaml")
	os.WriteFile(path, []byte("invalid: ["), 0600)
	poll(t, m)
	wantScreen(t, m, "/local-draft")
	external.Request.URL = "/theirs"
	raw, _ := json.Marshal(external.Case)
	os.WriteFile(path, raw, 0600)
	press(m, "enter", "ctrl+u")
	typeText(m, "/mine")
	press(m, "enter")
	poll(t, m)
	press(m, "ctrl+s", "t")
	wantScreen(t, m, "/theirs", "Loaded the version on disk")
	press(m, "enter", "ctrl+u")
	typeText(m, "/discard-me")
	press(m, "enter", ":")
	typeText(m, "discard edits")
	press(m, "enter")
	wantScreen(t, m, "/theirs")
	rejectScreen(t, m, "/discard-me")
}
