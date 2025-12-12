package guardrail

import "context"

// Guardrail is the generic interface for content safety checks.
// The type parameter T represents the input data type.
//
// This interface is intentionally generic - specific implementations
// (tool guardrails, agent guardrails) are created in their respective packages.
type Guardrail[T any] interface {
	// Name returns a unique identifier for this guardrail.
	Name() string

	// Check validates the input and returns a result.
	Check(ctx context.Context, input T) (*Result, error)
}

// Func is a function adapter for Guardrail.
type Func[T any] struct {
	name string
	fn   func(ctx context.Context, input T) (*Result, error)
}

// NewFunc creates a guardrail from a function.
func NewFunc[T any](name string, fn func(ctx context.Context, input T) (*Result, error)) *Func[T] {
	return &Func[T]{name: name, fn: fn}
}

// Name returns the name of the guardrail.
func (g *Func[T]) Name() string { return g.name }

// Check validates the input using the wrapped function.
func (g *Func[T]) Check(ctx context.Context, input T) (*Result, error) {
	return g.fn(ctx, input)
}

// StringGuardrail is a type alias for string-based guardrails.
// Most built-in guardrails operate on strings.
type StringGuardrail = Guardrail[string]
