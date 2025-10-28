package core

import (
	"context"
	"maps"
	"time"

	"github.com/google/uuid"
)

// EventWriter is a minimal queue abstraction for writing events with cancellation.
// Implementations must not mutate the provided event; treat it as immutable.
type EventWriter interface {
	Write(ctx context.Context, ev *Event) error
}

// EventActions encodes orchestration signals and side-effects attached to an Event.
// Fields are optional so absence can be distinguished from zero values.
// These are interpreted by the runner/engine after persistence (see ApplyToolActions).
type EventActions struct {
	SkipSummarization Opt[bool]           `json:"skip_summarization,omitempty"`
	StateDelta        Opt[map[string]any] `json:"state_delta,omitempty"`
	ArtifactDelta     Opt[map[string]int] `json:"artifact_delta,omitempty"`
	TransferToAgent   Opt[string]         `json:"transfer_to_agent,omitempty"`
	Escalate          Opt[bool]           `json:"escalate,omitempty"`
}

// Event is the primary unit of communication between agents, the engine, and
// external clients.
type Event struct {
	ID        string       `json:"id"`
	RunID     string       `json:"run_id"`
	Author    string       `json:"author"`
	Timestamp time.Time    `json:"timestamp"`
	Parts     []Part       `json:"parts,omitempty"`
	Actions   EventActions `json:"actions"`

	LongRunningToolIDs Opt[[]string]          `json:"long_running_tool_ids,omitempty"`
	Branch             Opt[string]            `json:"branch,omitempty"`
	Partial            Opt[bool]              `json:"partial,omitempty"`
	TurnComplete       Opt[bool]              `json:"turn_complete,omitempty"`
	ErrorCode          Opt[string]            `json:"error_code,omitempty"`
	ErrorMessage       Opt[string]            `json:"error_message,omitempty"`
	CustomMetadata     Opt[map[string]string] `json:"custom_metadata,omitempty"`
}

// NewFullAssistantEvent creates an assistant-authored event with the given parts.
func NewFullAssistantEvent(runID, author string, parts ...Part) *Event {
	e := newEvent(runID, author, parts...)

	// Mark turn complete if no function calls
	if !e.HasFunctionCalls() {
		e.TurnComplete = Bool(true)
	}

	return e
}

// NewPartialAssistantEvent creates a partial assistant event with the given parts.
func NewPartialAssistantEvent(runID, author string, parts ...Part) *Event {
	e := newEvent(runID, author, parts...)
	e.Partial = Bool(true)

	return e
}

// NewUserContentEvent creates a user-authored event with arbitrary parts.
// Useful for cases where the content is not just a simple text message.
func NewUserContentEvent(runID string, parts ...Part) *Event {
	e := newEvent(runID, "user", parts...)
	return e
}

// NewFunctionResponseEvent records the completion result (or error) of a tool/function invocation.
// If err is non-nil its message is copied into the response.Error field.
func NewFunctionResponseEvent(runID, author, callID, functionName string, result any) *Event {
	e := newEvent(runID, author, NewPartFromFunctionResponse(callID, functionName, result))
	return e
}

// IsPartial reports whether this event represents a streaming / incomplete
// fragment that will be followed by additional events composing the final
// assistant turn.
func (e *Event) IsPartial() bool { return e.Partial.Or(false) }

// HasFunctionCalls reports whether this event contains any function call parts.
func (e *Event) HasFunctionCalls() bool {
	for _, p := range e.Parts {
		if _, ok := p.(*FunctionCallPart); ok {
			return true
		}
	}

	return false
}

// GetFunctionCalls returns any FunctionCall parts contained within the event
// content preserving their original order.
func (e *Event) GetFunctionCalls() []*FunctionCall {
	var calls []*FunctionCall
	for _, p := range e.Parts {
		if fc, ok := p.(*FunctionCallPart); ok {
			calls = append(calls, fc.FunctionCall)
		}
	}

	return calls
}

// HasFunctionResponses reports whether this event contains any function response parts.
func (e *Event) HasFunctionResponses() bool {
	for _, p := range e.Parts {
		if _, ok := p.(*FunctionResponsePart); ok {
			return true
		}
	}

	return false
}

