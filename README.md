# 🤖🕸️ AgentMesh

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/hupe1980/agentmesh)](https://goreportcard.com/report/github.com/hupe1980/agentmesh)

> 🚀 **Production-grade multi-agent orchestration framework** powered by Pregel-style bulk-synchronous parallel (BSP) graph processing.

AgentMesh enables you to build sophisticated AI agent workflows with parallel execution, state management, and enterprise-grade observability. Built natively in Go for performance and type safety.

---

## ✨ Features

### 🎯 Core Capabilities
- **🔄 Parallel Graph Execution** - Pregel-based BSP engine for efficient multi-agent coordination
- **🧠 LLM Integration** - First-class support for OpenAI, Anthropic, and extensible model interfaces
- **🛠️ Tool Orchestration** - Type-safe function calling with automatic JSON schema generation
- **💾 State Management** - Channel-based state with versioning and time-travel debugging
- **🔁 Retry Policies** - Configurable exponential backoff with custom retry logic
- **🎭 Subgraph Support** - Compose complex workflows from reusable graph components

### 📊 Production Features
- **📈 Observability** - Built-in OpenTelemetry metrics and distributed tracing
- **💾 Automatic Checkpointing** - In-memory/persistent state with auto-resume capabilities
- **⏱️ Execution Control** - Max iterations, timeouts, and graceful termination
- **🔀 Conditional Routing** - Dynamic flow control based on node outputs
- **🎨 Flowchart Generation** - Auto-generate Mermaid diagrams from graph topology
- **⏸️ Human-in-the-Loop** - Pause workflows for human approval/input
- **🧪 Testing First** - Comprehensive test coverage across core features

---

## 🚀 Quick Start

### Installation

```bash
go get github.com/hupe1980/agentmesh@latest
```

---

## 📐 Architecture

AgentMesh follows a **layered architecture** that separates concerns and enables extensibility:

```
┌─────────────────────────────────────────────────────────────┐
│  Application Layer                                          │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  pkg/agent (High-level agent patterns)               │  │
│  │  • ReAct: Reasoning + Acting                         │  │
│  │  • RAG: Retrieval-Augmented Generation               │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│  Integration Layer                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │ pkg/model    │  │  pkg/tool    │  │  pkg/retrieval   │  │
│  │ • OpenAI     │  │  • Functions │  │  • Bedrock       │  │
│  │ • Anthropic  │  │  • A2A       │  │  • Kendra        │  │
│  │ • Custom     │  │  • LangChain │  │  • Custom        │  │
│  └──────────────┘  └──────────────┘  └──────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│  Core Framework Layer                                       │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  pkg/graph (Graph orchestration & execution)        │   │
│  │  • CompiledGraph: Immutable workflow                │   │
│  │  • Builder: Fluent graph construction               │   │
│  │  • Scheduler: Topology-based execution order        │   │
│  │  • State: Channel-based data flow                   │   │
│  └─────────────────────────────────────────────────────┘   │
│  ┌────────────┐  ┌──────────────┐  ┌─────────────────┐    │
│  │ pkg/channel│  │ pkg/checkpoint│  │  pkg/message    │    │
│  │ • Topic    │  │ • Memory      │  │  • Human        │    │
│  │ • LastValue│  │ • SQL         │  │  • AI           │    │
│  │ • BinaryOp │  │ • DynamoDB    │  │  • Tool         │    │
│  └────────────┘  └──────────────┘  └─────────────────┘    │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│  Infrastructure Layer                                       │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  internal/pregel (Bulk Synchronous Parallel Engine)  │  │
│  │  • Generic BSP runtime                               │  │
│  │  • Superstep coordination                            │  │
│  │  • Message passing & aggregation                     │  │
│  └──────────────────────────────────────────────────────┘  │
│  ┌────────────┐  ┌──────────────┐  ┌─────────────────┐    │
│  │ internal/  │  │ internal/    │  │  internal/      │    │
│  │ jsonschema │  │  mermaid     │  │  stream         │    │
│  └────────────┘  └──────────────┘  └─────────────────┘    │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│  Observability Layer (Cross-cutting)                        │
│  ┌────────────┐  ┌──────────────┐  ┌─────────────────┐    │
│  │ pkg/metrics│  │  pkg/trace   │  │  pkg/logging    │    │
│  │ • OpenTelem│  │  • OpenTelem │  │  • slog         │    │
│  └────────────┘  └──────────────┘  └─────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

**Key Design Principles:**
- **Bottom-Up Dependencies**: Higher layers depend on lower layers only
- **Interface-Based**: Each layer exposes clear interfaces for extension
- **Pregel BSP Core**: Enables parallel execution, cycles, and deterministic ordering
- **Channel-Based State**: Typed data flow with versioning and snapshots

---

### Hello World Agent

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/message"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
    "github.com/hupe1980/agentmesh/pkg/tool"
)

type WeatherArgs struct {
    Location string `json:"location"`
}

func main() {
    // Create a tool
    weatherTool, _ := tool.NewFuncTool(
        "get_weather",
        "Get current weather for a location",
        func(ctx context.Context, args WeatherArgs) (map[string]any, error) {
            return map[string]any{
                "location":    args.Location,
                "temperature": 22,
                "conditions":  "Sunny",
            }, nil
        },
    )

    // Build a ReAct agent
    compiled, err := agent.NewReActAgent(
        openai.NewModel(),
        []tool.Tool{weatherTool},
    )
    if err != nil {
        log.Fatal(err)
    }

    // Execute
    ctx := context.Background()
    messages := []message.Message{
        message.NewSystemMessageFromText("You are a helpful weather assistant."),
        message.NewHumanMessageFromText("What's the weather in Paris?"),
    }

    results, err := compiled.Invoke(ctx, messages)
    if err != nil {
        log.Fatal(err)
    }
    
    // Print the final AI response
    for _, msg := range results {
        if aiMsg, ok := msg.(*message.AIMessage); ok {
            for _, part := range aiMsg.Parts() {
                if text, ok := part.(message.TextPart); ok {
                    fmt.Println(text.Text)
                }
            }
        }
    }
}
```

**Output:**
```
The weather in Paris is currently sunny with a temperature of 22°C.
```

---

## 📚 Core Concepts

### 🕸️ Graph Architecture

AgentMesh uses a **directed graph** model where:
- **Nodes** = Computational units (agents, tools, functions)
- **Edges** = Data flow and execution dependencies
- **State** = Shared context accessible across all nodes

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

// Create a graph builder
builder := graph.NewBuilder()

// Add nodes with functions
builder.Node("step1", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    return &graph.NodeResult{
        Updates: map[string]any{"result": "processed"},
    }, nil
})

