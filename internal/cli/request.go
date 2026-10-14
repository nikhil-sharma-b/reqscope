package cli

import (
	"context"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"io"
	"os"
	"os/signal"
)

func requestCommand(o options, in io.Reader) (any, error) {
	if len(o.args) != 2 || o.args[1] != "send" || o.preview {
		return nil, project.Fail("invalid_input", "use request send --env ENV --input FILE|-")
	}
	root, err := rootPath(o)
	if err != nil {
		return nil, err
	}
	if o.caseSelector != "" {
		overrides := map[string]string{}
		if o.input != "" {
			b, err := input(o, in)
			if err != nil {
				return nil, err
			}
			value, err := validateInput("case.run", b)
			if err != nil {
				return nil, err
			}
			overrides = value.(CaseRunInput).Variables
		}
		if o.module != "" || o.url != "" || o.method != "" {
			return nil, project.Fail("invalid_input", "saved runs accept variable overrides only")
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return project.SendCase(ctx, root, o.env, o.caseSelector, overrides, project.SendOptions{Credentials: o.credentials})
	}
	b, err := input(o, in)
	if err != nil {
		return nil, err
	}
	var d project.RequestDraft
	if err = project.Decode(b, &d); err != nil {
		return nil, err
	}
	if o.module != "" {
		d.Module = o.module
	}
	if o.url != "" {
		d.URL = o.url
	}
	if o.method != "" {
		d.Method = o.method
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return project.Send(ctx, root, o.env, d, project.SendOptions{Credentials: o.credentials})
}
