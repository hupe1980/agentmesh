package agent

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/flow"
	"github.com/hupe1980/agentmesh/logging"
)

// ModelAgentOptions configures a ModelAgent instance.
//
// Use functional options with NewModelAgent to override defaults.
type ModelAgentOptions struct {
	// Instructions for the LLM
	Instructions Instructions
	// Human-readable agent description
	Description string
	// Enable streaming responses
	EnableStreaming bool
	// Enable function calling
	EnableFunctionCalling bool
	// Timeout for tool calls
	ToolTimeout time.Duration
	// Key for saving responses to session state
	OutputKey string
	// Maximum number of conversation history messages to keep
	MaxHistoryMessages int
	// Whether agent can transfer control to sub-agents (peer agents)
	AllowTransferToPeers bool
	// Whether agent can transfer control to its parent agent (escalation)
	AllowTransferToParent bool
	// Registered tools for function calling
	Tools map[string]core.Tool
	// Agent executor for running agent tasks
	AgentExecutor core.AgentExecutor
	// Selector for choosing the appropriate flow
	FlowSelector flow.Selector
	// Sub-agents managed by this agent
	SubAgents []core.Agent
}

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
// It uses flow processors under the hood and logs via the RequestContext's
// logging interface.
type ModelAgent struct {
	*BaseAgent                                 // Embedded base agent functionality
	model                 core.Model           // Language model interface
	instructions          Instructions         // Instructions for the LLM
	tools                 map[string]core.Tool // Registered tools for function calling
	toolsMu               sync.RWMutex         // Protects concurrent access to tools map
	enableFunctionCalling bool                 // Whether to enable tool usage
	enableStreaming       bool                 // Whether to stream responses
	toolTimeout           time.Duration        // Timeout for individual tool calls
	outputKey             string               // Key for saving responses to session state
	maxHistoryMessages    int                  // Maximum number of conversation history messages to keep
	allowTransferToPeers  bool                 // Whether agent can transfer control to sub-agents
	allowTransferToParent bool                 // Whether agent can transfer control to parent agent
	flowSelector          flow.Selector        // Selector for choosing the appropriate flow
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
//   - model: Language model implementation for text generation
//
// Children are wired at construction via options (ModelAgentOptions.SubAgents);
// the hierarchy is read-only at runtime. Returns a fully configured ModelAgent
// ready for conversation.
func NewModelAgent(name string, model core.Model, optFns ...func(o *ModelAgentOptions)) *ModelAgent {
	opts := ModelAgentOptions{
		Instructions:          NewInstructionsFromText(fmt.Sprintf("You are %s, a helpful AI assistant.", name)),
		Description:           "",
		EnableStreaming:       true,
		EnableFunctionCalling: true,
		ToolTimeout:           15 * time.Second,
		MaxHistoryMessages:    20,
		AllowTransferToPeers:  true,
		AllowTransferToParent: true,
		Tools:                 make(map[string]core.Tool),
		AgentExecutor:         DefaultAgentExecutor,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	if opts.FlowSelector == nil {
		opts.FlowSelector = flow.NewDefaultSelector(opts.AgentExecutor)
	}

	a := &ModelAgent{
		model:                 model,
		instructions:          opts.Instructions,
		enableStreaming:       opts.EnableStreaming,
		enableFunctionCalling: opts.EnableFunctionCalling,
		toolTimeout:           opts.ToolTimeout,
		outputKey:             opts.OutputKey,
		maxHistoryMessages:    opts.MaxHistoryMessages,
		allowTransferToPeers:  opts.AllowTransferToPeers,
		allowTransferToParent: opts.AllowTransferToParent,
		tools:                 opts.Tools,
		flowSelector:          opts.FlowSelector,
	}

	a.BaseAgent = NewBaseAgent(a, name, opts.Description)
	a.setSubAgents(opts.SubAgents...)

	return a
}

// RegisterTool adds a function tool to the agent's capability set.
//
// Registered tools become available for the language model to call during
// conversations when function calling is enabled. Tools should implement
// the core.Tool interface with proper JSON schema definitions.
//
// Example:
//
//	weatherTool := NewFuncTool("get_weather", "Get weather for a location", schema, weatherFunc)
//	agent.RegisterTool(weatherTool)
func (a *ModelAgent) RegisterTool(t core.Tool) {
	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()
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
func (a *ModelAgent) RegisterTools(tools ...core.Tool) {
	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()
	for _, t := range tools {
		a.tools[t.Name()] = t
	}
}

// UnregisterTool removes a tool from the agent's capability set.
//
// Returns true if the tool was found and removed, false if it wasn't registered.
func (a *ModelAgent) UnregisterTool(name string) bool {
	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()

	if _, exists := a.tools[name]; exists {
		delete(a.tools, name)
		return true
	}
	return false
}

// HasTool checks if a tool is registered with the agent.
func (a *ModelAgent) HasTool(name string) bool {
	a.toolsMu.RLock()
	defer a.toolsMu.RUnlock()
	_, exists := a.tools[name]
	return exists
}

// GetTool retrieves a specific tool by name.
//
// Returns the tool and true if found, nil and false if not registered.
func (a *ModelAgent) GetTool(name string) (core.Tool, bool) {
	a.toolsMu.RLock()
	defer a.toolsMu.RUnlock()
	t, exists := a.tools[name]
	return t, exists
}

// ClearTools removes all registered tools from the agent.
func (a *ModelAgent) ClearTools() {
	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()
	a.tools = make(map[string]core.Tool)
}

// Model returns the language model instance.
func (a *ModelAgent) Model() core.Model {
	return a.model
}

// Tools returns the registered tools for function calling.
func (a *ModelAgent) Tools() map[string]core.Tool {
	a.toolsMu.RLock()
	defer a.toolsMu.RUnlock()

	tools := make(map[string]core.Tool, len(a.tools))
	maps.Copy(tools, a.tools)

	return tools
}

// IsFunctionCallingEnabled returns whether function calling is enabled.
func (a *ModelAgent) IsFunctionCallingEnabled() bool {
	return a.enableFunctionCalling
}

// IsStreamingEnabled returns whether streaming responses are enabled.
func (a *ModelAgent) IsStreamingEnabled() bool {
	return a.enableStreaming
}

// IsTransferToPeersEnabled returns whether agent transfer to peers is enabled.
func (a *ModelAgent) IsTransferToPeersEnabled() bool {
	return a.allowTransferToPeers
}

// IsTransferToParentEnabled returns whether transfer to the parent agent is enabled.
func (a *ModelAgent) IsTransferToParentEnabled() bool {
	return a.allowTransferToParent
}

// OutputKey returns the session state key for saving responses.
func (a *ModelAgent) OutputKey() string {
	return a.outputKey
}

// MaxHistoryMessages returns the maximum number of conversation history messages to keep.
func (a *ModelAgent) MaxHistoryMessages() int {
	return a.maxHistoryMessages
}

// ResolveInstructions produces the final instruction string (system prompt)
// by resolving static or dynamic instruction sources.
func (a *ModelAgent) ResolveInstructions(ctx context.Context, roCtx core.ReadonlyContext) (string, error) {
	return a.instructions.Resolve(ctx, roCtx)
}

// attachOutputToEvent aggregates final assistant text parts into the event's StateDelta
// under the configured OutputKey, if applicable. No-op if author doesn't match,
// OutputKey is empty, event isn't final, or there are no text parts. Emits debug logs for decisions.
func (a *ModelAgent) attachOutputToEvent(ev *core.Event) {
	// Only mutate events authored by this agent
	if ev.Author != a.Name() {
		return
	}

	ok := a.OutputKey()
	if ok == "" || !ev.IsFinalResponse() || len(ev.Parts) == 0 {
		return
	}

	var b strings.Builder
	for _, p := range ev.Parts {
		if tp, okp := p.(*core.TextPart); okp {
			b.WriteString(tp.Text)
		}
	}

	sd := ev.Actions.StateDelta.Or(nil)
	if sd == nil {
		sd = map[string]any{}
	}

	out := b.String()
	sd[ok] = out
	ev.Actions.StateDelta = core.Map(sd)
}

// Run executes the agent using the flows architecture.
//
// This method provides a modular execution pipeline with processors for different
// aspects of agent functionality. It automatically selects the appropriate flow
// based on the agent's capabilities.
func (a *ModelAgent) Run(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error {
	log := logging.FromContext(ctx).With("agent", a.Name())

	log.Debug("agent.run.start")

	// Select appropriate flow based on agent capabilities
	fl := a.flowSelector.SelectFlow(a)

	log.Debug("agent.flow.selected", "flow", fmt.Sprintf("%T", fl))

	// Execute the flow; the flow writes events directly to queue
	if err := fl.Execute(ctx, reqCtx, eventWriterFunc(func(c context.Context, ev *core.Event) error {
		// Attach final output to state when configured
		a.attachOutputToEvent(ev)

		log.Debug("agent.event.forward", "event_id", ev.ID, "role", ev.Role(), "fn_calls", len(ev.GetFunctionCalls()))

		return queue.Write(c, ev)
	})); err != nil {
		log.Error("agent.flow.execute.error", "error", err)

		return fmt.Errorf("flow execution failed: %w", err)
	}

	log.Debug("agent.flow.execute.complete")

	return nil
}

// Interface compliance (compile-time assertions)
var _ core.Agent = (*ModelAgent)(nil)
var _ flow.Agent = (*ModelAgent)(nil)
var _ parentSetter = (*ModelAgent)(nil)
