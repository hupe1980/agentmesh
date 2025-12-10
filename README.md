# 🤖🕸️ AgentMesh

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/hupe1980/agentmesh)](https://goreportcard.com/report/github.com/hupe1980/agentmesh)
[![GoDoc](https://pkg.go.dev/badge/github.com/hupe1980/agentmesh.svg)](https://pkg.go.dev/github.com/hupe1980/agentmesh)

> Production-grade multi-agent orchestration framework powered by Pregel-style bulk-synchronous parallel (BSP) graph processing. Build sophisticated AI agent workflows with parallel execution, state management, and enterprise-grade observability.

**Requires Go 1.24+**

---

## ✨ Key Features

### Core Engine
- **🔄 Pregel BSP Execution** - Parallel graph processing with optimized concurrency (4-10x faster state access)
- **🧠 LLM Integration** - Native support for OpenAI, Anthropic, Gemini, Amazon Bedrock, Ollama with streaming and reasoning models
- **💾 State Management** - Lock-free channel-based state with checkpointing, managed value descriptors, and resume-time rehydration hooks
- **🛠️ Tool Orchestration** - Type-safe function calling with automatic schema generation
- **🔀 Model Routing** - Intelligent model selection based on cost, capabilities, and availability

### Production Ready
- **✅ Graph Validation** - Comprehensive compile-time error checking (cycles, missing nodes, unreachable paths)
- **💾 Checkpointing** - Persistent state with auto-resume, encryption, and signing
- **♻️ Zero-Copy Resume** - Copy-on-write checkpoint restores reuse the saved map and only allocate when keys mutate (10k+ key checkpoints resume without GC spikes)
- **⏸️ Human-in-the-Loop** - Approval workflows with conditional guards and audit trails
- **📊 Observability** - Built-in OpenTelemetry metrics, non-blocking event bus fan-out, and distributed tracing
- **🔁 Resilience** - Configurable retry policies, circuit breakers, and timeouts
- **🔒 Security** - WASM sandboxing for untrusted code, input validation, and integrity checks

### AI/ML Features
- **🔢 Embeddings & Memory** - Semantic search and long-term conversation storage
- **🔍 RAG Integration** - AWS Bedrock Knowledge Bases and Kendra support
- **🧠 Native Reasoning** - First-class support for o1, o3, Gemini 2.0, Claude reasoning models
- **📝 Prompt Templates** - Variable substitution with reusable patterns

### Extensibility
- **🤝 A2A Protocol** - Multi-agent collaboration with standardized communication
- **🔌 MCP Support** - Dynamic tool discovery from Model Context Protocol servers
- **🌐 LangChainGo Tools** - Import existing tool ecosystem
- **⚙️ Custom Backends** - Pluggable MessageBus for distributed execution (Redis, Kafka)

---

## 🚀 Quick Start

### Installation

```bash
go get github.com/hupe1980/agentmesh@latest
```

### Hello World ReAct Agent

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "strings"

    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/message"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
    "github.com/hupe1980/agentmesh/pkg/tool"
)

// WeatherArgs defines the JSON schema for the weather tool.
type WeatherArgs struct {
    Location string `json:"location" jsonschema:"description=The city to get weather for"`
}

func main() {
    ctx := context.Background()

    // Validate API key
    if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" {
        log.Fatal("OPENAI_API_KEY environment variable is required")
    }

    // Create OpenAI model (uses OPENAI_API_KEY env var)
    model := openai.NewModel()

    // Define a tool with typed arguments
    weatherTool, err := tool.NewFuncTool(
        "get_weather",
        "Get current weather for a location",
        func(ctx context.Context, args WeatherArgs) (string, error) {
            return fmt.Sprintf("Weather in %s: Sunny, 72°F", args.Location), nil
        },
    )
    if err != nil {
        log.Fatal(err)
    }

    // Create ReAct agent
    reactAgent, err := agent.NewReAct(model,
        agent.WithTools(weatherTool),
        agent.WithMaxIterations(5),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Execute agent and get the final result
    messages := []message.Message{
        message.NewHumanMessage("What's the weather in San Francisco?"),
    }

    lastMsg, err := graph.Last(reactAgent.Run(ctx, messages))
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(lastMsg.String())
}
```

**Output:**
```
Thought: I need to check the weather in San Francisco
Action: get_weather("San Francisco")
Observation: Weather in San Francisco: Sunny, 72°F
The weather in San Francisco is currently sunny with a temperature of 72°F.
```

---

## 📚 Documentation

### Getting Started
- 📘 **[Getting Started Guide](docs/getting-started.md)** - Complete tutorial with examples
- 🏗️ **[Architecture Overview](docs/architecture.md)** - Understanding the Pregel BSP design
- 📖 **[API Reference](https://pkg.go.dev/github.com/hupe1980/agentmesh)** - Complete godoc

### Core Concepts
- 🕸️ **[Graph Building](docs/core-concepts.md)** - Nodes, edges, and execution flow
- 🗂️ **[State Management](docs/state-management.md)** - Channels, reducers, checkpointing, and approval workflows
- 🔧 **[Tools Guide](docs/tools.md)** - Building and integrating tools
- 🤖 **[Model Integration](docs/models.md)** - LLM provider setup and configuration

### Advanced Features
- 🤖 **[Agent Patterns](docs/agents.md)** - ReAct, RAG, and Supervisor agents
- 📊 **[Observability](docs/observability.md)** - Metrics, tracing, and monitoring
- 🔌 **[Middleware](docs/middleware.md)** - Caching, rate limiting, circuit breakers
- 📅 **[Custom Schedulers](docs/advanced.md#custom-schedulers)** - Priority-based and resource-aware vertex execution
- 🧠 **[Memory & Embeddings](docs/memory.md)** - Semantic search and conversation storage
- 🤝 **[A2A Protocol](docs/a2a.md)** - Multi-agent collaboration
- 🔒 **[WASM Sandboxing](docs/wasm-sandboxing.md)** - Secure untrusted code execution

---

## 🎨 Examples

Explore **31 comprehensive examples** in the [`examples/`](examples/) directory:

| Example | Description |
|---------|-------------|
| **[basic_agent](examples/basic_agent/)** | Simple ReAct agent with tools |
| **[conversational_agent](examples/conversational_agent/)** | Agent with long-term memory across turns |
| **[document_loader](examples/document_loader/)** | Document loading and ingestion pipeline |
| **[supervisor_agent](examples/supervisor_agent/)** | Multi-agent coordination with supervisor |
| **[reflection_agent](examples/reflection_agent/)** | Self-critique and iterative refinement |
| **[checkpointing](examples/checkpointing/)** | State persistence and resume |
| **[human_approval](examples/human_approval/)** | Approval workflows with conditional guards |
| **[parallel_tasks](examples/parallel_tasks/)** | Concurrent node execution |
| **[streaming](examples/streaming/)** | Real-time response streaming |
| **[middleware](examples/middleware/)** | Rate limiting and circuit breakers |
| **[custom_scheduler](examples/custom_scheduler/)** | Priority and resource-aware scheduling |
| **[observability](examples/observability/)** | OpenTelemetry integration |
| **[a2a_integration](examples/a2a_integration/)** | Agent-to-agent communication |
| **[wasm_tool](examples/wasm_tool/)** | Sandboxed tool execution |

[See all examples →](examples/)

### Running Examples

```bash
# Run any example
cd examples/basic_agent
go run main.go

# Set required environment variables
export OPENAI_API_KEY=your-key-here
export ANTHROPIC_API_KEY=your-key-here  # For Anthropic examples
```

---

## 🏗️ Architecture

AgentMesh uses a **layered architecture** with clean separation of concerns:

```
┌─────────────────────────────────────────────────────────────┐
│           Application Layer (pkg/agent)                     │
│  • ReActAgent, SupervisorAgent, RAGAgent                    │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│           Graph Orchestration (pkg/graph)                   │
│  • Workflow construction • State management • Validation    │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│           Execution Engine (pkg/pregel)                     │
│                                                             │
│  • BSP Runtime      • Worker pools      • MessageBus        │
│  • Superstep sync   • Sharded frontier  • Backpressure      │
└─────────────────────────────────────────────────────────────┘
```

**Key Components:**
- **Graph** - Define nodes, edges, and execution flow with `NodeFunc`
- **BSPState** - Copy-on-write state with typed `Key[T]` and `ListKey[T]`
- **Pregel Runtime** - Bulk-synchronous parallel execution with sharded message passing
- **Checkpointer** - State persistence with encryption, signing, and two-phase commit
- **Agents** - High-level abstractions (ReAct, Supervisor, RAG)

[Learn more about the architecture →](docs/architecture.md)

---

## 🧪 Testing

```bash
# Run all tests
go test ./...

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Run benchmarks
go test ./... -bench=. -benchmem
```

---

## 🤝 Contributing

Contributions are welcome!

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Write tests for your changes
4. Commit your changes (`git commit -m 'Add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

### Development Guidelines

- All new code must have tests (target 85%+ coverage)
- Run `go fmt` and `golangci-lint` before committing
- Update documentation for public API changes
- Add examples for new features

---

## 📄 License

Licensed under the **Apache License 2.0** - see [LICENSE](LICENSE) for details.

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
