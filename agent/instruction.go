package agent

import "github.com/hupe1980/agentmesh/core"

// Provider supplies dynamic instruction text at runtime.
// Implementations can derive instructions from session state, environment, etc.
type Provider interface {
	Instruction(*core.RunContext) (string, error)
}

// Func is a functional adapter to allow ordinary functions to be used as Providers.
type Func func(*core.RunContext) (string, error)

// Instruction implements Provider.
func (f Func) Instruction(ic *core.RunContext) (string, error) { return f(ic) }

// Instruction represents either a static instruction string or a dynamic provider.
// This mirrors a union of string | provider in a Go-idiomatic way.
type Instruction struct {
	text     string
	provider Provider
}

// NewInstructionFromText creates an Instruction from a static string.
func NewInstructionFromText(text string) Instruction { return Instruction{text: text} }

// NewInstructionFromProvider creates an Instruction from a dynamic provider.
func NewInstructionFromProvider(p Provider) Instruction { return Instruction{provider: p} }

// NewInstructionFromFunc creates an Instruction from a function.
func NewInstructionFromFunc(f func(*core.RunContext) (string, error)) Instruction {
	return Instruction{provider: Func(f)}
}

// IsStatic returns true if the instruction is backed by a static string.
func (i Instruction) IsStatic() bool { return i.provider == nil }

// Resolve returns the instruction text, invoking the provider if needed.
func (i Instruction) Resolve(ctx *core.RunContext) (string, error) {
	if i.provider != nil {
		return i.provider.Instruction(ctx)
	}
	return i.text, nil
}
