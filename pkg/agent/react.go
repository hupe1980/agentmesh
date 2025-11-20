package agent

import (
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// NewReActAgent creates a Reasoning and Acting (ReAct) agent that iteratively:
//  1. Reasons about the task
//  2. Decides which tool to use
//  3. Observes the result
//  4. Repeats until the answer is found
//
// Returns a MessageRunnable that processes message sequences
// and streams execution results. This interface enables type-safe composition,
// easy mocking in tests, and swappable implementations.
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
func NewReActAgent(mdl model.Model, opts ...ReActOption) (MessageRunnable, error) {
	if mdl == nil {
		return nil, fmt.Errorf("model must not be nil")
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

	// Create state - StateBuilder no longer exists
	mgr := state.NewManager()
	if err := RegisterMessagesKey(mgr); err != nil {
		return nil, fmt.Errorf("react agent: failed to register messages key: %w", err)
	}

	g, err := graph.NewGraph(mgr)
	if err != nil {
		return nil, fmt.Errorf("react agent: failed to create graph: %w", err)
	}

	// Model node: generate response with tools and system prompt
	// System prompt is sent per-request (Pydantic AI style) for token efficiency
	modelNode, err := NewModelNode(mdl,
		WithModelTools(acceptedTools...),
		WithModelSystemPrompt(config.systemPrompt),
	)
	if err != nil {
		return nil, fmt.Errorf("react agent: failed to create model node: %w", err)
	}
	_ = g.AddNode(modelNode)

	// Tool node: execute tool calls
	toolNode, err := NewToolNode(toolRegistry, WithToolErrorPrefix("react agent"))
	if err != nil {
		return nil, fmt.Errorf("react agent: failed to create tool node: %w", err)
	}
	_ = g.AddNode(toolNode)

	// Build graph topology
	g.AddEdge(graph.StartNode, "model")

	g.AddConditionalEdges("model", RouteOnToolCalls("tool", graph.EndNode), []string{"tool", graph.EndNode})

	g.AddEdge("tool", "model")

	// Compile the graph
	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		return nil, fmt.Errorf("react agent: failed to compile graph: %w", err)
	}

	return compiled, nil
}

// reActOptions holds configuration for ReAct agents.
type reActOptions struct {
	maxIterations int
	tools         []tool.Tool // Optional static tools via WithTools option
	systemPrompt  string      // Optional system prompt prepended to all invocations
}

func defaultReActOptions() reActOptions {
	return reActOptions{
		maxIterations: 10,
		tools:         nil,
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
//	result, err := graph.Last(agent.Run(ctx, graph.NewInput(
//	    message.NewSystemMessageFromText("You are a helpful math tutor."),
//	    message.NewHumanMessageFromText("What is 2+2?"),
//	)))
func WithSystemPrompt(prompt string) ReActOption {
	return func(c *reActOptions) {
		c.systemPrompt = prompt
	}
}
