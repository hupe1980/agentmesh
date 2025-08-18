// Package logging provides a minimal logging interface and adapters for AgentMesh.
//
// The Logger interface defines the standard logging methods (Debug, Info, Warn, Error, Fatal)
// that the engine and agents use for observability. This package includes:
//
//   - Logger interface for dependency injection
//   - SlogLogger adapter wrapping Go's structured logging
//   - NoOpLogger for silent operation (testing, minimal setups)
//
// Usage:
//
//	logger := logging.NewSlogLogger(logging.LogLevelInfo, "json", false)
//	engine := engine.New(sessionStore, artifactStore, memoryStore, engine.WithLogger(logger))
//
// The design intentionally keeps the interface minimal to avoid vendor lock-in
// while supporting structured logging where available.
package logging
