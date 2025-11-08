# 🤖🕸️ AgentMesh

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/hupe1980/agentmesh)](https://goreportcard.com/report/github.com/hupe1980/agentmesh)
[![GoDoc](https://pkg.go.dev/badge/github.com/hupe1980/agentmesh.svg)](https://pkg.go.dev/github.com/hupe1980/agentmesh)

> 🚀 **Production-grade multi-agent orchestration framework** powered by Pregel-style bulk-synchronous parallel (BSP) graph processing.

AgentMesh enables you to build sophisticated AI agent workflows with parallel execution, state management, and enterprise-grade observability. Built natively in Go for performance and type safety.

**Requires Go 1.23+** for iterator support (`iter.Seq2`).

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
- **🔍 Graph Introspection** - Debug and visualize graphs with topology analysis and Mermaid flowcharts
- **🎨 Flowchart Generation** - Auto-generate Mermaid diagrams from graph topology
- **⏸️ Human-in-the-Loop** - Pause workflows for human approval/input
- **🔌 Callback System** - Intercept and transform model/tool requests with BeforeModel, AfterModel, OnError handlers
- **🧪 Testing First** - Comprehensive test coverage across core features

### 🧠 AI/ML Features
- **🔢 Embeddings** - Text-to-vector conversion for semantic search and RAG workflows (OpenAI, SimpleEmbedder)
- **🧠 Memory** - Long-term conversation storage with semantic vector search and session management
- **📝 Prompt Templates** - Variable substitution with {{.Variable}} syntax for reusable prompt patterns
- **🔍 Retrieval** - RAG integration with AWS Bedrock Knowledge Bases and Kendra
- **🔄 Unified Streaming** - Iterator-based model API (Go 1.23+ `iter.Seq2`) for consistent streaming/blocking modes

### 🌐 Integration & Extensibility
- **🤝 Agent-to-Agent (A2A) Protocol** - Expose agents as A2A services or connect to external A2A agents
- **🔌 Model Context Protocol (MCP)** - Dynamic tool discovery from MCP servers
- **🛠️ LangChainGo Tools** - Import and use LangChainGo tool ecosystem
- **🤖 Multi-Provider LLMs** - OpenAI, Anthropic, Gemini with functional options pattern
- **⚙️ Custom Execution Backends** - Public `pkg/pregel` API for distributed MessageBus (Redis, Kafka) and custom schedulers
- **🔒 Checkpoint Integrity** - State versioning detects corruption and concurrent modifications

---

## 🚀 Quick Start

### Requirements

- **Go 1.23+** (required for `iter.Seq2` support)

### Installation

```bash
go get github.com/hupe1980/agentmesh@latest
```

---

## 📐 Architecture

AgentMesh follows a **layered architecture** that separates concerns and enables extensibility:

```
┌───────────────────────────────────────────────────────────────┐
│  Application Layer                                            │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  pkg/agent (High-level agent patterns)                  │  │
│  │  • ReAct: Reasoning + Acting                            │  │
│  │  • RAG: Retrieval-Augmented Generation                  │  │
│  └─────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────┘
                              ↓
┌───────────────────────────────────────────────────────────────┐
│  Integration Layer                                            │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────────-─┐  │
│  │ pkg/model    │  │  pkg/tool    │  │  pkg/retrieval      │  │
│  │ • OpenAI     │  │  • Functions │  │  • Bedrock          │  │
│  │ • Anthropic  │  │  • A2A       │  │  • Kendra           │  │
│  │ • Custom     │  │  • ...       │  │  • Custom           │  │
│  └──────────────┘  └──────────────┘  └────────────────────-┘  │
└───────────────────────────────────────────────────────────────┘
                              ↓
┌───────────────────────────────────────────────────────────────┐
│  Core Framework Layer                                         │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  pkg/graph (Graph orchestration & execution)            │  │
│  │  • CompiledGraph: Immutable workflow                    │  │
│  │  • Builder: Fluent graph construction                   │  │
│  │  • Scheduler: Topology-based execution order            │  │
│  │  • State: Channel-based data flow                       │  │
│  └─────────────────────────────────────────────────────────┘  │
│  ┌──────────────┐  ┌────────────────┐  ┌────────────────--─┐  │
│  │ pkg/channel  │  │ pkg/checkpoint │  │  pkg/message      │  │
│  │ • Topic      │  │ • Memory       │  │  • Human          │  │
│  │ • LastValue  │  │ • SQL          │  │  • AI             │  │
│  │ • BinaryOp   │  │ • DynamoDB     │  │  • Tool           │  │
│  └──────────────┘  └────────────────┘  └────────────────--─┘  │
└───────────────────────────────────────────────────────────────┘
                              ↓
┌───────────────────────────────────────────────────────────────┐
│  Execution Engine Layer (PUBLIC API)                         │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  pkg/pregel (Bulk Synchronous Parallel Engine)         │  │
│  │  • Generic BSP runtime (public API for extensions)     │  │
│  │  • Superstep coordination                               │  │
│  │  • Message passing & aggregation                        │  │
│  │  • Pluggable MessageBus (in-memory, Redis, Kafka, etc) │  │
│  │  • Custom Scheduler support                             │  │
│  └─────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────┘
                              ↓
┌───────────────────────────────────────────────────────────────┐
│  Infrastructure Layer (Internal Utilities)                    │
│  ┌──────────────┐  ┌────────────────┐  ┌──────────────--───┐  │
│  │ internal/    │  │ internal/      │  │  internal/        │  │
│  │ jsonschema   │  │  mermaid       │  │  stream           │  │
│  └──────────────┘  └────────────────┘  └────────────────--─┘  │
└───────────────────────────────────────────────────────────────┘
                              ↓
┌───────────────────────────────────────────────────────────────┐
│  Observability Layer (Cross-cutting)                          │
│  ┌──────────────┐  ┌────────────────┐  ┌──────────────--───┐  │
│  │ pkg/metrics  │  │  pkg/trace     │  │  pkg/logging      │  │
│  │ • OpenTelem  │  │  • OpenTelem   │  │  • slog           │  │
│  └──────────────┘  └────────────────┘  └────────────────--─┘  │
└───────────────────────────────────────────────────────────────┘
```

**Key Design Principles:**
- **Bottom-Up Dependencies**: Higher layers depend on lower layers only
- **Interface-Based**: Each layer exposes clear interfaces for extension
- **Pregel BSP Core**: Public `pkg/pregel` API enables parallel execution, custom backends, and distributed processing
- **Channel-Based State**: Typed data flow with versioning, snapshots, and checkpoint integrity
- **Extensible by Design**: Custom MessageBus, Scheduler, and execution strategies supported

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

### Supervisor Multi-Agent Pattern

Create a supervisor agent that routes tasks to specialized worker agents:

```go
package main

import (
    "context"
    "log"

    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/message"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
)

func main() {
    model := openai.NewModel()

    // Create specialized worker agents
    mathAgent, _ := agent.NewReActAgent(model,
        agent.WithSystemPrompt("You are a math expert."))
    
    codeAgent, _ := agent.NewReActAgent(model,
        agent.WithSystemPrompt("You are a programming expert."))

    // Create supervisor that routes to specialists
    supervisor, err := agent.NewSupervisorAgent(
        model,
        agent.WithWorker("math", "Expert in mathematics", mathAgent),
        agent.WithWorker("code", "Expert in programming", codeAgent),
        agent.WithSupervisorMaxIterations(10),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Execute - supervisor automatically routes to the right specialist
    ctx := context.Background()
    messages := []message.Message{
        message.NewHumanMessageFromText("What is the derivative of x^2 + 3x?"),
    }

    results, _ := supervisor.Invoke(ctx, messages)
    // Supervisor routes to math agent → returns answer
}
```

**Key Benefits:**
- 🎯 **Automatic routing** to appropriate specialists
- 🔧 **Tool-based handoffs** using `HandoffToAgent` pattern
- 🔄 **Fresh context** per task (configurable)
- ♻️ **Retry logic** for robust execution
- ✨ **Clean API** with functional options

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

Explore **17 comprehensive examples** demonstrating different use cases and patterns:

| Example | Description | Key Features |
|---------|-------------|--------------|
| 🎯 [basic_agent](examples/basic_agent) | Simple ReAct agent with tools | Agent creation, tool calling, message handling |
| 👥 [supervisor_agent](examples/supervisor_agent) | Multi-agent coordination pattern | Supervisor routing, worker specialists, handoff tools |
| 🏗️ [state_builder](examples/state_builder) | Simplified state initialization | Fluent API, channel patterns, reduced boilerplate |
| 🔌 [mcp_tools](examples/mcp_tools) | Model Context Protocol integration | Dynamic tool discovery, MCP toolsets, runtime tools |
| 🌊 [streaming](examples/streaming) | Real-time event streaming | Live updates, partial results, progress tracking |
| 🔀 [conditional_flow](examples/conditional_flow) | Dynamic routing based on state | Conditional edges, branching logic, flow control |
| ⚡ [parallel_tasks](examples/parallel_tasks) | Concurrent execution patterns | Parallel nodes, fan-out/fan-in, result aggregation |
| ⏸️ [human_pause](examples/human_pause) | Human-in-the-loop workflows | Interrupt, resume, user approval |
| ⏰ [time_travel](examples/time_travel) | Debug with state versioning | Checkpointing, state replay, time-travel debugging |
| 💾 [checkpointing](examples/checkpointing) | Fault-tolerant workflows | Auto-save, auto-resume, persistence |
| 📞 [callback_integration](examples/callback_integration) | Callback system demonstration | BeforeModel, AfterModel, OnToolError handlers |
| 🛡️ [circuit_breaker](examples/circuit_breaker) | Fault tolerance patterns | Circuit breaker states, failure handling, policy composition |
| 🛡️ [guardrails](examples/guardrails) | Content filtering & PII protection | Input validation, output filtering, safety constraints |
| 📊 [observability](examples/observability) | Metrics and distributed tracing | OpenTelemetry integration, monitoring |
| 🔗 [subgraph](examples/subgraph) | Composable graph components | Reusable workflows, modular design |
| 📝 [message_retention](examples/message_retention) | Conversation history management | Message limits, pruning strategies |
| 🔢 [openai_embedder](examples/openai_embedder) | Text embeddings | Semantic search, RAG workflows, vector operations |
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

### 🔍 Graph Introspection

Debug and visualize your graphs:

```go
// Inspect topology
topo := compiled.GetTopology()
fmt.Printf("Entry points: %v\n", topo.EntryPoints)
fmt.Printf("Max depth: %d\n", topo.MaxDepth)

// Get execution paths
paths := compiled.GetExecutionPath(10)

// Generate Mermaid flowchart
flowchart := compiled.GenerateMermaidFlowchart("TD")
os.WriteFile("graph.mmd", []byte(flowchart), 0644)

// Track runtime metrics
metrics := compiled.GetMetrics()
fmt.Printf("Complexity: %d\n", metrics.CyclomaticComplexity)
```

### � Callbacks

Intercept and transform model/tool invocations with a composable callback system:

```go
import "github.com/hupe1980/agentmesh/pkg/callbacks"

// Create callback manager
cbManager := callbacks.NewManager()

// Register model callbacks
cbManager.RegisterBeforeModel(func(ctx context.Context, req *callbacks.ModelRequest) (*callbacks.ModelResponse, error) {
    // Content filtering/guardrails
    if containsUnsafeContent(req.Messages) {
        return nil, errors.New("unsafe content detected")
    }
    return nil, nil  // Continue to model
})

cbManager.RegisterAfterModel(func(ctx context.Context, req *callbacks.ModelRequest, resp *callbacks.ModelResponse) (*callbacks.ModelResponse, error) {
    // Post-process response, logging, metrics
    log.Printf("Model latency: %v", resp.Metadata["latency"])
    return resp, nil
})

cbManager.RegisterOnModelError(func(ctx context.Context, req *callbacks.ModelRequest, err error) (*callbacks.ModelResponse, error) {
    // Fallback logic, retry, or error transformation
    return getFallbackResponse(), nil
})

// Use with ReAct agent
compiled, _ := agent.NewReActAgent(
    model,
    tools,
    agent.WithModelCallbacks(cbManager),
    agent.WithToolCallbacks(cbManager),
)
```

**Callback Types:**
- `BeforeModel/BeforeTool` - Pre-execution validation, caching, transformation
- `AfterModel/AfterTool` - Post-processing, logging, metrics collection
- `OnModelError/OnToolError` - Error handling, fallbacks, retry logic

### �📊 Observability

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

### 🔢 Embeddings & Memory

Convert text to vectors for semantic search and maintain conversation history:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/embedding/openai"
    "github.com/hupe1980/agentmesh/pkg/memory"
)

// Create embedder for semantic search
embedder := openai.NewEmbedder(client, openai.WithModel("text-embedding-3-small"))

// Create vector memory for long-term storage
mem := memory.NewVectorMemory(embedder)

// Store conversation messages
err := mem.Store(ctx, "session-123", messages)

// Recall relevant messages by semantic similarity
recalled, err := mem.Recall(ctx, "session-123", memory.RecallFilter{
    Query: "What did we discuss about pricing?",
    K:     5,  // Top 5 most relevant messages
})
```

**Memory Features:**
- Semantic vector search with embeddings
- Session-based conversation storage
- Relevance ranking and filtering
- Time-based and metadata queries

### 📝 Prompt Templates

Reusable prompt templates with variable substitution:

```go
import "github.com/hupe1980/agentmesh/pkg/prompt"

// Create template with {{.Variable}} syntax
template := prompt.New(`You are a {{.Role}}.
Answer the following question: {{.Question}}
Use this context: {{.Context}}`)

// Render with variables
result, err := template.Render(map[string]any{
    "Role":     "helpful assistant",
    "Question": "What is AgentMesh?",
    "Context":  "A Go framework for AI agents",
})
```

**Template Features:**
- Simple {{.Variable}} syntax
- Missing variable detection
- Type-safe variable replacement
- No code execution (safe for untrusted templates)

### 🤝 Agent-to-Agent (A2A) Protocol

Enable multi-agent collaboration across different systems:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/a2a"
    "github.com/a2aproject/a2a-go/a2agrpc"
    "github.com/a2aproject/a2a-go/a2asrv"
)

// Server: Expose AgentMesh agent as A2A service
compiled, _ := agent.NewReActAgent(model, tools)
executor := a2a.NewExecutor(compiled)
requestHandler := a2asrv.NewHandler(executor)
grpcHandler := a2agrpc.NewHandler(requestHandler)

// Serve with gRPC
server := grpc.NewServer()
a2agrpc.RegisterAgentServer(server, grpcHandler)
server.Serve(listener)

// Client: Use external A2A agent as tool
client := a2a.NewClient("localhost:50051")
bridge := a2a.NewBridge(client)
tools, _ := bridge.GetTools(ctx)

// Use A2A tools in your agent
compiled, _ := agent.NewReActAgent(model, tools)
```

**A2A Features:**
- Multi-agent coordination across systems
- gRPC and JSON-RPC transport support
- Dynamic tool discovery from remote agents
- Bidirectional agent communication

---

## 🔌 Advanced Extensibility

### Custom MessageBus for Distributed Execution

The `pkg/pregel` package is now **public API**, enabling custom MessageBus implementations for distributed execution across Redis, Kafka, or custom backends:

```go
import "github.com/hupe1980/agentmesh/pkg/pregel"

// Implement custom MessageBus interface
type RedisMessageBus struct {
    client *redis.Client
}

func (r *RedisMessageBus) Send(from, to string, data MyMessageType) error {
    // Serialize and send via Redis pub/sub
    payload, _ := json.Marshal(data)
    return r.client.Publish(ctx, to, payload).Err()
}

func (r *RedisMessageBus) Pending(vertex string) ([]pregel.Message[MyMessageType], error) {
    // Fetch pending messages from Redis queue
    messages, _ := r.client.LRange(ctx, vertex, 0, -1).Result()
    // Deserialize and return
    return parseMessages(messages)
}

// Use with custom runtime
runtime := pregel.NewRuntime(
    graphAdapter,
    state,
    pregel.WithMessageBus[StateType, MessageType](redisMessageBus),
    pregel.WithMaxWorkers[StateType, MessageType](100),
)
```

### Custom Scheduler Strategies

Implement domain-specific scheduling for priority-based or GPU-optimized execution:

```go
// Implement Scheduler interface
type PriorityScheduler struct {
    priorities map[string]int
}

func (s *PriorityScheduler) Ready() []string {
    // Return vertices sorted by priority
    return s.sortByPriority(s.readyQueue)
}

// Use with Pregel runtime
runtime := pregel.NewRuntime(
    graphAdapter,
    state,
    pregel.WithScheduler[StateType, MessageType](priorityScheduler),
)
```

### Checkpoint Integrity with State Versioning

State versioning ensures checkpoint integrity and detects corruption:

```go
// Checkpoint now includes Version field
checkpoint := &checkpoint.Checkpoint{
    RunID:     "run-123",
    Superstep: 5,
    Version:   42,  // Monotonic counter incremented on every state mutation
    State:     stateSnapshot,
}

// On restore, version is validated
err := compiled.RestoreFromCheckpoint(ctx, checkpoint)
// Returns error if current version > checkpoint version (concurrent modification)
```

**State Versioning Benefits:**
- Detects checkpoint file corruption
- Prevents out-of-sequence checkpoint restores
- Identifies concurrent state modifications
- Enables debugging of non-deterministic execution

### Public API: pkg/pregel

Advanced users can now access the core Pregel BSP engine directly:

```go
import "github.com/hupe1980/agentmesh/pkg/pregel"

// Available public interfaces:
// - Runtime: Core BSP execution engine
// - MessageBus: Pluggable message backend
// - Scheduler: Custom vertex scheduling
// - Aggregator: Global reductions across vertices
// - PregelGraph/PregelNode: Vertex computation model
```

**Use Cases for pkg/pregel:**
- Custom distributed execution backends (Redis, Kafka, gRPC)
- Research and experimentation with BSP algorithms
- Domain-specific scheduling strategies (GPU, priority-based)
- Fine-grained control over execution lifecycle

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

## 📝 API Naming Conventions

AgentMesh follows a **consistent naming convention** across all components to improve code clarity and maintainability:

| Method | Component | Purpose | When to Use |
|--------|-----------|---------|-------------|
| **`Call()`** | `Tool` | Execute a tool function | Invoking tool/function logic |
| **`Run()`** | `Node`, `Runnable` | Execute node logic | Low-level synchronous node execution |
| **`Invoke()`** | `CompiledGraph` | Execute graph (blocking) | High-level public API for complete execution |
| **`Stream()`** | `CompiledGraph` | Execute graph (streaming) | High-level public API with real-time events |
| **`Execute()`** | `Executor`, Adapters | Strategy implementation | Internal execution strategy pattern |

### Rationale

- **`Run`** - Used for internal, synchronous operations (nodes, services, goroutines)
- **`Invoke`** - Used for high-level public APIs (RPC-style graph execution)
- **`Execute`** - Used for strategy pattern implementations (executor interfaces, adapters)
- **`Call`** - Used for function invocation semantics (tools, callbacks)

### Examples

```go
// Tools use Call() - function invocation
result, err := weatherTool.Call(ctx, `{"location": "Boston"}`)

// Nodes use Run() - low-level execution
result, err := node.Run(ctx, state)

// Graphs use Invoke() - high-level blocking API
messages, err := compiled.Invoke(ctx, initialMessages)

// Graphs use Stream() - high-level streaming API
stream, err := compiled.Stream(ctx, initialMessages)
for stream.Next() {
    event := stream.Current()
}

// Executors use Execute() - strategy implementation
result, err := executor.Execute(ctx, messages, options)
```

This convention aligns with Go idioms and provides clear semantic meaning at different abstraction levels.

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
- 📗 **[Examples](examples/)** - 17 comprehensive runnable examples
- 📙 **[Getting Started Guide](docs/getting-started.md)** - Quick start tutorial
- � **[Architecture Guide](docs/architecture.md)** - Pregel BSP design deep-dive
- � **[Callbacks Guide](docs/callbacks.md)** - Intercepting model/tool invocations
- 🧠 **[Memory Guide](docs/memory.md)** - Conversation storage with semantic search
- 🔢 **[Embeddings Guide](docs/embeddings.md)** - Text-to-vector conversion for RAG
- 🌐 **[A2A Protocol Guide](docs/a2a.md)** - Multi-agent collaboration
- �📊 **[Observability Guide](docs/observability.md)** - Metrics and tracing setup
- 🎯 **[Advanced Features](docs/advanced.md)** - Checkpointing, time travel, human-in-loop
- 🤖 **[Agent Patterns](docs/agents.md)** - ReAct and RAG agent guides
- 🔧 **[Tools Guide](docs/tools.md)** - Building and using tools
- 🤖 **[Model Integration](docs/models.md)** - LLM provider setup

---

## 📄 License

This project is licensed under the **Apache License 2.0** - see the [LICENSE](LICENSE) file for details.

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
