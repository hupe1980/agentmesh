package agent

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/agent/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// runnableWithCallbacks wraps a MessageRunnable and automatically injects
// plugin manager callbacks into the context before execution.
// Note: Model and tool callbacks are injected directly at the node level,
// only graph-level callbacks (node and state) are injected via context.
type runnableWithCallbacks struct {
	inner         MessageRunnable
	pluginManager *callbacks.PluginManager
}

// Run executes the wrapped runnable with automatic callback injection.
func (r *runnableWithCallbacks) Run(ctx context.Context, input []message.Message, opts ...graph.RunOption) iter.Seq2[message.Message, error] {
	// Inject plugin manager into context so nodes can retrieve it
	ctx = callbacks.WithPluginManager(ctx, r.pluginManager)
	// Also inject graph-level callbacks
	ctx = graph.WithNodeCallbacks(ctx, r.pluginManager)
	ctx = graph.WithStateCallbacks(ctx, r.pluginManager)

	// Execute with enriched context
	return r.inner.Run(ctx, input, opts...)
}

// wrapWithCallbacks wraps a MessageRunnable with automatic callback injection if a plugin manager is provided.
func wrapWithCallbacks(runnable MessageRunnable, pm *callbacks.PluginManager) MessageRunnable {
	if pm == nil {
		return runnable
	}
	return &runnableWithCallbacks{
		inner:         runnable,
		pluginManager: pm,
	}
}
