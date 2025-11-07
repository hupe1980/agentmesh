// Package pregel provides a generic Bulk Synchronous Parallel (BSP) computation engine
// implementing Google's Pregel model for distributed graph processing.
//
// # Overview
//
// This package implements the Pregel model with the following features:
//   - Vertices execute in parallel supersteps
//   - Communication via message passing between supersteps
//   - Global coordination via aggregators
//   - Computation quiesces when no messages remain
//   - Configurable worker pools for parallel execution
//   - Message combiners to reduce mailbox pressure
//
// The runtime is fully generic over state type S and message type M, making it
// reusable for any BSP computation (PageRank, shortest paths, graph coloring, etc.).
//
// # Key Concepts
//
// Superstep: A parallel computation phase where all active vertices execute concurrently.
// Vertices can send messages and contribute to aggregators during execution. Messages
// sent in superstep N are delivered in superstep N+1.
//
// Frontier: The set of vertices active in the next superstep. Vertices enter the frontier
// when they receive messages or are root nodes.
//
// Mailbox: Per-vertex message queue. Messages are buffered and delivered at the start of
// the next superstep. Mailbox size can be bounded to prevent memory exhaustion.
//
// Aggregator: Global reduction operation (sum, max, count, etc.) computed across all vertices
// in a superstep and made visible to all vertices in the next superstep. Useful for
// convergence detection, global counters, and coordination.
//
// Combiner: Optional function that merges multiple messages for the same target vertex,
// reducing mailbox size and improving performance.
//
// # Thread Safety
//
// The runtime is designed for concurrent use:
//   - Run() executes supersteps with a configurable worker pool
//   - Deliver() can be called concurrently with Run() to inject messages
//   - Multiple goroutines execute vertices in parallel within each superstep
//   - All internal state is protected by appropriate synchronization primitives
//
// # Example Usage
//
//	type MyState struct {
//	    Values map[string]float64
//	}
//
//	type MyMessage struct {
//	    Value float64
//	}
//
//	// Implement PregelGraph interface
//	graph := &myPregelGraph{
//	    state: MyState{Values: make(map[string]float64)},
//	}
//
//	// Create runtime with options
//	runtime := pregel.NewRuntime(graph, nil,
//	    pregel.WithMaxWorkers[MyState, MyMessage](4),
//	    pregel.WithMaxIterations[MyState, MyMessage](100),
//	    pregel.WithAggregators[MyState, MyMessage](map[string]Aggregator{
//	        "sum": &SumAggregator{},
//	    }),
//	)
//
//	// Execute computation
//	err := runtime.Run(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Get statistics
//	stats := runtime.Stats()
//	fmt.Printf("Completed in %d supersteps\n", stats.Supersteps)
//
// # Design Philosophy
//
// This package is intentionally pure and has NO dependencies on domain-specific
// concepts like agents, channels, or checkpoints. It can be used as a general-purpose
// BSP computation engine for any parallel graph algorithm.
//
// For agent-specific graph orchestration, see the pkg/graph package which builds
// on this engine using an adapter pattern.
//
// # References
//
// Original Pregel paper: https://research.google/pubs/pub37252/
// "Pregel: A System for Large-Scale Graph Processing" (Malewicz et al., 2010)
package pregel
