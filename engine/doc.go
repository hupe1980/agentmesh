// Package engine implements the core orchestration layer for AgentMesh.
//
// The Engine serves as the central coordination hub that manages the complete
// lifecycle of multi-agent conversations and workflows. It bridges the gap
// between high-level AgentMesh operations and low-level agent implementations,
// providing a robust foundation for scalable agent orchestration.
//
// # Core Responsibilities
//
// Agent Management:
//   - Thread-safe agent registry with name-based lookup
//   - Dynamic agent registration and replacement
//   - Agent lifecycle coordination and resource management
//
// Invocation Orchestration:
//   - Asynchronous and synchronous execution patterns
//   - Bounded concurrency with configurable limits
//   - Context-aware cancellation and timeout handling
//   - Graceful resource cleanup and error propagation
//
// Event Processing:
//   - Real-time event streaming with configurable buffering
//   - Event action processing and service coordination
//   - Session persistence and state management
//   - Non-blocking event emission with backpressure handling
//
// Service Integration:
//   - Session store coordination for persistence and state
//   - Artifact store integration for binary/blob management
//   - Memory store coordination for searchable recall
//   - Extensible callback system for cross-cutting concerns
//
// # Architecture
//
// The engine follows a layered architecture with clear separation of concerns:
//
//	┌─────────────────────────────────────────────────────────┐
//	│                    Client Layer                         │
//	├─────────────────────────────────────────────────────────┤
//	│                  Engine Interface                       │
//	│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────┐   │
//	│  │   Invoke    │ │ InvokeSync  │ │   Register      │   │
//	│  └─────────────┘ └─────────────┘ └─────────────────┘   │
//	├─────────────────────────────────────────────────────────┤
//	│                 Orchestration Layer                     │
//	│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────┐   │
//	│  │   Events    │ │  Callbacks  │ │  Concurrency    │   │
//	│  │ Processing  │ │  Manager    │ │   Control       │   │
//	│  └─────────────┘ └─────────────┘ └─────────────────┘   │
//	├─────────────────────────────────────────────────────────┤
//	│                   Service Layer                         │
//	│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────┐   │
//	│  │   Session   │ │  Artifact   │ │     Memory      │   │
//	│  │   Store     │ │    Store    │ │     Store       │   │
//	│  └─────────────┘ └─────────────┘ └─────────────────┘   │
//	└─────────────────────────────────────────────────────────┘
//
// # Key Components
//
// Engine Interface:
//   - Primary orchestration contract defining core operations
//   - Supports both streaming (Invoke) and batch (InvokeSync) patterns
//   - Provides agent registry and session management capabilities
//
// Configuration:
//   - Tunable parameters for concurrency, buffering, and streaming
//   - Production-ready defaults with environment-specific overrides
//   - Functional options pattern for flexible configuration
//
// Implementation:
//   - Concrete engine with comprehensive goroutine management
//   - Event processing pipeline with action application
//   - Resource tracking and cleanup mechanisms
//   - Structured logging and observability hooks
//
// Callback System:
//   - Extensible hooks for lifecycle events and cross-cutting concerns
//   - Built-in implementations for logging and validation
//   - Custom callback support for specialized requirements
//
// # Usage Patterns
//
// Basic Engine Setup:
//
//	engine := engine.New(sessionStore, artifactStore, memoryStore,
//	    engine.WithConfig(engine.DefaultConfig()),
//	    engine.WithLogger(logger),
//	    engine.WithMaxConcurrent(50))
//
// Agent Registration:
//
//	modelAgent := agent.NewModelAgent("Assistant", model)
//	engine.Register(modelAgent)
//
// Streaming Execution:
//
//	invocationID, events, errors, err := engine.Invoke(ctx, "session-1", "Assistant", userContent)
//	if err != nil {
//	    return err
//	}
//	_ = invocationID // use for cancellation or tracking
//	for event := range events {
//	    handleEvent(event)
//	}
//
// Synchronous Execution:
//
//	invocationID, events, err := engine.InvokeSync(ctx, "session-1", "Assistant", userContent)
//	if err != nil {
//	    return err
//	}
//	_ = invocationID
//	processEvents(events)
//
// # Concurrency Model
//
// The engine is designed for high-concurrency operation with the following guarantees:
//
//   - Thread-safe agent registration and lookup
//   - Bounded concurrent invocations to prevent resource exhaustion
//   - Per-invocation isolation with independent contexts and channels
//   - Graceful cancellation propagation and resource cleanup
//   - Non-blocking event emission with configurable buffering
//
// # Error Handling
//
// The engine employs a comprehensive error handling strategy:
//
//   - Immediate errors: Returned directly for startup failures
//   - Terminal errors: Propagated via dedicated error channels
//   - Context cancellation: Handled gracefully with proper cleanup
//   - Service errors: Treated as terminal to maintain consistency
//
// # Performance Considerations
//
// The engine is optimized for:
//   - Low-latency event streaming with minimal buffering overhead
//   - Efficient resource utilization through bounded concurrency
//   - Memory-conscious event processing with streaming patterns
//   - Scalable service integration with minimal coordination overhead
//
// # Extensibility
//
// The engine supports extension through:
//   - Callback interfaces for lifecycle hooks and custom logic
//   - Pluggable service implementations for different backends
//   - Configurable behavior through functional options
//   - Reserved action types for future feature development
//
// The engine handles the complexity of concurrent agent execution while maintaining
// deterministic event ordering, proper resource cleanup, and comprehensive error handling.
// It provides a solid foundation for building scalable, production-ready agent systems.
package engine
