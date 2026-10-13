package cli

import "github.com/nikhil-sharma-b/reqscope/internal/project"

func specificationCommand(o options) (any, error) {
	root, err := rootPath(o)
	if err != nil {
		return nil, err
	}
	group, action := o.args[0], o.args[1]
	if group == "spec" && action == "import" {
		if len(o.args) != 2 || o.input == "" || o.input == "-" {
			return nil, project.Fail("invalid_input", "spec import requires --input FILE; stdin cannot establish an approved reference root")
		}
		if _, err := project.Load(root); err != nil {
			return nil, err
		}
		return project.ImportSpecification(root, o.input, o.preview)
	}
	if (group == "spec" && action != "inspect") || (group != "spec" && action != "list" && action != "get") {
		return nil, project.Fail("invalid_input", "unknown specification operation")
	}
	if (action == "get" && len(o.args) != 3) || (action == "list" && len(o.args) != 2) || (action == "inspect" && len(o.args) != 2 && len(o.args) != 3) {
		return nil, project.Fail("invalid_input", "unexpected or missing positional arguments")
	}
	specs, err := project.Specifications(root)
	if err != nil {
		return nil, err
	}
	if group == "spec" {
		if len(o.args) == 2 {
			return specs, nil
		}
		for _, s := range specs {
			if s.ID == o.args[2] {
				return s, nil
			}
		}
	}
	endpoints := []project.Endpoint{}
	schemas := []project.Schema{}
	for _, s := range specs {
		for _, e := range s.Endpoints {
			if group == "endpoint" && action == "get" && e.ID == o.args[2] {
				return map[string]any{"endpoint": e, "revision": s.Revision, "source": s.Location}, nil
			}
			endpoints = append(endpoints, e)
		}
		for _, schema := range s.Schemas {
			if group == "schema" && action == "get" && schema.ID == o.args[2] {
				return map[string]any{"schema": schema, "revision": s.Revision, "source": s.Location}, nil
			}
			schemas = append(schemas, schema)
		}
	}
	if action == "list" {
		if group == "endpoint" {
			return endpoints, nil
		}
		return schemas, nil
	}
	return nil, project.Fail("not_found", "resource not found; list resources to select an ID")
}
