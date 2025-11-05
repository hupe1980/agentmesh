package langchaingo

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/tool"
	lg "github.com/tmc/langchaingo/tools"
)

// Options configures the langchaingo tool adapter.
type Options struct {
	Name        string
	Description string
}

// WithName sets a custom name for the tool.
func WithName(name string) func(*Options) {
	return func(o *Options) {
		o.Name = name
	}
}

// WithDescription sets a custom description for the tool.
func WithDescription(description string) func(*Options) {
	return func(o *Options) {
		o.Description = description
	}
}

// NewTool creates a tool.Tool that wraps the provided langchaingo Tool.
// The tool is configured with the provided options. If Name or Description
// are not set in options, they default to the values from the langchaingo Tool.
func NewTool(t lg.Tool, optFns ...func(o *Options)) (*tool.FuncTool[ToolArgs, any], error) {
	opts := Options{
		Name:        t.Name(),
		Description: t.Description(),
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return tool.NewFuncTool(
		opts.Name,
		opts.Description,
		func(ctx context.Context, args ToolArgs) (any, error) {
			return t.Call(ctx, args.Input)
		},
	)
}

// ToolArgs defines the arguments for langchaingo tools.
type ToolArgs struct {
	Input string `json:"input" jsonschema:"title=Tool input,description=The input to pass to the tool,required"`
}
