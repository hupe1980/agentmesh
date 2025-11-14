/*
Package graph provides a Pregel-inspired graph execution engine for orchestrating
agent workflows with conditional routing, parallel execution, and state management.

# Overview

The graph package implements a bulk synchronous parallel (BSP) computation model
where nodes (agents) execute in synchronized supersteps, communicating through
messages and shared state. It supports both simple sequential workflows and complex
conditional routing patterns.

# Core Concepts

A graph consists of:
  - Nodes: Units of computation (functions, agents, tools)
  - Edges: Connections defining execution order
  - State: Shared data accessible to all nodes
  - Messages: Communication between nodes across supersteps

# Builder Pattern

Create graphs using the fluent builder API:

	builder := graph.NewBuilder()
	builder.AddNode(&graph.Node{
		Name: "agent",
		RunFunc: func(ctx context.Context, state state.Reader) (*graph.NodeResult, error) {
			messages := state.MessagesSnapshot()
			// Process messages...
			return &graph.NodeResult{
				Messages: []message.Message{response},
				Updates:  map[string]any{"status": "complete"},
			}, nil
		},
	})
	builder.AddEdge("START", "agent")
	builder.AddEdge("agent", "END")

	compiled, err := builder.Compile()
	if err != nil {
		log.Fatal(err)
	}

# Execution Modes

Graphs can be executed in different modes:

	// Invoke: Execute once and return results
	results, err := lastResult, err := graph.Last(compiled.Run(ctx, messages)

	// Run: Execute with real-time execution result streaming
	seq := compiled.Run(ctx, messages)
	for result, err := range seq {
		if err != nil {
			log.Printf("Error: %v", err)
		}
		log.Printf("Node %s completed", result.Node)
	}

# Conditional Routing

Routes can be determined dynamically based on node output:

	builder.AddConditionalEdges("classifier", func(result *graph.NodeResult) []string {
		if result.Updates["category"] == "urgent" {
			return []string{"urgent_handler"}
		}
		return []string{"normal_handler"}
	})

# State Management

State is shared across all nodes with thread-safe access. The StateManager interface
provides a pluggable abstraction, allowing custom storage backends (Redis, PostgreSQL, etc.):

	// Read state (immutable snapshot)
	value := state.Get("key")
	messages := state.MessagesSnapshot()

	// Update state (via NodeResult)
	return &graph.NodeResult{
		Updates: map[string]any{
			"counter": counter + 1,
			"status":  "processed",
		},
	}, nil

The Graph and Builder types work exclusively with the StateManager interface,
enabling custom implementations while the default *State implementation provides
channel-based state with checkpointing and message history management.

	// Use default state
	compiled := graph.NewBuilder().
		WithState(graph.NewStateManager(100)).
		Compile()

	// Custom implementations can be provided by implementing StateManager interface
	// This enables Redis-backed, PostgreSQL-backed, or other storage backends

# Channel-Based State Architecture

The graph package uses a deterministic channel-based state system implemented in state_manager.go:

	// Create state with message limit (returns StateManager interface)
	state := graph.NewStateManager(maxMessages)

	// For direct ChannelState access (advanced use):
	channelState := graph.NewChannelState(maxMessages)
	channelState.AddChannel(channel.NewLastValueChannel("status"))
	channelState.AddChannel(channel.NewTopicChannel("events", 100))
	channelState.AddChannel(channel.NewBinaryOpChannel("counter", 0, func(a, b any) any {
		return a.(int) + b.(int)
	}))

# Simplified State Initialization

For common use cases, use StateBuilder instead of manual channel setup:

	// Old way (verbose)
	state := graph.NewChannelState(100)
	state.AddChannel(channel.NewLastValueChannel("status"))
	state.Set("status", "pending")
	state.AddChannel(channel.NewBinaryOpChannel("counter", 0, addFunc))

	// New way (fluent API)
	state := graph.NewStateBuilder().
		WithMessages(100).
		WithLastValue("status", "pending").
		WithCounter("iterations").
		WithFlag("completed").
		WithList("logs").
		WithMap("results").
		Build()

StateBuilder provides high-level methods that eliminate the need to understand
low-level channel mechanics. Available builders:
  - WithMessages(n): Message history with limit
  - WithLastValue(name, initial): Latest-only value (status, config)
  - WithCounter(name): Accumulating counter (iterations, scores)
  - WithFlag(name): Boolean flag (completed, validated)
  - WithList(name): Append-only list (logs, history)
  - WithListLimit(name, max): List with size limit
  - WithMap(name): Merge map values (task results)
  - WithBinaryOp(name, initial, reducer): Custom reducer function

Available channel types:
  - TopicChannel: Accumulates messages (append-only list)
  - LastValueChannel: Stores only most recent value (overwrite semantics)
  - BinaryOpChannel: Merges values using custom operator (e.g., sum, max)

# Architecture Overview

The graph package uses an Adapter pattern to bridge high-level graph concepts
to the generic Pregel BSP engine:

	User Code
	    ↓
	Builder API (graph construction)
	    ↓
	Compiled (execution orchestrator)
	    ↓
	Pregel Adapter (pregel.go)
	    ↓
	Pregel Runtime (pkg/pregel) - PUBLIC API

The adapter layer (pregel.go) translates between:
  - Graph nodes → Pregel vertices
  - Channel messages → BSP message payloads
  - Graph state → Pregel global state
  - Graph aggregators → Pregel aggregators

This separation allows the Pregel engine to remain pure and domain-agnostic
while the graph package provides agent-specific features like channels,
checkpoints, retry policies, and conditional routing.

The pkg/pregel package is now public API, enabling advanced users to:
  - Implement custom MessageBus backends (Redis, Kafka, distributed)
  - Create custom Scheduler implementations (priority-based, GPU-optimized)
  - Fine-tune execution engine parameters for specific use cases
  - Build research prototypes using the BSP model

# Core Files

Core files in pkg/graph:
  - graph.go: Graph builder and compilation
  - builder.go: Fluent builder API
  - node.go: Node definitions and execution
  - state_manager.go: State management (StateManager, State, bufferedStateWriter)
  - pregel.go: BSP execution adapter (ChannelMessage, graphRuntime, nodeAdapter)
  - compiled_graph.go: Compiled graph runtime (ConditionalEvaluator, state.ExecutionResult)
  - executor.go: Execution abstractions (Executor, ExecutionTracker, executionState)
  - scheduler.go: Topology-based node scheduling (vertexScheduler, TopologyScheduler)
  - options.go: Run options (checkpoint, retry, rate-limit configuration)
  - aggregators.go: Cross-node aggregations (sum, max, min, avg, variance)

# Performance

The graph engine is optimized for low-latency execution:
  - Parallel node execution with configurable workers
  - Channel-based state for deterministic data flow
  - Lock splitting for reduced contention
  - ~6μs per node execution overhead

# Error Handling

Nodes can return errors which will halt execution:

	return nil, &graph.ValidationError{
		Field:   "input",
		Message: "required field missing",
	}

Retries can be configured per-node using the fluent builder API:

	node.RetryPolicy = graph.NewRetryPolicy().
		WithMaxAttempts(5).
		WithExponentialBackoff(time.Second, 2.0).
		WithRetryableErrors(ErrTransient, ErrTimeout).
		Build()

Advanced backoff strategies are available:

	// Linear backoff: 1s, 2s, 3s, 4s, ...
	graph.NewRetryPolicy().WithLinearBackoff(time.Second)

	// Constant backoff: 1s, 1s, 1s, ...
	graph.NewRetryPolicy().WithConstantBackoff(time.Second)

	// Capped exponential: max 30 seconds
	policy := &graph.RetryPolicy{
		MaxAttempts: 10,
		Backoff: graph.CappedExponentialBackoff(time.Second, 2.0, 30*time.Second),
	}

	// Jittered exponential: prevents thundering herd
	policy := &graph.RetryPolicy{
		MaxAttempts: 5,
		Backoff: graph.JitteredExponentialBackoff(time.Second, 2.0, 0.1),
	}

Custom retry logic with flexible error matching:

	graph.NewRetryPolicy().
		WithRetryableFunc(func(err error) bool {
			var apiErr *APIError
			return errors.As(err, &apiErr) && apiErr.StatusCode >= 500
		})

# Input Validation

Protect workflows from DoS attacks and resource exhaustion with built-in validation:

	compiled.Run(ctx, messages,
		graph.WithMaxMessageSize(1_000_000),    // 1MB per message
		graph.WithMaxInputMessages(100),        // Max 100 messages
		graph.WithMaxTotalSize(10_000_000),     // 10MB total
	)

Validation occurs before graph execution, with detailed error messages:

	// Returns MessageValidationError with Type, Limit, Actual, MessageIndex
	err := compiled.Run(ctx, oversizedMessages)
	if errors.Is(err, graph.ErrMessageTooLarge) {
		// Handle validation failure
	}

See examples/input_validation for comprehensive usage.

# Observability

The graph engine supports OpenTelemetry for tracing and metrics:

	lastResult, err := graph.Last(compiled.Run(ctx, messages,
		graph.WithTracer(tracer),
		graph.WithMeterProvider(meterProvider),
	)

Execution results can be streamed for monitoring:

	for result, err := range compiled.Run(ctx, messages) {
		if err != nil {
			// handle error
		}
		log.Printf("Node: %s", result.Node)
	}

# Advanced Features

  - Subgraphs: Nest graphs within nodes for composition
  - Human-in-the-loop: Pause execution for human input
  - Time travel: Resume from any superstep via checkpointing
  - Aggregators: Compute global values across nodes (sum, count, convergence)
  - Message retention: Limit conversation history size to prevent memory issues
  - Circuit breakers: Prevent cascading failures with automatic recovery
  - Rate limiting: Control execution rate per node
*/
package graph
