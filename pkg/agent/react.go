package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// NewReActAgent creates a Reasoning and Acting (ReAct) agent that iteratively:
//  1. Reasons about the task
//  2. Decides which tool to use
//  3. Observes the result
//  4. Repeats until the answer is found
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
func NewReActAgent(mdl model.Model, opts ...ReActOption) (*graph.CompiledGraph, error) {
	if mdl == nil {
		return nil, fmt.Errorf("model must not be nil")
	}

	config := defaultReActOptions()
	for _, opt := range opts {
		opt(&config)
	}

	var allTools []tool.Tool

	// Combine static tools with toolset if provided
	if config.toolset != nil {
		// Fetch tools from toolset with nil state (toolset can use defaults)
		// Note: toolset.ListTools requires a context and StateReader
		// We'll use an empty state snapshot for initialization
		ctx := context.Background()
		emptyState := graph.NewGraphState(0)

		toolsetTools, err := config.toolset.ListTools(ctx, emptyState)
		if err != nil {
			return nil, fmt.Errorf("react agent: failed to list tools from toolset: %w", err)
		}

		allTools = append(allTools, toolsetTools...)
	}

	// Add tools from WithTools option
	allTools = append(allTools, config.tools...)

	// Build tool registry
	toolRegistry := make(map[string]tool.Tool, len(allTools))
	for _, t := range allTools {
		if t == nil {
			continue
		}
		toolRegistry[t.Name()] = t
	}

	acceptedTools := make([]tool.Tool, 0, len(toolRegistry))
	for _, t := range toolRegistry {
		acceptedTools = append(acceptedTools, t)
	}

	// Bind tools to model if supported
	if toolAware, ok := mdl.(model.ToolAware); ok {
		configured := toolAware.BindTools(acceptedTools...)
		if configured == nil {
			return nil, fmt.Errorf("react agent: model returned nil from BindTools (expected configured model)")
		}
		mdl = configured
	} else if len(acceptedTools) > 0 {
		return nil, fmt.Errorf("react agent: model does not support tool configuration (%d tools provided but model doesn't implement ToolAware)", len(acceptedTools))
	}

	// Create state using StateBuilder for cleaner initialization
	stateBuilder := graph.NewStateBuilder().
		WithUnlimitedMessages()

	// If system prompt configured, add it as initial message
	if config.systemPrompt != "" {
		systemMsg := message.NewSystemMessageFromText(config.systemPrompt)
		stateBuilder.WithInitialMessages(systemMsg)
	}

	state := stateBuilder.Build()

	g := graph.NewGraph(state)

	// Model node: generate response
	_ = g.AddNode(ModelNode(mdl))

	// Tool node: execute tool calls
	_ = g.AddNode(ToolNode(toolRegistry, WithToolErrorPrefix("react agent")))

	// Build graph topology
	g.AddEdge(graph.StartNode, "model")

	g.AddConditionalEdges("model", RouteOnToolCalls("tool", graph.EndNode), []string{"tool", graph.EndNode})

	g.AddEdge("tool", "model")

	return g.Compile()
}

// MustNewReActAgent is like NewReActAgent but panics on error.
// Use this in tests or when you're certain inputs are valid.
func MustNewReActAgent(mdl model.Model, opts ...ReActOption) *graph.CompiledGraph {
	agent, err := NewReActAgent(mdl, opts...)
	if err != nil {
		panic(fmt.Errorf("failed to create ReAct agent: %w", err))
	}
	return agent
}

// reActOptions holds configuration for ReAct agents.
type reActOptions struct {
	maxIterations int
	tools         []tool.Tool  // Optional static tools via WithTools option
	toolset       tool.Toolset // Optional dynamic toolset for runtime tool discovery
	systemPrompt  string       // Optional system prompt prepended to all invocations
}

func defaultReActOptions() reActOptions {
	return reActOptions{
		maxIterations: 10,
		tools:         nil,
		toolset:       nil,
		systemPrompt:  "",
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
// This is an alternative to passing tools as a parameter to NewReActAgent.
// Tools provided via this option are combined with tools from the parameter
// and any toolset provided via WithToolset.
func WithTools(tools ...tool.Tool) ReActOption {
	return func(c *reActOptions) {
		c.tools = append(c.tools, tools...)
	}
}

// WithToolset provides a dynamic toolset that discovers tools at runtime.
// When provided, the toolset's ListTools method will be called during graph
// execution to get the available tools based on the current state.
// This is useful for MCP (Model Context Protocol) toolsets or any scenario
// where tools need to be discovered dynamically.
func WithToolset(ts tool.Toolset) ReActOption {
	return func(c *reActOptions) {
		c.toolset = ts
	}
}

// WithSystemPrompt sets a system prompt that will be prepended to all agent invocations.
// The system prompt provides instructions and context to guide the agent's behavior.
// If a system message is also provided in the Invoke() call, the configured system
// prompt will appear first, followed by any additional system messages.
//
// Example:
//
//	agent, err := agent.NewReActAgent(
//	    model,
//	    agent.WithSystemPrompt("You are a helpful math tutor. Always show your work."),
//	    agent.WithMaxIterations(5),
//	)
func WithSystemPrompt(prompt string) ReActOption {
	return func(c *reActOptions) {
		c.systemPrompt = prompt
	}
}
