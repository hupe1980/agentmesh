// Package logging provides a tiny abstraction over slog so downstream code can
// depend on a minimal interface (Logger) while allowing users to plug any
// structured logger. It also offers a richer AgentMeshLogger with contextual
// helpers (session, invocation, component) and domain specific logging helpers
// for tools, models and flows.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"time"
)

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

// Logger defines the minimal logging interface for AgentMesh.
// This allows users to provide their own logger implementation or use the built-in adapters.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// SlogAdapter wraps *slog.Logger to implement the Logger interface.
type SlogAdapter struct {
	*slog.Logger
}

// Debug logs a debug message.
func (s *SlogAdapter) Debug(msg string, args ...any) { s.Logger.Debug(msg, args...) }

// Info logs an informational message.
func (s *SlogAdapter) Info(msg string, args ...any) { s.Logger.Info(msg, args...) }

// Warn logs a warning message.
func (s *SlogAdapter) Warn(msg string, args ...any) { s.Logger.Warn(msg, args...) }

// Error logs an error message.
func (s *SlogAdapter) Error(msg string, args ...any) { s.Logger.Error(msg, args...) }

// NewSlogAdapter creates a Logger from *slog.Logger.
func NewSlogAdapter(logger *slog.Logger) Logger {
	return &SlogAdapter{Logger: logger}
}

// NewDefaultSlogLogger creates a Logger using slog.Default().
func NewDefaultSlogLogger() Logger {
	return NewSlogAdapter(slog.Default())
}

// AgentMeshLogger wraps slog.Logger adding contextual cloning helpers and
// domain convenience methods. It should be cheap to copy via With* methods.
type AgentMeshLogger struct {
	logger       *slog.Logger
	level        LogLevel
	context      map[string]interface{}
	component    string
	sessionID    string
	invocationID string
}

// LoggerConfig configures construction of an AgentMeshLogger.
type LoggerConfig struct {
	Level        LogLevel
	Format       string // json or text
	Output       io.Writer
	AddSource    bool
	Component    string
	SessionID    string
	InvocationID string
	CustomAttrs  map[string]interface{}
}

// DefaultLoggerConfig returns a baseline JSON info level configuration.
func DefaultLoggerConfig() *LoggerConfig {
	return &LoggerConfig{Level: LogLevelInfo, Format: "json", Output: os.Stdout, AddSource: true, CustomAttrs: map[string]interface{}{}}
}

// NewLogger builds an AgentMeshLogger from a config (or defaults if nil).
func NewLogger(cfg *LoggerConfig) *AgentMeshLogger {
	if cfg == nil {
		cfg = DefaultLoggerConfig()
	}
	opts := &slog.HandlerOptions{Level: slogLevel(cfg.Level), AddSource: cfg.AddSource}
	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(cfg.Output, opts)
	} else {
		handler = slog.NewJSONHandler(cfg.Output, opts)
	}
	return &AgentMeshLogger{logger: slog.New(handler), level: cfg.Level, context: map[string]interface{}{}, component: cfg.Component, sessionID: cfg.SessionID, invocationID: cfg.InvocationID}
}

