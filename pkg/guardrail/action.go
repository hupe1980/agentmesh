package guardrail

// Action represents what action to take after a guardrail check.
type Action int

const (
	// ActionAllow allows execution to proceed normally.
	ActionAllow Action = iota

	// ActionReject rejects the content but continues execution.
	// The caller receives a rejection message and can try a different approach.
	ActionReject

	// ActionRaise halts execution completely.
	ActionRaise
)

// String returns the string representation of the action.
func (a Action) String() string {
	switch a {
	case ActionAllow:
		return "allow"
	case ActionReject:
		return "reject"
	case ActionRaise:
		return "raise"
	default:
		return "unknown"
	}
}
