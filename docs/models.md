---
layout: doc
title: Models
description: Connect language models and inject structured outputs, tool calls, and lifecycle hooks through a unified interface.
permalink: /models/
hero:
  title: Connect models with predictable orchestration
  description: Wrap OpenAI, LangChainGo, or custom providers while reusing AgentMesh streaming, tooling, and plugin hooks.
  primary_cta:
    label: Choose an adapter
    href: "#select-an-adapter"
  secondary_cta:
    label: Model API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/core#Model"
    external: true
sidebar:
  - title: Overview
    url: "#overview"
  - title: Select an adapter
    url: "#select-an-adapter"
    children:
      - title: OpenAI
        url: "#openai-adapter"
      - title: LangChainGo
        url: "#langchaingo-adapter"
      - title: Functional model
        url: "#functional-model"
  - title: Structured output
    url: "#structured-output"
  - title: Tools and function calling
    url: "#tools-and-function-calling"
  - title: Hook lifecycle
    url: "#hook-lifecycle"
  - title: Custom integrations
    url: "#custom-integrations"
---

## Overview {#overview}

AgentMesh treats every language or reasoning backend as a `core.Model`. That interface exposes two methods:

- `Generate(ctx, *core.ModelRequest)` – streams partial and final `core.ModelResponse` objects.
- `Capabilities()` – advertises feature flags (for example, structured output support).

Model agents, flows, and runners use those two calls exclusively. As a result, your orchestration logic stays portable—even when you swap providers or layer in custom adapters.

---

## Select an adapter {#select-an-adapter}

AgentMesh ships adapters for popular ecosystems and provides a functional helper for tests. Each adapter already implements streaming, function calling, and schema propagation so you can focus on prompts and tools.

### OpenAI {#openai-adapter}

