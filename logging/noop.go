package logging

// NoopLogger discards all log messages. Useful for testing or when logging is disabled.
type NoopLogger struct{}

// Debug logs a debug message.
func (NoopLogger) Debug(string, ...any) {}

// Info logs an informational message.
func (NoopLogger) Info(string, ...any) {}

// Warn logs a warning message.
func (NoopLogger) Warn(string, ...any) {}

// Error logs an error message.
func (NoopLogger) Error(string, ...any) {}

// With returns the same no-op logger (attributes are ignored).
func (NoopLogger) With(...any) Logger { return NoopLogger{} }
