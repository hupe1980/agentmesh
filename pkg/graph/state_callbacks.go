package graph

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// StateCallbacks defines the interface for state change lifecycle hooks.
//
// Design Pattern: Dependency Inversion Principle
//
// This interface allows pkg/graph to use callbacks without importing pkg/callbacks,
// breaking the import cycle: graph ← callbacks ← model ← (cycle).
//
// The callbacks.PluginManager automatically satisfies this interface, so users can:
//
//	pm := callbacks.NewPluginManager()
//	pm.Register(ctx, myPlugin)
//	ctx = graph.WithStateCallbacks(ctx, pm)  // PluginManager implements StateCallbacks
//
// This is not duplication - it's proper dependency inversion where the high-level
// module (graph) defines the contract it needs, and the low-level module (callbacks)
// implements it.
type StateCallbacks interface {
	ExecuteOnStateChange(ctx context.Context, nodeName string, updates state.Updates) error
}

type stateCallbacksKey struct{}

// WithStateCallbacks adds state callbacks to the context.
func WithStateCallbacks(ctx context.Context, callbacks StateCallbacks) context.Context {
	return context.WithValue(ctx, stateCallbacksKey{}, callbacks)
}

// getStateCallbacks retrieves state callbacks from context.
func getStateCallbacks(ctx context.Context) (StateCallbacks, bool) {
	callbacks, ok := ctx.Value(stateCallbacksKey{}).(StateCallbacks)
	return callbacks, ok
}
