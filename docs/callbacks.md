---
layout: doc
title: Plugin System
permalink: /callbacks/
hero:
  title: Plugin System
  description: Extend AgentMesh with type-safe plugins for observability, security, and resilience.
  primary_cta:
    label: View examples
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples/callback_integration"
    external: true
  secondary_cta:
    label: API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/callbacks"
    external: true
sidebar:
  - title: Overview
    url: "#overview"
  - title: Plugin interface
    url: "#plugin-interface"
  - title: Basic usage
    url: "#basic-usage"
  - title: Built-in plugins
    url: "#built-in-plugins"
  - title: Custom plugins
    url: "#custom-plugins"
  - title: Best practices
    url: "#best-practices"
---

# Plugin System

> **⚠️ DEPRECATED**: The plugin/callback system is deprecated in favor of the new [Middleware System](/middleware/). The middleware system provides better composability, type safety, and covers all three execution layers (graph, model, tool). See the [migration guide](/middleware/#migration-from-callbacks) for upgrading existing code.

The plugin system enables powerful extensions to AgentMesh workflows through a unified, type-safe interface for cross-cutting concerns like observability, security, and resilience.

---

## Overview {#overview}

Plugins provide lifecycle hooks into agent execution:

- **Init/Shutdown** - Resource management (connections, cleanup)
- **BeforeNode/AfterNode/OnNodeError** - Node execution interception (with short-circuit and state enrichment)
- **BeforeModel/AfterModel/OnModelError** - Model request/response transformation
- **BeforeTool/AfterTool/OnToolError** - Tool execution monitoring
- **OnStateChange** - State change tracking (nodeName + updates)

**Key features**:
- ✅ Type-safe - Constructor-based configuration, no `map[string]any`
- ✅ Composable - Multiple plugins work together seamlessly
- ✅ Stateful - Plugins can maintain internal state across invocations
- ✅ Thread-safe - Built-in concurrency protection
- ✅ Short-circuiting - Return early to skip model calls (caching, rate limiting)

---

## Plugin Interface {#plugin-interface}

All plugins implement the `Plugin` interface:

```go
type Plugin interface {
    // Lifecycle
    Init(ctx context.Context) error
    Shutdown(ctx context.Context) error
    
    // Node hooks
    BeforeNode(ctx context.Context, nodeName string, view *state.ReadView) (state.Updates, error)
    AfterNode(ctx context.Context, nodeName string, view *state.ReadView, updates state.Updates) error
    OnNodeError(ctx context.Context, nodeName string, err error) error
    
    // Model hooks
    BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error)
    AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error)
    OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error)
    
    // Tool hooks
    BeforeTool(ctx context.Context, toolName string, input any) error
    AfterTool(ctx context.Context, toolName string, result tool.ToolResult) error
    OnToolError(ctx context.Context, toolName string, err error) error
    
    // State hooks
    OnStateChange(ctx context.Context, nodeName string, updates state.Updates) error
}
```

### NoopPlugin Helper

Embed `NoopPlugin` to implement only the hooks you need:

```go
type MyPlugin struct {
    callbacks.NoopPlugin  // Default no-op implementations
    
    // Your plugin state
    counter int
}

func (p *MyPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
    p.counter++
    // Only implement what you need
    return nil, nil
}
```

---

## Basic Usage {#basic-usage}

### 1. Create Plugin Manager

```go
import (
    "github.com/hupe1980/agentmesh/pkg/callbacks"
    "github.com/hupe1980/agentmesh/pkg/callbacks/plugins"
)

pm := callbacks.NewPluginManager()
```

### 2. Register Plugins

```go
// Built-in plugins
pm.Register(ctx, plugin.NewLoggingPlugin(log.Default(), "[Agent]"))
pm.Register(ctx, plugin.NewCircuitBreakerPlugin(3, 5*time.Second, 1))
pm.Register(ctx, plugin.NewCachePlugin(100))

// Custom plugins
pm.Register(ctx, &MyCustomPlugin{})
```

### 3. Attach to Agent

```go
// Callbacks are automatically injected via context
reactAgent, err := agent.NewReAct(
    model,
    agent.WithTools(tools...),
    agent.WithPluginManager(pm),
)
```

### 4. Cleanup on Shutdown

```go
defer pm.Shutdown(context.Background())
```

---

## Built-in Plugins {#built-in-plugins}

AgentMesh provides production-ready plugins in `pkg/callbacks/plugins`:

### LoggingPlugin

Logs all lifecycle events for debugging and audit trails.

```go
plugin := plugin.NewLoggingPlugin(
    log.Default(),
    "[AgentMesh]",  // prefix
)
pm.Register(ctx, plugin)
```

### CircuitBreakerPlugin

Prevents cascading failures with three-state circuit breaker pattern.

```go
plugin := plugin.NewCircuitBreakerPlugin(
    3,              // maxFailures
    5*time.Second,  // resetTimeout
    1,              // halfOpenLimit
)
pm.Register(ctx, plugin)

// Monitor state
state := plugin.GetState()  // "closed", "open", "half-open"
plugin.Reset()              // Manual reset
```

### RateLimitPlugin

Enforces rate limiting with sliding window algorithm.

```go
plugin := plugin.NewRateLimitPlugin(
    100,           // maxRequests
    time.Minute,   // window
)
pm.Register(ctx, plugin)

// Check current rate
rate := plugin.GetCurrentRate()
```

### CachePlugin

In-memory response caching with LRU eviction.

```go
plugin := plugin.NewCachePlugin(100)  // max entries
pm.Register(ctx, plugin)

// Get statistics
stats := plugin.GetStats()
fmt.Printf("Hit rate: %.1f%%, size: %d\n",
    stats.HitRate*100, stats.Size)
```

### SemanticCachePlugin

Semantic similarity-based caching using embeddings.

```go
embedder := embedding.NewOpenAIEmbedder(client)
plugin := plugin.NewSemanticCachePlugin(
    embedder,
    0.95,  // similarity threshold
    100,   // max entries
)
pm.Register(ctx, plugin)

// Get statistics
stats := plugin.GetStats()
fmt.Printf("Hit rate: %.1f%%\n", stats.HitRate*100)
```

### RetryPlugin

Tracks retry attempts with exponential backoff.

```go
plugin := plugin.NewRetryPlugin(
    3,                  // maxRetries
    100*time.Millisecond, // baseDelay
    5*time.Second,      // maxDelay
)
pm.Register(ctx, plugin)
```

---

## Custom Plugins {#custom-plugins}

### Basic Plugin

```go
type ValidationPlugin struct {
    callbacks.NoopPlugin
    
    blocklist []string
    mu        sync.RWMutex
}

func (p *ValidationPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
    p.mu.RLock()
    defer p.mu.RUnlock()
    
    for _, msg := range req.Messages {
        content := message.Stringify(msg)
        for _, blocked := range p.blocklist {
            if strings.Contains(content, blocked) {
                return nil, fmt.Errorf("blocked: contains '%s'", blocked)
            }
        }
    }
    return nil, nil  // Continue to model
}
```

### Stateful Plugin with Node Callbacks

```go
type NodeMetricsPlugin struct {
    callbacks.NoopPlugin
    
    nodeCallCount atomic.Int64
    errorCount    atomic.Int64
    mu            sync.Mutex
    startTimes    map[string]time.Time
}

func (p *NodeMetricsPlugin) BeforeNode(ctx context.Context, nodeName string, view *state.ReadView) (state.Updates, error) {
    p.nodeCallCount.Add(1)
    
    p.mu.Lock()
    p.startTimes[nodeName] = time.Now()
    p.mu.Unlock()
    
    return nil, nil  // Continue to node execution
}

func (p *NodeMetricsPlugin) AfterNode(ctx context.Context, nodeName string, view *state.ReadView, updates state.Updates) error {
    p.mu.Lock()
    if start, ok := p.startTimes[nodeName]; ok {
        duration := time.Since(start)
        log.Printf("Node %s took %v", nodeName, duration)
        delete(p.startTimes, nodeName)
    }
    p.mu.Unlock()
    
    return nil
}

func (p *NodeMetricsPlugin) OnNodeError(ctx context.Context, nodeName string, err error) error {
    p.errorCount.Add(1)
    log.Printf("Node %s failed: %v", nodeName, err)
    return nil
}
```

### Plugin with State Enrichment

```go
type MetadataPlugin struct {
    callbacks.NoopPlugin
}

// AfterNode enriches state with metadata after node execution
func (p *MetadataPlugin) AfterNode(ctx context.Context, nodeName string, view *state.ReadView, updates state.Updates) error {
    // Add metadata to updates (mutable map)
    updates["_last_node"] = nodeName
    updates["_timestamp"] = time.Now().Unix()
    updates["_iteration"] = view.Get("_iteration", 0).(int) + 1
    
    return nil
}

// OnStateChange tracks all state modifications
func (p *MetadataPlugin) OnStateChange(ctx context.Context, nodeName string, updates state.Updates) error {
    log.Printf("Node %s modified %d keys", nodeName, len(updates))
    return nil
}
```

---

## Best Practices {#best-practices}

### Plugin Design

1. **Single Responsibility** - Each plugin should handle one concern
2. **Constructor Configuration** - Pass dependencies via constructor, not runtime config
3. **Thread-Safe State** - Use `sync.Mutex` or `atomic` for shared state
4. **Embed NoopPlugin** - Override only the hooks you need

### Registration Order

Plugins execute in registration order:

```go
pm.Register(ctx, authPlugin)      // Security first
pm.Register(ctx, rateLimitPlugin) // Then rate limiting
pm.Register(ctx, cachePlugin)     // Then caching
pm.Register(ctx, loggingPlugin)   // Finally logging
```

### Error Handling

- **BeforeModel**: Return error to block execution
- **AfterModel**: Return error to propagate failure
- **OnModelError**: Transform error or return fallback response

### Short-Circuiting

```go
func (p *CachePlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
    if cached := p.cache.Get(req); cached != nil {
        return cached, nil  // Skip model call
    }
    return nil, nil  // Continue to model
}
```

### Resource Management

```go
func (p *DatabasePlugin) Init(ctx context.Context) error {
    db, err := sql.Open("postgres", p.connectionString)
    if err != nil {
        return err
    }
    p.db = db
    return nil
}

func (p *DatabasePlugin) Shutdown(ctx context.Context) error {
    return p.db.Close()
}
```

---

## Examples

- [Plugin Integration](https://github.com/hupe1980/agentmesh/tree/main/examples/callback_integration) - Complete plugin system demo
- [Circuit Breaker](https://github.com/hupe1980/agentmesh/tree/main/examples/circuit_breaker) - Resilience patterns
- [Guardrails](https://github.com/hupe1980/agentmesh/tree/main/examples/guardrails) - Security and PII protection

See [CALLBACK.md](https://github.com/hupe1980/agentmesh/blob/main/CALLBACK.md) for detailed design documentation.
