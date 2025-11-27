# Subgraph Composition

## Overview
Demonstrates composing complex graphs from reusable subgraph components using `SubgraphNode`.
Shows how to build isolated subgraphs with type-safe input/output mapping.

## Key Concepts
- **SubgraphNode**: Wraps a compiled graph as a reusable node with type-safe I/O
- **InputMapper**: Type-safe function that extracts data from parent state → subgraph input
- **OutputMapper**: Type-safe function that converts subgraph output → parent state updates
- **State Isolation**: Each subgraph has its own state manager and cannot directly access parent state
- **Reusability**: Build once, use in multiple graphs - organize as Go packages/functions

## Running
```bash
cd examples/subgraph
go run main.go
```

## Expected Output
```
=== Multi-Stage Data Processing Pipeline ===

Stage 1: Validation
  [validate_format] Checking data format... ✓
  [validate_schema] Checking schema... ✓
  Status: validation_passed

Stage 2: Enrichment
  [lookup_metadata] Adding metadata...
  [calculate_derived] Computing derived fields...
  Status: enrichment_complete

Stage 3: Analysis
  [analyze_patterns] Analyzing patterns...
  [generate_insights] Generating insights...
  Status: analysis_complete

Stage 4: Report Generation
  [compile_report] Creating final report...
  Status: pipeline_complete

Pipeline completed successfully!
Final report: {...}
```

## Code Walkthrough

### 1. Create Validation Subgraph
```go
func createValidationSubgraph() *graph.Graph {
    stateManager := newStateManager()
    g, _ := graph.NewGraph(stateManager)

    g.AddNode(&graph.BaseNode{
        NodeName:        "validate_format",
        DeclaredTargets: []string{"validate_schema"},
        Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
            updates := state.Updates{"format_valid": true}
            return []string{"validate_schema"}, updates, nil
        },
    })

    g.AddNode(&graph.BaseNode{
        NodeName:        "validate_schema",
        DeclaredTargets: []string{graph.END},
        Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
            updates := state.Updates{"schema_valid": true}
            return []string{graph.END}, updates, nil
        },
    })

    g.SetEntryPoint("validate_format")
    return g
}
```

### 2. Compile Subgraphs
```go
compiledValidation, _ := validationSub.Compile()
compiledEnrichment, _ := enrichmentSub.Compile()
compiledAnalysis, _ := analysisSub.Compile()
```

### 3. Create Main Pipeline
```go
func createPipeline(validation, enrichment, analysis *graph.Compiled) *graph.Graph {
    stateManager := newStateManager()
    pipeline, _ := graph.NewGraph(stateManager)

    // Stage 1: Validation subgraph
    pipeline.AddNode(&graph.BaseNode{
        NodeName:        "validation_stage",
        DeclaredTargets: []string{"enrichment_stage"},
        Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
            // Run validation subgraph
            result, _ := graph.Last(validation.Run(ctx, nil))
            return []string{"enrichment_stage"}, result.State, nil
        },
    })

    // Stage 2: Enrichment subgraph
    pipeline.AddNode(&graph.BaseNode{
        NodeName:        "enrichment_stage",
        DeclaredTargets: []string{"analysis_stage"},
        Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
            data := state.GetFromView(view, dataKey)
            result, _ := graph.Last(enrichment.Run(ctx, nil,
                graph.WithInput(map[string]any{"data": data}),
            ))
            return []string{"analysis_stage"}, result.State, nil
        },
    })

    // Stage 3: Analysis subgraph (similar pattern)
    // ...

    pipeline.SetEntryPoint("validation_stage")
    return pipeline
}
```

### 4. Execute Pipeline
```go
compiled, _ := pipeline.Compile()
result, _ := graph.Last(compiled.Run(ctx, nil,
    graph.WithInput(map[string]any{
        "data": rawData,
    }),
))
```

## Workflow Architecture

```
Main Pipeline:
┌─────────────────────────────────────────────┐
│ validation_stage                            │
│   ├─ validate_format  (subgraph)           │
│   └─ validate_schema                       │
└───────────────┬─────────────────────────────┘
                ↓
┌─────────────────────────────────────────────┐
│ enrichment_stage                            │
│   ├─ lookup_metadata  (subgraph)           │
│   └─ calculate_derived                     │
└───────────────┬─────────────────────────────┘
                ↓
┌─────────────────────────────────────────────┐
│ analysis_stage                              │
│   ├─ analyze_patterns  (subgraph)          │
│   └─ generate_insights                     │
└───────────────┬─────────────────────────────┘
                ↓
┌─────────────────────────────────────────────┐
│ report_stage                                │
│   └─ compile_report                        │
└─────────────────────────────────────────────┘
```

## What This Example Teaches
- ✅ Subgraph composition
- ✅ Modular workflow design
- ✅ Multi-stage pipelines
- ✅ State isolation and mapping
- ✅ Reusable components

## Benefits

### Modularity
- Develop and test stages independently
- Reuse subgraphs across pipelines
- Easy to swap implementations

### Clarity
- Clear separation of concerns
- Self-documenting architecture
- Easier to understand complex workflows

### Maintainability
- Changes isolated to specific subgraphs
- Independent versioning
- Simpler debugging

## Common Patterns

### ETL Pipeline
```
Extract → Transform → Load
```

### Data Processing
```
Validate → Clean → Enrich → Analyze → Report
```

### Multi-Agent Workflow
```
Research Agent → Analysis Agent → Writing Agent → Review Agent
```

## Next Steps
- Build modular multi-stage workflows
- Create reusable subgraph library
- Implement dynamic subgraph selection
- See **examples/parallel_tasks** for parallel subgraphs

## See Also
- [pkg/graph](../../pkg/graph) - Graph composition API
- [examples/conditional_flow](../conditional_flow) - Dynamic routing
- [examples/parallel_tasks](../parallel_tasks) - Parallel execution
