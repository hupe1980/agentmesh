# Example: Subgraph

## Overview
Demonstrates multi-stage data processing pipeline using nested subgraphs. Shows how to compose complex workflows from smaller, reusable graph components.

## Key Concepts
- **Subgraph Composition**: Nested graphs as building blocks
- **Modular Workflows**: Reusable pipeline stages
- **State Mapping**: Transform state between subgraphs
- **Isolation**: Independent execution contexts

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

    g.AddNode(&graph.BaseCommandNode{
        NodeName:        "validate_format",
        DeclaredTargets: []string{"validate_schema"},
        Fn: func(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
            updates := map[string]any{"format_valid": true}
            return graph.Goto(updates, "validate_schema"), nil
        },
    })

    g.AddNode(&graph.BaseCommandNode{
        NodeName:        "validate_schema",
        DeclaredTargets: []string{graph.EndNode},
        Fn: func(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
            updates := map[string]any{"schema_valid": true}
            return graph.End(updates), nil
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
    pipeline.AddNode(&graph.BaseCommandNode{
        NodeName:        "validation_stage",
        DeclaredTargets: []string{"enrichment_stage"},
        Fn: func(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
            // Run validation subgraph
            result, _ := graph.Last(validation.Run(ctx, nil))
            return graph.Goto(result.State, "enrichment_stage"), nil
        },
    })

    // Stage 2: Enrichment subgraph
    pipeline.AddNode(&graph.BaseCommandNode{
        NodeName:        "enrichment_stage",
        DeclaredTargets: []string{"analysis_stage"},
        Fn: func(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
            data := state.GetFromView(view, dataKey)
            result, _ := graph.Last(enrichment.Run(ctx, nil,
                graph.WithInput(map[string]any{"data": data}),
            ))
            return graph.Goto(result.State, "analysis_stage"), nil
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
