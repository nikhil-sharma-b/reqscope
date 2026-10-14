package cli

import (
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"io"
)

type CaseInput struct {
	project.Case `yaml:",inline"`
	Revision     string `json:"revision,omitempty" yaml:"revision,omitempty"`
}
type CaseDuplicateInput struct {
	Name string `json:"name" yaml:"name"`
}
type CaseRunInput struct {
	Variables map[string]string `json:"variables" yaml:"variables"`
}
type CaseDeleteInput struct {
	Revision string `json:"revision" yaml:"revision"`
}

func caseCommand(o options, in io.Reader) (any, error) {
	root, err := rootPath(o)
	if err != nil {
		return nil, err
	}
	action := o.args[1]
	expected := 2
	if action == "get" || action == "duplicate" || action == "delete" {
		expected = 3
	}
	if len(o.args) != expected {
		return nil, project.Fail("invalid_input", "unexpected or missing case arguments")
	}
	switch action {
	case "list":
		return project.Cases(root)
	case "get":
		return project.GetCase(root, o.args[2])
	case "create", "update", "duplicate", "delete":
		b, err := input(o, in)
		if err != nil {
			return nil, err
		}
		value, err := validateInput("case."+action, b)
		if err != nil {
			return nil, err
		}
		switch v := value.(type) {
		case CaseInput:
			if action == "update" {
				if _, err := project.GetCase(root, v.ID); err != nil {
					return nil, err
				}
			}
			resource, err := project.SaveCase(root, v.Case, v.Revision, o.preview)
			return map[string]any{"preview": o.preview, "resource": resource, "affectedScenarios": resource.References}, err
		case CaseDuplicateInput:
			resource, err := project.DuplicateCase(root, o.args[2], v.Name, o.preview)
			return map[string]any{"preview": o.preview, "resource": resource}, err
		case CaseDeleteInput:
			return project.DeleteCase(root, o.args[2], v.Revision, o.preview)
		}
	}
	return nil, project.Fail("invalid_input", "unknown case operation")
}