builder.Node("step2", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    result := s.Get("result").(string)
    fmt.Println("Received:", result)
    return &graph.NodeResult{}, nil
})

// Define flow
builder.AddEdge("START", "step1")
builder.AddEdge("step1", "step2")
builder.AddEdge("step2", "END")

// Compile and run
compiled, _ := builder.Compile()
messages, _ := compiled.Invoke(context.Background(), initialMessages)
```

### 🔄 Pregel-Style Execution

AgentMesh executes graphs in synchronized **supersteps**:

1. **Compute Phase** - All ready nodes execute in parallel
2. **Synchronization** - Wait for all nodes to complete
3. **State Update** - Nodes update shared state atomically
4. **Repeat** - Until reaching END node or max iterations

This enables:
- ⚡ **Parallel execution** of independent nodes (~6μs overhead per node)
- 🔒 **Deterministic** ordering within supersteps
- 📊 **Easy reasoning** about distributed state
- 💾 **Automatic checkpointing** at superstep boundaries

---

## 🎨 Examples

Explore **10 comprehensive examples** demonstrating different use cases and patterns:

| Example | Description | Key Features |
|---------|-------------|--------------|
| 🎯 [basic_agent](examples/basic_agent) | Simple ReAct agent with tools | Agent creation, tool calling, message handling |
| 🏗️ [state_builder](examples/state_builder) | Simplified state initialization | Fluent API, channel patterns, reduced boilerplate |
| 🔌 [mcp_tools](examples/mcp_tools) | Model Context Protocol integration | Dynamic tool discovery, MCP toolsets, runtime tools |
| 🌊 [streaming](examples/streaming) | Real-time event streaming | Live updates, partial results, progress tracking |
| 🔀 [conditional_flow](examples/conditional_flow) | Dynamic routing based on state | Conditional edges, branching logic, flow control |
| ⚡ [parallel_tasks](examples/parallel_tasks) | Concurrent execution patterns | Parallel nodes, fan-out/fan-in, result aggregation |
| ⏸️ [human_pause](examples/human_pause) | Human-in-the-loop workflows | Interrupt, resume, user approval |
| ⏰ [time_travel](examples/time_travel) | Debug with state versioning | Checkpointing, state replay, time-travel debugging |
| 💾 [checkpointing](examples/checkpointing) | Fault-tolerant workflows | Auto-save, auto-resume, persistence |
| 📊 [observability](examples/observability) | Metrics and distributed tracing | OpenTelemetry integration, monitoring |
| 🔗 [subgraph](examples/subgraph) | Composable graph components | Reusable workflows, modular design |
| 📝 [message_retention](examples/message_retention) | Conversation history management | Message limits, pruning strategies |
| 🌐 [a2a_integration](examples/a2a_integration) | Agent-to-Agent protocol | A2A server/client, multi-agent coordination |

### Running Examples

```bash
# Set your OpenAI API key
export OPENAI_API_KEY="sk-..."

# Run an example
cd examples/basic_agent
go run main.go

# Try streaming
cd examples/streaming
go run main.go

