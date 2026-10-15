package cli

import (
	"io"
	"strconv"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func historyCommand(o options, in io.Reader) (any, error) {
	root, err := rootPath(o)
	if err != nil {
		return nil, err
	}
	if o.preview {
		return nil, project.Fail("invalid_input", "history does not support --preview")
	}
	action := o.args[1]
	n := 2
	switch action {
	case "get", "pin", "unpin", "delete", "associate":
		n = 3
	}
	if len(o.args) != n {
		return nil, project.Fail("invalid_input", "unexpected history arguments")
	}
	switch action {
	case "list":
		limit, offset := 50, 0
		if o.limit != "" {
			limit, err = strconv.Atoi(o.limit)
			if err != nil {
				return nil, project.Fail("invalid_input", "invalid limit")
			}
		}
		if o.offset != "" {
			offset, err = strconv.Atoi(o.offset)
			if err != nil {
				return nil, project.Fail("invalid_input", "invalid offset")
			}
		}
		caseID := o.caseSelector
		if caseID != "" {
			c, e := project.GetCase(root, caseID)
			if e != nil {
				return nil, e
			}
			caseID = c.ID
		}
		return project.History(root, caseID, limit, offset)
	case "get":
		r, e := project.GetRun(root, o.args[2])
		if e != nil {
			return nil, e
		}
		return project.PreviewRun(r, 4096), nil
	case "pin", "unpin", "delete", "associate":
		err = project.ChangeRun(root, o.args[2], action, o.caseSelector)
	case "prune":
		err = project.PruneHistory(root)
	case "settings":
		if o.input == "" {
			return project.History(root, "", 1, 0)
		}
		b, e := input(o, in)
		if e != nil {
			return nil, e
		}
		v, e := validateInput("history.settings", b)
		if e != nil {
			return nil, e
		}
		err = project.SetHistoryLimits(root, v.(project.HistoryLimits))
	default:
		return nil, project.Fail("invalid_input", "unknown history operation")
	}
	return map[string]any{"operation": action}, err
}
