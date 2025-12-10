package agent

import (
	"context"
	"fmt"

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
	Instructions *Instructions // Dynamic instructions (supports templates and providers)
	Tools        []tool.Tool   // Static tools for this node
	Toolset      tool.Toolset  // Dynamic toolset for runtime tool discovery
	OutputSchema *schema.OutputSchema
	ToolTarget   string // Target node when tool calls are present (default: "tool")
}

// ModelNodeOption configures a ModelNodeConfig.
type ModelNodeOption func(*ModelNodeConfig)

// WithModelInstructions sets instructions from a template string for this model node.
// Uses Go text/template syntax - placeholders like {{.userName}} are substituted from state.
func WithModelInstructions(templateStr string) ModelNodeOption {
	return func(c *ModelNodeConfig) {
		inst := NewInstructions(templateStr)
		c.Instructions = &inst
	}
}

// WithModelInstructionsFunc sets instructions from a dynamic function for this model node.
// Use when instructions need complex logic or access to graph state beyond template substitution.
func WithModelInstructionsFunc(f func(context.Context, graph.View) (string, error)) ModelNodeOption {
	return func(c *ModelNodeConfig) {
		inst := NewInstructionsFromFunc(f)
		c.Instructions = &inst
	}
}

// WithModelTools sets static tools available to the model for this node.
// For dynamic tool discovery, use WithModelToolset instead.
func WithModelTools(tools ...tool.Tool) ModelNodeOption {
	return func(c *ModelNodeConfig) {
		c.Tools = tools
	}
}

// WithModelToolset sets a dynamic toolset for runtime tool discovery.
// Tools are discovered on each invocation with access to the current graph state.
func WithModelToolset(ts tool.Toolset) ModelNodeOption {
	return func(c *ModelNodeConfig) {
		c.Toolset = ts
	}
}

// WithModelOutputSchema sets a structured output schema for the model node.
// The schema constrains the model to generate valid JSON matching the schema.
// Only works with models that support structured output (check model.Capabilities().StructuredOutput).
// For agent-level configuration, use WithOutputSchema instead.
func WithModelOutputSchema(outputSchema *schema.OutputSchema) ModelNodeOption {
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
//   - Discovers tools from the configured Toolset (or uses static Tools)
//   - Resolves instructions (supports templates with state placeholders)
//   - Collects and appends tool instructions from InstructionProvider tools
//   - Builds a Request with messages + configuration
//   - Delegates execution to the provided Executor
//   - Routes based on tool calls in the response
//
// Routing logic:
//   - If the AI message contains tool calls -> routes to tool target (default: "tool")
//   - Otherwise -> routes to END
//
// Example with static tools:
//
//	executor := model.NewExecutor(myModel, model.WithExecutorName("gpt-4"))
//	modelFn, err := agent.NewModelNodeFunc(executor,
//	    agent.WithModelInstructions("You are a helpful assistant"),
//	    agent.WithModelTools(searchTool, calculatorTool))
//
// Example with dynamic instructions:
//
//	modelFn, err := agent.NewModelNodeFunc(executor,
//	    agent.WithModelInstructions("You are helping {{.userName}}"),
//	    agent.WithModelToolset(mcpToolset))
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
		// Resolve tools: use Toolset if provided, otherwise fall back to static Tools
		var tools []tool.Tool
		if cfg.Toolset != nil {
			var err error
			tools, err = cfg.Toolset.ListTools(ctx, view)
			if err != nil {
				return graph.Fail(fmt.Errorf("failed to list tools: %w", err))
			}
		} else {
			tools = cfg.Tools
		}

		// Resolve base instructions if configured
		var instructions string
		if cfg.Instructions != nil {
			var err error
			instructions, err = cfg.Instructions.Resolve(ctx, view)
			if err != nil {
				return graph.Fail(fmt.Errorf("failed to resolve instructions: %w", err))
			}
		}

		// Collect and append tool instructions from InstructionProvider tools
		// (e.g., SetModelResponseTool adds instructions for structured output)
		if toolInstructions := tool.CollectInstructions(tools); toolInstructions != "" {
			if instructions != "" {
				instructions = instructions + "\n\n" + toolInstructions
			} else {
				instructions = toolInstructions
			}
		}

		// Get messages from state using type-safe key
		messages := GetMessages(view)

		// Build request with messages + node configuration
		req := &model.Request{
			Messages:     messages,
			Instructions: instructions,
			Tools:        tools,
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
