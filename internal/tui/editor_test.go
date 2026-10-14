package tui_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
)

// Run the command through Bubble Tea itself, which owns terminal suspension.
// Stop after the editor outcome reaches the model, without inspecting commands.
type editorHarness struct {
	*tui.Model
	launch tea.Cmd
}

func (h *editorHarness) Init() tea.Cmd { return h.launch }
func (h *editorHarness) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		msg = tea.WindowSizeMsg{Width: 140, Height: 40}
	}
	_, cmd := h.Model.Update(msg)
	view := h.View().Content
	for _, outcome := range []string{"Edited body retained", "Invalid JSON body", "Editor cancelled or failed", "Editor closed without changes"} {
		if strings.Contains(view, outcome) {
			return h, tea.Quit
		}
	}
	return h, cmd
}

func TestExternalEditorRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		name, script, body, outcome string
		changed                     bool
	}{
		{"success", `printf '%s' '{"edited":true}' > "$1"`, `{"edited":true}`, "Edited body retained", true},
		{"invalid JSON", `printf '%s' '{' > "$1"`, `{`, "Invalid JSON body", true},
		{"unchanged cancellation", `exit 0`, `{"token":"{{secret:login}}"}`, "Editor closed without changes", false},
		{"failure after write", `printf changed > "$1"; exit 7`, `{"token":"{{secret:login}}"}`, "Editor cancelled or failed", false},
		{"signal cancellation", `printf changed > "$1"; kill -INT $$`, `{"token":"{{secret:login}}"}`, "Editor cancelled or failed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			scratch := t.TempDir()
			t.Setenv("TMPDIR", scratch)
			root := t.TempDir()
			snapshot, _ := project.Init(root, "Editor", false)
			env, _ := snapshot.Resolve("staging")
			env.BaseURLs["api"] = "https://api.example.test"
			if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
				t.Fatal(err)
			}
			vault := project.NewCredentials(&tuiCredentialStore{values: map[string]string{}})
			if err := vault.Save(root, "staging", project.CredentialBinding{Name: "login", Kind: "bearer", Module: "api", Destination: "https://api.example.test/", In: "header", Field: "Authorization"}, "editor-private-token", true); err != nil {
				t.Fatal(err)
			}
			draft := project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: "json", Body: `{"token":"{{secret:login}}"}`}
			saved, err := project.SaveCase(root, project.Case{ID: "bb6789be-1526-4422-9e98-540c7f848ae1", Name: "Editor case", Request: draft}, "", false)
			if err != nil {
				t.Fatal(err)
			}
			m, err := tui.NewWithCredentials(root, tui.KeyConfig{}, vault)
			if err != nil {
				t.Fatal(err)
			}
			m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
			for _, k := range []string{"space", "c", "l", "enter"} {
				key(m, k)
			}
			script := filepath.Join(t.TempDir(), "editor.sh")
			captured := filepath.Join(t.TempDir(), "captured")
			t.Setenv("EDITOR_CAPTURE", captured)
			scriptText := "#!/bin/sh\ncat \"$1\" > \"$EDITOR_CAPTURE\"\n" + tc.script + "\n"
			if err := os.WriteFile(script, []byte(scriptText), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("VISUAL", "sh "+script)
			t.Setenv("EDITOR", "does-not-exist")
			for _, k := range []string{"space", "r"} {
				key(m, k)
			}
			_, launch := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
			if launch == nil {
				t.Fatal(m.View().Content)
			}
			runEditor(t, m, launch)
			content, err := os.ReadFile(captured)
			if err != nil || string(content) != draft.Body || strings.Contains(string(content), "editor-private-token") {
				t.Fatalf("scratch content %q, %v", content, err)
			}
			entries, err := os.ReadDir(scratch)
			if err != nil || len(entries) != 0 {
				t.Fatalf("scratch not cleaned: %v %v", entries, err)
			}
			view := m.View().Content
			if !strings.Contains(view, tc.outcome) || !strings.Contains(view, "NORMAL") || !strings.Contains(view, "Editor case") {
				t.Fatal(view)
			}
			key(m, "q")
			if strings.Contains(m.View().Content, "Unsaved request/case changes") != tc.changed {
				t.Fatal(m.View().Content)
			}
			if tc.changed {
				key(m, "c")
			}
			// Saving and reading the case confirms the body and case identity survived.
			for _, k := range []string{"space", "c", "s"} {
				key(m, k)
			}
			current, err := project.GetCase(root, saved.ID)
			if err != nil || current.Request.Body != tc.body {
				t.Fatalf("body %q, %v", current.Request.Body, err)
			}
			for _, k := range []string{"space", "r", "e"} {
				key(m, k)
			}
			if !strings.Contains(m.View().Content, "INSERT") {
				t.Fatal(m.View().Content)
			}
			key(m, "esc")
			if !strings.Contains(m.View().Content, "NORMAL") {
				t.Fatal(m.View().Content)
			}
			if tc.name == "invalid JSON" {
				for _, k := range []string{"space", "r"} {
					key(m, k)
				}
				_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
				if cmd != nil {
					t.Fatal("invalid body scheduled a send")
				}
			}
		})
	}
}
func runEditor(t *testing.T, m *tui.Model, launch tea.Cmd) {
	t.Helper()
	p := tea.NewProgram(&editorHarness{Model: m, launch: launch}, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	timer := time.AfterFunc(5*time.Second, p.Kill)
	defer timer.Stop()
	if _, err := p.Run(); err != nil {
		t.Fatalf("%v: %s", err, m.View().Content)
	}
}

func TestExternalEditorStartFailure(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	root := t.TempDir()
	project.Init(root, "Editor", false)
	m, _ := tui.New(root, tui.KeyConfig{})
	for _, k := range []string{"space", "r", "n"} {
		key(m, k)
	}
	b, _ := json.Marshal(project.RequestDraft{Method: "POST", URL: "https://example.test", BodyType: "raw", Body: "original"})
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: string(b)})
	enter(m)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "reqscope-editor-does-not-exist")
	for _, k := range []string{"space", "r"} {
		key(m, k)
	}
	_, launch := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	runEditor(t, m, launch)
	if !strings.Contains(m.View().Content, "original") {
		t.Fatal(m.View().Content)
	}
	entries, _ := os.ReadDir(scratch)
	if len(entries) != 0 {
		t.Fatal(entries)
	}
}

func TestExternalEditorRejectsRegisteredCredentialBeforeExport(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	root := t.TempDir()
	snapshot, _ := project.Init(root, "Editor", false)
	env, _ := snapshot.Resolve("staging")
	env.BaseURLs["api"] = "https://api.example.test"
	if err := project.SaveEnvironment(root, env.Environment, env.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	vault := project.NewCredentials(&tuiCredentialStore{values: map[string]string{}})
	if err := vault.Save(root, "staging", project.CredentialBinding{Name: "login", Kind: "bearer", Module: "api", Destination: "https://api.example.test/", In: "header", Field: "Authorization"}, "never-export-this", true); err != nil {
		t.Fatal(err)
	}
	m, err := tui.NewWithCredentials(root, tui.KeyConfig{}, vault)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	for _, k := range []string{"space", "r", "n"} {
		key(m, k)
	}
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Text: `{"method":"POST","url":"/","bodyType":"raw","body":"never-export-this"}`})
	enter(m)
	for _, k := range []string{"space", "r"} {
		key(m, k)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd != nil || !strings.Contains(m.View().Content, "registered credential") {
		t.Fatal(m.View().Content)
	}
	entries, _ := os.ReadDir(scratch)
	if len(entries) != 0 {
		t.Fatal(entries)
	}
}
