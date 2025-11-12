# 🤖🕸️ AgentMesh

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/hupe1980/agentmesh)](https://goreportcard.com/report/github.com/hupe1980/agentmesh)
[![GoDoc](https://pkg.go.dev/badge/github.com/hupe1980/agentmesh.svg)](https://pkg.go.dev/github.com/hupe1980/agentmesh)

> 🚀 **Production-grade multi-agent orchestration framework** powered by Pregel-style bulk-synchronous parallel (BSP) graph processing.

AgentMesh enables you to build sophisticated AI agent workflows with parallel execution, state management, and enterprise-grade observability. Built natively in Go for performance and type safety.

**Requires Go 1.24+**
---

## ✨ Features

### 🎯 Core Capabilities
- **🔄 Parallel Graph Execution** - Pregel-based BSP engine for efficient multi-agent coordination
- **🧠 LLM Integration** - First-class support for OpenAI, Anthropic, and extensible model interfaces
- **🛠️ Tool Orchestration** - Type-safe function calling with automatic JSON schema generation
- **🔒 WASM Tool Sandboxing** - Memory-safe sandbox for executing untrusted code with strict isolation
- **💾 State Management** - Channel-based state with versioning and time-travel debugging
- **🔁 Retry Policies** - Configurable exponential backoff with custom retry logic
- **🎭 Subgraph Support** - Compose complex workflows from reusable graph components

### 📊 Production Features
- **📈 Observability** - Built-in OpenTelemetry metrics and distributed tracing
- **💾 Automatic Checkpointing** - In-memory/persistent state with auto-resume capabilities
- **🔐 Checkpoint Signing** - HMAC-SHA256 signatures prevent state tampering and ensure integrity
- **🛡️ Input Validation** - Configurable size/count limits protect against DoS and resource exhaustion
- **⏱️ Execution Control** - Max iterations, timeouts, and graceful termination
- **🔀 Conditional Routing** - Dynamic flow control based on node outputs
- **🔍 Graph Introspection** - Debug and visualize graphs with topology analysis and Mermaid flowcharts
- **🎨 Flowchart Generation** - Auto-generate Mermaid diagrams from graph topology
- **⏸️ Human-in-the-Loop** - Pause workflows for human approval/input
- **🔌 Plugin System** - Type-safe lifecycle hooks for model/tool interception, metrics, tracing, and custom logic
- **🧪 Testing First** - Comprehensive test coverage across core features

### 🧠 AI/ML Features
- **🔢 Embeddings** - Text-to-vector conversion for semantic search and RAG workflows (OpenAI, SimpleEmbedder)
- **🧠 Memory** - Long-term conversation storage with semantic vector search and session management
- **📝 Prompt Templates** - Variable substitution with {{.Variable}} syntax for reusable prompt patterns
- **🔍 Retrieval** - RAG integration with AWS Bedrock Knowledge Bases and Kendra
- **🔄 Unified Model API** - Iterator-based `iter.Seq2[*model.Response, error]` with streaming/blocking support
- **🧠 Native Reasoning** - First-class support for reasoning-capable models (OpenAI o1/o3, Gemini 2.0, Claude)
- **📊 Rich Metadata** - Access reasoning traces, finish reasons, token probabilities, and usage statistics
- **🎯 Token Tracking** - Comprehensive usage info (prompt, completion, reasoning tokens) for cost monitoring

### 🌐 Integration & Extensibility
- **🤝 Agent-to-Agent (A2A) Protocol** - Expose agents as A2A services or connect to external A2A agents
- **🔌 Model Context Protocol (MCP)** - Dynamic tool discovery from MCP servers
- **🛠️ LangChainGo Tools** - Import and use LangChainGo tool ecosystem
- **🔒 WebAssembly Sandboxing** - Memory-safe execution of untrusted tools with runtime-enforced isolation
- **🤖 Multi-Provider LLMs** - OpenAI, Anthropic, Gemini with functional options pattern
- **⚙️ Custom Execution Backends** - Public `pkg/pregel` API for distributed MessageBus (Redis, Kafka) and custom schedulers
- **🔒 Checkpoint Integrity** - State versioning detects corruption and concurrent modifications

---

## 🚀 Quick Start

### Requirements

- **Go 1.24+**

### Installation

```bash
go get github.com/hupe1980/agentmesh@latest
```

---

## 📐 Architecture

AgentMesh follows a **component-based architecture** with clean separation of concerns:

```
┌──────────────────────────────────────────────────────────────┐
│              Application Layer (pkg/agent)                   │
│  • ReActAgent: Reasoning + Acting pattern                    │
│  • SupervisorAgent: Multi-agent coordination                 │
│  • RAGAgent: Retrieval-Augmented Generation                  │
└──────────────────────────┬───────────────────────────────────┘
                           │ builds on
                           ▼
┌──────────────────────────────────────────────────────────────┐
│               CompiledGraph (Coordinator)                    │
│  • Immutable graph topology (nodes, edges, conditionals)     │
│  • Public API (Invoke, Stream, Pause, Resume)                │
│  • Coordinates StateManager ↔ Executor                       │
│  • Rate limiting & retry policies                            │
└────────────────┬─────────────────────────┬───────────────────┘
                 │                         │
                 │ delegates to            │ delegates to
                 ▼                         ▼
    ┌────────────────────────┐  ┌────────────────────────────┐
    │    StateManager        │  │       Executor             │
    │    (Interface)         │  │       (Interface)          │
    │                        │  │                            │
    │  • Channels            │  │  • Execution Strategy      │
    │  • Checkpoints         │  │  • Superstep Coordination  │
    │  • Aggregates          │  │  • Event Streaming         │
    │  • Thread-safe access  │  │  • Pause/Resume Control    │
    │  • State versioning    │  │  • Execution Statistics    │
    └────────────────────────┘  └──────────┬─────────────────┘
                                           │
                         ┌─────────────────┴─────────────────┐
                         │ implements                        │ implements
                         ▼                                   ▼
            ┌─────────────────────────┐      ┌─────────────────────────┐
            │ Built-in Pregel BSP     │      │ SimpleGraphExecutor     │
            │ (graphRuntime + pregel) │      │ (sequential)            │
            │                         │      │                         │
            │ • BSP Supersteps        │      │ • Topological order     │
            │ • Parallel execution    │      │ • Single-threaded       │
            │ • Worker Pool           │      │ • No synchronization    │
            │ • Mailbox System        │      │ • For debugging         │
            │ • pkg/pregel runtime    │      │                         │
            └─────────────────────────┘      └─────────────────────────┘
```

### Component Layers

**Application Layer** (`pkg/agent`)
- High-level agent patterns: ReAct, RAG, Supervisor
- Built on top of Compiled

**Integration Layer** (`pkg/model`, `pkg/tool`, `pkg/retrieval`)
- LLM providers: OpenAI, Anthropic, Gemini
- Tool integrations: Functions, A2A, MCP
- Retrieval: Bedrock Knowledge Bases, Kendra

**Core Framework** (`pkg/graph`)
- **Compiled**: Orchestrates execution via StateManager + Executor
- **Builder**: Fluent API for graph construction
- **StateManager**: Composed interface with focused sub-interfaces
  - `StateReader`: Read-only state access for nodes
  - `StateWriter`: Write capabilities (extends StateReader)
  - `ChannelManager`: Channel lifecycle management
  - `AggregateManager`: Cross-node aggregate operations
  - `CheckpointManager`: State persistence and restoration
- **Executor**: Abstracts execution strategy (interface)
- **Built-in Pregel BSP**: Default parallel execution via `graphRuntime` + `pkg/pregel`
- **SimpleGraphExecutor**: Sequential execution for debugging

**Supporting Packages**
- `pkg/channel`: Topic, LastValue, BinaryOp channels
- `pkg/checkpoint`: Memory, SQL, DynamoDB persistence
- `pkg/message`: Human, AI, Tool message types

**Execution Engine** (`pkg/pregel` - PUBLIC API)
- Generic BSP runtime for custom extensions
- Pluggable MessageBus (Redis, Kafka, etc.)
- Custom Scheduler support

**Observability** (`pkg/metrics`, `pkg/trace`, `pkg/callbacks`)
- OpenTelemetry metrics and tracing
- Callback system for interception

**Key Design Principles:**
- **Separation of Concerns**: State, execution, and topology are independent
- **Interface Segregation**: Focused, composable interfaces (StateReader, ChannelManager, etc.)
- **Pluggable Execution**: Default Pregel BSP or custom Executor implementations
- **Extensibility**: Public `pkg/pregel` API for custom backends
- **Testability**: Mock small, focused interfaces instead of monolithic StateManager

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

    // Execute and collect all messages
    messages, err = graph.CollectMessages(compiled.Run(ctx, messages))
    if err != nil {
        log.Fatal(err)
    }
    
    // Print the final AI response
    for _, msg := range messages {
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

### Using Model Responses with Metadata

Access reasoning traces, usage statistics, and metadata from model responses:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/model"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
)

// Create model
mdl := openai.NewModel(
    openai.WithModel("gpt-4o"),
    openai.WithLogprobs(true, 5), // Request token probabilities
)

// For blocking mode - get final response with metadata
resp, err := model.Last(mdl.Generate(ctx, messages))
if err != nil {
    log.Fatal(err)
}

// Access the message content
fmt.Println("Response:", message.Stringify(resp.Message))

// Access native reasoning (for o1/o3, Gemini 2.0, Claude)
if resp.Reasoning != "" {
    fmt.Println("Model's reasoning:", resp.Reasoning)
}

// Check why generation stopped
fmt.Println("Finish reason:", resp.FinishReason) // "stop", "length", "tool_calls", etc.

// Track token usage for cost monitoring
if resp.Usage != nil {
    fmt.Printf("Tokens - Prompt: %d, Completion: %d, Reasoning: %d, Total: %d\n",
        resp.Usage.PromptTokens,
        resp.Usage.CompletionTokens,
        resp.Usage.ReasoningTokens,
        resp.Usage.TotalTokens)
}

// Analyze token probabilities (OpenAI only)
if resp.Logprobs != nil {
    for _, token := range resp.Logprobs.Content {
        fmt.Printf("Token: %s, Probability: %.2f%%\n",
            token.Token,
            math.Exp(token.Logprob)*100)
        
        // See alternative tokens the model considered
        for _, alt := range token.TopLogprobs {
            fmt.Printf("  Alternative: %s (%.2f%%)\n",
                alt.Token,
                math.Exp(alt.Logprob)*100)
        }
    }
}

// For streaming mode - get incremental responses
for resp, err := range mdl.Generate(ctx, messages) {
    if err != nil {
        log.Printf("Error: %v", err)
        break
    }
    
    // Print content as it arrives
    fmt.Print(message.Stringify(resp.Message))
    
    // Access partial reasoning (if available)
    if resp.Reasoning != "" {
        fmt.Printf("\n[Reasoning: %s]\n", resp.Reasoning)
    }
}
```

**Response Fields:**
- `Message` - The actual message content (text, tool calls, images)
- `Reasoning` - Native reasoning/thinking from o1/o3, Gemini 2.0, Claude (empty for other models)
- `FinishReason` - Why generation stopped: "stop", "length", "tool_calls", "content_filter"
- `Logprobs` - Token-level probabilities and alternatives (OpenAI only, requires opt-in)
- `Usage` - Token consumption with separate prompt/completion/reasoning tracking
- `Metadata` - Additional provider-specific information
- `Partial` - true for streaming chunks, false for final complete response

---

### Discovering Model Capabilities

Every model exposes its features and limitations via `Capabilities()`:

```go
import "github.com/hupe1980/agentmesh/pkg/model/openai"

// Create a model
mdl := openai.NewModel(openai.WithModel("gpt-4o"))

// Discover what it can do
caps := mdl.Capabilities()

fmt.Printf("Model: %s\n", mdl.Name())
fmt.Printf("Streaming: %v\n", caps.Streaming)
fmt.Printf("Tools: %v\n", caps.Tools)
fmt.Printf("Native Reasoning: %v\n", caps.NativeReasoning)
fmt.Printf("Vision: %v\n", caps.Vision)
fmt.Printf("Max Context: %d tokens\n", caps.MaxContextTokens)
fmt.Printf("Supported inputs: %v\n", caps.SupportedModalities)

// Conditionally use features based on capabilities
if caps.Tools {
    // Safe to bind tools
    if toolAware, ok := mdl.(model.ToolAware); ok {
        mdl = toolAware.BindTools(myTools...)
    }
}

if caps.NativeReasoning {
    fmt.Println("This model will populate Response.Reasoning automatically")
}

if caps.Vision {
    // Can send images
    messages = append(messages, message.NewHumanMessage(
        message.NewImagePart(imageData, "image/jpeg"),
    ))
}
```

**Capability Fields:**
- `Streaming` - Supports incremental response chunks
- `Tools` - Supports function calling via `BindTools()`
- `StructuredOutput` - Supports JSON schema validation
- `NativeReasoning` - Exposes internal reasoning in `Response.Reasoning`
- `Logprobs` - Can provide token-level probabilities
- `Vision` - Can process image inputs
- `Audio` - Can process audio inputs
- `MaxContextTokens` - Total context window size
- `MaxOutputTokens` - Maximum generation length
- `SupportedModalities` - List of accepted input types

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

    // Supervisor routes to math agent and returns answer
    events, _ := graph.Collect(supervisor.Run(ctx, messages))
    messages = graph.ExtractMessages(events)
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
builder.AddEdge(graph.StartNode, "step1")
builder.AddEdge("step1", "step2")
builder.AddEdge("step2", graph.EndNode)

// Compile and run
compiled, _ := builder.Compile()
events, _ := graph.Collect(compiled.Run(context.Background(), initialMessages))
messages := graph.ExtractMessages(events)
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

Explore **19 comprehensive examples** demonstrating different use cases and patterns:

| Example | Description | Key Features |
|---------|-------------|--------------|
| 🎯 [basic_agent](examples/basic_agent) | Simple ReAct agent with tools | Agent creation, tool calling, message handling |
| 👥 [supervisor_agent](examples/supervisor_agent) | Multi-agent coordination pattern | Supervisor routing, worker specialists, handoff tools |
| 🏗️ [state_builder](examples/state_builder) | Simplified state initialization | Fluent API, channel patterns, reduced boilerplate |
| 🔌 [mcp_tools](examples/mcp_tools) | Model Context Protocol integration | Dynamic tool discovery, MCP toolsets, runtime tools |
| 🔒 [wasm_tool](examples/wasm_tool) | WebAssembly sandboxed tools | Memory-safe isolation, Rust WASM modules, security policies |
| 🌊 [streaming](examples/streaming) | Real-time event streaming | Live updates, partial results, progress tracking |
| 🔀 [conditional_flow](examples/conditional_flow) | Dynamic routing based on state | Conditional edges, branching logic, flow control |
| ⚡ [parallel_tasks](examples/parallel_tasks) | Concurrent execution patterns | Parallel nodes, fan-out/fan-in, result aggregation |
| ⏸️ [human_pause](examples/human_pause) | Human-in-the-loop workflows | Interrupt, resume, user approval |
| ⏰ [time_travel](examples/time_travel) | Debug with state versioning | Checkpointing, state replay, time-travel debugging |
| 💾 [checkpointing](examples/checkpointing) | Fault-tolerant workflows | Auto-save, auto-resume, persistence |
| 🔐 [checkpoint_signing](examples/checkpoint_signing) | HMAC-SHA256 checkpoint integrity | Tamper detection, cryptographic signing, security |
| 🛡️ [input_validation](examples/input_validation) | Message size/count validation | DoS prevention, resource limits, security |
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

// Create checkpointer
checkpointer := checkpoint.NewInMemoryCheckpointer()

// Compile graph
compiled, _ := builder.Compile()

// Execute - state is automatically saved
runID := "conversation-123"
events, _ := graph.Collect(compiled.Run(ctx, initialMessages,
    graph.WithCheckpointer(checkpointer),
    graph.WithRunID(runID),
    graph.WithCheckpointConfig(checkpoint.Config{SaveInterval: 1}),
))

// Resume from checkpoint after failure
events, _ = graph.Collect(compiled.Run(ctx, initialMessages,
    graph.WithCheckpointer(checkpointer),
    graph.WithRunID(runID),
    graph.WithCheckpointConfig(checkpoint.Config{AutoRestore: true}),
))
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
    parentMessages := graph.ExtractMessages(s.EventsSnapshot())
    events, err := graph.Collect(researchGraph.Run(ctx, parentMessages))
    if err != nil {
        return nil, err
    }
    return &graph.NodeResult{
        Messages: graph.ExtractMessages(events),
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

### 🔒 WASM Tool Sandboxing

Execute untrusted or third-party code securely using WebAssembly:

```go
import "github.com/hupe1980/agentmesh/pkg/tool/wasm"

// Load WASM module
wasmBytes, _ := os.ReadFile("calculator.wasm")

// Create sandboxed tool with compute-only policy
tool, err := wasm.NewWASMTool(
    "calculator",
    "Evaluate mathematical expressions with guaranteed isolation",
    wasmBytes,
    wasm.WithPolicy(wasm.ComputeOnlyPolicy()),  // No network, filesystem, or syscalls
)

// Use in agent - tool runs in isolated WASM environment
agent, _ := agent.NewReActAgent(model, []tool.Tool{tool})
```

**Security Policies:**

```go
// Compute-only: Pure computation, no external access
wasm.ComputeOnlyPolicy()

// Network-only: HTTP/API access, no filesystem
wasm.NetworkOnlyPolicy()

// File processing: Access specific directories
wasm.FileProcessingPolicy([]string{"/data"}, false)

// Deterministic: Fresh instance per call, reproducible results
wasm.DeterministicPolicy()

// Custom: Fine-grained control
policy := &wasm.SandboxPolicy{
    MaxMemoryBytes:    50 * 1024 * 1024,  // 50 MB
    TimeoutDuration:   5 * time.Second,
    AllowNetworkAccess: false,
    AllowFilesystemAccess: false,
    SecurityLevel: wasm.SecurityLevelUntrusted,
}
```

**Why WASM sandboxing?**
- ✅ **Runtime-enforced isolation** - Cannot be bypassed by malicious code
- ✅ **Memory-safe** - Isolated linear memory, no access to host or other processes
- ✅ **Controlled capabilities** - All host access via explicitly granted WASI interfaces
- ✅ **Resource limits** - Strict memory, timeout, and CPU constraints
- ✅ **Cross-platform** - Same security on Linux, macOS, Windows
- ✅ **Minimal overhead** - 1-5ms per invocation

See the [wasm_tool example](examples/wasm_tool) for building WASM modules with Rust.

### 📞 Plugin System

Extend AgentMesh with a type-safe plugin system for cross-cutting concerns:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/callbacks"
    "github.com/hupe1980/agentmesh/pkg/callbacks/plugins"
    "github.com/hupe1980/agentmesh/pkg/model"
)

// Create plugin manager
pm := callbacks.NewPluginManager()

// Register built-in plugins
pm.Register(ctx, plugins.NewLoggingPlugin(log.Default(), "[AgentMesh]"))

// Create custom plugin with typed config
type CachePlugin struct {
    callbacks.NoopPlugin  // Embed for default no-op implementations
    cache *Cache
}

func (p *CachePlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
    // Check cache before model invocation
    if cached := p.cache.Get(req); cached != nil {
        return cached, nil  // Short-circuit with cached response
    }
    return nil, nil  // Continue to model
}

func (p *CachePlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
    // Cache the response
    p.cache.Set(req, resp)
    return nil, nil  // No transformation
}

// Register custom plugin
pm.Register(ctx, &CachePlugin{cache: myCache})

// Use with agents
compiled, _ := agent.NewReActAgent(
    model,
    tools,
    agent.WithModelCallbacks(pm),
    agent.WithToolCallbacks(pm),
)
```

**Plugin Lifecycle Hooks:**
- `Init/Shutdown` - Resource management (connections, cleanup)
- `OnGraphStart/OnGraphComplete/OnGraphError` - Graph lifecycle tracking
- `BeforeNode/AfterNode` - Node-level interception
- `BeforeModel/AfterModel/OnModelError` - Model request/response transformation (uses `model.Request/Response`)
- `BeforeTool/AfterTool/OnToolError` - Tool execution monitoring
- `OnStateChange/OnMessage` - State and message tracking

**Why Plugins?**
- **Type-safe configuration** - Pass dependencies via constructor, not `map[string]any`
- **Simple registration** - Order-based execution, no priority management
- **Composable** - Embed `NoopPlugin` and override only what you need
- **Request/Response based** - Model hooks use `model.Request/Response` for short-circuiting

### 📊 Observability

Built-in OpenTelemetry integration:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/logging"
    "github.com/hupe1980/agentmesh/pkg/metrics"
    "github.com/hupe1980/agentmesh/pkg/trace"
    "github.com/hupe1980/agentmesh/pkg/graph"
)

// Configure observability providers
logger := logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatJSON)
metricsProvider := metrics.NewOpenTelemetry(meterProvider)
traceProvider := trace.NewOpenTelemetry(tracerProvider)

// Automatic instrumentation - structured logs throughout execution!
events, _ := graph.Collect(compiled.Run(ctx, initialMessages,
    graph.WithLogger(logger),          // Structured logging (JSON/text)
    graph.WithTracer(traceProvider),   // Distributed tracing
    graph.WithMetrics(metricsProvider), // Metrics collection
))
```


### �📊 Observability

Built-in OpenTelemetry integration:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/logging"
    "github.com/hupe1980/agentmesh/pkg/metrics"
    "github.com/hupe1980/agentmesh/pkg/trace"
    "github.com/hupe1980/agentmesh/pkg/graph"
)

// Configure observability providers
logger := logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatJSON)
metricsProvider := metrics.NewOpenTelemetry(meterProvider)
traceProvider := trace.NewOpenTelemetry(tracerProvider)

// Automatic instrumentation - structured logs throughout execution!
events, _ := graph.Collect(compiled.Run(ctx, initialMessages,
    graph.WithLogger(logger),          // Structured logging (JSON/text)
    graph.WithTracer(traceProvider),   // Distributed tracing
    graph.WithMetrics(metricsProvider), // Metrics collection
))
```

**Automatically Tracked:**
- ✅ Node execution count and duration
- ✅ Superstep execution time
- ✅ Error rates per node with labels
- ✅ Graph-level execution metrics

**Automatic Distributed Tracing:**
- ✅ Span per graph execution
- ✅ Nested spans for each node
- ✅ Checkpoint operation spans
- ✅ Providers available in nodes via `FromContext()`

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
// - Graph/Node: Vertex computation model
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
| **`Run()`** | `Node`, `Compiled` | Execute logic, return iterator | Node execution, graph execution with streaming |
| **`Execute()`** | `Executor`, Adapters | Strategy implementation | Internal execution strategy pattern |

### Rationale

- **`Run`** - Used for execution that returns results directly (nodes) or via iterators (graphs)
- **`Call`** - Used for function invocation semantics (tools, callbacks)
- **`Execute`** - Used for strategy pattern implementations (executor interfaces, adapters)
- **`Call`** - Used for function invocation semantics (tools, callbacks)

### Examples

```go
// Tools use Call() - function invocation
result, err := weatherTool.Call(ctx, `{"location": "Boston"}`)

// Nodes use Run() - low-level execution
result, err := node.Run(ctx, state)

// Graphs use Run() - high-level streaming API (returns iterator)
seq := compiled.Run(ctx, initialMessages)
for event, err := range seq {
    // handle event and error
}

// Helper: Collect all events for blocking-style execution
events, err := graph.Collect(compiled.Run(ctx, initialMessages))
messages := graph.ExtractMessages(events)

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

Made with ❤️ by [hupe1980](https://github.com/hupe1980)

</div>
