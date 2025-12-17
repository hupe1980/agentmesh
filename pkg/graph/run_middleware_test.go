package graph

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunMiddleware(t *testing.T) {
	t.Run("intercepts input before execution", func(t *testing.T) {
		var interceptedInput any

		inputMiddleware := func(next RunFunc) RunFunc {
			return func(ctx context.Context, input []message.Message) iter.Seq2[message.Message, error] {
				interceptedInput = input
				return next(ctx, input)
			}
		}

		g, err := New().
			Node("echo", func(ctx context.Context, scope Scope) (*Command, error) {
				scope.Stream(message.NewAIMessageFromText("response"))
				return To(END)
			}, END).
			Start("echo").
			WithRunMiddleware(inputMiddleware).
			Build()
		require.NoError(t, err)

		var outputs []message.Message
		for output, err := range g.Run(context.Background(), nil) {
			require.NoError(t, err)
			if output != nil {
				outputs = append(outputs, output)
			}
		}

		// Input was intercepted (nil in this case)
		assert.Nil(t, interceptedInput)
		require.Len(t, outputs, 1)
		assert.Equal(t, "response", outputs[0].String())
	})

	t.Run("intercepts output after execution", func(t *testing.T) {
		var interceptedOutputs []message.Message

		outputMiddleware := func(next RunFunc) RunFunc {
			return func(ctx context.Context, input []message.Message) iter.Seq2[message.Message, error] {
				return func(yield func(message.Message, error) bool) {
					for output, err := range next(ctx, input) {
						if output != nil {
							interceptedOutputs = append(interceptedOutputs, output)
						}
						if !yield(output, err) {
							return
						}
					}
				}
			}
		}

		g, err := New().
			Node("echo", func(ctx context.Context, scope Scope) (*Command, error) {
				scope.Stream(message.NewAIMessageFromText("out1"))
				scope.Stream(message.NewAIMessageFromText("out2"))
				return To(END)
			}, END).
			Start("echo").
			WithRunMiddleware(outputMiddleware).
			Build()
		require.NoError(t, err)

		var outputs []message.Message
		for output, err := range g.Run(context.Background(), nil) {
			require.NoError(t, err)
			if output != nil {
				outputs = append(outputs, output)
			}
		}

		require.Len(t, interceptedOutputs, 2)
		assert.Equal(t, "out1", interceptedOutputs[0].String())
		assert.Equal(t, "out2", interceptedOutputs[1].String())
		require.Len(t, outputs, 2)
		assert.Equal(t, "out1", outputs[0].String())
		assert.Equal(t, "out2", outputs[1].String())
	})

	t.Run("blocks execution on input validation failure", func(t *testing.T) {
		nodeExecuted := false
		testErr := errors.New("input validation failed")

		// This middleware blocks ALL execution to simulate input validation failure
		validationMiddleware := func(next RunFunc) RunFunc {
			return func(ctx context.Context, input []message.Message) iter.Seq2[message.Message, error] {
				return func(yield func(message.Message, error) bool) {
					// Return error immediately without calling next
					yield(nil, testErr)
				}
			}
		}

		g, err := New().
			Node("echo", func(ctx context.Context, scope Scope) (*Command, error) {
				nodeExecuted = true
				scope.Stream(message.NewAIMessageFromText("response"))
				return To(END)
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
		var order []string

		mw1 := func(next RunFunc) RunFunc {
			return func(ctx context.Context, input []message.Message) iter.Seq2[message.Message, error] {
				order = append(order, "mw1-before")
				result := next(ctx, input)
				return func(yield func(message.Message, error) bool) {
					for output, err := range result {
						if !yield(output, err) {
							return
						}
					}
					order = append(order, "mw1-after")
				}
			}
		}

		mw2 := func(next RunFunc) RunFunc {
			return func(ctx context.Context, input []message.Message) iter.Seq2[message.Message, error] {
				order = append(order, "mw2-before")
				result := next(ctx, input)
				return func(yield func(message.Message, error) bool) {
					for output, err := range result {
						if !yield(output, err) {
							return
						}
					}
					order = append(order, "mw2-after")
				}
			}
		}

		g, err := New().
			Node("echo", func(ctx context.Context, scope Scope) (*Command, error) {
				order = append(order, "node")
				scope.Stream(message.NewAIMessageFromText("response"))
				return To(END)
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

		mw1 := func(next RunFunc) RunFunc {
			return func(ctx context.Context, input []message.Message) iter.Seq2[message.Message, error] {
				order = append(order, "mw1")
				return next(ctx, input)
			}
		}

		mw2 := func(next RunFunc) RunFunc {
			return func(ctx context.Context, input []message.Message) iter.Seq2[message.Message, error] {
				order = append(order, "mw2")
				return next(ctx, input)
			}
		}

		combined := ChainRunMiddleware(mw1, mw2)

		// Create a simple next function
		next := func(ctx context.Context, input []message.Message) iter.Seq2[message.Message, error] {
			order = append(order, "next")
			return func(yield func(message.Message, error) bool) {
				if len(input) > 0 {
					yield(input[0], nil)
				}
			}
		}

		wrapped := combined(next)
		for _, _ = range wrapped(context.Background(), nil) {
		}

		// mw1 is outermost, mw2 is inner
		assert.Equal(t, []string{"mw1", "mw2", "next"}, order)
	})
}
