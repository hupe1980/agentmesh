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
- `BenchmarkState_GetFromView` - Typed key lookup (~588ns)
- `BenchmarkState_ApplyUpdates` - State mutations (~141ns)
- `BenchmarkState_AddMessages` - Message appending (~150µs)
- `BenchmarkState_GetMessages` - Message retrieval (~5.2µs)

### Graph Execution
- `BenchmarkGraph_SimpleExecution` - Single node execution (~12.6µs)
- `BenchmarkGraph_LinearChain` - Chain of N nodes (length 5: ~33.5µs, length 10: ~61.5µs)
- `BenchmarkGraph_Compile` - Graph compilation (~4.7µs)

## Performance Characteristics

**State Access:**
- ~588ns per typed key access with snapshot + view pattern
- ~141ns for state mutations via ApplyUpdates
- Immutable snapshots enable lock-free concurrent reads
- Type safety with zero runtime overhead

**Graph Execution:**
- ~450ns overhead per node (from ~12.6µs total / ~29 operations)
- Linear scaling with graph depth
- Compilation is fast (~4.7µs) and done once

## Comparing with Previous Results

To track performance regressions:

```bash
# Save baseline
go test -bench=. ./benchmark_test/... > baseline.txt

# Compare after changes
go test -bench=. ./benchmark_test/... > current.txt
benchstat baseline.txt current.txt
```
