# Architecture Refactoring Plan: StateManager & Executor

**Version**: 2.0  
**Status**: Design Complete, Implementation In Progress  
**Breaking Changes**: Yes  

---

## Overview

This document outlines the refactoring of AgentMesh's core architecture to introduce cleaner separation of concerns through **StateManager** and **Executor** interfaces. This addresses architectural complexity identified in the code review (FINDINGS.md Section 7).

---

## Current Architecture Problems

### 1. Tight Coupling
- `CompiledGraph` has too many responsibilities:
  - Graph topology management
  - State management (channels, aggregates)
  - Execution coordination
  - Checkpoint persistence
  - Runtime lifecycle

### 2. State Management Scattered
- State logic split between:
  - `GraphState` (channels)
  - `CompiledGraph` (lifecycle)
  - `graphRuntime` (execution state)
  - Aggregate management separate from channels

### 3. Hard to Extend
- Cannot easily add new execution strategies
- Difficult to test state management in isolation
- No clear boundaries between components

---

## Proposed Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    CompiledGraph (Simplified)                 │
│  Responsibilities:                                            │
│    - Graph topology (immutable)                               │
│    - Public API (Invoke, Stream)                              │
│    - Coordination between StateManager and Executor           │
└────────────────┬─────────────────────────┬───────────────────┘
                 │                         │
                 │ delegates to            │ delegates to
                 ▼                         ▼
    ┌────────────────────────┐  ┌────────────────────────────┐
    │    StateManager        │  │       Executor             │
    │                        │  │                            │
    │  • Channels            │  │  • Execution Strategy      │
    │  • Checkpoints         │  │  • Superstep Coordination  │
    │  • Aggregates          │  │  • Event Streaming         │
    │  • Thread-safe access  │  │  • Pause/Resume            │
    └────────────────────────┘  └──────────┬─────────────────┘
                                           │
                                           │ implementation
                                           ▼
                                  ┌──────────────────┐
                                  │ PregelExecutor   │
                                  │                  │
                                  │ • BSP Model      │
                                  │ • Worker Pool    │
                                  │ • Mailbox System │
                                  └──────────────────┘
```

---

## Component Design

### 1. StateManager Interface

**Location**: `pkg/graph/state_manager.go`

**Responsibilities**:
- Own all state concerns (single source of truth)
- Manage channels (Topic, LastValue, BinaryOp)
- Handle checkpoint persistence/restoration
- Manage aggregate values
- Provide thread-safe state access

**Key Methods**:
```go
type StateManager interface {
    // State access
    Get(key string) any
    GetAll() map[string]any
    MessagesSnapshot() []message.Message
    
    // Channel management
    AddChannel(ch channel.Channel)
    UpdateChannel(ctx context.Context, name string, value any) error
    UpdateChannels(ctx context.Context, updates map[string]any) error
    
    // Aggregate management
    GetAggregate(name string) any
    SetAggregates(aggregates map[string]any)
    RecordAggregation(name string, value any) error
    
    // Checkpoint management
    SaveCheckpoint(ctx context.Context, runID string, superstep int64, metadata map[string]any) error
    LoadCheckpoint(ctx context.Context, runID string) (*checkpoint.Checkpoint, error)
    
    // Lifecycle
    Snapshot() map[string]any
    Clone() StateManager
}
```

**Implementation**: `DefaultStateManager`
- Thread-safe with `sync.RWMutex`
- Wraps `channel.ChannelSet`
- Integrates checkpoint backends
- No execution logic

### 2. Executor Interface

**Location**: `pkg/graph/executor.go`

**Responsibilities**:
- Abstract execution strategy
- Coordinate superstep execution
- Emit execution events
- Manage pause/resume
- Provide execution statistics

**Key Methods**:
```go
type Executor interface {
    // Execution
    Execute(ctx context.Context, initialMessages []message.Message, options ExecuteOptions) (*InvokeResult, error)
    Stream(ctx context.Context, initialMessages []message.Message, options ExecuteOptions) (<-chan StreamEvent, <-chan error)
    
    // Control
    Pause(nodeName string)
    Resume(nodeName string)
    IsPaused(nodeName string) bool
    
    // Observability
    CurrentSuperstep() int64
    Stats() ExecutionStats
}
```

**Implementation**: `PregelExecutor`
- Wraps existing `internal/pregel.Runtime`
- Implements BSP execution model
- Delegates state management to StateManager
- Provides event streaming

### 3. CompiledGraph (Refactored)

**Responsibilities** (reduced):
- Hold graph topology (nodes, edges, conditionals)
- Provide public API (Invoke, Stream)
- Coordinate StateManager and Executor
- Apply rate limiting and retry policies

**Removed Responsibilities**:
- Direct state management → delegated to StateManager
- Execution logic → delegated to Executor
- Checkpoint persistence → delegated to StateManager

---

## Migration Path

### Phase 1: Create New Interfaces (DONE)
- ✅ Create `state_manager.go` with StateManager interface
- ✅ Create `executor.go` with Executor interface
- ✅ Implement `DefaultStateManager`
- ⏸️ Implement `PregelExecutor` (deferred)

### Phase 2: Refactor CompiledGraph (PENDING)
- Update Builder.Compile() to instantiate StateManager
- Update Builder.Compile() to instantiate PregelExecutor
- Refactor CompiledGraph to delegate to new components
- Remove direct state management from CompiledGraph

### Phase 3: Update Tests (PENDING)
- Fix compilation errors
- Update test assertions
- Add new tests for StateManager
- Add new tests for PregelExecutor

### Phase 4: Documentation (PENDING)
- Update architecture.md with new design
- Create migration guide for users
- Update examples to use new API
- Document breaking changes

---

## Breaking Changes

### API Changes

**Before (v1.x)**:
```go
state := graph.NewGraphState(100)
state.AddChannel(channel.NewLastValueChannel("status"))

