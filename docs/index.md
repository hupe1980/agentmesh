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
  - title: Quick examples
    url: "#quick-examples"
  - title: Explore the documentation
    url: "#explore-the-documentation"
---

## What is AgentMesh?

AgentMesh is a Go framework for building sophisticated multi-agent AI systems powered by Pregel-style bulk-synchronous parallel (BSP) graph processing. It provides production-grade orchestration for LLM-powered workflows with deterministic execution and enterprise observability.

<div class="feature-grid">
  <div class="feature-card">
    <h3><span class="feature-icon">🔄</span> Graph-native Architecture</h3>
    <p>Build agents as directed graphs where nodes execute in parallel supersteps with deterministic ordering.</p>
  </div>
  <div class="feature-card">
    <h3><span class="feature-icon">⚙️</span> Strongly Typed Tools</h3>
    <p>Register functions as tools with automatic JSON Schema generation and compile-time type safety.</p>
  </div>
  <div class="feature-card">
    <h3><span class="feature-icon">📡</span> Streaming Execution</h3>
    <p>Monitor graph execution in real-time with event streams for responsive user experiences.</p>
  </div>
  <div class="feature-card">
    <h3><span class="feature-icon">🔒</span> Enterprise Ready</h3>
    <p>Built-in checkpointing, time-travel debugging, OpenTelemetry integration, and production reliability.</p>
  </div>
</div>

---

## Core capabilities

<div class="feature-grid">
  <div class="feature-card">
    <h3><span class="feature-icon">⚡</span> Pregel Graph Execution</h3>
    <p>Bulk-synchronous parallel processing enables efficient multi-agent coordination with deterministic superstep ordering.</p>
  </div>
  <div class="feature-card">
    <h3><span class="feature-icon">🤖</span> LLM Integration</h3>
    <p>First-class support for OpenAI, Anthropic, Amazon Bedrock, Google Gemini, and custom model providers.</p>
  </div>
  <div class="feature-card">
    <h3><span class="feature-icon">🛠️</span> Tool Orchestration</h3>
    <p>Type-safe function calling with automatic JSON schema generation, parallel execution, and robust error handling.</p>
  </div>
  <div class="feature-card">
    <h3><span class="feature-icon">💾</span> State Management</h3>
    <p>Versioned state store with channel-based updates, automatic checkpointing, and time-travel debugging.</p>
  </div>
  <div class="feature-card">
    <h3><span class="feature-icon">🔀</span> Conditional Routing</h3>
    <p>Dynamic flow control based on agent outputs, enabling complex decision trees and multi-agent collaboration.</p>
  </div>
  <div class="feature-card">
    <h3><span class="feature-icon">🔭</span> Observability</h3>
    <p>Built-in OpenTelemetry metrics, distributed tracing, and structured logging for production monitoring.</p>
  </div>
</div>

---

## Quick examples

Explore hands-on examples to see AgentMesh in action:

<div class="example-grid">
  <a href="https://github.com/hupe1980/agentmesh/tree/main/examples/basic_agent" target="_blank" rel="noopener" class="example-card">
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
    <span>Basic Agent</span>
  </a>
  <a href="https://github.com/hupe1980/agentmesh/tree/main/examples/streaming" target="_blank" rel="noopener" class="example-card">
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
    <span>Streaming Responses</span>
  </a>
  <a href="https://github.com/hupe1980/agentmesh/tree/main/examples/checkpointing" target="_blank" rel="noopener" class="example-card">
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
    <span>Checkpointing</span>
  </a>
  <a href="https://github.com/hupe1980/agentmesh/tree/main/examples/supervisor_agent" target="_blank" rel="noopener" class="example-card">
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
    <span>Supervisor Agent</span>
  </a>
  <a href="https://github.com/hupe1980/agentmesh/tree/main/examples/guardrails" target="_blank" rel="noopener" class="example-card">
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
    <span>Guardrails</span>
  </a>
  <a href="https://github.com/hupe1980/agentmesh/tree/main/examples/human_approval" target="_blank" rel="noopener" class="example-card">
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
    <span>Human Approval</span>
  </a>
</div>

<div class="related-links">
  <a href="https://github.com/hupe1980/agentmesh/tree/main/examples" target="_blank" rel="noopener" class="related-link">
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>
    Browse all 35 examples →
  </a>
</div>

---

## Explore the documentation

<div class="feature-grid">
  <div class="feature-card">
    <h3>🚀 <a href="/getting-started/">Getting Started</a></h3>
    <p>Install the module, run your first agent, and explore example workflows.</p>
  </div>
  <div class="feature-card">
    <h3>🏗️ <a href="/architecture/">Architecture</a></h3>
    <p>Understand the Pregel BSP model, graph builder pattern, and state management.</p>
  </div>
  <div class="feature-card">
    <h3>🤖 <a href="/agents/">Agents Guide</a></h3>
    <p>Build ReAct agents, RAG agents, and custom graph-based workflows.</p>
  </div>
  <div class="feature-card">
    <h3>🛠️ <a href="/tools/">Tools Guide</a></h3>
    <p>Create function tools with automatic schema generation and integrate external capabilities.</p>
  </div>
  <div class="feature-card">
    <h3>🧠 <a href="/models/">Models Guide</a></h3>
    <p>Connect OpenAI, Anthropic, Bedrock, Gemini, or custom LLM providers with routing.</p>
  </div>
  <div class="feature-card">
    <h3>🔭 <a href="/observability/">Observability</a></h3>
    <p>Configure OpenTelemetry metrics, tracing, and structured logging.</p>
  </div>
</div>

---
