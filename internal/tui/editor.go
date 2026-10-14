package tui

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

type editorFinishedMsg struct {
	previous string
	draft    project.RequestDraft
	body     string
	err      error
}

// editorProcess creates scratch content only after Bubble Tea releases the
// terminal. Run owns cleanup, including when the editor fails to start.
type editorProcess struct {
	command        string
	body           string
	result         string
	stdin          io.Reader
	stdout, stderr io.Writer
}

func (p *editorProcess) SetStdin(r io.Reader)  { p.stdin = r }
func (p *editorProcess) SetStdout(w io.Writer) { p.stdout = w }
func (p *editorProcess) SetStderr(w io.Writer) { p.stderr = w }
func (p *editorProcess) Run() (err error) {
	dir, err := os.MkdirTemp("", "reqscope-editor-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	path := filepath.Join(dir, "body.txt")
	if err = os.WriteFile(path, []byte(p.body), 0600); err != nil {
		return err
	}
	// The editor setting is a user-owned shell command; the scratch path is a
	// positional argument so neither its name nor body can become shell code.
	cmd := exec.Command("sh", "-c", "exec "+p.command+` "$1"`, "reqscope-editor", path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = p.stdin, p.stdout, p.stderr
	if err = cmd.Run(); err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p.result = string(content)
	return nil
}

func (m *Model) openExternalEditor() tea.Cmd {
	if m.requestCancel != nil {
		m.status = "Cancel the running request first"
		return nil
	}
	var d project.RequestDraft
	if err := project.Decode([]byte(m.requestText), &d); err != nil {
		m.status = err.Error()
		return nil
	}
	if d.BodyFile != "" || d.BodyType != "raw" && d.BodyType != "json" && d.BodyType != "" {
		m.status = "External editing requires an inline raw or JSON body"
		return nil
	}
	if m.credentials.Sanitize(d.Body) != d.Body {
		m.status = "Body contains a registered credential; use an unexpanded secret reference before editing"
		return nil
	}
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vi"
	}
	p := &editorProcess{command: editor, body: d.Body}
	previous := m.requestText
	m.status = "Editing unexpanded request body"
	return tea.Exec(p, func(err error) tea.Msg {
		return editorFinishedMsg{previous: previous, draft: d, body: p.result, err: err}
	})
}

func (m *Model) finishExternalEditor(msg editorFinishedMsg) {
	if msg.err != nil {
		m.status = "Editor cancelled or failed; previous buffer retained"
		return
	}
	if m.requestText != msg.previous {
		m.status = "Request changed while editing; current buffer retained"
		return
	}
	if msg.body == msg.draft.Body {
		m.status = "Editor closed without changes; previous buffer retained"
		return
	}
	msg.draft.Body = msg.body
	b, _ := json.Marshal(msg.draft)
	m.requestText = string(b)
	m.status = "Edited body retained; inputs will be validated before send"
	if msg.draft.BodyType == "json" && !json.Valid([]byte(msg.body)) {
		m.status = "Invalid JSON body; edit the body again before sending"
	}
}
