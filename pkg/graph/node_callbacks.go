package graph

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// NodeCallbacks defines the interface for node lifecycle hooks.
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
//	ctx = graph.WithNodeCallbacks(ctx, pm)  // PluginManager implements NodeCallbacks
//
// This is not duplication - it's proper dependency inversion where the high-level
// module (graph) defines the contract it needs, and the low-level module (callbacks)
// implements it.
type NodeCallbacks interface {
	ExecuteBeforeNode(ctx context.Context, nodeName string, view *state.ReadView) (state.Updates, error)
	ExecuteAfterNode(ctx context.Context, nodeName string, view *state.ReadView, updates state.Updates) error
	ExecuteOnNodeError(ctx context.Context, nodeName string, err error) error
}

type nodeCallbacksKey struct{}

// WithNodeCallbacks adds node callbacks to the context.
func WithNodeCallbacks(ctx context.Context, callbacks NodeCallbacks) context.Context {
	return context.WithValue(ctx, nodeCallbacksKey{}, callbacks)
}

// getNodeCallbacks retrieves node callbacks from context.
func getNodeCallbacks(ctx context.Context) (NodeCallbacks, bool) {
	callbacks, ok := ctx.Value(nodeCallbacksKey{}).(NodeCallbacks)
	return callbacks, ok
}
