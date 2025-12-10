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
//  2. Decides which tool to use (if tools provided)
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
//	agent, err := agent.NewReAct(model,
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

	// Handle structured output fallback if needed
	outputSchema, tools, err := prepareStructuredOutputFallback(mdl, config.outputSchema, config.tools)
	if err != nil {
		return nil, err
	}

	// Combine static tools with any toolsets into a single toolset
	combinedToolset := buildCombinedToolset(tools, config.toolsets)

	// Create model executor
	modelExecutor := model.NewExecutor(mdl, model.WithExecutorName("react-model"))
	if len(config.modelMiddleware) > 0 {
		modelExecutor = model.Chain(modelExecutor, config.modelMiddleware...)
	}

	// Build model node options
	modelNodeOpts := []ModelNodeOption{
		WithModelToolset(combinedToolset),
	}

	// Add output schema if configured (nil when using tool fallback)
	if outputSchema != nil {
		modelNodeOpts = append(modelNodeOpts, WithModelOutputSchema(outputSchema))
	}

	// Add instructions if configured
	if config.instructions != nil {
		modelNodeOpts = append(modelNodeOpts, func(c *ModelNodeConfig) {
			c.Instructions = config.instructions
		})
	}

	// Create model node function with dynamic tool discovery
	modelFn, err := NewModelNodeFunc(modelExecutor, modelNodeOpts...)
	if err != nil {
		return nil, fmt.Errorf("agent/react: create model node: %w", err)
	}

	// Create tool node function with dynamic tool resolution
	toolFn, err := NewToolNodeFunc(
		WithToolNodeToolset(combinedToolset),
		WithToolNodeMiddleware(config.toolMiddleware...),
	)
	if err != nil {
		return nil, fmt.Errorf("agent/react: create tool node: %w", err)
	}

	// Build and configure graph
	return buildReActGraph(modelFn, toolFn, config)
}

// buildCombinedToolset creates a single toolset from static tools and dynamic toolsets.
func buildCombinedToolset(staticTools []tool.Tool, toolsets []tool.Toolset) tool.Toolset {
	var allToolsets []tool.Toolset

	// Add static tools as a StaticToolset if provided
	if len(staticTools) > 0 {
		allToolsets = append(allToolsets, tool.NewStaticToolset(staticTools...))
	}

	// Add dynamic toolsets
	allToolsets = append(allToolsets, toolsets...)

	// If only one toolset, return it directly
	if len(allToolsets) == 1 {
		return allToolsets[0]
	}

	// Combine all toolsets
	if len(allToolsets) > 0 {
		return tool.Combine(allToolsets...)
	}

	// Return empty static toolset if no tools/toolsets provided
	return tool.NewStaticToolset()
}

// prepareStructuredOutputFallback checks if structured output needs to use tool-based fallback.
// If the model doesn't support native structured output but supports tools, it injects
// SetModelResponseTool to get structured output via tool calling.
//
// Returns:
//   - outputSchema: nil if using tool fallback, original schema if model supports it natively
//   - tools: original tools, possibly with SetModelResponseTool appended
//   - error: if creating the fallback tool fails
func prepareStructuredOutputFallback(
	mdl model.Model,
	outputSchema *schema.OutputSchema,
	tools []tool.Tool,
) (*schema.OutputSchema, []tool.Tool, error) {
	if outputSchema == nil {
		return nil, tools, nil
	}

	caps := mdl.Capabilities()
	if caps.StructuredOutput || !caps.Tools {
		// Model supports native structured output, or doesn't support tools (can't use fallback)
		return outputSchema, tools, nil
	}

	// Check if SetModelResponseTool is already provided by the caller
	for _, t := range tools {
		if t.Name() == "set_model_response" {
			// Already has the tool, just clear the schema
			return nil, tools, nil
		}
	}

	// Create SetModelResponseTool with the output schema
	setResponseTool, err := tool.NewSetModelResponseTool(outputSchema)
	if err != nil {
		return nil, nil, fmt.Errorf("create set_model_response tool: %w", err)
	}

	// Add to tools and clear outputSchema (will be handled via tool)
	return nil, append(tools, setResponseTool), nil
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
	tools    []tool.Tool
	toolsets []tool.Toolset
}

func defaultReActOptions() reActOptions {
	return reActOptions{
		commonOptions: commonOptions{
			instructions:    nil,
			maxIterations:   10,
			outputSchema:    nil,
			graphMiddleware: nil,
			modelMiddleware: nil,
			toolMiddleware:  nil,
		},
		tools:    nil,
		toolsets: nil,
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

// WithToolset adds a dynamic toolset for runtime tool discovery.
// Tools are discovered from the toolset on each model invocation,
// with access to the current graph state via the View parameter.
// Multiple toolsets can be added; they will be combined.
func WithToolset(ts tool.Toolset) ReActOption {
	return reActOptionFunc(func(c *reActOptions) {
		c.toolsets = append(c.toolsets, ts)
	})
}
