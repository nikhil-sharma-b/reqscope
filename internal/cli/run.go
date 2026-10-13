// Package cli translates noninteractive commands into shared project operations.
package cli

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"github.com/nikhil-sharma-b/reqscope/internal/tui"
	"go.yaml.in/yaml/v3"
	"golang.org/x/term"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const Usage = `Usage: reqscope [--project PATH] [--output json] COMMAND
  project init [NAME] [--input FILE|-] [--preview]
  project inspect|validate
  project update --input FILE|- [--preview]
  spec import --input FILE [--preview]
  spec inspect [SOURCE_ID]
  endpoint list|get ID
  schema list|get ID
  environment list|get ID_OR_NAME
  environment create|update --input FILE|- [--preview]
  input-schema list|get NAME
  input-schema validate NAME --input FILE|-
  tui [--keys FILE]
No arguments opens the TUI only when stdin and stdout are terminals.
`

type options struct {
	project, output, input, keys string
	preview                      bool
	args                         []string
}

func parse(args []string) (options, error) {
	var o options
	// Preserve JSON error envelopes even if parsing fails before --output.
	for i, a := range args {
		if a == "--output=json" || a == "--output" && i+1 < len(args) && args[i+1] == "json" {
			o.output = "json"
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--preview" {
			o.preview = true
			continue
		}
		if a == "--help" || a == "-h" {
			o.args = []string{"help"}
			return o, nil
		}
		if strings.HasPrefix(a, "--") {
			parts := strings.SplitN(a, "=", 2)
			key := parts[0]
			var value string
			if len(parts) == 2 {
				value = parts[1]
			} else {
				i++
				if i >= len(args) {
					return o, project.Fail("invalid_input", "%s requires a value", key)
				}
				value = args[i]
			}
			switch key {
			case "--project":
				o.project = value
			case "--output":
				o.output = value
			case "--input":
				o.input = value
			case "--keys":
				o.keys = value
			default:
				return o, project.Fail("invalid_input", "unknown option %s", key)
			}
		} else {
			o.args = append(o.args, a)
		}
	}
	if o.output != "" && o.output != "json" && o.output != "text" {
		return o, project.Fail("invalid_input", "--output must be text or json")
	}
	return o, nil
}

type envelope struct {
	SchemaVersion int            `json:"schemaVersion"`
	Success       bool           `json:"success"`
	Data          any            `json:"data"`
	Warnings      []string       `json:"warnings"`
	Error         *project.Error `json:"error"`
}

func exitCode(err error) int {
	var e *project.Error
	if errors.As(err, &e) {
		switch e.Code {
		case "invalid_input", "unsupported_format":
			return 2
		case "not_found", "revision_conflict", "ambiguous_resource":
			return 3
		case "access_denied":
			return 4
		case "cancelled":
			return 130
		}
	}
	return 7
}
func terminal(r io.Reader, w io.Writer) bool {
	in, a := r.(*os.File)
	out, b := w.(*os.File)
	return a && b && term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
}

