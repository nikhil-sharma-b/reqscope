package tui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
)

func editorWorkspace(t *testing.T, body string) (string, project.CaseResource, *project.Credentials, *tui.Model) {
	t.Helper()
	root := workspace(t, "https://api.example.test", "staging", false)
	vault := project.NewCredentials(&memoryStore{values: map[string]string{}})
	if err := vault.Save(root, "staging", project.CredentialBinding{Name: "login", Kind: "bearer", Module: "api", Destination: "https://api.example.test/", In: "header", Field: "Authorization"}, "editor-private-token", true); err != nil {
		t.Fatal(err)
	}
	c := saveCase(t, root, "Editor case", project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: "json", Body: body})
	m := newCredentialModel(t, root, vault)
	press(m, "2", "enter")
	return root, c, vault, m
}

func TestBodyEditorRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		name, script, body, outcome string
		changed                     bool
	}{
		{"success", `printf '%s' '{"edited":true}' > "$1"`, `{"edited":true}`, "Body updated", true},
		{"invalid JSON", `printf '%s' '{' > "$1"`, `{`, "not valid JSON", true},
		{"no changes", `exit 0`, `{"token":"{{secret:login}}"}`, "No changes to the body", false},
		{"failure after write", `printf changed > "$1"; exit 7`, `{"token":"{{secret:login}}"}`, "closed with an error", false},
		{"signal", `printf changed > "$1"; kill -INT $$`, `{"token":"{{secret:login}}"}`, "closed with an error", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scratch := t.TempDir()
			t.Setenv("TMPDIR", scratch)
			root, c, _, m := editorWorkspace(t, `{"token":"{{secret:login}}"}`)
			captured := filepath.Join(t.TempDir(), "captured")
			editorScript(t, `cat "$1" > "`+captured+`"`+"\n"+tc.script)
			runEditor(t, m, send(m, "b"))
			content, err := os.ReadFile(captured)
			if err != nil || string(content) != c.Request.Body || strings.Contains(string(content), "editor-private-token") {
				t.Fatalf("editor saw %q, %v", content, err)
			}
			if entries, _ := os.ReadDir(scratch); len(entries) != 0 {
				t.Fatalf("scratch files left behind: %v", entries)
			}
			wantScreen(t, m, tc.outcome, "Editor case")
			if cmd := send(m, "q"); (cmd == nil) != tc.changed {
				t.Fatalf("quit protection wrong for changed=%v:\n%s", tc.changed, screen(m))
			}
			press(m, "esc", "ctrl+s")
			if current, _ := project.GetCase(root, c.ID); current.Request.Body != tc.body {
				t.Fatalf("saved body %q", current.Request.Body)
			}
			if tc.name == "invalid JSON" {
				press(m, "s")
				wantScreen(t, m, "invalid JSON body")
			}
		})
	}
}

func TestEditorThatCannotStartLeavesTheBody(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	_, _, _, m := editorWorkspace(t, `"original"`)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "reqscope-editor-does-not-exist")
	runEditor(t, m, send(m, "b"))
	wantScreen(t, m, `"original"`, "closed with an error")
	if entries, _ := os.ReadDir(scratch); len(entries) != 0 {
		t.Fatal(entries)
	}
}

func TestBodyWithAStoredCredentialNeverLeavesTheApp(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	_, _, _, m := editorWorkspace(t, `"editor-private-token"`)
	if cmd := send(m, "b"); cmd != nil {
		t.Fatal("exported a stored credential to an editor")
	}
	wantScreen(t, m, "stored credential")
	if entries, _ := os.ReadDir(scratch); len(entries) != 0 {
		t.Fatal(entries)
	}
}

func TestWholeRequestEditsAsJSON(t *testing.T) {
	_, _, _, m := editorWorkspace(t, `{}`)
	editorScript(t, `printf '%s' '{"module":"api","method":"PATCH","url":"/raw","bodyType":"raw","body":"","timeoutMs":5000}' > "$1"`)
	runEditor(t, m, send(m, "E"))
	wantScreen(t, m, "Request updated", "PATCH", "/raw")
	editorScript(t, `printf '%s' '{"unknown":true}' > "$1"`)
	runEditor(t, m, send(m, "E"))
	wantScreen(t, m, "Not saved", "unknown", "e edit again")
}

func TestSavedRequestDetailsValidateChecks(t *testing.T) {
	root := workspace(t, "https://api.example.test", "staging", false)
	c := project.Case{ID: uuid.NewString(), Name: "Checks", Request: project.RequestDraft{Module: "api", Method: "GET", URL: "/"}}
	if _, err := project.SaveCase(root, c, "", false); err != nil {
		t.Fatal(err)
	}
	invalid := "id: " + c.ID + "\nname: Checks\nrequest: {module: api, method: GET, url: /, body: '', bodyType: ''}\nvalidation:\n  assertions:\n    - {kind: status-range, min: 300, max: 200}\n"
	valid := "id: " + c.ID + "\nname: Checked\nrequest: {module: api, method: GET, url: /, body: '', bodyType: ''}\nvalidation:\n  responseSchema: true\n  assertions:\n    - {kind: status-equals, value: 200}\n    - {kind: header-present, name: x-test}\n"
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte(invalid), 0600)
	os.WriteFile(filepath.Join(dir, "valid.yaml"), []byte(valid), 0600)
	m := newModel(t, root)
	press(m, "2")
	editorScript(t, `cp "`+filepath.Join(dir, "invalid.yaml")+`" "$1"`)
	runEditor(t, m, send(m, "E"))
	wantScreen(t, m, "Not saved")
	if saved, _ := project.GetCase(root, c.ID); len(saved.Validation) != 0 {
		t.Fatal("invalid checks saved")
	}
	editorScript(t, `cp "`+filepath.Join(dir, "valid.yaml")+`" "$1"`)
	runEditor(t, m, send(m, "e"))
	saved, _ := project.GetCase(root, c.ID)
	if saved.Name != "Checked" || len(saved.Validation) != 2 {
		t.Fatalf("%+v\n%s", saved, screen(m))
	}
}

func TestEmptyBodyStartsFromTheContractExample(t *testing.T) {
	root := workspace(t, "https://api.example.test", "staging", false)
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: API, version: '1'}\npaths:\n  /pets:\n    post:\n      requestBody: {required: true, content: {application/json: {example: {name: Rex}}}}\n      responses: {'200': {description: OK}}\n"), 0600)
	if _, err := project.ImportSpecification(root, file, false); err != nil {
		t.Fatal(err)
	}
	m := newModel(t, root)
	press(m, "enter")
	wantScreen(t, m, "JSON  empty  required  press b to use the example")
	editorScript(t, `exit 0`)
	runEditor(t, m, send(m, "b"))
	wantScreen(t, m, "Body set from the contract example", `"name": "Rex"`)
}
