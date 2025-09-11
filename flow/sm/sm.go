package sm

import "fmt"

// Machine is a generic condition-based state machine.
type Machine[S comparable, D any] struct {
	start       S
	transitions []Transition[S, D]
}

// Transition describes one edge between two states.
type Transition[S comparable, D any] struct {
	From      S
	To        S
	Condition func(*D) bool
}

// New creates a new machine with a start state.
func New[S comparable, D any](start S) *Machine[S, D] {
	return &Machine[S, D]{
		start:       start,
		transitions: []Transition[S, D]{},
	}
}

// AddTransition adds a transition with an optional condition.
// If Condition == nil, it always matches.
func (m *Machine[S, D]) AddTransition(from, to S, cond func(*D) bool) {
	m.transitions = append(m.transitions, Transition[S, D]{From: from, To: to, Condition: cond})
}

// Run executes the state machine until no more transitions apply or Stop is returned.
// Each state must be handled by the provided stepFn.
func (m *Machine[S, D]) Run(frame *D, stepFn func(state S, frame *D) error) error {
	state := m.start

	for {
		// Run state logic
		if err := stepFn(state, frame); err != nil {
			return fmt.Errorf("state %v failed: %w", state, err)
		}

		// Find next transition
		next, ok := m.next(state, frame)
		if !ok {
			// no valid transition -> stop
			return nil
		}

		// allow self-transitions; they'll execute stepFn again
		state = next
	}
}

func (m *Machine[S, D]) next(from S, frame *D) (S, bool) {
	for _, t := range m.transitions {
		if t.From == from && (t.Condition == nil || t.Condition(frame)) {
			return t.To, true
		}
	}

	return from, false
}
