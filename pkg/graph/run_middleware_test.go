package graph

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunMiddleware(t *testing.T) {
	t.Run("intercepts input before execution", func(t *testing.T) {
		messagesKey := NewListKey[string]("messages")

		var interceptedInput any

		inputMiddleware := func(next RunFunc[any, any]) RunFunc[any, any] {
			return func(ctx context.Context, input any) iter.Seq2[any, error] {
				interceptedInput = input
				return next(ctx, input)
			}
		}

		g, err := New[any, any](messagesKey).
			Node("echo", func(ctx context.Context, scope Scope[any]) (*Command, error) {
				return Set(messagesKey, []string{"response"}).End()
			}, END).
			Start("echo").
			WithRunMiddleware(inputMiddleware).
			Build()
		require.NoError(t, err)

		var outputs []any
		for output, err := range g.Run(context.Background(), nil) {
			require.NoError(t, err)
			outputs = append(outputs, output)
		}

		// Input was intercepted (nil in this case)
		assert.Nil(t, interceptedInput)
		assert.Equal(t, []any{"response"}, outputs)
	})

	t.Run("intercepts output after execution", func(t *testing.T) {
		messagesKey := NewListKey[string]("messages")

		var interceptedOutputs []any

		outputMiddleware := func(next RunFunc[any, any]) RunFunc[any, any] {
			return func(ctx context.Context, input any) iter.Seq2[any, error] {
				return func(yield func(any, error) bool) {
					for output, err := range next(ctx, input) {
						interceptedOutputs = append(interceptedOutputs, output)
						if !yield(output, err) {
							return
						}
					}
				}
			}
		}

		g, err := New[any, any](messagesKey).
			Node("echo", func(ctx context.Context, scope Scope[any]) (*Command, error) {
				return Set(messagesKey, []string{"out1", "out2"}).End()
			}, END).
			Start("echo").
			WithRunMiddleware(outputMiddleware).
			Build()
		require.NoError(t, err)

		var outputs []any
		for output, err := range g.Run(context.Background(), nil) {
			require.NoError(t, err)
			outputs = append(outputs, output)
		}

		assert.Equal(t, []any{"out1", "out2"}, interceptedOutputs)
		assert.Equal(t, []any{"out1", "out2"}, outputs)
	})

	t.Run("blocks execution on input validation failure", func(t *testing.T) {
		messagesKey := NewListKey[string]("messages")

		nodeExecuted := false
		testErr := errors.New("input validation failed")

		// This middleware blocks ALL execution to simulate input validation failure
		validationMiddleware := func(next RunFunc[any, any]) RunFunc[any, any] {
			return func(ctx context.Context, input any) iter.Seq2[any, error] {
				return func(yield func(any, error) bool) {
					// Return error immediately without calling next
					yield(nil, testErr)
				}
			}
		}

		g, err := New[any, any](messagesKey).
			Node("echo", func(ctx context.Context, scope Scope[any]) (*Command, error) {
				nodeExecuted = true
				return Set(messagesKey, []string{"response"}).End()
			}, END).
			Start("echo").
			WithRunMiddleware(validationMiddleware).
			Build()
		require.NoError(t, err)

		// Should return error and not execute node
		for _, err := range g.Run(context.Background(), nil) {
			assert.ErrorIs(t, err, testErr)
		}
		assert.False(t, nodeExecuted)
	})

	t.Run("chains multiple middleware in correct order", func(t *testing.T) {
		messagesKey := NewListKey[string]("messages")

		var order []string

		mw1 := func(next RunFunc[any, any]) RunFunc[any, any] {
			return func(ctx context.Context, input any) iter.Seq2[any, error] {
				order = append(order, "mw1-before")
				result := next(ctx, input)
				return func(yield func(any, error) bool) {
					for output, err := range result {
						if !yield(output, err) {
							return
						}
					}
					order = append(order, "mw1-after")
				}
			}
		}

		mw2 := func(next RunFunc[any, any]) RunFunc[any, any] {
			return func(ctx context.Context, input any) iter.Seq2[any, error] {
				order = append(order, "mw2-before")
				result := next(ctx, input)
				return func(yield func(any, error) bool) {
					for output, err := range result {
						if !yield(output, err) {
							return
						}
					}
					order = append(order, "mw2-after")
				}
			}
		}

		g, err := New[any, any](messagesKey).
			Node("echo", func(ctx context.Context, scope Scope[any]) (*Command, error) {
				order = append(order, "node")
				return Set(messagesKey, []string{"response"}).End()
			}, END).
			Start("echo").
			WithRunMiddleware(mw1, mw2).
			Build()
		require.NoError(t, err)

		for _, err := range g.Run(context.Background(), nil) {
			require.NoError(t, err)
		}

		// First middleware is outermost, second is inner
		assert.Equal(t, []string{
			"mw1-before",
			"mw2-before",
			"node",
			"mw2-after",
			"mw1-after",
		}, order)
	})
}

func TestChainRunMiddleware(t *testing.T) {
	t.Run("chains multiple middleware", func(t *testing.T) {
		var order []string

		mw1 := func(next RunFunc[string, string]) RunFunc[string, string] {
			return func(ctx context.Context, input string) iter.Seq2[string, error] {
				order = append(order, "mw1")
				return next(ctx, input)
			}
		}

		mw2 := func(next RunFunc[string, string]) RunFunc[string, string] {
			return func(ctx context.Context, input string) iter.Seq2[string, error] {
				order = append(order, "mw2")
				return next(ctx, input)
			}
		}

		combined := ChainRunMiddleware(mw1, mw2)

		// Create a simple next function
		next := func(ctx context.Context, input string) iter.Seq2[string, error] {
			order = append(order, "next")
			return func(yield func(string, error) bool) {
				yield(input, nil)
			}
		}

		wrapped := combined(next)
		for _, _ = range wrapped(context.Background(), "test") {
		}

		// mw1 is outermost, mw2 is inner
		assert.Equal(t, []string{"mw1", "mw2", "next"}, order)
	})
}
