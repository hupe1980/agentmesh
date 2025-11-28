package graph

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeExecutionError(t *testing.T) {
	t.Run("Error method formats correctly", func(t *testing.T) {
		baseErr := errors.New("test error")
		nodeErr := &NodeExecutionError{
			NodeName: "test_node",
			Err:      baseErr,
		}

		expected := "graph: node test_node execution failed: test error"
		assert.Equal(t, expected, nodeErr.Error())
	})

	t.Run("Unwrap returns wrapped error", func(t *testing.T) {
		baseErr := errors.New("test error")
		nodeErr := &NodeExecutionError{
			NodeName: "test_node",
			Err:      baseErr,
		}

		unwrapped := nodeErr.Unwrap()
		assert.Equal(t, baseErr, unwrapped)
	})

	t.Run("errors.Is works with ErrNodeExecution", func(t *testing.T) {
		baseErr := errors.New("test error")
		nodeErr := &NodeExecutionError{
			NodeName: "test_node",
			Err:      baseErr,
		}

		// Should match ErrNodeExecution sentinel
		assert.True(t, errors.Is(nodeErr, ErrNodeExecution))

		// Should also unwrap to base error
		assert.True(t, errors.Is(nodeErr, baseErr))
	})

	t.Run("errors.As extracts NodeExecutionError", func(t *testing.T) {
		baseErr := errors.New("test error")
		nodeErr := &NodeExecutionError{
			NodeName: "test_node",
			Err:      baseErr,
		}

		// Wrap in another error
		wrappedErr := fmt.Errorf("wrapper: %w", nodeErr)

		var target *NodeExecutionError
		assert.True(t, errors.As(wrappedErr, &target))
		assert.Equal(t, "test_node", target.NodeName)
		assert.Equal(t, baseErr, target.Err)
	})

	t.Run("Works with nested wrapping", func(t *testing.T) {
		baseErr := errors.New("base error")
		nodeErr := &NodeExecutionError{
			NodeName: "test_node",
			Err:      baseErr,
		}
		wrappedErr := fmt.Errorf("level 1: %w", nodeErr)
		doubleWrappedErr := fmt.Errorf("level 2: %w", wrappedErr)

		// Should still find ErrNodeExecution through multiple wraps
		assert.True(t, errors.Is(doubleWrappedErr, ErrNodeExecution))

		// Should still extract NodeExecutionError
		var target *NodeExecutionError
		assert.True(t, errors.As(doubleWrappedErr, &target))
		assert.Equal(t, "test_node", target.NodeName)
	})

	t.Run("Multiple NodeExecutionErrors in chain", func(t *testing.T) {
		err1 := &NodeExecutionError{
			NodeName: "node1",
			Err:      errors.New("error1"),
		}
		err2 := &NodeExecutionError{
			NodeName: "node2",
			Err:      err1,
		}

		// errors.As should find the outermost NodeExecutionError first
		var target *NodeExecutionError
		assert.True(t, errors.As(err2, &target))
		assert.Equal(t, "node2", target.NodeName)

		// Should still identify as ErrNodeExecution
		assert.True(t, errors.Is(err2, ErrNodeExecution))
	})

	t.Run("Different node names in error messages", func(t *testing.T) {
		nodeErr := &NodeExecutionError{
			NodeName: "processor_node",
			Err:      errors.New("processing failed"),
		}

		assert.Contains(t, nodeErr.Error(), "processor_node")
		assert.Contains(t, nodeErr.Error(), "processing failed")
	})

	t.Run("Nil wrapped error", func(t *testing.T) {
		nodeErr := &NodeExecutionError{
			NodeName: "test_node",
			Err:      nil,
		}

		assert.Equal(t, "graph: node test_node execution failed", nodeErr.Error())
		assert.Nil(t, nodeErr.Unwrap())
	})
}
