# Graph Introspection Example

This example demonstrates AgentMesh's comprehensive graph introspection API, which allows you to inspect, analyze, and debug compiled graphs.

## Features Demonstrated

### 1. **Node Inspection**
- List all nodes in the graph
- Get detailed information about specific nodes
- Check node types, edge counts, and configurations

### 2. **Topology Analysis**
- Identify entry and exit points
- Find conditional nodes
- Calculate maximum graph depth
- Estimate total execution paths

### 3. **Graph Metrics**
- Total node and edge counts
- Fan-in/fan-out statistics
- Cyclomatic complexity calculation
- Node distribution by type

### 4. **Dependency Analysis**
- Direct predecessors and successors
- Transitive dependency closure
- Node depth from START

### 5. **Execution Path Discovery**
- Enumerate all possible execution paths
- Trace conditional branching
- Identify potential bottlenecks

### 6. **JSON Export**
- Export complete topology as JSON
- Enable external visualization tools
- Support for graph analysis pipelines

### 7. **Runtime Metrics**
- Track execution progress (superstep count)
- Monitor completed nodes
- Detect paused nodes (human-in-the-loop)

### 8. **Mermaid Flowchart Generation**
- Generate Mermaid flowchart syntax
- Visualize graph structure with proper node shapes
- Save flowchart to .mmd file for documentation

## Running the Example

```bash
cd examples/graph_introspection
go run main.go
```

## Output

The example produces a comprehensive report including:

- **All Nodes**: Complete list of nodes in the graph
- **Node Details**: Type, edges, conditional status, retry policy
- **Topology Overview**: Entry/exit points, conditional nodes, depth
- **Graph Metrics**: Statistics about graph structure
- **Dependencies**: For each node, show predecessors/successors
- **Execution Paths**: All possible paths from START to END
- **JSON Export**: Machine-readable topology for external tools
- **Runtime Metrics**: Execution state after graph completion
- **Mermaid Flowchart**: Visual representation saved to `graph.mmd`

## Use Cases

### Debugging
Use introspection to:
- Verify graph structure before execution
- Identify unreachable nodes
- Check conditional routing logic
- Debug dependency issues

### Monitoring
Track runtime state:
- Current superstep number
- Which nodes have completed
- Which nodes are paused (awaiting human input)

### Visualization
Export graph data for:
- Custom visualization tools
- Documentation generation
- Performance analysis
- Architecture diagrams

### Testing
Validate graph properties:
- Ensure correct topology
- Verify edge connections
- Check conditional branching
- Measure complexity metrics

## API Reference

### Basic Introspection

```go
compiled, _ := builder.Compile()

// Get all node names
nodes := compiled.GetNodes()

// Get node information
info, _ := compiled.GetNodeInfo("my_node")
fmt.Printf("Node has %d incoming edges\n", info.IncomingEdges)

// Get all edges
edges := compiled.GetEdges()
```

### Topology Analysis

```go
// Get complete topology
topo := compiled.GetTopology()
fmt.Printf("Entry points: %v\n", topo.EntryPoints)
fmt.Printf("Max depth: %d\n", topo.MaxDepth)

// Get metrics
metrics := compiled.GetMetrics()
fmt.Printf("Cyclomatic complexity: %d\n", metrics.CyclomaticComplexity)
```

### Dependency Analysis

```go
// Get dependencies for a node
deps, _ := compiled.GetDependencies("router")
fmt.Printf("Direct predecessors: %v\n", deps.DirectPredecessors)
fmt.Printf("All successors: %v\n", deps.AllSuccessors)
fmt.Printf("Depth: %d\n", deps.Depth)
```

### Execution Paths

```go
// Get possible execution paths (limit to 100)
paths := compiled.GetExecutionPath(100)
for i, path := range paths {
    fmt.Printf("Path %d: %v\n", i+1, path)
}
```

### Runtime Monitoring

```go
// Execute graph
_, _ = graph.Last(compiled.Run(ctx, nil))

// Check runtime state
metrics := compiled.GetMetrics()
fmt.Printf("Current superstep: %d\n", metrics.CurrentSuperstep)
fmt.Printf("Completed: %v\n", metrics.CompletedNodes)
fmt.Printf("Paused: %v\n", metrics.PausedNodes)
```

### Flowchart Generation

```go
// Generate Mermaid flowchart (top-down direction)
flowchart := compiled.GenerateMermaidFlowchart("TD")

// Save to file
os.WriteFile("graph.mmd", []byte(flowchart), 0644)

// Supported directions: TD (top-down), LR (left-right), BT (bottom-top), RL (right-left)
flowchartLR := compiled.GenerateMermaidFlowchart("LR")
```

The generated Mermaid syntax includes:
- **Stadium shapes** for START and END nodes: `([label])`
- **Diamond shapes** for conditional nodes: `{label}`
- **Rectangle shapes** for standard nodes: `[label]`
- **Solid arrows** for direct edges: `-->`
- **Dashed arrows** for conditional branches: `-.->|label|`

## JSON Export Format

The topology can be exported as JSON for external tools:

```json
{
  "nodes": [
    {
      "name": "router",
      "type": "standard",
      "incoming_edges": 1,
      "outgoing_edges": 0,
      "is_conditional": true,
      "is_conditional_gate": false,
      "has_retry_policy": false
    }
  ],
  "edges": [
    {
      "from": "router",
      "to": "",
      "type": "conditional",
      "conditional_targets": ["path_a", "path_b"]
    }
  ],
  "entry_points": ["input_validator"],
  "exit_points": ["aggregator"],
  "conditional_nodes": ["router"],
  "max_depth": 3,
  "total_paths": 2
}
```

## Performance Considerations

Introspection methods are designed to be efficient:

- **GetNodes()**, **GetNodeInfo()**: O(1) or O(n) - very fast
- **GetTopology()**: O(n + e) where n=nodes, e=edges - fast
- **GetMetrics()**: O(n + e) - fast
- **GetDependencies()**: O(n + e) with DFS - moderate
- **GetExecutionPath()**: Can be expensive for complex graphs with many branches

For graphs with extensive conditional branching, limit the number of paths returned by `GetExecutionPath(maxPaths)`.

## Integration Examples

### CI/CD Pipeline
```go
// Validate graph structure in tests
func TestGraphStructure(t *testing.T) {
    compiled, _ := buildGraph()
    metrics := compiled.GetMetrics()
    
    // Ensure complexity is reasonable
    assert.Less(t, metrics.CyclomaticComplexity, 20)
    
    // Verify no isolated nodes
    topo := compiled.GetTopology()
    assert.Empty(t, topo.IsolatedNodes)
}
```

### Documentation Generation
```go
// Generate graph documentation
topo := compiled.GetTopology()
json.NewEncoder(docFile).Encode(topo)

// Generate Mermaid flowchart for visual documentation
flowchart := compiled.GenerateMermaidFlowchart("TD")
os.WriteFile("docs/graph.mmd", []byte(flowchart), 0644)
```

### Performance Monitoring
```go
// Track execution progress
metrics := compiled.GetMetrics()
prometheus.RecordSuperstep(metrics.CurrentSuperstep)
prometheus.RecordCompleted(len(metrics.CompletedNodes))
```

## See Also

- [Core Concepts](../../docs/core-concepts.md)
- [Architecture Documentation](../../docs/architecture.md)
- [Observability Guide](../../docs/observability.md)
