---
layout: doc
title: Agents
description: Detailed guidance for Model, Sequential, Parallel, Routing, Loop, and Func agents.
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
  - title: RoutingAgent
    url: "#routing-agent"
  - title: LoopAgent
    url: "#loop-agent"
  - title: FuncAgent
    url: "#func-agent"
  - title: Flowchart helper
    url: "#flowchart-helper"
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

## RoutingAgent {#routing-agent}

`RoutingAgent` inspects the current context and chooses exactly one child agent to run. It is useful when you need a router or switch statement in the middle of an orchestration tree.

- **When to use**: branch to specialized agents, enforce guardrails before heavy work, or build hierarchical planners.
- **Behavior**:
  - Selector (`RoutingFunc`) receives the Go `context.Context`, the invocation `core.ReadonlyContext`, and available children.
  - Returns the target child's `Agent.Name()`, or an error to stop execution.
  - Emits clear errors when no children are configured or the selector picks an unknown name.

```go
router := am.NewRoutingAgent("planner_router", []core.Agent{planner, executor}, func(ctx context.Context, roCtx core.ReadonlyContext, children []core.Agent) (string, error) {
  if roCtx.SessionID() == "dry_run" {
    return "planner", nil
  }

  if roCtx.StateSnapshot()["needs_execution"] == true {
    return "executor", nil
  }

  return "planner", nil
})
```

Pair `RoutingAgent` with `SequentialAgent` or `ParallelAgent` to build deterministic branching without scattering `if` statements across your orchestration code.

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

## Flowchart helper {#flowchart-helper}

Need to visualize a complex agent tree? Call `agent.Flowchart` to render any hierarchy (including tool attachments) as a Mermaid diagram and drop it into docs or dashboards.

- **When to use**: document execution order, review nested planners, or generate diagrams for runbooks.
- **Behavior**:
  - Parents link to their first child and siblings chain together so sequential execution order is obvious
  - Tool edges render as dashed arrows (`-.->`) to distinguish them from control flow
  - Supports direction (`WithDirection`), descriptions, and optional tool nodes through familiar option functions

```go
workflow := am.NewSequentialAgent("QuarterlyReport", []core.Agent{researcher, writer, reviewer})

chart, err := agent.Flowchart(workflow,
  agent.WithDirection("LR"),
  agent.WithDescriptions(true),
  agent.WithTools(true),
)
if err != nil {
  log.Fatal(err)
}

fmt.Println(chart)
// Persist beside the example for easy sharing
_ = os.WriteFile("examples/agent_flowchart/flowchart.mmd", []byte(chart), 0o600)
```

Grab [`examples/agent_flowchart`](https://github.com/hupe1980/agentmesh/tree/main/examples/agent_flowchart) for a runnable end-to-end demo that prints and saves the diagram.

---

## Composition tips

- Mix and match agents freely—every agent implements the same `core.Agent` interface.
- Consider pairing `SequentialAgent` + `ParallelAgent` to orchestrate plan/act cycles with fan-out work stages.
- Use `LoopAgent` for convergence or polling loops, and emit escalation actions to exit early when needed.
- Leverage plugins to intercept agent lifecycle events for audit logging, caching, or analytics.
