package integration_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFuncToolGuardrails_Integration tests the full guardrail flow at the tool level.
func TestFuncToolGuardrails_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("input guardrail blocks SQL injection", func(t *testing.T) {
		// Create SQL injection detection guardrail
		sqlInjectionGuardrail := guardrail.NewRegexGuardrail(
			"sql_injection",
			regexp.MustCompile(`(?i)(drop\s+table|delete\s+from|;\s*--|union\s+select)`),
			guardrail.WithRegexAction(guardrail.ActionRaise),
		)

		type SearchArgs struct {
			Query string `json:"query"`
		}

		searchTool, err := tool.NewFuncTool(
			"search",
			"Search the database",
			func(ctx context.Context, args SearchArgs) (string, error) {
				return "Results: " + args.Query, nil
			},
			tool.WithInputGuardrails(sqlInjectionGuardrail),
		)
		require.NoError(t, err)

		// Normal query should succeed
		result, err := searchTool.Call(ctx, `{"query": "weather forecast"}`)
		require.NoError(t, err)
		assert.Equal(t, "Results: weather forecast", result)

		// SQL injection should trigger tripwire
		_, err = searchTool.Call(ctx, `{"query": "'; DROP TABLE users; --"}`)
		require.Error(t, err)

		var tripwireErr *guardrail.TripwireError
		require.True(t, errors.As(err, &tripwireErr), "expected TripwireError, got %T", err)
		assert.Equal(t, "search:input", tripwireErr.GuardrailName)
	})

	t.Run("output guardrail blocks sensitive data", func(t *testing.T) {
		// Create content filter for sensitive data
		sensitiveFilter := guardrail.NewContentFilterGuardrail(
			[]string{"password", "secret", "api_key"},
			guardrail.WithContentFilterAction(guardrail.ActionRaise),
		)

		type QueryArgs struct {
			Query string `json:"query"`
		}

		dbTool, err := tool.NewFuncTool(
			"query_db",
			"Query the database",
			func(ctx context.Context, args QueryArgs) (string, error) {
				// Simulate returning sensitive data when querying users
				if args.Query == "users" {
					return "user: admin, password: secret123", nil
				}
				return "No results", nil
			},
			tool.WithOutputGuardrails(sensitiveFilter),
		)
		require.NoError(t, err)

		// Normal query should succeed
		result, err := dbTool.Call(ctx, `{"query": "products"}`)
		require.NoError(t, err)
		assert.Equal(t, "No results", result)

		// Query returning sensitive data should trigger tripwire
		_, err = dbTool.Call(ctx, `{"query": "users"}`)
		require.Error(t, err)

		var tripwireErr *guardrail.TripwireError
		require.True(t, errors.As(err, &tripwireErr), "expected TripwireError, got %T", err)
		assert.Equal(t, "query_db:output", tripwireErr.GuardrailName)
	})

	t.Run("combined input and output guardrails", func(t *testing.T) {
		// Input: block profanity
		profanityFilter := guardrail.NewContentFilterGuardrail(
			[]string{"badword"},
			guardrail.WithContentFilterAction(guardrail.ActionReject),
		)

		// Output: block PII patterns
		piiFilter := guardrail.NewRegexGuardrail(
			"pii_filter",
			regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`), // SSN pattern
			guardrail.WithRegexAction(guardrail.ActionRaise),
		)

		type ProcessArgs struct {
			Text string `json:"text"`
		}

		processTool, err := tool.NewFuncTool(
			"process",
			"Process text",
			func(ctx context.Context, args ProcessArgs) (string, error) {
				// Simulate processing that might return PII
				if args.Text == "get_ssn" {
					return "SSN: 123-45-6789", nil
				}
				return "Processed: " + args.Text, nil
			},
			tool.WithInputGuardrails(profanityFilter),
			tool.WithOutputGuardrails(piiFilter),
		)
		require.NoError(t, err)

		// Normal input/output should succeed
		result, err := processTool.Call(ctx, `{"text": "hello world"}`)
		require.NoError(t, err)
		assert.Equal(t, "Processed: hello world", result)

		// Input with profanity should be rejected
		_, err = processTool.Call(ctx, `{"text": "this contains badword"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "input rejected")

		// Output with PII should trigger tripwire
		_, err = processTool.Call(ctx, `{"text": "get_ssn"}`)
		require.Error(t, err)

		var tripwireErr *guardrail.TripwireError
		require.True(t, errors.As(err, &tripwireErr), "expected TripwireError, got %T", err)
		assert.Equal(t, "process:output", tripwireErr.GuardrailName)
	})
}

