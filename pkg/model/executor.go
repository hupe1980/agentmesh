// Package model provides model execution with lifecycle management.
//
// The Executor pattern separates model execution from graph orchestration:
//   - Executor: Handles execution lifecycle (plugins, observability, error handling)
//   - Model: Core generation logic (API calls, streaming)
//   - ModelNode: Graph orchestration (state extraction, routing)
//
// This separation enables:
//   - Reusable execution logic across different contexts (graphs, chains, direct calls)
//   - Custom executor implementations (retry, caching, rate limiting)
//   - Clean testing boundaries (test execution independent of graph/state)
//   - Centralized observability and plugin handling
//
// Architecture:
//
//	┌─────────────┐
//	│  ModelNode  │  Graph layer: state extraction, routing
//	└──────┬──────┘
//	       │ delegates to
//	┌──────▼──────┐
//	│  Executor   │  Execution layer: lifecycle, plugins, observability
//	└──────┬──────┘
//	       │ calls
//	┌──────▼──────┐
//	│    Model    │  Core layer: API calls, streaming
//	└─────────────┘
//
// Example (basic usage):
//
//	executor := model.NewExecutor(openaiModel)
//	resp, err := model.Last(executor.Generate(ctx, req))
//
// Example (streaming):
//
//	for resp, err := range executor.Generate(ctx, req) {
//	    if err != nil { return err }
//	    fmt.Print(resp.Message.Content())
//	}
//
// Example (custom executor):
//
//	type RetryExecutor struct {
//	    wrapped    model.Executor
//	    maxRetries int
//	}
//
//	func (e *RetryExecutor) Generate(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
//	    // Implement retry logic wrapping e.wrapped
//	}
package model

