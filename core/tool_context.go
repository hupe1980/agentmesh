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
	invocationCtx  *InvocationContext
	functionCallID string
	agentInfo      AgentInfo
	eventActions   EventActions
	valid          bool
}

// NewToolContext constructs a tool context bound to a parent InvocationContext
// and unique functionCallID.
func NewToolContext(invocationCtx *InvocationContext, functionCallID string) *ToolContext {
	return &ToolContext{invocationCtx: invocationCtx, functionCallID: functionCallID, agentInfo: invocationCtx.Agent, valid: true}
}

// Context returns the context associated with the tool invocation.
func (tc *ToolContext) Context() context.Context { return tc.invocationCtx.Context }

// SessionID returns the session ID associated with the tool invocation.
func (tc *ToolContext) SessionID() string { return tc.invocationCtx.SessionID }

// InvocationID returns the invocation ID associated with the tool invocation.
func (tc *ToolContext) InvocationID() string { return tc.invocationCtx.InvocationID }

// Logger returns the logger associated with the tool invocation.
func (tc *ToolContext) Logger() logging.Logger { return tc.invocationCtx.Logger }

// FunctionCallID returns the function call ID associated with the tool invocation.
func (tc *ToolContext) FunctionCallID() string { return tc.functionCallID }

// AgentName returns the agent name associated with the tool invocation.
func (tc *ToolContext) AgentName() string { return tc.agentInfo.Name }

// AgentType returns the agent type associated with the tool invocation.
func (tc *ToolContext) AgentType() string { return tc.agentInfo.Type }

// GetState retrieves the state associated with the given key.
func (tc *ToolContext) GetState(k string) (interface{}, bool) {
	return tc.invocationCtx.GetState(k)
}

// SetState records a state mutation both on the underlying invocation context
// (for immediate visibility) and in the local EventActions delta for emission.
func (tc *ToolContext) SetState(k string, v interface{}) {
	tc.invocationCtx.SetState(k, v)
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
	if tc.eventActions.TransferToAgent == nil {
		tc.eventActions.TransferToAgent = &name
	}

	if tc.invocationCtx.Logger != nil {
		tc.invocationCtx.Logger.Info("tool.transfer.request", "from_agent", tc.AgentName(), "to_agent", name, "function_call_id", tc.functionCallID)
	}
}

// Escalate requests escalation (e.g., to a higher-skill agent or human).
func (tc *ToolContext) Escalate() {
	b := true
	if tc.eventActions.Escalate == nil {
		tc.eventActions.Escalate = &b
	}

	if tc.invocationCtx.Logger != nil {
		tc.invocationCtx.Logger.Info("tool.escalate.request", "agent", tc.AgentName(), "function_call_id", tc.functionCallID)
	}
}

// SaveArtifact persists artifact bytes and records the delta size for emission.
func (tc *ToolContext) SaveArtifact(id string, data []byte) error {
	if tc.invocationCtx.ArtifactService == nil {
		return fmt.Errorf("artifact service not configured")
	}

	if err := tc.invocationCtx.ArtifactService.Save(tc.SessionID(), id, data); err != nil {
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
	if tc.invocationCtx.ArtifactService == nil {
		return nil, fmt.Errorf("artifact service not configured")
	}

	return tc.invocationCtx.ArtifactService.Get(tc.SessionID(), id)
}

// ListArtifacts returns artifact IDs stored for the session.
func (tc *ToolContext) ListArtifacts() ([]string, error) {
	if tc.invocationCtx.ArtifactService == nil {
		return nil, fmt.Errorf("artifact service not configured")
	}

	return tc.invocationCtx.ArtifactService.List(tc.SessionID())
}

// SearchMemory performs a recall query against the configured MemoryStore.
func (tc *ToolContext) SearchMemory(q string, limit int) ([]SearchResult, error) {
	if tc.invocationCtx.MemoryService == nil {
		return nil, fmt.Errorf("memory service not configured")
	}
	return tc.invocationCtx.MemoryService.Search(tc.SessionID(), q, limit)
}

// StoreMemory appends new content to the session's memory store with metadata.
func (tc *ToolContext) StoreMemory(content string, md map[string]interface{}) error {
	if tc.invocationCtx.MemoryService == nil {
		return fmt.Errorf("memory service not configured")
	}

	return tc.invocationCtx.MemoryService.Store(tc.SessionID(), content, md)
}

// GetSessionHistory returns conversation history (filtered) for context.
func (tc *ToolContext) GetSessionHistory() []Event {
	if tc.invocationCtx.Session == nil {
		return nil
	}

	return tc.invocationCtx.Session.GetConversationHistory()
}

// RefreshSession reloads the underlying session from the SessionStore.
func (tc *ToolContext) RefreshSession() error {
	if tc.invocationCtx.SessionService == nil {
		return fmt.Errorf("session service not configured")
	}

	s, err := tc.invocationCtx.SessionService.Get(tc.SessionID())
	if err != nil {
		return err
	}

	tc.invocationCtx.Session = s

	return nil
}

// EmitEvent sends an event directly without merging accumulated actions.
func (tc *ToolContext) EmitEvent(ev Event) error {
	if tc.invocationCtx.Emit == nil {
		return fmt.Errorf("emit channel not configured")
	}

	select {
	case <-tc.invocationCtx.Context.Done():
		return tc.invocationCtx.Context.Err()
	case tc.invocationCtx.Emit <- ev:
	}

	return nil
}

// Validate performs a structural sanity check of the context.
func (tc *ToolContext) Validate() error {
	if !tc.valid || tc.invocationCtx == nil || tc.invocationCtx.SessionID == "" || tc.functionCallID == "" {
		return fmt.Errorf("invalid ToolContext")
	}

	return nil
}

// IsValid reports whether Validate would succeed (fast path).
func (tc *ToolContext) IsValid() bool {
	return tc.valid && tc.invocationCtx != nil && tc.invocationCtx.SessionID != "" && tc.functionCallID != ""
}

// InternalInvocationContext returns the internal invocation context.
func (tc *ToolContext) InternalInvocationContext() *InvocationContext { return tc.invocationCtx }

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
		if tc.invocationCtx.Logger != nil {
			tc.invocationCtx.Logger.Info("tool.transfer.applied", "from_agent", tc.AgentName(), "to_agent", *tc.eventActions.TransferToAgent, "function_call_id", tc.functionCallID)
		}
	}

	if tc.eventActions.Escalate != nil {
		ev.Actions.Escalate = tc.eventActions.Escalate
		if tc.invocationCtx.Logger != nil {
			tc.invocationCtx.Logger.Info("tool.escalate.applied", "agent", tc.AgentName(), "function_call_id", tc.functionCallID)
		}
	}
}