// Run never prompts unless an interactive TUI was selected with terminal streams.
func Run(args []string, in io.Reader, out, diag io.Writer) int {
	o, err := parse(args)
	if err != nil {
		return emit(o, nil, err, out, diag)
	}
	if len(o.args) == 0 {
		if terminal(in, out) && o.output != "json" {
			o.args = []string{"tui"}
		} else {
			if o.output == "json" {
				return emit(o, map[string]string{"usage": Usage}, nil, out, diag)
			}
			fmt.Fprint(out, Usage)
			return 0
		}
	}
	if o.args[0] == "help" {
		if o.output == "json" {
			return emit(o, map[string]string{"usage": Usage}, nil, out, diag)
		}
		fmt.Fprint(out, Usage)
		return 0
	}
	if o.args[0] == "tui" {
		if !terminal(in, out) || o.output == "json" {
			return emit(o, nil, project.Fail("invalid_input", "tui requires terminal stdin/stdout and text output"), out, diag)
		}
		if len(o.args) != 1 {
			return emit(o, nil, project.Fail("invalid_input", "tui takes no positional arguments"), out, diag)
		}
		err = tui.Run(o.project, o.keys, in, out)
		if errors.Is(err, tea.ErrInterrupted) {
			err = project.Fail("cancelled", "TUI cancelled")
		}
		if err != nil {
			return emit(o, nil, err, out, diag)
		}
		return 0
	}
	data, err := execute(o, in)
	return emit(o, data, err, out, diag)
}
func emit(o options, data any, err error, out, diag io.Writer) int {
	code := 0
	var e *project.Error
	if err != nil {
		code = exitCode(err)
		if !errors.As(err, &e) {
			e = &project.Error{Code: "storage_error", Message: err.Error()}
		}
		fmt.Fprintln(diag, e.Message)
	}
	if o.output == "json" {
		if encErr := json.NewEncoder(out).Encode(envelope{1, err == nil, data, []string{}, e}); encErr != nil {
			fmt.Fprintln(diag, encErr)
			return 7
		}
	} else if err == nil {
		fmt.Fprintln(out, formatted(data))
	}
	return code
}
func input(o options, in io.Reader) ([]byte, error) {
	if o.input == "" {
		return nil, project.Fail("invalid_input", "--input FILE or --input - is required")
	}
	r := in
	if o.input != "-" {
		f, err := os.Open(o.input)
		if err != nil {
			return nil, project.Fail("invalid_input", "cannot open input: %v", err)
		}
		defer f.Close()
		r = f
	}
	b, err := io.ReadAll(io.LimitReader(r, 1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 1024*1024 {
		return nil, project.Fail("invalid_input", "structured input exceeds 1 MiB")
	}
	return b, nil
}
func rootPath(o options) (string, error) {
	if o.project != "" {
		p, err := filepath.Abs(o.project)
		if err != nil {
			return "", err
		}
		if filepath.Base(p) == "reqscope.yaml" {
			p = filepath.Dir(p)
		}
		return p, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return project.Discover(cwd)
}
func execute(o options, in io.Reader) (any, error) {
	if len(o.args) < 2 {
		return nil, project.Fail("invalid_input", "command and operation required; use --help")
	}
	group, action := o.args[0], o.args[1]
	if group == "input-schema" {
		switch action {
		case "list":
			if len(o.args) != 2 {
				return nil, project.Fail("invalid_input", "input-schema list takes no arguments")
			}
			return schemaNames, nil
		case "get", "validate":
			if len(o.args) != 3 {
				return nil, project.Fail("invalid_input", "schema name required")
			}
			if action == "get" {
				return schema(o.args[2])
			}
			b, err := input(o, in)
			if err != nil {
				return nil, err
			}
			v, err := validateInput(o.args[2], b)
			return map[string]any{"valid": err == nil, "input": v}, err
		}
		return nil, project.Fail("invalid_input", "unknown input-schema operation")
	}
	if group == "project" && action == "init" {
		var name string
		if o.input != "" {
			if len(o.args) != 2 {
				return nil, project.Fail("invalid_input", "use either a name or --input")
			}
			b, err := input(o, in)
			if err != nil {
				return nil, err
			}
			v, err := validateInput("project.init", b)
			if err != nil {
				return nil, err
			}
			name = v.(InitInput).Name
		} else {
			if len(o.args) != 3 {
				return nil, project.Fail("invalid_input", "project init requires NAME or --input")
			}
			name = o.args[2]
		}
		root := o.project
		if filepath.Base(root) == "reqscope.yaml" {
			root = filepath.Dir(root)
		}
		if root == "" {
			root = "."
		}
		return project.Init(root, name, o.preview)
	}
	if group == "spec" || group == "endpoint" || group == "schema" {
		return specificationCommand(o)
	}
	if group != "project" && group != "environment" {
		return nil, project.Fail("invalid_input", "unknown command %q; use --help", group)
	}
	if group == "project" && action != "inspect" && action != "validate" && action != "update" {
		return nil, project.Fail("invalid_input", "unknown project operation %q", action)
	}
	if group == "environment" && action != "list" && action != "get" && action != "create" && action != "update" {
		return nil, project.Fail("invalid_input", "unknown environment operation %q", action)
	}
	expectedArgs := 2
	if group == "environment" && action == "get" {
		expectedArgs = 3
	}
	if len(o.args) != expectedArgs {
		return nil, project.Fail("invalid_input", "unexpected or missing positional arguments")
	}
	root, err := rootPath(o)
	if err != nil {
		return nil, err
	}
	s, err := project.Load(root)
	if err != nil {
		return nil, err
	}
	if group == "project" {
		switch action {
		case "inspect":
			return s, nil
		case "validate":
			return map[string]any{"valid": true, "id": s.Manifest.ID, "revision": s.Revision}, nil
		case "update":
			b, err := input(o, in)
			if err != nil {
				return nil, err
			}
			v, err := validateInput("project.update", b)
			if err != nil {
				return nil, err
			}
			u := v.(UpdateInput)
			m := s.Manifest
			m.Name = u.Name
			b, _ = yaml.Marshal(m)
			err = project.Apply(root, []project.Change{{Path: "reqscope.yaml", ExpectedRevision: u.Revision, Content: b}}, o.preview)
			if err != nil {
				return nil, err
			}
			if o.preview {
				return map[string]any{"preview": true, "project": m, "revision": project.Digest(b)}, nil
			}
			return project.Load(root)
		}
	}
	switch action {
	case "list":
		return s.Environments, nil
	case "get":
		return s.Resolve(o.args[2])
	case "create", "update":
		b, err := input(o, in)
		if err != nil {
			return nil, err
		}
		v, err := validateInput("environment."+action, b)
		if err != nil {
			return nil, err
		}
		u := v.(EnvironmentInput)
		if action == "update" {
			if _, err = s.Resolve(u.ID); err != nil {
				return nil, err
			}
		}
		b, _ = yaml.Marshal(u.Environment)
		err = project.Apply(root, []project.Change{{Path: filepath.Join("environments", u.ID+".yaml"), ExpectedRevision: u.Revision, Content: b}}, o.preview)
		if err != nil {
			return nil, err
		}
		return map[string]any{"preview": o.preview, "resource": project.Resource{Environment: u.Environment, Revision: project.Digest(b)}}, nil
	}
	return nil, project.Fail("invalid_input", "unknown operation")
}
