package core

import "context"

// Example represents a single input/output pair that can be surfaced to the model
// as part of few-shot prompting or demonstrations for desired behavior.
type Example struct {
	Input  Parts
	Output Parts
}

// ExampleProvider supplies dynamic example lists at runtime.
// Implementations can derive examples from session state, configuration, or environment.
type ExampleProvider interface {
	Examples(ctx context.Context, roCtx ReadonlyContext) ([]Example, error)
}

// ExampleProviderFunc is a functional adapter to allow ordinary functions to be used as ExampleProviders.
type ExampleProviderFunc func(ctx context.Context, roCtx ReadonlyContext) ([]Example, error)

// Examples implements ExampleProvider for ExampleProviderFunc.
func (f ExampleProviderFunc) Examples(ctx context.Context, roCtx ReadonlyContext) ([]Example, error) {
	return f(ctx, roCtx)
}

// Examples represents either a static example list or a dynamic provider.
// This mirrors a union of []Example | provider in a Go-idiomatic way.
type Examples struct {
	list     []Example
	provider ExampleProvider
}

// NewExamples creates Examples from one or more static examples, copying the slice to avoid external mutation.
func NewExamples(examples ...Example) Examples {
	return Examples{list: append([]Example(nil), examples...)}
}

// NewExamplesFromProvider creates Examples from a dynamic provider.
func NewExamplesFromProvider(p ExampleProvider) Examples {
	return Examples{provider: p}
}

// NewExamplesFromFunc creates Examples from a function.
func NewExamplesFromFunc(f func(context.Context, ReadonlyContext) ([]Example, error)) Examples {
	return Examples{provider: ExampleProviderFunc(f)}
}

// IsStatic returns true if the examples are backed by a static slice.
func (e Examples) IsStatic() bool { return e.provider == nil }

// Resolve returns the examples, invoking the provider if needed.
func (e Examples) Resolve(ctx context.Context, roCtx ReadonlyContext) ([]Example, error) {
	if e.provider != nil {
		return e.provider.Examples(ctx, roCtx)
	}

	return append([]Example(nil), e.list...), nil
}
