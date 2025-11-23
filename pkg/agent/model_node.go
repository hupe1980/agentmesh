package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/agent/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// ModelNode is a graph node that executes a model.Executor to generate responses.
//
// The ModelNode is a thin orchestration layer that:
//   - Extracts messages from state
//   - Builds a Request with messages + configuration (system prompt, tools, schema)
//   - Delegates execution to the provided Executor
//   - Routes based on tool calls in the response
//
// Routing logic:
//   - If the AI message contains tool calls -> routes to "tool" node
//   - Otherwise -> routes to END
//
// The Executor handles execution concerns:
//   - Plugin lifecycle (BeforeModel, AfterModel, OnModelError)
//   - Observability (tracing, metrics, logging)
//   - Streaming support
//
// Configuration (system prompt, tools, output schema) is stored in the node
// and used to build the Request on each execution.
//
// Example:
//
//	executor := model.NewExecutor(myModel, model.WithExecutorName("gpt-4"))
//	node, err := agent.NewModelNode(executor,
//	    agent.WithModelSystemPrompt("You are a helpful assistant"),
//	    agent.WithModelTools(searchTool, calculatorTool))
type ModelNode struct {
	name         string
	executor     model.Executor
	systemPrompt string
	tools        []tool.Tool
	outputSchema *schema.OutputSchema
	targets      []string
}

// ModelNodeOption configures a ModelNode.
type ModelNodeOption func(*ModelNode)

// WithModelNodeName sets the name of the model node (default: "model").
func WithModelNodeName(name string) ModelNodeOption {
	return func(n *ModelNode) {
		n.name = name
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

// WithOutputSchema sets a structured output schema for the model.
// The schema constrains the model to generate valid JSON matching the schema.
// Only works with models that support structured output (check model.Capabilities().StructuredOutput).
func WithOutputSchema(outputSchema *schema.OutputSchema) ModelNodeOption {
	return func(n *ModelNode) {
		n.outputSchema = outputSchema
	}
}

// WithModelTargets sets the possible routing targets for this node.
// Default is []string{"tool", graph.EndNode}.
func WithModelTargets(targets []string) ModelNodeOption {
	return func(n *ModelNode) {
		n.targets = targets
	}
}

// NewModelNode creates a new model node that executes the provided executor.
//
// The executor encapsulates all model execution logic including configuration,
// plugins, and observability. This allows for flexible executor implementations
// that can be swapped without modifying the node.
//
// Example:
//
//	executor := model.NewExecutor(myModel,
//	    model.WithExecutorName("assistant"))
//	node, err := agent.NewModelNode(executor,
//	    agent.WithModelNodeName("model"),
//	    agent.WithModelTargets([]string{"tool", graph.EndNode}))
func NewModelNode(executor model.Executor, opts ...ModelNodeOption) (*ModelNode, error) {
	if executor == nil {
		return nil, fmt.Errorf("executor cannot be nil")
	}

	node := &ModelNode{
		name:     "model",
		executor: executor,
		targets:  []string{"tool", graph.EndNode}, // Default targets
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

// Targets returns the possible routing destinations for this node.
func (n *ModelNode) Targets() []string {
	if len(n.targets) > 0 {
		return n.targets
	}
	// Default targets for backward compatibility
	return []string{"tool", graph.EndNode}
}

// Execute runs the model node logic by delegating to the executor.
func (n *ModelNode) Execute(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
	// Get messages from state
	messages := GetMessages(view)

	// Build request with messages + node configuration
	req := &model.Request{
		Messages:     messages,
		SystemPrompt: n.systemPrompt,
		Tools:        n.tools,
		OutputSchema: n.outputSchema,
	}

	// Inject plugin manager from context into executor context
	if pm := callbacks.FromContext(ctx); pm != nil {
		ctx = model.WithPlugin(ctx, pm)
	}

	// Execute via the executor - it handles plugins, observability, streaming, etc.
	resp, err := model.Last(n.executor.Generate(ctx, req))
	if err != nil {
		return nil, err
	}

	// Return message in updates map (agent layer handles message storage)
	builder := state.NewUpdateBuilder()
	state.AppendUpdate(builder, MessagesKey, resp.Message)
	updates, _ := builder.Build()

	// Route based on tool calls
	if aiMsg, ok := resp.Message.(*message.AIMessage); ok && len(aiMsg.ToolCalls) > 0 {
		return graph.Goto("tool", updates), nil
	}
	return graph.End(updates), nil
}
