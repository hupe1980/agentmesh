# Subgraph Composition

## Overview
Demonstrates composing complex graphs from reusable subgraph components using `graph.Subgraph()`.
Shows how to build isolated subgraphs with type-safe input/output mapping.

## Key Concepts
- **graph.Subgraph()**: Embed a compiled subgraph as a node in a parent graph
- **Input Mapper**: Transform parent messages into subgraph input messages
- **Output Mapper**: Transform subgraph output message into parent state updates
- **State Isolation**: Each subgraph has its own state, cannot access parent state
- **Reusability**: Build once, use in multiple parent graphs

## Running
```bash
go run examples/subgraph/main.go
```

## Expected Output
```
=== Subgraph Composition Example ===
  Demonstrates graph.Subgraph() for composing reusable graphs

  [start] Beginning workflow
  [parent] Mapping input to validation subgraph:   Raw Data  
    [validate] Checking format of:   Raw Data  
    [validate] Checking content of:   Raw Data  
  [parent] Got validation result: validated:  Raw Data  
  [parent] Mapping input to transform subgraph: validated:  Raw Data  
    [transform] Normalized: validated:  raw data
    [transform] Enriched: enriched(validated:  raw data)
  [parent] Got transform result: enriched(validated:  raw data)

  Workflow Summary:
    Final result: enriched(validated:  raw data)
    Steps executed:
      1. Started main workflow
      2. Validation completed
      3. Transform completed

  Subgraph features:
    • graph.Subgraph(sub, inputMapper, outputMapper)
    • Subgraphs have isolated state
    • Input/output mappers bridge parent ↔ child messages
    • Subgraphs can be reused across multiple nodes
```

## Code Walkthrough

### 1. Define State Keys
```go
// Parent graph keys for tracking workflow state
var (
    stepsKey = graph.NewListKey[string]("steps")
)
```

### 2. Create a Reusable Subgraph
```go
func createValidationSubgraph() *graph.Graph {
    g := graph.New(stepsKey)

    g.Node("validate_format", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
        lastMsg := graph.LastMessage(scope)
        input := ""
        if lastMsg != nil {
            input = lastMsg.String()
        }
        if strings.TrimSpace(input) == "" {
            return graph.Fail(fmt.Errorf("empty input"))
        }
        return graph.To("validate_content")
    }, "validate_content")

    g.Node("validate_content", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
        lastMsg := graph.LastMessage(scope)
        input := ""
        if lastMsg != nil {
            input = lastMsg.String()
        }
        validatedMsg := message.NewAIMessageFromText("validated:" + input)
        return graph.Reply(validatedMsg).End()
    }, graph.END)

    g.Start("validate_format")
    compiled, _ := g.Build()
    return compiled
}
```

### 3. Use graph.Subgraph() to Embed in Parent
```go
// Use graph.Subgraph() to embed the validation subgraph
g.Node("run_validation", graph.Subgraph(
    validationSubgraph,
    // Input mapper: parent messages -> subgraph input messages
    func(ctx context.Context, view graph.ReadOnlyScope) ([]message.Message, error) {
        lastMsg := graph.LastMessage(view)
        if lastMsg != nil {
            return []message.Message{lastMsg}, nil
        }
        return nil, nil
    },
    // Output mapper: subgraph output message -> parent state updates
    func(ctx context.Context, output message.Message) (graph.Updates, error) {
        return graph.Updates{
            graph.MessagesKeyName: []message.Message{output},
            stepsKey.Name():       []string{"Validation completed"},
        }, nil
    },
), "next_node")
```

### 4. Chain Multiple Subgraphs
```go
g.Start("start")

g.Node("start", startFn, "run_validation")
g.Node("run_validation", graph.Subgraph(validationSub, inMap, outMap), "run_transform")
g.Node("run_transform", graph.Subgraph(transformSub, inMap, outMap), "finalize")
g.Node("finalize", finalizeFn, graph.END)
```

## API Reference

### graph.Subgraph()
```go
func Subgraph(
    sub *Graph,                                                        // The subgraph to embed
    inputMapper func(ctx, view ReadOnlyScope) ([]message.Message, error),  // Maps parent messages to subgraph input
    outputMapper func(ctx, output message.Message) (Updates, error),       // Maps subgraph output to parent updates
) NodeFunc
```

### graph.Updates
```go
// Updates is a map of key names to values for batch state updates
type Updates map[string]any

// Example usage in output mapper:
return graph.Updates{
    graph.MessagesKeyName: []message.Message{output},
    stepsKey.Name():       []string{"Step completed"},
}, nil
```

## Workflow Architecture

```
Parent Graph:
┌─────────────────────────────────────────────┐
│ start                                       │
│   └─ Set initial input                     │
└───────────────┬─────────────────────────────┘
                ↓
┌─────────────────────────────────────────────┐
│ run_validation (graph.Subgraph)             │
│   ┌─────────────────────────────────────┐   │
│   │ Validation Subgraph (isolated)      │   │
│   │   validate_format → validate_content│   │
│   └─────────────────────────────────────┘   │
└───────────────┬─────────────────────────────┘
                ↓
┌─────────────────────────────────────────────┐
│ run_transform (graph.Subgraph)              │
│   ┌─────────────────────────────────────┐   │
│   │ Transform Subgraph (isolated)       │   │
│   │   normalize → enrich                │   │
│   └─────────────────────────────────────┘   │
└───────────────┬─────────────────────────────┘
                ↓
┌─────────────────────────────────────────────┐
│ finalize                                    │
│   └─ Display results                       │
└─────────────────────────────────────────────┘
```

## Benefits

### Modularity
- Develop and test subgraphs independently
- Reuse subgraphs across different parent graphs
- Easy to swap implementations

### State Isolation
- Subgraphs cannot access parent state directly
- Input/output mappers are the only interface
- Clear data contracts between components

### Type Safety
- Message input and output types are checked at compile time
- Input/output mappers use `[]message.Message` and `message.Message`
- Clear contracts between components

## Common Patterns

### ETL Pipeline
```go
g.Node("extract", graph.Subgraph(extractSub, ...))
g.Node("transform", graph.Subgraph(transformSub, ...))
g.Node("load", graph.Subgraph(loadSub, ...))
```

### Multi-Agent Workflow
```go
g.Node("research", graph.Subgraph(researchAgent, ...))
g.Node("analyze", graph.Subgraph(analysisAgent, ...))
g.Node("write", graph.Subgraph(writingAgent, ...))
```

## See Also
- [pkg/graph](../../pkg/graph) - Graph composition API
- [examples/conditional_flow](../conditional_flow) - Dynamic routing
- [examples/parallel_tasks](../parallel_tasks) - Parallel execution
- [examples/namespaces](../namespaces) - Namespace-based isolation
