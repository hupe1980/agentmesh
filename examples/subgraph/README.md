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
    state := graph.NewStateManager(0)
    g := graph.NewGraph(state)
    
    g.AddNode(&graph.Node{
        Name: "validate_format",
        RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
            // Validate data format
            return &graph.NodeResult{
                Updates: map[string]any{
                    "format_valid": true,
                },
            }, nil
        },
    })
    
    g.AddNode(&graph.Node{
        Name: "validate_schema",
        RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
            // Validate schema
            return &graph.NodeResult{
                Updates: map[string]any{
                    "schema_valid": true,
                },
            }, nil
        },
    })
    
    g.AddEdge("validate_format", "validate_schema")
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
    state := graph.NewStateManager(0)
    pipeline := graph.NewGraph(state)
    
    // Stage 1: Validation subgraph
    pipeline.AddNode(&graph.Node{
        Name: "validation_stage",
        RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
            data, _ := s.Get("data").(map[string]any)
            result, _ := validation.Invoke(ctx, nil,
                graph.WithInput(map[string]any{"data": data}),
            )
            return &graph.NodeResult{
                Updates: result.State,
            }, nil
        },
    })
    
    // Stage 2: Enrichment subgraph
    pipeline.AddNode(&graph.Node{
        Name: "enrichment_stage",
        RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
            data, _ := s.Get("data").(map[string]any)
            result, _ := enrichment.Invoke(ctx, nil,
                graph.WithInput(map[string]any{"data": data}),
            )
            return &graph.NodeResult{
                Updates: result.State,
            }, nil
        },
    })
    
    // Connect stages
    pipeline.AddEdge("validation_stage", "enrichment_stage")
    pipeline.AddEdge("enrichment_stage", "analysis_stage")
    
    return pipeline
}
```

### 4. Execute Pipeline
```go
compiled, _ := pipeline.Compile()
result, _ := compiled.Invoke(ctx, nil,
    graph.WithInput(map[string]any{
        "data": rawData,
    }),
)
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
