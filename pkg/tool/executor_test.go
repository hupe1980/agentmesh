package tool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTool is a test double for Tool
type mockTool struct {
	callFunc func(ctx context.Context, input string) (any, error)
}

func (m *mockTool) Call(ctx context.Context, input string) (any, error) {
	if m.callFunc != nil {
		return m.callFunc(ctx, input)
	}
	return "mock result", nil
}

func (m *mockTool) Name() string {
	return "mock-tool"
}

func (m *mockTool) Description() string {
	return "A mock tool for testing"
}

func (m *mockTool) Definition() *Definition {
	return &Definition{
		Type: "function",
		Function: FunctionDefinition{
			Name:        m.Name(),
			Description: m.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{"type": "string"},
				},
			},
		},
	}
}

// mockPlugin is a test double for Plugin
type mockPlugin struct {
	beforeToolFunc  func(ctx context.Context, name string, input any) error
	afterToolFunc   func(ctx context.Context, name string, result any) error
	onToolErrorFunc func(ctx context.Context, name string, err error) error
}

func (p *mockPlugin) ExecuteBeforeTool(ctx context.Context, name string, input any) error {
	if p.beforeToolFunc != nil {
		return p.beforeToolFunc(ctx, name, input)
	}
	return nil
}

func (p *mockPlugin) ExecuteAfterTool(ctx context.Context, name string, result any) error {
	if p.afterToolFunc != nil {
		return p.afterToolFunc(ctx, name, result)
	}
	return nil
}

func (p *mockPlugin) ExecuteOnToolError(ctx context.Context, name string, err error) error {
	if p.onToolErrorFunc != nil {
		return p.onToolErrorFunc(ctx, name, err)
	}
	return nil
}

// Ensure mockPlugin implements Plugin interface
var _ Plugin = (*mockPlugin)(nil)

// TestSequentialExecutor_BasicExecution tests basic sequential execution
func TestSequentialExecutor_BasicExecution(t *testing.T) {
	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "result1", nil
			},
		},
		"tool2": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "result2", nil
			},
		},
	}

	executor := NewSequentialExecutor(registry)

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{"input":"test1"}`},
		{ID: "2", Name: "tool2", Arguments: `{"input":"test2"}`},
	}

	results, err := executor.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "1", results[0].ToolCallID)
	assert.Equal(t, "tool1", results[0].ToolName)
	assert.Equal(t, "result1", results[0].Result)
	assert.NoError(t, results[0].Error)

	assert.Equal(t, "2", results[1].ToolCallID)
	assert.Equal(t, "tool2", results[1].ToolName)
	assert.Equal(t, "result2", results[1].Result)
	assert.NoError(t, results[1].Error)
}

// TestSequentialExecutor_ErrorHandling tests error propagation
func TestSequentialExecutor_ErrorHandling(t *testing.T) {
	expectedErr := errors.New("tool error")
	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "result1", nil
			},
		},
		"tool2": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "", expectedErr
			},
		},
		"tool3": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "result3", nil
			},
		},
	}

	executor := NewSequentialExecutor(registry)

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{}`},
		{ID: "2", Name: "tool2", Arguments: `{}`},
		{ID: "3", Name: "tool3", Arguments: `{}`},
	}

	results, err := executor.Execute(context.Background(), calls)
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Len(t, results, 2) // Only tool1 and tool2 executed

	assert.NoError(t, results[0].Error)
	assert.Equal(t, "result1", results[0].Result)
	assert.Equal(t, expectedErr, results[1].Error)
}

