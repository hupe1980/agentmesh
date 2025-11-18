package integration_test

import (
	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func newTestManager() state.Manager {
	mgr := state.NewManager()
	state.RegisterListKey(mgr, agent.MessagesKey)
	return mgr
}
