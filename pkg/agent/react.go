package agent

import (
	"fmt"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// NewReActAgent creates a Reasoning and Acting (ReAct) agent that iteratively:
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
//	agent, err := agent.NewReActAgent(model,
//	    agent.WithTools(searchTool, calculatorTool),
//	    agent.WithMaxIterations(5))
//
// Example with dynamic toolset:
//
//	mcpToolset := mcp.NewToolset(mcp.NewStdioSessionFactory("mcp-server", []string{}))
//	agent, err := agent.NewReActAgent(model,
//	    agent.WithToolset(mcpToolset),
//	    agent.WithMaxIterations(5))
func NewReActAgent(mdl model.Model, opts ...ReActOption) (*graph.CompiledMessageGraph, error) {
	if err := validate.NotNil(mdl, "model"); err != nil {
		return nil, err
	}

	config := defaultReActOptions()
	for _, opt := range opts {
		opt(&config)
	}

	// Build tool registry
	toolRegistry := make(map[string]tool.Tool, len(config.tools))
	for _, t := range config.tools {
		if t == nil {
			continue
		}
		toolRegistry[t.Name()] = t
	}

	tools := make([]tool.Tool, 0, len(toolRegistry))
	for _, t := range toolRegistry {
		tools = append(tools, t)
	}

	// Check if model supports tools (via Capabilities)
	if len(tools) > 0 {
		caps := mdl.Capabilities()
		if !caps.Tools {
			return nil, fmt.Errorf("agent/react: model does not support tools (%d tools provided but Capabilities().Tools is false)", len(tools))
		}
	}

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
	)
	if err != nil {
		return nil, fmt.Errorf("agent/react: create model node: %w", err)
	}

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

	// Build graph - MessagesKey is automatically included by NewMessageGraph
	g := graph.NewMessageGraph()
	g.Node("model", modelFn, "tool", graph.END)
	g.Node("tool", toolFn, "model")
	g.Start("model")

	// Apply graph middleware if provided
	if len(config.graphMiddleware) > 0 {
		g.WithMiddleware(config.graphMiddleware...)
	}

	return g.Build()
}

// reActOptions holds configuration for ReAct agents.
type reActOptions struct {
	maxIterations   int
	tools           []tool.Tool
	systemPrompt    string
	outputSchema    *schema.OutputSchema
	graphMiddleware []graph.Middleware
	modelMiddleware []model.Middleware
	toolMiddleware  []tool.Middleware
}

func defaultReActOptions() reActOptions {
	return reActOptions{
		maxIterations:   10,
		tools:           nil,
		systemPrompt:    "",
		outputSchema:    nil,
		graphMiddleware: nil,
		modelMiddleware: nil,
		toolMiddleware:  nil,
	}
}

// ReActOption configures a ReAct agent.
type ReActOption func(*reActOptions)

// WithMaxIterations sets the maximum reasoning iterations for ReAct.
func WithMaxIterations(n int) ReActOption {
	return func(c *reActOptions) {
		if n > 0 {
			c.maxIterations = n
		}
	}
}

// WithTools provides static tools to the agent via options.
func WithTools(tools ...tool.Tool) ReActOption {
	return func(c *reActOptions) {
		c.tools = append(c.tools, tools...)
	}
}

// WithSystemPrompt sets a system prompt sent with every model invocation.
// The system prompt provides instructions and context to guide the agent's behavior.
func WithSystemPrompt(prompt string) ReActOption {
	return func(c *reActOptions) {
		c.systemPrompt = prompt
	}
}

// WithReActOutputSchema sets a structured output schema for the ReAct agent.
func WithReActOutputSchema(outputSchema *schema.OutputSchema) ReActOption {
	return func(c *reActOptions) {
		c.outputSchema = outputSchema
	}
}

// WithGraphMiddleware adds middleware to the graph.
func WithGraphMiddleware(middleware ...graph.Middleware) ReActOption {
	return func(c *reActOptions) {
		c.graphMiddleware = append(c.graphMiddleware, middleware...)
	}
}

// WithModelMiddleware adds middleware to the model executor.
func WithModelMiddleware(middleware ...model.Middleware) ReActOption {
	return func(c *reActOptions) {
		c.modelMiddleware = append(c.modelMiddleware, middleware...)
	}
}

// WithToolMiddleware adds middleware to the tool executor.
func WithToolMiddleware(middleware ...tool.Middleware) ReActOption {
	return func(c *reActOptions) {
		c.toolMiddleware = append(c.toolMiddleware, middleware...)
	}
}
