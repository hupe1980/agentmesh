# Graph Validation Example

This example demonstrates AgentMesh's comprehensive graph validation layer that catches errors at compile time, before runtime.

## Features Demonstrated

1. **Valid Graph**: Shows successful compilation and execution
2. **Invalid Graph**: Demonstrates validation catching missing node references
3. **Strict Validation**: Shows enforced constraints (no unreachable nodes, cycles, dead ends)
4. **Custom Validation**: Demonstrates fine-grained validation control

## Running the Example

```bash
cd examples/graph_validation
go run main.go
```

## Expected Output

```
=== Graph Validation Examples ===

--- Example 1: Valid Graph ---
Processing...
✓ Graph compiled successfully
✓ Graph executed successfully

--- Example 2: Invalid Graph (Caught at Compile Time) ---
✓ Validation caught error:
compilation failed: graph validation failed with 1 error(s):
  1. missing_node: edge references non-existent target node "non_existent_node"

--- Example 3: Strict Validation ---
✓ Default validation passed (unreachable node allowed)
✓ Strict validation caught unreachable node:
compilation failed: graph validation failed with 1 error(s):
  1. unreachable_node: node "unreachable" is not reachable from START

--- Example 4: Custom Validation Options ---
✓ Default validation passed (cycle allowed for iterative pattern)
✓ Strict validation caught cycle:
compilation failed: graph validation failed with 1 error(s):
  1. cycle_detected: cycle detected: agent -> evaluator -> agent
✓ Custom validation passed (cycles allowed, reachability enforced)

=== Validation Examples Complete ===
```

## Validation Modes

### Default Validation (Permissive)
- Allows unreachable nodes (they simply won't execute)
- Allows dead-end nodes (valid for logging, metrics)
- Allows cycles (needed for iterative algorithms)
- Checks: missing nodes, nil functions, invalid edge patterns

### Strict Validation (Production)
- Rejects unreachable nodes
- Rejects dead-end nodes
- Rejects cycles (usually a bug)
- Enforces START and END connections
- Use for production deployments

### Custom Validation
- Fine-grained control over individual checks
- Mix and match validation rules
- Example: Allow cycles for iterative patterns while enforcing reachability

## API Usage

```go
// Default validation
runnable, err := exec.CompileGraph(g)

// Strict validation
runnable, err := exec.CompileGraph(g,
    exec.WithStrictValidation())

// Custom validation
runnable, err := exec.CompileGraph(g,
    exec.WithValidation(compile.ValidationOptions{
        AllowCycles:      true,
        AllowUnreachable: false,
        AllowDeadEnds:    false,
    }))

// Disable validation (use with caution)
runnable, err := exec.CompileGraph(g,
    exec.WithoutValidation())
```

## Validation Error Types

- `missing_node`: Edge references non-existent node
- `invalid_node`: Node has nil RunFunc, empty name, or reserved name
- `invalid_edge`: Edge has invalid pattern (to START, from END)
- `invalid_condition`: Conditional edge has nil condition function
- `cycle_detected`: Graph contains a cycle
- `unreachable_node`: Node not reachable from START
- `dead_end_node`: Node has no path to END
- `empty_graph`: Graph has no nodes
- `missing_start`: No nodes connected from START
- `missing_end`: No nodes connected to END
