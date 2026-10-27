package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

// fixture builds a model from data alone, the way the screen sees it.
func fixture(t testing.TB, endpoints int) *Model {
	t.Helper()
	k, err := newKeymap(KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{root: "/work/petstore", credentials: project.NewCredentials(nil), keys: k, width: 120, height: 32, drafts: map[string]*draft{}}
	m.data.snapshot.Manifest.Name = "Petstore"
	staging := project.Resource{Environment: project.Environment{ID: "env-staging", Name: "staging", Classification: "nonproduction", BaseURLs: map[string]string{"pets": "https://staging.petstore.dev/v2"}}}
	m.data.snapshot.Environments = []project.Resource{staging}
	m.data.activeEnv = "env-staging"
	spec := project.Specification{ID: "source", ModuleID: "pets", Location: "openapi.yaml", Base: map[string]any{"info": map[string]any{"title": "Petstore"}}}
	verbs := []string{"GET", "POST", "PUT", "DELETE"}
	for i := range endpoints {
		path := fmt.Sprintf("/pets/%d/{petId}", i/4)
		if i < 3 {
			path = []string{"/pets", "/pets", "/pets/{petId}"}[i]
		}
		spec.Endpoints = append(spec.Endpoints, project.Endpoint{
			ID: fmt.Sprintf("ep-%d", i), SourceID: "source", ModuleID: "pets", Method: verbs[i%4], Path: path,
			Operation: map[string]any{"summary": fmt.Sprintf("Operation %d", i), "tags": []any{"pets"},
				"parameters": []any{map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "example": 20}}},
				"responses":  map[string]any{"200": map[string]any{"description": "OK"}, "404": map[string]any{"description": "Not found"}}},
		})
	}
	if endpoints >= 3 {
		spec.Endpoints[0].Operation["summary"] = "List all pets"
		spec.Endpoints[1].Operation["summary"] = "Add a pet to the store"
		spec.Endpoints[2].Operation["summary"] = "Find a pet by ID"
	}
	m.data.specs = []project.Specification{spec}
	m.data.specGen++
	m.indexModules()
	return m
}

func keyPress(m *Model, key string) {
	msg := tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
	switch key {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "tab":
		msg = tea.KeyPressMsg{Code: tea.KeyTab}
	}
	m.Update(msg)
}

// TestRenderCostDoesNotGrowWithTheAPI guards the fix for slow navigation in
// large APIs: drawing and moving must not re-read, re-filter or re-encode
// every endpoint. The limits are far above the expected cost so only a
// return to per-frame work over the whole API fails them.
func TestRenderCostDoesNotGrowWithTheAPI(t *testing.T) {
	m := fixture(t, 2000)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	m.View()
	measure := func(name string, n int, fn func()) {
		start := time.Now()
		for range n {
			fn()
		}
		if avg := time.Since(start) / time.Duration(n); avg > 15*time.Millisecond {
			t.Errorf("%s takes %v per step with 2000 endpoints", name, avg)
		}
	}
	measure("drawing", 50, func() { m.View() })
	measure("moving", 100, func() { keyPress(m, "j"); m.View() })
	keyPress(m, "/")
	measure("filtering", 10, func() { keyPress(m, "1"); m.View() })
}

func TestGoldenScreens(t *testing.T) {
	cases := []struct {
		name string
		w, h int
		keys []string
	}{
		{"browse-120x32", 120, 32, nil},
		{"browse-80x24", 80, 24, nil},
		{"request-120x32", 120, 32, []string{"j", "j", "enter"}},
		{"request-80x24", 80, 24, []string{"j", "j", "enter"}},
		{"keys-100x30", 100, 30, []string{"?"}},
		{"leader-120x32", 120, 32, []string{"space"}},
		{"leader-request-120x32", 120, 32, []string{"j", "j", "enter", "space", "r"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := fixture(t, 3)
			m.Update(tea.WindowSizeMsg{Width: c.w, Height: c.h})
			for _, k := range c.keys {
				keyPress(m, k)
			}
			got := ansi.Strip(m.View().Content) + "\n"
			path := filepath.Join("testdata", c.name+".golden")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				os.MkdirAll("testdata", 0755)
				os.WriteFile(path, []byte(got), 0644)
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("screen changed; review it, then rerun with UPDATE_GOLDEN=1:\n%s", got)
			}
		})
	}
}