func slogLevel(l LogLevel) slog.Level {
	switch l {
	case LogLevelDebug:
		return slog.LevelDebug
	case LogLevelInfo:
		return slog.LevelInfo
	case LogLevelWarn:
		return slog.LevelWarn
	case LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (l *AgentMeshLogger) clone() *AgentMeshLogger {
	nl := *l
	nl.context = map[string]interface{}{}
	for k, v := range l.context {
		nl.context[k] = v
	}
	return &nl
}

// WithContext adds a key/value attribute that will be attached to every log entry.
func (l *AgentMeshLogger) WithContext(key string, value interface{}) *AgentMeshLogger {
	nl := l.clone()
	nl.context[key] = value
	return nl
}

// WithComponent sets the logical component (agent, flow, engine, etc.).
func (l *AgentMeshLogger) WithComponent(c string) *AgentMeshLogger {
	nl := l.clone()
	nl.component = c
	return nl
}

// WithSession attaches session and invocation identifiers.
func (l *AgentMeshLogger) WithSession(sid, iid string) *AgentMeshLogger {
	nl := l.clone()
	nl.sessionID = sid
	nl.invocationID = iid
	return nl
}

func (l *AgentMeshLogger) buildAttrs() []slog.Attr {
	attrs := make([]slog.Attr, 0, len(l.context)+5)
	if l.component != "" {
		attrs = append(attrs, slog.String("component", l.component))
	}
	if l.sessionID != "" {
		attrs = append(attrs, slog.String("session_id", l.sessionID))
	}
	if l.invocationID != "" {
		attrs = append(attrs, slog.String("invocation_id", l.invocationID))
	}
	attrs = append(attrs, slog.Time("timestamp", time.Now()))
	for k, v := range l.context {
		attrs = append(attrs, slog.Any(k, v))
	}
	return attrs
}

func (l *AgentMeshLogger) log(level slog.Level, allowed bool, msg string, args ...interface{}) {
	if !allowed {
		return
	}
	attrs := l.buildAttrs()
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	l.logger.LogAttrs(context.Background(), level, msg, attrs...)
}

// Debug logs at debug level.
func (l *AgentMeshLogger) Debug(msg string, args ...interface{}) {
	l.log(slog.LevelDebug, l.level <= LogLevelDebug, msg, args...)
}

// Info logs at info level.
func (l *AgentMeshLogger) Info(msg string, args ...interface{}) {
	l.log(slog.LevelInfo, l.level <= LogLevelInfo, msg, args...)
}

// Warn logs at warn level.
func (l *AgentMeshLogger) Warn(msg string, args ...interface{}) {
	l.log(slog.LevelWarn, l.level <= LogLevelWarn, msg, args...)
}

// Error logs at error level.
func (l *AgentMeshLogger) Error(msg string, args ...interface{}) {
	l.log(slog.LevelError, l.level <= LogLevelError, msg, args...)
}

// ErrorWithStack logs an error plus a runtime stack snapshot.
func (l *AgentMeshLogger) ErrorWithStack(err error, msg string, args ...interface{}) {
	if l.level > LogLevelError {
		return
	}
	attrs := l.buildAttrs()
	attrs = append(attrs, slog.String("error", err.Error()), slog.String("error_type", fmt.Sprintf("%T", err)))
	stack := make([]byte, 4096)
	n := runtime.Stack(stack, false)
	attrs = append(attrs, slog.String("stack_trace", string(stack[:n])))
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	l.logger.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
}

// LogToolCall records execution details for a tool invocation.
func (l *AgentMeshLogger) LogToolCall(tool string, dur time.Duration, success bool, err error) {
	attrs := l.buildAttrs()
	attrs = append(attrs, slog.String("tool_name", tool), slog.Duration("duration", dur), slog.Bool("success", success))
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	level := slog.LevelInfo
	msg := "Tool execution completed"
	if !success {
		level = slog.LevelError
		msg = "Tool execution failed"
	}
	l.logger.LogAttrs(context.Background(), level, msg, attrs...)
}

// LogLLMCall records model call latency, token usage and success.
func (l *AgentMeshLogger) LogLLMCall(model string, tokens int, dur time.Duration, success bool, err error) {
	attrs := l.buildAttrs()

	attrs = append(attrs, slog.String("model", model), slog.Int("token_count", tokens), slog.Duration("duration", dur), slog.Bool("success", success))

	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}

	level := slog.LevelInfo

	msg := "LLM call completed"

	if !success {
		level = slog.LevelError
		msg = "LLM call failed"
	}

	l.logger.LogAttrs(context.Background(), level, msg, attrs...)
}

// LogFlowExecution records aggregate flow run metrics.
func (l *AgentMeshLogger) LogFlowExecution(flow string, steps int, dur time.Duration, success bool, err error) {
	attrs := l.buildAttrs()
	attrs = append(attrs, slog.String("flow_type", flow), slog.Int("step_count", steps), slog.Duration("duration", dur), slog.Bool("success", success))
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	level := slog.LevelInfo
	msg := "Flow execution completed"
	if !success {
		level = slog.LevelError
		msg = "Flow execution failed"
	}
	l.logger.LogAttrs(context.Background(), level, msg, attrs...)
}

// StartTimer returns a closure that logs the elapsed duration when invoked.
func (l *AgentMeshLogger) StartTimer(op string) func() {
	start := time.Now()
	return func() { l.Info("Operation completed", "operation", op, "duration", time.Since(start)) }
}

// LogPerformance logs arbitrary performance metrics for an operation.
func (l *AgentMeshLogger) LogPerformance(op string, dur time.Duration, metrics map[string]interface{}) {
	attrs := l.buildAttrs()
	attrs = append(attrs, slog.String("operation", op), slog.Duration("duration", dur))
	for k, v := range metrics {
		attrs = append(attrs, slog.Any("metric_"+k, v))
	}
	l.logger.LogAttrs(context.Background(), slog.LevelInfo, "Performance metrics", attrs...)
}

// NoOpLogger discards all log messages. Useful for testing or when logging is disabled.
type NoOpLogger struct{}

// Debug logs a debug message.
func (NoOpLogger) Debug(string, ...any) {}

// Info logs an informational message.
func (NoOpLogger) Info(string, ...any) {}

// Warn logs a warning message.
func (NoOpLogger) Warn(string, ...any) {}

// Error logs an error message.
func (NoOpLogger) Error(string, ...any) {}

// NewSlogLogger creates a new AgentMeshLogger with the specified configuration.
func NewSlogLogger(level LogLevel, format string, addSource bool) *AgentMeshLogger {
	cfg := DefaultLoggerConfig()
	cfg.Level = level
	if format != "" {
		cfg.Format = format
	}
	cfg.AddSource = addSource
	return NewLogger(cfg)
}
