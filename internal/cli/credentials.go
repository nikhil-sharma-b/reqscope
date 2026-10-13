package cli

import "github.com/nikhil-sharma-b/reqscope/internal/project"

func credentialCommand(o options) (any, error) {
	action := o.args[1]
	if action != "inspect" && !(o.args[0] == "secret" && action == "list") {
		return nil, project.Fail("access_denied", "credential values, protected bindings and grants are managed interactively in the TUI")
	}
	if len(o.args) != 2 {
		return nil, project.Fail("invalid_input", "secret/access inspection takes no positional arguments")
	}
	if o.input != "" || o.preview || o.env == "" {
		return nil, project.Fail("invalid_input", "secret/access inspection requires --env and accepts no input or preview")
	}
	root, err := rootPath(o)
	if err != nil {
		return nil, err
	}
	states, err := o.credentials.Inspect(root, o.env)
	if err != nil {
		return nil, err
	}
	if o.args[0] == "secret" {
		return states, nil
	}
	review, err := project.ReviewAccess(root, o.env)
	if err != nil {
		return nil, err
	}
	module := o.module
	if module == "" {
		for id := range review.Origins {
			module = id
			break
		}
	}
	_, grantErr := project.CheckCLIAccess(root, o.env, module, o.url)
	return struct {
		Environment string                `json:"environmentID"`
		Secrets     []project.SecretState `json:"secrets"`
		Granted     bool                  `json:"granted"`
	}{review.EnvironmentID, states, grantErr == nil}, nil
}
