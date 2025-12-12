package middleware

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// GuardrailMiddleware applies guardrails to model inputs and outputs.
//
// This middleware allows checking prompts before they are sent to the model
// (input guardrails) and checking model responses before they are returned
// (output guardrails).
type GuardrailMiddleware struct {
	inputGuardrails  []guardrail.Guardrail[string]
	outputGuardrails []guardrail.Guardrail[string]
	inputParallel    bool // If true, input guardrails run concurrently with model execution
}

// GuardrailOptions configures the guardrail middleware.
type GuardrailOptions struct {
	// InputGuardrails are applied to user messages before model execution.
	InputGuardrails []guardrail.Guardrail[string]

	// OutputGuardrails are applied to model responses after execution.
	OutputGuardrails []guardrail.Guardrail[string]

	// InputParallel controls whether input guardrails run concurrently with model execution.
	// When true: guardrails run in parallel with model - better latency but model may
	// consume tokens before guardrail completes.
	// When false (default): guardrails complete before model starts - prevents token consumption.
	InputParallel bool
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

// WithInputParallel sets whether input guardrails run in parallel with model execution.
// When true, guardrails run concurrently with the model - better latency but model may
// consume tokens before guardrail completes.
// When false (default), guardrails complete before model starts - prevents token consumption.
func WithInputParallel(parallel bool) GuardrailOption {
	return func(o *GuardrailOptions) {
		o.InputParallel = parallel
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
		inputParallel:    options.InputParallel,
	}
}

// Wrap wraps the model executor with guardrail checks.
//
//nolint:gocyclo,nestif // Complexity is inherent to the iterator-based middleware pattern with parallel execution
func (m *GuardrailMiddleware) Wrap(next model.Executor) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			// Collect input content for guardrail checking
			var inputContent string
			for _, msg := range req.Messages {
				if msg.Type() == message.TypeHuman {
					inputContent += msg.String() + "\n"
				}
			}

			if len(m.inputGuardrails) == 0 || inputContent == "" {
				// No input guardrails or no human messages, just check outputs
				m.executeWithOutputGuardrails(ctx, next, req, yield)
				return
			}

			if m.inputParallel {
				// Parallel mode: run input guardrails concurrently with model
				guardrailDone := make(chan struct {
					result *guardrail.Result
					err    error
				}, 1)

				go func() {
					result, err := guardrail.Chain(ctx, inputContent, m.inputGuardrails...)
					guardrailDone <- struct {
						result *guardrail.Result
						err    error
					}{result, err}
				}()

				// Start model execution immediately
				for resp, err := range next.Generate(ctx, req) {
					// Check if guardrail has completed with a violation
					select {
					case gr := <-guardrailDone:
						if gr.err != nil {
							yield(nil, gr.err)
							return
						}

						if gr.result.IsTripwire() {
							yield(nil, guardrail.NewTripwireError("model-input", gr.result))
							return
						}

						if !gr.result.IsAllowed() {
							yield(nil, guardrail.NewRejection("model-input", gr.result))
							return
						}
						// Guardrail passed, continue with response
						guardrailDone = nil // Don't check again
					default:
						// Guardrail still running, continue with model output
					}

					if err != nil {
						if !yield(resp, err) {
							return
						}
						continue
					}

					// Check output guardrails on non-partial responses
					if len(m.outputGuardrails) > 0 && !resp.Partial {
						if !m.checkOutputGuardrails(ctx, resp, yield) {
							return
						}
					}

					if !yield(resp, nil) {
						return
					}
				}

				// Wait for guardrail to complete if it hasn't yet
				if guardrailDone != nil {
					gr := <-guardrailDone
					if gr.err != nil {
						yield(nil, gr.err)
						return
					}

					if gr.result.IsTripwire() {
						yield(nil, guardrail.NewTripwireError("model-input", gr.result))
						return
					}

					if !gr.result.IsAllowed() {
						yield(nil, guardrail.NewRejection("model-input", gr.result))
						return
					}
				}
			} else {
				// Blocking mode: check input guardrails before model execution
				result, err := guardrail.Chain(ctx, inputContent, m.inputGuardrails...)
				if err != nil {
					yield(nil, err)
					return
				}

				if result.IsTripwire() {
					yield(nil, guardrail.NewTripwireError("model-input", result))
					return
				}

				if !result.IsAllowed() {
					yield(nil, guardrail.NewRejection("model-input", result))
					return
				}

				// Input passed, execute with output guardrails
				m.executeWithOutputGuardrails(ctx, next, req, yield)
			}
		}
	})
}

// executeWithOutputGuardrails runs the model and checks output guardrails.
func (m *GuardrailMiddleware) executeWithOutputGuardrails(
	ctx context.Context,
	next model.Executor,
	req *model.Request,
	yield func(*model.Response, error) bool,
) {
	for resp, err := range next.Generate(ctx, req) {
		if err != nil {
			if !yield(resp, err) {
				return
			}
			continue
		}

		// Check output guardrails on non-partial responses
		if len(m.outputGuardrails) > 0 && !resp.Partial {
			if !m.checkOutputGuardrails(ctx, resp, yield) {
				return
			}
		}

		if !yield(resp, nil) {
			return
		}
	}
}

// checkOutputGuardrails validates a response against output guardrails.
// Returns true if allowed, false if rejected (and yields the error).
func (m *GuardrailMiddleware) checkOutputGuardrails(
	ctx context.Context,
	resp *model.Response,
	yield func(*model.Response, error) bool,
) bool {
	content := resp.Message.String()

	result, err := guardrail.Chain(ctx, content, m.outputGuardrails...)
	if err != nil {
		yield(nil, err)
		return false
	}

	if result.IsTripwire() {
		yield(nil, guardrail.NewTripwireError("model-output", result))
		return false
	}

	if !result.IsAllowed() {
		yield(nil, guardrail.NewRejection("model-output", result))
		return false
	}

	return true
}
