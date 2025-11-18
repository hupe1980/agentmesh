// Package state provides a type-safe, BSP-compatible state management system.
//
// This package is designed specifically for Bulk-Synchronous Parallel (BSP)
// execution in the Pregel runtime, but works equally well with sequential execution.
//
// Key Features:
//
//   - Type-safe operations using generics (Key[T], ListKey[T])
//   - BSP-compatible: sync.Map for lock-free concurrent reads, synchronous updates
//   - Lock-free channel registry eliminates RWMutex contention during superstep execution
//   - Immutable snapshots for consistent superstep views
//   - Immutability enforcement: nodes receive ReadView for safe concurrent reads
//   - Explicit error handling (no silent failures)
//   - Zero-allocation streaming with iter.Seq
//
// Basic Usage:
//
//	// Define type-safe keys
//	var CounterKey = state.NewKey[int]("counter", 0)
//	var TaskListKey = state.NewListKey[string]("tasks", 100)
//
//	// For message history, use agent.MessagesKey from pkg/agent
//	// (messages are agent-level concept, not general state)
//
//	// Create state and register keys
//	st := state.NewState()
//	state.Register(st, CounterKey)
//	state.Register(st, TaskListKey)
//
//	// Type-safe operations
//	counter := state.Get(mgr, CounterKey)  // Returns int
//	state.Set(ctx, mgr, CounterKey, 42)    // Type-checked at compile time
//	state.Append(ctx, mgr, TaskListKey, "new task")
//
//	// BSP execution pattern
//	snap, _ := mgr.Snapshot(ctx)  // All vertices get consistent view
//	view, _ := mgr.CreateReadView(ctx)
//	result, _ := node.Run(ctx, view)  // Concurrent reads OK
//	mgr.ApplyUpdates(ctx, result.Updates)  // After BSP barrier
//
// BSP Compatibility:
//
// The state package is built for Pregel's BSP execution model:
//
//  1. Superstep N: All vertices read from immutable ReadView (concurrent, lock-free)
//  2. BSP Barrier: Wait for all vertices to complete
//  3. Apply Updates: Single writer calls Manager.ApplyUpdates() (exclusive lock)
//  4. Superstep N+1: Vertices see updated state
//
// This design ensures race-free execution without complex actor models or
// asynchronous coordination. The ChannelRegistry uses sync.Map for lock-free
// reads, eliminating RWMutex contention when all workers read state concurrently
// at superstep boundaries. This provides 10-100x better performance for
// read-heavy workloads typical of agent graphs.
package state
