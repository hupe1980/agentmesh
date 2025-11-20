package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// ModelNode is a reusable graph node that generates responses using a language model.
// It implements the graph.Node interface and handles conversion between state and model inputs/outputs.
type ModelNode struct {
	name         string
	model        model.Model
	callbacks    *callbacks.PluginManager
	systemPrompt string
	tools        []tool.Tool
}

// ModelNodeOption configures a ModelNode.
type ModelNodeOption func(*ModelNode)

// WithModelNodeName sets the name of the model node (default: "model").
func WithModelNodeName(name string) ModelNodeOption {
	return func(n *ModelNode) {
		n.name = name
	}
}

// WithModelCallbacks sets the plugin manager for the model node.
// Plugins enable intercepting and modifying model invocations for guardrails,
// caching, metrics, and other cross-cutting concerns.
func WithModelCallbacks(cb *callbacks.PluginManager) ModelNodeOption {
	return func(n *ModelNode) {
		n.callbacks = cb
	}
}

// WithModelSystemPrompt sets a system prompt for this model node.
// The system prompt is sent per-request and not stored in conversation state.
func WithModelSystemPrompt(prompt string) ModelNodeOption {
	return func(n *ModelNode) {
		n.systemPrompt = prompt
	}
}

// WithModelTools sets the tools available to the model for this node.
// The tools are passed to the model along with the request.
func WithModelTools(tools ...tool.Tool) ModelNodeOption {
	return func(n *ModelNode) {
		n.tools = tools
	}
}

// NewModelNode creates a reusable graph node that generates responses using the provided model.
// The node takes the current message history from the state and produces a new AI message.
//
// This component is commonly used in agent implementations to delegate response generation
// to a language model. It automatically handles the conversion between state and model inputs/outputs.
//
// Returns an error if the model parameter is nil.
//
// Example:
//
//	node, err := NewModelNode(myModel)
//	node, err := NewModelNode(myModel, WithModelNodeName("generator"))
//
// With plugins:
//
//	pm := callbacks.NewPluginManager()
//	pm.Register(ctx, guardrails.NewBlockUnsafeContentPlugin())
//	pm.Register(ctx, guardrails.NewFilterPIIPlugin())
//	node, err := NewModelNode(myModel, WithModelCallbacks(pm))
func NewModelNode(mdl model.Model, opts ...ModelNodeOption) (*ModelNode, error) {
	if mdl == nil {
		return nil, fmt.Errorf("agent: model cannot be nil")
	}

	node := &ModelNode{
		name:  "model",
		model: mdl,
	}

	for _, opt := range opts {
		opt(node)
	}

	return node, nil
}

// Name returns the node's name.
func (n *ModelNode) Name() string {
	return n.name
}

// Execute runs the model node logic.
func (n *ModelNode) Execute(ctx context.Context, view *state.ReadView) (state.Updates, error) {
	// Get messages from state
	messages := GetMessages(view)

	// Create request
	req := &model.Request{
		Messages:     messages,
		SystemPrompt: n.systemPrompt,
		Tools:        n.tools,
	}

	// Execute BeforeModel plugins
	if n.callbacks != nil && n.callbacks.HasPlugins() {
		resp, err := n.callbacks.ExecuteBeforeModel(ctx, req)
		if err != nil {
			return nil, err
		}
		if resp != nil {
			// Short-circuit: use plugin response instead of calling model
			builder := state.NewUpdateBuilder()
			state.AppendUpdate(builder, MessagesKey, resp.Message)
			return builder.Build()
		}
	}

	// Call the model
	resp, err := model.Last(n.model.Generate(ctx, req))
	if err != nil {
		return n.handleModelError(ctx, req, err)
	}

	// Execute AfterModel plugins
	if n.callbacks != nil && n.callbacks.HasPlugins() {
		transformed, err := n.callbacks.ExecuteAfterModel(ctx, req, resp)
		if err != nil {
			return nil, err
		}
		if transformed != nil {
			resp = transformed
		}
	}

	// Return message in updates map (agent layer handles message storage)
	builder := state.NewUpdateBuilder()
	state.AppendUpdate(builder, MessagesKey, resp.Message)

	return builder.Build()
}

// handleModelError processes model execution errors through plugins.
func (n *ModelNode) handleModelError(ctx context.Context, req *model.Request, err error) (state.Updates, error) {
	// Execute OnModelError plugins
	if n.callbacks != nil && n.callbacks.HasPlugins() {
		fallback, transformedErr := n.callbacks.ExecuteOnModelError(ctx, req, err)
		if fallback != nil {
			// Plugin provided fallback response
			builder := state.NewUpdateBuilder()
			state.AppendUpdate(builder, MessagesKey, fallback.Message)
			return builder.Build()
		}
		if transformedErr != nil {
			return nil, transformedErr
		}
	}
	return nil, err
}
