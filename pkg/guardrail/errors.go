package guardrail

import "fmt"

// TripwireError is returned when a guardrail raises (halts execution).
type TripwireError struct {
	GuardrailName string
	Message       string
	Info          any
}

// Error implements the error interface.
func (e *TripwireError) Error() string {
	return fmt.Sprintf("guardrail %q triggered: %s", e.GuardrailName, e.Message)
}

// NewTripwireError creates a tripwire error from a result.
func NewTripwireError(guardrailName string, result *Result) *TripwireError {
	return &TripwireError{
		GuardrailName: guardrailName,
		Message:       result.Message,
		Info:          result.Info,
	}
}

// Rejection represents a soft rejection from a guardrail.
// Implements the error interface for use in error returns.
type Rejection struct {
	GuardrailName string
	Message       string
	Info          any
}

// Error implements the error interface.
func (r *Rejection) Error() string {
	return fmt.Sprintf("guardrail %q rejected: %s", r.GuardrailName, r.Message)
}

// NewRejection creates a rejection from a result.
func NewRejection(guardrailName string, result *Result) *Rejection {
	return &Rejection{
		GuardrailName: guardrailName,
		Message:       result.Message,
		Info:          result.Info,
	}
}