import (
	"context"
	"iter"
	"time"

	"github.com/hupe1980/agentmesh/pkg/event"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

// Executor handles the complete lifecycle of model generation requests.
// It wraps a Model with observability, plugin support, and error handling.
//
// This interface allows users to provide custom executor implementations
// for specialized behavior while maintaining consistent lifecycle management.
//
// Example custom implementations:
//   - RetryExecutor: Adds retry logic with exponential backoff
//   - CachedExecutor: Caches responses for deterministic requests
//   - RateLimitedExecutor: Enforces rate limiting
//   - CircuitBreakerExecutor: Implements circuit breaker pattern
type Executor interface {
	// Generate executes a model generation with full lifecycle management.
	// It handles plugins, observability, and error recovery automatically.
	// Returns an iterator that yields responses. For non-streaming requests,
	// a single response is yielded. For streaming requests, incremental
	// responses are yielded as they become available.
	//
	// The iterator pattern (iter.Seq2) provides a unified interface for both
	// streaming and non-streaming execution, allowing consumers to use the
	// same code path regardless of Request.Stream setting.
	//
	// Example (non-streaming):
	//   resp, err := model.Last(executor.Generate(ctx, req))
	//
	// Example (streaming):
	//   for resp, err := range executor.Generate(ctx, req) {
	//       if err != nil { return err }
	//       // Process incremental response
	//   }
	Generate(ctx context.Context, req *Request) iter.Seq2[*Response, error]
}

// DefaultExecutor is the standard implementation of Executor.
// It provides full lifecycle management with observability and plugins.
type DefaultExecutor struct {
	model Model
	name  string // For observability labels
}

// ExecutorOption configures a DefaultExecutor.
type ExecutorOption func(*DefaultExecutor)

// WithExecutorName sets the executor name for observability.
// The name is used in traces, logs, and metrics to identify this executor.
//
// Example:
//
//	executor := model.NewExecutor(myModel,
//	    model.WithExecutorName("my-agent-model"))
func WithExecutorName(name string) ExecutorOption {
	return func(e *DefaultExecutor) {
		e.name = name
	}
}

// NewExecutor creates a new default model executor.
// Returns an Executor interface for maximum flexibility.
//
// The executor wraps the model with:
//   - Observability (tracing, metrics, logging)
//   - Error handling and recovery
//   - Middleware support via Chain()
//
// Example:
//
//	executor := model.NewExecutor(myModel,
//	    model.WithExecutorName("react-model"))
//	resp, err := model.Last(executor.Generate(ctx, req))
func NewExecutor(mdl Model, opts ...ExecutorOption) Executor {
	executor := &DefaultExecutor{
		model: mdl,
		name:  "model",
	}

	for _, opt := range opts {
		opt(executor)
	}

	return executor
}

// handleGenerationError handles errors during model generation.
// It records metrics, logs errors, and invokes plugin error handlers.
func (e *DefaultExecutor) handleGenerationError(
	ctx context.Context,
	_ *Request,
	err error,
	startTime time.Time,
	yield func(*Response, error) bool,
	spanErr *error,
) {
	*spanErr = err

	// Record error metrics
	mp := metrics.FromContext(ctx)
	logger := logging.FromContext(ctx)

	duration := time.Since(startTime)
	histogram := mp.Histogram("model.duration_ms")
	histogram.Record(ctx, float64(duration.Milliseconds()),
		metrics.Attr{Key: "model", Value: e.name})

	errorCounter := mp.Counter("model.errors")
	errorCounter.Add(ctx, 1, metrics.Attr{Key: "model", Value: e.name})
	logger.Error("model generation failed", "model", e.name, "error", err, "duration_ms", duration.Milliseconds())

	// Publish model error event
	event.Publish(ctx, event.Event{
		Type:      event.EventModelError,
		Timestamp: time.Now(),
		Data: map[string]any{
			"model":       e.name,
			"duration_ms": duration.Milliseconds(),
		},
		Error: err.Error(),
	})

	yield(nil, err)
}

// Generate executes a model generation with full lifecycle management.
// See Executor.Generate interface documentation for details.
func (e *DefaultExecutor) Generate(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	return func(yield func(*Response, error) bool) {
		// 1. Start observability span
		tp := trace.FromContext(ctx)
		tracer := tp.Tracer("agentmesh.model")
		ctx, span := tracer.Start(ctx, "model.generate",
			trace.Attr{Key: "model.name", Value: e.name},
			trace.Attr{Key: "model.messages", Value: len(req.Messages)})
		var spanErr error
		defer func() {
			span.End(spanErr)
		}()

		// 3. Log start
		logger := logging.FromContext(ctx)
		logger.Debug("model generation starting", "model", e.name, "messages", len(req.Messages))

		// 4. Record metrics - start
		mp := metrics.FromContext(ctx)
		startTime := time.Now()
		counter := mp.Counter("model.requests")
		counter.Add(ctx, 1, metrics.Attr{Key: "model", Value: e.name})

		// 4b. Publish model start event
		event.Publish(ctx, event.Event{
			Type:      event.EventModelStart,
			Timestamp: startTime,
			Data: map[string]any{
				"model":    e.name,
				"messages": len(req.Messages),
				"tools":    len(req.Tools),
			},
		})

		// 5. Call underlying model and process responses
		hasResponse := false
		var lastResp *Response
		for resp, err := range e.model.Generate(ctx, req) {
			if err != nil {
				e.handleGenerationError(ctx, req, err, startTime, yield, &spanErr)
				return
			}

			hasResponse = true
			lastResp = resp

			// 6b. Record success metrics (per response chunk)
			if resp.Usage != nil {
				tokensUsed := mp.Counter("model.tokens_used")
				tokensUsed.Add(ctx, float64(resp.Usage.TotalTokens),
					metrics.Attr{Key: "model", Value: e.name},
					metrics.Attr{Key: "type", Value: "total"})
			}

			// 7. Yield response to consumer
			if !yield(resp, nil) {
				return // Consumer stopped iteration
			}
		}

		if !hasResponse {
			// No responses were generated
			spanErr = ErrNoResponse
			yield(nil, ErrNoResponse)
			return
		}

		// 8. Record final metrics after all responses
		duration := time.Since(startTime)
		histogram := mp.Histogram("model.duration_ms")
		histogram.Record(ctx, float64(duration.Milliseconds()),
			metrics.Attr{Key: "model", Value: e.name})
		logger.Debug("model generation completed", "model", e.name, "duration_ms", duration.Milliseconds())

		// 8b. Publish model complete event
		eventData := map[string]any{
			"model":       e.name,
			"duration_ms": duration.Milliseconds(),
		}
		if lastResp != nil {
			if lastResp.Usage != nil {
				eventData["usage"] = map[string]any{
					"prompt_tokens":     lastResp.Usage.PromptTokens,
					"completion_tokens": lastResp.Usage.CompletionTokens,
					"total_tokens":      lastResp.Usage.TotalTokens,
				}
			}
			if lastResp.FinishReason != "" {
				eventData["finish_reason"] = lastResp.FinishReason
			}
		}
		event.Publish(ctx, event.Event{
			Type:      event.EventModelComplete,
			Timestamp: time.Now(),
			Data:      eventData,
		})
	}
}
