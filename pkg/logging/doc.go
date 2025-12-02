/*
Package logging provides a minimal logging interface and adapters for AgentMesh.

# Overview

The logging package defines a standard Logger interface for observability across
graph execution, agent workflows, and tool invocations. Implementations include:
  - SlogLogger: Adapter for Go's structured logging (slog)
  - NoopLogger: Silent logger for testing or minimal setups

# Quick Start

Using structured logging:

	import "github.com/hupe1980/agentmesh/pkg/logging"

	logger := logging.NewSlogLogger(
		logging.LogLevelInfo,
		logging.LogFormatJSON,
		false, // disable color
	)

	logger.Info("Graph execution started", "runID", "abc123")
	logger.Debug("Node completed", "node", "agent", "duration", "12ms")

# Logger Interface

The minimal interface supports standard log levels:

	type Logger interface {
		Debug(msg string, keysAndValues ...any)
		Info(msg string, keysAndValues ...any)
		Warn(msg string, keysAndValues ...any)
		Error(msg string, keysAndValues ...any)
		Fatal(msg string, keysAndValues ...any)
	}

# Integration with Graphs

Pass logger to instrumentation:

	import "github.com/hupe1980/agentmesh/pkg/graph"

	compiled, _ := g.Build(
		graph.WithInstrumentation(&graph.Instrumentation{
			Logger: logger,
		}),
	)

# Log Levels

Configure verbosity:
  - LogLevelDebug: Detailed execution traces
  - LogLevelInfo: High-level workflow progress
  - LogLevelWarn: Recoverable issues
  - LogLevelError: Failures requiring attention
  - LogLevelFatal: Unrecoverable errors (exits process)

# Custom Loggers

Implement the Logger interface for custom backends:

	type CustomLogger struct {
		backend SomeLoggingService
	}

	func (l *CustomLogger) Info(msg string, keysAndValues ...any) {
		l.backend.Log("INFO", msg, keysAndValues...)
	}
	// ... implement other methods

# Design Philosophy

The interface intentionally stays minimal to:
  - Avoid vendor lock-in to specific logging frameworks
  - Support structured logging where available
  - Enable easy testing with NoopLogger
  - Keep dependencies lightweight
*/
package logging
