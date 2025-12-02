package tool

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// HandoffArgs defines the arguments for agent handoff operations.
type HandoffArgs struct {
	Task    string `json:"task" jsonschema:"required,description=The specific task to delegate to the agent"`
	Context string `json:"context,omitempty" jsonschema:"description=Optional context or background information for the agent"`
}

// HandoffResult represents the result of a handoff operation.
type HandoffResult struct {
	Output   string
	Messages []message.Message
}

// HandoffConfig configures handoff behavior.
type HandoffConfig struct {
	IncludeContext  bool
	RetryAttempts   int
	ValidateResults bool
	SystemPrompt    string // Optional system prompt to prepend to worker agent
}

// HandoffOption configures handoff behavior.
type HandoffOption func(*HandoffConfig)

// WithContext enables passing context to the worker agent.
func WithContext(include bool) HandoffOption {
	return func(c *HandoffConfig) {
		c.IncludeContext = include
	}
}

// WithRetries sets the number of retry attempts on failure.
func WithRetries(attempts int) HandoffOption {
	return func(c *HandoffConfig) {
		c.RetryAttempts = attempts
	}
}

// WithValidation enables/disables result validation.
func WithValidation(validate bool) HandoffOption {
	return func(c *HandoffConfig) {
		c.ValidateResults = validate
	}
}

// WithSystemPrompt sets a system prompt for the worker agent.
func WithSystemPrompt(prompt string) HandoffOption {
	return func(c *HandoffConfig) {
		c.SystemPrompt = prompt
	}
}

// HandoffToAgent creates a tool that delegates work to a worker agent graph.
// This is the core building block for supervisor patterns with tool-based handoffs.
//
// The tool automatically handles:
//   - Message history control (only passes task + optional context)
//   - Retry logic on failures
//   - Result validation
//   - Error handling
//
// Example:
//
//	researchAgent := createResearchAgentGraph(ctx)
//	researchTool, err := tool.HandoffToAgent(
//	    "research_agent",
//	    "Use this to find information, research papers, or gather data on any topic",
//	    researchAgent,
//	    tool.WithContext(true),
//	    tool.WithRetries(2),
//	)
//
// The supervisor agent can then use this tool:
//
//	supervisor, err := agent.NewReActAgent(
//	    llm,
//	    agent.WithTools(researchTool, codeTool),
//	)
func HandoffToAgent(
	agentName string,
	agentDescription string,
	agentGraph *graph.Graph[[]message.Message, message.Message],
	options ...HandoffOption,
) (*FuncTool[HandoffArgs, string], error) {
	if agentGraph == nil {
		return nil, fmt.Errorf("handoff tool %q: agent graph cannot be nil", agentName)
	}

	config := &HandoffConfig{
		IncludeContext:  true,
		RetryAttempts:   1,
		ValidateResults: true,
	}

	for _, opt := range options {
		opt(config)
	}

	toolName := fmt.Sprintf("handoff_to_%s", agentName)
	toolDescription := fmt.Sprintf("Delegate work to the %s. %s", agentName, agentDescription)

	handoffFn := func(ctx context.Context, args HandoffArgs) (string, error) {
		// Validate task is provided
		if args.Task == "" {
			return "", fmt.Errorf("task is required for handoff to %s", agentName)
		}

		// Execute with retry logic
		var lastErr error
		for attempt := 0; attempt < config.RetryAttempts; attempt++ {
			result, err := executeHandoff(ctx, agentGraph, args, config)
			if err == nil {
				if config.ValidateResults && !isValidResult(result) {
					lastErr = fmt.Errorf("agent returned invalid result")
					continue
				}
				return result, nil
			}
			lastErr = err
		}

		return "", fmt.Errorf("handoff to %s failed after %d attempts: %w",
			agentName, config.RetryAttempts, lastErr)
	}

	return NewFuncTool(toolName, toolDescription, handoffFn)
}

// executeHandoff performs the actual agent invocation with context control.
func executeHandoff(
	ctx context.Context,
	agentGraph *graph.Graph[[]message.Message, message.Message],
	args HandoffArgs,
	config *HandoffConfig,
) (string, error) {
	// Build message list for the worker agent
	messages := []message.Message{}

	// Add system prompt if configured
	if config.SystemPrompt != "" {
		messages = append(messages, message.NewSystemMessageFromText(config.SystemPrompt))
	}

	// Add context as system message if provided
	if config.IncludeContext && args.Context != "" {
		messages = append(messages, message.NewSystemMessageFromText(
			fmt.Sprintf("Context: %s", args.Context),
		))
	}

	// Add the actual task
	messages = append(messages, message.NewHumanMessageFromText(args.Task))

	// Execute the worker agent graph (assumes graph is already built)
	lastMsg, err := graph.Last(agentGraph.Run(ctx, messages))
	if err != nil {
		return "", fmt.Errorf("agent execution failed: %w", err)
	}

	if lastMsg == nil {
		return "", fmt.Errorf("agent produced no messages")
	}

	// Extract text from the last message
	return extractTextFromMessage(lastMsg), nil
}

// extractTextFromMessage extracts text content from a message.
func extractTextFromMessage(msg message.Message) string {
	if msg == nil {
		return ""
	}

	var texts []string
	for _, part := range msg.Parts() {
		if textPart, ok := part.(message.TextPart); ok {
			texts = append(texts, textPart.Text)
		}
	}

	if len(texts) == 0 {
		return ""
	}

	// Join multiple text parts with space
	result := texts[0]
	for i := 1; i < len(texts); i++ {
		result += " " + texts[i]
	}
	return result
}

// isValidResult checks if the agent returned a meaningful result.
func isValidResult(result string) bool {
	return result != "" && result != "error" && result != "failed"
}
