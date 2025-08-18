package core

import (
	"time"

	"github.com/google/uuid"
)

// EventActions encodes side‑effects or orchestration signals attached to an Event.
// All fields are optional pointers / maps so absence can be distinguished from zero values.
// The engine interprets these after persistence (see engine.ApplyEventActions).
type EventActions struct {
	SkipSummarization    *bool          `json:"skip_summarization,omitempty"`
	StateDelta           map[string]any `json:"state_delta,omitempty"`
	ArtifactDelta        map[string]int `json:"artifact_delta,omitempty"`
	TransferToAgent      *string        `json:"transfer_to_agent,omitempty"`
	Escalate             *bool          `json:"escalate,omitempty"`
	RequestedAuthConfigs map[string]any `json:"requested_auth_configs,omitempty"`
}

// Event is the primary unit of communication between agents, the engine and
// external clients. After emission it should be treated as immutable. It
// captures:
//   - Correlation (InvocationID, ID, Author)
//   - Conversational content (optional role-based Parts)
//   - Orchestration directives (Actions)
//   - Tool / long‑running operation hints (LongRunningToolIDs)
//   - Error / interruption metadata
//   - High precision UTC timestamp
//
// Content may be nil for control or error-only events. Timestamp uses a native
// time.Time (UTC). Use helper methods (e.g. UnixSeconds) if numeric forms are
// needed for metrics or legacy clients.
type Event struct {
	ID                 string            `json:"id"`
	InvocationID       string            `json:"invocation_id"`
	Author             string            `json:"author"`
	Actions            EventActions      `json:"actions"`
	LongRunningToolIDs []string          `json:"long_running_tool_ids,omitempty"`
	Branch             *string           `json:"branch,omitempty"`
	Timestamp          time.Time         `json:"timestamp"`
	Content            *Content          `json:"content,omitempty"`
	Partial            *bool             `json:"partial,omitempty"`
	TurnComplete       *bool             `json:"turn_complete,omitempty"`
	ErrorCode          *string           `json:"error_code,omitempty"`
	ErrorMessage       *string           `json:"error_message,omitempty"`
	Interrupted        *bool             `json:"interrupted,omitempty"`
	GroundingMetadata  any               `json:"grounding_metadata,omitempty"`
	CustomMetadata     map[string]string `json:"custom_metadata,omitempty"`
}

// NewEvent creates a bare event authored by 'author' bound to an invocation.
// Prefer helper constructors for common semantic categories (message, function call/response).
func NewEvent(invocationID, author string) Event {
	return Event{
		ID:           NewID(),
		InvocationID: invocationID,
		Author:       author,
		Timestamp:    time.Now().UTC(),
		Actions:      EventActions{},
	}
}

// NewMessageEvent constructs an assistant-style message event with single text part.
// Author can be an agent name or system identifier.
// NewMessageEvent creates a non-user assistant message event with a single text part.
func NewMessageEvent(author, message string) Event {
	e := NewEvent("", author)
	e.Content = &Content{Role: "assistant", Parts: []Part{TextPart{Text: message}}}
	return e
}

// NewUserMessageEvent convenience wrapper for a user-authored text message.
// NewUserMessageEvent creates a user-authored text message event.
func NewUserMessageEvent(invocationID, message string) Event {
	e := NewEvent(invocationID, "user")
	e.Content = &Content{Role: "user", Parts: []Part{TextPart{Text: message}}}
	return e
}

// NewUserContentEvent creates a user-authored event with arbitrary Content.
// Useful for cases where the Content is not just a simple text message.
func NewUserContentEvent(invocationID string, content *Content) Event {
	e := NewEvent(invocationID, "user")
	e.Content = content
	return e
}

// NewFunctionCallEvent represents a tool / function invocation request emitted by an agent.
// NewFunctionCallEvent represents an agent requesting execution of a named function/tool.
func NewFunctionCallEvent(author, functionName, args string) Event {
	e := NewEvent("", author)
	e.Content = &Content{
		Role: "assistant",
		Parts: []Part{
			FunctionCallPart{
				FunctionCall: FunctionCall{
					Name:      functionName,
					Arguments: args,
				},
			},
		},
	}
	return e
}

// NewFunctionResponseEvent captures the outcome of a previously emitted function call.
// If err is non-nil its message is copied into the response.Error field.
// NewFunctionResponseEvent records the completion result (or error) of a tool/function invocation.
func NewFunctionResponseEvent(author, id, functionName string, result interface{}, err error) Event {
	e := NewEvent("", author)
	fr := FunctionResponse{ID: id, Name: functionName, Response: result}
	if err != nil {
		fr.Error = err.Error()
	}
	e.Content = &Content{Role: "tool", Parts: []Part{FunctionResponsePart{FunctionResponse: fr}}}
	return e
}

// NewID generates a new unique identifier for events.
//
// This function creates a UUID-based unique identifier that can be used
// for event tracking and correlation throughout the framework.
//
// Returns a string representation of a new UUID.
func NewID() string { return uuid.NewString() }

// IsPartial reports whether this event represents a streaming / incomplete
// fragment that will be followed by additional events composing the final
// assistant turn.
func (e Event) IsPartial() bool { return e.Partial != nil && *e.Partial }

// GetFunctionCalls returns any FunctionCall parts contained within the event
// content preserving their original order.
func (e Event) GetFunctionCalls() []FunctionCall {
	if e.Content == nil {
		return nil
	}
	var calls []FunctionCall
	for _, p := range e.Content.Parts {
		if fc, ok := p.(FunctionCallPart); ok {
			calls = append(calls, fc.FunctionCall)
		}
	}
	return calls
}

// GetFunctionResponses returns any FunctionResponse parts contained within the
// event content preserving their original order.
func (e Event) GetFunctionResponses() []FunctionResponse {
	if e.Content == nil {
		return nil
	}
	var responses []FunctionResponse
	for _, p := range e.Content.Parts {
		if fr, ok := p.(FunctionResponsePart); ok {
			responses = append(responses, fr.FunctionResponse)
		}
	}
	return responses
}

// HasTrailingCodeExecutionResult always false (future code exec placeholder).
func (e Event) HasTrailingCodeExecutionResult() bool { return false }

// IsFinalResponse implements heuristic used by higher layers to decide when an
// assistant turn is complete (no pending tool calls/responses, not partial, not skipped summarization).
func (e Event) IsFinalResponse() bool {
	if (e.Actions.SkipSummarization != nil && *e.Actions.SkipSummarization) || len(e.LongRunningToolIDs) > 0 {
		return true
	}

	return len(e.GetFunctionCalls()) == 0 &&
		len(e.GetFunctionResponses()) == 0 &&
		!e.IsPartial() &&
		!e.HasTrailingCodeExecutionResult()
}

// UnixSeconds returns the timestamp as fractional seconds since Unix epoch.
// Useful for metrics & numeric serialization paths.
func (e Event) UnixSeconds() float64 { return float64(e.Timestamp.UnixNano()) / 1e9 }
