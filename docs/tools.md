---
layout: doc
title: Tools
permalink: /tools/
hero:
  title: Give agents reliable capabilities
  description: Build function tools, wrap sub-agents, or ship production retrievers with consistent validation and observability.
  primary_cta:
    label: Publish a tool
    href: "#function-tools"
  secondary_cta:
    label: Tool & retrieval API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/tool"
    external: true
sidebar:
  - title: Function tools
    url: "#function-tools"
  - title: Long-running tools
    url: "#long-running-tools"
  - title: ExampleTool
    url: "#example-tool"
  - title: Toolsets
    url: "#toolsets"
    children:
      - title: MCP toolset
        url: "#mcp-toolset"
  - title: AgentTool
    url: "#agenttool"
  - title: LangChainGo tool
    url: "#langchaingo-tool"
  - title: Retrieval tools
    url: "#retrieval-tools"
    children:
      - title: Wrap retrievers
        url: "#retrieval-wrapper"
      - title: Merger retriever
        url: "#merger-retriever"
      - title: Built-in connectors
        url: "#retrieval-connectors"
  - title: Tool execution
    url: "#tool-execution"
---

Tools let models trigger deterministic side effects—API calls, computations, retrieval queries, or even other agents—while respecting schema validation and observability. Everything lives under [`tool/`](https://github.com/hupe1980/agentmesh/tree/main/tool) and works with the shared [`core.Tool`](https://pkg.go.dev/github.com/hupe1980/agentmesh/core#Tool) contract, while [`tool/retrieval`](https://github.com/hupe1980/agentmesh/tree/main/tool/retrieval) adds convenience helpers for search-style capabilities.

_Examples assume `am := github.com/hupe1980/agentmesh` and `core := github.com/hupe1980/agentmesh/core`._

---

## Function tools {#function-tools}

`tool.NewFuncTool` and `tool.NewFuncToolFromType` wrap pure Go functions with JSON Schema validation. They are perfect when you want quick utility functions or lightweight integrations.

- **When to use**: simple RPC-like operations, deterministic calculations, thin wrappers over internal services.
- **Behavior**:
  - Validates incoming JSON arguments against your schema before calling the function
  - Normalizes errors to `tool.Error` codes (`VALIDATION_ERROR`, `EXECUTION_ERROR`)
  - Runs safely in parallel; no shared mutable state

```go
type SumArgs struct {
  A float64 `json:"a"`
  B float64 `json:"b"`
}

sumTool, _ := tool.NewFuncToolFromType("calculate_sum", "Add two numbers", SumArgs{}, func(ctx context.Context, tc core.ToolContext, args SumArgs) (any, error) {
  return args.A + args.B, nil
})

planner, _ := am.NewModelAgent("planner", llm, func(o *am.ModelAgentOptions) {
  o.Tools = []core.Tool{sumTool}
})
```

Prefer handwritten schemas? `NewFuncTool` accepts a `map[string]any` schema instead of deriving it from a struct.

---

## Long-running tools {#long-running-tools}

Use `tool.NewLongRunningTool` when an operation may stream intermediate results or take minutes to complete. This tool is designed to help you start and manage tasks that happen outside the normal agent workflow and would otherwise block execution. It is a subclass of `FuncTool` that adds a prominent warning to the description and reports `IsLongRunning()` so planners can avoid spamming retries. The long-running work itself should execute in a separate service or worker—you use this tool to initiate and track it, not to run the heavy computation inline.

- **When to use**: polling APIs, human-in-the-loop steps, long compute jobs.
- **Behavior**:
  - Appends a cautionary note to the tool description (or supplies one if you omit it).
  - Lets the handler kick off the long-running operation and optionally return an initial result (for example, a job ID).
  - Pauses the agent run so the client can decide whether to continue immediately or wait for completion.
  - Allows the agent client to poll or push intermediate/final responses before the run resumes and other tasks continue.
  - Keeps the same JSON schema and handler signature as `NewFuncTool`.

```
type ApprovalArgs struct {
  Purpose string  `json:"purpose"`
  Amount  float64 `json:"amount"`
}

approval := tool.NewLongRunningTool(
  "ask_for_approval",
  "Create an approval ticket and wait for a reviewer to respond.",
  map[string]any{
    "type": "object",
    "properties": map[string]any{
      "purpose": map[string]any{"type": "string"},
      "amount":  map[string]any{"type": "number"},
    },
    "required": []string{"purpose", "amount"},
  },
  func(ctx context.Context, tc core.ToolContext, args ApprovalArgs) (any, error) {
    ticketID, reviewer := approvalService.CreateTicket(ctx, args.Purpose, args.Amount)

    return map[string]any{
      "status":     "pending",
      "approver":   reviewer,
      "purpose":    args.Purpose,
      "amount":     args.Amount,
      "ticket_id":  ticketID,
    }, nil
  },
)
```

Typical scenarios include human-in-the-loop approvals, large data exports, or ML training jobs where the agent should yield control until the external process finishes.

---

## ExampleTool {#example-tool}

`tool.NewExampleTool` injects few-shot examples into model requests right before the call is dispatched. It pulls examples from a `core.ExampleProvider`, renders them with a configurable template, and appends the result to the instructions so planners stay in sync with the latest conversational traces or curated demonstrations.

- **When to use**: you want to prime a model with dynamic, context-aware exemplars without baking them into the static prompt.
- **Behavior**:
  - Calls the provider on every request, so examples can depend on user/session state
  - Supports text and `core.FunctionCallPart` examples; unsupported parts fail fast
  - Ships sensible defaults (`<examples>` wrapper, `[user]`/`[assistant]` prefixes) but accepts custom templates, prefixes, and separators

```go
examples := []core.Example{
  {
    Input:  []core.Part{core.NewPartFromText("What is the capital of France?")},
    Output: []core.Part{core.NewPartFromText("Paris is the capital of France.")},
  },
}

provider := core.ExampleProviderFunc(func(ctx context.Context, ro core.ReadonlyContext) ([]core.Example, error) {
  return examples, nil
})

exampleTool := tool.NewExampleTool(provider, func(o *tool.ExampleToolOptions) {
  o.ExamplesIntro = "# Few-shot examples"
  o.UserPrefix = "User:"
  o.AssistantPrefix = "Assistant:"
})

writer, _ := am.NewModelAgent("writer", llm, func(o *am.ModelAgentOptions) {
  o.Tools = append(o.Tools, exampleTool)
})
```

Need richer formatting? Set `ExampleToolOptions.Template` to a Go template that receives the resolved examples (`.Examples`) and options (`.Options`). `tool.RenderExamples` is also exported so you can preview or unit-test rendering without wiring the full tool.

---

## Toolsets {#toolsets}

Registering dozens of tools up front can overwhelm the prompt. Implement `core.Toolset` to load tools on demand based on the current context, or reuse the built-in adapters (for example, MCP).

- **When to use**: dynamic connectors, per-user tool catalogs, or rate-limited APIs.
- **Behavior**:
  - Toolsets decide at call time which tools to expose via `ListTools`
  - Works alongside inline tools; the executor merges duplicates by name
  - Often paired with caching or feature flags to keep prompts trim

```go
type DocsToolset struct{}

func (DocsToolset) ListTools(ctx context.Context, ro core.ReadonlyContext) ([]core.Tool, error) {
  // e.g., only surface tools relevant to the active workspace
  return []core.Tool{sumTool, searchDocsTool}, nil
}

researcher, _ := am.NewModelAgent("researcher", llm, func(o *am.ModelAgentOptions) {
  o.Toolsets = []core.Toolset{DocsToolset{}}
})
```

---

## MCP toolset {#mcp-toolset}

The [`tool/mcp`](https://github.com/hupe1980/agentmesh/tree/main/tool/mcp) adapter lets you connect to external MCP servers and expose their declared tools to your agents. It handles session pooling, schema conversion, and remote execution over stdio or HTTP transports.

- **When to use**: integrate hosted tool providers, share capabilities with other MCP-compliant runtimes, or proxy heavy operations out of process.
- **Behavior**:
  - Discovers remote tools at runtime via `ListTools`
  - Reuses pooled sessions keyed by auth headers for efficiency
  - Supports stdio (`command`), streamable HTTP, and SSE transports out of the box

```go
import mcptool "github.com/hupe1980/agentmesh/tool/mcp"

factory := mcptool.NewStdioSessionFactory("mcp-server", []string{"serve"})
mcpToolset := mcptool.NewToolset(factory, func(o *mcptool.ToolsetOptions) {
  o.NamePrefix = "remote"
})
defer mcpToolset.Close()

planner, _ := am.NewModelAgent("planner", llm, func(o *am.ModelAgentOptions) {
  o.Toolsets = append(o.Toolsets, mcpToolset)
})
```

Need to authenticate over HTTP instead? Swap in `mcptool.NewStreamableSessionFactory` or `mcptool.NewSSESessionFactory` with custom headers. The adapter forwards `ToolContext` metadata so nested tool calls can still access artifacts and plugins.

---

## AgentTool {#agenttool}

`tool.NewAgentTool` turns an existing agent into a tool, allowing higher-level planners to delegate entire flows. It spins up a nested runner with isolated artifacts and state.

- **When to use**: hierarchical planners, reusable sub-agents, fallback escalation paths.
- **Behavior**:
  - Shares the caller’s plugin manager and artifact store via `ToolContext`
  - Streams events back into the parent run; final text becomes the tool response
  - Works with any `core.Agent` (model-based or purely functional)

```go
summarizer := am.NewSequentialAgent("summarizer", []core.Agent{writer, editor})
summarizerTool := tool.NewAgentTool(summarizer)

planner, _ := am.NewModelAgent("planner", llm, func(o *am.ModelAgentOptions) {
  o.Tools = append(o.Tools, summarizerTool)
})
```

---

## LangChainGo tool {#langchaingo-tool}

The [`tool/langchaingo`](https://github.com/hupe1980/agentmesh/tree/main/tool/langchaingo) adapter wraps any [`langchaingo`](https://github.com/tmc/langchaingo) `tools.Tool` so it can be used as an AgentMesh `core.Tool` without rewriting integrations. Try it with the built-in calculator from `github.com/tmc/langchaingo/tools`—the same one showcased in [`examples/langchaingo`](https://github.com/hupe1980/agentmesh/tree/main/examples/langchaingo).

- **When to use**: reuse existing LangChainGo tool implementations alongside native AgentMesh tools.
- **Behavior**:
  - Mirrors name and description from the wrapped tool by default (override via options)
  - Presents a single string argument (`__arg1`) that is forwarded to the LangChainGo tool
  - Surfaces validation errors using `tool.Error` for consistent error handling

```go
import (
  langchainTool "github.com/hupe1980/agentmesh/tool/langchaingo"
  lctools "github.com/tmc/langchaingo/tools"
)

calcTool := langchainTool.NewTool(&lctools.Calculator{})

planner, _ := am.NewModelAgent("planner", llm, func(o *am.ModelAgentOptions) {
  o.Tools = append(o.Tools, calcTool)
})
```

Need additional metadata or custom validation? Pass option functions to `NewTool` to override the generated name and description or wrap the result with your own schema enforcement.

---

## Retrieval tools {#retrieval-tools}

The `tool/retrieval` helpers make it easy to expose search connectors as strongly typed tools and to compose multiple retrievers together.

### Wrap retrievers as tools {#retrieval-wrapper}

`retrieval.NewTool` converts any `retrieval.Retriever` into a regular `core.Tool` that accepts a `query` string. Returned documents use the shared `retrieval.Document` shape (`PageContent`, `Score`, `Metadata`) so downstream agents receive consistent payloads.

```go
retriever := retrieval.NewMergerRetriever([]retrieval.Retriever{bedrock, kendra})

searchTool := retrieval.NewTool(
  "knowledge_base_search",
  "Search the enterprise knowledge sources and return the top documents.",
  retriever,
)

planner, _ := am.NewModelAgent("planner", llm, func(o *am.ModelAgentOptions) {
  o.Tools = append(o.Tools, searchTool)
})
```

### Merger retriever {#merger-retriever}

`retrieval.NewMergerRetriever` fans out to multiple retrievers and merges their document lists. Use option functions to tune behavior:

- `WithMergerMaxParallel(n)` bounds concurrent requests (default is `4`; pass `0` to force sequential execution).
- `WithMergerStopOnFirstError(true)` cancels remaining calls after the first failure (default is `true`); otherwise errors are aggregated via `errors.Join` and successful documents are still returned.

```go
retriever := retrieval.NewMergerRetriever(
  []retrieval.Retriever{bedrock, kendra, langchain},
  retrieval.WithMergerMaxParallel(2),
  retrieval.WithMergerStopOnFirstError(false),
)
```

Documents preserve the order of the input retriever slice, and duplicate metadata is left untouched so you can attribute results to the right source.

### Built-in connectors {#retrieval-connectors}

AgentMesh ships ready-to-use retrievers that plug straight into the wrapper above:

- `tool/retrieval/amazonbedrock` – call Amazon Bedrock Agent Runtime knowledge bases and translate their scores into `retrieval.Document` objects.
- `tool/retrieval/amazonkendra` – query Amazon Kendra indexes with optional attribute filters and user context.
- `tool/retrieval/langchaingo` – adapt any LangChainGo retriever or vector store into the AgentMesh interface.

Each package uses the same `Options` pattern (`func(*Options)`) for advanced tuning and includes unit tests demonstrating expected behavior. Mix and match them with `MergerRetriever` to build hybrid search stacks.

---

## Tool execution {#tool-execution}

Under the hood, agents rely on `tool.NewParallelToolExecutor(maxParallel)` to execute function calls. It enforces concurrency limits, records metrics, emits trace spans, and protects against panics.

- Max concurrency defaults to the batch size; configure it to bound resource usage.
- Tool runs gain a `core.ToolContext` exposing session state, artifact helpers, and plugin hooks.
- Errors are aggregated so the agent can decide whether to retry, escalate, or continue.

```go
selector := flow.NewDefaultSelector(&flow.Executors{
  AgentExecutor: agent.DefaultAgentExecutor,
  ModelExecutor: model.DefaultModelExecutor,
  ToolExecutor:  tool.NewParallelToolExecutor(4),
})

planner, _ := am.NewModelAgent("planner", llm, func(o *am.ModelAgentOptions) {
  o.FlowSelector = selector
  o.Tools = []core.Tool{sumTool, summarizerTool}
})
```

Combine these building blocks to give agents actionable capabilities without sacrificing determinism or observability.
