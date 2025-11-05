package graph

import (
	"context"
	"errors"
	"testing"

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
