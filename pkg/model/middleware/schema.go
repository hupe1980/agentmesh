package middleware

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
)

// SchemaValidationError is returned when schema validation fails after all retries.
type SchemaValidationError struct {
	// Errors contains the validation errors from the last attempt.
	Errors []schema.ValidationError

	// Attempts is the total number of attempts made.
	Attempts int
}

// Error implements the error interface.
func (e *SchemaValidationError) Error() string {
	return fmt.Sprintf("schema validation failed after %d attempt(s): %d error(s)", e.Attempts, len(e.Errors))
}

// SchemaValidationMiddleware validates model output against the request's OutputSchema.
// It reads the ValidationPolicy from OutputSchema to determine retry behavior and failure handling.
type SchemaValidationMiddleware struct{}

// NewSchemaValidationMiddleware creates a new schema validation middleware.
func NewSchemaValidationMiddleware() *SchemaValidationMiddleware {
	return &SchemaValidationMiddleware{}
}

// Wrap wraps the model executor with schema validation logic.
//
//nolint:gocyclo // Validation retry logic with multiple exit paths; refactoring would obscure flow
func (m *SchemaValidationMiddleware) Wrap(next model.Executor) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			// Pass through if no schema or validation disabled
			if req.OutputSchema == nil || req.OutputSchema.Validation == nil || !req.OutputSchema.Validation.Enabled {
				for resp, err := range next.Generate(ctx, req) {
					if !yield(resp, err) {
						return
					}
				}

				return
			}

			policy := req.OutputSchema.Validation
			logger := logging.FromContext(ctx)

			// Use custom validator if provided, otherwise create default validator
			var validator schema.Validator
			if policy.Validator != nil {
				validator = policy.Validator
			} else {
				validator = schema.NewValidator()
			}

			currentReq := req
			var lastErrors []schema.ValidationError

			for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
				// Collect responses, yielding partials immediately for streaming
				var partialResponses []*model.Response
				var finalResponse *model.Response
				var genErr error

				for resp, err := range next.Generate(ctx, currentReq) {
					if err != nil {
						genErr = err
						break
					}

					if resp == nil {
						continue
					}

					// Yield partial responses immediately for streaming
					if resp.Partial {
						partialResponses = append(partialResponses, resp)
						if !yield(resp, nil) {
							return
						}
						continue
					}

					finalResponse = resp
				}

				// Handle generation error
				if genErr != nil {
					yield(nil, genErr)
					return
				}

				// Should have a final response
				if finalResponse == nil {
					yield(nil, fmt.Errorf("no final response from model"))
					return
				}

				// Skip validation for tool calls - they don't have schema output
				if message.HasToolCalls(finalResponse.Message) {
					yield(finalResponse, nil)
					return
				}

				// Validate against schema
				result, err := validator.Validate(ctx, req.OutputSchema.Schema, finalResponse.Message.String())
				if err != nil {
					yield(nil, fmt.Errorf("schema validation error: %w", err))
					return
				}

				// Success - yield valid response
				if result.Valid {
					yield(finalResponse, nil)
					return
				}

				// Failed validation - prepare retry with error feedback
				lastErrors = result.Errors

				if attempt < policy.MaxRetries {
					logger.Debug("schema validation failed, retrying",
						"attempt", attempt+1,
						"max_retries", policy.MaxRetries,
						"errors", len(lastErrors),
					)

					// Build feedback message for the model
					feedback := buildErrorFeedback(finalResponse.Message.String(), result.Errors)

					// Create new request with error feedback appended
					currentReq = appendFeedback(currentReq, finalResponse.Message, feedback)
				} else {
					// All retries exhausted - handle based on OnFailure policy
					switch policy.OnFailure {
					case schema.WarnOnError:
						logger.Warn("schema validation failed",
							"errors", len(lastErrors),
							"attempts", attempt+1,
						)
						yield(finalResponse, nil) // Return invalid response with warning

						return

					case schema.IgnoreOnError:
						yield(finalResponse, nil) // Return invalid response silently

						return

					default: // FailOnError
						yield(nil, &SchemaValidationError{
							Errors:   lastErrors,
							Attempts: attempt + 1,
						})

						return
					}
				}
			}
		}
	})
}

// buildErrorFeedback creates a helpful message for the model to fix its output.
func buildErrorFeedback(output string, errors []schema.ValidationError) string {
	var sb strings.Builder

	sb.WriteString("Your previous response had schema validation errors. Please fix and try again.\n\n")
	sb.WriteString("Your output:\n```json\n")
	sb.WriteString(output)
	sb.WriteString("\n```\n\n")
	sb.WriteString("Validation errors:\n")

	for _, err := range errors {
		fmt.Fprintf(&sb, "- Path '%s': %s", err.Path, err.Message)

		if err.Expected != "" {
			fmt.Fprintf(&sb, " (expected: %s, got: %s)", err.Expected, err.Actual)
		}

		sb.WriteString("\n")
	}

	sb.WriteString("\nPlease provide a corrected JSON response that matches the schema.")

	return sb.String()
}

// appendFeedback adds the model's response and error feedback to the conversation.
func appendFeedback(req *model.Request, modelResp message.Message, feedback string) *model.Request {
	newMessages := make([]message.Message, len(req.Messages)+2)
	copy(newMessages, req.Messages)

	// Add model's invalid response
	newMessages[len(req.Messages)] = modelResp

	// Add human feedback with errors
	newMessages[len(req.Messages)+1] = message.NewHumanMessageFromText(feedback)

	return &model.Request{
		Messages:     newMessages,
		OutputSchema: req.OutputSchema,
		Tools:        req.Tools,
		Instructions: req.Instructions,
		Stream:       req.Stream,
		Metadata:     req.Metadata,
	}
}
