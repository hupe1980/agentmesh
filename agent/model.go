package agent

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/flow"
	"github.com/hupe1980/agentmesh/model"
	"github.com/hupe1980/agentmesh/tool"
)

// Helper functions for pointer types used throughout the agent implementations.

// boolPtr creates a pointer to a boolean value.
// This is useful for optional fields in structs where nil indicates "not set".
func boolPtr(b bool) *bool {
	return &b
}

// stringPtr creates a pointer to a string value.
// This is useful for optional fields in structs where nil indicates "not set".
func stringPtr(s string) *string {
	return &s
}

// ModelAgentOptions configures a ModelAgent instance.
//
// Use functional options with NewModelAgent to override defaults.
type ModelAgentOptions struct {
	Instruction           Instruction
	GlobalInstruction     Instruction
	EnableStreaming       bool
	EnableFunctionCalling bool
	ToolTimeout           time.Duration
	OutputKey             string
	MaxHistoryMessages    int
	AllowTransfer         bool
	Tools                 map[string]tool.Tool
}

// ModelAgent Implementation
//
// ModelAgent provides a complete agent implementation that integrates with
// language models to process natural language inputs and generate responses.

// ModelAgent integrates with language models to provide intelligent text processing capabilities.
//
// This agent implementation supports:
//   - Natural language conversation through system prompts
//   - Function calling with registered tools
//   - Streaming responses for real-time interactions
//   - Session state management with output keys
//   - Template-based prompt customization
//   - Configurable timeouts and retry logic
//
// ModelAgent embeds BaseAgent to inherit standard agent lifecycle and hierarchy management.
type ModelAgent struct {
	BaseAgent                                  // Embedded base agent functionality
	llm                   model.Model          // Language model interface
	instruction           Instruction          // Instructions for the LLM
	tools                 map[string]tool.Tool // Registered tools for function calling
	enableFunctionCalling bool                 // Whether to enable tool usage
	enableStreaming       bool                 // Whether to stream responses
	toolTimeout           time.Duration        // Timeout for individual tool calls
	outputKey             string               // Key for saving responses to session state
	maxHistoryMessages    int                  // Maximum number of conversation history messages to keep
	allowTransfer         bool                 // Whether agent can transfer control to sub-agents
}

