package core

import "github.com/hupe1980/agentmesh/logging"

// loggerAdapter wraps a logging.Logger and exposes convenience methods
// (LogDebug/LogInfo/LogWarn/LogError). It guarantees a non-nil logger by
// substituting a NoOpLogger when constructed with nil.
type loggerAdapter struct {
	logger logging.Logger
}

// newLoggerAdapter constructs a loggerAdapter with a non-nil logger.
func newLoggerAdapter(l logging.Logger) *loggerAdapter {
	if l == nil {
		l = logging.NoOpLogger{}
	}
	return &loggerAdapter{logger: l}
}

// Logger returns the underlying logger.
func (l *loggerAdapter) Logger() logging.Logger {
	return l.logger
}

// LogDebug logs a debug message.
func (l *loggerAdapter) LogDebug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

// LogInfo logs an info message.
func (l *loggerAdapter) LogInfo(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// LogWarn logs a warning message.
func (l *loggerAdapter) LogWarn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// LogError logs an error message.
func (l *loggerAdapter) LogError(msg string, args ...any) {
	l.logger.Error(msg, args...)
}
