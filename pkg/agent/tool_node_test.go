package agent

import (
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolNode_Configuration(t *testing.T) {
	t.Run("sets custom node name", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewSequentialExecutor(registry)

		node, err := NewToolNode(executor, WithToolNodeName("custom_tools"))
		require.NoError(t, err)
		assert.Equal(t, "custom_tools", node.Name())
	})

	t.Run("sets custom targets", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewSequentialExecutor(registry)

		customTargets := []string{"validator", "model"}
		node, err := NewToolNode(executor, WithToolTargets(customTargets))
		require.NoError(t, err)
		assert.Equal(t, customTargets, node.Targets())
	})

	t.Run("uses default values", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewSequentialExecutor(registry)

		node, err := NewToolNode(executor)
		require.NoError(t, err)
		assert.Equal(t, "tool", node.Name())
		assert.Equal(t, []string{"model"}, node.Targets())
	})
}

func TestFormatToolResult(t *testing.T) {
	t.Run("formats nil as 'null'", func(t *testing.T) {
		result := formatToolResult(nil)
		assert.Equal(t, "null", result)
	})

	t.Run("returns string as-is", func(t *testing.T) {
		result := formatToolResult("test string")
		assert.Equal(t, "test string", result)
	})

	t.Run("calls String() on Stringer types", func(t *testing.T) {
		// Use the customStringer defined at bottom of file
		obj := customStringer{value: "test"}
		result := formatToolResult(obj)
		assert.Equal(t, "CustomStringer[test]", result)
	})

	t.Run("formats numbers", func(t *testing.T) {
		result := formatToolResult(42)
		assert.Equal(t, "42", result)

		result = formatToolResult(3.14)
		assert.Equal(t, "3.14", result)
	})

	t.Run("formats booleans", func(t *testing.T) {
		result := formatToolResult(true)
		assert.Equal(t, "true", result)

		result = formatToolResult(false)
		assert.Equal(t, "false", result)
	})

	t.Run("formats maps", func(t *testing.T) {
		data := map[string]any{
			"temperature": 21,
			"condition":   "sunny",
		}
		result := formatToolResult(data)
		// Map formatting is non-deterministic in order, but should contain key data
		assert.Contains(t, result, "temperature")
		assert.Contains(t, result, "sunny")
	})

	t.Run("formats structs", func(t *testing.T) {
		type Response struct {
			Status  string
			Code    int
			Success bool
		}
		data := Response{Status: "ok", Code: 200, Success: true}
		result := formatToolResult(data)
		assert.Contains(t, result, "ok")
		assert.Contains(t, result, "200")
	})

	t.Run("formats slices", func(t *testing.T) {
		data := []string{"apple", "banana", "cherry"}
		result := formatToolResult(data)
		assert.Contains(t, result, "apple")
		assert.Contains(t, result, "banana")
		assert.Contains(t, result, "cherry")
	})

	t.Run("formats pointer types", func(t *testing.T) {
		str := "pointer value"
		result := formatToolResult(&str)
		// Pointer formatting uses %v which shows the pointer address
		assert.Contains(t, result, "0x")
	})

	t.Run("formats empty string", func(t *testing.T) {
		result := formatToolResult("")
		assert.Equal(t, "", result)
	})
}

func TestNewToolNode_Validation(t *testing.T) {
	t.Run("returns error for nil executor", func(t *testing.T) {
		node, err := NewToolNode(nil)
		assert.Error(t, err)
		assert.Nil(t, node)
		assert.Contains(t, err.Error(), "executor cannot be nil")
	})

	t.Run("accepts sequential executor", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewSequentialExecutor(registry)

		node, err := NewToolNode(executor)
		require.NoError(t, err)
		assert.NotNil(t, node)
	})

	t.Run("accepts parallel executor", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewParallelExecutor(registry)

		node, err := NewToolNode(executor)
		require.NoError(t, err)
		assert.NotNil(t, node)
	})
}

func TestToolNode_WithOptions(t *testing.T) {
	t.Run("applies multiple options", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		registry["test_tool"] = &testutil.MockTool{NameValue: "test_tool"}
		executor := tool.NewSequentialExecutor(registry)

		node, err := NewToolNode(executor,
			WithToolNodeName("my_tools"),
			WithToolTargets([]string{"validator", "model", "end"}))

		require.NoError(t, err)
		assert.Equal(t, "my_tools", node.Name())
		assert.Equal(t, []string{"validator", "model", "end"}, node.Targets())
	})

	t.Run("option order doesn't matter", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewSequentialExecutor(registry)

		// Apply in different order
		node1, err1 := NewToolNode(executor,
			WithToolNodeName("tools"),
			WithToolTargets([]string{"model"}))

		node2, err2 := NewToolNode(executor,
			WithToolTargets([]string{"model"}),
			WithToolNodeName("tools"))

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, node1.Name(), node2.Name())
		assert.Equal(t, node1.Targets(), node2.Targets())
	})
}

func TestToolNode_NameAndTargets(t *testing.T) {
	registry := make(map[string]tool.Tool)
	executor := tool.NewSequentialExecutor(registry)

	t.Run("Name() returns configured name", func(t *testing.T) {
		node, err := NewToolNode(executor, WithToolNodeName("execution_tools"))
		require.NoError(t, err)
		assert.Equal(t, "execution_tools", node.Name())
	})

	t.Run("Targets() returns configured targets", func(t *testing.T) {
		targets := []string{"validator", "processor", "model"}
		node, err := NewToolNode(executor, WithToolTargets(targets))
		require.NoError(t, err)
		assert.Equal(t, targets, node.Targets())
	})

	t.Run("Name() and Targets() work together", func(t *testing.T) {
		name := "custom_executor"
		targets := []string{"next_node"}

		node, err := NewToolNode(executor,
			WithToolNodeName(name),
			WithToolTargets(targets))

		require.NoError(t, err)
		assert.Equal(t, name, node.Name())
		assert.Equal(t, targets, node.Targets())
	})
}

// Test Stringer interface implementation
type customStringer struct {
	value string
}

func (cs customStringer) String() string {
	return fmt.Sprintf("CustomStringer[%s]", cs.value)
}

func TestFormatToolResult_Stringer(t *testing.T) {
	t.Run("uses Stringer.String() method", func(t *testing.T) {
		obj := customStringer{value: "test"}
		result := formatToolResult(obj)
		assert.Equal(t, "CustomStringer[test]", result)
	})

	t.Run("handles pointer to Stringer", func(t *testing.T) {
		obj := &customStringer{value: "pointer"}
		result := formatToolResult(obj)
		assert.Equal(t, "CustomStringer[pointer]", result)
	})
}
