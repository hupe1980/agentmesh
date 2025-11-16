# Package state

Complete state management for AgentMesh graph execution.

## Overview

This package provides all state management functionality for AgentMesh graphs, including:
- State reading/writing interfaces
- Channel-based state implementation  
- Execution tracking and pause/resume
- Aggregation support
- State persistence and checkpointing

**Package Size:** 2,076 lines across 9 files

## Architecture

```
pkg/state/
├── interfaces.go         - Core Reader/Writer interfaces (35 lines)
├── state_manager.go      - StateManager implementation (827 lines)
├── state_builder.go      - Fluent builder API (247 lines)
├── execution_state.go    - Execution tracking (275 lines)
├── execution_result.go   - Message result wrapper (92 lines)
├── storage.go            - State persistence (120 lines)
├── errors.go             - Error definitions (18 lines)
├── doc.go                - Package documentation (12 lines)
└── aggregators/
    └── aggregators.go    - Built-in aggregators (449 lines)
```

## Key Types

### Interfaces

```go
// Reader provides read-only access to state
type Reader interface {
    Get(key string) any
    GetAll() map[string]any
    MessagesSnapshot() []ExecutionResult
}

// Writer extends Reader with write operations
type Writer interface {
    Reader
    Set(key string, value any) error
    Update(ctx context.Context, updates map[string]any) error
    AddMessage(msg Message)
    Aggregate(name string, value any) error
}

// StateManager combines all state capabilities
type StateManager interface {
    Reader
    ChannelManager
    AggregateManager
    CheckpointManager
    Version() uint64
    Snapshot() map[string]any
    Clone() StateManager
}
```

### Implementations

- **ChannelState**: Main implementation using channels for state storage
- **ExecutionTracker**: Tracks which nodes have executed  
- **ExecutionState**: Manages pause/resume and supersteps
- **BufferedStateWriter**: Buffers writes for batch operations

## Usage

### Basic State Management

```go
// Create a state manager with unlimited messages
state, err := state.NewChannelState(0)
if err != nil {
    log.Fatal(err)
}

// Set values
state.Set("count", 42)
state.Set("name", "agent")

// Read values
count := state.Get("count").(int)
all := state.GetAll()
```

### Using the Builder

```go
// Fluent API for common patterns
state := state.NewStateBuilder().
    WithMessages(100).                    // Message history with limit
    WithLastValue("status", "pending").   // Latest-only channel
    WithCounter("iterations").            // Accumulating counter
    Build()
```

### Aggregation

```go
// Built-in aggregators
import "github.com/hupe1980/agentmesh/pkg/state/aggregators"

state.RegisterAggregator("total", &aggregators.SumAggregator{})
state.RegisterAggregator("average", &aggregators.AvgAggregator{})
state.RegisterAggregator("variance", &aggregators.VarianceAggregator{})

// Nodes can aggregate values
state.Aggregate("total", 10)

// Read aggregated results
total := state.GetAggregate("total").(int)
```

### Execution Tracking

```go
tracker := state.NewExecutionTracker()

// Mark nodes as executed
tracker.MarkCompleted("node1")
tracker.MarkCompleted("node2")

// Check execution status
executed := tracker.AllExecutedNodes()  // ["node1", "node2"]
count := tracker.CompletedCount()       // 2

// Pause/resume support
tracker.MarkPaused("node3")
isPaused := tracker.IsPaused("node3")   // true
```

## Refactoring History

### Phase 1 (November 2024)
- **Consolidated** from `pkg/runtime/state` → `pkg/state`
- **Moved** 1,941 lines to correct semantic location
- **Fixed** circular import issues
- **Result**: Single source of truth for state management

### Before Refactoring
```
pkg/state/           139 lines  (interfaces only)
pkg/runtime/state/ 1,941 lines  (implementation)
```

### After Refactoring  
```
pkg/state/         2,076 lines  (complete package)
pkg/runtime/state/    DELETED
```

## Design Principles

1. **Interface Segregation**: Small, focused interfaces (Reader, Writer)
2. **Semantic Location**: State belongs in `pkg/state`, not `pkg/runtime/state`
3. **Zero Dependencies**: Only depends on `pkg/channel`, `pkg/checkpoint`, `pkg/message`
4. **Thread-Safe**: All operations are safe for concurrent use
5. **Extensible**: Easy to add custom aggregators and channels

## Testing

The package includes comprehensive tests in the `pkg/graph` test suite:
- State operations (`state_manager_test.go`)
- State builder patterns (`state_builder_test.go`)  
- Message handling (`message_retention_test.go`)
- Buffered operations (`buffered_state_test.go`)
- Storage and persistence (`storage_test.go`)

## Related Packages

- **pkg/graph**: Uses state for graph execution
- **pkg/channel**: Underlying channel implementation
- **pkg/checkpoint**: State persistence
- **pkg/message**: Message types stored in state

## Future Improvements

See [REFACTORING_NEW.md](../../REFACTORING_NEW.md) for planned enhancements:
- Phase 5: Interface segregation improvements
- Phase 6: Type-safe state access with generics
- Phase 7: Enhanced observability integration
