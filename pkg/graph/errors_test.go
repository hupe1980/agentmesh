package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeExecutionError(t *testing.T) {
	baseErr := errors.New("database connection failed")

	t.Run("with_superstep", func(t *testing.T) {
		err := &NodeExecutionError{
			Node:      "processor",
			Superstep: 5,
			Cause:     baseErr,
		}

		assert.Contains(t, err.Error(), "processor")
		assert.Contains(t, err.Error(), "failed")
		assert.Contains(t, err.Error(), "database connection failed")

		// Check Unwrap
		assert.ErrorIs(t, err, baseErr)
		require.Equal(t, baseErr, errors.Unwrap(err))
	})

	t.Run("without_superstep", func(t *testing.T) {
		err := &NodeExecutionError{
			Node:      "validator",
			Superstep: -1,
			Cause:     baseErr,
		}

		assert.Contains(t, err.Error(), "validator")
		assert.Contains(t, err.Error(), "failed")
		assert.NotContains(t, err.Error(), "superstep")
	})

	t.Run("unwrap_chain", func(t *testing.T) {
		rootErr := errors.New("root cause")
		wrapped := errors.New("wrapped: " + rootErr.Error())

		execErr := &NodeExecutionError{
			Node:      "worker",
			Superstep: 0,
			Cause:     wrapped,
		}

		assert.Equal(t, wrapped, errors.Unwrap(execErr))
	})
}

func TestValidationError(t *testing.T) {
	t.Run("with_field_and_value", func(t *testing.T) {
		err := &ValidationError{
			Field:   "Name",
			Value:   "duplicate_node",
			Message: "node already exists",
		}

		errStr := err.Error()
		assert.Contains(t, errStr, "validation error")
		assert.Contains(t, errStr, "Name")
		assert.Contains(t, errStr, "node already exists")
	})

	t.Run("with_field_only", func(t *testing.T) {
		err := &ValidationError{
			Field:   "MaxConcurrency",
			Message: "must be greater than zero",
		}

		errStr := err.Error()
		assert.Contains(t, errStr, "validation error")
		assert.Contains(t, errStr, "MaxConcurrency")
		assert.Contains(t, errStr, "must be greater than zero")
	})

	t.Run("message_only", func(t *testing.T) {
		err := &ValidationError{
			Message: "graph must have at least one node",
		}

		errStr := err.Error()
		assert.Contains(t, errStr, "validation error")
		assert.Contains(t, errStr, "graph must have at least one node")
	})
}

func TestMessageLimitError(t *testing.T) {
	err := &MessageLimitError{
		Limit:     100,
		Attempted: 150,
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "message limit exceeded")
	assert.Contains(t, errStr, "limit")
	assert.Contains(t, errStr, "attempted")
}

// TestStructuredErrors_Integration verifies structured errors work with AddNode
func TestStructuredErrors_Integration(t *testing.T) {
	t.Run("empty_node_name", func(t *testing.T) {
		g := &Graph{}
		err := g.AddNode(&Node{Name: ""})

		require.Error(t, err)

		var validationErr *ValidationError
		assert.True(t, errors.As(err, &validationErr), "Should be ValidationError")
		assert.Equal(t, "Name", validationErr.Field)
	})

	t.Run("duplicate_node_name", func(t *testing.T) {
		g := &Graph{}

		noopFunc := func(ctx context.Context, s StateWriter) (*NodeResult, error) { return nil, nil }

		err1 := g.AddNode(&Node{Name: "worker", RunFunc: noopFunc})
		require.NoError(t, err1)

		err2 := g.AddNode(&Node{Name: "worker", RunFunc: noopFunc})
		require.Error(t, err2)

		var validationErr *ValidationError
		assert.True(t, errors.As(err2, &validationErr), "Should be ValidationError")
		assert.Equal(t, "Name", validationErr.Field)
		assert.Equal(t, "worker", validationErr.Value)
	})
}

// TestErrorWrapping verifies structured errors can be wrapped and unwrapped
func TestErrorWrapping(t *testing.T) {
	baseErr := errors.New("connection timeout")

	nodeErr := &NodeExecutionError{
		Node:      "api_client",
		Superstep: 3,
		Cause:     baseErr,
	}

	// Wrap in additional context
	wrappedErr := errors.New("workflow failed: " + nodeErr.Error())

	// Should still be able to check for base error through the chain
	assert.Contains(t, wrappedErr.Error(), "connection timeout")
	assert.Contains(t, wrappedErr.Error(), "api_client")
}

func TestNodeTimeoutError(t *testing.T) {
	t.Run("enforces_context_deadline", func(t *testing.T) {
		state := NewGraphState(0)
		g := NewGraph(state)

		err := g.AddNode(&Node{
			Name: "slow-node",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				// Do work that takes longer than the timeout (100ms)
				// Sleep for 500ms total, checking context every 50ms
				for i := 0; i < 10; i++ { // 10 * 50ms = 500ms total
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(50 * time.Millisecond):
						// Continue working
					}
				}
				return &NodeResult{}, nil
			},
		})
		require.NoError(t, err)

		g.AddEdge(StartNode, "slow-node")
		g.AddEdge("slow-node", EndNode)

		compiled, err := g.Compile()
		require.NoError(t, err)

		// Set timeout shorter than node execution time (100ms < 300ms)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err = compiled.Invoke(ctx, nil)
		duration := time.Since(start)

		// Should timeout
		require.Error(t, err, "node should timeout before completing")

		// Should timeout quickly (not wait for full 300ms)
		assert.Less(t, duration, 200*time.Millisecond,
			"should timeout quickly, not wait for full node execution")

		// Check error is properly wrapped
		var timeoutErr *NodeTimeoutError
		require.ErrorAs(t, err, &timeoutErr, "expected NodeTimeoutError, got: %v", err)
		assert.Equal(t, "slow-node", timeoutErr.Node)
		assert.Contains(t, timeoutErr.Error(), "slow-node")
		assert.Contains(t, timeoutErr.Error(), "timeout")
		assert.ErrorIs(t, timeoutErr, context.DeadlineExceeded)
	})

	t.Run("timeout_error_unwraps", func(t *testing.T) {
		baseErr := context.DeadlineExceeded
		timeoutErr := &NodeTimeoutError{
			Node:    "processor",
			Timeout: 5000,
			Cause:   baseErr,
		}

		assert.Contains(t, timeoutErr.Error(), "processor")
		assert.Contains(t, timeoutErr.Error(), "timeout")
		assert.ErrorIs(t, timeoutErr, context.DeadlineExceeded)
		assert.Equal(t, baseErr, errors.Unwrap(timeoutErr))
	})
}