builder := graph.NewBuilder()
builder.SetState(state)
// ...
compiled, err := builder.Compile()
```

**After (v2.0)**:
```go
stateManager := graph.NewStateManager(100)
stateManager.AddChannel(channel.NewLastValueChannel("status"))

builder := graph.NewBuilder()
builder.SetStateManager(stateManager)
// ...
compiled, err := builder.Compile()
```

### Internal Changes
- `CompiledGraph.State()` → removed (use `StateManager()` instead)
- `CompiledGraph.runtime` → removed (internal to Executor)
- Direct state mutations → must go through StateManager
- Execution control → through Executor interface

---

## Benefits

### 1. Cleaner Architecture
- **Single Responsibility**: Each component has one clear purpose
- **Separation of Concerns**: State, execution, and topology are independent
- **Better Testability**: Can test StateManager and Executor in isolation

### 2. Extensibility
- Easy to add new execution strategies (SimpleExecutor, DistributedExecutor)
- Can swap StateManager implementations (e.g., distributed state)
- Pluggable checkpoint backends already supported

### 3. Maintainability
- Clearer code organization
- Easier to reason about component interactions
- Better documentation of responsibilities

### 4. Future-Proof
- Foundation for distributed execution
- Support for different execution models
- Easier to add new features

---

## Implementation Status

### Completed
- ✅ StateManager interface design
- ✅ DefaultStateManager implementation
- ✅ Executor interface design
- ✅ StateReaderAdapter/StateWriterAdapter for backward compatibility
- ✅ Documentation of design

### In Progress
- ⏸️ PregelExecutor implementation (70% complete)
- ⏸️ CompiledGraph refactoring (not started)

### Pending
- ⏸️ Builder.Compile() updates
- ⏸️ Test suite updates
- ⏸️ Migration guide
- ⏸️ Example updates

---

## Decision: Defer to v2.0

**Reason**: This is a significant breaking change that requires:
1. Extensive test updates (100+ test files)
2. Migration guide for users
3. Example updates
4. Documentation overhaul

**Current Status**: 
- Design is complete and validated
- Core interfaces implemented
- Foundation laid for v2.0

**v1.x Strategy**:
- All Priority 1 items complete (concurrency, checkpoints, docs, panic recovery)
- Framework is production-ready with current architecture
- Focus on incremental improvements (StateBuilder, evaluation, templates)

**v2.0 Plan**:
- Complete refactoring when ready for breaking changes
- Provide comprehensive migration guide
- Deprecate old APIs in v1.5 with warnings
- Clean break in v2.0

---

## Files Created

1. **pkg/graph/state_manager.go** (400+ lines)
   - StateManager interface
   - DefaultStateManager implementation
   - StateReaderAdapter/StateWriterAdapter

2. **pkg/graph/executor.go** (150+ lines)
   - Executor interface
   - ExecuteOptions configuration
   - ExecutionStats observability

3. **docs/refactoring_plan.md** (this file)
   - Architecture design
   - Migration path
   - Implementation status

---

## References

- **FINDINGS.md Section 7**: Original redesign recommendations
- **docs/architecture.md**: Current architecture documentation
- **internal/pregel/**: BSP execution engine
- **pkg/graph/compiled_graph.go**: Current implementation

---

**Last Updated**: November 5, 2025  
**Next Review**: When planning v2.0 release
