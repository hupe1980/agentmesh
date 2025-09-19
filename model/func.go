package model

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// GenerateFunc is a function type implementing the core.Model Generate behavior.
// It should return two channels: one for streaming ModelResponse chunks and one
// for a terminal error (if any). Implementations must close both channels.
type GenerateFunc func(ctx context.Context, req *core.ModelRequest) (<-chan *core.ModelResponse, <-chan error)

// FuncModel is a lightweight adapter allowing tests or custom code to supply
// generation logic as a first-class function alongside static ModelInfo.
type FuncModel struct {
	gen GenerateFunc
}

// NewFuncModel constructs a new FuncModel. Panics if gen is nil to surface configuration errors early.
func NewFuncModel(gen GenerateFunc) *FuncModel {
	if gen == nil {
		panic("FuncModel: nil GenerateFunc")
	}
	return &FuncModel{gen: gen}
}

// Generate delegates to the underlying function.
func (m *FuncModel) Generate(ctx context.Context, req *core.ModelRequest) (<-chan *core.ModelResponse, <-chan error) {
	return m.gen(ctx, req)
}

// Compile-time assertion that FuncModel implements core.Model.
var _ core.Model = (*FuncModel)(nil)
