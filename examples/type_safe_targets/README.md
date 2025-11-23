# Type-Safe Targets Example

This example demonstrates **Phase 5** of the Command pattern: type-safe routing targets using `TargetSet`.

## What is TargetSet?

`TargetSet` provides compile-time type safety for routing targets in AgentMesh graphs. Instead of passing string slices to `AddCommandNode`, you create a `TargetSet` that explicitly declares all possible routing destinations.

## Benefits

1. **Compile-time Safety**: All routing targets declared upfront
2. **IDE Autocomplete**: Type `targets.Get("")` and see available options
3. **Typo Protection**: `Get("typo")` returns `""` instead of panicking
4. **Self-Documenting**: TargetSet clearly shows all possible routes
5. **Refactoring-Friendly**: Renaming nodes caught immediately

## Usage

```go
// Create a TargetSet with all possible routing targets
targets := graph.NewTargetSet("validation", "processing", graph.EndNode)

// Add node with type-safe targets
builder.AddCommandNode("router", targets,
    func(ctx, view) (*graph.Command, error) {
        // Get target safely - returns "" if not in set
        validationTarget := targets.Get("validation")
        
        // Route using type-safe helper
        return targets.Goto(validationTarget, updates), nil
    },
)
```

## Comparison: Standard vs Type-Safe

### Standard AddCommandNode
```go
builder.AddCommandNode("router", graph.NewTargetSet("validation", "processing", graph.END),
    func(ctx, view) (*graph.Command, error) {
        // Manual string - typos caught only at runtime
        return graph.Goto("validaton", updates), nil  // Typo!
    },
)
```

### Type-Safe AddCommandNode (Recommended)
```go
targets := graph.NewTargetSet("validation", "processing", graph.END)

builder.AddCommandNode("router", targets,
    func(ctx, view) (*graph.Command, error) {
        // IDE autocomplete helps you get it right
        return targets.Goto(targets.Get("validation"), updates), nil
    },
)
```

## API Methods

### TargetSet Creation
```go
targets := graph.NewTargetSet("node_a", "node_b", graph.EndNode)
```

### Target Access
```go
target := targets.Get("node_a")           // Returns "node_a" or ""
exists := targets.Has("node_a")           // Returns true/false
all := targets.All()                      // Returns []string{"node_a", "node_b", "__end__"}
```

### Command Creation
```go
// Direct routing
cmd := targets.Goto(targets.Get("node_a"), updates)

// Single target shorthand
cmd := targets.GotoOne(targets.Get("node_a"), updates)

// End execution
cmd := targets.End(updates)

// Fluent API
cmd := targets.Update(updates).Goto(targets.Get("node_a"))
cmd := targets.Update(updates).End()
```

## Running the Example

```bash
cd examples/type_safe_targets
go run main.go
```

## Output

```
Type-safe targets demonstration:
- TargetSet ensures all routing targets are declared upfront
- targets.Get() provides safe access to target names
- Typos in target names return empty string, caught at runtime
- IDE autocomplete shows all available targets

Graph compiled successfully with type-safe routing!
  -> Router: Checking targets...
     Validation target: validation
     Processing target: processing
  -> Validation: Passed
  -> Processing: Complete
```

## When to Use

**Use Type-Safe Targets when:**
- Building complex routing logic with many possible paths
- Working on large graphs where typos are likely
- Want IDE autocomplete for target names
- Prefer explicit over implicit target declaration

**Use Standard AddCommandNode when:**
- Simple graphs with 1-2 routing targets
- Prototyping / quick experiments
- Targets are obvious from context

## See Also

- [Command Pattern Documentation](../../COMMAND.md)
- [Conditional Flow Example](../conditional_flow/) - Standard Command usage
- [Builder API Documentation](../../docs/builder-api.md)
