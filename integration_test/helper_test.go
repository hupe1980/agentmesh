package integration_test

import "github.com/hupe1980/agentmesh/pkg/state"

func newTestState() *state.State {
	st := state.NewState()
	state.RegisterList(st, state.MessagesKey)
	return st
}
