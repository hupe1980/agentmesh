---
title: AgentMesh Documentation
nav:
  - label: Get Started
    href: "#get-started"
  - label: Capabilities
    href: "#capabilities"
  - label: Architecture
    href: "#architecture"
  - label: Resources
    href: "#resources"
hero:
  title: Composable multi-agent orchestration for Go
  description: AgentMesh helps teams build production AI systems with streaming outputs, deterministic control, and pluggable observability.
  primary_cta:
    label: Start building
    href: "#get-started"
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
---

## Get started in minutes {#get-started}

### 1. Install

```bash
go get github.com/hupe1980/agentmesh
```

> Requires Go 1.24+ and appropriate model credentials (for example, `OPENAI_API_KEY`).

### 2. Run the quick start

```bash
export OPENAI_API_KEY="your-key"
go run ./examples/basic_agent/main.go
```

Ready-to-run examples demonstrate streaming outputs, tool usage, and multi-agent orchestration.

---

## Core capabilities {#capabilities}

- **Agent patterns**: compose sequential, parallel, and looping agents to model complex workflows.
- **Tool ecosystem**: register strongly typed tools with JSON Schema validation, cooldowns, and action propagation.
- **Streaming orchestration**: flows emit partial and final events in real time for responsive UX.
- **Observability**: integrate structured logging, metrics, and tracing via pluggable providers.
- **Stateful sessions**: manage history, artifacts, and memory through configurable stores.
- **Extensibility**: intercept model and agent lifecycles with plugins for custom behavior.

---

## Architecture at a glance {#architecture}

AgentMesh embraces a layered design so teams can evolve capabilities without rewiring everything:

1. **Core contracts** – minimal interfaces for agents, models, tools, sessions, and events ensure loose coupling.
2. **Flows** – deterministic state machines assemble model requests, process responses, and loop over tool calls.
3. **Runner** – coordinates session lifecycle, plugin hooks, observability, and streaming to consumers.
4. **Adapters** – model and tool adapters translate third-party SDKs into AgentMesh abstractions.

---

## Key resources {#resources}

- Repository: [github.com/hupe1980/agentmesh](https://github.com/hupe1980/agentmesh)
- Examples: [examples directory](https://github.com/hupe1980/agentmesh/tree/main/examples)
- Issues & roadmap: [GitHub issues](https://github.com/hupe1980/agentmesh/issues)
