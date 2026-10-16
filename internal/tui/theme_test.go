package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func workbench(t *testing.T) *Model {
	t.Helper()
	m, err := New("", KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.overlay, m.mode, m.draft = "", "NORMAL", "Petstore API"
	m.pane = endpointsPane
	m.specs = []project.Specification{{ID: "source", ModuleID: "pets", Location: "openapi.yaml", Endpoints: []project.Endpoint{
		{ID: "list", Method: "GET", Path: "/pets", Operation: map[string]any{"summary": "List all pets"}},
		{ID: "create", Method: "POST", Path: "/pets", Operation: map[string]any{"summary": "Add a pet to the store"}},
		{ID: "get", Method: "GET", Path: "/pets/{petId}", Operation: map[string]any{"summary": "Find a pet by ID"}},
	}}}
	m.requestText = `{"method":"GET","url":"/pets","headers":{"Accept":"application/json"}}`
	m.requestBaseline = m.requestText
	return m
}

func TestWorkbenchGolden(t *testing.T) {
	for _, size := range [][2]int{{120, 32}, {80, 24}, {25, 9}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := workbench(t)
			m.width, m.height = size[0], size[1]
			// Strip only styling: the snapshot still verifies every terminal cell.
			got := ansi.Strip(m.View().Content) + "\n"
			path := filepath.Join("testdata", fmt.Sprintf("workbench-%dx%d.golden", size[0], size[1]))
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll("testdata", 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("workspace changed; inspect before updating golden:\n%s", got)
			}
		})
	}
}

func TestPanelDimensions(t *testing.T) {
	for width := 1; width <= 45; width++ {
		for height := 1; height <= 10; height++ {
			text := panel(selectStyle.Render("wide 界 text"), "Endpoints", "g e", width, height, true)
			lines := strings.Split(text, "\n")
			if len(lines) != height {
				t.Fatalf("%dx%d: got %d rows", width, height, len(lines))
			}
			for _, line := range lines {
				if n := ansi.StringWidth(line); n > width || width >= 4 && height >= 3 && n != width {
					t.Fatalf("%dx%d: row has %d columns: %q", width, height, n, line)
				}
			}
		}
	}
}

func TestInactivePaneUsesOwnDataWithoutChangingFocus(t *testing.T) {
	m := workbench(t)
	m.pane, m.query, m.index = schemasPane, "Missing", 7
	before := m.paneView(endpointsPane, 40, 15)
	if !strings.Contains(ansi.Strip(before), "/pets") || strings.Contains(ansi.Strip(before), selectedMarker+"1 GET") {
		t.Fatal("inactive endpoint pane inherited schema search or focus", before)
	}
	if m.pane != schemasPane || m.query != "Missing" || m.index != 7 {
		t.Fatal("rendering changed focus, search or selection")
	}
}

func TestRequestFormattingDoesNotChangeDraft(t *testing.T) {
	m := workbench(t)
	m.pane = requestPane
	before := m.requestText
	view := ansi.Strip(m.paneView(requestPane, 60, 16))
	if !strings.Contains(view, `  "method": "GET",`) || m.requestText != before || m.requestDirty() {
		t.Fatal("pretty rendering lost JSON fields or modified the draft", view)
	}
}

func TestWorkbenchHintsRespectRemappingAndUnbinding(t *testing.T) {
	m, err := New("", KeyConfig{Leader: ",", Bindings: map[string][]string{"help": {"H"}, "search": {}, "request": {"Q"}}})
	if err != nil {
		t.Fatal(err)
	}
	footer := ansi.Strip(m.statusLine(120))
	if !strings.Contains(footer, "H help") || strings.Contains(footer, "? help") || m.paneHint(requestPane) != "Q" {
		t.Fatal("hints did not use effective bindings", footer)
	}
}

func TestWorkbenchResizeZoomAndOverlays(t *testing.T) {
	m := workbench(t)
	m.index = 2
	for _, size := range [][2]int{{120, 32}, {80, 24}, {25, 9}, {4, 3}, {1, 1}, {120, 32}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, overlay := range []string{"", "palette", "help", "import"} {
			m.overlay = overlay
			view := m.View()
			if !view.AltScreen {
				t.Fatal("workspace left alternate screen")
			}
			lines := strings.Split(view.Content, "\n")
			if len(lines) > size[1] {
				t.Fatalf("%s at %v exceeds terminal height: %d", overlay, size, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("%s at %v exceeds terminal width: %q", overlay, size, line)
				}
			}
		}
	}
	m.overlay, m.zoom = "", true
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "[zoom]") || strings.Contains(view, "╭ Request") || m.index != 2 || m.pane != endpointsPane {
		t.Fatal("zoom or resize changed selection or focus", view)
	}
}

func TestEndpointSelectionStaysVisible(t *testing.T) {
	labels := []string{}
	for i := range 30 {
		labels = append(labels, fmt.Sprintf("GET /pets/%d | Pet details | openapi.yaml | module uuid", i))
	}
	view := ansi.Strip(fit(endpointTable(labels, 29, 28, 7, true), 28, 7))
	if !strings.Contains(view, selectedMarker+"30 GET") || !strings.Contains(view, "/pets/29") || strings.Contains(view, "module uuid") {
		t.Fatal("selection or readable endpoint identity lost", view)
	}
	wide := ansi.Strip(endpointTable(labels, 0, 100, 7, true))
	if !strings.Contains(wide, "SOURCE") || !strings.Contains(wide, "openapi.yaml") {
		t.Fatal("wide table dropped the source column", wide)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 28 {
			t.Fatalf("row overflows pane: %q", line)
		}
	}
}
