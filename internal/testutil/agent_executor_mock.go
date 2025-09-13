package testutil

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// AgentExecutorMock implements core.AgentExecutor for tests.
type AgentExecutorMock struct {
	ExecuteFunc func(ctx context.Context, rc core.RequestContext, ag core.Agent, w core.EventWriter) error
	Calls       int
}

func (m *AgentExecutorMock) Execute(
	ctx context.Context,
	rc core.RequestContext,
	ag core.Agent,
	w core.EventWriter,
) error {
	m.Calls++
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, rc, ag, w)
	}
	return ag.Run(ctx, rc, w)
}

// NewAgentExecutorMock returns a mock executor that delegates when ExecuteFunc is nil.
func NewAgentExecutorMock() *AgentExecutorMock { return &AgentExecutorMock{} }

var _ core.AgentExecutor = (*AgentExecutorMock)(nil)
