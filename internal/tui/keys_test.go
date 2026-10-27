package tui_test

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
)

func TestKeyConfigRejectsSequencesAndConflicts(t *testing.T) {
	for name, c := range map[string]tui.KeyConfig{
		"leader conflict": {Leader: "s"},
		"leader sequence": {Leader: "g g"},
		"sequence":        {Bindings: map[string][]string{"help": {"g g"}}},
		"global conflict": {Bindings: map[string][]string{"help": {"j"}}},
		"screen conflict": {Bindings: map[string][]string{"send": {"a"}}},
		"reserved":        {Bindings: map[string][]string{"quit": {"esc"}}},
		"unknown":         {Bindings: map[string][]string{"request-send": {"x"}}},
		"unsupported":     {Bindings: map[string][]string{"help": {"hyper+x"}}},
	} {
		if _, err := tui.Bindings(c); err == nil {
			t.Errorf("%s: accepted %+v", name, c)
		}
	}
	// One letter may mean different things on screens that never overlap.
	if _, err := tui.Bindings(tui.KeyConfig{Bindings: map[string][]string{"send": {"r"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestRemappedKeysDriveFooterHelpAndCommands(t *testing.T) {
	root := t.TempDir()
	if _, err := project.Init(root, "Demo", false); err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{Bindings: map[string][]string{"help": {"K"}, "environment": {}}})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	wantScreen(t, m, "K keys")
	rejectScreen(t, m, "? keys", "e env")
	press(m, "K")
	wantScreen(t, m, "Keys", "Show the keys for this screen")
	press(m, "esc", "?")
	wantScreen(t, m, "? does nothing here")
	press(m, ":")
	typeText(m, "check project")
	press(m, "enter")
	wantScreen(t, m, "Project files are valid")
	// An unbound action still runs from the command search.
	press(m, ":")
	typeText(m, "switch environment")
	press(m, "enter")
	wantScreen(t, m, "Send requests to", "staging")
}

func TestTabsAndQuit(t *testing.T) {
	root := t.TempDir()
	if _, err := project.Init(root, "Demo", false); err != nil {
		t.Fatal(err)
	}
	m := newModel(t, root, 120, 30)
	wantScreen(t, m, "No API imported yet", "i import")
	press(m, "2")
	wantScreen(t, m, "No saved requests yet")
	press(m, "5")
	wantScreen(t, m, "3 environments", "local", "staging", "production")
	press(m, "tab")
	wantScreen(t, m, "No API imported yet")
	press(m, "shift+tab", "shift+tab")
	wantScreen(t, m, "No runs yet")
	if cmd := send(m, "q"); cmd == nil {
		t.Fatal("quit on a clean workspace did not exit")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("quit returned something other than quit")
	}
}

func TestLayoutFitsEveryTerminalSize(t *testing.T) {
	root := t.TempDir()
	if _, err := project.Init(root, strings.Repeat("Long project name ", 8), false); err != nil {
		t.Fatal(err)
	}
	m := newModel(t, root)
	for _, size := range [][2]int{{160, 50}, {120, 32}, {80, 24}, {45, 16}, {25, 9}, {4, 3}, {1, 1}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, keys := range [][]string{{}, {"?"}, {":"}, {"i"}, {"5", "enter"}, {"e"}} {
			press(m, keys...)
			view := m.View()
			if !view.AltScreen {
				t.Fatal("workspace left the alternate screen")
			}
			lines := strings.Split(view.Content, "\n")
			if len(lines) > size[1] {
				t.Fatalf("%v after %v: %d rows", size, keys, len(lines))
			}
			for _, line := range lines {
				if w := ansiWidth(line); w > size[0] {
					t.Fatalf("%v after %v: row of %d columns: %q", size, keys, w, line)
				}
			}
			press(m, "esc", "esc", "1")
		}
	}
}

func TestFirstRunCreatesAProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new-api")
	m, err := tui.New("", tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	wantScreen(t, m, "Create a Reqscope project", "Folder", "Name")
	press(m, "ctrl+u")
	typeText(m, dir)
	press(m, "tab", "ctrl+u")
	typeText(m, "Orders API")
	press(m, "enter")
	wantScreen(t, m, "Project created", "Orders API", "No API imported yet")
	if s, err := project.Load(dir); err != nil || s.Manifest.Name != "Orders API" {
		t.Fatalf("%+v %v", s, err)
	}
	fresh, _ := tui.New("", tui.KeyConfig{})
	if cmd := send(fresh, "esc"); cmd == nil {
		t.Fatal("esc on the first-run form did not quit")
	}
}

func TestLeaderMenuRunsGroupedCommands(t *testing.T) {
	root := t.TempDir()
	if _, err := project.Init(root, "Demo", false); err != nil {
		t.Fatal(err)
	}
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	wantScreen(t, m, "space menu")
	press(m, "space")
	wantScreen(t, m, "+request", "+environments", "esc close")
	press(m, "g")
	wantScreen(t, m, "space g", "saved requests")
	press(m, "backspace")
	wantScreen(t, m, "+request")
	press(m, "g", "h")
	rejectScreen(t, m, "esc close")
	wantScreen(t, m, "History")
	press(m, "space", "p", "v")
	wantScreen(t, m, "Project files are valid")
	press(m, "space", "Z")
	wantScreen(t, m, "space Z does nothing")
	press(m, "space", "esc")
	rejectScreen(t, m, "+request")
}

func TestLeaderIsConfigurable(t *testing.T) {
	if _, err := tui.Bindings(tui.KeyConfig{Leader: ","}); err != nil {
		t.Fatal(err)
	}
}
