package agent

import (
	"context"
	"fmt"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/agent/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// RunnableWithCallbacks wraps a MessageRunnable and automatically injects
// plugin manager callbacks into the context before execution.
// Note: Model and tool callbacks are injected directly at the node level,
// only graph-level callbacks (node and state) are injected via context.
type RunnableWithCallbacks struct {
	inner         MessageRunnable
	pluginManager *callbacks.PluginManager
}

// Run executes the wrapped runnable with automatic callback injection.
func (r *RunnableWithCallbacks) Run(ctx context.Context, input []message.Message, opts ...graph.RunOption) iter.Seq2[message.Message, error] {
	// Inject plugin manager into context so nodes can retrieve it
	ctx = callbacks.WithPluginManager(ctx, r.pluginManager)
	// Also inject graph-level callbacks
	ctx = graph.WithNodeCallbacks(ctx, r.pluginManager)
	ctx = graph.WithStateCallbacks(ctx, r.pluginManager)

	// Execute with enriched context
	return r.inner.Run(ctx, input, opts...)
}

// Unwrap returns the inner MessageRunnable for introspection
func (r *RunnableWithCallbacks) Unwrap() MessageRunnable {
	return r.inner
}

// Delegate introspection methods using clean interface checks

// Graph returns the underlying graph if the inner runnable supports it
func (r *RunnableWithCallbacks) Graph() *graph.Graph {
	if provider, ok := r.inner.(GraphProvider); ok {
		return provider.Graph()
	}
	return nil
}

// GetNodes returns the list of node names if supported
func (r *RunnableWithCallbacks) GetNodes() []string {
	if provider, ok := r.inner.(TopologyProvider); ok {
		return provider.GetNodes()
	}
	return nil
}

// GetTopology returns the graph topology if supported
func (r *RunnableWithCallbacks) GetTopology() *graph.Topology {
	if provider, ok := r.inner.(TopologyProvider); ok {
		return provider.GetTopology()
	}
	return nil
}

// GetNodeInfo returns detailed node information if supported
func (r *RunnableWithCallbacks) GetNodeInfo(name string) (*graph.NodeInfo, error) {
	if introspector, ok := r.inner.(NodeIntrospector); ok {
		return introspector.GetNodeInfo(name)
	}
	return nil, fmt.Errorf("node introspection not supported")
}

// GetMetrics returns execution metrics if supported
func (r *RunnableWithCallbacks) GetMetrics() *graph.Metrics {
	if provider, ok := r.inner.(MetricsProvider); ok {
		return provider.GetMetrics()
	}
	return nil
}

// GetNodeDependencies returns node dependencies if supported
func (r *RunnableWithCallbacks) GetNodeDependencies(name string) (*graph.NodeDependencies, error) {
	if introspector, ok := r.inner.(NodeIntrospector); ok {
		return introspector.GetNodeDependencies(name)
	}
	return nil, fmt.Errorf("node introspection not supported")
}

// MermaidFlowchart generates a Mermaid diagram if supported
func (r *RunnableWithCallbacks) MermaidFlowchart(direction string) string {
	if generator, ok := r.inner.(DiagramGenerator); ok {
		return generator.MermaidFlowchart(direction)
	}
	return "graph LR\n    Start[Agent] --> End[Output]"
}

// WrapWithCallbacks wraps a MessageRunnable with automatic callback injection if a plugin manager is provided.
// This is useful for adding plugin support to custom runnables or agents.
func WrapWithCallbacks(runnable MessageRunnable, pm *callbacks.PluginManager) MessageRunnable {
	if pm == nil {
		return runnable
	}
	return &RunnableWithCallbacks{
		inner:         runnable,
		pluginManager: pm,
	}
}
