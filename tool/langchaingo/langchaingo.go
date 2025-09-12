package langchaingo

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/tool"
	lg "github.com/tmc/langchaingo/tools"
)

// Options configures the langchain tool adapter.
type Options struct {
	Name        string
	Description string
}

// New creates a core.Tool that wraps the provided langchaingo Tool.
// The tool is configured with the provided options. If Name or Description
// are not set in options, they default to the values from the langchaingo Tool.
func New(t lg.Tool, optFns ...func(o *Options)) core.Tool {
	opts := Options{
		Name:        t.Name(),
		Description: t.Description(),
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"__arg1": map[string]string{"title": "__arg1", "type": "string"},
		},
		"required": []string{"__arg1"},
	}

	return tool.NewFuncTool(
		opts.Name,
		opts.Description,
		params,
		func(ctx context.Context, tc core.ToolContext, args map[string]any) (any, error) {
			toolInput, ok := args["__arg1"].(string)
			if !ok {
				return nil, tool.NewError(opts.Name, "missing required field '__arg1'", "VALIDATION_ERROR")
			}

			return t.Call(ctx, toolInput)
		},
	)
}