// TestGuardrailChain_Integration tests chaining multiple guardrails.
func TestGuardrailChain_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("chain stops at first rejection", func(t *testing.T) {
		filter1 := guardrail.NewContentFilterGuardrail(
			[]string{"blocked1"},
			guardrail.WithContentFilterAction(guardrail.ActionReject),
		)
		filter2 := guardrail.NewContentFilterGuardrail(
			[]string{"blocked2"},
			guardrail.WithContentFilterAction(guardrail.ActionReject),
		)

		// Content with blocked1 should be rejected by first filter
		result, err := guardrail.Chain(ctx, "this contains blocked1", filter1, filter2)
		require.NoError(t, err)
		assert.Equal(t, guardrail.ActionReject, result.Action)
		assert.Contains(t, result.Message, "blocked1")

		// Content with blocked2 should be rejected by second filter
		result, err = guardrail.Chain(ctx, "this contains blocked2", filter1, filter2)
		require.NoError(t, err)
		assert.Equal(t, guardrail.ActionReject, result.Action)
		assert.Contains(t, result.Message, "blocked2")

		// Clean content should be allowed
		result, err = guardrail.Chain(ctx, "this is clean content", filter1, filter2)
		require.NoError(t, err)
		assert.Equal(t, guardrail.ActionAllow, result.Action)
	})

	t.Run("chain stops at first raise", func(t *testing.T) {
		rejectFilter := guardrail.NewContentFilterGuardrail(
			[]string{"reject_me"},
			guardrail.WithContentFilterAction(guardrail.ActionReject),
		)
		raiseFilter := guardrail.NewContentFilterGuardrail(
			[]string{"raise_me"},
			guardrail.WithContentFilterAction(guardrail.ActionRaise),
		)

		// Raise takes precedence when hit first
		result, err := guardrail.Chain(ctx, "this will raise_me", rejectFilter, raiseFilter)
		require.NoError(t, err)
		assert.Equal(t, guardrail.ActionRaise, result.Action)
		assert.True(t, result.IsTripwire())
	})
}

// TestGuardrailActions_Integration tests all three actions (Allow, Reject, Raise).
func TestGuardrailActions_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("ActionAllow proceeds normally", func(t *testing.T) {
		filter := guardrail.NewContentFilterGuardrail([]string{"blocked"})

		result, err := guardrail.Chain(ctx, "clean content", filter)
		require.NoError(t, err)
		assert.True(t, result.IsAllowed())
		assert.False(t, result.IsRejection())
		assert.False(t, result.IsTripwire())
	})

	t.Run("ActionReject is a soft rejection", func(t *testing.T) {
		filter := guardrail.NewContentFilterGuardrail(
			[]string{"blocked"},
			guardrail.WithContentFilterAction(guardrail.ActionReject),
		)

		result, err := guardrail.Chain(ctx, "this is blocked content", filter)
		require.NoError(t, err)
		assert.False(t, result.IsAllowed())
		assert.True(t, result.IsRejection())
		assert.False(t, result.IsTripwire())
	})

	t.Run("ActionRaise triggers tripwire", func(t *testing.T) {
		filter := guardrail.NewContentFilterGuardrail(
			[]string{"dangerous"},
			guardrail.WithContentFilterAction(guardrail.ActionRaise),
		)

		result, err := guardrail.Chain(ctx, "this is dangerous content", filter)
		require.NoError(t, err)
		assert.False(t, result.IsAllowed())
		assert.False(t, result.IsRejection())
		assert.True(t, result.IsTripwire())
	})
}
