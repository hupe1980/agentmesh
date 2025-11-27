package agent

import (
	"fmt"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
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
//
//nolint:gocyclo // acceptable complexity for agent initialization with many configuration options
func NewReActAgent(mdl model.Model, opts ...ReActOption) (MessageRunnable, error) {
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
			return nil, fmt.Errorf("react agent: model does not support tools (%d tools provided but Capabilities().Tools is false)", len(tools))
		}
	}

	// Create state manager
	mgr := state.NewManager()
	if err := RegisterMessagesKey(mgr); err != nil {
		return nil, fmt.Errorf("react agent: failed to register messages key: %w", err)
	}

	// Create model executor - encapsulates model lifecycle management
	// Apply model middleware if provided
	modelExecutor := model.NewExecutor(mdl, model.WithExecutorName("react-model"))
	if len(config.modelMiddleware) > 0 {
		modelExecutor = model.Chain(modelExecutor, config.modelMiddleware...)
	}

	// Model node: orchestration layer that builds requests and delegates to executor
	// System prompt, tools, and schema are stored in the node and used per-request
	modelNodeOpts := []ModelNodeOption{
		WithModelNodeName("model"),
		WithModelSystemPrompt(config.systemPrompt),
		WithModelTools(tools...),
		WithModelTargets([]string{"tool", graph.EndNode}),
	}
	if config.outputSchema != nil {
		modelNodeOpts = append(modelNodeOpts, WithOutputSchema(config.outputSchema))
	}
	modelNode, err := NewModelNode(modelExecutor, modelNodeOpts...)
	if err != nil {
		return nil, fmt.Errorf("react agent: failed to create model node: %w", err)
	}

	// Create tool executor - use sequential by default for deterministic behavior
	// Apply tool middleware if provided
	toolExecutor := tool.NewSequentialExecutor(toolRegistry,
		tool.WithErrorPrefix("react agent"),
		tool.WithContinueOnError(false))
	if len(config.toolMiddleware) > 0 {
		toolExecutor = tool.Chain(toolExecutor, config.toolMiddleware...)
	}

	// Tool node: orchestration layer that extracts calls and delegates to executor
	toolNode, err := NewToolNode(toolExecutor,
		WithToolNodeName("tool"),
		WithToolTargets([]string{"model"}))
	if err != nil {
		return nil, fmt.Errorf("react agent: failed to create tool node: %w", err)
	}

	// Build graph using fluent builder API
	// Apply graph middleware if provided
	var graphExecutor graph.Executor[[]message.Message, message.Message] = graph.NewMessagePregelExecutor()
	if len(config.graphMiddleware) > 0 {
		graphExecutor = graph.Chain(graphExecutor, config.graphMiddleware...)
	}

	builder, err := graph.NewBuilder(graphExecutor, graph.WithManager[[]message.Message, message.Message](mgr))
	if err != nil {
		return nil, fmt.Errorf("react agent: failed to create builder: %w", err)
	}

	compiled, err := builder.
		AddNode(modelNode).
		AddNode(toolNode).
		SetEntryPoint("model").
		Compile()
	if err != nil {
		return nil, fmt.Errorf("react agent: failed to build graph: %w", err)
	}

	return compiled, nil
}

// reActOptions holds configuration for ReAct agents.
type reActOptions struct {
	maxIterations   int
	tools           []tool.Tool                                            // Optional static tools via WithTools option
	systemPrompt    string                                                 // Optional system prompt prepended to all invocations
	outputSchema    *schema.OutputSchema                                   // Optional structured output schema
	graphMiddleware []graph.Middleware[[]message.Message, message.Message] // Optional graph middleware
	modelMiddleware []model.Middleware                                     // Optional model middleware
	toolMiddleware  []tool.Middleware                                      // Optional tool middleware
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

// WithReActOutputSchema sets a structured output schema with metadata for the ReAct agent.
// The schema constrains the model to generate valid JSON matching the schema.
// Only works with models that support structured output (check model.Capabilities().StructuredOutput).
//
// This option provides better type safety and includes metadata like name, description, and strict mode.
// Model implementations can use the Strict flag, Description, and other metadata for provider-specific behavior.
//
// Example:
//
//	type AgentResponse struct {
//	    Reasoning string `json:"reasoning" jsonschema:"required,description=Step-by-step reasoning"`
//	    Action    string `json:"action" jsonschema:"required,description=The action to take"`
//	    Answer    string `json:"answer" jsonschema:"description=Final answer if available"`
//	}
//	outputSchema, _ := schema.NewOutputSchema("agent_response", AgentResponse{},
//	    schema.WithStrict(true),
//	    schema.WithDescription("ReAct agent structured response"))
//	agent, err := agent.NewReActAgent(model,
//	    agent.WithTools(tools...),
//	    agent.WithReActOutputSchema(&outputSchema),
//	)
func WithReActOutputSchema(outputSchema *schema.OutputSchema) ReActOption {
	return func(c *reActOptions) {
		c.outputSchema = outputSchema
	}
}

// WithGraphMiddleware adds middleware to the graph executor.
// Middleware is applied in the order provided: Chain(executor, m1, m2, m3).
//
// Example:
//
//	import graphmw "github.com/hupe1980/agentmesh/pkg/graph/middleware"
//
//	agent, err := agent.NewReActAgent(model,
//	    agent.WithTools(tools...),
//	    agent.WithGraphMiddleware(
//	        graphmw.NewLoggingMiddleware[[]message.Message, message.Message](logger),
//	        graphmw.NewEventMiddleware[[]message.Message, message.Message](),
//	    ),
//	)
func WithGraphMiddleware(middleware ...graph.Middleware[[]message.Message, message.Message]) ReActOption {
	return func(c *reActOptions) {
		c.graphMiddleware = append(c.graphMiddleware, middleware...)
	}
}

// WithModelMiddleware adds middleware to the model executor.
// Middleware is applied in the order provided: Chain(executor, m1, m2, m3).
//
// Example:
//
//	import modelmw "github.com/hupe1980/agentmesh/pkg/model/middleware"
//
//	agent, err := agent.NewReActAgent(model,
//	    agent.WithTools(tools...),
//	    agent.WithModelMiddleware(
//	        modelmw.NewCacheMiddleware(),
//	        modelmw.NewRetryMiddleware(3, time.Second),
//	    ),
//	)
func WithModelMiddleware(middleware ...model.Middleware) ReActOption {
	return func(c *reActOptions) {
		c.modelMiddleware = append(c.modelMiddleware, middleware...)
	}
}

// WithToolMiddleware adds middleware to the tool executor.
// Middleware is applied in the order provided: Chain(executor, m1, m2, m3).
//
// Example:
//
//	import toolmw "github.com/hupe1980/agentmesh/pkg/tool/middleware"
//
//	agent, err := agent.NewReActAgent(model,
//	    agent.WithTools(tools...),
//	    agent.WithToolMiddleware(
//	        toolmw.NewTimeoutMiddleware(30*time.Second),
//	        toolmw.NewAuditMiddleware(logger),
//	    ),
//	)
func WithToolMiddleware(middleware ...tool.Middleware) ReActOption {
	return func(c *reActOptions) {
		c.toolMiddleware = append(c.toolMiddleware, middleware...)
	}
}
