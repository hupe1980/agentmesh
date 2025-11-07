# Example: Conditional Flow

## Overview
Demonstrates conditional routing and dynamic branching in AgentMesh graphs. Shows how to create decision trees where execution path depends on runtime state.

## Key Concepts
- **Conditional Edges**: Dynamic routing based on state
- **Decision Trees**: Branch to different nodes based on conditions
- **State-Based Routing**: Read state values to determine next node
- **TopicChannel**: Accumulate action history

## Running
```bash
cd examples/conditional_flow
go run main.go
```

## Expected Output
```
=== Conditional Flow Example: path_a ===

[start] Choice selected: path_a

[Router] Routing to: path_a_handler

[path_a_handler] Processing path A
  Action logged: Executed path A

[end] Workflow complete
  Final actions: [start, route_a, path_a_handler, end]

=== Conditional Flow Example: path_b ===

[start] Choice selected: path_b

[Router] Routing to: path_b_handler

[path_b_handler] Processing path B
  Action logged: Executed path B

[end] Workflow complete
  Final actions: [start, route_b, path_b_handler, end]
```

## Code Walkthrough

### 1. Define Router Function
```go
func routeByChoice(ctx context.Context, s graph.StateReader) (string, error) {
    choice, _ := s.Get("choice").(string)
    
    switch choice {
    case "path_a":
        return "path_a_handler", nil
    case "path_b":
        return "path_b_handler", nil
    default:
        return "error_handler", nil
    }
}
```

### 2. Add Conditional Edge
```go
builder.AddConditionalEdges("router", routeByChoice)
```

### 3. Create Branch Nodes
```go
builder.Node("path_a_handler", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    fmt.Println("Processing path A")
    return &graph.NodeResult{
        Updates: map[string]any{
            "result": "Path A executed",
        },
    }, nil
})

builder.Node("path_b_handler", /* similar */)
```

## Routing Patterns

### Simple If-Else
```go
func simpleRoute(ctx context.Context, s graph.StateReader) (string, error) {
    value, _ := s.Get("value").(int)
    if value > 10 {
        return "high_value_handler", nil
    }
    return "low_value_handler", nil
}
```

### Multi-Way Branch
```go
func multiRoute(ctx context.Context, s graph.StateReader) (string, error) {
    status, _ := s.Get("status").(string)
    switch status {
    case "pending":
        return "process", nil
    case "approved":
        return "execute", nil
    case "rejected":
        return "cancel", nil
    default:
        return "error", nil
    }
}
```

### Data-Driven Routing
```go
func dataRoute(ctx context.Context, s graph.StateReader) (string, error) {
    data, _ := s.Get("data").(map[string]any)
    
    if data["urgent"] == true {
        return "priority_queue", nil
    }
    if data["category"] == "support" {
        return "support_team", nil
    }
    return "general_queue", nil
}
```

## What This Example Teaches
- ✅ Conditional edge routing
- ✅ Dynamic execution paths
- ✅ State-based decision making
- ✅ Building decision trees
- ✅ Workflow branching patterns

## Common Use Cases

### Error Handling
```go
func errorRoute(ctx context.Context, s graph.StateReader) (string, error) {
    if err := s.Get("error"); err != nil {
        return "error_handler", nil
    }
    return "success_handler", nil
}
```

### Content Routing
```go
func contentRoute(ctx context.Context, s graph.StateReader) (string, error) {
    contentType, _ := s.Get("content_type").(string)
    return contentType + "_processor", nil
}
```

### Priority Queue
```go
func priorityRoute(ctx context.Context, s graph.StateReader) (string, error) {
    priority, _ := s.Get("priority").(int)
    if priority >= 9 {
        return "urgent", nil
    } else if priority >= 5 {
        return "normal", nil
    }
    return "low_priority", nil
}
```

## Next Steps
- Implement complex decision trees
- Add error handling routes
- Create content-based routing
- See **examples/subgraph** for nested workflows

## See Also
- [pkg/graph](../../pkg/graph) - Conditional edges API
- [examples/subgraph](../subgraph) - Complex workflows
- [examples/parallel_tasks](../parallel_tasks) - Parallel execution
