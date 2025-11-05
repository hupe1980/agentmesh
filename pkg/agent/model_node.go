package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// modelNodeOptions holds configuration for a model node.
type modelNodeOptions struct {
	nodeName string
}

// ModelNodeOption configures a model node.
type ModelNodeOption func(*modelNodeOptions)

// WithModelNodeName sets the name of the model node (default: "model").
func WithModelNodeName(name string) ModelNodeOption {
	return func(c *modelNodeOptions) {
		c.nodeName = name
	}
}

// ModelNode creates a reusable graph node that generates responses using the provided model.
// The node takes the current message history from the state and produces a new AI message.
//
// This component is commonly used in agent implementations to delegate response generation
// to a language model. It automatically handles the conversion between state and model inputs/outputs.
//
// Example:
//
//	g.AddNode(ModelNode(myModel))
//	g.AddNode(ModelNode(myModel, WithModelNodeName("generator")))
func ModelNode(mdl model.Model, opts ...ModelNodeOption) *graph.Node {
	config := modelNodeOptions{
		nodeName: "model",
	}

	for _, opt := range opts {
		opt(&config)
	}

	return &graph.Node{
		Name: config.nodeName,
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			msg, err := mdl.Generate(ctx, s.MessagesSnapshot())
			if err != nil {
				return nil, err
			}

			return &graph.NodeResult{
				Messages: []message.Message{msg},
				Updates:  map[string]any{},
			}, nil
		},
	}
}
