// Package tool provides sentinel errors for the tool package.
package tool

import "errors"

var (
	// ErrNilOutputSchema is returned when an output schema is nil.
	ErrNilOutputSchema = errors.New("tool/set_model_response: nil output schema")

	// ErrNilOutputSchemaPointer is returned when an output schema pointer is nil.
	ErrNilOutputSchemaPointer = errors.New("tool/set_model_response: nil output schema pointer")

	// ErrEmptyQuery is returned when a query is empty.
	ErrEmptyQuery = errors.New("tool: query cannot be empty")

	// ErrInvalidAgentResult is returned when an agent returns an invalid result.
	ErrInvalidAgentResult = errors.New("tool/handoff: agent returned invalid result")

	// ErrNoAgentMessages is returned when an agent produces no messages.
	ErrNoAgentMessages = errors.New("tool/handoff: agent produced no messages")
)
