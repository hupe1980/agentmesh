// Package tool provides tool execution with lifecycle management.
//
// The Executor pattern separates tool execution from graph orchestration:
//   - Executor: Handles execution lifecycle (observability, error handling)
//   - Tool: Core tool logic (actual work)
//   - ToolNode: Graph orchestration (message extraction, routing)
//
// This separation enables:
//   - Reusable execution logic across different contexts
//   - Multiple execution strategies (sequential, parallel)
//   - Custom executor implementations (rate limiting, caching, batching)
//   - Clean testing boundaries
//   - Centralized observability handling
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

	"github.com/hupe1980/agentmesh/pkg/event"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

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
	// The executor handles observability and error recovery.
	Execute(ctx context.Context, calls []Call) ([]ExecutionResult, error)
}

// executorConfig holds common configuration shared by all executor implementations.
// This struct is embedded in concrete executor types to avoid code duplication
// and provide consistent configuration options across executor variants.
type executorConfig struct {
	registry        map[string]Tool
	continueOnError bool
	errorPrefix     string
}

// defaultExecutorConfig returns an executorConfig with sensible defaults.
func defaultExecutorConfig(registry map[string]Tool) executorConfig {
	return executorConfig{
		registry:        registry,
		continueOnError: false,
		errorPrefix:     "tool executor",
	}
}

// SequentialExecutor executes tools one by one in order.
// This is the safest option when tools have dependencies or side effects.
type SequentialExecutor struct {
	executorConfig
}

// ParallelExecutor executes tools concurrently using goroutines.
// This provides better performance when tools are independent.
type ParallelExecutor struct {
	executorConfig
	maxConcurrency int // 0 = unlimited
}

// ExecutorOption configures an executor.
//
// This interface-based design (rather than function types) provides:
//   - Full compile-time type safety for both SequentialExecutor and ParallelExecutor
//   - Shared options that work with multiple executor types via sharedExecutorOption
//   - No runtime type switches or silent failures from invalid option types
//
// Options work with executorConfig to provide consistent behavior across executor variants.
type ExecutorOption interface {
	applyExecutor(*executorConfig)
}

// SharedExecutorOption implements both ExecutorOption and ParallelExecutorOption interfaces.
//
// This allows common options (like WithContinueOnError, WithErrorPrefix) to work with
// both sequential and parallel executors without code duplication or type prefixes.
//
// The pattern uses embedded executorConfig to achieve this:
//   - Both SequentialExecutor and ParallelExecutor embed executorConfig
//   - SharedExecutorOption modifies the embedded executorConfig fields
//   - Interface methods delegate to the embedded struct
//
// Example usage:
//
//	// Same option works for both executor types
//	seq := tool.NewSequentialExecutor(registry, tool.WithErrorPrefix("agent"))
//	par := tool.NewParallelExecutor(registry, tool.WithErrorPrefix("agent"))
type SharedExecutorOption func(*executorConfig)

// Implement ExecutorOption interface
func (s SharedExecutorOption) applyExecutor(cfg *executorConfig) {
	s(cfg)
}

// WithContinueOnError configures error handling behavior.
// If true, execution continues even when individual tools fail.
// Errors are still returned in ExecutionResult.Error for each failed tool.
//
// Works with both SequentialExecutor and ParallelExecutor.
//
// Example:
//
//	executor := tool.NewSequentialExecutor(registry,
//	    tool.WithContinueOnError(true))
func WithContinueOnError(continueOnError bool) SharedExecutorOption {
	return func(cfg *executorConfig) {
		cfg.continueOnError = continueOnError
	}
}

// WithErrorPrefix sets the error message prefix.
// This prefix is added to all error messages from the executor.
//
// Works with both SequentialExecutor and ParallelExecutor.
//
// Example:
//
//	executor := tool.NewSequentialExecutor(registry,
//	    tool.WithErrorPrefix("my-agent"))
func WithErrorPrefix(prefix string) SharedExecutorOption {
	return func(cfg *executorConfig) {
		cfg.errorPrefix = prefix
	}
}

// ParallelExecutorOption configures a ParallelExecutor.
// These options are specific to parallel execution and don't apply to sequential executors.
type ParallelExecutorOption interface {
	applyParallelExecutor(*ParallelExecutor)
}

// parallelExecutorOptionFunc wraps a function to implement ParallelExecutorOption.
type parallelExecutorOptionFunc func(*ParallelExecutor)

func (f parallelExecutorOptionFunc) applyParallelExecutor(e *ParallelExecutor) {
	f(e)
}

// Implement ParallelExecutorOption interface for SharedExecutorOption
func (s SharedExecutorOption) applyParallelExecutor(e *ParallelExecutor) {
	s(&e.executorConfig)
}

