# 🤖 AgentMesh

[![Go Reference](https://pkg.go.dev/badge/github.com/hupe1980/agentmesh.svg)](https://pkg.go.dev/github.com/hupe1980/agentmesh)
[![Go Report Card](https://goreportcard.com/badge/github.com/hupe1980/agentmesh)](https://goreportcard.com/report/github.com/hupe1980/agentmesh)

**Composable, testable, and extensible multi-agent orchestration for Go.**

AgentMesh helps you build production-grade AI systems in Go. It provides 
composable agent patterns (sequential, parallel, looping), streaming event 
delivery, plugin hooks, persistent session storage, and first-class 
observability (logging, metrics, tracing). Everything is deterministic 
and testable by design.

- 🧩 Composability: Sequential, Parallel, and Loop agents nest arbitrarily  
- 📡 Streaming: Unified event stream with partial and final responses  
- 🛠️ Tools: Strongly typed, schema-validated tool calls with bounded execution  
- 🗂️ Pluggable stores: memory, artifacts, session; tools run with a scoped ToolContext  
- 🔭 Observability: Structured logging, metrics, and tracing (OpenTelemetry providers included)  
- 🔁 Determinism: Explicit control over ordering and side-effects for testability 

---

## 📚 Table of Contents

- [Install](#install)
- [Quick Start](#quick-start)
- [Examples](#examples)
- [Core Concepts](#core-concepts)
- [Multi-agent Patterns](#multi-agent-patterns)
- [Observability](#observability)
- [Backpressure](#backpressure)
- [Development](#development)
- [Production Considerations](#production-considerations)
- [Contributing](#contributing)
- [License](#license)

---

## 📦 Install

```sh
go get github.com/hupe1980/agentmesh
```

Requirements:
- Go >= 1.24
- For OpenAI examples: set OPENAI_API_KEY

---

## 🚀 Quick Start

Minimal single-agent run mirroring examples/basic_agent.

```go
package main

import (
  "context"
  "fmt"
  "log"
  "os"
  "time"

  "github.com/hupe1980/agentmesh/agent"
  "github.com/hupe1980/agentmesh/core"
  "github.com/hupe1980/agentmesh/model/openai"
  "github.com/hupe1980/agentmesh/runner"
)

func main() {
  model := openai.NewModel()

  basic, err := agent.NewModelAgent("basic_agent", model)
  if err != nil {
		log.Fatalf("failed to create agent: %v", err)
	}

  r := runner.New("basic_app", basic)
  defer r.Close()

  userParts := []core.Part{core.NewPartFromText("Hello! What can you do?")}

  runID, results, err := r.Run(context.Background(), "user1", "sess1", userParts)
  if err != nil {
    log.Fatalf("run failed: %v", err)
  }

  fmt.Printf("=== Basic Agent [runID=%s] ===\n", runID)

  for res := range results {
    if res.Err != nil {
      log.Printf("error: %v", res.Err)
      continue
    }
    
    fmt.Println(res.Event.Text())
  }
}
```

Reference: [examples/basic_agent/main.go](examples/basic_agent/main.go)

---

## 📖 Examples

- Basic agent: [examples/basic_agent/main.go](examples/basic_agent/main.go)
- Tool usage: [examples/tool_usage/main.go](examples/tool_usage/main.go)
- Agent tool: [examples/agent_tool/main.go](examples/agent_tool/main.go)
- Output schema: [examples/output_schema/main.go](examples/output_schema/main.go)
- Multi-agent: [examples/multi_agent/main.go](examples/multi_agent/main.go)
- Transfer between agents: [examples/transfer_agent/main.go](examples/transfer_agent/main.go)
- OpenTelemetry (tracing & metrics): [examples/opentelemetry/main.go](examples/opentelemetry/main.go)

Run an example ▶️:

```sh
go run ./examples/basic_agent/main.go
```

---

## 🧠 Core Concepts

- Agent: anything that can Run with a scoped context. See [`core.Agent`](core/agent.go).
- ModelAgent: LLM + tools + flow selection (streaming/function calling/transfer). See [`agent.NewModelAgent`](agent/model.go).
- Model: provider-agnostic interface implemented by adapters. See [`model.Model`](model/model.go).
- Runner: orchestrates invocations, streaming, persistence, lifecycle. See [runner/](runner/).
- Events: streaming outputs (partial/final) carrying content and actions. See [core/](core/).

Common event actions:
- 🔧 StateDelta: merge session state
- 🔁 TransferToAgent: request control transfer
- ⚠️ Escalate: signal escalation
- ⏭️ SkipSummarization: opt out of post-processing

---

## 🕸️ Multi-agent Patterns

- Sequential: run children in order, stop on first error
- Parallel: run children concurrently with branch isolation
- Loop: iterate a child with optional predicate/escalation

Example (sequential + parallel composition):

```go
// Create leaf model agents
a := agent.NewModelAgent("A", openai.NewModel())
b := agent.NewModelAgent("B", openai.NewModel())

// Parallel branch
par := agent.NewParallelAgent("FanOut", []core.Agent{a, b})

// Sequential pipeline
pipe := agent.NewSequentialAgent("Pipeline", []core.Agent{par /* then more children... */})

// Run with runner like in Quick Start
```

References:
- [agent/sequential.go](agent/sequential.go)
- [agent/parallel.go](agent/parallel.go)
- [agent/loop.go](agent/loop.go)
- Example: [examples/multi_agent/main.go](examples/multi_agent/main.go)

---

## 🔭 Observability

AgentMesh propagates observability via context.Context. Inject providers on the Runner, then use helpers inside your agents/tools.

- Logging (structured): pass a logging.Logger to Runner; use logging.FromContext(ctx) in your code.
- Metrics: pass a metrics.Provider; use metrics.FromContext(ctx) and record counters/histograms.
- Tracing: pass a trace.Provider; get a tracer via trace.FromContext(ctx).

Example wiring (OpenTelemetry + slog logger):

```go
logger := logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatJSON, true)
tp, mp, _ := initOTel() // see examples/opentelemetry

r := runner.New("app", agent, func(o *runner.Options) {
  o.Logger = logger
  o.Metrics = metricsotel.New(mp)
  o.Tracer = traceotel.New(tp)
})
```

Usage inside an agent/tool:

```go
func (a *MyAgent) Run(ctx context.Context, req core.RequestContext, q core.EventWriter) error {
  // Logs
  logging.FromContext(ctx).Info("start", "agent", a.Name())

  // Tracing
  tr := trace.FromContext(ctx).Tracer("agentmesh/myagent")
  ctx, span := tr.Start(ctx, "MyAgent.Run")
  defer span.End(nil)

  // Metrics
  metrics.FromContext(ctx).Counter("myagent_runs_total").Add(ctx, 1)
  // ...
  return nil
}
```

See a complete example in [examples/opentelemetry/main.go](examples/opentelemetry/main.go).

## 📦 Backpressure

Streaming: read from the results channel until it closes; handle res.Err and res.Event distinctly.
Tune Runner buffers and concurrency to match consumer speed.

---

## 🛠️ Development

Using just:

```sh
just test         # go test ./...
just test-race    # go test ./... -race
just lint         # golangci-lint
just cover        # HTML coverage
```

Or plain Go:

```sh
go test ./... -race
golangci-lint run --config .golangci.yml
```

CI runs tests, lint, and publishes releases via GoReleaser. See [.github/workflows/build.yml](.github/workflows/build.yml).

---

## 🏭 Production Considerations

- Persistence: replace in-memory stores with durable implementations
- Observability: structured logs, metrics/tracing wrappers
- Backpressure: tune buffer sizes and concurrency
- Security: sanitize tool outputs; restrict tool set
- Cost: cache responses; prune session history

---

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for philosophy, style, and PR workflow.

---

## 📄 License

MIT — see [LICENSE](LICENSE)
