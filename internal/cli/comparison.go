package cli

import (
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"io"
)

func comparisonCommand(o options, in io.Reader) (any, error) {
	if len(o.args) != 3 || o.preview {
		return nil, project.Fail("invalid_input", "compare requires two run IDs and does not support --preview")
	}
	root, err := rootPath(o)
	if err != nil {
		return nil, err
	}
	var rules *project.IgnoreRules
	if o.input != "" {
		b, e := input(o, in)
		if e != nil {
			return nil, e
		}
		v, e := validateInput("comparison.ignore", b)
		if e != nil {
			return nil, e
		}
		r := v.(project.IgnoreRules)
		rules = &r
	}
	return project.CompareRuns(root, o.args[1], o.args[2], rules)
}