// TestSequentialExecutor_ContinueOnError tests continue-on-error behavior
func TestSequentialExecutor_ContinueOnError(t *testing.T) {
	expectedErr := errors.New("tool error")
	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "result1", nil
			},
		},
		"tool2": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "", expectedErr
			},
		},
		"tool3": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "result3", nil
			},
		},
	}

	executor := NewSequentialExecutor(registry, WithContinueOnError(true))

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{}`},
		{ID: "2", Name: "tool2", Arguments: `{}`},
		{ID: "3", Name: "tool3", Arguments: `{}`},
	}

	results, err := executor.Execute(context.Background(), calls)
	require.NoError(t, err) // No error returned when continuing
	require.Len(t, results, 3)

	assert.NoError(t, results[0].Error)
	assert.Equal(t, "result1", results[0].Result)
	assert.Equal(t, expectedErr, results[1].Error)
	assert.Nil(t, results[1].Result)
	assert.NoError(t, results[2].Error)
	assert.Equal(t, "result3", results[2].Result)
}

// TestSequentialExecutor_ToolNotFound tests missing tool handling
func TestSequentialExecutor_ToolNotFound(t *testing.T) {
	registry := map[string]Tool{
		"tool1": &mockTool{},
	}

	executor := NewSequentialExecutor(registry, WithErrorPrefix("test-agent"))

	calls := []Call{
		{ID: "1", Name: "nonexistent", Arguments: `{}`},
	}

	results, err := executor.Execute(context.Background(), calls)
	assert.Error(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Error.Error(), "test-agent")
	assert.Contains(t, results[0].Error.Error(), "nonexistent")
	assert.Contains(t, results[0].Error.Error(), "not registered")
}

// TestSequentialExecutor_WithPlugins tests plugin lifecycle
func TestSequentialExecutor_WithPlugins(t *testing.T) {
	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "original result", nil
			},
		},
	}

	var beforeCalled, afterCalled bool
	var capturedResult any
	pm := &mockPlugin{
		beforeToolFunc: func(ctx context.Context, name string, input any) error {
			beforeCalled = true
			assert.Equal(t, "tool1", name)
			return nil
		},
		afterToolFunc: func(ctx context.Context, name string, result any) error {
			afterCalled = true
			capturedResult = result
			return nil
		},
	}

	ctx := WithPlugin(context.Background(), pm)
	executor := NewSequentialExecutor(registry)

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{"input":"test"}`},
	}

	results, err := executor.Execute(ctx, calls)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, beforeCalled, "BeforeTool should be called")
	assert.True(t, afterCalled, "AfterTool should be called")
	assert.Equal(t, "original result", capturedResult)
}

// TestSequentialExecutor_PluginErrorHandling tests plugin error handling
func TestSequentialExecutor_PluginErrorHandling(t *testing.T) {
	toolErr := errors.New("tool error")
	transformedErr := errors.New("transformed error")

	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "", toolErr
			},
		},
	}

	var onErrorCalled bool
	pm := &mockPlugin{
		onToolErrorFunc: func(ctx context.Context, name string, err error) error {
			onErrorCalled = true
			assert.Equal(t, toolErr, err)
			return transformedErr
		},
	}

	ctx := WithPlugin(context.Background(), pm)
	executor := NewSequentialExecutor(registry)

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{}`},
	}

	results, err := executor.Execute(ctx, calls)
	assert.Error(t, err)
	assert.Len(t, results, 1)
	assert.True(t, onErrorCalled)
	assert.Equal(t, transformedErr, results[0].Error)
}

// TestParallelExecutor_BasicExecution tests basic parallel execution
func TestParallelExecutor_BasicExecution(t *testing.T) {
	var mu sync.Mutex
	executionOrder := []string{}

	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				executionOrder = append(executionOrder, "tool1")
				mu.Unlock()
				return "result1", nil
			},
		},
		"tool2": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				time.Sleep(5 * time.Millisecond)
				mu.Lock()
				executionOrder = append(executionOrder, "tool2")
				mu.Unlock()
				return "result2", nil
			},
		},
		"tool3": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				mu.Lock()
				executionOrder = append(executionOrder, "tool3")
				mu.Unlock()
				return "result3", nil
			},
		},
	}

	executor := NewParallelExecutor(registry)

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{}`},
		{ID: "2", Name: "tool2", Arguments: `{}`},
		{ID: "3", Name: "tool3", Arguments: `{}`},
	}

	start := time.Now()
	results, err := executor.Execute(context.Background(), calls)
	duration := time.Since(start)

	require.NoError(t, err)
	require.Len(t, results, 3)

	// Verify results are in the same order as calls (not execution order)
	assert.Equal(t, "1", results[0].ToolCallID)
	assert.Equal(t, "result1", results[0].Result)
	assert.Equal(t, "2", results[1].ToolCallID)
	assert.Equal(t, "result2", results[1].Result)
	assert.Equal(t, "3", results[2].ToolCallID)
	assert.Equal(t, "result3", results[2].Result)

	// Parallel execution should be faster than sequential (< 20ms instead of 15ms)
	assert.Less(t, duration, 20*time.Millisecond, "Parallel execution should be faster")

	// Verify tools executed in parallel (tool3 should finish before tool1)
	assert.NotEqual(t, []string{"tool1", "tool2", "tool3"}, executionOrder,
		"Execution order should not be sequential")
}

