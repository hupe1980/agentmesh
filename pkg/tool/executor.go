// Package tool provides tool execution with lifecycle management.
//
// The Executor pattern separates tool execution from graph orchestration:
//   - Executor: Handles execution lifecycle (plugins, observability, error handling)
//   - Tool: Core tool logic (actual work)
//   - ToolNode: Graph orchestration (message extraction, routing)
//
// This separation enables:
//   - Reusable execution logic across different contexts
//   - Multiple execution strategies (sequential, parallel)
//   - Custom executor implementations (rate limiting, caching, batching)
//   - Clean testing boundaries
//   - Centralized observability and plugin handling
//
// Architecture:
//
//	┌─────────────┐
//	│  ToolNode   │  Graph layer: message extraction, routing
//	└──────┬──────┘
//	       │ delegates to
//	┌──────▼──────┐
//	│  Executor   │  Execution layer: lifecycle, parallelism, observability
//	└──────┬──────┘
//	       │ calls
//	┌──────▼──────┐
//	│    Tool     │  Core layer: actual work
//	└─────────────┘
//
// Execution Strategies:
//
//   - SequentialExecutor: Executes tools one by one in order
//     Use when tools have dependencies or side effects
//
//   - ParallelExecutor: Executes tools concurrently with optional concurrency limits
//     Use when tools are independent for better performance
//
// Arguments as JSON String:
//
// Tool arguments are passed as JSON strings (not maps) to eliminate wasteful
// marshal/unmarshal cycles. Arguments flow as JSON from LLM → Executor → Tool:
//
//	LLM generates: {"location": "Berlin", "unit": "celsius"}
//	    ↓
//	ToolCall.Arguments: "{\"location\": \"Berlin\", \"unit\": \"celsius\"}"
//	    ↓
//	Tool receives: "{\"location\": \"Berlin\", \"unit\": \"celsius\"}"
//
// This avoids: JSON string → map → JSON string → tool unmarshal
//
// Example (basic usage):
//
//	executor := tool.NewSequentialExecutor(registry)
//	calls := []tool.Call{{
//	    ID: "call_1",
//	    Name: "weather",
//	    Arguments: `{"location":"Berlin"}`,
//	}}
//	results, err := executor.Execute(ctx, calls)
//
// Example (parallel execution):
//
//	executor := tool.NewParallelExecutor(registry,
//	    tool.WithMaxConcurrency(5))
//
// Example (custom executor):
//
//	type CachedExecutor struct {
//	    wrapped tool.Executor
//	    cache   map[string]tool.ExecutionResult
//	}
//
//	func (e *CachedExecutor) Execute(ctx context.Context, calls []Call) ([]ExecutionResult, error) {
//	    // Implement caching logic wrapping e.wrapped
//	}
package tool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

type pluginKey struct{}

// WithPlugin adds a Plugin to the context for executor lifecycle hooks.
// This is typically used by the callbacks package to inject the PluginManager.
func WithPlugin(ctx context.Context, plugin Plugin) context.Context {
	return context.WithValue(ctx, pluginKey{}, plugin)
}

// Call represents a single tool invocation request.
//
// The Arguments field is a JSON string (not a map) to avoid wasteful
// marshal/unmarshal cycles. This design keeps arguments as JSON throughout
// the pipeline from LLM generation to tool execution.
//
// Example:
//
//	call := tool.Call{
//	    ID:        "call_123",
//	    Name:      "get_weather",
//	    Arguments: `{"location":"Berlin","unit":"celsius"}`,
//	}
type Call struct {
	ID        string // Unique identifier for this call
	Name      string // Tool name to execute
	Arguments string // Tool arguments as JSON string (not map[string]any)
}

// ExecutionResult contains the outcome of a tool execution.
type ExecutionResult struct {
	ToolCallID string        // ID of the tool call
	ToolName   string        // Name of the tool executed
	Result     any           // Tool result (nil if error)
	Error      error         // Execution error (nil if success)
	Duration   time.Duration // Execution time
}

// Executor handles the complete lifecycle of tool executions.
//
// This interface allows users to provide custom executor implementations
// for specialized behavior (e.g., rate limiting, caching, custom parallelism).
//
// Example custom implementations:
//   - RateLimitedExecutor: Wraps with rate limiting
//   - CachedExecutor: Caches deterministic tool results
//   - CircuitBreakerExecutor: Implements circuit breaker pattern
//   - BatchedExecutor: Batches multiple calls for efficiency
type Executor interface {
	// Execute runs one or more tool calls with full lifecycle.
	// Returns execution results for each tool call in the same order.
	// The executor handles plugins, observability, and error recovery.
	Execute(ctx context.Context, calls []Call) ([]ExecutionResult, error)
}

