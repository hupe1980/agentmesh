package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/prompt"
)

// InstructionsProvider supplies dynamic instruction text at runtime.
// Implementations can derive instructions from session state, configuration, or environment.
type InstructionsProvider interface {
	Instructions(ctx context.Context, view graph.View) (string, error)
}

// InstructionsProviderFunc is a functional adapter for InstructionsProvider.
type InstructionsProviderFunc func(ctx context.Context, view graph.View) (string, error)

// Instructions implements InstructionsProvider.
func (f InstructionsProviderFunc) Instructions(ctx context.Context, view graph.View) (string, error) {
	return f(ctx, view)
}

// Instructions represents either a static instruction string or a dynamic provider.
// Supports Go text/template syntax via pkg/prompt for placeholder substitution.
type Instructions struct {
	template *prompt.Template     // Compiled template (nil for provider-based)
	provider InstructionsProvider // Dynamic provider (nil for template-based)
}

// NewInstructions creates Instructions from a template string.
// Uses Go text/template syntax with helper functions from pkg/prompt:
//   - {{.keyName}} - substitute from graph state
//   - {{default "fallback" .Value}} - use fallback if nil/empty
//   - {{.Name | upper}} - convert to uppercase
//   - {{.Name | lower}} - convert to lowercase
//   - {{if .Condition}}...{{end}} - conditionals
//
// Example:
//
//	NewInstructions("You are helping {{.userName}}. Task: {{default \"general\" .task}}")
func NewInstructions(templateStr string) Instructions {
	return Instructions{template: prompt.New(templateStr)}
}

// NewInstructionsFromProvider creates Instructions from a dynamic provider.
func NewInstructionsFromProvider(p InstructionsProvider) Instructions {
	return Instructions{provider: p}
}

// NewInstructionsFromFunc creates Instructions from a function.
func NewInstructionsFromFunc(f func(context.Context, graph.View) (string, error)) Instructions {
	return Instructions{provider: InstructionsProviderFunc(f)}
}

// IsStatic returns true if backed by a template (not a dynamic provider).
func (i Instructions) IsStatic() bool {
	return i.provider == nil
}

// Resolve returns the instruction text, invoking the provider if dynamic,
// or rendering the template with state values if static.
func (i Instructions) Resolve(ctx context.Context, view graph.View) (string, error) {
	if i.provider != nil {
		return i.provider.Instructions(ctx, view)
	}

	if i.template == nil {
		return "", nil
	}

	// Fast path: template has no placeholders
	if !i.template.HasPlaceholders() {
		return i.template.Render(nil)
	}

	// Build template data from graph state only when needed
	data := view.ToMap()
	return i.template.Render(data)
}
