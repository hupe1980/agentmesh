package core

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/logging"
)

// ToolContext provides a constrained, auditable surface for tool / function
// implementations invoked by an agent. It accumulates EventActions (state
// deltas, transfers, escalation signals, artifact diffs) without directly
// mutating the underlying session until applied.
type ToolContext struct {
	runCtx         *RunContext
	functionCallID string
	agentInfo      AgentInfo
	eventActions   EventActions
	valid          bool

	*loggerAdapter
}

// NewToolContext constructs a tool context bound to a parent RunContext
// and unique functionCallID.
func NewToolContext(runCtx *RunContext, functionCallID string) *ToolContext {
	return &ToolContext{
		runCtx:         runCtx,
		functionCallID: functionCallID,
		agentInfo:      runCtx.Agent,
		eventActions:   EventActions{},
		valid:          true,
		loggerAdapter:  newLoggerAdapter(runCtx.Logger()),
	}
}

// Context returns the context associated with the tool invocation.
func (tc *ToolContext) Context() context.Context { return tc.runCtx.Context }

// SessionID returns the session ID associated with the tool invocation.
func (tc *ToolContext) SessionID() string { return tc.runCtx.SessionID }

// RunID returns the run ID associated with the tool invocation.
func (tc *ToolContext) RunID() string { return tc.runCtx.RunID }

// Logger returns the logger associated with the tool invocation.
func (tc *ToolContext) Logger() logging.Logger { return tc.loggerAdapter.Logger() }

// FunctionCallID returns the function call ID associated with the tool invocation.
func (tc *ToolContext) FunctionCallID() string { return tc.functionCallID }

// AgentName returns the agent name associated with the tool invocation.
func (tc *ToolContext) AgentName() string { return tc.agentInfo.Name }

// AgentType returns the agent type associated with the tool invocation.
func (tc *ToolContext) AgentType() string { return tc.agentInfo.Type }

// GetState retrieves the state associated with the given key.
func (tc *ToolContext) GetState(k string) (any, bool) {
	return tc.runCtx.GetState(k)
}

// SetState records a state mutation both on the underlying invocation context
// (for immediate visibility) and in the local EventActions delta for emission.
func (tc *ToolContext) SetState(k string, v any) {
	tc.runCtx.SetState(k, v)
	if tc.eventActions.StateDelta == nil {
		tc.eventActions.StateDelta = map[string]any{}
	}

	tc.eventActions.StateDelta[k] = v
}

// Actions returns the event actions accumulated in the tool context.
func (tc *ToolContext) Actions() *EventActions { return &tc.eventActions }

// SkipSummarization requests that post-processing summarization be bypassed
// for the originating event.
func (tc *ToolContext) SkipSummarization() {
	b := true
	if tc.eventActions.SkipSummarization == nil {
		tc.eventActions.SkipSummarization = &b
	}
}

// TransferToAgent signals orchestration to handoff control to another agent.
func (tc *ToolContext) TransferToAgent(name string) {
	tc.eventActions.TransferToAgent = &name
	tc.LogInfo("tool.transfer.request", "from_agent", tc.AgentName(), "to_agent", name, "function_call_id", tc.functionCallID)
}

// Escalate requests escalation (e.g., to a higher-skill agent or human).
func (tc *ToolContext) Escalate() {
	b := true
	if tc.eventActions.Escalate == nil {
		tc.eventActions.Escalate = &b
	}

	tc.LogInfo("tool.escalate.request", "agent", tc.AgentName(), "function_call_id", tc.functionCallID)
}

// SaveArtifact persists artifact bytes and records the delta size for emission.
func (tc *ToolContext) SaveArtifact(id string, data []byte) error {
	if tc.runCtx.ArtifactStore == nil {
		return fmt.Errorf("artifact service not configured")
	}

	if err := tc.runCtx.ArtifactStore.Save(tc.SessionID(), id, data); err != nil {
		return err
	}

	if tc.eventActions.ArtifactDelta == nil {
		tc.eventActions.ArtifactDelta = map[string]int{}
	}

	tc.eventActions.ArtifactDelta[id] = len(data)

	return nil
}

