---
layout: doc
title: Getting Started
permalink: /getting-started/
hero:
  title: Build with AgentMesh in minutes
  description: Install the library, create your first ReAct agent, and explore graph-based workflows.
  primary_cta:
    label: Install the module
    href: "#install"
  secondary_cta:
    label: Browse examples →
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples"
    external: true
sidebar:
  - title: Install
    url: "#install"
  - title: Quick start
    url: "#quick-start"
  - title: Explore examples
    url: "#explore-examples"
  - title: Local docs preview
    url: "#local-docs-preview"
  - title: Next steps
    url: "#next-steps"
---

## Install {#install}

```bash
go get github.com/hupe1980/agentmesh@latest
```

> Requires Go 1.24+ and appropriate model credentials (for example, `OPENAI_API_KEY`).

---

## Quick start {#quick-start}

Create a simple ReAct agent that can call tools to answer questions:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/message"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
    "github.com/hupe1980/agentmesh/pkg/tool"
)

type WeatherArgs struct {
    Location string `json:"location"`
}

func main() {
    // Define a tool
    weatherTool, err := tool.NewFuncTool(
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
    if err != nil {
        log.Fatal(err)
    }

    // Create a ReAct agent (returns *message.Graph)
    reactAgent, err := agent.NewReAct(
        openai.NewModel(),
        agent.WithTools(weatherTool),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Execute the agent
    ctx := context.Background()
    messages := []message.Message{
        message.NewSystemMessageFromText("You are a helpful weather assistant."),
        message.NewHumanMessageFromText("What's the weather in Paris?"),
    }

    // Execute the agent with iterator pattern
    for msg, err := range reactAgent.Run(ctx, messages) {
        if err != nil {
            log.Fatal(err)
        }
        // Print AI response content
        if aiMsg, ok := msg.(*message.AIMessage); ok {
            fmt.Println(aiMsg.Content())
        }
    }
}
```

Run it:

```bash
export OPENAI_API_KEY="your-key"
go run main.go
```

The agent will automatically reason about the user's query, decide to call the weather tool, and generate a natural language response based on the tool's output.

---

## Explore examples {#explore-examples}

The repository includes **18 comprehensive examples** demonstrating key features:

- **`examples/basic_agent`** – Simple ReAct agent with tool calling
- **`examples/supervisor_agent`** – Multi-agent coordination with supervisor pattern
- **`examples/state_builder`** – Simplified state initialization with fluent API
- **`examples/mcp_tools`** – Model Context Protocol integration
- **`examples/streaming`** – Real-time event streaming for responsive UIs
- **`examples/conditional_flow`** – Dynamic routing based on agent outputs
- **`examples/parallel_tasks`** – Parallel execution of independent graph nodes
- **`examples/human_pause`** – Human-in-the-loop workflows
- **`examples/human_approval`** – Approval workflows with conditional guards and audit trails
- **`examples/time_travel`** – Debug workflows by replaying from checkpoints
- **`examples/checkpointing`** – Automatic state persistence and recovery
- **`examples/middleware`** – Middleware system demonstration
- **`examples/circuit_breaker`** – Fault tolerance patterns
- **`examples/guardrails`** – Content filtering and PII protection
- **`examples/observability`** – OpenTelemetry metrics and distributed tracing
- **`examples/subgraph`** – Compose complex workflows from reusable graphs
- **`examples/message_retention`** – Conversation history management
- **`examples/semantic_caching`** – Text embeddings for semantic search
- **`examples/a2a_integration`** – Agent-to-Agent protocol

Browse all examples: [github.com/hupe1980/agentmesh/tree/main/examples](https://github.com/hupe1980/agentmesh/tree/main/examples)

Run any example with:

```bash
go run ./examples/<dir>/main.go
```

---

## Local docs preview

The devcontainer (or a local Ruby install) includes everything you need to iterate on this documentation site.

```bash
cd docs
bundle config set --local path 'vendor/bundle'
bundle install
bundle exec jekyll serve --livereload --host 0.0.0.0 --config _config.yml,_config.dev.yml
```

Prefer a single command in the devcontainer? Run `just docs-serve` from the repo root.

Once the server is running, open `http://localhost:4000` for the rendered docs with live reload.

---

## Graph Introspection {#introspection}

Debug and visualize your graphs with the introspection API:

```go
// Inspect graph structure
nodes := compiled.GetNodes()
topo := compiled.GetTopology()
fmt.Printf("Entry points: %v\n", topo.EntryPoints)
fmt.Printf("Max depth: %d\n", topo.MaxDepth)

// Get execution paths
paths := compiled.GetExecutionPath(10)

// Generate Mermaid flowchart
flowchart := compiled.GenerateMermaidFlowchart("TD")
os.WriteFile("graph.mmd", []byte(flowchart), 0644)
```

See the [graph_introspection example](https://github.com/hupe1980/agentmesh/tree/main/examples/graph_introspection) for complete usage.

---

## Next steps {#next-steps}

- **[Architecture](/architecture/)** – Understand the Pregel BSP execution model and graph builder pattern
- **[Agents](/agents/)** – Learn about ReAct agents, RAG agents, and custom graph workflows
- **[Tools](/tools/)** – Create function tools with automatic JSON schema generation
- **[Models](/models/)** – Connect OpenAI, Anthropic, LangChainGo, or custom LLM providers
