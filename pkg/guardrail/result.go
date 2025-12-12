package guardrail

// Result represents the outcome of a guardrail check.
// This is the generic result type used by all guardrails.
type Result struct {
	Action   Action         // What action to take (allow, reject, raise)
	Message  string         // Human-readable explanation
	Info     any            // Additional structured output (e.g., scores, categories)
	Metadata map[string]any // Additional context (e.g., matched patterns)
}

// Allow creates a result that allows execution to proceed.
func Allow() *Result {
	return &Result{Action: ActionAllow}
}

// AllowWithInfo creates an allow result with additional info.
func AllowWithInfo(info any) *Result {
	return &Result{Action: ActionAllow, Info: info}
}

// Reject creates a result that rejects content but continues execution.
// The message is sent back to the caller so it can try a different approach.
func Reject(message string) *Result {
	return &Result{Action: ActionReject, Message: message}
}

// RejectWithInfo creates a rejection result with additional info.
func RejectWithInfo(message string, info any) *Result {
	return &Result{Action: ActionReject, Message: message, Info: info}
}

// Raise creates a result that halts execution completely.
func Raise(message string) *Result {
	return &Result{Action: ActionRaise, Message: message}
}

// RaiseWithInfo creates a raise result with additional info.
func RaiseWithInfo(message string, info any) *Result {
	return &Result{Action: ActionRaise, Message: message, Info: info}
}

// IsTripwire returns true if this result should halt execution (for tripwire pattern).
func (r *Result) IsTripwire() bool {
	return r.Action == ActionRaise
}

// IsRejection returns true if this is a soft rejection (content rejected, can retry).
func (r *Result) IsRejection() bool {
	return r.Action == ActionReject
}

// IsAllowed returns true if execution should proceed normally.
func (r *Result) IsAllowed() bool {
	return r.Action == ActionAllow
}