// SequentialExecutor executes tools one by one in order.
// This is the safest option when tools have dependencies or side effects.
type SequentialExecutor struct {
	registry        map[string]Tool
	continueOnError bool
	errorPrefix     string
}

// ParallelExecutor executes tools concurrently using goroutines.
// This provides better performance when tools are independent.
type ParallelExecutor struct {
	registry        map[string]Tool
	continueOnError bool
	errorPrefix     string
	maxConcurrency  int // 0 = unlimited
}

// ExecutorOption configures an executor.
type ExecutorOption func(any)

// WithContinueOnError configures error handling behavior.
// If true, execution continues even when individual tools fail.
// Errors are still returned in ExecutionResult.Error for each failed tool.
//
// Example:
//
//	executor := tool.NewSequentialExecutor(registry,
//	    tool.WithContinueOnError(true))
func WithContinueOnError(continueOnError bool) ExecutorOption {
	return func(e any) {
		switch executor := e.(type) {
		case *SequentialExecutor:
			executor.continueOnError = continueOnError
		case *ParallelExecutor:
			executor.continueOnError = continueOnError
		}
	}
}

// WithErrorPrefix sets the error message prefix.
// This prefix is added to all error messages from the executor.
//
// Example:
//
//	executor := tool.NewSequentialExecutor(registry,
//	    tool.WithErrorPrefix("my-agent"))
func WithErrorPrefix(prefix string) ExecutorOption {
	return func(e any) {
		switch executor := e.(type) {
		case *SequentialExecutor:
			executor.errorPrefix = prefix
		case *ParallelExecutor:
			executor.errorPrefix = prefix
		}
	}
}

// WithMaxConcurrency limits concurrent tool executions (ParallelExecutor only).
// A value of 0 means unlimited concurrency (default).
//
// Example:
//
//	executor := tool.NewParallelExecutor(registry,
//	    tool.WithMaxConcurrency(5)) // Max 5 concurrent tools
func WithMaxConcurrency(maxConcurrency int) ExecutorOption {
	return func(e any) {
		if executor, ok := e.(*ParallelExecutor); ok {
			executor.maxConcurrency = maxConcurrency
		}
	}
}

// NewSequentialExecutor creates a sequential tool executor.
// Tools are executed one by one in the order provided.
//
// Use this when:
//   - Tools have dependencies on each other
//   - Tools have side effects that must be ordered
//   - You want deterministic execution order
//
// Example:
//
//	executor := tool.NewSequentialExecutor(registry,
//	    tool.WithContinueOnError(false),
//	    tool.WithErrorPrefix("react-agent"))
func NewSequentialExecutor(registry map[string]Tool, opts ...ExecutorOption) Executor {
	executor := &SequentialExecutor{
		registry:        registry,
		continueOnError: false,
		errorPrefix:     "tool executor",
	}

	for _, opt := range opts {
		opt(executor)
	}

	return executor
}

// NewParallelExecutor creates a parallel tool executor.
// Tools are executed concurrently using goroutines.
//
// Use this when:
//   - Tools are independent of each other
//   - You want maximum performance
//   - Tools can safely run concurrently
//
// Example:
//
//	executor := tool.NewParallelExecutor(registry,
//	    tool.WithMaxConcurrency(10),
//	    tool.WithContinueOnError(true),
//	    tool.WithErrorPrefix("react-agent"))
func NewParallelExecutor(registry map[string]Tool, opts ...ExecutorOption) Executor {
	executor := &ParallelExecutor{
		registry:        registry,
		continueOnError: false,
		errorPrefix:     "tool executor",
		maxConcurrency:  0, // unlimited
	}

	for _, opt := range opts {
		opt(executor)
	}

	return executor
}

// NewExecutor creates a tool executor with the recommended default (sequential).
// For parallel execution, use NewParallelExecutor explicitly.
//
// Example:
//
//	executor := tool.NewExecutor(registry,
//	    tool.WithErrorPrefix("my-agent"))
func NewExecutor(registry map[string]Tool, opts ...ExecutorOption) Executor {
	return NewSequentialExecutor(registry, opts...)
}

// Execute implements Executor for SequentialExecutor.
func (e *SequentialExecutor) Execute(ctx context.Context, calls []Call) ([]ExecutionResult, error) {
	results := make([]ExecutionResult, 0, len(calls))

	for _, call := range calls {
		result := executeSingleTool(ctx, call, e.registry, e.errorPrefix)
		results = append(results, result)

		if result.Error != nil && !e.continueOnError {
			return results, result.Error
		}
	}

	return results, nil
}

