package cli

import (
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
	"io"
)

type VariableInput struct {
	Name     string `json:"name" yaml:"name"`
	Value    string `json:"value" yaml:"value"`
	Revision string `json:"revision" yaml:"revision"`
}

func variablesCommand(o options, in io.Reader) (any, error) {
	action := o.args[1]
	n := 2
	if action == "get" || action == "delete" {
		n = 3
	}
	if len(o.args) != n || o.env == "" {
		return nil, project.Fail("invalid_input", "variables requires --env and the documented arguments")
	}
	root, err := rootPath(o)
	if err != nil {
		return nil, err
	}
	s, err := project.Load(root)
	if err != nil {
		return nil, err
	}
	env, err := s.Resolve(o.env)
	if err != nil {
		return nil, err
	}
	switch action {
	case "list":
		return map[string]any{"environmentId": env.ID, "revision": env.Revision, "variables": env.Variables}, nil
	case "get":
		value, ok := env.Variables[o.args[2]]
		if !ok {
			return nil, project.Fail("not_found", "variable not found")
		}
		return map[string]any{"environmentId": env.ID, "revision": env.Revision, "name": o.args[2], "value": value}, nil
	case "create", "update", "delete":
		b, err := input(o, in)
		if err != nil {
			return nil, err
		}
		v, err := validateInput("variables."+action, b)
		if err != nil {
			return nil, err
		}
		var name, value, revision string
		if action == "delete" {
			name = o.args[2]
			revision = v.(CaseDeleteInput).Revision
		} else {
			u := v.(VariableInput)
			name, value, revision = u.Name, u.Value, u.Revision
		}
		// Check the enclosing revision before existence so stale edits always conflict.
		if revision != env.Revision {
			return nil, project.Fail("revision_conflict", "environment changed; reread variables or environment get")
		}
		_, exists := env.Variables[name]
		if action == "create" && exists {
			return nil, project.Fail("invalid_input", "variable already exists; use update")
		}
		if action != "create" && !exists {
			return nil, project.Fail("not_found", "variable not found")
		}
		if action == "delete" {
			delete(env.Variables, name)
		} else {
			env.Variables[name] = value
		}
		if err := project.SaveEnvironment(root, env.Environment, revision, o.preview, false); err != nil {
			return nil, err
		}
		b, _ = yaml.Marshal(env.Environment)
		return map[string]any{"environmentId": env.ID, "revision": project.Digest(b), "preview": o.preview}, nil
	}
	return nil, project.Fail("invalid_input", "unknown variables operation")
}
