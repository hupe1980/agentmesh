package model

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// GenerateFunc is a function type implementing the core.Model Generate behavior.
// It should return two channels: one for streaming ModelResponse chunks and one
// for a terminal error (if any). Implementations must close both channels.
type GenerateFunc func(ctx context.Context, req *core.ModelRequest) (<-chan *core.ModelResponse, <-chan error)

// FuncModelOptions holds the options for a FuncModel.
type FuncModelOptions struct {
	// Capabilities defines the features supported by the model.
	Capabilities *core.ModelCapabilities
}

// FuncModel is a lightweight adapter allowing tests or custom code to supply
// generation logic as a first-class function alongside static ModelInfo.
type FuncModel struct {
	gen  GenerateFunc
	opts FuncModelOptions
}

// NewFuncModel constructs a new FuncModel. Panics if gen is nil to surface configuration errors early.
func NewFuncModel(gen GenerateFunc, optFns ...func(o *FuncModelOptions)) *FuncModel {
	if gen == nil {
		panic("FuncModel: nil GenerateFunc")
	}

	opts := FuncModelOptions{
		Capabilities: &core.ModelCapabilities{
			SupportsStructuredOutput: false,
		},
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &FuncModel{gen: gen, opts: opts}
}

// Generate delegates to the underlying function.
func (m *FuncModel) Generate(ctx context.Context, req *core.ModelRequest) (<-chan *core.ModelResponse, <-chan error) {
	return m.gen(ctx, req)
}

// Capabilities returns the advertised feature set for this functional model.
// Tests may mutate the capabilities directly to simulate provider behavior.
func (m *FuncModel) Capabilities() *core.ModelCapabilities {
	return m.opts.Capabilities
}

// Compile-time assertion that FuncModel implements core.Model.
var _ core.Model = (*FuncModel)(nil)