// LoadArtifact retrieves a persisted artifact by id.
func (tc *ToolContext) LoadArtifact(id string) ([]byte, error) {
	if tc.runCtx.ArtifactStore == nil {
		return nil, fmt.Errorf("artifact service not configured")
	}

	return tc.runCtx.ArtifactStore.Get(tc.SessionID(), id)
}

// ListArtifacts returns artifact IDs stored for the session.
func (tc *ToolContext) ListArtifacts() ([]string, error) {
	if tc.runCtx.ArtifactStore == nil {
		return nil, fmt.Errorf("artifact service not configured")
	}

	return tc.runCtx.ArtifactStore.List(tc.SessionID())
}

// SearchMemory performs a recall query against the configured MemoryStore.
func (tc *ToolContext) SearchMemory(q string, limit int) ([]SearchResult, error) {
	if tc.runCtx.MemoryStore == nil {
		return nil, fmt.Errorf("memory service not configured")
	}

	return tc.runCtx.MemoryStore.Search(tc.SessionID(), q, limit)
}

// StoreMemory appends new content to the session's memory store with metadata.
func (tc *ToolContext) StoreMemory(content string, md map[string]any) error {
	if tc.runCtx.MemoryStore == nil {
		return fmt.Errorf("memory service not configured")
	}

	return tc.runCtx.MemoryStore.Store(tc.SessionID(), content, md)
}

// GetSessionHistory returns conversation history (filtered) for context.
func (tc *ToolContext) GetSessionHistory() []Event {
	if tc.runCtx.Session == nil {
		return nil
	}

	return tc.runCtx.Session.GetConversationHistory()
}

// RefreshSession reloads the underlying session from the SessionStore.
func (tc *ToolContext) RefreshSession() error {
	if tc.runCtx.SessionStore == nil {
		return fmt.Errorf("session service not configured")
	}

	s, err := tc.runCtx.SessionStore.Get(tc.SessionID())
	if err != nil {
		return err
	}

	tc.runCtx.Session = s

	return nil
}

// EmitEvent sends an event directly without merging accumulated actions.
func (tc *ToolContext) EmitEvent(ev Event) error {
	if tc.runCtx.Emit == nil {
		return fmt.Errorf("emit channel not configured")
	}

	select {
	case <-tc.runCtx.Context.Done():
		return tc.runCtx.Context.Err()
	case tc.runCtx.Emit <- ev:
	}

	return nil
}

// Validate performs a structural sanity check of the context.
func (tc *ToolContext) Validate() error {
	if !tc.valid || tc.runCtx == nil || tc.runCtx.SessionID == "" || tc.functionCallID == "" {
		return fmt.Errorf("invalid ToolContext")
	}

	return nil
}

// IsValid reports whether Validate would succeed (fast path).
func (tc *ToolContext) IsValid() bool {
	return tc.valid && tc.runCtx != nil && tc.runCtx.SessionID != "" && tc.functionCallID != ""
}

// InternalRunContext returns the internal run context.
func (tc *ToolContext) InternalRunContext() *RunContext { return tc.runCtx }

// InternalApplyActions merges accumulated EventActions into the provided event.
// (Used internally by engine when finalizing tool invocation events.)
func (tc *ToolContext) InternalApplyActions(ev *Event) {
	if len(tc.eventActions.StateDelta) > 0 {
		if ev.Actions.StateDelta == nil {
			ev.Actions.StateDelta = map[string]any{}
		}
		for k, v := range tc.eventActions.StateDelta {
			ev.Actions.StateDelta[k] = v
		}
	}

	if tc.eventActions.TransferToAgent != nil {
		ev.Actions.TransferToAgent = tc.eventActions.TransferToAgent

		tc.LogInfo("tool.transfer.applied", "from_agent", tc.AgentName(), "to_agent", *tc.eventActions.TransferToAgent, "function_call_id", tc.functionCallID)
	}

	if tc.eventActions.Escalate != nil {
		ev.Actions.Escalate = tc.eventActions.Escalate

		tc.LogInfo("tool.escalate.applied", "agent", tc.AgentName(), "function_call_id", tc.functionCallID)
	}
}
