// Package logging provides a tiny abstraction over slog so downstream code can
// depend on a minimal interface (Logger) while allowing users to plug any
// structured logger.
package logging

import "context"

// Logger defines the minimal logging interface for AgentMesh.
// This allows users to provide their own logger implementation or use the built-in adapters.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	// With returns a child logger enriched with structured attributes.
	// Args are interpreted as structured attributes (key/value pairs).
	With(args ...any) Logger
}

// ---- Context helpers for logger propagation ----

// loggerCtxKey is an unexported unique key type to avoid collisions.
type loggerCtxKey struct{}

var _loggerKey = loggerCtxKey{}

// WithLogger attaches the provided logger to the context. Nil becomes NoopLogger.
func WithLogger(ctx context.Context, l Logger) context.Context {
	if l == nil {
		l = NoopLogger{}
	}
	return context.WithValue(ctx, _loggerKey, l)
}

// FromContext retrieves a logger from context or returns a no-op logger.
func FromContext(ctx context.Context) Logger {
	if v := ctx.Value(_loggerKey); v != nil {
		if l, ok := v.(Logger); ok && l != nil {
			return l
		}
	}
	return NoopLogger{}
}

// LogLevel represents different logging levels.
// LogLevel is a thin enum for user friendly level configuration decoupled from slog.
type LogLevel int

const (
	// LogLevelDebug is the debug logging level.
	LogLevelDebug LogLevel = iota
	// LogLevelInfo is the informational logging level.
	LogLevelInfo
	// LogLevelWarn is the warning logging level.
	LogLevelWarn
	// LogLevelError is the error logging level.
	LogLevelError
)

// String returns the string representation of the log level.
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}
