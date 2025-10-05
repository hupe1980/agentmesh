---
layout: doc
title: AgentMesh Documentation
hero:
  title: Composable multi-agent orchestration for Go
  description: AgentMesh helps teams build production AI systems with streaming outputs, deterministic control, and pluggable observability.
  primary_cta:
    label: Get started
    href: "/getting-started/"
  secondary_cta:
    label: View on GitHub →
    href: "https://github.com/hupe1980/agentmesh"
    external: true
  highlights:
    title: Why AgentMesh?
    items:
      - ✅ Deterministic flows (Sequential, Parallel, Loop)
      - ⚙️ Strongly typed tool integration with JSON Schema
      - 📡 Streaming-first event pipeline with partial updates
      - 🧩 Pluggable stores for session, memory, artifact data
      - 🔭 Built-in logging, tracing, and metrics hooks
sidebar:
  - title: What is AgentMesh?
    url: "#what-is-agentmesh"
  - title: Core capabilities
    url: "#core-capabilities"
  - title: Explore the documentation
    url: "#explore-the-documentation"
---

## What is AgentMesh?

AgentMesh is a Go framework for composing reliable AI agents. It gives you deterministic orchestration patterns, streaming event pipelines, and pluggable observability so you can ship production-grade AI systems with confidence.

Key ideas:

- Compose agents as sequential pipelines, parallel fan-outs, or iterative loops.
- Expose strongly typed tools (including MCP providers) to your models.
- Stream partial and final results to downstream consumers.
- Plug in your logging, metrics, tracing, and storage without forking the runtime.

---

## Core capabilities

- **Agent patterns**: compose sequential, parallel, and looping agents to model complex workflows.
- **Tool ecosystem**: register strongly typed tools with JSON Schema validation, cooldowns, and action propagation.
- **Streaming orchestration**: flows emit partial and final events in real time for responsive UX.
- **Observability**: integrate structured logging, metrics, and tracing via pluggable providers.
- **Stateful sessions**: manage history, artifacts, and memory through configurable stores.
- **Extensibility**: intercept model and agent lifecycles with plugins for custom behavior.

---

## Explore the documentation

- **[Getting started →](/getting-started/)** – Install the module, run the quick start, and browse local tooling tips.
- **[Models guide →](/models/)** – Connect OpenAI, LangChainGo, or custom providers with structured outputs and tool calls.
- **[Agents guide →](/agents/)** – Learn how Sequential, Parallel, Loop, Model, and Func agents compose orchestration graphs.
- **[Tools guide →](/tools/)** – Build function tools, the AgentTool wrapper, and MCP-backed toolsets with confidence.
- **[Plugins guide →](/plugins/)** – Hook into runner, agent, model, and tool lifecycles with reusable interceptors.
- **[Observability →](/observability/)** – Configure logging, metrics, and tracing providers and read them from context.
- **[Architecture →](/architecture/)** – Understand the flow engine, runner lifecycle, and how the core packages fit together.
- **[Resources →](/resources/)** – Jump to examples, issues, and contribution guides.

---
