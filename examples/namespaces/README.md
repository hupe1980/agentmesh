# Namespace Example

This example demonstrates AgentMesh's namespace system for state isolation.

## Overview

Namespaces allow different components (agents, subgraphs, tools) to have their own isolated state while sharing the same state manager. This is useful for:

- **Multi-agent systems** - Each agent has its own state space
- **Subgraph isolation** - Subgraphs don't interfere with each other
- **Tool state** - Tools can maintain their own state without conflicts
- **State organization** - Group related keys together

## Key Concepts

### Global Keys (Default)
Global keys have no prefix and are simple to use:
```go
var ConfigKey = state.NewKey[string]("config", "")
var CounterKey = state.NewKey[int]("counter", 0)
```

### Namespaced Keys
Namespaced keys use dot notation (`namespace.keyname`) for isolation:
```go
agent1NS := state.MustNamespace("agent1")
statusKey := state.TypedKey[string](agent1NS, "status", "")  // "agent1.status"
```

### Benefits
- **Zero overhead** - Just string prefixes, no runtime cost
- **Full type safety** - Compile-time type checking with generics
- **No collisions** - Keys with same name in different namespaces don't conflict
- **Progressive complexity** - Start with global keys, add namespaces when needed

## Running the Example

```bash
go run examples/namespaces/main.go
```

## Output

The example demonstrates:

1. **Global Keys** - Simple keys without namespaces
2. **Namespaced Keys** - Isolated state for different agents
3. **Namespace Views** - Filtering state by namespace
4. **Listing Namespaces** - Discovering active namespaces
5. **Copying Namespaces** - Transferring state between agents
6. **Key Introspection** - Examining key structure

## API Reference

### Creating Namespaces
```go
// Create namespace (returns error if invalid)
ns, err := state.NewNamespace("agent1")

// Create namespace (panics if invalid) 
ns := state.MustNamespace("agent1")

// Global namespace (no prefix)
globalNS := state.Global
```

### Creating Keys
```go
// Global key
key := state.NewKey[int]("counter", 0)

// Namespaced key
ns := state.MustNamespace("agent1")
key := state.TypedKey[int](ns, "counter", 0)  // "agent1.counter"

// Namespaced list key
listKey := state.TypedListKey[string](ns, "messages", 100, nil)  // "agent1.messages"
```

### Namespace Operations
```go
// Get view of namespace
view, _ := mgr.CreateReadView(ctx)
nsView := state.GetNamespaceView(view, ns)

// List all namespaces
namespaces := state.ListNamespaces(view)

// Copy namespace (requires target keys to be registered)
state.CopyNamespace(ctx, mgr, fromNS, toNS)
```

### Key Introspection
```go
// Check if key is namespaced
isNS := state.IsNamespaced("agent1.status")  // true
isNS = state.IsNamespaced("config")          // false

// Parse namespaced key
ns, local := state.ParseNamespacedKey("agent1.status")  // "agent1", "status"

// Extract namespace from key
ns := state.ExtractNamespace("agent1.status")  // Namespace{name: "agent1"}
```

## Design Philosophy

### Global First
The namespace system follows a **global-first** philosophy:
- Default behavior: Use simple global keys
- Opt-in complexity: Add namespaces only when you need isolation
- No forced prefixes: Most code doesn't need namespaces

### When to Use Namespaces
Use namespaces when you need:
- Multiple instances of the same component
- Isolation between subsystems
- Clear organizational boundaries
- Subgraph state separation

Don't use namespaces when:
- You have a single agent
- Keys are naturally unique
- Simplicity is more important than organization

## Related Examples
- `basic_agent/` - Simple agent without namespaces
- `supervisor_agent/` - Multi-agent coordination
- `subgraph/` - Subgraph isolation