// TestParallelExecutor_MaxConcurrency tests concurrency limiting
func TestParallelExecutor_MaxConcurrency(t *testing.T) {
	var mu sync.Mutex
	var maxConcurrent int
	var currentConcurrent int

	registry := map[string]Tool{
		"tool": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				mu.Lock()
				currentConcurrent++
				if currentConcurrent > maxConcurrent {
					maxConcurrent = currentConcurrent
				}
				mu.Unlock()

				time.Sleep(10 * time.Millisecond)

				mu.Lock()
				currentConcurrent--
				mu.Unlock()

				return "result", nil
			},
		},
	}

	executor := NewParallelExecutor(registry, WithMaxConcurrency(2))

	calls := []Call{
		{ID: "1", Name: "tool", Arguments: `{}`},
		{ID: "2", Name: "tool", Arguments: `{}`},
		{ID: "3", Name: "tool", Arguments: `{}`},
		{ID: "4", Name: "tool", Arguments: `{}`},
		{ID: "5", Name: "tool", Arguments: `{}`},
	}

	results, err := executor.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 5)

	// Verify max concurrency was respected
	assert.LessOrEqual(t, maxConcurrent, 2, "Should not exceed max concurrency of 2")
	assert.Greater(t, maxConcurrent, 0, "Should have some concurrency")
}

// TestParallelExecutor_ErrorHandling tests error handling in parallel execution
func TestParallelExecutor_ErrorHandling(t *testing.T) {
	expectedErr := errors.New("tool error")
	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "result1", nil
			},
		},
		"tool2": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "", expectedErr
			},
		},
		"tool3": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "result3", nil
			},
		},
	}

	executor := NewParallelExecutor(registry)

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{}`},
		{ID: "2", Name: "tool2", Arguments: `{}`},
		{ID: "3", Name: "tool3", Arguments: `{}`},
	}

	results, err := executor.Execute(context.Background(), calls)
	assert.Error(t, err)
	assert.Len(t, results, 3)

	// Find the error in results
	var foundError bool
	for _, result := range results {
		if result.Error != nil {
			foundError = true
			assert.Equal(t, expectedErr, result.Error)
		}
	}
	assert.True(t, foundError, "Should have found an error in results")
}

// TestParallelExecutor_ContinueOnError tests continue-on-error with parallel execution
func TestParallelExecutor_ContinueOnError(t *testing.T) {
	expectedErr := errors.New("tool error")
	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "result1", nil
			},
		},
		"tool2": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "", expectedErr
			},
		},
		"tool3": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "result3", nil
			},
		},
	}

	executor := NewParallelExecutor(registry, WithContinueOnError(true))

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{}`},
		{ID: "2", Name: "tool2", Arguments: `{}`},
		{ID: "3", Name: "tool3", Arguments: `{}`},
	}

	results, err := executor.Execute(context.Background(), calls)
	require.NoError(t, err) // Should not return error when continuing
	require.Len(t, results, 3)

	// Count successes and failures
	var successCount, errorCount int
	for _, result := range results {
		if result.Error != nil {
			errorCount++
			assert.Equal(t, expectedErr, result.Error)
		} else {
			successCount++
		}
	}
	assert.Equal(t, 2, successCount)
	assert.Equal(t, 1, errorCount)
}

