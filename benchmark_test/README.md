# Benchmark Tests

Performance benchmarks for AgentMesh core operations.

## Running Benchmarks

```bash
# Run all benchmarks
go test -bench=. ./benchmark_test/...

# Run specific benchmark
go test -bench=BenchmarkGraph_SimpleExecution ./benchmark_test/...

# With memory allocations
go test -bench=. -benchmem ./benchmark_test/...
```

## Benchmark Coverage

### Graph Execution
- `BenchmarkGraph_SimpleExecution` - Single node execution
- `BenchmarkGraph_LinearChain` - Chain of N nodes
- `BenchmarkGraph_Build` - Graph compilation
- `BenchmarkGraph_ParallelNodes` - Parallel node execution
- `BenchmarkGraph_MessageExecution` - Message-based execution
- `BenchmarkGraph_MessageChain` - Chained message processing
- `BenchmarkGraph_PrebuiltExecution` - Pre-compiled graph execution
- `BenchmarkGraph_PrebuiltMessageExecution` - Pre-compiled message execution

## Performance Characteristics

**Graph Execution:**
- Linear scaling with graph depth
- Compilation is fast and done once
- Pre-built graphs have lower per-execution overhead

## Comparing with Previous Results

To track performance regressions:

```bash
# Save baseline
go test -bench=. ./benchmark_test/... > baseline.txt

# Compare after changes
go test -bench=. ./benchmark_test/... > current.txt
benchstat baseline.txt current.txt
```
