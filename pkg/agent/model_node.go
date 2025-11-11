package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// modelNodeOptions holds configuration for a model node.
type modelNodeOptions struct {
	nodeName     string
	callbacks    *callbacks.Manager
	systemPrompt string
	tools        []tool.Tool
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

// WithModelSystemPrompt sets a system prompt for this model node.
// The system prompt is sent per-request and not stored in conversation state.
func WithModelSystemPrompt(prompt string) ModelNodeOption {
	return func(c *modelNodeOptions) {
		c.systemPrompt = prompt
	}
}

// WithModelTools sets the tools available to the model for this node.
// The tools are passed to the model along with the request.
func WithModelTools(tools ...tool.Tool) ModelNodeOption {
	return func(c *modelNodeOptions) {
		c.tools = tools
	}
}

// handleModelError handles model errors with callbacks and returns fallback message if provided.
// Returns (fallbackMessage, transformedError).
func handleModelError(ctx context.Context, s graph.StateWriter, err error, config *modelNodeOptions) (message.Message, error) {
	if config.callbacks == nil || !config.callbacks.HasOnModelErrorCallbacks() {
		return nil, err
	}

	fallback, cbErr := config.callbacks.ExecuteOnModelError(ctx, s, err)
	if cbErr != nil {
		return nil, cbErr
	}

	return fallback, err
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
			// Execute BeforeModel callbacks
			if config.callbacks != nil && config.callbacks.HasBeforeModelCallbacks() {
				result, err := config.callbacks.ExecuteBeforeModel(ctx, s)
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

			// Get messages for model invocation
			events := s.EventsSnapshot()

			// Create request
			req := &model.Request{
				Messages:     graph.ExtractMessages(events),
				SystemPrompt: config.systemPrompt,
				Tools:        config.tools,
			}

			// Call the model
			resp, err := model.Last(mdl.Generate(ctx, req))
			if err != nil {
				fallback, transformedErr := handleModelError(ctx, s, err, &config)
				if transformedErr != nil {
					return nil, transformedErr
				}
				if fallback != nil {
					// Callback provided fallback response
					return &graph.NodeResult{
						Messages: []message.Message{fallback},
						Updates:  map[string]any{},
					}, nil
				}
				return nil, err
			}

			// Execute AfterModel callbacks
			msg := resp.Message
			if config.callbacks != nil && config.callbacks.HasAfterModelCallbacks() {
				transformed, err := config.callbacks.ExecuteAfterModel(ctx, s, msg)
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