// TestNewExecutor_DefaultsToSequential tests that NewExecutor creates sequential executor
func TestNewExecutor_DefaultsToSequential(t *testing.T) {
	var executionOrder []string
	var mu sync.Mutex

	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				mu.Lock()
				executionOrder = append(executionOrder, "tool1")
				mu.Unlock()
				time.Sleep(5 * time.Millisecond)
				return "result1", nil
			},
		},
		"tool2": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				mu.Lock()
				executionOrder = append(executionOrder, "tool2")
				mu.Unlock()
				return "result2", nil
			},
		},
	}

	executor := NewExecutor(registry)

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{}`},
		{ID: "2", Name: "tool2", Arguments: `{}`},
	}

	results, err := executor.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Should execute in order (sequential)
	assert.Equal(t, []string{"tool1", "tool2"}, executionOrder)
}

// TestExecutor_ContextCancellation tests context cancellation handling
func TestExecutor_ContextCancellation(t *testing.T) {
	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return "result1", nil
				}
			},
		},
	}

	executor := NewSequentialExecutor(registry)

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{}`},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	results, err := executor.Execute(ctx, calls)
	assert.Error(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, context.Canceled, results[0].Error)
}

// TestExecutor_Duration tests that duration is tracked
func TestExecutor_Duration(t *testing.T) {
	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				time.Sleep(10 * time.Millisecond)
				return "result", nil
			},
		},
	}

	executor := NewSequentialExecutor(registry)

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{}`},
	}

	results, err := executor.Execute(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Greater(t, results[0].Duration, 10*time.Millisecond)
	assert.Less(t, results[0].Duration, 100*time.Millisecond)
}

// TestExecutor_InvalidJSON tests handling of invalid JSON arguments
func TestExecutor_InvalidJSON(t *testing.T) {
	registry := map[string]Tool{
		"tool1": &mockTool{},
	}

	executor := NewSequentialExecutor(registry)

	// Create a call with invalid JSON arguments
	calls := []Call{
		{
			ID:        "1",
			Name:      "tool1",
			Arguments: "not valid json{",
		},
	}

	results, err := executor.Execute(context.Background(), calls)
	// Tool receives invalid JSON string directly - no marshal error anymore
	assert.NoError(t, err)
	assert.Len(t, results, 1)
}

// TestExecutor_BeforeToolError tests error from BeforeTool plugin
func TestExecutor_BeforeToolError(t *testing.T) {
	pluginErr := errors.New("before tool error")
	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				t.Fatal("Tool should not be called when BeforeTool fails")
				return "", nil
			},
		},
	}

	pm := &mockPlugin{
		beforeToolFunc: func(ctx context.Context, name string, input any) error {
			return pluginErr
		},
	}

	ctx := WithPlugin(context.Background(), pm)
	executor := NewSequentialExecutor(registry)

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{}`},
	}

	results, err := executor.Execute(ctx, calls)
	assert.Error(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, pluginErr, results[0].Error)
}

// TestExecutor_AfterToolError tests error from AfterTool plugin
func TestExecutor_AfterToolError(t *testing.T) {
	pluginErr := errors.New("after tool error")
	registry := map[string]Tool{
		"tool1": &mockTool{
			callFunc: func(ctx context.Context, input string) (any, error) {
				return "result", nil
			},
		},
	}

	pm := &mockPlugin{
		afterToolFunc: func(ctx context.Context, name string, result any) error {
			assert.Equal(t, "result", result)
			return pluginErr
		},
	}

	ctx := WithPlugin(context.Background(), pm)
	executor := NewSequentialExecutor(registry)

	calls := []Call{
		{ID: "1", Name: "tool1", Arguments: `{}`},
	}

	results, err := executor.Execute(ctx, calls)
	assert.Error(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, pluginErr, results[0].Error)
}
