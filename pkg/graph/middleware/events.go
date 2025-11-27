package middleware

import (
	"context"
	"iter"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// EventMiddleware publishes execution events to the event bus.
// This middleware requires an event bus to be attached to the context via graph.WithEventBus().
type EventMiddleware[I, O any] struct {
	runIDFunc func() string // Optional function to generate custom run IDs
}

// NewEventMiddleware creates an event publishing middleware.
func NewEventMiddleware[I, O any]() *EventMiddleware[I, O] {
	return &EventMiddleware[I, O]{}
}

// WithRunIDFunc sets a custom run ID generator.
func (m *EventMiddleware[I, O]) WithRunIDFunc(fn func() string) *EventMiddleware[I, O] {
	m.runIDFunc = fn
	return m
}

// Wrap implements graph.Middleware.
func (m *EventMiddleware[I, O]) Wrap(next graph.Executor[I, O]) graph.Executor[I, O] {
	return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[I, O], input I, opts ...graph.RunOption) iter.Seq2[O, error] {
		// Generate run ID
		runID := "run-" + time.Now().Format("20060102-150405")
		if m.runIDFunc != nil {
			runID = m.runIDFunc()
		}

		start := time.Now()

		// Publish graph start event
		graph.Publish(ctx, graph.Event{
			Type:      graph.EventGraphStart,
			Timestamp: start,
			RunID:     runID,
		})

		// Execute
		results := next.Run(ctx, compiled, input, opts...)

		// Wrap iterator to publish completion
		return func(yield func(O, error) bool) {
			hasError := false
			stoppedEarly := false

			for output, err := range results {
				if err != nil {
					hasError = true
				}

				if !yield(output, err) {
					stoppedEarly = true
					break // IMPORTANT: break instead of return to allow cleanup
				}
			}

			// Handle early termination
			if stoppedEarly {
				graph.Publish(ctx, graph.Event{
					Type:      graph.EventGraphComplete,
					Timestamp: time.Now(),
					RunID:     runID,
					Duration:  time.Since(start),
					Data: map[string]any{
						"stopped_by_consumer": true,
					},
				})
				return
			}

			// Publish completion event
			if hasError {
				graph.Publish(ctx, graph.Event{
					Type:      graph.EventGraphError,
					Timestamp: time.Now(),
					RunID:     runID,
					Duration:  time.Since(start),
				})
			} else {
				graph.Publish(ctx, graph.Event{
					Type:      graph.EventGraphComplete,
					Timestamp: time.Now(),
					RunID:     runID,
					Duration:  time.Since(start),
				})
			}
		}
	})
}
