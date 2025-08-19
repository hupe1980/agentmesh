# AgentMesh 🚀

[![Go Reference](https://pkg.go.dev/badge/github.com/hupe1980/agentmesh.svg)](https://pkg.go.dev/github.com/hupe1980/agentmesh)
[![Go Report Card](https://goreportcard.com/badge/github.com/hupe1980/agentmesh)](https://goreportcard.com/report/github.com/hupe1980/agentmesh)

> Composable, testable multi‑agent orchestration for Go - batteries included.

## ✨ Why AgentMesh?
AgentMesh gives you a lightweight, opinionated toolkit for building **LLM + tool + workflow** systems in Go without reinventing core runtime pieces. It focuses on:

- 🧩 **Composability** - nest `Sequential`, `Parallel`, and `Loop` agents into arbitrary graphs
- 🧠 **Model integration** - plug in different model backends behind a clean `model.Model` interface
- 🛠️ **Tool / function calling** - strongly typed, schema‑validated tools with execution context & state deltas
- 📡 **Streaming events** - structured event pipeline for real‑time UIs & tracing
- 🗂️ **State & memory** - session state deltas + pluggable memory & artifact stores
- 🔁 **Control flow primitives** - escalation, transfer, branching, iteration
- 🔍 **Explicit observability points** - logger hooks at every important lifecycle boundary

All defaults are in‑memory & dependency‑free → drop into a prototype in minutes. Swap stores & loggers for production.

## 🔌 Core Concepts
| Concept | Description |
|---------|-------------|
| Runner | Orchestrates invocations, streaming, persistence & lifecycle. |
| Agent | Implements `core.Agent` - anything that can `Run`. Includes model + flow coordinators. |
| ModelAgent | LLM + tools + flow selection (streaming, function calling, transfer). |
| SequentialAgent | Runs children in order, stops on first error. |
| ParallelAgent | Runs children concurrently with branch isolation. |
| LoopAgent | Repeats a child with iteration, predicate, escalation support. |
| Event | Immutable message + action container flowing through the system. |
| ToolContext | Sandboxed surface for tool execution (state, artifacts, memory). |
| RunContext | Per‑run execution scope for an agent. |

## 🚀 Quick Start
Minimal single-agent run (mirrors `examples/basic_agent`) using the low-level `runner` package directly:
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
  "github.com/hupe1980/agentmesh/logging"
  "github.com/hupe1980/agentmesh/model/openai"
  "github.com/hupe1980/agentmesh/runner"
)

func main() {
  if os.Getenv("OPENAI_API_KEY") == "" {
    log.Fatal("OPENAI_API_KEY required")
  }

  model := openai.NewModel()

  basic := agent.NewModelAgent("BasicAgent", model, func(o *agent.ModelAgentOptions) {
    o.Instruction = agent.NewInstructionFromText("You are a concise, helpful assistant.")
  })

  r := runner.New(basic, func(o *runner.Options) {
    o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, "text", false)
  })

  content := core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: "Hello! What can you do?"}}}

  ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
  defer cancel()

  _, eventsCh, errsCh, err := r.Run(ctx, "session-1", content)
  if err != nil { log.Fatalf("run failed: %v", err) }

  for eventsCh != nil || errsCh != nil {
    select {
    case ev, ok := <-eventsCh:
      if !ok { eventsCh = nil; continue }
      if ev.Author == basic.Name() && ev.Content != nil {
        for _, part := range ev.Content.Parts {
          if tp, ok := part.(core.TextPart); ok { fmt.Println("Agent:", tp.Text) }
        }
      }
    case err, ok := <-errsCh:
      if !ok { errsCh = nil; continue }
      if err != nil { log.Printf("error: %v", err) }
    }
  }
}
```

## 🛠️ Adding Tools
```go
sumTool := tool.NewFunctionTool(
  "sum_numbers",
  "Add two numbers",
  map[string]any{
    "type": "object",
    "properties": map[string]any{
      "a": map[string]any{"type": "number"},
      "b": map[string]any{"type": "number"},
    },
    "required": []string{"a","b"},
  },
  func(toolCtx *core.ToolContext, args map[string]any) (any, error) {
    a := args["a"].(float64)
    b := args["b"].(float64)
    return a + b, nil
  },
)

assistant.RegisterTool(sumTool)
```
The LLM can now trigger function calls (depending on backend capabilities) and the runner will execute them with a `ToolContext`.

## 🔄 Control Flow Patterns
| Pattern | Use Case |
|---------|----------|
| Sequential | Pipelines with strict ordering |
| Parallel | Fan‑out aggregation / concurrent enrichment |
| Loop | Iterative refinement, polling, retries |

Agents can be nested arbitrarily: a SequentialAgent may contain ParallelAgents, which contain ModelAgents, etc.

## 🧬 Event Actions
Events carry `Actions` to request orchestration side‑effects:
- `StateDelta` - merge session state
- `ArtifactDelta` - (future) artifact lifecycle hooks
- `TransferToAgent` - request control transfer
- `Escalate` - signal escalation to a higher‑order agent/human
- `SkipSummarization` - bypass post‑processing summarizers

## 🔧 Extending
- Implement `core.Agent` to add new coordination behavior
- Implement `model.Model` to support a new LLM backend
- Implement `tool.Tool` for new callable functions
- Swap stores with custom persistence layers (SQL, S3, Vector DB, etc.)

## 📦 Production Considerations
| Concern | Notes |
|---------|-------|
| Persistence | Replace in‑memory stores with durable implementations |
| Observability | Provide structured logger; add metrics/tracing wrappers |
| Backpressure | Tune `MaxConcurrentInvocations` & buffer sizes |
| Security | Sanitize tool outputs; control allowed tool set |
| Cost | Cache model responses; prune session history |

## 🗺️ Roadmap (Indicative)
- 🔐 Auth propagation to tools
- 🧵 Conversation summarization middleware
- 🧩 Plugin registry / discovery
- 📊 Metrics hooks (OpenTelemetry)
- 🗄️ Pluggable vector memory examples

## 🤝 Contributing
Contributions welcome! Please see `CONTRIBUTING.md`.
1. Fork & branch
2. Add tests for changes
3. Run `go vet` / linters
4. Open PR with context & screenshots/logs where helpful

## 📜 License
Licensed under the MIT License - see [LICENSE.md](./LICENSE.md) for full text.
