# 🤖 AgentMesh

[![Go Reference](https://pkg.go.dev/badge/github.com/hupe1980/agentmesh.svg)](https://pkg.go.dev/github.com/hupe1980/agentmesh)
[![Documentation](https://img.shields.io/badge/docs-online-success)](https://hupe1980.github.io/agentmesh/)
[![Go Report Card](https://goreportcard.com/badge/github.com/hupe1980/agentmesh)](https://goreportcard.com/report/github.com/hupe1980/agentmesh)

AgentMesh is a Go-first framework for orchestrating AI agents with predictable flows, typed tool calls, and built-in observability.

## ✨ Highlights

- **Deterministic orchestration** – Compose sequential, routing, parallel, or looping agents with explicit error handling.
- **Typed tools & retrieval** – Wrap Go functions, sub-agents, or search connectors with JSON Schema validation.
- **Streaming-first runtime** – Stream partial events, route transfers, and merge structured output deterministically.
- **Pluggable observability** – Bring your own logging, tracing, metrics, and persistence layers.

---

## 📦 Install

```sh
go get github.com/hupe1980/agentmesh
```

---

## 🚀 Quick start

Requirements:
- Go 1.24 or newer
- `OPENAI_API_KEY` exported in your environment

The snippet below mirrors [`examples/basic_agent/main.go`](examples/basic_agent/main.go) and creates a model-backed agent with streaming output and logging:

```go
package main

import (
  "context"
  "fmt"
  "log"
  "os"
  "time"

  am "github.com/hupe1980/agentmesh"
  "github.com/hupe1980/agentmesh/core"
  "github.com/hupe1980/agentmesh/logging"
  "github.com/hupe1980/agentmesh/model/openai"
)

func main() {
  if os.Getenv("OPENAI_API_KEY") == "" {
    log.Fatal("OPENAI_API_KEY is required")
  }

  model := openai.NewModel()

  agent, err := am.NewModelAgent("basic", model, func(o *am.ModelAgentOptions) {
    o.Instructions = core.NewInstructionsFromText("Keep responses short and helpful.")
  })
  if err != nil {
    log.Fatalf("build agent: %v", err)
  }

  app := am.NewApp("basic_app", agent)

  runner := am.NewRunner(app, func(o *am.RunnerOptions) {
    o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
  })
  defer runner.Close()

  ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
  defer cancel()

  parts := []core.Part{core.NewPartFromText("Hello! What can you help with?")}

  runID, text, err := runner.RunFinalText(ctx, "user1", "session1", parts)
  if err != nil {
    log.Fatalf("run failed: %v", err)
  }

  fmt.Printf("=== Run %s ===\n%s\n", runID, text)
}
```

From here you can add tools, switch to sequential/parallel flows, or persist sessions. Check the [Getting Started guide](https://hupe1980.github.io/agentmesh/getting-started/) for a deeper walkthrough.

---

## 🛠️ What you can build
- Compose [`Sequential`](agent/sequential.go), [`Routing`](agent/routing.go), [`Parallel`](agent/parallel.go), and [`Loop`](agent/loop.go) agents for complex flows.
- Use transfer events to hand off control across agents or escalate when necessary.
- Rely on the event stream to interleave partial outputs with tool activity.

### 🔧 Tools and retrieval

- Create function tools with [`tool.NewFuncTool`](tool/func.go) or wrap an entire agent via [`tool.NewAgentTool`](tool/agent.go).
- Flag background or out-of-band work with [`tool.NewLongRunningTool`](tool/long_running.go) to start and track lengthy external tasks without blocking the agent.
- Promote search connectors into tools using [`retrieval.NewTool`](tool/retrieval/func.go) and merge multiple sources with [`retrieval.NewMergerRetriever`](tool/retrieval/merger.go).
- Choose from built-in adapters for Amazon Bedrock, Amazon Kendra, and LangChainGo retrievers, or implement your own.

### 🔭 Observability & state

- Plug in custom loggers, metrics, and tracers using [`logging`](logging/), [`metrics`](metrics/), and [`trace`](trace/) packages.
- Store artifacts, session data, and memories with pluggable backends under [`artifact`](artifact/) and [`session`](session/).
- Control concurrency and buffering through runner options for predictable throughput.

---

## 🗺️ Documentation map

- **[Getting Started](https://hupe1980.github.io/agentmesh/getting-started/)** – Install, run the basic agent, and learn the runner lifecycle.
- **[Agents Guide](https://hupe1980.github.io/agentmesh/agents/)** – Build sequential, parallel, loop, and functional agents.
- **[Models Guide](https://hupe1980.github.io/agentmesh/models/)** – Connect OpenAI, LangChainGo, gateway, or functional models with structured outputs.
- **[Tools Guide](https://hupe1980.github.io/agentmesh/tools/)** – Author function tools, toolsets, retrieval mergers, and MCP adapters.
- **[Observability](https://hupe1980.github.io/agentmesh/observability/)** – Wire logging, metrics, tracing, and inspect emitted events.
- **[Architecture](https://hupe1980.github.io/agentmesh/architecture/)** – Understand flows, the runner, plugins, and execution context.

---

## 🧪 Examples

Run any example locally with `go run`:

```sh
go run ./examples/basic_agent/main.go
```

Featured samples:

- Basic model agent – [`examples/basic_agent`](examples/basic_agent/main.go)
- Tooling bundle – [`examples/tool_usage`](examples/tool_usage/main.go)
- Agent-as-a-tool – [`examples/agent_tool`](examples/agent_tool/main.go)
- Structured output – [`examples/output_schema`](examples/output_schema/main.go)
- Multi-agent fan-out – [`examples/multi_agent`](examples/multi_agent/main.go)
- Agent transfer – [`examples/transfer_agent`](examples/transfer_agent/main.go)
- OpenTelemetry wiring – [`examples/opentelemetry`](examples/opentelemetry/main.go)

More integrations live under [`examples/`](examples/).

---

## 🧰 Development workflow

Use the `just` recipes provided in the repo:

```sh
just test        # go test ./...
just test-race   # go test ./... -race
just lint        # golangci-lint with project config
just cover       # generate HTML coverage report
```

Prefer raw tools?

```sh
go test ./... -race
golangci-lint run --config .golangci.yml
```

To iterate on the docs locally:

```sh
just docs-serve
```

This launches Jekyll with livereload at `http://localhost:4000` using the bundled `docs/_config.dev.yml`.

---

## 🏭 Production checklist

- Swap the in-memory stores (session, memory, artifact) for your durable implementations.
- Enforce tool allow-lists and sanitize tool responses before surfacing them to models.
- Tune runner buffers, flow concurrency, and limiter settings to match downstream capacity.
- Instrument with OpenTelemetry exporters or your preferred providers through runner options.
- Cache model responses and trim session history to manage cost.

---

## 🤝 Contributing

We welcome issues and pull requests! Review [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines, coding style, and the release process.

---

## 📄 License

MIT © [AgentMesh contributors](LICENSE)
