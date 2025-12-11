package integration_test

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// MessagesKey is a convenience alias for agent.MessagesKey
var MessagesKey = agent.MessagesKey

// Common keys used across integration tests
var (
	ResultKey    = graph.NewKey[string]("result", "")
	CountKey     = graph.NewKey[int]("count", 0)
	ExecutedKey  = graph.NewKey[bool]("executed", false)
	CompletedKey = graph.NewKey[bool]("completed", false)
)

// buildSimpleGraph creates a simple single-node graph for testing
func buildSimpleGraph() (*graph.Graph[any, any], error) {
	g := graph.New[any, any](ResultKey)
	g.Node("test", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		return graph.Set(ResultKey, "done").End()
	}, graph.END)
	g.Start("test")
	return g.Build()
}

// buildMessageGraph creates a graph that works with messages
func buildMessageGraph() (*message.Graph, error) {
	g := message.NewGraphBuilder()
	g.Node("process", func(ctx context.Context, scope graph.Scope[message.Message]) (*graph.Command, error) {
		var msg message.Message = message.NewAIMessageFromText("processed")
		return graph.Append(message.MessagesKey, msg).End()
	}, graph.END)
	g.Start("process")
	return g.Build()
}
