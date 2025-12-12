package middleware

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// mockExecutor creates a simple mock model executor for testing.
func mockExecutor(response string, delay time.Duration) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			if delay > 0 {
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return
				}
			}
			yield(&model.Response{
				Message: message.NewAIMessageFromText(response),
			}, nil)
		}
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

// slowGuardrail creates a guardrail with a delay to simulate processing time.
func slowGuardrail(delay time.Duration) guardrail.Guardrail[string] {
	return guardrail.NewFunc("slow-guardrail", func(ctx context.Context, _ string) (*guardrail.Result, error) {
		select {
		case <-time.After(delay):
			return guardrail.Allow(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
}

// collect gathers all responses from an executor into a slice.
func collect(ctx context.Context, exec model.Executor, req *model.Request) ([]*model.Response, []error) {
	var responses []*model.Response
	var errs []error

	for resp, err := range exec.Generate(ctx, req) {
		if err != nil {
			errs = append(errs, err)
		}
		if resp != nil {
			responses = append(responses, resp)
		}
	}

	return responses, errs
}

func TestGuardrailMiddleware_InputBlocking(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(blockingGuardrail("forbidden")),
	)

	exec := mw.Wrap(mockExecutor("ok", 0))
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("this is forbidden content"),
		},
	}

	_, errs := collect(context.Background(), exec, req)
	require.Len(t, errs, 1)

	var rejection *guardrail.Rejection
	assert.True(t, errors.As(errs[0], &rejection))
}

func TestGuardrailMiddleware_InputAllowsValid(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(blockingGuardrail("forbidden")),
	)

	exec := mw.Wrap(mockExecutor("hello", 0))
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("this is valid content"),
		},
	}

	responses, errs := collect(context.Background(), exec, req)
	require.Empty(t, errs)
	require.Len(t, responses, 1)
	assert.Contains(t, responses[0].Message.String(), "hello")
}

func TestGuardrailMiddleware_InputTripwire(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(tripwireGuardrail("dangerous")),
	)

	exec := mw.Wrap(mockExecutor("ok", 0))
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("this is dangerous content"),
		},
	}

	_, errs := collect(context.Background(), exec, req)
	require.Len(t, errs, 1)

	var tripwireErr *guardrail.TripwireError
	assert.True(t, errors.As(errs[0], &tripwireErr))
}

func TestGuardrailMiddleware_OutputBlocking(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithOutputGuardrails(blockingGuardrail("secret")),
	)

	exec := mw.Wrap(mockExecutor("this contains secret data", 0))
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("tell me a secret"),
		},
	}

	_, errs := collect(context.Background(), exec, req)
	require.Len(t, errs, 1)

	var rejection *guardrail.Rejection
	assert.True(t, errors.As(errs[0], &rejection))
}

func TestGuardrailMiddleware_OutputAllowsValid(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithOutputGuardrails(blockingGuardrail("secret")),
	)

	exec := mw.Wrap(mockExecutor("this is a normal response", 0))
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("hello"),
		},
	}

	responses, errs := collect(context.Background(), exec, req)
	require.Empty(t, errs)
	require.Len(t, responses, 1)
	assert.Contains(t, responses[0].Message.String(), "normal response")
}

func TestGuardrailMiddleware_OutputTripwire(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithOutputGuardrails(tripwireGuardrail("dangerous")),
	)

	exec := mw.Wrap(mockExecutor("this is dangerous output", 0))
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("hello"),
		},
	}

	_, errs := collect(context.Background(), exec, req)
	require.Len(t, errs, 1)

	var tripwireErr *guardrail.TripwireError
	assert.True(t, errors.As(errs[0], &tripwireErr))
}

func TestGuardrailMiddleware_ParallelMode_Allows(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(slowGuardrail(50*time.Millisecond)),
		WithInputParallel(true),
	)

	exec := mw.Wrap(mockExecutor("ok", 100*time.Millisecond))
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("valid input"),
		},
	}

	responses, errs := collect(context.Background(), exec, req)
	require.Empty(t, errs)
	require.Len(t, responses, 1)
	assert.Contains(t, responses[0].Message.String(), "ok")
}

func TestGuardrailMiddleware_ParallelMode_BlocksDuringStream(t *testing.T) {
	// Guardrail takes 50ms, model takes 10ms per response
	// Guardrail should block after model has started
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(blockingGuardrail("blocked")),
		WithInputParallel(true),
	)

	exec := mw.Wrap(mockExecutor("response", 10*time.Millisecond))
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("this is blocked content"),
		},
	}

	_, errs := collect(context.Background(), exec, req)
	require.Len(t, errs, 1)

	var rejection *guardrail.Rejection
	assert.True(t, errors.As(errs[0], &rejection))
}

func TestGuardrailMiddleware_NoGuardrails(t *testing.T) {
	mw := NewGuardrailMiddleware()

	exec := mw.Wrap(mockExecutor("hello", 0))
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("any input"),
		},
	}

	responses, errs := collect(context.Background(), exec, req)
	require.Empty(t, errs)
	require.Len(t, responses, 1)
	assert.Contains(t, responses[0].Message.String(), "hello")
}

func TestGuardrailMiddleware_NoHumanMessages(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(blockingGuardrail("blocked")),
	)

	exec := mw.Wrap(mockExecutor("ok", 0))
	req := &model.Request{
		Messages: []message.Message{
			message.NewAIMessageFromText("AI message only"),
		},
	}

	responses, errs := collect(context.Background(), exec, req)
	require.Empty(t, errs)
	require.Len(t, responses, 1)
	assert.Contains(t, responses[0].Message.String(), "ok")
}

func TestGuardrailMiddleware_MultipleGuardrails(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(
			allowAllGuardrail(),
			blockingGuardrail("forbidden"),
		),
	)

	exec := mw.Wrap(mockExecutor("ok", 0))
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("this is forbidden"),
		},
	}

	_, errs := collect(context.Background(), exec, req)
	require.Len(t, errs, 1)

	var rejection *guardrail.Rejection
	assert.True(t, errors.As(errs[0], &rejection))
}

func TestGuardrailMiddleware_InputAndOutputGuardrails(t *testing.T) {
	mw := NewGuardrailMiddleware(
		WithInputGuardrails(blockingGuardrail("input-bad")),
		WithOutputGuardrails(blockingGuardrail("output-bad")),
	)

	// Test input blocked
	exec := mw.Wrap(mockExecutor("ok", 0))
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("input-bad"),
		},
	}
	_, errs := collect(context.Background(), exec, req)
	require.Len(t, errs, 1)

	var rejection *guardrail.Rejection
	assert.True(t, errors.As(errs[0], &rejection))

	// Test output blocked
	exec2 := mw.Wrap(mockExecutor("output-bad", 0))
	req2 := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("valid input"),
		},
	}
	_, errs2 := collect(context.Background(), exec2, req2)
	require.Len(t, errs2, 1)

	var rejection2 *guardrail.Rejection
	assert.True(t, errors.As(errs2[0], &rejection2))
}
