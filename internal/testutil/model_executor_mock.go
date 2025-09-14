package testutil

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// ModelExecutorMock implements core.ModelExecutor for tests with hookable behavior.
type ModelExecutorMock struct {
	ExecuteFunc func(
		ctx context.Context,
		rc core.RequestContext,
		m core.Model,
		req *core.ModelRequest,
	) (<-chan *core.ModelResponse, <-chan error)
	Calls int
}

func (m *ModelExecutorMock) Execute(
	ctx context.Context,
	rc core.RequestContext,
	mdl core.Model,
	req *core.ModelRequest,
) (<-chan *core.ModelResponse, <-chan error) {
	m.Calls++
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, rc, mdl, req)
	}
	// Default: immediate passthrough to model.Generate
	return mdl.Generate(ctx, req)
}

// NewModelExecutorMock constructs a ModelExecutorMock.
func NewModelExecutorMock() *ModelExecutorMock { return &ModelExecutorMock{} }

var _ core.ModelExecutor = (*ModelExecutorMock)(nil)
