package testutil

import (
	"time"

	"github.com/hupe1980/agentmesh/core"
)

// EventBuilder provides a fluent helper for constructing events in tests.
// Example:
//
//	ev := NewEventBuilder().Author("agent").Invocation("inv-1").AssistantText("hello").Build()
//
// Chain only the parts you need; sensible defaults are applied.
type EventBuilder struct {
	author        string
	invocationID  string
	id            string
	role          string
	textParts     []string
	funcCalls     []core.FunctionCall
	funcResponses []core.FunctionResponse
	partial       *bool
	turnComplete  *bool
	customParts   []core.Part
	actions       core.EventActions
	branch        *string
	longRunning   []string
}

// NewEventBuilder creates a builder with default author "agent".
func NewEventBuilder() *EventBuilder { return &EventBuilder{author: "agent"} }

// Author sets the author name for the event (chainable).
func (b *EventBuilder) Author(a string) *EventBuilder { b.author = a; return b }

// Invocation sets the invocation ID associated with the event (chainable).
func (b *EventBuilder) Invocation(id string) *EventBuilder { b.invocationID = id; return b }

// ID overrides the auto-generated event ID (chainable). Use mainly in tests where determinism matters.
func (b *EventBuilder) ID(id string) *EventBuilder { b.id = id; return b }

// Branch sets the branch pointer for forked conversation paths (chainable).
func (b *EventBuilder) Branch(br string) *EventBuilder { b.branch = &br; return b }

// Partial marks the event as a streaming / partial chunk (chainable).
func (b *EventBuilder) Partial(p bool) *EventBuilder { b.partial = &p; return b }

// TurnComplete sets the TurnComplete flag indicating model turn completion (chainable).
func (b *EventBuilder) TurnComplete(c bool) *EventBuilder { b.turnComplete = &c; return b }

// UserText appends a user role text part and sets role to user (chainable).
func (b *EventBuilder) UserText(t string) *EventBuilder {
	b.role = "user"
	b.textParts = append(b.textParts, t)
	return b
}

// AssistantText appends an assistant role text part and sets role to assistant (chainable).
func (b *EventBuilder) AssistantText(t string) *EventBuilder {
	b.role = "assistant"
	b.textParts = append(b.textParts, t)
	return b
}

// ToolText appends a tool role text part and sets role to tool (chainable).
func (b *EventBuilder) ToolText(t string) *EventBuilder {
	b.role = "tool"
	b.textParts = append(b.textParts, t)
	return b
}

// AddPart appends a custom content part (chainable).
func (b *EventBuilder) AddPart(p core.Part) *EventBuilder {
	b.customParts = append(b.customParts, p)
	return b
}

// FunctionCall adds a function call part with the provided name and JSON argument string (chainable).
func (b *EventBuilder) FunctionCall(name, args string) *EventBuilder {
	b.funcCalls = append(b.funcCalls, core.FunctionCall{Name: name, Arguments: args})
	return b
}

// FunctionResponse adds a function response part representing tool execution output (chainable).
func (b *EventBuilder) FunctionResponse(id, name string, result interface{}, err error) *EventBuilder {
	fr := core.FunctionResponse{ID: id, Name: name, Response: result}
	if err != nil {
		fr.Error = err.Error()
	}
	b.funcResponses = append(b.funcResponses, fr)
	return b
}

// SkipSummarization sets the SkipSummarization action flag (chainable).
func (b *EventBuilder) SkipSummarization() *EventBuilder {
	t := true
	b.actions.SkipSummarization = &t
	return b
}

// Escalate sets the Escalate action flag (chainable).
func (b *EventBuilder) Escalate() *EventBuilder { t := true; b.actions.Escalate = &t; return b }

// Transfer sets the target agent for a transfer action (chainable).
func (b *EventBuilder) Transfer(to string) *EventBuilder { b.actions.TransferToAgent = &to; return b }

// LongRunning registers one or more long-running tool IDs on the event (chainable).
func (b *EventBuilder) LongRunning(ids ...string) *EventBuilder {
	b.longRunning = append(b.longRunning, ids...)
	return b
}

// Build constructs the core.Event value.
func (b *EventBuilder) Build() core.Event {
	ev := core.NewEvent(b.author, b.invocationID)
	if b.id != "" {
		ev.ID = b.id
	}
	if b.branch != nil {
		ev.Branch = b.branch
	}
	if b.partial != nil {
		ev.Partial = b.partial
	}
	if b.turnComplete != nil {
		ev.TurnComplete = b.turnComplete
	}
	if len(b.longRunning) > 0 {
		ev.LongRunningToolIDs = append([]string{}, b.longRunning...)
	}
	ev.Actions = b.actions

	// Assemble content parts (preallocate for efficiency)
	estimatedParts := len(b.textParts) + len(b.funcCalls) + len(b.funcResponses) + len(b.customParts)
	parts := make([]core.Part, 0, estimatedParts)
	for _, t := range b.textParts {
		parts = append(parts, core.TextPart{Text: t})
	}
	for _, fc := range b.funcCalls {
		parts = append(parts, core.FunctionCallPart{FunctionCall: fc})
	}
	for _, fr := range b.funcResponses {
		parts = append(parts, core.FunctionResponsePart{FunctionResponse: fr})
	}
	parts = append(parts, b.customParts...)
	if len(parts) > 0 {
		role := b.role
		if role == "" {
			role = "assistant"
		}
		ev.Content = &core.Content{Role: role, Parts: parts}
	}
	// Ensure timestamp monotonic-ish (already set in constructor); we could adjust if needed.
	_ = time.Now()
	return ev
}