func TestSelectionKeepsMethodColoursAndStaysOnScreen(t *testing.T) {
	m := fixture(t, 60)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	for range 59 {
		keyPress(m, "j")
	}
	view := m.View().Content
	selectedLine := ""
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(ansi.Strip(line), "▌") {
			selectedLine = line
		}
	}
	e, _ := m.selectedEndpoint()
	if selectedLine == "" || e.Path != "/pets/{petId}" {
		t.Fatalf("the last endpoint is not selected on screen:\n%s", ansi.Strip(view))
	}
	// The band sits under each span rather than replacing their colours.
	if !strings.Contains(selectedLine, methodStyle(e.Method).Background(selectionColor).Render(pad(e.Method, 7))) {
		t.Fatalf("selected row lost its method colour: %q", selectedLine)
	}
}

func TestFormEditsBuildTheDraft(t *testing.T) {
	m := fixture(t, 3)
	d := newDraft("x", project.RequestDraft{Module: "pets", Method: "GET", URL: "/pets/{petId}/{{version}}/{owner}", BodyType: "json", Body: `{"a":1}`})
	if got := strings.Join(pathParams(d.request.URL), ","); got != "petId,owner" {
		t.Fatalf("path params %q", got)
	}
	m.commitEdit(d, "path:petId", "7")
	m.commitEdit(d, "add:query", "q = one")
	m.commitEdit(d, "add:query", "flag")
	m.commitEdit(d, "add:header", "X-Trace: abc:def")
	m.commitEdit(d, "add:var", "version=v2")
	m.commitEdit(d, "query:0", "two")
	r := d.request
	if r.Path["petId"] != "7" || len(r.Query) != 2 || r.Query[0] != (project.RequestField{Name: "q", Value: "two"}) || r.Query[1].Name != "flag" ||
		r.Headers[0] != (project.RequestField{Name: "X-Trace", Value: "abc:def"}) || r.Variables["version"] != "v2" {
		t.Fatalf("%+v", r)
	}
	dest, ok := m.destination(d)
	if !ok || dest != "https://staging.petstore.dev/v2/pets/7/v2/{owner}" {
		t.Fatalf("%q %v", dest, ok)
	}
	m.req = d
	for i, row := range m.rows(d) {
		if row.key == "query:0" {
			d.cursor = i
		}
	}
	m.removeRow(d)
	if len(d.request.Query) != 1 || d.request.Query[0].Name != "flag" {
		t.Fatalf("%+v", d.request.Query)
	}
	// Switching to a form keeps the raw body for when it switches back.
	m.cycleBodyType(d)
	m.cycleBodyType(d)
	if d.request.BodyType != "multipart" || d.request.Body != "" {
		t.Fatalf("%+v", d.request)
	}
	m.commitEdit(d, "add:form", "tag=one")
	m.cycleBodyType(d)
	if d.request.BodyType != "raw" || d.request.Body != `{"a":1}` || len(d.request.Form) != 0 {
		t.Fatalf("%+v", d.request)
	}
	m.cycleBodyType(d)
	m.cycleBodyType(d)
	if d.request.BodyType != "form" || len(d.request.Form) != 1 || d.request.Body != "" {
		t.Fatalf("form fields lost while cycling: %+v", d.request)
	}
}

