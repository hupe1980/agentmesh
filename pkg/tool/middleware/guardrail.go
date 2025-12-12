package middleware

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// GuardrailMiddleware applies guardrails to tool inputs and outputs.
//
// This middleware allows checking tool arguments before execution (input guardrails)
// and checking tool results after execution (output guardrails).
type GuardrailMiddleware struct {
	inputGuardrails  []guardrail.Guardrail[string]
	outputGuardrails []guardrail.Guardrail[string]
}

// GuardrailOptions configures the guardrail middleware.
type GuardrailOptions struct {
	// InputGuardrails are applied to tool arguments before execution.
	InputGuardrails []guardrail.Guardrail[string]

	// OutputGuardrails are applied to tool results after execution.
	OutputGuardrails []guardrail.Guardrail[string]
}

// GuardrailOption is a function that configures the guardrail middleware.
type GuardrailOption func(*GuardrailOptions)

// WithInputGuardrails sets the input guardrails.
func WithInputGuardrails(guardrails ...guardrail.Guardrail[string]) GuardrailOption {
	return func(o *GuardrailOptions) {
		o.InputGuardrails = guardrails
	}
}

// WithOutputGuardrails sets the output guardrails.
func WithOutputGuardrails(guardrails ...guardrail.Guardrail[string]) GuardrailOption {
	return func(o *GuardrailOptions) {
		o.OutputGuardrails = guardrails
	}
}

// NewGuardrailMiddleware creates a new guardrail middleware.
func NewGuardrailMiddleware(opts ...GuardrailOption) *GuardrailMiddleware {
	options := &GuardrailOptions{}

	for _, opt := range opts {
		opt(options)
	}

	return &GuardrailMiddleware{
		inputGuardrails:  options.InputGuardrails,
		outputGuardrails: options.OutputGuardrails,
	}
}

// Wrap wraps the tool executor with guardrail checks.
//
//nolint:nestif // Nested conditionals are inherent to the guardrail checking pattern
func (m *GuardrailMiddleware) Wrap(next tool.Executor) tool.Executor {
	return tool.WrapFunc(func(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
		// Check input guardrails on tool arguments
		if len(m.inputGuardrails) > 0 {
			for i, call := range calls {
				result, err := guardrail.Chain(ctx, call.Arguments, m.inputGuardrails...)
				if err != nil {
					return nil, fmt.Errorf("input guardrail error for tool %s: %w", call.Name, err)
				}

				guardrailName := fmt.Sprintf("tool-input[%d]:%s", i, call.Name)

				if result.IsTripwire() {
					return nil, guardrail.NewTripwireError(guardrailName, result)
				}

				if !result.IsAllowed() {
					// Return rejection as an error in the result
					return []tool.ExecutionResult{{
						ToolCallID: call.ID,
						ToolName:   call.Name,
						Error:      guardrail.NewRejection(guardrailName, result),
					}}, nil
				}
			}
		}

		// Execute the tool
		results, err := next.Execute(ctx, calls)
		if err != nil {
			return results, err
		}

		// Check output guardrails on tool results
		if len(m.outputGuardrails) > 0 {
			for i := range results {
				// Skip if there was an execution error
				if results[i].Error != nil {
					continue
				}

				// Convert result to string for guardrail check
				resultStr := fmt.Sprintf("%v", results[i].Result)

				result, err := guardrail.Chain(ctx, resultStr, m.outputGuardrails...)
				if err != nil {
					results[i].Error = fmt.Errorf("output guardrail error: %w", err)
					results[i].Result = nil
					continue
				}

				guardrailName := fmt.Sprintf("tool-output[%d]:%s", i, results[i].ToolName)

				if result.IsTripwire() {
					return nil, guardrail.NewTripwireError(guardrailName, result)
				}

				if !result.IsAllowed() {
					// Replace result with rejection
					results[i].Error = guardrail.NewRejection(guardrailName, result)
					results[i].Result = nil
				}
			}
		}

		return results, nil
	})
}
