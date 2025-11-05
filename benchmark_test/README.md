# AgentMesh Benchmarks

This directory contains comprehensive benchmarks for the AgentMesh framework.

## Running Benchmarks

### Run all benchmarks
```bash
go test ./benchmark_test -bench=. -benchmem
```

### Run specific benchmark categories

**Graph execution benchmarks:**
```bash
go test ./benchmark_test -bench=BenchmarkGraph -benchmem
```

**State operation benchmarks:**
```bash
go test ./benchmark_test -bench=BenchmarkState -benchmem
```

**Comprehensive workflow benchmarks:**
```bash
go test ./benchmark_test -bench=BenchmarkComprehensive -benchmem
```

### Run with longer duration for more accurate results
```bash
go test ./benchmark_test -bench=. -benchmem -benchtime=1s
```

## Benchmark Files

- `graph_benchmark_test.go` - Core graph execution and state operation benchmarks
- `graph_comprehensive_benchmark_test.go` - Complex workflow pattern benchmarks
- `graph_state_benchmark_test.go` - Graph execution and parallelism benchmarks

## Key Metrics

The benchmarks measure:
- **State operations**: Get, Set, GetAll performance
- **Message handling**: Aggregation, retention, cloning
- **Graph compilation**: Builder to compiled graph conversion
- **Execution performance**: Single node, chains, parallel execution
- **Complex workflows**: Data pipelines, concurrent processing, aggregation patterns

## Example Output

```
BenchmarkState_Get-14                  12844522         9.632 ns/op        0 B/op        0 allocs/op
BenchmarkState_Set-14                   6445381        18.53 ns/op        64 B/op        1 allocs/op
BenchmarkGraph_10Nodes-14                284032         4293 ns/op      5728 B/op       73 allocs/op
```

## Continuous Integration

These benchmarks can be used for:
- Performance regression detection
- Comparing different implementations
- Profiling and optimization
- Documentation of performance characteristics
