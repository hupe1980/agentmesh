package middleware

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/hupe1980/agentmesh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaValidationMiddleware_PassThrough(t *testing.T) {
	t.Run("no schema - passes through", func(t *testing.T) {
		mdl := testutil.NewModelBuilder().
			WithResponse("Hello world").
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Hi"),
			},
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)
		assert.Equal(t, "Hello world", responses[0].Message.String())
	})

	t.Run("validation disabled - passes through", func(t *testing.T) {
		mdl := testutil.NewModelBuilder().
			WithResponse(`{"invalid": true}`).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("test", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationDisabled()))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Generate"),
			},
			OutputSchema: &outputSchema,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)
		assert.Equal(t, `{"invalid": true}`, responses[0].Message.String())
	})

	t.Run("tool calls - skips validation", func(t *testing.T) {
		mdl := testutil.NewModelBuilder().
			WithToolCalls(message.ToolCall{ID: "call_1", Name: "search", Arguments: `{"query": "test"}`}).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("test", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationStrict()))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Search"),
			},
			OutputSchema: &outputSchema,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)
		assert.True(t, message.HasToolCalls(responses[0].Message))
	})
}

func TestSchemaValidationMiddleware_ValidOutput(t *testing.T) {
	t.Run("valid output passes", func(t *testing.T) {
		mdl := testutil.NewModelBuilder().
			WithResponse(`{"name": "John", "age": 30}`).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("person", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "number"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationStrict()))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Generate person"),
			},
			OutputSchema: &outputSchema,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)
		assert.Contains(t, responses[0].Message.String(), "John")
	})
}

func TestSchemaValidationMiddleware_StrictMode(t *testing.T) {
	t.Run("invalid output fails in strict mode", func(t *testing.T) {
		mdl := testutil.NewModelBuilder().
			WithResponse(`{"invalid": true}`).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("person", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationStrict()))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Generate person"),
			},
			OutputSchema: &outputSchema,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Len(t, errs, 1)
		require.Empty(t, responses)

		var schemaErr *SchemaValidationError
		require.ErrorAs(t, errs[0], &schemaErr)
		assert.Equal(t, 1, schemaErr.Attempts)
		assert.NotEmpty(t, schemaErr.Errors)
	})
}

func TestSchemaValidationMiddleware_RetryMode(t *testing.T) {
	t.Run("retries and succeeds", func(t *testing.T) {
		// First response is invalid, second is valid
		mdl := testutil.NewModelBuilder().
			WithResponses(
				`{"invalid": true}`,
				`{"name": "John"}`,
			).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("person", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationWithRetry(2)))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Generate person"),
			},
			OutputSchema: &outputSchema,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)
		assert.Contains(t, responses[0].Message.String(), "John")
	})

	t.Run("fails after max retries", func(t *testing.T) {
		// All responses are invalid
		mdl := testutil.NewModelBuilder().
			WithResponses(
				`{"invalid": 1}`,
				`{"invalid": 2}`,
				`{"invalid": 3}`,
			).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("person", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationWithRetry(2)))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Generate person"),
			},
			OutputSchema: &outputSchema,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Len(t, errs, 1)
		require.Empty(t, responses)

		var schemaErr *SchemaValidationError
		require.ErrorAs(t, errs[0], &schemaErr)
		assert.Equal(t, 3, schemaErr.Attempts) // 1 initial + 2 retries
	})
}

func TestSchemaValidationMiddleware_WarnMode(t *testing.T) {
	t.Run("warns but returns invalid output", func(t *testing.T) {
		mdl := testutil.NewModelBuilder().
			WithResponse(`{"invalid": true}`).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("person", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationWarnOnly()))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Generate person"),
			},
			OutputSchema: &outputSchema,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)
		assert.Equal(t, `{"invalid": true}`, responses[0].Message.String())
	})
}