# Run A2A server
cd examples/a2a_integration/server
go run main.go
```

---

## 🔧 Advanced Features

### ⏱️ Max Iterations

Prevent infinite loops in cyclic graphs:

```go
compiled, _ := builder.Compile(
    graph.WithMaxIterations(10),
)
```

### 🔁 Retry Policies

Resilient execution with exponential backoff:

```go
builder.Node("flaky_api", apiCallFunc)
builder.SetRetryPolicy("flaky_api", &graph.RetryPolicy{
    MaxAttempts:    3,
    InitialBackoff: 100 * time.Millisecond,
    MaxBackoff:     1 * time.Second,
    Multiplier:     2.0,
})
```

### 💾 Checkpointing

Automatic state persistence and recovery:

```go
import "github.com/hupe1980/agentmesh/pkg/checkpoint"

// Create checkpoint store
store := checkpoint.NewMemory()

// Compile with checkpointing
compiled, _ := builder.Compile(
    graph.WithCheckpointStore(store),
    graph.WithCheckpointInterval(1), // Save every superstep
)

// Execute - state is automatically saved
messages, _ := compiled.Invoke(ctx, initialMessages)

// Resume from checkpoint after failure
threadID := "conversation-123"
messages, _ := compiled.InvokeFromCheckpoint(ctx, threadID, initialMessages)
```

### 🕰️ Time Travel Debugging

Debug workflows by replaying from any superstep:

```go
// List available checkpoints
checkpoints, _ := store.ListCheckpoints(ctx, threadID)

// Resume from a specific superstep
messages, _ := compiled.InvokeFromSuperstep(ctx, threadID, superstep, initialMessages)
```

### 🎭 Subgraphs

Compose complex workflows from reusable components:

```go
// Create a reusable research graph
researchGraph := createResearchSubgraph()

// Embed in parent workflow
builder.Node("research", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    msgs, err := researchGraph.Invoke(ctx, s.MessagesSnapshot())
    if err != nil {
        return nil, err
    }
    return &graph.NodeResult{
        Messages: msgs,
    }, nil
})
```

### 📊 Observability

Built-in OpenTelemetry integration:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/metrics"
    "github.com/hupe1980/agentmesh/pkg/trace"
    "github.com/hupe1980/agentmesh/pkg/graph"
)

metricsProvider := metrics.NewOpenTelemetry(meterProvider)
traceProvider := trace.NewOpenTelemetry(tracerProvider)

inst := graph.NewInstrumentation(metricsProvider, traceProvider)

// Use instrumentation during execution
ctx, span := inst.TraceGraphExecution(ctx, "my-workflow")
defer span.End()
messages, _ := compiled.Invoke(ctx, initialMessages)
```

**Metrics Tracked:**
- Node execution count and duration
- Superstep execution time
- Error rates per node
- Graph-level execution metrics

**Distributed Tracing:**
- Span per graph execution
- Nested spans for each node
- Automatic context propagation
- Error recording with stack traces

---

## 🧪 Testing

Run the full test suite:

```bash
# All tests
go test ./...

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Specific package
go test ./graph -v

# Run benchmarks
go test ./graph -bench=. -benchmem
```

---

## 🤝 Contributing

We welcome contributions! Here's how to get started:

1. **Fork** the repository
2. **Create** a feature branch (`git checkout -b feature/amazing-feature`)
3. **Write tests** for your changes
4. **Commit** your changes (`git commit -m 'Add amazing feature'`)
5. **Push** to the branch (`git push origin feature/amazing-feature`)
6. **Open** a Pull Request

### Development Guidelines

- ✅ All new code must have tests (target 85%+ coverage)
- ✅ Run `go fmt` and `go vet` before committing
- ✅ Update documentation for public API changes
- ✅ Add examples for new features
- ✅ Follow [Effective Go](https://go.dev/doc/effective_go) conventions

---

## 📖 Documentation

- 📘 **[API Reference](https://pkg.go.dev/github.com/hupe1980/agentmesh)** - Complete godoc documentation
- 📗 **[Examples](examples/)** - 10 comprehensive runnable examples
- 📙 **[Getting Started Guide](docs/getting-started.md)** - Quick start tutorial
- 📕 **[Architecture Guide](docs/architecture.md)** - Pregel BSP design deep-dive
- 📊 **[Observability Guide](docs/observability.md)** - Metrics and tracing setup
- 🤖 **[Agent Patterns](docs/agents.md)** - ReAct and RAG agent guides
- 🔧 **[Tools Guide](docs/tools.md)** - Building and using tools
- 🤖 **[Model Integration](docs/models.md)** - LLM provider setup

---

## 📄 License

This project is licensed under the **MIT License** - see the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

- **Pregel Paper** - Google's bulk-synchronous parallel graph processing model
- **Go Community** - Exceptional tooling and ecosystem
- **OpenTelemetry** - Production-grade observability standards

---

<div align="center">

**⭐ Star this repo if you find it useful!**

Made with ❤️ by [Frank Hübner](https://github.com/hupe1980)

</div>
