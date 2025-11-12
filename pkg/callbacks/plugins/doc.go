// Package plugins provides built-in plugin implementations for AgentMesh.
//
// This package contains ready-to-use plugins for common cross-cutting concerns:
//   - LoggingPlugin: Logs all lifecycle events for debugging and monitoring
//   - MetricsPlugin: Tracks execution metrics (counts, durations, errors)
//   - TracingPlugin: OpenTelemetry distributed tracing integration
//   - PersistencePlugin: Saves execution history for replay and auditing
//   - AuditPlugin: Compliance logging for security and governance
//
// Example usage:
//
//	pm := callbacks.NewPluginManager()
//	pm.Register(ctx, plugins.NewLoggingPlugin(log.Default()))
//	pm.Register(ctx, plugins.NewMetricsPlugin(metricsRegistry))
//
//	graph := graph.New(
//	    graph.WithPlugins(pm),
//	)
package plugins
