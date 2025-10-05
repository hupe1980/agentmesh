// Package logging provides a minimal logging interface and adapters for AgentMesh.
//
// The Logger interface defines the standard logging methods (Debug, Info, Warn, Error, Fatal)
// that the engine and agents use for observability. This package includes:
//
//   - Logger interface for dependency injection
//   - SlogLogger adapter wrapping Go's structured logging
//   - NoopLogger for silent operation (testing, minimal setups)
//
// Usage:
//
//	logger := logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatJSON, false)
//	app := app.New("demo", rootAgent)
//	r := runner.New(app, func(o *runner.Options){ o.Logger = logger })
//
// The design intentionally keeps the interface minimal to avoid vendor lock-in
// while supporting structured logging where available.
package logging
