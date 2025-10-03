package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hupe1980/agentmesh/core"
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
	// Output schema for the agent's responses
	OutputSchema core.Opt[core.OutputSchema]
	// Key for saving responses to session state
	OutputKey string
	// Maximum number of conversation history messages to keep
	MaxHistoryMessages int
	// What kind of history the agent receives
	HistoryMode core.HistoryMode
	// Whether agent can transfer control to sub-agents (peer agents)
	AllowTransferToPeers bool
	// Whether agent can transfer control to its parent agent (escalation)
	AllowTransferToParent bool
	// Registered tools for function calling
	Tools []core.Tool
	// Registered toolsets for function calling
	Toolsets []core.Toolset
	// Sub-agents managed by this agent
	SubAgents []core.Agent
}

// DefaultModelAgentOptions returns a fresh copy of the sensible defaults used by
// NewModelAgent. The options are parameterized by name so the default
// instructions can reference the agent.
func DefaultModelAgentOptions(name string) ModelAgentOptions {
	return ModelAgentOptions{
		Instructions:          NewInstructionsFromText(fmt.Sprintf("You are %s, a helpful AI assistant.", name)),
		Description:           "",
		EnableStreaming:       true,
		EnableFunctionCalling: true,
		ToolTimeout:           15 * time.Second,
		OutputSchema:          core.None[core.OutputSchema](),
		OutputKey:             "",
		MaxHistoryMessages:    20,
		HistoryMode:           core.HistoryAll,
		AllowTransferToPeers:  true,
		AllowTransferToParent: true,
		Tools:                 []core.Tool{},
		Toolsets:              []core.Toolset{},
		SubAgents:             []core.Agent{},
	}
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
	*BaseAgent                           // Embedded base agent functionality
	model                 core.Model     // Language model interface
	instructions          Instructions   // Instructions for the LLM
	tools                 []core.Tool    // Registered tools for function calling
	toolsets              []core.Toolset // Registered toolsets for function calling
	enableFunctionCalling bool           // Whether to enable tool usage
	enableStreaming       bool           // Whether to stream responses
	toolTimeout           time.Duration  // Timeout for individual tool calls

	outputSchema core.Opt[core.OutputSchema] // Expected output schema for responses
	outputKey    string                      // Key for saving responses to session state

	maxHistoryMessages int // Maximum number of conversation history
	// messages to keep
	historyMode           core.HistoryMode  // What kind of history the agent receives
	allowTransferToPeers  bool              // Whether agent can transfer control to sub-agents
	allowTransferToParent bool              // Whether agent can transfer control to parent agent
	flowSelector          core.FlowSelector // Selector for choosing the appropriate flow
}

// NewModelAgent creates a new model-based agent with sensible defaults.
func NewModelAgent(
	name string,
	m core.Model,
	flowSelector core.FlowSelector,
	optFns ...func(o *ModelAgentOptions),
) (*ModelAgent, error) {
	if flowSelector == nil {
		return nil, fmt.Errorf("flow selector is required")
	}

	opts := DefaultModelAgentOptions(name)

	for _, fn := range optFns {
		fn(&opts)
	}

	a := &ModelAgent{
		model:                 m,
		instructions:          opts.Instructions,
		enableStreaming:       opts.EnableStreaming,
		enableFunctionCalling: opts.EnableFunctionCalling,
		toolTimeout:           opts.ToolTimeout,
		outputSchema:          opts.OutputSchema,
		outputKey:             opts.OutputKey,
		maxHistoryMessages:    opts.MaxHistoryMessages,
		historyMode:           opts.HistoryMode,
		allowTransferToPeers:  opts.AllowTransferToPeers,
		allowTransferToParent: opts.AllowTransferToParent,
		tools:                 opts.Tools,
		toolsets:              opts.Toolsets,
		flowSelector:          flowSelector,
	}

	a.BaseAgent = NewBaseAgent(a, name, opts.Description)
	if len(opts.SubAgents) > 0 {
		if err := a.AddSubAgents(opts.SubAgents...); err != nil {
			return nil, fmt.Errorf("failed to add sub-agents: %w", err)
		}
	}

	return a, nil
}

// Tools returns the registered tools for function calling.
func (a *ModelAgent) Tools() []core.Tool {
	return a.tools
}

// Toolsets returns the registered toolsets for function calling.
func (a *ModelAgent) Toolsets() []core.Toolset {
	return a.toolsets
}

// ResolveTools aggregates tools from tools and toolsets.
func (a *ModelAgent) ResolveTools(ctx context.Context, roCtx core.ReadonlyContext) ([]core.Tool, error) {
	// Start with locally registered tools
	allTools := append([]core.Tool(nil), a.tools...)

	for _, ts := range a.toolsets {
		tools, err := ts.ListTools(ctx, roCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to list tools from toolset: %w", err)
		}

		allTools = append(allTools, tools...)
	}

	return allTools, nil
}

// Model returns the language model instance.
func (a *ModelAgent) Model() core.Model {
	return a.model
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

// OutputSchema returns the expected output schema for responses.
func (a *ModelAgent) OutputSchema() core.Opt[core.OutputSchema] {
	return a.outputSchema
}

// OutputKey returns the session state key for saving responses.
func (a *ModelAgent) OutputKey() string {
	return a.outputKey
}

// MaxHistoryMessages returns the maximum number of conversation history messages to keep.
func (a *ModelAgent) MaxHistoryMessages() int {
	return a.maxHistoryMessages
}

// HistoryMode returns what kind of history the agent receives.
func (a *ModelAgent) HistoryMode() core.HistoryMode {
	return a.historyMode
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

	sd := ev.Actions.StateDelta.Or(map[string]any{})

	sd[ok] = b.String()

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
	fl := a.flowSelector.Select(a)

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
var _ core.FlowAgent = (*ModelAgent)(nil)
