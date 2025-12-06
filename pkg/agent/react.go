package agent

import (
	"fmt"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// NewReAct creates a Reasoning and Acting (ReAct) agent that iteratively:
//  1. Reasons about the task
//  2. Decides which tool to use
//  3. Observes the result
//  4. Repeats until the answer is found
//
// Returns a Graph that processes message sequences and streams execution results.
//
// This pattern is effective for multi-step problem solving with tool use.
//
// You can provide tools in two ways:
//  1. Static list: use WithTools() option
//  2. Dynamic toolset: use WithToolset() option for runtime tool discovery
//
// Example with static tools:
//
//	agent, err := agent.NewReAct(model,
//	    agent.WithTools(searchTool, calculatorTool),
//	    agent.WithMaxIterations(5))
//
// Example with dynamic toolset:
//
//	mcpToolset := mcp.NewToolset(mcp.NewStdioSessionFactory("mcp-server", []string{}))
//	agt, err := agent.NewReAct(model,
//	    agent.WithToolset(mcpToolset),
//	    agent.WithMaxIterations(5))
func NewReAct(mdl model.Model, opts ...ReActOption) (*message.Graph, error) {
	if err := validate.NotNil(mdl, "model"); err != nil {
		return nil, err
	}

	config := defaultReActOptions()
	for _, opt := range opts {
		opt.applyReAct(&config)
	}

	// Build and validate tool registry
	tools, toolRegistry := buildToolRegistry(config.tools)

	// Validate model capabilities
	if err := validateModelCapabilities(mdl, tools); err != nil {
		return nil, err
	}

	// Create model node function
	modelFn, err := createModelNode(mdl, config, tools)
	if err != nil {
		return nil, err
	}

	// Create tool node function
	toolFn, err := createToolNode(toolRegistry, config)
	if err != nil {
		return nil, err
	}

	// Build and configure graph
	return buildReActGraph(modelFn, toolFn, config)
}

// buildToolRegistry constructs a deduplicated tool registry from the provided tools.
func buildToolRegistry(configTools []tool.Tool) ([]tool.Tool, map[string]tool.Tool) {
	toolRegistry := make(map[string]tool.Tool, len(configTools))
	for _, t := range configTools {
		if t == nil {
			continue
		}
		toolRegistry[t.Name()] = t
	}

	tools := make([]tool.Tool, 0, len(toolRegistry))
	for _, t := range toolRegistry {
		tools = append(tools, t)
	}

	return tools, toolRegistry
}

// validateModelCapabilities checks if the model supports tools when tools are provided.
func validateModelCapabilities(mdl model.Model, tools []tool.Tool) error {
	if len(tools) > 0 {
		caps := mdl.Capabilities()
		if !caps.Tools {
			return fmt.Errorf("agent/react: model does not support tools (%d tools provided but Capabilities().Tools is false)", len(tools))
		}
	}
	return nil
}

// createModelNode creates and configures the model node function with middleware.
func createModelNode(mdl model.Model, config reActOptions, tools []tool.Tool) (graph.NodeFunc, error) {
	// Create model executor - encapsulates model lifecycle management
	// Apply model middleware if provided
	modelExecutor := model.NewExecutor(mdl, model.WithExecutorName("react-model"))
	if len(config.modelMiddleware) > 0 {
		modelExecutor = model.Chain(modelExecutor, config.modelMiddleware...)
	}

	// Model node function
	modelFn, err := NewModelNodeFunc(modelExecutor,
		WithModelSystemPrompt(config.systemPrompt),
		WithModelTools(tools...),
		WithOutputSchema(config.outputSchema),
	)
	if err != nil {
		return nil, fmt.Errorf("agent/react: create model node: %w", err)
	}

	return modelFn, nil
}

// createToolNode creates and configures the tool node function with middleware.
func createToolNode(toolRegistry map[string]tool.Tool, config reActOptions) (graph.NodeFunc, error) {
	// Create tool executor - use sequential by default for deterministic behavior
	// Apply tool middleware if provided
	toolExecutor := tool.NewSequentialExecutor(toolRegistry,
		tool.WithErrorPrefix("react agent"),
		tool.WithContinueOnError(false))
	if len(config.toolMiddleware) > 0 {
		toolExecutor = tool.Chain(toolExecutor, config.toolMiddleware...)
	}

	// Tool node function
	toolFn, err := NewToolNodeFunc(toolExecutor)
	if err != nil {
		return nil, fmt.Errorf("agent/react: create tool node: %w", err)
	}

	return toolFn, nil
}

// buildReActGraph constructs the ReAct agent graph with nodes and middleware.
func buildReActGraph(modelFn, toolFn graph.NodeFunc, config reActOptions) (*message.Graph, error) {
	// Build graph - MessagesKey is automatically included by message.NewGraphBuilder
	b := message.NewGraphBuilder()
	b.Node("model", modelFn, "tool", graph.END)
	b.Node("tool", toolFn, "model")
	b.Start("model")

	// Apply graph middleware if provided
	if len(config.graphMiddleware) > 0 {
		b.WithMiddleware(config.graphMiddleware...)
	}

	return b.Build()
}

// reActOptions holds configuration for ReAct agents.
type reActOptions struct {
	commonOptions
	tools        []tool.Tool
	outputSchema *schema.OutputSchema
}

func defaultReActOptions() reActOptions {
	return reActOptions{
		commonOptions: commonOptions{
			systemPrompt:    "",
			maxIterations:   10,
			graphMiddleware: nil,
			modelMiddleware: nil,
			toolMiddleware:  nil,
		},
		tools:        nil,
		outputSchema: nil,
	}
}

// ReActOption configures a ReAct agent.
// It can be either a function or a sharedOption.
type ReActOption interface {
	applyReAct(*reActOptions)
}

// reActOptionFunc wraps a function to implement ReActOption.
type reActOptionFunc func(*reActOptions)

func (f reActOptionFunc) applyReAct(opts *reActOptions) {
	f(opts)
}

// WithTools provides static tools to the agent via options.
func WithTools(tools ...tool.Tool) ReActOption {
	return reActOptionFunc(func(c *reActOptions) {
		c.tools = append(c.tools, tools...)
	})
}

// WithReActOutputSchema sets a structured output schema for the ReAct agent.
func WithReActOutputSchema(outputSchema *schema.OutputSchema) ReActOption {
	return reActOptionFunc(func(c *reActOptions) {
		c.outputSchema = outputSchema
	})
}
