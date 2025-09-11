package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// mockAgent is a lightweight concrete agent for tests.
// It embeds BaseAgent, records the last RequestContext, and delegates Run to a provided function.
type mockAgent struct {
	*BaseAgent
	run func(context.Context, core.RequestContext, core.EventWriter) error

	received core.RequestContext
	runCount int
}

// newMockAgent creates a test agent with the given name and run function.
func newMockAgent(
	name string,
	run func(context.Context, core.RequestContext, core.EventWriter) error,
) *mockAgent {
	a := &mockAgent{run: run}
	a.BaseAgent = NewBaseAgent(a, name, "Agent "+name)
	return a
}

// Run implements core.Agent and captures invocation metadata.
func (g *mockAgent) Run(ctx context.Context, reqCtx core.RequestContext, q core.EventWriter) error {
	g.received = reqCtx
	g.runCount++
	if g.run != nil {
		return g.run(ctx, reqCtx, q)
	}
	return nil
}

// ReceivedCtx returns the last RequestContext seen by Run.
func (g *mockAgent) ReceivedCtx() core.RequestContext { return g.received }

// RunCount returns how many times Run has been called.
func (g *mockAgent) RunCount() int { return g.runCount }
