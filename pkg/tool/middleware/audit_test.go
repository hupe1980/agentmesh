package middleware

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// mockLogger captures log entries for testing.
type mockLogger struct {
	entries []logEntry
}

type logEntry struct {
	level   string
	message string
	args    []any
}

func newMockLogger() *mockLogger {
	return &mockLogger{entries: make([]logEntry, 0)}
}

func (l *mockLogger) Debug(msg string, args ...any) {
	l.entries = append(l.entries, logEntry{level: "debug", message: msg, args: args})
}

func (l *mockLogger) Info(msg string, args ...any) {
	l.entries = append(l.entries, logEntry{level: "info", message: msg, args: args})
}

func (l *mockLogger) Warn(msg string, args ...any) {
	l.entries = append(l.entries, logEntry{level: "warn", message: msg, args: args})
}

func (l *mockLogger) Error(msg string, args ...any) {
	l.entries = append(l.entries, logEntry{level: "error", message: msg, args: args})
}

func (l *mockLogger) With(args ...any) logging.Logger {
	return l
}

var _ logging.Logger = (*mockLogger)(nil)

func TestNewAuditMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("creates audit middleware with logger", func(t *testing.T) {
		t.Parallel()

		logger := newMockLogger()
		mw := NewAuditMiddleware(logger)
		require.NotNil(t, mw)
		assert.Equal(t, logger, mw.logger)
	})

	t.Run("creates audit middleware with nil logger", func(t *testing.T) {
		t.Parallel()

		mw := NewAuditMiddleware(nil)
		require.NotNil(t, mw)
		assert.Nil(t, mw.logger)
	})
}

func TestAuditMiddleware_Wrap(t *testing.T) {
	t.Parallel()

	t.Run("logs execution start and completion", func(t *testing.T) {
		t.Parallel()

		logger := newMockLogger()
		mw := NewAuditMiddleware(logger)
		exec := mw.Wrap(mockToolExecutor("success"))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{"input": "test"}`},
		}

		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 1)

		// Should have 2 log entries: start and completion
		require.Len(t, logger.entries, 2)
		assert.Equal(t, "info", logger.entries[0].level)
		assert.Equal(t, "tool execution started", logger.entries[0].message)
		assert.Equal(t, "info", logger.entries[1].level)
		assert.Equal(t, "tool execution completed", logger.entries[1].message)
	})

	t.Run("logs error on execution failure", func(t *testing.T) {
		t.Parallel()

		logger := newMockLogger()
		mw := NewAuditMiddleware(logger)
		exec := mw.Wrap(erroringToolExecutor(assert.AnError))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		_, err := exec.Execute(context.Background(), calls)
		require.Error(t, err)

		// Should have start log and error log
		require.Len(t, logger.entries, 2)
		assert.Equal(t, "info", logger.entries[0].level)
		assert.Equal(t, "error", logger.entries[1].level)
		assert.Equal(t, "tool execution failed", logger.entries[1].message)
	})

	t.Run("handles multiple tool calls", func(t *testing.T) {
		t.Parallel()

		logger := newMockLogger()
		mw := NewAuditMiddleware(logger)
		exec := mw.Wrap(mockToolExecutor("result"))

		calls := []tool.Call{
			{ID: "1", Name: "tool_a", Arguments: `{}`},
			{ID: "2", Name: "tool_b", Arguments: `{}`},
			{ID: "3", Name: "tool_c", Arguments: `{}`},
		}

		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 3)

		// Check start log contains count
		assert.Equal(t, "tool execution started", logger.entries[0].message)
	})

	t.Run("works without logger", func(t *testing.T) {
		t.Parallel()

		mw := NewAuditMiddleware(nil)
		exec := mw.Wrap(mockToolExecutor("success"))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "success", results[0].Result)
	})

	t.Run("logs tool result errors in completion", func(t *testing.T) {
		t.Parallel()

		logger := newMockLogger()
		mw := NewAuditMiddleware(logger)
		exec := mw.Wrap(toolExecutorWithResultError(assert.AnError))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Error(t, results[0].Error)

		// Should still log completion (not error level)
		assert.Equal(t, "info", logger.entries[1].level)
		assert.Equal(t, "tool execution completed", logger.entries[1].message)
	})
}

func TestToJSON(t *testing.T) {
	t.Parallel()

	t.Run("converts value to JSON string", func(t *testing.T) {
		t.Parallel()

		result := toJSON(map[string]string{"key": "value"})
		assert.Equal(t, `{"key":"value"}`, result)
	})

	t.Run("returns empty string for unmarshable value", func(t *testing.T) {
		t.Parallel()

		// Channels can't be marshaled to JSON
		ch := make(chan int)
		result := toJSON(ch)
		assert.Equal(t, "", result)
	})
}

// erroringToolExecutor creates a tool executor that returns an error.
func erroringToolExecutor(err error) tool.Executor {
	return tool.WrapFunc(func(_ context.Context, _ []tool.Call) ([]tool.ExecutionResult, error) {
		return nil, err
	})
}

// toolExecutorWithResultError creates a tool executor that returns results with errors.
func toolExecutorWithResultError(err error) tool.Executor {
	return tool.WrapFunc(func(_ context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
		results := make([]tool.ExecutionResult, len(calls))
		for i, call := range calls {
			results[i] = tool.ExecutionResult{
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Error:      err,
			}
		}
		return results, nil
	})
}
