// Package plugins provides built-in plugin implementations for AgentMesh.
//
// This package contains ready-to-use plugins for common cross-cutting concerns:
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
//	pm.Register(ctx, plugins.NewLoggingPlugin(log.Default(), "[Agent]"))
//	pm.Register(ctx, plugins.NewCachePlugin(100))
//
//	// Use with agents - callbacks automatically injected
//	agent, _ := agent.NewReActAgent(model,
//	    agent.WithTools(tools...),
//	    agent.WithPluginManager(pm))
package plugins