// NewModelAgent creates a new model-based agent with sensible defaults.
//
// The agent is initialized with:
//   - Standard agent lifecycle inherited from BaseAgent
//   - Empty tool registry for function calling
//   - Streaming enabled for real-time responses
//   - Function calling enabled for tool usage
//   - 15-second timeout for tool calls
//   - 20-message conversation history limit
//   - Sub-agent transfer capabilities enabled
//
// Parameters:
//   - name: Human-readable name used in system prompt
//   - llm: Language model implementation for text generation
//
// Returns a fully configured ModelAgent ready for conversation.
func NewModelAgent(name string, llm model.Model, optFns ...func(o *ModelAgentOptions)) *ModelAgent {
	opts := ModelAgentOptions{
		Instruction:           NewInstructionFromText(fmt.Sprintf("You are %s, a helpful AI assistant.", name)),
		EnableStreaming:       true,
		EnableFunctionCalling: true,
		ToolTimeout:           15 * time.Second,
		MaxHistoryMessages:    20,
		AllowTransfer:         true,
		Tools:                 make(map[string]tool.Tool),
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &ModelAgent{
		BaseAgent:             NewBaseAgent(name),
		llm:                   llm,
		instruction:           opts.Instruction,
		enableStreaming:       opts.EnableStreaming,
		enableFunctionCalling: opts.EnableFunctionCalling,
		toolTimeout:           opts.ToolTimeout,
		outputKey:             opts.OutputKey,
		maxHistoryMessages:    opts.MaxHistoryMessages,
		allowTransfer:         opts.AllowTransfer,
		tools:                 opts.Tools,
	}
}

// RegisterTool adds a function tool to the agent's capability set.
//
// Registered tools become available for the language model to call during
// conversations when function calling is enabled. Tools should implement
// the tool.Tool interface with proper JSON schema definitions.
//
// Example:
//
//	weatherTool := NewFunctionTool("get_weather", "Get weather for a location", schema, weatherFunc)
//	agent.RegisterTool(weatherTool)
func (a *ModelAgent) RegisterTool(t tool.Tool) {
	a.tools[t.Name()] = t
}

// RegisterTools adds multiple tools to the agent's capability set.
//
// This is a convenience method for registering multiple tools at once.
// If any tool fails to register, the operation continues with remaining tools.
//
// Example:
//
//	mathTools := tool.CreateMathTools()
//	agent.RegisterTools(mathTools...)
func (a *ModelAgent) RegisterTools(tools ...tool.Tool) {
	for _, t := range tools {
		a.RegisterTool(t)
	}
}

// UnregisterTool removes a tool from the agent's capability set.
//
// Returns true if the tool was found and removed, false if it wasn't registered.
func (a *ModelAgent) UnregisterTool(name string) bool {
	if _, exists := a.tools[name]; exists {
		delete(a.tools, name)
		return true
	}
	return false
}

// HasTool checks if a tool is registered with the agent.
func (a *ModelAgent) HasTool(name string) bool {
	_, exists := a.tools[name]
	return exists
}

// ListTools returns the names of all registered tools.
func (a *ModelAgent) ListTools() []string {
	names := make([]string, 0, len(a.tools))
	for name := range a.tools {
		names = append(names, name)
	}
	return names
}

// GetTool retrieves a specific tool by name.
//
// Returns the tool and true if found, nil and false if not registered.
func (a *ModelAgent) GetTool(name string) (tool.Tool, bool) {
	tool, exists := a.tools[name]
	return tool, exists
}

// ClearTools removes all registered tools from the agent.
func (a *ModelAgent) ClearTools() {
	a.tools = make(map[string]tool.Tool)
}

// FlowAgent Interface Implementation
//
// The following methods implement the FlowAgent interface, enabling the ModelAgent
// to work with the flows architecture for modular execution pipeline.

// GetID returns the agent's unique identifier.
// GetID removed (IDs deprecated)

// GetName returns the agent's display name.
func (a *ModelAgent) GetName() string {
	return a.Name()
}

// GetLLM returns the language model instance.
func (a *ModelAgent) GetLLM() model.Model {
	return a.llm
}

// GetTools returns the registered tools for function calling.
func (a *ModelAgent) GetTools() map[string]tool.Tool {
	tools := make(map[string]tool.Tool)
	for name, tool := range a.tools {
		tools[name] = tool
	}

	return tools
}

// GetSubAgents returns the list of child agents as FlowAgents.
func (a *ModelAgent) GetSubAgents() []flow.FlowAgent {
	subAgents := a.SubAgents()
	flowAgents := make([]flow.FlowAgent, 0, len(subAgents))
	for _, subAgent := range subAgents {
		if flowAgent, ok := subAgent.(flow.FlowAgent); ok {
			flowAgents = append(flowAgents, flowAgent)
		}
	}
	return flowAgents
}

// IsFunctionCallingEnabled returns whether function calling is enabled.
func (a *ModelAgent) IsFunctionCallingEnabled() bool {
	return a.enableFunctionCalling
}

// IsStreamingEnabled returns whether streaming responses are enabled.
func (a *ModelAgent) IsStreamingEnabled() bool {
	return a.enableStreaming
}

// IsTransferEnabled returns whether agent transfer is enabled.
func (a *ModelAgent) IsTransferEnabled() bool {
	return a.allowTransfer
}

// GetOutputKey returns the session state key for saving responses.
func (a *ModelAgent) GetOutputKey() string {
	return a.outputKey
}

// MaxHistoryMessages returns the maximum number of conversation history messages to keep.
func (a *ModelAgent) MaxHistoryMessages() int {
	return a.maxHistoryMessages
}

// ResolveInstructions produces the final instruction string (system prompt)
// by resolving static or dynamic instruction sources.
func (a *ModelAgent) ResolveInstructions(runCtx *core.RunContext) (string, error) {
	return a.instruction.Resolve(runCtx)
}

// ExecuteTool executes a named tool with the given arguments.
// ExecuteTool deserializes JSON arguments and invokes the named tool returning
// its result or an error if the tool is unknown or validation fails.
func (a *ModelAgent) ExecuteTool(toolCtx *core.ToolContext, toolName string, args string) (interface{}, error) {
	tool, exists := a.tools[toolName]
	if !exists {
		return nil, fmt.Errorf("tool %s not found", toolName)
	}

	argsMap := make(map[string]interface{})
	if err := json.Unmarshal([]byte(args), &argsMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal args: %w", err)
	}

	return tool.Call(toolCtx, argsMap)
}

// TransferToAgent transfers execution to a named sub-agent.
// TransferToAgent delegates execution to a named descendant agent using the
// same invocation context (shared session state, emit channel).
func (a *ModelAgent) TransferToAgent(runCtx *core.RunContext, agentName string) error {
	// Find the target agent
	targetAgent := a.FindAgent(agentName)
	if targetAgent == nil {
		return fmt.Errorf("agent '%s' not found in hierarchy", agentName)
	}

	// Execute the target agent with the current context
	return targetAgent.Run(runCtx)
}

// Flow-Based Execution Methods
//
// The agent uses the flows architecture for modular execution.

// Run executes the agent using the flows architecture.
//
// This method provides a modular execution pipeline with processors for different
// aspects of agent functionality. It automatically selects the appropriate flow
// based on the agent's capabilities.
// Run implements core.Agent using the flow selector to choose execution
// strategy then streams flow events to the parent invocation context.
func (a *ModelAgent) Run(runCtx *core.RunContext) error {
	runCtx.LogDebug(
		"agent.run.start",
		"agent", a.Name(),
		"run", runCtx.RunID,
	)

	ctx := runCtx.Context // engine manages Start/Stop lifecycle now

	// Select appropriate flow based on agent capabilities
	selector := flow.NewSelector()
	fl := selector.SelectFlow(a)

	runCtx.LogDebug(
		"agent.flow.selected",
		"agent", a.Name(),
		"flow", fmt.Sprintf("%T", fl),
	)

	// Execute the flow
	eventChan, err := fl.Execute(runCtx)
	if err != nil {
		runCtx.LogError(
			"agent.flow.execute.error",
			"agent", a.Name(),
			"error", err.Error(),
		)

		return fmt.Errorf("flow execution failed: %w", err)
	}

	runCtx.LogDebug(
		"agent.flow.execute.start",
		"agent", a.Name(),
	)

	// Process events emitted by the flow
	for event := range eventChan {
		select {
		case runCtx.Emit <- event:
			role := ""
			if event.Content != nil {
				role = event.Content.Role
			}

			runCtx.LogDebug(
				"agent.event.forward",
				"agent", a.Name(),
				"event_id", event.ID,
				"role", role,
				"fn_calls", len(event.GetFunctionCalls()),
			)
		case <-ctx.Done():
			runCtx.LogWarn("agent.run.context_done", "agent", a.Name(), "error", ctx.Err())

			return ctx.Err()
		}
	}

	runCtx.LogDebug("agent.flow.execute.complete", "agent", a.Name())

	return nil
}
