---
layout: doc
title: Agents
description: Detailed guidance for Sequential, Parallel, Loop, Model, and Func agents.
permalink: /agents/
hero:
  title: Compose orchestration with reusable agents
  description: Choose the right agent for each stage—model integration, coordination, iteration, or custom logic.
  primary_cta:
    label: Create a model agent
    href: "#model-agent"
  secondary_cta:
    label: API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/agent"
    external: true
sidebar:
  - title: ModelAgent
    url: "#model-agent"
  - title: SequentialAgent
    url: "#sequential-agent"
  - title: ParallelAgent
    url: "#parallel-agent"
  - title: LoopAgent
    url: "#loop-agent"
  - title: FuncAgent
    url: "#func-agent"
  - title: Composition tips
    url: "#composition-tips"
---

AgentMesh ships a focused set of agent implementations in [`agent/`](https://github.com/hupe1980/agentmesh/tree/main/agent). They compose cleanly because each implements [`core.Agent`](https://pkg.go.dev/github.com/hupe1980/agentmesh/core#Agent) and inherits shared lifecycle behavior from `BaseAgent`.

_Examples below assume `am := github.com/hupe1980/agentmesh` and `core := github.com/hupe1980/agentmesh/core`._

---

## ModelAgent {#model-agent}

If you want to plug an LLM into a flow, start here. `ModelAgent` wraps any `core.Model` adapter, applies instructions, and optionally exposes structured tool calling.

- **When to use**: chat-style interactions, tool orchestration, schema-constrained outputs.
- **Highlights**:
  - Streaming and non-streaming responses
  - Function calling with registered tools/toolsets
  - Attach final responses into session state via `OutputKey`
  - Transfer control to peer/parent agents for escalation or delegation

```go
llm := openai.NewModel() // github.com/hupe1980/agentmesh/model/openai
planner, _ := am.NewModelAgent("planner", llm, func(o *am.ModelAgentOptions) {
  o.Instructions = core.NewInstructionsFromText("You plan tasks then call tools.")
  o.Tools = []core.Tool{todoListTool}
})
```

---

## SequentialAgent {#sequential-agent}

`SequentialAgent` runs child agents one after another, propagating the same `RequestContext` and event stream through the pipeline. It is ideal for deterministic, stage-based workflows.

- **When to use**: pipelines with clear ordering (plan → act → summarize).
- **Behavior**:
  - Stops on first error (unless children handle their own failures)
  - Shares accumulated session state across stages
  - Easy to nest—children can be `ModelAgent`, `ParallelAgent`, or other sequential pipelines

```go
pipeline := am.NewSequentialAgent("research_pipe", []core.Agent{
  planner,
  researcher,
  summarizer,
})
```

---

## ParallelAgent {#parallel-agent}

`ParallelAgent` fans out work to children concurrently. Each child runs with an isolated branch context so state mutations stay scoped, while still inheriting the parent session.

- **When to use**: gather multiple perspectives, run independent sub-tasks, or reduce latency.
- **Behavior**:
  - Optional global timeout for the entire fan-out
  - First error terminates the run and surfaces upstream
  - Branch context IDs include parent/child names for traceability

```go
workers := []core.Agent{analystA, analystB, analystC}
fanout := am.NewParallelAgent("analysis_fanout", workers, func(o *am.ParallelAgentOptions) {
  o.Timeout = 30 * time.Second
})
```

---

## LoopAgent {#loop-agent}

When you need to repeat an agent until a condition is met, wrap it with `LoopAgent`. It handles iteration limits, backoff intervals, and escalation signals out of the box.

- **When to use**: iterative refinement, polling/external checks, convergence loops.
- **Behavior**:
  - Configurable max iterations, sleep between runs, and error handling
  - Escalation events from the child bubble up immediately (`ErrEscalated`)
  - Shares session state so successive runs can build on previous context

```go
refiner := am.NewLoopAgent("writer_loop", writerAgent, func(o *am.LoopAgentOptions) {
  o.MaxIters = 5
  o.Interval = 2 * time.Second
  o.StopOnError = false
})
```

---

## FuncAgent {#func-agent}

When you already have imperative Go logic and just need to drop it into an orchestration graph, `am.NewFuncAgent` offers the lightest-weight option. Provide a `RunFunc` and AgentMesh handles lifecycle wiring, state propagation, and hierarchy.

- **When to use**: glue code, deterministic adapters, or quick experiments before promoting to a full type.
- **Behavior**:
  - Runs whatever logic you supply and can emit events through the provided `core.EventWriter`
  - Supports nested sub-agents via options, so you can still compose
  - Honors the same RequestContext/Plugin infrastructure as other agents

```go
logger := logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)

healthCheck := am.NewFuncAgent("health_check", func(ctx context.Context, req core.RequestContext, q core.EventWriter) error {
  // Write a final message into the stream
  return q.Write(ctx, core.NewTextEvent(req.RunID(), "health_check", "All systems nominal."))
}, func(o *am.FuncAgentOptions) {
  o.Description = "Runs infrastructure health probes"
})

application := am.NewApp("ops", healthCheck)
runner := am.NewRunner(application, func(o *am.RunnerOptions) { o.Logger = logger })
```

---

## Composition tips

- Mix and match agents freely—every agent implements the same `core.Agent` interface.
- Consider pairing `SequentialAgent` + `ParallelAgent` to orchestrate plan/act cycles with fan-out work stages.
- Use `LoopAgent` for convergence or polling loops, and emit escalation actions to exit early when needed.
- Leverage plugins to intercept agent lifecycle events for audit logging, caching, or analytics.
