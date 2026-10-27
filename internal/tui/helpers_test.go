package tui_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
)

// keyMsg builds the key event a terminal sends for a key name.
func keyMsg(s string) tea.KeyPressMsg {
	named := map[string]tea.Key{
		"enter": {Code: tea.KeyEnter}, "esc": {Code: tea.KeyEscape}, "tab": {Code: tea.KeyTab},
		"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift}, "backspace": {Code: tea.KeyBackspace},
		"space": {Code: tea.KeySpace, Text: " "}, "up": {Code: tea.KeyUp}, "down": {Code: tea.KeyDown},
		"left": {Code: tea.KeyLeft}, "right": {Code: tea.KeyRight},
	}
	if k, ok := named[s]; ok {
		return tea.KeyPressMsg(k)
	}
	if rest, ok := strings.CutPrefix(s, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: []rune(rest)[0], Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

// send delivers one key and returns its command without running it.
func send(m *tui.Model, key string) tea.Cmd {
	_, cmd := m.Update(keyMsg(key))
	return cmd
}

// press delivers keys and runs the work each one starts, as the program
// would, so flows that wait on background work finish before the next key.
func press(m *tui.Model, keys ...string) {
	for _, k := range keys {
		run(m, send(m, k))
	}
}

// typeText delivers text the way a terminal delivers a burst of characters.
func typeText(m *tui.Model, text string) {
	m.Update(tea.KeyPressMsg{Text: text})
}

func run(m *tui.Model, cmd tea.Cmd) {
	for depth := 0; cmd != nil && depth < 20; depth++ {
		msg := cmd()
		switch msg := msg.(type) {
		case nil, tea.QuitMsg:
			return
		case tea.BatchMsg:
			for _, c := range msg {
				run(m, c)
			}
			return
		}
		_, cmd = m.Update(msg)
		// A load schedules the next once-a-second poll; tests poll explicitly.
		if fmt.Sprintf("%T", msg) == "tui.loadedMsg" {
			return
		}
	}
}

func screen(m *tui.Model) string { return ansi.Strip(m.View().Content) }

func wantScreen(t *testing.T, m *tui.Model, texts ...string) {
	t.Helper()
	view := strings.ToLower(screen(m))
	for _, text := range texts {
		if !strings.Contains(view, strings.ToLower(text)) {
			t.Fatalf("screen lacks %q:\n%s", text, screen(m))
		}
	}
}

func rejectScreen(t *testing.T, m *tui.Model, texts ...string) {
	t.Helper()
	view := strings.ToLower(screen(m))
	for _, text := range texts {
		if strings.Contains(view, strings.ToLower(text)) {
			t.Fatalf("screen shows %q:\n%s", text, screen(m))
		}
	}
}

func newModel(t *testing.T, root string, size ...int) *tui.Model {
	t.Helper()
	m, err := tui.New(root, tui.KeyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	w, h := 140, 40
	if len(size) == 2 {
		w, h = size[0], size[1]
	}
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

// poll runs one look for outside changes, as the once-a-second timer does.
func poll(t *testing.T, m *tui.Model) {
	t.Helper()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("no workspace poll")
	}
	m.Update(cmd())
}

// editorHarness runs an editor command through Bubble Tea itself, which owns
// terminal suspension, and stops once the editor's outcome reaches the model.
type editorHarness struct {
	*tui.Model
	launch tea.Cmd
	after  tea.Cmd
}

func (h *editorHarness) Init() tea.Cmd { return h.launch }
func (h *editorHarness) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		msg = tea.WindowSizeMsg{Width: 140, Height: 40}
	}
	_, cmd := h.Model.Update(msg)
	if fmt.Sprintf("%T", msg) == "tui.editorFinishedMsg" {
		h.after = cmd
		return h, tea.Quit
	}
	return h, cmd
}

// runEditor runs the editor command, applies its outcome and finishes any
// work the outcome started.
func runEditor(t *testing.T, m *tui.Model, launch tea.Cmd) {
	t.Helper()
	if launch == nil {
		t.Fatalf("no editor launched:\n%s", screen(m))
	}
	h := &editorHarness{Model: m, launch: launch}
	p := tea.NewProgram(h, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	timer := time.AfterFunc(5*time.Second, p.Kill)
	defer timer.Stop()
	if _, err := p.Run(); err != nil {
		t.Fatalf("%v:\n%s", err, screen(m))
	}
	run(m, h.after)
}

// editorScript points $VISUAL at a shell script for the next editor run.
func editorScript(t *testing.T, body string) {
	t.Helper()
	path := t.TempDir() + "/editor.sh"
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "sh "+path)
	t.Setenv("EDITOR", "does-not-exist")
}

func ansiWidth(s string) int { return ansi.StringWidth(s) }

// selected reports whether the highlighted row contains text; the cursor
// bar survives styling being stripped.
func selected(m *tui.Model, text string) bool {
	for _, line := range strings.Split(screen(m), "\n") {
		if strings.Contains(line, "▌") && strings.Contains(line, text) {
			return true
		}
	}
	return false
}

// moveTo presses j until the highlighted row contains text.
func moveTo(t *testing.T, m *tui.Model, text string) {
	t.Helper()
	for i := 0; i < 40; i++ {
		if selected(m, text) {
			return
		}
		press(m, "j")
	}
	t.Fatalf("never reached %q:\n%s", text, screen(m))
}

// hasLine reports whether some screen row, trimmed, is exactly text.
func hasLine(m *tui.Model, text string) bool {
	for _, line := range strings.Split(screen(m), "\n") {
		for _, part := range strings.Split(line, "│") {
			if strings.TrimSpace(part) == text {
				return true
			}
		}
	}
	return false
}

func newCredentialModel(t *testing.T, root string, vault *project.Credentials) *tui.Model {
	t.Helper()
	m, err := tui.NewWithCredentials(root, tui.KeyConfig{}, vault)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	return m
}

// memoryStore stands in for the OS keychain; unavailable makes it fail.
type memoryStore struct {
	unavailable bool
	values      map[string]string
}

func (s *memoryStore) Set(k, v string) error {
	if s.unavailable {
		return errors.New("unavailable store")
	}
	s.values[k] = v
	return nil
}
func (s *memoryStore) Get(k string) (string, error) {
	if s.unavailable {
		return "", errors.New("unavailable store")
	}
	return s.values[k], nil
}
func (s *memoryStore) Delete(k string) error { delete(s.values, k); return nil }
