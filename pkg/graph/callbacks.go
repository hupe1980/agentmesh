package graph

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// ============================================================================
// Node Callbacks
// ============================================================================

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
type NodeCallbacks interface {
	// ExecuteBeforeNode is called before a node executes.
	// Can return a Command to short-circuit node execution (node won't run).
	// The Command must include valid routing (Goto targets).
	// Return nil to proceed with normal node execution.
	ExecuteBeforeNode(ctx context.Context, nodeName string, view state.ReadView) (*Command, error)
	ExecuteAfterNode(ctx context.Context, nodeName string, view state.ReadView, updates state.Updates) error
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

// ============================================================================
// State Callbacks
// ============================================================================

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
