// Package state provides a type-safe, BSP-compatible state management system.
//
// This package is designed specifically for Bulk-Synchronous Parallel (BSP)
// execution in the Pregel runtime, but works equally well with sequential execution.
//
// Key Features:
//
//   - Type-safe operations using generics (Key[T], ListKey[T])
//   - BSP-compatible: RWMutex for concurrent reads, synchronous updates
//   - Immutable snapshots for consistent superstep views
//   - Interface segregation: nodes receive ReadView, not mutable State
//   - Explicit error handling (no silent failures)
//   - Zero-allocation streaming with iter.Seq
//
// Basic Usage:
//
//	// Define type-safe keys
//	var CounterKey = state.NewKey[int]("counter", 0)
//	var MessagesKey = state.NewListKey[message.Message]("messages", 100)
//
//	// Create state and register keys
//	st := state.NewState()
//	state.Register(st, CounterKey)
//	state.Register(st, MessagesKey)
//
//	// Type-safe operations
//	counter := state.Get(st, CounterKey)  // Returns int
//	state.Set(ctx, st, CounterKey, 42)    // Type-checked at compile time
//	state.Append(ctx, st, MessagesKey, msg)
//
//	// BSP execution pattern
//	snap := st.Snapshot()  // All vertices get consistent view
//	view := state.NewReadView(snap)
//	result, _ := node.Run(ctx, view)  // Concurrent reads OK
//	st.ApplyUpdates(ctx, result.Updates)  // After BSP barrier
//
// BSP Compatibility:
//
// The state package is built for Pregel's BSP execution model:
//
//  1. Superstep N: All vertices read from immutable Snapshot (concurrent, safe)
//  2. BSP Barrier: Wait for all vertices to complete
//  3. Apply Updates: Single writer calls ApplyUpdates() (exclusive lock)
//  4. Superstep N+1: Vertices see updated state
//
// This design ensures race-free execution without complex actor models or
// asynchronous coordination - just simple RWMutex + BSP barriers.
package state
