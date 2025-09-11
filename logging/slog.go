package logging

import (
	"io"
	"log/slog"
	"os"
)

// SlogAdapter wraps *slog.Logger to implement the Logger interface.
type SlogAdapter struct{ *slog.Logger }

// Debug logs a debug message with structured attributes.
func (s *SlogAdapter) Debug(msg string, args ...any) {
	s.Logger.Debug(msg, args...)
}

// Info logs an informational message with structured attributes.
func (s *SlogAdapter) Info(msg string, args ...any) {
	s.Logger.Info(msg, args...)
}

// Warn logs a warning message with structured attributes.
func (s *SlogAdapter) Warn(msg string, args ...any) {
	s.Logger.Warn(msg, args...)
}

// Error logs an error message with structured attributes.
func (s *SlogAdapter) Error(msg string, args ...any) {
	s.Logger.Error(msg, args...)
}

// NewSlogAdapter creates a Logger from *slog.Logger.
func NewSlogAdapter(logger *slog.Logger) Logger { return &SlogAdapter{Logger: logger} }

// With returns a child logger enriched with structured attributes.
func (s *SlogAdapter) With(args ...any) Logger { return &SlogAdapter{Logger: s.Logger.With(args...)} }

// NewDefaultSlogLogger creates a Logger using slog.Default().
func NewDefaultSlogLogger() Logger { return NewSlogAdapter(slog.Default()) }

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

// LogFormat enumerates supported slog output formats.
type LogFormat int

const (
	// LogFormatJSON emits logs as JSON (default).
	LogFormatJSON LogFormat = iota
	// LogFormatText emits logs as plain text.
	LogFormatText
)

// NewSlogLoggerWithWriter creates a Logger backed by slog configured by level/format/addSource.
// Output is written to the provided writer.
func NewSlogLoggerWithWriter(level LogLevel, format LogFormat, addSource bool, w io.Writer) Logger {
	opts := &slog.HandlerOptions{Level: slogLevel(level), AddSource: addSource}

	var handler slog.Handler
	switch format {
	case LogFormatText:
		handler = slog.NewTextHandler(w, opts)
	case LogFormatJSON:
		fallthrough
	default:
		handler = slog.NewJSONHandler(w, opts)
	}

	return NewSlogAdapter(slog.New(handler))
}

// NewSlogLogger creates a Logger backed by slog configured by level/format/addSource.
// Default output: os.Stderr.
func NewSlogLogger(level LogLevel, format LogFormat, addSource bool) Logger {
	return NewSlogLoggerWithWriter(level, format, addSource, os.Stderr)
}
