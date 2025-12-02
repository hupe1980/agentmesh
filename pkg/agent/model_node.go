package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// ModelNodeConfig holds configuration for creating a model node function.
type ModelNodeConfig struct {
	Executor     model.Executor
	SystemPrompt string
	Tools        []tool.Tool
	OutputSchema *schema.OutputSchema
	ToolTarget   string // Target node when tool calls are present (default: "tool")
}

// ModelNodeOption configures a ModelNodeConfig.
type ModelNodeOption func(*ModelNodeConfig)

// WithModelSystemPrompt sets a system prompt for this model node.
// The system prompt is sent per-request and not stored in conversation state.
func WithModelSystemPrompt(prompt string) ModelNodeOption {
	return func(c *ModelNodeConfig) {
		c.SystemPrompt = prompt
	}
}

// WithModelTools sets the tools available to the model for this node.
// The tools are passed to the model along with the request.
func WithModelTools(tools ...tool.Tool) ModelNodeOption {
	return func(c *ModelNodeConfig) {
		c.Tools = tools
	}
}

// WithOutputSchema sets a structured output schema for the model.
// The schema constrains the model to generate valid JSON matching the schema.
// Only works with models that support structured output (check model.Capabilities().StructuredOutput).
func WithOutputSchema(outputSchema *schema.OutputSchema) ModelNodeOption {
	return func(c *ModelNodeConfig) {
		c.OutputSchema = outputSchema
	}
}

// WithToolTarget sets the target node when tool calls are present.
// Default is "tool".
func WithToolTarget(target string) ModelNodeOption {
	return func(c *ModelNodeConfig) {
		c.ToolTarget = target
	}
}

// NewModelNodeFunc creates a graph.NodeFunc that executes a model.
//
// The function:
//   - Extracts messages from state
//   - Builds a Request with messages + configuration (system prompt, tools, schema)
//   - Delegates execution to the provided Executor
//   - Routes based on tool calls in the response
//
// Routing logic:
//   - If the AI message contains tool calls -> routes to tool target (default: "tool")
//   - Otherwise -> routes to END
//
// Example:
//
//	executor := model.NewExecutor(myModel, model.WithExecutorName("gpt-4"))
//	modelFn, err := agent.NewModelNodeFunc(executor,
//	    agent.WithModelSystemPrompt("You are a helpful assistant"),
//	    agent.WithModelTools(searchTool, calculatorTool))
//
//	g.Node("model", modelFn, "tool", graph.END)
func NewModelNodeFunc(executor model.Executor, opts ...ModelNodeOption) (graph.NodeFunc, error) {
	if err := validate.NotNil(executor, "executor"); err != nil {
		return nil, err
	}

	cfg := &ModelNodeConfig{
		Executor:   executor,
		ToolTarget: "tool",
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return func(ctx context.Context, view graph.View) (*graph.Command, error) {
		// Get messages from state using type-safe key
		messages := GetMessages(view)

		// Build request with messages + node configuration
		req := &model.Request{
			Messages:     messages,
			SystemPrompt: cfg.SystemPrompt,
			Tools:        cfg.Tools,
			OutputSchema: cfg.OutputSchema,
		}

		// Execute via the executor
		resp, err := model.Last(cfg.Executor.Generate(ctx, req))
		if err != nil {
			return graph.Fail(err)
		}

		// Route based on tool calls
		if message.HasToolCalls(resp.Message) {
			return graph.Append(MessagesKey, resp.Message).To(cfg.ToolTarget)
		}

		return graph.Append(MessagesKey, resp.Message).To(graph.END)
	}, nil
}
