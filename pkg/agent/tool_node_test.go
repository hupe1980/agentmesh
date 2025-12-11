package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/testutil"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewToolNodeFunc(t *testing.T) {
	t.Run("returns error when no executor or toolset provided", func(t *testing.T) {
		fn, err := NewToolNodeFunc()
		assert.Error(t, err)
		assert.Nil(t, fn)
		assert.Contains(t, err.Error(), "Executor or Toolset")
	})

	t.Run("creates function successfully with executor", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewSequentialExecutor(registry)

		fn, err := NewToolNodeFunc(WithToolExecutor(executor))
		require.NoError(t, err)
		assert.NotNil(t, fn)
	})

	t.Run("creates function successfully with toolset", func(t *testing.T) {
		toolset := tool.NewStaticToolset()

		fn, err := NewToolNodeFunc(WithToolNodeToolset(toolset))
		require.NoError(t, err)
		assert.NotNil(t, fn)
	})

	t.Run("with custom model target", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewSequentialExecutor(registry)

		fn, err := NewToolNodeFunc(WithToolExecutor(executor), WithModelTarget("custom_model"))
		require.NoError(t, err)
		assert.NotNil(t, fn)
	})
}

func TestToolNodeFunc_Execution(t *testing.T) {
	t.Run("routes to model when no message", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewSequentialExecutor(registry)

		fn, err := NewToolNodeFunc(WithToolExecutor(executor))
		require.NoError(t, err)

		scope := createToolNodeTestScope(map[string]any{
			MessagesKey.Name(): []message.Message{},
		})

		cmd, err := fn(context.Background(), scope)
		require.NoError(t, err)
		require.NotNil(t, cmd)
		assert.Contains(t, cmd.Next, "model")
	})

	t.Run("routes to model when last message is not AI", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewSequentialExecutor(registry)

		fn, err := NewToolNodeFunc(WithToolExecutor(executor))
		require.NoError(t, err)

		scope := createToolNodeTestScope(map[string]any{
			MessagesKey.Name(): []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
		})

		cmd, err := fn(context.Background(), scope)
		require.NoError(t, err)
		require.NotNil(t, cmd)
		assert.Contains(t, cmd.Next, "model")
	})

	t.Run("routes to model when AI message has no tool calls", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewSequentialExecutor(registry)

		fn, err := NewToolNodeFunc(WithToolExecutor(executor))
		require.NoError(t, err)

		scope := createToolNodeTestScope(map[string]any{
			MessagesKey.Name(): []message.Message{
				message.NewAIMessageFromText("No tools needed"),
			},
		})

		cmd, err := fn(context.Background(), scope)
		require.NoError(t, err)
		require.NotNil(t, cmd)
		assert.Contains(t, cmd.Next, "model")
	})

	t.Run("routes to custom target", func(t *testing.T) {
		registry := make(map[string]tool.Tool)
		executor := tool.NewSequentialExecutor(registry)

		fn, err := NewToolNodeFunc(WithToolExecutor(executor), WithModelTarget("custom_node"))
		require.NoError(t, err)

		scope := createToolNodeTestScope(map[string]any{
			MessagesKey.Name(): []message.Message{
				message.NewHumanMessageFromText("Hello"),
			},
		})

		cmd, err := fn(context.Background(), scope)
		require.NoError(t, err)
		require.NotNil(t, cmd)
		assert.Contains(t, cmd.Next, "custom_node")
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

	t.Run("formats empty string", func(t *testing.T) {
		result := formatToolResult("")
		assert.Equal(t, "", result)
	})
}

// customStringer implements fmt.Stringer for testing
type customStringer struct {
	value string
}

func (cs customStringer) String() string {
	return fmt.Sprintf("CustomStringer[%s]", cs.value)
}

// createToolNodeTestScope creates a Scope for testing using BSPState
func createToolNodeTestScope(data map[string]any) graph.Scope[message.Message] {
	return testutil.NewTestScopeFromMap[message.Message](data)
}
