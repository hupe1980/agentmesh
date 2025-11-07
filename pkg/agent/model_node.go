package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// modelNodeOptions holds configuration for a model node.
type modelNodeOptions struct {
	nodeName  string
	callbacks *callbacks.Manager
}

// ModelNodeOption configures a model node.
type ModelNodeOption func(*modelNodeOptions)

// WithModelNodeName sets the name of the model node (default: "model").
func WithModelNodeName(name string) ModelNodeOption {
	return func(c *modelNodeOptions) {
		c.nodeName = name
	}
}

// WithModelCallbacks sets the callback manager for the model node.
// Callbacks enable intercepting and modifying model invocations for guardrails,
// caching, metrics, and other cross-cutting concerns.
func WithModelCallbacks(cb *callbacks.Manager) ModelNodeOption {
	return func(c *modelNodeOptions) {
		c.callbacks = cb
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
//
// With callbacks:
//
//	cb := callbacks.NewManager()
//	cb.RegisterBeforeModel(guardrails.BlockUnsafeContent)
//	cb.RegisterAfterModel(guardrails.FilterPII)
//	g.AddNode(ModelNode(myModel, WithModelCallbacks(cb)))
func ModelNode(mdl model.Model, opts ...ModelNodeOption) *graph.Node {
	config := modelNodeOptions{
		nodeName:  "model",
		callbacks: nil,
	}

	for _, opt := range opts {
		opt(&config)
	}

	return &graph.Node{
		Name: config.nodeName,
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			messages := s.MessagesSnapshot()

			// Execute BeforeModel callbacks
			if config.callbacks != nil && config.callbacks.HasBeforeModelCallbacks() {
				result, err := config.callbacks.ExecuteBeforeModel(ctx, messages)
				if err != nil {
					return nil, err
				}
				if result != nil {
					// Short-circuit: use callback response instead of calling model
					return &graph.NodeResult{
						Messages: []message.Message{result},
						Updates:  map[string]any{},
					}, nil
				}
			}

			// Call the model
			msg, err := mdl.Generate(ctx, messages)
			if err != nil {
				// Execute OnModelError callbacks
				if config.callbacks != nil && config.callbacks.HasOnModelErrorCallbacks() {
					fallback, cbErr := config.callbacks.ExecuteOnModelError(ctx, messages, err)
					if cbErr != nil {
						err = cbErr // Use transformed error
					}
					if fallback != nil {
						// Callback provided fallback response
						return &graph.NodeResult{
							Messages: []message.Message{fallback},
							Updates:  map[string]any{},
						}, nil
					}
				}
				return nil, err
			}

			// Execute AfterModel callbacks
			if config.callbacks != nil && config.callbacks.HasAfterModelCallbacks() {
				transformed, err := config.callbacks.ExecuteAfterModel(ctx, messages, msg)
				if err != nil {
					return nil, err
				}
				if transformed != nil {
					msg = transformed
				}
			}

			return &graph.NodeResult{
				Messages: []message.Message{msg},
				Updates:  map[string]any{},
			}, nil
		},
	}
}
