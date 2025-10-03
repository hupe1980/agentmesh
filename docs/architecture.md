---
layout: doc
title: Architecture
permalink: /architecture/
hero:
  title: Understand the runtime layers
  description: See how flows, runners, and core contracts interact to keep AgentMesh deterministic and extensible.
  primary_cta:
    label: Explore the flow engine
    href: "#core-layers"
  secondary_cta:
    label: Core package API →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/core"
    external: true
sidebar:
  - title: Core layers
    url: "#core-layers"
  - title: Flow selection
    url: "#flow-selection"
  - title: Runner lifecycle
    url: "#runner-lifecycle"
  - title: State and memory
    url: "#state-and-memory"
  - title: Extensibility hooks
    url: "#extensibility-hooks"
---

AgentMesh embraces a layered design so teams can evolve capabilities without rewiring everything. This page highlights the core runtime components and how they collaborate during a run.

---

## Core layers {#core-layers}

1. **Core contracts** – minimal interfaces for agents, models, tools, sessions, and events ensure loose coupling.
2. **Flows** – deterministic state machines assemble model requests, process responses, and loop over tool calls.
3. **Runner** – coordinates session lifecycle, plugin hooks, observability, and streaming to consumers.
4. **Adapters** – model and tool adapters translate third-party SDKs into AgentMesh abstractions.

---

## Flow selection

Flow selection determines how a `ModelAgent` fulfills a request—streaming responses, tool invocation loops, or function-call-first flows. The default selector wires together executors for agents, models, and tools:

```go
selector := flow.NewDefaultSelector(&flow.Executors{
  AgentExecutor: agent.DefaultAgentExecutor,
  ModelExecutor: model.DefaultModelExecutor,
  ToolExecutor:  tool.NewParallelToolExecutor(4),
})
```

Override the selector when you need custom batching, different tool execution semantics, or experiment-specific flows.

---

## Runner lifecycle

The runner manages application-level concerns around an agent graph:

- Establishes sessions and maintains conversational history, artifacts, and memory via pluggable stores.
- Propagates observability (logging, metrics, tracing) into every agent/tool call.
- Streams partial and final events to consumers, enabling real-time UIs.
- Hosts plugin hooks (`BeforeTool`, `AfterTool`, `OnEvent`, ...) for cross-cutting behaviors.

Construct a runner via the facade to keep configuration terse:

```go
app := am.NewRunner("support_assistant", agentGraph, func(o *am.RunnerOptions) {
  o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatJSON, true)
  o.PluginManager = myPlugins
  o.SessionStore = sessionStore
})
```

---

## State and memory

AgentMesh keeps application data explicit:

- **RequestContext** – per-run view with access to user/session IDs, plugin manager, and helpers.
- **ToolContext** – scoped context inside tool calls with artifact helpers and state snapshot APIs.
- **Memory stores** – pluggable implementations for short- or long-term recall; swap the defaults for your persistence layer.

---

## Extensibility hooks

- **Plugins** let you observe or override behaviors before/after tools, during model responses, or at agent boundaries.
- **Toolsets** enable dynamic discovery of capabilities based on runtime context.
- **Transfer actions** allow agents to hand off control to peers or parents in complex graphs.

Use these extension points to adapt AgentMesh to your domain without forked code paths.
