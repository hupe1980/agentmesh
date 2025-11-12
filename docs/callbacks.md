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

The plugin system enables powerful extensions to AgentMesh workflows through a unified, type-safe interface for cross-cutting concerns like observability, security, and resilience.

---

## Overview {#overview}

Plugins provide lifecycle hooks into agent execution:

- **Init/Shutdown** - Resource management (connections, cleanup)
- **BeforeModel/AfterModel/OnModelError** - Model request/response transformation
- **BeforeTool/AfterTool/OnToolError** - Tool execution monitoring
- **OnGraphStart/OnGraphComplete/OnGraphError** - Graph lifecycle tracking
- **BeforeNode/AfterNode** - Node-level interception
- **OnStateChange/OnMessage** - State and message tracking

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
    
    // Graph-level hooks
    OnGraphStart(ctx context.Context, graphID string) error
    OnGraphComplete(ctx context.Context, graphID string, stats GraphStats) error
    OnGraphError(ctx context.Context, graphID string, err error) error
    
    // Node-level hooks
    BeforeNode(ctx context.Context, nodeName string) error
    AfterNode(ctx context.Context, nodeName string, result NodeResult) error
    
    // Model hooks
    BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error)
    AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error)
    OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error)
    
    // Tool hooks
    BeforeTool(ctx context.Context, toolName string, input any) error
    AfterTool(ctx context.Context, toolName string, result ToolResult) error
    OnToolError(ctx context.Context, toolName string, err error) error
    
    // State hooks
    OnStateChange(ctx context.Context, changes StateChanges) error
    OnMessage(ctx context.Context, msg message.Message) error
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
pm.Register(plugins.NewLoggingPlugin(log.Default(), "[Agent]"))
pm.Register(plugins.NewMetricsPlugin(metricsProvider))
pm.Register(plugins.NewCircuitBreakerPlugin(3, 5*time.Second, 1))

// Custom plugins
pm.Register(&MyCustomPlugin{})
```

### 3. Attach to Agent

```go
compiled, err := agent.NewReActAgent(
    model,
    tools,
    agent.WithModelCallbacks(pm),
    agent.WithToolCallbacks(pm),
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
plugin := plugins.NewLoggingPlugin(
    log.Default(),
    "[AgentMesh]",  // prefix
)
pm.Register(plugin)
```

### MetricsPlugin

Collects execution metrics (latency, errors, throughput) for observability.

```go
provider := prometheus.NewProvider()
plugin := plugins.NewMetricsPlugin(provider)
pm.Register(plugin)

// Get snapshot
snapshot := plugin.GetSnapshot()
fmt.Printf("Model calls: %d, errors: %d, avg latency: %v\n",
    snapshot.ModelCalls, snapshot.ModelErrors, snapshot.AvgModelLatency)
```

### TracingPlugin

Creates distributed tracing spans for OpenTelemetry/Jaeger integration.

```go
tracer := trace.NewOpenTelemetryTracer("agentmesh")
plugin := plugins.NewTracingPlugin(tracer)
pm.Register(plugin)
```

### CircuitBreakerPlugin

Prevents cascading failures with three-state circuit breaker pattern.

```go
plugin := plugins.NewCircuitBreakerPlugin(
    3,              // maxFailures
    5*time.Second,  // resetTimeout
    1,              // halfOpenLimit
)
pm.Register(plugin)

// Monitor state
state := plugin.GetState()  // "closed", "open", "half-open"
plugin.Reset()              // Manual reset
```

### RateLimitPlugin

Enforces rate limiting with sliding window algorithm.

```go
plugin := plugins.NewRateLimitPlugin(
    100,           // maxRequests
    time.Minute,   // window
)
pm.Register(plugin)

// Check current rate
rate := plugin.GetCurrentRate()
```

### CachePlugin

In-memory response caching with LRU eviction.

```go
plugin := plugins.NewCachePlugin(100)  // max entries
pm.Register(plugin)

// Get statistics
stats := plugin.GetStats()
fmt.Printf("Hit rate: %.1f%%, size: %d\n",
    stats.HitRate*100, stats.Size)
```

### RetryPlugin

Tracks retry attempts with exponential backoff (note: actual retry requires model layer integration).

```go
plugin := plugins.NewRetryPlugin(
    3,                  // maxRetries
    100*time.Millisecond, // baseDelay
    5*time.Second,      // maxDelay
)
pm.Register(plugin)
```

### PersistencePlugin

Persists execution data to SQL database for audit and analytics.

```go
db, _ := sql.Open("sqlite3", "audit.db")
plugin := plugins.NewPersistencePlugin(db)
pm.Register(plugin)
```

### ReplayPlugin

Records and replays model responses for deterministic testing.

```go
// Record mode
plugin := plugins.NewReplayPlugin(plugins.RecordMode)
pm.Register(plugin)
// ... run tests ...
f, _ := os.Create("recordings.json")
plugin.SaveRecordings(f)

// Replay mode
plugin := plugins.NewReplayPlugin(plugins.ModeReplay)
f, _ := os.Open("recordings.json")
plugin.LoadRecordings(f)
pm.Register(plugin)
```

### AuditPlugin

Writes JSON audit logs to any `io.Writer`.

```go
f, _ := os.Create("audit.log")
plugin := plugins.NewAuditPlugin(f)
pm.Register(plugin)
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

### Stateful Plugin

```go
type MetricsPlugin struct {
    callbacks.NoopPlugin
    
    callCount   atomic.Int64
    errorCount  atomic.Int64
    latencies   []time.Duration
    mu          sync.Mutex
    startTimes  map[string]time.Time
}

func (p *MetricsPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
    p.callCount.Add(1)
    
    p.mu.Lock()
    p.startTimes[fmt.Sprintf("%p", req)] = time.Now()
    p.mu.Unlock()
    
    return nil, nil
}

func (p *MetricsPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
    p.mu.Lock()
    key := fmt.Sprintf("%p", req)
    if start, ok := p.startTimes[key]; ok {
        p.latencies = append(p.latencies, time.Since(start))
        delete(p.startTimes, key)
    }
    p.mu.Unlock()
    
    return nil, nil
}

func (p *MetricsPlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
    p.errorCount.Add(1)
    return nil, err
}
```

### Plugin with Dependencies

```go
type CachePlugin struct {
    callbacks.NoopPlugin
    
    metricsPlugin *MetricsPlugin  // Dependency
    cache         map[string]*model.Response
}

func NewCachePlugin(metrics *MetricsPlugin) *CachePlugin {
    return &CachePlugin{
        metricsPlugin: metrics,
        cache:         make(map[string]*model.Response),
    }
}

// Register in order
metrics := &MetricsPlugin{}
cache := NewCachePlugin(metrics)

pm.Register(metrics)
pm.Register(cache)
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
pm.Register(authPlugin)      // Security first
pm.Register(rateLimitPlugin) // Then rate limiting
pm.Register(cachePlugin)     // Then caching
pm.Register(metricsPlugin)   // Finally metrics
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
