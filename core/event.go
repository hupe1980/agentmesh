package core

import (
	"time"

	"github.com/hupe1980/agentmesh/internal/util"
)

// EventActions encodes side‑effects or orchestration signals attached to an Event.
// All fields are optional pointers / maps so absence can be distinguished from zero values.
// The engine interprets these after persistence (see engine.ApplyEventActions).
type EventActions struct {
	SkipSummarization *bool          `json:"skip_summarization,omitempty"`
	StateDelta        map[string]any `json:"state_delta,omitempty"`
	ArtifactDelta     map[string]int `json:"artifact_delta,omitempty"`
	TransferToAgent   *string        `json:"transfer_to_agent,omitempty"`
	Escalate          *bool          `json:"escalate,omitempty"`
}

// Event is the primary unit of communication between agents, the engine and
// external clients. After emission it should be treated as immutable. It
// captures:
//   - Correlation (RunID, ID, Author)
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
	ID             string            `json:"id"`
	RunID          string            `json:"run_id"`
	Author         string            `json:"author"`
	Actions        EventActions      `json:"actions"`
	Branch         *string           `json:"branch,omitempty"`
	Timestamp      time.Time         `json:"timestamp"`
	Content        *Content          `json:"content,omitempty"`
	Partial        *bool             `json:"partial,omitempty"`
	TurnComplete   *bool             `json:"turn_complete,omitempty"`
	ErrorCode      *string           `json:"error_code,omitempty"`
	ErrorMessage   *string           `json:"error_message,omitempty"`
	Interrupted    *bool             `json:"interrupted,omitempty"`
	CustomMetadata map[string]string `json:"custom_metadata,omitempty"`
}

// NewAssistantEvent creates an assistant-authored event with the given content and partial flag.
// If partial is false and there are no function calls, the event is marked as turn complete.
func NewAssistantEvent(runID, author string, content Content, partial bool) Event {
	e := newEvent(runID, author)

	e.Content = &content
	e.Partial = &partial

	// Mark turn complete if not partial and no function calls
	if !partial && len(e.GetFunctionCalls()) == 0 {
		complete := true
		e.TurnComplete = &complete
	}

	return e
}

// NewMessageEvent constructs an assistant-style message event with single text part.
// Author can be an agent name or system identifier.
// NewMessageEvent creates a non-user assistant message event with a single text part.
func NewMessageEvent(author, message string) Event {
	e := newEvent("", author)
	e.Content = &Content{Role: "assistant", Parts: []Part{TextPart{Text: message}}}
	return e
}

// NewUserMessageEvent convenience wrapper for a user-authored text message.
// NewUserMessageEvent creates a user-authored text message event.
func NewUserMessageEvent(runID, message string) Event {
	e := newEvent(runID, "user")
	e.Content = &Content{Role: "user", Parts: []Part{TextPart{Text: message}}}
	return e
}

// NewUserContentEvent creates a user-authored event with arbitrary Content.
// Useful for cases where the Content is not just a simple text message.
func NewUserContentEvent(runID string, content *Content) Event {
	e := newEvent(runID, "user")
	e.Content = content
	return e
}

// NewFunctionCallEvent represents a tool / function invocation request emitted by an agent.
// NewFunctionCallEvent represents an agent requesting execution of a named function/tool.
func NewFunctionCallEvent(author, functionName, args string) Event {
	e := newEvent("", author)
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
	e := newEvent("", author)
	fr := FunctionResponse{ID: id, Name: functionName, Response: result}
	if err != nil {
		fr.Error = err.Error()
	}
	e.Content = &Content{Role: "tool", Parts: []Part{FunctionResponsePart{FunctionResponse: fr}}}
	return e
}

func NewSkipSummarizationEvent(runID, author string, content Content) Event {
	ev := newEvent(runID, author)

	skip := true
	ev.Actions.SkipSummarization = &skip
	ev.Content = &content

	return ev
}

// NewEscalationEvent creates an event that signals escalation to the parent agent.
func NewEscalationEvent(runID, author string, content Content) Event {
	ev := newEvent(runID, author)

	escalate := true
	ev.Actions.Escalate = &escalate
	ev.Content = &content

	return ev
}

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
	if e.Actions.SkipSummarization != nil && *e.Actions.SkipSummarization {
		return true
	}

	return len(e.GetFunctionCalls()) == 0 &&
		len(e.GetFunctionResponses()) == 0 &&
		!e.IsPartial() &&
		!e.HasTrailingCodeExecutionResult()
}

// newEvent creates a new Event with the given runID and author.
func newEvent(runID, author string) Event {
	return Event{
		ID:        util.NewID(),
		RunID:     runID,
		Author:    author,
		Timestamp: time.Now().UTC(),
		Actions:   EventActions{},
	}
}
