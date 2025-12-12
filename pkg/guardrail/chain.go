package guardrail

import "context"

// Chain runs multiple guardrails in sequence, stopping at the first non-allow result.
func Chain[T any](ctx context.Context, input T, guardrails ...Guardrail[T]) (*Result, error) {
	for _, g := range guardrails {
		result, err := g.Check(ctx, input)
		if err != nil {
			return nil, err
		}

		if !result.IsAllowed() {
			return result, nil
		}
	}

	return Allow(), nil
}

// ChainGuardrail wraps multiple guardrails into a single guardrail.
type ChainGuardrail[T any] struct {
	name       string
	guardrails []Guardrail[T]
}

// NewChainGuardrail creates a guardrail that chains multiple guardrails.
func NewChainGuardrail[T any](name string, guardrails ...Guardrail[T]) *ChainGuardrail[T] {
	return &ChainGuardrail[T]{name: name, guardrails: guardrails}
}

// Name returns the name of the chain.
func (c *ChainGuardrail[T]) Name() string { return c.name }

// Check runs all guardrails in sequence, stopping at the first non-allow result.
func (c *ChainGuardrail[T]) Check(ctx context.Context, input T) (*Result, error) {
	return Chain(ctx, input, c.guardrails...)
}

// All runs multiple guardrails in sequence, stopping at the first non-allow result.
// This is an alias for NewChainGuardrail with a default name.
func All[T any](guardrails ...Guardrail[T]) *ChainGuardrail[T] {
	return NewChainGuardrail("all", guardrails...)
}

// AnyGuardrail runs multiple guardrails and returns Allow if any guardrail allows.
// Returns the first Allow result, or the last non-Allow result if none allow.
type AnyGuardrail[T any] struct {
	name       string
	guardrails []Guardrail[T]
}

// NewAnyGuardrail creates a guardrail that returns Allow if any guardrail allows.
func NewAnyGuardrail[T any](name string, guardrails ...Guardrail[T]) *AnyGuardrail[T] {
	return &AnyGuardrail[T]{name: name, guardrails: guardrails}
}

// Name returns the name of the any-guardrail.
func (a *AnyGuardrail[T]) Name() string { return a.name }

// Check runs all guardrails and returns Allow if any guardrail allows.
func (a *AnyGuardrail[T]) Check(ctx context.Context, input T) (*Result, error) {
	var lastResult *Result

	for _, g := range a.guardrails {
		result, err := g.Check(ctx, input)
		if err != nil {
			return nil, err
		}

		if result.IsAllowed() {
			return result, nil
		}

		lastResult = result
	}

	if lastResult != nil {
		return lastResult, nil
	}

	return Allow(), nil
}

// Any creates an any-guardrail with a default name.
func Any[T any](guardrails ...Guardrail[T]) *AnyGuardrail[T] {
	return NewAnyGuardrail("any", guardrails...)
}
