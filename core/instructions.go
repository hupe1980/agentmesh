package core

import "context"

// InstructionsProvider supplies dynamic instruction text at runtime.
// Implementations can derive instructions from session state, configuration, or environment.
type InstructionsProvider interface {
	Instructions(ctx context.Context, roCtx ReadonlyContext) (string, error)
}

// InstructionsProviderFunc is a functional adapter to allow ordinary functions to be used as InstructionsProviders.
type InstructionsProviderFunc func(ctx context.Context, roCtx ReadonlyContext) (string, error)

// Instructions implements InstructionsProvider for InstructionsProviderFunc.
func (f InstructionsProviderFunc) Instructions(ctx context.Context, roCtx ReadonlyContext) (string, error) {
	return f(ctx, roCtx)
}

// Instructions represents either a static instruction string or a dynamic provider.
// This mirrors a union of string | provider in a Go-idiomatic way.
type Instructions struct {
	text     string
	provider InstructionsProvider
}

// NewInstructionsFromText creates Instructions from a static string.
func NewInstructionsFromText(text string) Instructions { return Instructions{text: text} }

// NewInstructionsFromProvider creates Instructions from a dynamic provider.
func NewInstructionsFromProvider(p InstructionsProvider) Instructions {
	return Instructions{provider: p}
}

// NewInstructionsFromFunc creates Instructions from a function.
func NewInstructionsFromFunc(f func(context.Context, ReadonlyContext) (string, error)) Instructions {
	return Instructions{provider: InstructionsProviderFunc(f)}
}

// IsStatic returns true if the instruction is backed by a static string.
func (i Instructions) IsStatic() bool { return i.provider == nil }

// Resolve returns the instruction text, invoking the provider if needed.
func (i Instructions) Resolve(ctx context.Context, roCtx ReadonlyContext) (string, error) {
	if i.provider != nil {
		return i.provider.Instructions(ctx, roCtx)
	}

	return i.text, nil
}
