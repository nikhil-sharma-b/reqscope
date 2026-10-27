package tui

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type editorFinishedMsg struct {
	text string
	err  error
}

// editorProcess creates scratch content only after Bubble Tea releases the
// terminal. Run owns cleanup, including when the editor fails to start.
type editorProcess struct {
	command        string
	body           string
	ext            string
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
	// The extension lets the editor pick syntax highlighting.
	path := filepath.Join(dir, "reqscope."+p.ext)
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

func editorCommand() string {
	if editor := strings.TrimSpace(os.Getenv("VISUAL")); editor != "" {
		return editor
	}
	if editor := strings.TrimSpace(os.Getenv("EDITOR")); editor != "" {
		return editor
	}
	return "vi"
}

// openEditor hands the terminal to $VISUAL or $EDITOR and calls done with
// what was saved.
func (m *Model) openEditor(text, ext string, done func(text string, err error) tea.Cmd) tea.Cmd {
	p := &editorProcess{command: editorCommand(), body: text, ext: ext}
	m.editorDone = done
	return tea.Exec(p, func(err error) tea.Msg { return editorFinishedMsg{text: p.result, err: err} })
}

// editText edits a document and applies it. When applying fails, the error
// is shown with a way back into the editor that keeps what was typed.
func (m *Model) editText(text, ext string, apply func(m *Model, text string) (tea.Cmd, error)) tea.Cmd {
	var edit func(current string) tea.Cmd
	edit = func(current string) tea.Cmd {
		return m.openEditor(current, ext, func(edited string, err error) tea.Cmd {
			switch {
			case err != nil:
				m.say("The editor closed with an error; nothing changed")
				return nil
			case edited == text:
				m.say("No changes")
				return nil
			}
			cmd, err := apply(m, edited)
			if err != nil {
				lines := []string{}
				for _, line := range wrap(sentence(err.Error()), 70) {
					lines = append(lines, badStyle.Render(line))
				}
				lines = append(lines, "", quiet.Render("Your text is kept. Edit it again to fix this, or close to discard it."))
				m.modal = &viewerModal{title: "Not saved", lines: lines, width: 76,
					extra: []extraKey{{"e", "edit again", func(m *Model) tea.Cmd { return edit(edited) }}}}
			}
			return cmd
		})
	}
	return edit(text)
}