func TestSchemaValidationMiddleware_IgnoreMode(t *testing.T) {
	t.Run("ignores errors and returns invalid output", func(t *testing.T) {
		mdl := testutil.NewModelBuilder().
			WithResponse(`{"invalid": true}`).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("person", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationPolicy{
			Enabled:   true,
			OnFailure: schema.IgnoreOnError,
		}))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Generate person"),
			},
			OutputSchema: &outputSchema,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)
		assert.Equal(t, `{"invalid": true}`, responses[0].Message.String())
	})
}

func TestSchemaValidationError(t *testing.T) {
	err := &SchemaValidationError{
		Errors: []schema.ValidationError{
			{Path: "$.name", Message: "required field missing"},
		},
		Attempts: 3,
	}

	assert.Contains(t, err.Error(), "3 attempt(s)")
	assert.Contains(t, err.Error(), "1 error(s)")
}

func TestSchemaValidationMiddleware_Streaming(t *testing.T) {
	t.Run("streaming mode - yields partial responses and validates final", func(t *testing.T) {
		mdl := testutil.NewModelBuilder().
			WithStreamingResponse(
				`{"na`,
				`me": `,
				`"John"}`,
			).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("person", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationStrict()))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Generate person"),
			},
			OutputSchema: &outputSchema,
			Stream:       true,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Empty(t, errs)
		require.Len(t, responses, 3) // 2 partial + 1 final

		// First 2 should be partial
		for i := 0; i < 2; i++ {
			assert.True(t, responses[i].Partial, "response %d should be partial", i)
		}

		// Last should be complete and valid
		assert.False(t, responses[2].Partial)
		assert.Equal(t, `{"name": "John"}`, responses[2].Message.String())
	})

	t.Run("streaming mode - invalid final response fails strict", func(t *testing.T) {
		mdl := testutil.NewModelBuilder().
			WithStreamingResponse(
				`{"in`,
				`valid": `,
				`true}`,
			).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("person", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationStrict()))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Generate person"),
			},
			OutputSchema: &outputSchema,
			Stream:       true,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Len(t, errs, 1)

		var schemaErr *SchemaValidationError
		require.ErrorAs(t, errs[0], &schemaErr)

		// Should have received 2 partial responses before error (3 chunks = 2 partial + 1 final that failed)
		assert.GreaterOrEqual(t, len(responses), 2)
	})

	t.Run("streaming mode - retry with partial responses", func(t *testing.T) {
		// First attempt: invalid streaming response
		// Second attempt: valid streaming response
		mdl := testutil.NewModelBuilder().
			WithStreamingResponses(
				[]string{`{"in`, `valid": `, `1}`},   // First attempt (invalid)
				[]string{`{"na`, `me": "`, `John"}`}, // Second attempt (valid)
			).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("person", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationWithRetry(1)))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Generate person"),
			},
			OutputSchema: &outputSchema,
			Stream:       true,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Empty(t, errs)

		// Should get partial responses from second (successful) attempt only
		// First attempt's partials are consumed but not yielded
		assert.GreaterOrEqual(t, len(responses), 3)
		assert.False(t, responses[len(responses)-1].Partial)
		assert.Contains(t, responses[len(responses)-1].Message.String(), "John")
	})
}

func TestSchemaValidationMiddleware_PartialResponse(t *testing.T) {
	t.Run("only partial response - returns error (no final response)", func(t *testing.T) {
		mdl := testutil.NewModelBuilder().
			WithPartialResponse(`{"incomplete`).
			Build()

		executor := model.NewExecutor(mdl)
		wrapped := NewSchemaValidationMiddleware().Wrap(executor)

		outputSchema, err := schema.NewOutputSchema("person", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		}, schema.WithValidationPolicy(schema.ValidationStrict()))
		require.NoError(t, err)

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("Generate person"),
			},
			OutputSchema: &outputSchema,
		}

		responses, errs := collect(context.Background(), wrapped, req)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "no final response")
		// Partial response should still be yielded
		require.Len(t, responses, 1)
		assert.True(t, responses[0].Partial)
	})
}
