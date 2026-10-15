package cli

import (
	"context"
	"io"
	"os"
	"os/signal"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

type ScenarioInput struct {
	project.Scenario `yaml:",inline"`
	Revision         string `json:"revision,omitempty" yaml:"revision,omitempty"`
}

func scenarioCommand(o options, in io.Reader) (any, error) {
	root, err := rootPath(o)
	if err != nil {
		return nil, err
	}
	action := o.args[1]
	expected := 3
	if action == "list" || action == "create" || action == "update" {
		expected = 2
	}
	if len(o.args) != expected {
		return nil, project.Fail("invalid_input", "unexpected or missing scenario arguments")
	}
	switch action {
	case "list":
		return project.Scenarios(root)
	case "get":
		return project.GetScenario(root, o.args[2])
	case "validate":
		return project.CheckScenario(root, o.args[2])
	case "create", "update", "delete":
		b, err := input(o, in)
		if err != nil {
			return nil, err
		}
		value, err := validateInput("scenario."+action, b)
		if err != nil {
			return nil, err
		}
		if v, ok := value.(ScenarioInput); ok {
			if action == "update" {
				if _, err := project.GetScenario(root, v.ID); err != nil {
					return nil, err
				}
			}
			resource, err := project.SaveScenario(root, v.Scenario, v.Revision, o.preview)
			return map[string]any{"preview": o.preview, "resource": resource}, err
		}
		return project.DeleteScenario(root, o.args[2], value.(CaseDeleteInput).Revision, o.preview)
	case "run":
		if o.preview {
			return nil, project.Fail("invalid_input", "scenario run does not support preview; use scenario validate")
		}
		variables := map[string]string{}
		if o.input != "" {
			b, err := input(o, in)
			if err != nil {
				return nil, err
			}
			v, err := validateInput("scenario.run", b)
			if err != nil {
				return nil, err
			}
			variables = v.(CaseRunInput).Variables
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		report, err := project.RunScenario(ctx, root, o.env, o.args[2], project.ScenarioOptions{SendOptions: project.SendOptions{Credentials: o.credentials, Variables: variables}})
		bounded := project.PreviewRun(project.Run{Scenario: &report}, 0)
		return bounded.Scenario, err
	}
	return nil, project.Fail("invalid_input", "unknown scenario operation")
}
