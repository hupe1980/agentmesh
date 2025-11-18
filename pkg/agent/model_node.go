package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// modelNodeOptions holds configuration for a model node.
type modelNodeOptions struct {
	nodeName     string
	callbacks    *callbacks.PluginManager
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

// WithModelCallbacks sets the plugin manager for the model node.
// Plugins enable intercepting and modifying model invocations for guardrails,
// caching, metrics, and other cross-cutting concerns.
func WithModelCallbacks(cb *callbacks.PluginManager) ModelNodeOption {
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
// With plugins:
//
//	pm := callbacks.NewPluginManager()
//	pm.Register(ctx, guardrails.NewBlockUnsafeContentPlugin())
//	pm.Register(ctx, guardrails.NewFilterPIIPlugin())
//	g.AddNode(ModelNode(myModel, WithModelCallbacks(pm)))
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
		RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
			// Get messages from state
			messages := GetMessages(view)

			// Create request
			req := &model.Request{
				Messages:     messages,
				SystemPrompt: config.systemPrompt,
				Tools:        config.tools,
			}

			// Execute BeforeModel plugins
			if config.callbacks != nil && config.callbacks.HasPlugins() {
				resp, err := config.callbacks.ExecuteBeforeModel(ctx, req)
				if err != nil {
					return nil, err
				}
				if resp != nil {
					// Short-circuit: use plugin response instead of calling model
					updates := state.Updates{}
					AppendMessages(updates, []message.Message{resp.Message})

					return &graph.NodeResult{
						Updates: updates,
					}, nil
				}
			}

			// Call the model
			resp, err := model.Last(mdl.Generate(ctx, req))
			if err != nil {
				return handleModelError(ctx, req, err, config)
			}

			// Execute AfterModel plugins
			if config.callbacks != nil && config.callbacks.HasPlugins() {
				transformed, err := config.callbacks.ExecuteAfterModel(ctx, req, resp)
				if err != nil {
					return nil, err
				}
				if transformed != nil {
					resp = transformed
				}
			}

			// Return message in updates map (agent layer handles message storage)
			updates := state.Updates{}
			AppendMessages(updates, []message.Message{resp.Message})

			return &graph.NodeResult{
				Updates: updates,
			}, nil
		},
	}
}

// handleModelError processes model execution errors through plugins.
func handleModelError(ctx context.Context, req *model.Request, err error, config modelNodeOptions) (*graph.NodeResult, error) {
	// Execute OnModelError plugins
	if config.callbacks != nil && config.callbacks.HasPlugins() {
		fallback, transformedErr := config.callbacks.ExecuteOnModelError(ctx, req, err)
		if fallback != nil {
			// Plugin provided fallback response
			updates := state.Updates{}
			AppendMessages(updates, []message.Message{fallback.Message})

			return &graph.NodeResult{
				Updates: updates,
			}, nil
		}
		if transformedErr != nil {
			return nil, transformedErr
		}
	}
	return nil, err
}