// Execute implements Executor for ParallelExecutor.
func (e *ParallelExecutor) Execute(ctx context.Context, calls []Call) ([]ExecutionResult, error) {
	results := make([]ExecutionResult, len(calls))
	errors := make([]error, len(calls))

	// Limit concurrency if configured
	if e.maxConcurrency > 0 {
		sem := make(chan struct{}, e.maxConcurrency)
		var wg sync.WaitGroup

		for i, call := range calls {
			wg.Add(1)
			go func(idx int, c Call) {
				defer wg.Done()
				sem <- struct{}{}        // Acquire
				defer func() { <-sem }() // Release

				results[idx] = executeSingleTool(ctx, c, e.registry, e.errorPrefix)
				if results[idx].Error != nil && !e.continueOnError {
					errors[idx] = results[idx].Error
				}
			}(i, call)
		}
		wg.Wait()
	} else {
		// Unlimited concurrency
		var wg sync.WaitGroup
		for i, call := range calls {
			wg.Add(1)
			go func(idx int, c Call) {
				defer wg.Done()
				results[idx] = executeSingleTool(ctx, c, e.registry, e.errorPrefix)
				if results[idx].Error != nil && !e.continueOnError {
					errors[idx] = results[idx].Error
				}
			}(i, call)
		}
		wg.Wait()
	}

	// Check for errors if not continuing
	if !e.continueOnError {
		for _, err := range errors {
			if err != nil {
				return results, err
			}
		}
	}

	return results, nil
}

// executeSingleTool is a shared helper used by all executor implementations.
// It executes a single tool call with full lifecycle management including
// plugins, observability, and error handling.
func executeSingleTool(ctx context.Context, call Call, registry map[string]Tool, errorPrefix string) ExecutionResult {
	result := ExecutionResult{
		ToolCallID: call.ID,
		ToolName:   call.Name,
	}

	// 1. Execute BeforeTool plugin
	pm, _ := ctx.Value(pluginKey{}).(Plugin)
	if pm != nil {
		if err := pm.ExecuteBeforeTool(ctx, call.Name, call.Arguments); err != nil {
			result.Error = err
			return result
		}
	}

	// 2. Get tool from registry
	t := registry[call.Name]
	if t == nil {
		result.Error = fmt.Errorf("%s: tool %q not registered", errorPrefix, call.Name)
		return result
	}

	// 3. Start observability
	tp := trace.FromContext(ctx)
	tracer := tp.Tracer("agentmesh.tool")
	ctx, span := tracer.Start(ctx, "tool.execute",
		trace.Attr{Key: "tool.name", Value: call.Name},
		trace.Attr{Key: "tool.id", Value: call.ID})
	var toolErr error
	defer func() {
		span.End(toolErr)
	}()

	logger := logging.FromContext(ctx)
	logger.Debug("tool execution starting", "tool", call.Name, "tool_id", call.ID)

	mp := metrics.FromContext(ctx)
	startTime := time.Now()
	counter := mp.Counter("tool.executions")
	counter.Add(ctx, 1, metrics.Attr{Key: "tool", Value: call.Name})

	// 4. Execute tool
	toolResult, err := t.Call(ctx, call.Arguments)
	result.Duration = time.Since(startTime)

	// 5. Record metrics
	histogram := mp.Histogram("tool.duration_ms")
	histogram.Record(ctx, float64(result.Duration.Milliseconds()),
		metrics.Attr{Key: "tool", Value: call.Name})

	if err != nil {
		toolErr = err
		// 6a. Handle error
		errorCounter := mp.Counter("tool.errors")
		errorCounter.Add(ctx, 1, metrics.Attr{Key: "tool", Value: call.Name})
		logger.Error("tool execution failed", "tool", call.Name, "error", err, "duration_ms", result.Duration.Milliseconds())

		if pm != nil {
			transformedErr := pm.ExecuteOnToolError(ctx, call.Name, err)
			if transformedErr != nil {
				err = transformedErr
			}
		}
		result.Error = err
		return result
	}

	// 6b. Success
	logger.Debug("tool execution completed", "tool", call.Name, "duration_ms", result.Duration.Milliseconds())

	// 7. Execute AfterTool plugin
	if pm != nil {
		if err := pm.ExecuteAfterTool(ctx, call.Name, toolResult); err != nil {
			toolErr = err
			result.Error = err
			return result
		}
	}

	result.Result = toolResult
	return result
}
