# Managed Values Example

This example demonstrates **managed values** in AgentMesh - ephemeral runtime state that is NOT included in checkpoints.

## What are Managed Values?

Managed values are runtime state that exists only during execution:

- **NOT persisted** to checkpoints
- **Lost** on process restart
- **Must be reinitialized** at runtime
- **Perfect for** ephemeral/sensitive data

## Use Cases

| Type | Use Case |
|------|----------|
| **StaticManagedValue** | API keys, auth tokens, session state, config |
| **ManagedValueProvider** | Current timestamp, dynamic metrics, derived state |
| **ManagedValueProvider + WithCacheTTL** | Expensive computations with TTL, rate-limited API responses |

## Key Concepts

### 1. Static Managed Value

Thread-safe storage for runtime configuration:

```go
// Create with initial value
var configMV = graph.NewManagedValue("config", &Config{
    APIKey:  os.Getenv("API_KEY"),
    Timeout: 30 * time.Second,
})

// Create with nil (will be set later)
var sessionMV = graph.NewManagedValue("session", (*Session)(nil))
```

### 2. Provider (Always Fresh)

Recomputed on every access:

```go
var counterMV = graph.NewManagedValueProvider("counter", func(ctx context.Context) (int64, error) {
    return atomic.AddInt64(&count, 1), nil
})
```

### 3. Provider with Caching

Add `WithCacheTTL` to cache the computed value:

```go
// Cached: reuses value for 5 seconds, then recomputes
var cachedTimeMV = graph.NewManagedValueProvider("cached_time", func(ctx context.Context) (time.Time, error) {
    return time.Now(), nil
}, graph.WithCacheTTL(5*time.Second))
```

## Usage in Nodes

Access managed values via scope - same pattern as regular state:

```go
// Define managed values at package level for type-safe access
var configMV = graph.NewManagedValue("config", &Config{...})

g.Node("process", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    // Access managed value via scope - same pattern as graph.Get(scope, key)
    config := graph.GetManaged(ctx, scope, configMV)

    // Use config.APIKey, config.Timeout, etc.
    return graph.Set(resultKey, result).End()
}, graph.END)
```

## Checkpoint Resume Behavior

Managed values never live inside checkpoints, so checkpoint resume must explicitly
reattach them. Use the managed value options to make this safer:

```go
var runtimeConfigMV = graph.NewManagedValue(
  "runtime_config",
  &RuntimeConfig{APIKey: os.Getenv("API_KEY"), Timeout: 30 * time.Second},
  graph.WithManagedValueRequired(), // resume fails fast if missing
  graph.WithManagedValueRehydrator(func(ctx context.Context) error {
    cfg, err := runtimeConfigMV.Get(ctx)
    if err != nil {
      return err
    }
    cfg.APIKey = os.Getenv("API_KEY") // refresh secret on restore
    return nil
  }),
)
```

- **Required**: `WithManagedValueRequired` forces `graph.WithManagedValues` during resume, otherwise the executor surfaces a helpful error before any user code runs.
- **Rehydrator**: `WithManagedValueRehydrator` runs after a checkpoint restore and after providers refresh cached state, ideal for rotating credentials or re-opening connections.

## Running the Graph

Pass managed values directly when running the graph:

```go
compiled.Run(ctx, input, graph.WithManagedValues(configMV, counterMV, cachedTimeMV))
```

## Comparison with Regular State

| Feature | Regular State (`graph.Get`) | Managed Values (`graph.GetManaged`) |
|---------|----------------------------|-------------------------------------|
| Access Pattern | `graph.Get(view, key)` | `graph.GetManaged(ctx, view, mv)` |
| Checkpointed | ✅ Yes | ❌ No |
| Survives restart | ✅ Yes | ❌ No |
| Visible to all nodes | ✅ Yes | ✅ Yes |
| Type-safe | ✅ Yes | ✅ Yes |
| Thread-safe | ✅ Yes | ✅ Yes |
| Use for sensitive data | ❌ No | ✅ Yes |

## Running the Example

```bash
go run main.go
```

Output:

```
=== Managed Values Demo ===

Managed values are ephemeral runtime state NOT included in checkpoints.
They're perfect for API keys, session state, metrics, and cached values.

--- Run 1 ---
Node 'process' executed:
  Execution #1 | API Key: sk_demo_ke... | Timeout: 30s | Cached Time: 2025-01-15T10:30:00Z

--- Run 2 ---
Node 'process' executed:
  Execution #2 | API Key: sk_demo_ke... | Timeout: 30s | Cached Time: 2025-01-15T10:30:00Z

--- Run 3 ---
Node 'process' executed:
  Execution #3 | API Key: sk_demo_ke... | Timeout: 30s | Cached Time: 2025-01-15T10:30:00Z

=== Key Points ===
1. Managed values are NOT persisted in checkpoints
2. Access via graph.GetManaged(ctx, view, managedValue) - same pattern as state
3. NewManagedValue(name, value) - static thread-safe storage
4. NewManagedValueProvider(name, fn) - recomputed on every access
5. NewManagedValueProvider(name, fn, WithCacheTTL(ttl)) - cached with TTL
```

Note how:
- The execution count increments (ManagedValueProvider without cache)
- The cached time stays the same within the TTL (ManagedValueProvider with WithCacheTTL)
- Config values are accessible without being checkpointed (StaticManagedValue)
