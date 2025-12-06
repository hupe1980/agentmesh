// Package agentmesh provides a production-grade multi-agent orchestration framework
// using Pregel-style bulk-synchronous parallel (BSP) graph processing.
//
// # Architecture
//
// AgentMesh is built on three main layers:
//
//  1. Execution Engine (pkg/pregel) - Public BSP runtime API for parallel computation and custom extensions
//  2. Graph Orchestration (pkg/graph) - Workflow construction, state management, and scheduling
//  3. Agent Layer (pkg/agent, pkg/model, pkg/tool) - High-level APIs for building LLM-powered agents
//
// # Quick Start
//
// Build a simple ReAct agent:
//
//	import (
//	    "github.com/hupe1980/agentmesh/pkg/agent"
//	    "github.com/hupe1980/agentmesh/pkg/model/openai"
//	    "github.com/hupe1980/agentmesh/pkg/tool"
//	    "github.com/hupe1980/agentmesh/pkg/message"
//	)
//
//	type WeatherArgs struct {
//	    Location string `json:"location"`
//	}
//
//	// Create tools
//	weatherTool, _ := tool.NewFuncTool("get_weather",
//	    "Get current weather for a location",
//	    func(ctx context.Context, args WeatherArgs) (map[string]any, error) {
//	        return map[string]any{
//	            "location":    args.Location,
//	            "temperature": 22,
//	            "conditions":  "Sunny",
//	        }, nil
//	    },
//	)
//
//	// Build the agent
//	compiled, _ := agent.NewReAct(
//	    openai.NewModel(),
//	    []tool.Tool{weatherTool},
//	)
//
//	// Execute
//	messages := []message.Message{
//	    message.NewSystemMessageFromText("You are a helpful assistant."),
//	    message.NewHumanMessageFromText("What's the weather in Paris?"),
//	}
//	result, _ := graph.Last(compiled.Run(ctx, messages))
//
// # Core Packages
//
// For graph construction:
//
//	import "github.com/hupe1980/agentmesh/pkg/graph"
//
// For agent building:
//
//	import "github.com/hupe1980/agentmesh/pkg/agent"
//
// For LLM models:
//
//	import "github.com/hupe1980/agentmesh/pkg/model/openai"
//	import "github.com/hupe1980/agentmesh/pkg/model/anthropic"
//
// For tools:
//
//	import "github.com/hupe1980/agentmesh/pkg/tool"
//
// For observability:
//
//	import "github.com/hupe1980/agentmesh/pkg/metrics"
//	import "github.com/hupe1980/agentmesh/pkg/trace"
//	import "github.com/hupe1980/agentmesh/pkg/logging"
//
// # Features
//
//   - Parallel graph execution with Pregel-style BSP model (~430ns overhead per node)
//   - Channel-based state management (Topic, LastValue, BinaryOp channels)
//   - Automatic checkpointing and state persistence
//   - Time-travel debugging by replaying from any superstep
//   - Retry policies with configurable exponential backoff
//   - Subgraph composition for modular workflows
//   - OpenTelemetry metrics and distributed tracing
//   - Human-in-the-loop pause/resume capabilities
//   - Conditional routing based on node outputs
//   - Message retention limits for conversation management
//   - ReAct and RAG agent patterns
//
// # Examples
//
// See the examples/ directory for comprehensive examples:
//
//   - examples/basic_agent/ - Simple ReAct agent with tools
//   - examples/streaming/ - Real-time execution result streaming
//   - examples/conditional_flow/ - Dynamic routing
//   - examples/parallel_tasks/ - Parallel node execution
//   - examples/checkpointing/ - State persistence and recovery
//   - examples/time_travel/ - Debug with superstep replay
//   - examples/observability/ - OpenTelemetry integration
//   - examples/human_pause/ - Human-in-the-loop workflows
//   - examples/subgraph/ - Reusable graph components
//   - examples/message_retention/ - Conversation history management
//
// # Production Features
//
// MaxIterations:
//
//	compiled, _ := g.Build(
//	    graph.WithMaxIterations(100),
//	)
//
// Checkpointing:
//
//	store := checkpoint.NewMemory()
//	compiled, _ := g.Build(
//	    graph.WithCheckpointer(store),
//	    graph.WithCheckpointInterval(1),
//	)
//	result, _ := graph.Last(compiled.Run(ctx, messages,
//	    graph.WithRunID(threadID),
//	))
//
// Retry Policies:
//
//	g.Node("api_call", apiCallFunc, "next").
//	    WithRetryPolicy(&graph.RetryPolicy{
//	        MaxAttempts:    3,
//	        InitialBackoff: 100 * time.Millisecond,
//	        MaxBackoff:     1 * time.Second,
//	        Multiplier:     2.0,
//	    })
//
// Observability:
//
//	inst := graph.NewInstrumentation(metricsProvider, traceProvider)
//	ctx, span := inst.TraceNodeExecution(ctx, nodeName, superstep)
//	inst.RecordNodeExecution(ctx, nodeName, duration, err)
//
// # Documentation
//
// For detailed documentation, see:
//
//   - README.md - Project overview and quick start
//   - docs/getting-started.md - Comprehensive tutorial
//   - docs/architecture.md - Pregel BSP design deep-dive
//   - docs/agents.md - ReAct and RAG agent patterns
//   - docs/tools.md - Building and using tools
//   - docs/models.md - LLM provider integration
//   - docs/observability.md - Metrics and tracing setup
//
// # API Documentation
//
// Complete API reference: https://pkg.go.dev/github.com/hupe1980/agentmesh
package agentmesh
