package cli

import (
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"io"
	"os"
	"strconv"
	"strings"
)

func specificationCommand(o options, in io.Reader) (any, error) {
	root, err := rootPath(o)
	if err != nil {
		return nil, err
	}
	group, action := o.args[0], o.args[1]
	if (group == "endpoint" || group == "schema") && (action == "create" || action == "update" || action == "delete") {
		if len(o.args) != 2 {
			return nil, project.Fail("invalid_input", "contract edits require --input and no positional arguments")
		}
		b, err := input(o, in)
		if err != nil {
			return nil, err
		}
		value, err := validateInput(group+"."+action, b)
		if err != nil {
			return nil, err
		}
		return project.EditContract(root, group, action, value.(project.ContractEdit), o.preview, o.credentials)
	}
	if group == "spec" && action == "export" {
		if len(o.args) != 3 {
			return nil, project.Fail("invalid_input", "spec export requires a source ID")
		}
		return project.ExportContract(root, o.args[2], o.credentials)
	}

	if group == "endpoint" && (action == "search" || action == "list") {
		return endpointSearch(root, o, action == "search")
	}
	if group == "spec" && action == "import" {
		if o.url != "" {
			if len(o.args) != 2 {
				return nil, project.Fail("invalid_input", "unexpected positional arguments")
			}
			opts := project.SourceImportOptions{Environment: o.env, Module: o.module, Credentials: o.credentials}
			if o.input != "" {
				b, err := os.ReadFile(o.input)
				if err != nil {
					return nil, project.Fail("invalid_input", "cannot read import authentication file")
				}
				var auth struct {
					Headers []project.RequestField `json:"headers" yaml:"headers"`
					Query   []project.RequestField `json:"query" yaml:"query"`
				}
				if err := project.Decode(b, &auth); err != nil {
					return nil, project.Fail("invalid_input", "invalid import authentication file")
				}
				opts.Headers, opts.Query = auth.Headers, auth.Query
			}
			discovery, err := project.DiscoverSource(root, o.url, opts)
			if err != nil {
				return discovery, err
			}
			if len(discovery.Candidates) == 0 {
				return discovery, project.Fail("invalid_input", "%s", discovery.Hint)
			}
			var selected *project.SourceCandidate
			for i := range discovery.Candidates {
				c := &discovery.Candidates[i]
				if (o.selection == "" && len(discovery.Candidates) == 1) || o.selection == c.Name || o.selection == c.Location {
					if selected != nil {
						return discovery, project.Fail("ambiguous_resource", "selection matches several specifications; use the raw URL")
					}
					selected = c
				}
			}
			if selected == nil {
				return discovery, project.Fail("invalid_input", "choose a candidate with --select NAME_OR_URL, or supply its raw --url")
			}
			return project.ImportSource(root, *selected, o.preview, "", opts)
		}
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

func endpointSearch(root string, o options, paged bool) (any, error) {
	if len(o.args) != 2 {
		return nil, project.Fail("invalid_input", "endpoint search/list takes no positional arguments")
	}
	if o.groupBy != "" && o.groupBy != "tag" {
		return nil, project.Fail("invalid_input", "--group-by must be tag")
	}
	method := strings.ToUpper(o.method)
	if method != "" && !strings.Contains("|GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|TRACE|", "|"+method+"|") {
		return nil, project.Fail("invalid_input", "unknown HTTP method")
	}
	limit, offset := 100, 0
	for _, arg := range []struct {
		raw              string
		target           *int
		minimum, maximum int
	}{{o.limit, &limit, 1, 1000}, {o.offset, &offset, 0, int(^uint(0) >> 1)}} {
		if arg.raw != "" {
			n, err := strconv.Atoi(arg.raw)
			if err != nil || n < arg.minimum || n > arg.maximum {
				return nil, project.Fail("invalid_input", "invalid pagination value")
			}
			*arg.target = n
		}
	}
	specs, err := project.Specifications(root)
	if err != nil {
		return nil, err
	}
	matches := project.SearchEndpoints(specs, project.EndpointQuery{Query: o.query, Method: method, Tag: o.tag, GroupByTag: o.groupBy == "tag"})
	if !paged && o.query == "" && o.method == "" && o.tag == "" && o.groupBy == "" && o.limit == "" && o.offset == "" {
		return matches, nil
	}
	start := min(offset, len(matches))
	end := start + min(limit, len(matches)-start)
	var next *int
	if end < len(matches) {
		n := end
		next = &n
	}
	return map[string]any{"items": matches[start:end], "total": len(matches), "nextOffset": next, "empty": len(matches) == 0}, nil
}
