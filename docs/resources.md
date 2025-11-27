---
layout: doc
title: Resources
permalink: /resources/
hero:
  title: Explore AgentMesh further
  description: Find examples, API documentation, and contribution guidelines.
  primary_cta:
    label: Browse examples
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples"
    external: true
  secondary_cta:
    label: API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh"
    external: true
sidebar:
  - title: Examples
    url: "#examples"
  - title: API documentation
    url: "#api-documentation"
  - title: Community
    url: "#community"
  - title: Development
    url: "#development"
---

## Examples {#examples}

The repository includes **17 comprehensive examples** demonstrating core features and production patterns:

### Getting started
- **`basic_agent`** – Simple ReAct agent with tool calling and message handling
- **`state_builder`** – Simplified state initialization with fluent API
- **`streaming`** – Real-time event streaming for responsive UIs and progress tracking

### Tool integration
- **`mcp_tools`** – Model Context Protocol integration with dynamic tool discovery

### Advanced workflows
- **`conditional_flow`** – Dynamic routing and branching based on state
- **`parallel_tasks`** – Concurrent execution with fan-out/fan-in patterns
- **`subgraph`** – Modular workflows with reusable graph components

### State management
- **`checkpointing`** – Automatic state persistence and fault recovery
- **`time_travel`** – Debug workflows by replaying from any superstep
- **`message_retention`** – Conversation history management and pruning strategies

### Production features
- **`observability`** – OpenTelemetry metrics, distributed tracing, and monitoring
- **`human_pause`** – Human-in-the-loop workflows with interrupt/resume
- **`a2a_integration`** – Agent-to-Agent protocol for multi-agent coordination
- **`middleware`** – Middleware system for graph/model/tool execution layers
- **`circuit_breaker`** – Circuit breaker middleware for fault tolerance
- **`guardrails`** – Content filtering, PII protection with custom middleware

### Embeddings & RAG
- **`openai_embedder`** – Text embeddings for semantic search and RAG workflows

Browse all examples: [github.com/hupe1980/agentmesh/tree/main/examples](https://github.com/hupe1980/agentmesh/tree/main/examples)

---

## API documentation {#api-documentation}

Complete API reference with examples:

- **Main package** – [pkg.go.dev/github.com/hupe1980/agentmesh](https://pkg.go.dev/github.com/hupe1980/agentmesh)
- **Graph package** – [pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph)
- **Agent package** – [pkg.go.dev/github.com/hupe1980/agentmesh/pkg/agent](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/agent)
- **Model package** – [pkg.go.dev/github.com/hupe1980/agentmesh/pkg/model](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/model)
- **Tool package** – [pkg.go.dev/github.com/hupe1980/agentmesh/pkg/tool](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/tool)

---

## Community {#community}

### Get help

- **GitHub Issues** – [Report bugs or request features](https://github.com/hupe1980/agentmesh/issues)
- **Discussions** – [Ask questions and share ideas](https://github.com/hupe1980/agentmesh/discussions)

### Contributing

Contributions are welcome! See:
- **Contributing guide** – [CONTRIBUTING.md](https://github.com/hupe1980/agentmesh/blob/main/CONTRIBUTING.md)
- **Code of conduct** – [CODE_OF_CONDUCT.md](https://github.com/hupe1980/agentmesh/blob/main/CODE_OF_CONDUCT.md)

---

## Development {#development}

### Running tests

```bash
# Run all tests
go test ./...

# Run with race detection
go test ./... -race

# Run with coverage
go test ./... -cover
```

### Benchmarks

```bash
# Run all benchmarks
go test ./... -bench=.

# Run specific benchmark
go test ./pkg/graph -bench=BenchmarkOptimized
```

### Local documentation

Preview documentation locally:

```bash
cd docs
bundle install
bundle exec jekyll serve --livereload
```

Navigate to `http://localhost:4000`