// GetFunctionResponses returns any FunctionResponse parts contained within the
// event content preserving their original order.
func (e *Event) GetFunctionResponses() []*FunctionResponse {
	var responses []*FunctionResponse
	for _, p := range e.Parts {
		if fr, ok := p.(*FunctionResponsePart); ok {
			responses = append(responses, fr.FunctionResponse)
		}
	}

	return responses
}

// IsFinalResponse implements heuristic used by higher layers to decide when an
// assistant turn is complete (no pending tool calls/responses, not partial, not skipped summarization).
func (e *Event) IsFinalResponse() bool {
	if e.Role() == RoleUser {
		return false
	}

	if sa, ok := e.Actions.SkipSummarization.Get(); ok && sa {
		return true
	}

	return !e.HasFunctionCalls() && !e.HasFunctionResponses() && !e.IsPartial()
}

// Text aggregates all text parts into a single string.
func (e *Event) Text() string {
	txt, _ := ExtractTextFromParts(e.Parts, false)
	return txt
}

// Clone returns a copy of the event with independent maps and option pointers.
// Content is shallow-cloned: the Content struct and Parts slice header are copied,
// but individual Part values are not deep-copied. This is sufficient to protect
// persisted history from accidental mutations to top-level fields while keeping
// cloning inexpensive. For stronger guarantees, introduce type-specific clones
// for Part implementations as needed.
func (e *Event) Clone() *Event {
	clone := &Event{
		ID:        e.ID,
		RunID:     e.RunID,
		Author:    e.Author,
		Timestamp: e.Timestamp,
	}

	// Copy Actions
	clone.Actions = EventActions{
		SkipSummarization: e.Actions.SkipSummarization,
		TransferToAgent:   e.Actions.TransferToAgent,
		Escalate:          e.Actions.Escalate,
	}

	// Clone StateDelta if set
	if e.Actions.StateDelta.IsSet() {
		clone.Actions.StateDelta = Map(maps.Clone(e.Actions.StateDelta.Or(nil)))
	}

	// Clone ArtifactDelta if set
	if e.Actions.ArtifactDelta.IsSet() {
		clone.Actions.ArtifactDelta = Map(maps.Clone(e.Actions.ArtifactDelta.Or(nil)))
	}

	// Copy optional fields
	clone.Branch = e.Branch
	clone.Partial = e.Partial
	clone.TurnComplete = e.TurnComplete
	clone.ErrorCode = e.ErrorCode
	clone.ErrorMessage = e.ErrorMessage

	// Deep-copy Parts
	if len(e.Parts) > 0 {
		clone.Parts = make([]Part, len(e.Parts))
		for i, p := range e.Parts {
			clone.Parts[i] = ClonePart(p)
		}
	}

	// Copy CustomMetadata if set
	if e.CustomMetadata.IsSet() {
		clone.CustomMetadata = Map(maps.Clone(e.CustomMetadata.Or(nil)))
	}

	return clone
}

// Role derives the role for this event's content for provider messages/history.
// Heuristic:
// - If event contains any function responses: tool
// - Else if the author is "user": user
// - Else: assistant
func (e *Event) Role() Role {
	if e.HasFunctionResponses() {
		return RoleTool
	}

	if e.Author == string(RoleUser) {
		return RoleUser
	}

	return RoleAssistant
}

// ApplyActions merges the provided actions into this event.
// It is intended to be called by the engine/flows right before emission/persistence.
func (e *Event) ApplyActions(actions *EventActions) {
	if actions == nil {
		return
	}

	// Merge state and artifact deltas using helper
	e.Actions.StateDelta = MergeMap(e.Actions.StateDelta, actions.StateDelta)
	e.Actions.ArtifactDelta = MergeMap(e.Actions.ArtifactDelta, actions.ArtifactDelta)

	e.Actions.TransferToAgent = actions.TransferToAgent
	e.Actions.Escalate = actions.Escalate
	e.Actions.SkipSummarization = actions.SkipSummarization
}

// newEvent creates a new Event with the given runID, author and optional parts.
func newEvent(runID, author string, parts ...Part) *Event {
	return &Event{
		ID:        uuid.NewString(),
		RunID:     runID,
		Author:    author,
		Timestamp: time.Now(),
		Parts:     parts,
		Actions:   EventActions{},
	}
}
