# Benchmark Tests

Performance benchmarks for AgentMesh core operations.

## Running Benchmarks

```bash
# Run all benchmarks
go test -bench=. ./benchmark_test/...

# Run specific benchmark
go test -bench=BenchmarkState_GetFromView ./benchmark_test/...

# With memory allocations
go test -bench=. -benchmem ./benchmark_test/...
```

## Benchmark Coverage

### State Operations
- `BenchmarkState_GetFromView` - Typed key lookup (~120ns)
- `BenchmarkState_ApplyUpdates` - State mutations (~101ns)
- `BenchmarkState_AddMessages` - Message appending (~86ns)
- `BenchmarkState_GetMessages` - Message retrieval (~104ns)

### Graph Execution
- `BenchmarkGraph_SimpleExecution` - Single node execution (~13.2µs)
- `BenchmarkGraph_LinearChain` - Chain of N nodes (length 5: ~34µs, length 10: ~62µs)
- `BenchmarkGraph_Compile` - Graph compilation (~3.8µs)

## Performance Characteristics

**State Access:**
- ~100-120ns per typed key access with snapshot + view pattern
- Immutable snapshots enable lock-free concurrent reads
- Type safety with zero runtime overhead

**Graph Execution:**
- ~450ns overhead per node (from ~13.2µs total / ~29 operations)
- Linear scaling with graph depth
- Compilation is fast (~3.8µs) and done once

## Comparing with Previous Results

To track performance regressions:

```bash
# Save baseline
go test -bench=. ./benchmark_test/... > baseline.txt

# Compare after changes
go test -bench=. ./benchmark_test/... > current.txt
benchstat baseline.txt current.txt
```
