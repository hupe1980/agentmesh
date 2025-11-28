package integration_test

import (
	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// newTestManagerBuilder returns a ManagerBuilder with MessagesKey pre-registered
func newTestManagerBuilder() *state.ManagerBuilder {
	builder := state.NewManagerBuilder()
	state.RegisterListKey(builder, agent.MessagesKey)
	return builder
}

// newTestManager returns a Manager with MessagesKey pre-registered
func newTestManager() *state.Manager {
	return newTestManagerBuilder().Build()
}
