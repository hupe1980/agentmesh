---
layout: doc
title: AgentMesh Documentation
hero:
  title: Production-grade multi-agent orchestration for Go
  description: AgentMesh leverages Pregel-style bulk-synchronous parallel graph processing to build sophisticated AI agent workflows with deterministic execution and enterprise observability.
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
      - 🔄 Pregel-based BSP engine for parallel graph execution
      - ⚙️ Strongly typed tool integration with JSON Schema
      - 📡 Streaming-first event pipeline with real-time updates
      - 💾 Versioned state store with checkpointing and time-travel
      - 🔭 Built-in OpenTelemetry metrics and distributed tracing
sidebar:
  - title: What is AgentMesh?
    url: "#what-is-agentmesh"
  - title: Core capabilities
    url: "#core-capabilities"
  - title: Explore the documentation
    url: "#explore-the-documentation"
---

## What is AgentMesh?

AgentMesh is a Go framework for building sophisticated multi-agent AI systems powered by Pregel-style bulk-synchronous parallel (BSP) graph processing. It provides production-grade orchestration for LLM-powered workflows with deterministic execution and enterprise observability.

Key ideas:

- **Graph-native architecture**: Build agents as directed graphs where nodes execute in parallel supersteps with deterministic ordering.
- **Strongly typed tools**: Register functions as tools with automatic JSON Schema generation and validation.
- **Streaming execution**: Monitor graph execution in real-time with event streams for responsive UX.
- **Enterprise-ready**: Built-in checkpointing, time-travel debugging, OpenTelemetry integration, and production-tested reliability.

---

## Core capabilities

- **Pregel graph execution**: Bulk-synchronous parallel processing enables efficient multi-agent coordination with deterministic superstep ordering.
- **LLM integration**: First-class support for OpenAI, Anthropic (via AWS Bedrock), and extensible model interfaces with streaming and tool calling.
- **Tool orchestration**: Type-safe function calling with automatic JSON schema generation, parallel execution, and robust error handling.
- **State management**: Versioned state store with channel-based updates, automatic checkpointing, and time-travel debugging capabilities.
- **Conditional routing**: Dynamic flow control based on agent outputs, enabling complex decision trees and multi-agent collaboration.
- **Observability**: Built-in OpenTelemetry metrics, distributed tracing, and structured logging for production monitoring.

---

## Explore the documentation

- **[Getting started →](/getting-started/)** – Install the module, run your first agent, and explore example workflows.
- **[Architecture →](/architecture/)** – Understand the Pregel BSP model, graph builder pattern, and state management.
- **[Agents guide →](/agents/)** – Build ReAct agents, RAG agents, and custom graph-based workflows.
- **[Tools guide →](/tools/)** – Create function tools with automatic schema generation and integrate external capabilities.
- **[Models guide →](/models/)** – Connect OpenAI, Anthropic, LangChainGo, or custom LLM providers.
- **[Observability →](/observability/)** – Configure OpenTelemetry metrics, tracing, and structured logging.
- **[Resources →](/resources/)** – Explore examples, best practices, and contribution guidelines.

---
