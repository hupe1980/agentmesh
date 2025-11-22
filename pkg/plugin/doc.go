// Package plugin provides the plugin system and built-in plugin implementations for AgentMesh.
//
// This package includes:
//   - Plugin interfaces for extending graph and node behavior
//   - Built-in plugin implementations for common cross-cutting concerns:
//   - LoggingPlugin: Logs all lifecycle events for debugging and monitoring
//   - CachePlugin: In-memory response caching with LRU eviction
//   - SemanticCachePlugin: Semantic similarity-based caching
//   - CircuitBreakerPlugin: Prevents cascading failures
//   - RateLimitPlugin: Rate limiting with sliding window algorithm
//   - RetryPlugin: Tracks retry attempts with exponential backoff
//
// Example usage:
//
//	pm := callbacks.NewPluginManager()
//	pm.Register(ctx, plugin.NewLoggingPlugin(log.Default(), "[Agent]"))
//	pm.Register(ctx, plugin.NewCachePlugin(100))
//
//	// Use with agents - callbacks automatically injected
//	agent, _ := agent.NewReActAgent(model,
//	    agent.WithTools(tools...),
//	    agent.WithPluginManager(pm))
package plugin