// WithMaxConcurrency limits concurrent tool executions.
// A value of 0 means unlimited concurrency (default).
//
// Example:
//
//	executor := tool.NewParallelExecutor(registry,
//	    tool.WithMaxConcurrency(5)) // Max 5 concurrent tools
func WithMaxConcurrency(maxConcurrency int) ParallelExecutorOption {
	return parallelExecutorOptionFunc(func(e *ParallelExecutor) {
		e.maxConcurrency = maxConcurrency
	})
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
	cfg := defaultExecutorConfig(registry)
	for _, opt := range opts {
		opt.applyExecutor(&cfg)
	}

	return &SequentialExecutor{
		executorConfig: cfg,
	}
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
//	    tool.WithContinueOnError(true),
//	    tool.WithMaxConcurrency(10))
func NewParallelExecutor(registry map[string]Tool, opts ...ParallelExecutorOption) *ParallelExecutor {
	cfg := defaultExecutorConfig(registry)

	e := &ParallelExecutor{
		executorConfig: cfg,
		maxConcurrency: 0, // unlimited
	}

	for _, opt := range opts {
		opt.applyParallelExecutor(e)
	}

	return e
}

// WithParallelOptions applies ParallelExecutor-specific options.
// This method is provided for compatibility but is not required -
// ParallelExecutorOption can be passed directly to NewParallelExecutor.
//
// Example:
//
//	executor := tool.NewParallelExecutor(registry,
//	    tool.WithMaxConcurrency(5))
func (e *ParallelExecutor) WithParallelOptions(opts ...ParallelExecutorOption) *ParallelExecutor {
	for _, opt := range opts {
		opt.applyParallelExecutor(e)
	}
	return e
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

	// Execute tools with or without concurrency limit
	if e.maxConcurrency > 0 {
		e.executeLimited(ctx, calls, results, errors)
	} else {
		e.executeUnlimited(ctx, calls, results, errors)
	}

	// Check for errors if not continuing
	return e.checkErrors(results, errors)
}

// executeLimited runs tools with a concurrency limit.
func (e *ParallelExecutor) executeLimited(ctx context.Context, calls []Call, results []ExecutionResult, errors []error) {
	sem := make(chan struct{}, e.maxConcurrency)
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c Call) {
			defer wg.Done()

			// Check context before acquiring semaphore
			if ctx.Err() != nil {
				results[idx] = ExecutionResult{Error: ctx.Err()}
				return
			}

			select {
			case sem <- struct{}{}: // Acquire
				defer func() { <-sem }() // Release
			case <-ctx.Done():
				results[idx] = ExecutionResult{Error: ctx.Err()}
				return
			}

			e.executeOne(ctx, idx, c, results, errors)
		}(i, call)
	}
	wg.Wait()
}

// executeUnlimited runs tools without concurrency limits.
func (e *ParallelExecutor) executeUnlimited(ctx context.Context, calls []Call, results []ExecutionResult, errors []error) {
	var wg sync.WaitGroup
	for i, call := range calls {
		// Check context before starting goroutine
		if ctx.Err() != nil {
			results[i] = ExecutionResult{Error: ctx.Err()}
			continue
		}

		wg.Add(1)
		go func(idx int, c Call) {
			defer wg.Done()

			// Double-check context inside goroutine
			if ctx.Err() != nil {
				results[idx] = ExecutionResult{Error: ctx.Err()}
				return
			}

			e.executeOne(ctx, idx, c, results, errors)
		}(i, call)
	}
	wg.Wait()
}

// executeOne executes a single tool call.
func (e *ParallelExecutor) executeOne(ctx context.Context, idx int, call Call, results []ExecutionResult, errors []error) {
	results[idx] = executeSingleTool(ctx, call, e.registry, e.errorPrefix)
	if results[idx].Error != nil && !e.continueOnError {
		errors[idx] = results[idx].Error
	}
}

// checkErrors returns an error if any tool failed and continueOnError is false.
func (e *ParallelExecutor) checkErrors(results []ExecutionResult, errors []error) ([]ExecutionResult, error) {
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
// observability and error handling.
func executeSingleTool(ctx context.Context, call Call, registry map[string]Tool, errorPrefix string) ExecutionResult {
	result := ExecutionResult{
		ToolCallID: call.ID,
		ToolName:   call.Name,
	}

	// 1. Get tool from registry
	t := registry[call.Name]
	if t == nil {
		result.Error = fmt.Errorf("%s: tool %q not registered", errorPrefix, call.Name)
		return result
	}

	// 2. Start observability
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

	// 2b. Publish tool start event
	event.Publish(ctx, event.Event{
		Type:      event.EventToolStart,
		Timestamp: startTime,
		Data: map[string]any{
			"tool":      call.Name,
			"tool_id":   call.ID,
			"arguments": call.Arguments,
		},
	})

	// 3. Execute tool
	toolResult, err := t.Call(ctx, call.Arguments)
	result.Duration = time.Since(startTime)

	// 4. Record metrics
	histogram := mp.Histogram("tool.duration_ms")
	histogram.Record(ctx, float64(result.Duration.Milliseconds()),
		metrics.Attr{Key: "tool", Value: call.Name})

	if err != nil {
		toolErr = err
		// 5a. Handle error
		errorCounter := mp.Counter("tool.errors")
		errorCounter.Add(ctx, 1, metrics.Attr{Key: "tool", Value: call.Name})
		logger.Error("tool execution failed", "tool", call.Name, "error", err, "duration_ms", result.Duration.Milliseconds())

		// Publish tool error event
		event.Publish(ctx, event.Event{
			Type:      event.EventToolError,
			Timestamp: time.Now(),
			Data: map[string]any{
				"tool":        call.Name,
				"tool_id":     call.ID,
				"duration_ms": result.Duration.Milliseconds(),
			},
			Error: err.Error(),
		})

		result.Error = err
		return result
	}

	// 5b. Success
	logger.Debug("tool execution completed", "tool", call.Name, "duration_ms", result.Duration.Milliseconds())

	// Publish tool complete event
	event.Publish(ctx, event.Event{
		Type:      event.EventToolComplete,
		Timestamp: time.Now(),
		Data: map[string]any{
			"tool":        call.Name,
			"tool_id":     call.ID,
			"duration_ms": result.Duration.Milliseconds(),
		},
	})

	result.Result = toolResult
	return result
}
