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
| Engine | Orchestrates invocations, streaming, persistence & lifecycle. |
| Agent | Implements `core.Agent` - anything that can `Run`. Includes model + flow coordinators. |
| ModelAgent | LLM + tools + flow selection (streaming, function calling, transfer). |
| SequentialAgent | Runs children in order, stops on first error. |
| ParallelAgent | Runs children concurrently with branch isolation. |
| LoopAgent | Repeats a child with iteration, predicate, escalation support. |
| Event | Immutable message + action container flowing through the system. |
| ToolContext | Sandboxed surface for tool execution (state, artifacts, memory). |
| InvocationContext | Per‑run execution scope for an agent. |

## 🚀 Quick Start
Minimal single-agent run (mirrors `examples/basic_agent`):
```go
package main

import (
  "context"
  "fmt"
  "log"
  "os"

  "github.com/hupe1980/agentmesh"
  "github.com/hupe1980/agentmesh/agent"
  "github.com/hupe1980/agentmesh/core"
  "github.com/hupe1980/agentmesh/logging"
  "github.com/hupe1980/agentmesh/model/openai"
)

func main() {
  if os.Getenv("OPENAI_API_KEY") == "" {
    log.Fatal("OPENAI_API_KEY required")
  }

  mesh := agentmesh.New()

  model := openai.NewModel()
  
  agent := agent.NewModelAgent("BasicAgent", model, func(o *agent.ModelAgentOptions) {
    o.Instruction = agent.NewInstructionFromText("You are a concise, helpful assistant.")
  })

  mesh.RegisterAgent(agent)

  userContent := core.Content{
    Role: "user", 
    Parts: []core.Part{core.TextPart{Text: "Hello! What can you do?"}},
  }

  _, eventsCh, errsCh, err := mesh.Invoke(context.Background(), "session-1", agent.Name(), userContent)
  if err != nil { 
    log.Fatalf("invoke failed: %v", err)
  }

  for {
    select {
    case ev, ok := <-eventsCh:
      if !ok { return }
      if ev.Author == agent.Name() && ev.Content != nil {
        for _, part := range ev.Content.Parts {
          if tp, ok := part.(core.TextPart); ok { fmt.Println("Agent:", tp.Text) }
        }
      }
    case err, ok := <-errsCh:
      if ok && err != nil { log.Fatalf("agent error: %v", err) }
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
The LLM can now trigger function calls (depending on backend capabilities) and the engine will execute them with a `ToolContext`.

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

## 🧪 Testing Philosophy
- Deterministic: Supply mock models & stores
- Isolated: Use in‑memory implementations
- Observable: Inspect streamed `Event`s

Example (pseudo):
```go
events := runAndCollect(t, engine, sessionID, agentName, content)
assert.Contains(t, last(events).Content.Text(), "expected")
```

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
Licensed under the MIT License – see [LICENSE.md](./LICENSE.md) for full text.

---
Made with Go 🦫 - Build something awesome.