func TestTextInputEditing(t *testing.T) {
	in := newInput("hello world", "")
	for _, k := range []tea.KeyPressMsg{{Code: tea.KeyLeft, Mod: tea.ModAlt}, {Code: 'w', Mod: tea.ModCtrl}} {
		in.update(k)
	}
	if in.String() != "world" {
		t.Fatalf("%q", in.String())
	}
	in.update(tea.KeyPressMsg{Code: tea.KeyEnd})
	in.update(tea.KeyPressMsg{Text: "!"})
	in.update(tea.KeyPressMsg{Code: tea.KeyHome})
	in.update(tea.KeyPressMsg{Code: tea.KeyDelete})
	if in.String() != "orld!" {
		t.Fatalf("%q", in.String())
	}
	in.insert("multi\nline")
	if strings.Contains(in.String(), "\n") {
		t.Fatal("a newline reached a one-line field")
	}
}

func TestZoomShowsOnePaneAndEscRestoresTheSplit(t *testing.T) {
	m := fixture(t, 3)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	screen := func() string { return ansi.Strip(m.View().Content) }
	keyPress(m, "z")
	if s := screen(); strings.Contains(s, "╭─ Details") || !strings.Contains(s, "zoomed") {
		t.Fatalf("browse zoom kept the details pane:\n%s", s)
	}
	keyPress(m, "esc")
	if s := screen(); !strings.Contains(s, "╭─ Details") {
		t.Fatalf("esc did not restore the split:\n%s", s)
	}
	for _, k := range []string{"j", "j", "enter", "l", "z"} {
		keyPress(m, k)
	}
	s := screen()
	if !strings.Contains(s, "╭─ Response") || strings.Contains(s, "╭─ Request") || !strings.Contains(s, "zoomed") {
		t.Fatalf("request zoom did not show the response alone:\n%s", s)
	}
	keyPress(m, "h")
	if s := screen(); !strings.Contains(s, "╭─ Request") || strings.Contains(s, "╭─ Response") {
		t.Fatalf("h while zoomed did not switch to the request:\n%s", s)
	}
	keyPress(m, "l")
	keyPress(m, "esc")
	if s := screen(); !strings.Contains(s, "╭─ Request") || !strings.Contains(s, "╭─ Response") {
		t.Fatalf("esc did not restore the request split:\n%s", s)
	}
	// Esc steps back the same way on both screens: the right pane hands the
	// keys to the left one before the screen is left.
	keyPress(m, "esc")
	if s := screen(); !strings.Contains(s, "╭─ Request") {
		t.Fatalf("esc from the response left the request screen:\n%s", s)
	}
	keyPress(m, "j")
	if !strings.Contains(screen(), "▌") {
		t.Fatalf("keys did not return to the request form:\n%s", screen())
	}
	keyPress(m, "esc")
	if s := screen(); !strings.Contains(s, "╭─ Details") {
		t.Fatalf("browse zoom leaked from the request screen:\n%s", s)
	}
}

func TestDetailsTakeFocusScrollAndZoom(t *testing.T) {
	m := fixture(t, 3)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	screen := func() string { return ansi.Strip(m.View().Content) }
	keyPress(m, "l")
	if !strings.Contains(screen(), "↓ 1–") {
		t.Fatalf("a short window should hide part of the details:\n%s", screen())
	}
	first := screen()
	keyPress(m, "j")
	if screen() == first || !strings.Contains(screen(), "1/3") {
		t.Fatalf("j should scroll the details and leave the selection alone:\n%s", screen())
	}
	keyPress(m, "z")
	if s := screen(); strings.Contains(s, "╭─ Endpoints") || !strings.Contains(s, "╭─ Details") {
		t.Fatalf("zoom should show the details alone:\n%s", s)
	}
	keyPress(m, "esc")
	keyPress(m, "esc")
	keyPress(m, "j")
	if !strings.Contains(screen(), "2/3") {
		t.Fatalf("esc should return the keys to the list:\n%s", screen())
	}
	// Narrow windows show the details on their own.
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	keyPress(m, "l")
	if s := screen(); strings.Contains(s, "╭─ Endpoints") || !strings.Contains(s, "POST  /pets") {
		t.Fatalf("narrow details should replace the list:\n%s", s)
	}
	keyPress(m, "h")
	if !strings.Contains(screen(), "╭─ Endpoints") {
		t.Fatalf("h should return to the list:\n%s", screen())
	}
}
