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
func NewReActAgent(mdl model.Model, opts ...ReActOption) (*graph.Compiled, error) {
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
		emptyState := graph.NewState(0)

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

	// Check if model supports tools (via Capabilities)
	if len(acceptedTools) > 0 {
		caps := mdl.Capabilities()
		if !caps.Tools {
			return nil, fmt.Errorf("react agent: model does not support tools (%d tools provided but Capabilities().Tools is false)", len(acceptedTools))
		}
	}

	// Create state using StateBuilder for cleaner initialization
	stateBuilder := graph.NewStateBuilder().
		WithUnlimitedMessages()

	state := stateBuilder.Build()

	g := graph.NewGraph(state)

	// Model node: generate response with tools and system prompt
	// System prompt is sent per-request (Pydantic AI style) for token efficiency
	modelNode := &graph.Node{
		Name: "model",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			messages := s.MessagesSnapshot()

			// Create request with tools and system prompt
			req := &model.Request{
				Messages:     messages,
				Tools:        acceptedTools,
				SystemPrompt: config.systemPrompt, // Sent per-request, not stored in state
			}

			// Call the model
			resp, err := model.Last(mdl.Generate(ctx, req))
			if err != nil {
				return nil, err
			}

			return &graph.NodeResult{
				Messages: []message.Message{resp.Message},
				Updates:  map[string]any{},
			}, nil
		},
	}
	_ = g.AddNode(modelNode)

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
func MustNewReActAgent(mdl model.Model, opts ...ReActOption) *graph.Compiled {
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

// WithSystemPrompt sets a system prompt sent with every model invocation.
// The system prompt provides instructions and context to guide the agent's behavior.
//
// IMPORTANT: The system prompt is sent per-request (not stored in conversation state).
// This makes it more token-efficient for multi-turn conversations, as the prompt
// is sent with each model call but doesn't accumulate in the message history.
//
// If you prefer the system prompt to be part of the conversation history (LangChain style),
// add a system message when invoking the agent instead:
//
//	// Option 1: Per-request system prompt (Pydantic AI style - recommended)
//	agent, err := agent.NewReActAgent(
//	    model,
//	    agent.WithSystemPrompt("You are a helpful math tutor."),
//	)
//
//	// Option 2: System message in history (LangChain style)
//	agent, err := agent.NewReActAgent(model)
//	result, err := agent.Invoke(ctx, graph.NewInput(
//	    message.NewSystemMessageFromText("You are a helpful math tutor."),
//	    message.NewHumanMessageFromText("What is 2+2?"),
//	))
func WithSystemPrompt(prompt string) ReActOption {
	return func(c *reActOptions) {
		c.systemPrompt = prompt
	}
}
