# Managed Values Example

This example demonstrates **type-safe managed values** - ephemeral runtime state that is NOT included in checkpoints.

## What Are Managed Values?

Managed values provide a clean separation between:

- **Persistent State** (channels/keys): Checkpointed, survives restarts, used for business logic
- **Managed Values**: Ephemeral runtime state that doesn't belong in checkpoints

## Use Cases

1. **Runtime Configuration**: API keys, timeouts, feature flags
2. **Session State**: User sessions, auth tokens, connection pools  
3. **Metrics Collectors**: Runtime statistics that don't need persistence
4. **Resource Handles**: Database connections, file handles, caches
5. **Computed Values**: Derived state that's recomputed on demand

## Type Safety

Managed values use Go generics for compile-time type safety:

```go
// Define typed managed values
configMV := state.NewManagedValue[*RuntimeConfig]("runtime_config")
sessionMV := state.NewManagedValue[*SessionInfo]("session")
metricsMV := state.NewManagedValue[*MetricsCollector]("metrics")

// Register with manager
state.RegisterManagedValue(mgr, configMV)
state.RegisterManagedValue(mgr, sessionMV)
state.RegisterManagedValue(mgr, metricsMV)

// Type-safe access (no type assertions needed!)
config, err := state.GetManagedValue[*RuntimeConfig](mgr, ctx, "runtime_config")
session, err := state.GetManagedValue[*SessionInfo](mgr, ctx, "session")
metrics, err := state.GetManagedValue[*MetricsCollector](mgr, ctx, "metrics")
```

## Three Implementations

### 1. SimpleManagedValue
Thread-safe storage with mutex protection:

```go
mv := state.NewManagedValue[string]("api_key")
mv.Set(ctx, "sk_live_abc123")
value, _ := mv.Get(ctx)
```

### 2. ComputedManagedValue
Recomputed on every access:

```go
currentTimeMV := state.NewComputedManagedValue("timestamp", func(ctx context.Context) (int64, error) {
    return time.Now().Unix(), nil
})
```

### 3. CachedManagedValue
Wraps another managed value with TTL caching:

```go
source := state.NewManagedValue[*Config]("config")
cached := state.NewCachedManagedValue("cached_config", source, 60) // 60s TTL
```

## Checkpoint Behavior

This is the key insight of managed values:

| Feature | Persistent State (Keys/Channels) | Managed Values |
|---------|----------------------------------|----------------|
| **Checkpointed** | ✅ YES | ❌ NO |
| **Survives restart** | ✅ YES | ❌ NO |
| **Time travel** | ✅ YES | ❌ NO |
| **Initialized at** | Graph definition | Runtime |
| **Use for** | Business logic, conversation state | Config, sessions, metrics |

## Example Output

```
✓ Registered managed values:
  - runtime_config
  - session
  - metrics
  - current_time

=== Starting Graph Execution ===

[processor_1] Node Configuration:
  - API Key: sk_l...cdef (masked)
  - Timeout: 200ms
  - Max Retries: 3
  - Debug Mode: true

[processor_1] Session Info:
  - User: user@example.com
  - Token: tok_...1234 (masked)
  - Session Duration: 123ms

[processor_1] Persistent State:
  - Counter: 0
  - Last Node: start

[processor_1] Work completed within timeout

[processor_2] Node Configuration:
  - API Key: sk_l...cdef (masked)
  - Timeout: 200ms
  - Max Retries: 3
  - Debug Mode: true

[processor_2] Session Info:
  - User: user@example.com
  - Token: tok_...1234 (masked)
  - Session Duration: 278ms

[processor_2] Persistent State:
  - Counter: 1
  - Last Node: processor_1

[processor_2] Work completed within timeout

=== Runtime Metrics ===
Total Executions: 2
Total Latency: 456ms

Per-Node Executions:
  - processor_1: 1 times
  - processor_2: 1 times
======================

=== Final Persistent State (Checkpointed) ===
Counter: 2
Last Node: processor_2

=== Final Managed Values (Ephemeral, NOT Checkpointed) ===
Runtime Config: APIKey=sk_l...cdef, Timeout=200ms, MaxRetries=3
Session: User=user@example.com, Duration=589ms
Current Time (computed): 2025-11-19T14:23:45Z

=== Updating Runtime Configuration ===
✓ Configuration updated (affects next execution)

=== Checkpoint Behavior ===
Persistent State (Counter, History):
  ✓ INCLUDED in checkpoints
  ✓ Survives process restart
  ✓ Used for time travel

Managed Values (Config, Session, Metrics):
  ✗ NOT included in checkpoints
  ✗ Lost on process restart
  ✓ Reinitialized at runtime
  ✓ Perfect for ephemeral state
```

## Run

```bash
go run main.go
```

## Key Takeaways

1. **Type Safety**: Compile-time type checking, no runtime assertions
2. **Clean Architecture**: Separate persistent from ephemeral state
3. **Checkpoint Efficiency**: Don't serialize runtime-only data
4. **Flexible**: Simple, computed, or cached implementations
5. **Practical**: Perfect for config, sessions, metrics, resources

## Related

- See `_prompts/PREGEL.md` for design rationale (inspired by LangGraph)
- See `pkg/state/managed_value_test.go` for comprehensive tests
