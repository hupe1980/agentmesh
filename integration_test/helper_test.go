package integration_test

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// Common keys used across integration tests
var (
	ResultKey    = graph.NewKey[string]("result")
	CountKey     = graph.NewKey[int]("count")
	ExecutedKey  = graph.NewKey[bool]("executed")
	CompletedKey = graph.NewKey[bool]("completed")
)

// buildSimpleGraph creates a simple single-node graph for testing
func buildSimpleGraph() (*graph.Graph, error) {
	g := graph.New(ResultKey)
	g.Node("test", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(ResultKey, "done").End()
	}, graph.END)
	g.Start("test")
	return g.Build()
}

// buildMessageGraph creates a graph that works with messages
func buildMessageGraph() (*graph.Graph, error) {
	g := graph.New()
	g.Node("process", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		var msg message.Message = message.NewAIMessageFromText("processed")
		return graph.Reply(msg).End()
	}, graph.END)
	g.Start("process")
	return g.Build()
}
