package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// mockToolExecutor creates a simple mock tool executor for testing.
func mockToolExecutor(result any) tool.Executor {
	return tool.WrapFunc(func(_ context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
		results := make([]tool.ExecutionResult, len(calls))
		for i, call := range calls {
			results[i] = tool.ExecutionResult{
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Result:     result,
			}
		}
		return results, nil
	})
}

// blockingGuardrail creates a guardrail that rejects content containing the specified keyword.
func blockingGuardrail(keyword string) guardrail.Guardrail[string] {
	return guardrail.NewContentFilterGuardrail(
		[]string{keyword},
		guardrail.WithContentFilterAction(guardrail.ActionReject),
	)
}

// tripwireGuardrail creates a guardrail that raises tripwire on matching content.
func tripwireGuardrail(keyword string) guardrail.Guardrail[string] {
	return guardrail.NewContentFilterGuardrail(
		[]string{keyword},
		guardrail.WithContentFilterAction(guardrail.ActionRaise),
	)
}

// allowAllGuardrail creates a guardrail that allows everything.
func allowAllGuardrail() guardrail.Guardrail[string] {
	return guardrail.NewFunc("allow-all", func(_ context.Context, _ string) (*guardrail.Result, error) {
		return guardrail.Allow(), nil
	})
}

func TestGuardrailMiddleware_InputBlocking(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(blockingGuardrail("forbidden")),
	)

	exec := mw.Wrap(mockToolExecutor("ok"))
	calls := []tool.Call{
		{ID: "1", Name: "test_tool", Arguments: "{\"input\": \"this is forbidden content\"}"},
	}

	results, err := exec.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 1)

	var rejection *guardrail.Rejection
	assert.True(t, errors.As(results[0].Error, &rejection))
}

func TestGuardrailMiddleware_InputAllowsValid(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(blockingGuardrail("forbidden")),
	)

	exec := mw.Wrap(mockToolExecutor("success"))
	calls := []tool.Call{
		{ID: "1", Name: "test_tool", Arguments: "{\"input\": \"valid content\"}"},
	}

	results, err := exec.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Nil(t, results[0].Error)
	assert.Equal(t, "success", results[0].Result)
}

func TestGuardrailMiddleware_InputTripwire(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(tripwireGuardrail("dangerous")),
	)

	exec := mw.Wrap(mockToolExecutor("ok"))
	calls := []tool.Call{
		{ID: "1", Name: "test_tool", Arguments: "{\"input\": \"dangerous content\"}"},
	}

	_, err := exec.Execute(context.Background(), calls)
	require.Error(t, err)

	var tripwireErr *guardrail.TripwireError
	assert.True(t, errors.As(err, &tripwireErr))
}

func TestGuardrailMiddleware_OutputBlocking(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithOutputGuardrails(blockingGuardrail("secret")),
	)

	exec := mw.Wrap(mockToolExecutor("this contains secret data"))
	calls := []tool.Call{
		{ID: "1", Name: "test_tool", Arguments: "{}"},
	}

	results, err := exec.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 1)

	var rejection *guardrail.Rejection
	assert.True(t, errors.As(results[0].Error, &rejection))
	assert.Nil(t, results[0].Result)
}

func TestGuardrailMiddleware_OutputAllowsValid(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithOutputGuardrails(blockingGuardrail("secret")),
	)

	exec := mw.Wrap(mockToolExecutor("normal result"))
	calls := []tool.Call{
		{ID: "1", Name: "test_tool", Arguments: "{}"},
	}

	results, err := exec.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Nil(t, results[0].Error)
	assert.Equal(t, "normal result", results[0].Result)
}

func TestGuardrailMiddleware_OutputTripwire(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithOutputGuardrails(tripwireGuardrail("dangerous")),
	)

	exec := mw.Wrap(mockToolExecutor("dangerous output"))
	calls := []tool.Call{
		{ID: "1", Name: "test_tool", Arguments: "{}"},
	}

	_, err := exec.Execute(context.Background(), calls)
	require.Error(t, err)

	var tripwireErr *guardrail.TripwireError
	assert.True(t, errors.As(err, &tripwireErr))
}

func TestGuardrailMiddleware_NoGuardrails(t *testing.T) {
	mw := NewGuardrailMiddleware()

	exec := mw.Wrap(mockToolExecutor("hello"))
	calls := []tool.Call{
		{ID: "1", Name: "test_tool", Arguments: "{}"},
	}

	results, err := exec.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "hello", results[0].Result)
}

func TestGuardrailMiddleware_MultipleGuardrails(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(
			allowAllGuardrail(),
			blockingGuardrail("forbidden"),
		),
	)

	exec := mw.Wrap(mockToolExecutor("ok"))
	calls := []tool.Call{
		{ID: "1", Name: "test_tool", Arguments: "{\"input\": \"forbidden\"}"},
	}

	results, err := exec.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 1)

	var rejection *guardrail.Rejection
	assert.True(t, errors.As(results[0].Error, &rejection))
}

func TestGuardrailMiddleware_InputAndOutputGuardrails(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(blockingGuardrail("input-bad")),
		WithOutputGuardrails(blockingGuardrail("output-bad")),
	)

	// Test input blocked
	exec := mw.Wrap(mockToolExecutor("ok"))
	calls := []tool.Call{
		{ID: "1", Name: "test_tool", Arguments: "{\"input\": \"input-bad\"}"},
	}
	results, err := exec.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 1)

	var rejection *guardrail.Rejection
	assert.True(t, errors.As(results[0].Error, &rejection))

	// Test output blocked
	exec2 := mw.Wrap(mockToolExecutor("output-bad"))
	calls2 := []tool.Call{
		{ID: "1", Name: "test_tool", Arguments: "{\"input\": \"valid\"}"},
	}
	results2, err2 := exec2.Execute(context.Background(), calls2)
	require.NoError(t, err2)
	require.Len(t, results2, 1)

	var rejection2 *guardrail.Rejection
	assert.True(t, errors.As(results2[0].Error, &rejection2))
}

func TestGuardrailMiddleware_MultipleToolCalls(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(blockingGuardrail("blocked")),
	)

	exec := mw.Wrap(mockToolExecutor("result"))
	calls := []tool.Call{
		{ID: "1", Name: "tool1", Arguments: "{\"input\": \"valid1\"}"},
		{ID: "2", Name: "tool2", Arguments: "{\"input\": \"blocked content\"}"},
		{ID: "3", Name: "tool3", Arguments: "{\"input\": \"valid2\"}"},
	}

	// The middleware should reject on the second call
	results, err := exec.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 1)

	var rejection *guardrail.Rejection
	assert.True(t, errors.As(results[0].Error, &rejection))
}