[`model/openai`](https://github.com/hupe1980/agentmesh/tree/main/model/openai) wraps the official [`openai-go`](https://github.com/openai/openai-go) Chat Completions client. It converts AgentMesh requests into ChatCompletion payloads, streams deltas back into `core.ModelResponse`, and reconciles tool calls.

```go
import (
  am "github.com/hupe1980/agentmesh"
  "github.com/hupe1980/agentmesh/core"
  openaimodel "github.com/hupe1980/agentmesh/model/openai"
)

model := openaimodel.NewModel(func(o *openaimodel.Options) {
  o.Model = "gpt-4o-mini"
  o.Temperature = 0.2
})

planner, _ := am.NewModelAgent("planner", model, func(o *am.ModelAgentOptions) {
  o.Instructions = am.NewInstructionsFromText("Plan the task before calling tools.")
  o.Tools = []core.Tool{todoListTool}
})
```

Highlights:

- Native structured output through OpenAI's `response_format` (`Capabilities().SupportsStructuredOutput` == `true`).
- Tool call reconstruction (handles streaming deltas, attaches tool responses automatically).
- Works seamlessly with the default model executor hooks.

### LangChainGo {#langchaingo-adapter}

[`model/langchaingo`](https://github.com/hupe1980/agentmesh/tree/main/model/langchaingo) adapts any [`tmc/langchaingo`](https://github.com/tmc/langchaingo) LLM. Use it when you already rely on LangChainGo primitives or want to reuse its toolset ecosystem. Pair it with the [`tool/langchaingo` wrapper](/tools/#langchaingo-tool) to surface existing LangChainGo tools.

```go
import (
  am "github.com/hupe1980/agentmesh"
  lcg "github.com/hupe1980/agentmesh/model/langchaingo"
  lcopenaillm "github.com/tmc/langchaingo/llms/openai"
)

llm, _ := lcopenaillm.New(lcopenaillm.WithModel("gpt-4o-mini"))
model, _ := lcg.NewModel(llm)

agent, _ := am.NewModelAgent("langchain", model, nil)
```

The adapter mirrors LangChainGo's streaming model and surfaces partial/final responses as AgentMesh events. Capabilities reflect the underlying LLM—if the wrapped model cannot enforce structured output, orchestration falls back to tool-based enforcement automatically.

### Functional model {#functional-model}

Need a lightweight stub for tests or quick demos? `model.NewFuncModel` converts a plain Go function into a `core.Model`. Pass option functions to customize the advertised capabilities (they default to an empty `core.ModelCapabilities`).

```go
import (
  "context"
  "github.com/hupe1980/agentmesh/core"
  amodel "github.com/hupe1980/agentmesh/model"
)

mock := amodel.NewFuncModel(func(ctx context.Context, req *core.ModelRequest) (<-chan *core.ModelResponse, <-chan error) {
  respCh := make(chan *core.ModelResponse, 1)
  errCh := make(chan error, 1)

  go func() {
    defer close(respCh)
    defer close(errCh)
    respCh <- &core.ModelResponse{
      Parts: []core.Part{core.NewPartFromText("stubbed reply")},
      FinishReason: "stop",
    }
  }()

  return respCh, errCh
}, func(o *amodel.FuncModelOptions) {
  o.Capabilities = &core.ModelCapabilities{
    SupportsStructuredOutput: false,
  }
})

// Toggle features on the fly during tests.
mock.Capabilities().SupportsStructuredOutput = true
```

This is perfect for unit tests that exercise agents, flows, or tool plumbing without invoking an external provider.

---

## Structured output {#structured-output}

AgentMesh can ask compatible models to produce JSON that matches a schema. Create an `core.OutputSchema` and attach it via `ModelRequest.OutputSchema`—ModelAgent does this automatically when you call `agent.RequireStructuredOutput(schema)`.

```go
type Plan struct {
  Tasks []struct {
    Name        string `json:"name"`
    Description string `json:"description"`
  } `json:"tasks"`
}

schema := core.MustNewOutputSchema("plan", Plan{}, func(o *core.OutputSchemaOptions) {
  o.Description = "List concise steps to complete the task."
})

planner.RequireStructuredOutput(schema)
```

If `Capabilities().SupportsStructuredOutput` is `false`, the flow injects the internal `set_model_response` tool so the model must call a deterministic function that validates the schema before returning. Either path yields a consistent JSON payload downstream.

---

## Tools and function calling {#tools-and-function-calling}

Every model request bundles tool definitions in `ModelRequest.Tools`. Agents populate that registry automatically from registered tools and toolsets. During streaming, the adapters translate provider-specific tool call deltas into `core.FunctionCallPart` events and reconcile responses via `core.FunctionResponsePart`.

Need to expose search results as tools? Use `retrieval.NewTool` to wrap `retrieval.Retriever` implementations (including Bedrock, Kendra, or LangChainGo vector stores) so planners can fetch documents with the same function-calling flow.

Best practices:

- Keep tool names globally unique per agent run to avoid collisions.
- Add human-friendly descriptions—most providers surface them directly to the model.
- When combining toolsets and inline tools, rely on the flow executor to dedupe by name.

---

## Hook lifecycle {#hook-lifecycle}

The shared [`model.ExecuteModel`](https://pkg.go.dev/github.com/hupe1980/agentmesh/model#ExecuteModel) function drives every invocation. It respects plugin hooks from the active `RequestContext`:

1. `RunBeforeModel` – short-circuit execution with a synthetic response.
2. `Model.Generate` – stream partial chunks and the final response.
3. `RunOnModelError` – convert provider errors into fallback responses.
4. `RunAfterModel` – mutate or replace the final response before downstream flows receive it.

Use these hooks to add tracing spans, redact sensitive tokens, or collect completion metrics without changing adapter code.

---

## Custom integrations {#custom-integrations}

Implementing your own adapter is straightforward:

1. Satisfy `core.Model` by translating `core.ModelRequest` into your provider's API call.
2. Stream partial results through a channel of `*core.ModelResponse`; close the channel on completion.
3. Populate `Capabilities()` with accurate feature flags so flows can toggle structured-output fallbacks.
4. Optional: expose configuration through option functions similar to the OpenAI adapter.

With that in place, a `ModelAgent` can reuse the same instructions, tools, structured-output contracts, and plugin hooks regardless of which backend powers the generation.
