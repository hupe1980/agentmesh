package middleware

import (
	"context"
	"iter"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/logging"
)

// LoggingMiddleware adds structured logging to graph execution.
type LoggingMiddleware[I, O any] struct {
	logger logging.Logger
}

// NewLoggingMiddleware creates a logging middleware with the given logger.
// If logger is nil, it uses the logger from context.
func NewLoggingMiddleware[I, O any](logger logging.Logger) *LoggingMiddleware[I, O] {
	return &LoggingMiddleware[I, O]{
		logger: logger,
	}
}

// Wrap implements graph.Middleware.
func (m *LoggingMiddleware[I, O]) Wrap(next graph.Executor[I, O]) graph.Executor[I, O] {
	return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[I, O], input I, opts ...graph.RunOption) iter.Seq2[O, error] {
		start := time.Now()

		// Use provided logger or get from context
		logger := m.logger
		if logger == nil {
			logger = logging.FromContext(ctx)
		}

		logger.Info("Graph execution started")

		// Execute
		results := next.Run(ctx, compiled, input, opts...)

		// Wrap iterator to log completion
		return func(yield func(O, error) bool) {
			resultCount := 0
			hasError := false
			stoppedEarly := false

			for output, err := range results {
				if err != nil {
					hasError = true
					logger.Error("Graph execution error", "error", err.Error())
				}
				resultCount++

				if !yield(output, err) {
					stoppedEarly = true
					break // IMPORTANT: break instead of return to allow cleanup
				}
			}

			duration := time.Since(start)
			if stoppedEarly {
				logger.Info("Graph execution stopped by consumer",
					"duration", duration,
					"results", resultCount)
				return
			}

			if hasError {
				logger.Warn("Graph execution completed with errors",
					"duration", duration,
					"results", resultCount)
			} else {
				logger.Info("Graph execution completed successfully",
					"duration", duration,
					"results", resultCount)
			}
		}
	})
}
