# Namespace Example

This example demonstrates AgentMesh's namespace system for state isolation using `graph.WithNamespace()`.

## Overview

Namespaces restrict which state keys a node can read and write. Nodes wrapped with `WithNamespace` can only access keys within their namespace prefix. This is useful for:

- **Multi-agent systems** - Each agent has its own isolated state space
- **Security** - Prevent nodes from accessing data they shouldn't
- **State organization** - Group related keys together with prefixes

## Key Concepts

### Key Naming Convention
Keys use dot notation (`namespace.keyname`) for isolation:
```go
// Agent 1's private keys
agent1Data   := graph.NewKey("agent1.data", "")
agent1Status := graph.NewKey("agent1.status", "")

// Agent 2's private keys
agent2Data   := graph.NewKey("agent2.data", "")
agent2Status := graph.NewKey("agent2.status", "")

// Global key (no namespace prefix)
sharedResult := graph.NewKey("result", "")
```

### Creating Namespaces
```go
// Create a namespace
ns1 := graph.NewNamespace("agent1")
ns2 := graph.NewNamespace("agent2")
```

### Wrapping Nodes with Namespaces
```go
// Node can only access "agent1.*" keys
g.Node("agent1_process", graph.WithNamespace(
    func(ctx context.Context, view graph.View) (*graph.Command, error) {
        // Can read: agent1.data, agent1.status
        // Cannot read: agent2.data, agent2.status (returns zero value)
        data := graph.Get(view, agent1Data)
        return graph.Set(agent1Status, "done").To("next")
    },
    ns1,           // namespace to restrict to
    false,         // includeGlobal: can this node access global keys?
), "next")
```

### Benefits
- **Zero overhead** - Just string prefix matching, no runtime cost
- **Full type safety** - Compile-time type checking with generics
- **No collisions** - Keys with same name in different namespaces don't conflict
- **Graceful degradation** - Reading blocked keys returns zero value, writing returns error

## Running the Example

```bash
go run examples/namespaces/main.go
```

## Expected Output

```
=== Namespaces Example ===
  Demonstrates state isolation with graph.WithNamespace()

  [init] Setting up initial state
  [agent1] Read own data: agent1-initial
  [agent1] Tried to read agent2.data: '' (empty = blocked)
  [agent2] Read own data: agent2-initial
  [agent2] Tried to read agent1.data: '' (empty = blocked)
  [merge] Combining results from both agents

  Namespace features:
    • graph.NewNamespace('prefix') - create a namespace
    • graph.WithNamespace(fn, ns, includeGlobal) - restrict node access
    • Keys with 'prefix.' are in the namespace
    • Keys without dots are global (if includeGlobal=true)
    • Violations return zero values for reads, error for writes
```

## API Reference

### Creating Namespaces
```go
// Create a namespace for prefix matching
ns := graph.NewNamespace("agent1")
```

### Wrapping Node Functions
```go
// Restrict node to only access keys matching "agent1.*"
wrappedFn := graph.WithNamespace(nodeFn, ns, includeGlobal)

// Parameters:
// - nodeFn: The original node function
// - ns: The namespace to restrict access to
// - includeGlobal: If true, also allows access to keys without a namespace prefix
```

### includeGlobal Parameter
- `false`: Node can ONLY access keys with the namespace prefix
- `true`: Node can access namespace keys AND global keys (no dot in name)

```go
// Can access: agent1.data, agent1.status
// Cannot access: result, agent2.data
graph.WithNamespace(fn, ns1, false)

// Can access: agent1.data, agent1.status, result
// Cannot access: agent2.data
graph.WithNamespace(fn, ns1, true)
```

## When to Use Namespaces

**Use namespaces when:**
- Multiple agents need isolated state
- You want to prevent accidental cross-agent data access
- Building multi-tenant or sandboxed workflows

**Don't use namespaces when:**
- You have a single agent
- All nodes need full state access
- Simplicity is more important than isolation

## Related Examples
- `basic_agent/` - Simple agent without namespaces
- `supervisor_agent/` - Multi-agent coordination
- `subgraph/` - Subgraph state isolation
