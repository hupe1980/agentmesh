---
layout: doc
title: Tools
permalink: /tools/
example: wasm_tool
hero:
  title: Give agents powerful capabilities
  description: Create function tools with automatic JSON schema generation, integrate external APIs, and build retrieval systems.
  primary_cta:
    label: Create a tool
    href: "#function-tools"
  secondary_cta:
    label: Tool API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/tool"
    external: true
sidebar:
  - title: Function tools
    url: "#function-tools"
  - title: Tool interface
    url: "#tool-interface"
  - title: Instruction providers
    url: "#instruction-providers"
  - title: Parameter types
    url: "#parameter-types"
  - title: Error handling
    url: "#error-handling"
  - title: Best practices
    url: "#best-practices"
  - title: WASM tools
    url: "#wasm-tools"
  - title: LangChainGo integration
    url: "#langchaingo-tool"
  - title: Retrieval tools
    url: "#retrieval-tools"
---

Tools enable agents to perform actions, fetch data, and interact with external systems. AgentMesh provides automatic JSON schema generation from Go types, making tool creation straightforward and type-safe.

---

## Function tools {#function-tools}

`tool.NewFuncTool` wraps a Go function with automatic JSON schema generation:

```go
import (
    "context"
    "github.com/hupe1980/agentmesh/pkg/tool"
)

type WeatherArgs struct {
    Location string `json:"location" description:"City name or zip code"`
    Units    string `json:"units,omitempty" description:"Temperature units: celsius or fahrenheit"`
}

weatherTool, err := tool.NewFuncTool(
    "get_weather",
    "Get current weather for a location",
    func(ctx context.Context, args WeatherArgs) (map[string]any, error) {
        // Fetch weather data...
        return map[string]any{
            "temperature": 72,
            "conditions":  "Sunny",
            "location":    args.Location,
            "units":       args.Units,
        }, nil
    },
)
```

### Generic return types

Tools can return any type:

```go
type SearchResult struct {
    Title   string `json:"title"`
    Content string `json:"content"`
    Score   float64 `json:"score"`
}

searchTool, _ := tool.NewFuncTool(
    "search",
    "Search the knowledge base",
    func(ctx context.Context, args struct {
        Query string `json:"query"`
        Limit int    `json:"limit,omitempty"`
    }) ([]SearchResult, error) {
        // Perform search...
        return results, nil
    },
)
```

### Simple functions

For simple tools without complex parameters:

```go
timeTool, _ := tool.NewFuncTool(
    "get_time",
    "Get current time",
    func(ctx context.Context, _ struct{}) (string, error) {
        return time.Now().Format(time.RFC3339), nil
    },
)
```

---

## Tool execution {#tool-execution}

### Executor pattern

AgentMesh uses an executor pattern to separate tool execution from graph orchestration:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/tool"
)

// Create tool registry
registry := map[string]tool.Tool{
    "search": searchTool,
    "calculate": calcTool,
}

// Default executor (sequential, one at a time)
executor := tool.NewExecutor(registry,
    tool.WithContinueOnError(false),
    tool.WithErrorPrefix("execution failed"))

// Parallel execution (concurrent)
executor := tool.NewParallelExecutor(registry,
    tool.WithContinueOnError(true),
    tool.WithMaxConcurrency(5))

// Execute tools directly (without graph)
calls := []tool.Call{
    {
        ID:        "call_1",
        Name:      "search",
        Arguments: `{"query":"AgentMesh","limit":10}`,
    },
}

results, err := executor.Execute(ctx, calls)
for _, result := range results {
    fmt.Printf("Tool: %s, Duration: %v\n", result.ToolName, result.Duration)
    if result.Error != nil {
        log.Printf("Error: %v", result.Error)
    } else {
        fmt.Printf("Result: %v\n", result.Result)
    }
}
```

### Arguments as JSON strings

Tool arguments are passed as **JSON strings** (not maps) to eliminate wasteful marshal/unmarshal cycles:

```
LLM generates: {"query": "weather", "location": "Berlin"}
    ↓
ToolCall.Arguments: "{\"query\": \"weather\", \"location\": \"Berlin\"}"  (string)
    ↓
tool.Call.Arguments: "{\"query\": \"weather\", \"location\": \"Berlin\"}"  (string)
    ↓
Tool receives: "{\"query\": \"weather\", \"location\": \"Berlin\"}"  (string)
    ↓
Tool unmarshals to struct
```

This design avoids the inefficient pattern:
1. ❌ Unmarshal JSON string to map (from LLM)
2. ❌ Marshal map back to JSON string (to tool)
3. ❌ Unmarshal JSON string to struct (in tool)

Instead, arguments stay as JSON strings throughout the pipeline until the tool unmarshals them once.

**Example:**
```go
// In your code (creating tool calls)
call := tool.Call{
    ID:        "call_abc",
    Name:      "get_weather",
    Arguments: `{"location":"Berlin","unit":"celsius"}`,  // JSON string
}

// Tool receives the JSON string and unmarshals once
func (t *WeatherTool) Run(ctx context.Context, input string) (any, error) {
    var args WeatherArgs
    if err := json.Unmarshal([]byte(input), &args); err != nil {
        return nil, err
    }
    // Use args...
}
```

### Custom executors

Create custom executors for specialized behavior:

```go
// Cached executor for deterministic tools
type CachedExecutor struct {
    wrapped tool.Executor
    cache   map[string]tool.ExecutionResult
    mu      sync.RWMutex
}

func (e *CachedExecutor) Execute(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
    // Check cache first
    // Execute uncached calls
    // Store results
    return results, nil
}

// Use custom executor
cachedExec := &CachedExecutor{
    wrapped: tool.NewSequentialExecutor(registry),
    cache:   make(map[string]tool.ExecutionResult),
}
```

---

## Tool interface {#tool-interface}

For more control, implement the `tool.Tool` interface:

```go
type Tool interface {
    Name() string
    Description() string
    Definition() *Definition
    Call(ctx context.Context, args string) (any, error)
}
```

Example custom tool:

```go
type DatabaseTool struct {
    db *sql.DB
}

func (t *DatabaseTool) Name() string {
    return "query_database"
}

func (t *DatabaseTool) Description() string {
    return "Execute SQL queries against the database"
}

func (t *DatabaseTool) Definition() *tool.Definition {
    return &tool.Definition{
        Type: "function",
        Function: tool.FunctionDefinition{
            Name:        t.Name(),
            Description: t.Description(),
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "query": map[string]any{
                        "type":        "string",
                        "description": "SQL query to execute",
                    },
                },
                "required": []string{"query"},
            },
        },
    }
}

func (t *DatabaseTool) Call(ctx context.Context, input string) (any, error) {
    var params struct {
        Query string `json:"query"`
    }
    if err := json.Unmarshal([]byte(input), &params); err != nil {
        return nil, err
    }
    
    rows, err := t.db.QueryContext(ctx, params.Query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    // Process results...
    return results, nil
}
```

---

## Instruction providers {#instruction-providers}

Tools can contribute additional instructions to the model's system prompt by implementing the optional `InstructionProvider` interface:

```go
// InstructionProvider is an optional interface for tools that need to
// extend the model's system prompt with usage instructions.
type InstructionProvider interface {
    Instruction() string
}
```

This is useful when a tool requires the model to follow specific usage patterns. For example, the built-in `SetModelResponseTool` (used when models don't support native structured output) implements this interface to instruct the model to always call the tool for final responses.

### Example: Custom tool with instructions

```go
type SearchTool struct {
    apiKey string
}

func (t *SearchTool) Name() string { return "web_search" }

func (t *SearchTool) Description() string {
    return "Search the web for information"
}

func (t *SearchTool) Definition() *tool.Definition {
    return &tool.Definition{
        Type: "function",
        Function: tool.FunctionDefinition{
            Name:        t.Name(),
            Description: t.Description(),
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "query": map[string]any{
                        "type":        "string",
                        "description": "Search query",
                    },
                },
                "required": []string{"query"},
            },
        },
    }
}

func (t *SearchTool) Call(ctx context.Context, args string) (any, error) {
    // Implementation...
    return results, nil
}

// Instruction provides usage guidelines for the model
func (t *SearchTool) Instruction() string {
    return `When using web_search:
- Use specific, targeted queries rather than broad searches
- Limit searches to 3 per response to avoid rate limits
- Prefer authoritative sources in your query formulation`
}
```

### How instructions are merged

When an agent is configured with tools, the model node automatically:

1. Collects instructions from all tools implementing `InstructionProvider`
2. Joins them with double newlines for readability
3. Appends the combined instructions to the base system prompt

```go
// Example: Instructions are automatically merged
agent, _ := agent.New(model,
    agent.WithInstructions("You are a helpful assistant."),
    agent.WithTools(searchTool, dataTool),
)
// If searchTool.Instruction() returns "Search guidelines..."
// and dataTool.Instruction() returns "Data handling rules..."
// The model receives:
// "You are a helpful assistant.
//
// Search guidelines...
//
// Data handling rules..."
```

### Collecting instructions manually

If you need to collect tool instructions outside of the agent framework:

```go
tools := []tool.Tool{searchTool, dataTool, regularTool}
instructions := tool.CollectInstructions(tools)
// Returns combined instructions from tools implementing InstructionProvider
// Tools without Instruction() method are silently skipped
```
```

---

## Parameter types {#parameter-types}

Function tools support various parameter types:

### Primitives

```go
tool.NewFuncTool("example", "description",
    func(ctx context.Context, args struct {
        Text    string  `json:"text"`
        Count   int     `json:"count"`
        Score   float64 `json:"score"`
        Enabled bool    `json:"enabled"`
    }) (any, error) {
        // ...
    },
)
```

### Nested structs

```go
type Address struct {
    Street string `json:"street"`
    City   string `json:"city"`
    Zip    string `json:"zip"`
}

type ContactArgs struct {
    Name    string  `json:"name"`
    Address Address `json:"address"`
}

tool.NewFuncTool("update_contact", "Update contact information",
    func(ctx context.Context, args ContactArgs) (any, error) {
        // ...
    },
)
```

### Arrays and slices

```go
tool.NewFuncTool("batch_process", "Process multiple items",
    func(ctx context.Context, args struct {
        Items []string `json:"items"`
        Tags  []string `json:"tags,omitempty"`
    }) (any, error) {
        // ...
    },
)
```

### Optional fields

Use `omitempty` for optional parameters:

```go
tool.NewFuncTool("search", "Search with optional filters",
    func(ctx context.Context, args struct {
        Query   string   `json:"query"`
        MaxResults int   `json:"max_results,omitempty"`
        Categories []string `json:"categories,omitempty"`
    }) (any, error) {
        // ...
    },
)
```

---

## Error handling {#error-handling}

Tool errors are returned to the agent as tool results:

```go
tool.NewFuncTool("divide", "Divide two numbers",
    func(ctx context.Context, args struct {
        A float64 `json:"a"`
        B float64 `json:"b"`
    }) (float64, error) {
        if args.B == 0 {
            return 0, fmt.Errorf("division by zero")
        }
        return args.A / args.B, nil
    },
)
```

The agent receives:

```json
{
    "role": "tool",
    "name": "divide",
    "content": "error: division by zero"
}
```

And can reason about the error and try alternative approaches.

### Context cancellation

Tools should respect context cancellation:

```go
tool.NewFuncTool("long_operation", "Perform a long operation",
    func(ctx context.Context, args Args) (any, error) {
        for i := 0; i < 100; i++ {
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            default:
                // Do work...
            }
        }
        return result, nil
    },
)
```

---

## Best practices {#best-practices}

### Keep tools focused

Each tool should have a single, clear purpose:

```go
// Good: Focused tool
searchTool, _ := tool.NewFuncTool("search", "Search the knowledge base", searchFunc)
filterTool, _ := tool.NewFuncTool("filter", "Filter search results", filterFunc)

// Avoid: Tools that do too much
multiTool, _ := tool.NewFuncTool("search_and_filter_and_rank", "...", complexFunc)
```

### Use descriptive names and descriptions

Help the model understand when to use each tool:

```go
tool.NewFuncTool(
    "calculate_mortgage_payment",
    "Calculate monthly mortgage payment given principal, interest rate, and term in years. Returns payment amount in dollars.",
    mortgageFunc,
)
```

### Provide detailed schema descriptions

Use struct tags to document parameters:

```go
type AnalyzeArgs struct {
    Text   string `json:"text" description:"The text to analyze for sentiment and key entities"`
    Lang   string `json:"language,omitempty" description:"ISO 639-1 language code (e.g., 'en', 'es'). Defaults to auto-detect."`
    Depth  string `json:"depth,omitempty" description:"Analysis depth: 'quick' or 'thorough'. Default is 'quick'."`
}
```

### Return structured data

Return structured data that agents can reason about:

```go
// Good: Structured response
return map[string]any{
    "success": true,
    "user_id": 12345,
    "email":   "user@example.com",
}, nil

// Avoid: Unstructured text
return "Successfully created user 12345 with email user@example.com", nil
```

### Handle edge cases gracefully

```go
func(ctx context.Context, args SearchArgs) ([]Result, error) {
    if args.Query == "" {
        return nil, fmt.Errorf("query cannot be empty")
    }
    
    results, err := search(ctx, args.Query)
    if err != nil {
        return nil, fmt.Errorf("search failed: %w", err)
    }
    
    if len(results) == 0 {
        // Return empty slice, not error
        return []Result{}, nil
    }
    
    return results, nil
}
```

### Use timeouts for external calls

```go
func(ctx context.Context, args APIArgs) (any, error) {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    
    resp, err := httpClient.Get(ctx, args.URL)
    if err != nil {
        return nil, fmt.Errorf("API call failed: %w", err)
    }
    
    return resp, nil
}
```
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

Registering dozens of tools up front can overwhelm the prompt. Implement `tool.Toolset` to load tools on demand based on the current context, or reuse the built-in adapters (for example, MCP).

- **When to use**: dynamic connectors, per-user tool catalogs, or rate-limited APIs.
- **Behavior**:
  - Toolsets decide at call time which tools to expose via `ListTools(ctx, view)`
  - The `view` parameter provides read access to the current graph state for context-aware tool selection
  - Works alongside inline tools; use `WithTools()` for static tools and `WithToolset()` for dynamic discovery
  - Often paired with caching or feature flags to keep prompts trim

```go
import (
    "context"
    
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/tool"
)

// Custom toolset that filters tools based on user permissions in state
type PermissionAwareToolset struct {
    inner      tool.Toolset
    permKey    graph.Key[[]string]
}

func (t *PermissionAwareToolset) ListTools(ctx context.Context, view graph.View) ([]tool.Tool, error) {
    allTools, err := t.inner.ListTools(ctx, view)
    if err != nil {
        return nil, err
    }
    
    // If no view, return all tools (static discovery)
    if view == nil {
        return allTools, nil
    }
    
    // Filter tools based on user permissions from state
    permissions := graph.Get(view, t.permKey)
    return filterByPermissions(allTools, permissions), nil
}

func (t *PermissionAwareToolset) Close() error {
    return t.inner.Close()
}

// Use with the agent
reactAgent, _ := agent.NewReAct(model,
    agent.WithToolset(&PermissionAwareToolset{
        inner:   mcpToolset,
        permKey: UserPermissionsKey,
    }),
)
```

### Combining Toolsets

Use `tool.Combine()` to merge multiple toolsets:

```go
// Combine static tools with dynamic MCP tools
combined := tool.Combine(
    tool.NewStaticToolset(calculatorTool, weatherTool),
    mcpToolset,
)

reactAgent, _ := agent.NewReAct(model, agent.WithToolset(combined))
```

---

## MCP toolset {#mcp-toolset}

The [`tool/mcp`](https://github.com/hupe1980/agentmesh/tree/main/pkg/tool/mcp) adapter lets you connect to external MCP servers and expose their declared tools to your agents. It handles session pooling, schema conversion, and remote execution over stdio or HTTP transports.

- **When to use**: integrate hosted tool providers, share capabilities with other MCP-compliant runtimes, or proxy heavy operations out of process.
- **Behavior**:
  - Discovers remote tools at runtime via `ListTools(ctx, view)`
  - Reuses pooled sessions keyed by auth headers for efficiency
  - Supports stdio (`command`), streamable HTTP, and SSE transports out of the box

```go
import (
    "github.com/hupe1980/agentmesh/pkg/agent"
    mcptool "github.com/hupe1980/agentmesh/pkg/tool/mcp"
)

factory := mcptool.NewStdioSessionFactory("mcp-server", []string{"serve"})
mcpToolset := mcptool.NewToolset(factory, mcptool.WithNamePrefix("remote"))
defer mcpToolset.Close()

// Use WithToolset for dynamic tool discovery - no manual ListTools() call needed!
reactAgent, _ := agent.NewReAct(model, agent.WithToolset(mcpToolset))
```

Need to authenticate over HTTP instead? Swap in `mcptool.NewStreamableSessionFactory` or `mcptool.NewSSESessionFactory` with custom headers. The adapter forwards `ToolContext` metadata so nested tool calls can still access artifacts.

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

The [`tool/langchaingo`](https://github.com/hupe1980/agentmesh/tree/main/pkg/tool/langchaingo) adapter wraps any [`langchaingo`](https://github.com/tmc/langchaingo) `tools.Tool` so it can be used as an AgentMesh `tool.Tool` without rewriting integrations. Try it with the built-in calculator from `github.com/tmc/langchaingo/tools`.

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

## WASM Tools {#wasm-tools}

AgentMesh provides **WebAssembly-based tool sandboxing** for securely executing untrusted or third-party code.

WASM tools run inside a lightweight, memory-safe sandbox enforced by the WebAssembly runtime. Each tool operates in its own isolated environment with strict resource limits and no access to the host system unless explicitly granted through controlled interfaces (e.g., WASI capabilities).

When combined with containerization or process isolation, this approach achieves defense-in-depth comparable to kernel-level isolation—but with the speed and portability of WebAssembly.

### Why WASM sandboxing?

Traditional tool sandboxing approaches have critical limitations:

- **User-space restrictions** can be bypassed by malicious code creating its own HTTP clients or file handles
- **Docker containers** add deployment complexity and significant resource overhead
- **Process isolation** requires OS-specific implementations and careful privilege management

WASM provides **runtime-enforced security** through the WebAssembly sandbox:

- ✅ **Isolated memory** - No access to host memory or pointers
- ✅ **No syscalls by default** - Network, filesystem, and system calls are blocked unless explicitly enabled via WASI
- ✅ **Cannot be bypassed** - Security is enforced by the WASM runtime, not the guest code
- ✅ **Cross-platform** - Same security guarantees on Linux, macOS, and Windows
- ✅ **Minimal overhead** - 1-5ms startup time per invocation
- ✅ **Resource limits** - Configurable memory, timeout, and compute constraints

### Quick start

Create a WASM tool from a compiled `.wasm` file:

```go
import (
    "context"
    "github.com/hupe1980/agentmesh/pkg/tool/wasm"
)

// Load WASM module
wasmBytes, err := os.ReadFile("calculator.wasm")
if err != nil {
    log.Fatal(err)
}

// Create tool with compute-only policy (no network/filesystem)
tool, err := wasm.NewWASMTool(
    "calculator",
    "Evaluate mathematical expressions",
    wasmBytes,
    wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
)
if err != nil {
    log.Fatal(err)
}

// Use in agent
agent, _ := agent.NewReAct(model, []tool.Tool{tool})
```

### Security policies

WASM tools enforce security through **sandbox policies** that define what capabilities are available:

#### Compute-only (default)

Allows pure computation with no external access:

```go
tool, _ := wasm.NewWASMTool(
    "math",
    "Pure mathematical computations",
    wasmBytes,
    wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
)
```

**Allowed:**
- Mathematical operations
- String processing
- Data transformations
- Memory allocations (within limits)

**Blocked:**
- Network access (TCP, UDP, HTTP)
- Filesystem access (read/write)
- System calls
- Random number generation
- Clock access

#### Network-only

Allows network access but blocks filesystem:

```go
tool, _ := wasm.NewWASMTool(
    "api_client",
    "Call external HTTP APIs",
    wasmBytes,
    wasm.WithPolicy(wasm.NetworkOnlyPolicy()),
)
```

**Use cases:**
- HTTP API clients
- Data fetching from external services
- Webhook notifications

#### File processing

Allows filesystem access to specific directories:

```go
tool, _ := wasm.NewWASMTool(
    "csv_processor",
    "Process CSV files from data directory",
    wasmBytes,
    wasm.WithPolicy(wasm.FileProcessingPolicy(
        []string{"/data/input", "/data/output"}, // Allowed paths
        false,                                     // Read-write access
    )),
)
```

**Use cases:**
- Data file processing
- Log analysis
- Report generation

#### Deterministic

Ensures the same input always produces the same output by creating fresh module instances:

```go
tool, _ := wasm.NewWASMTool(
    "hash_function",
    "Cryptographic hashing",
    wasmBytes,
    wasm.WithPolicy(wasm.DeterministicPolicy()),
)
```

**Use cases:**
- Cryptographic operations
- Reproducible computations
- Testing and validation

#### Permissive

Allows all capabilities (use with caution):

```go
tool, _ := wasm.NewWASMTool(
    "system_tool",
    "Trusted system operations",
    wasmBytes,
    wasm.WithPolicy(wasm.PermissiveSandboxPolicy()),
)
```

### Custom policies

Create fine-grained policies for specific use cases:

```go
customPolicy := &wasm.SandboxPolicy{
    // Resource limits
    MaxMemoryBytes:    50 * 1024 * 1024,  // 50 MB
    TimeoutDuration:   5 * time.Second,
    
    // Capabilities
    AllowNetworkAccess:    false,
    AllowFilesystemAccess: false,
    AllowRandomness:       false,
    AllowClockAccess:      false,
    
    // Module instantiation
    InstanceReuse: wasm.ReuseNever,  // Fresh instance per call
    
    // Security level
    SecurityLevel: wasm.SecurityLevelThirdParty,
}

tool, _ := wasm.NewWASMTool(
    "custom_tool",
    "Tool with custom security policy",
    wasmBytes,
    wasm.WithPolicy(customPolicy),
)
```

### Resource limits

All policies support configurable resource constraints:

```go
policy := wasm.ComputeOnlyPolicy()
policy.MaxMemoryBytes = 100 * 1024 * 1024  // 100 MB limit
policy.TimeoutDuration = 10 * time.Second  // 10 second timeout

tool, _ := wasm.NewWASMTool("compute", "desc", wasmBytes,
    wasm.WithPolicy(policy))
```

### Building WASM modules

WASM tools require compiled WebAssembly modules. Here's how to build them:

#### Rust (recommended)

Rust produces the smallest and most efficient WASM binaries:

```rust
// src/lib.rs
use std::ffi::{CStr, CString};
use std::os::raw::c_char;

#[no_mangle]
pub extern "C" fn add(a: f64, b: f64) -> f64 {
    a + b
}

#[no_mangle]
pub extern "C" fn process(input_ptr: *const c_char) -> *mut c_char {
    let input = unsafe { CStr::from_ptr(input_ptr).to_string_lossy() };
    let result = format!("Processed: {}", input);
    CString::new(result).unwrap().into_raw()
}
```

**Build configuration (Cargo.toml):**

```toml
[package]
name = "my-wasm-tool"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[profile.release]
opt-level = "z"      # Optimize for size
lto = true           # Link-time optimization
strip = true         # Strip debug symbols
panic = "abort"      # Smaller panic handler
codegen-units = 1    # Better optimization
```

**Build:**

```bash
cargo build --target wasm32-unknown-unknown --release
# Output: target/wasm32-unknown-unknown/release/my_wasm_tool.wasm
```

**Typical binary size:** 70-130 KB (optimized)

#### TinyGo

For Go developers, TinyGo can compile to WASM:

```go
//go:build wasm
package main

import "syscall/js"

func add(this js.Value, args []js.Value) interface{} {
    a := args[0].Float()
    b := args[1].Float()
    return a + b
}

func main() {
    js.Global().Set("add", js.FuncOf(add))
    <-make(chan bool)
}
```

**Build:**

```bash
tinygo build -o tool.wasm -target wasm main.go
```

**Binary size:** 400-1000 KB (larger than Rust)

### Tool interface contract

WASM modules must expose a `call` function that accepts and returns JSON:

```rust
#[no_mangle]
pub extern "C" fn call(input_ptr: *const c_char) -> *mut c_char {
    let input = unsafe { CStr::from_ptr(input_ptr).to_string_lossy() };
    
    // Parse JSON input
    let args: serde_json::Value = serde_json::from_str(&input)
        .unwrap_or(serde_json::Value::Null);
    
    // Perform operation
    let result = process(args);
    
    // Return JSON result
    let output = serde_json::to_string(&result).unwrap();
    CString::new(output).unwrap().into_raw()
}
```

### Integration with agents

WASM tools work seamlessly with all agent types:

```go
// Create WASM tools
mathTool, _ := wasm.NewWASMTool("math", "Math operations", mathWasm,
    wasm.WithPolicy(wasm.ComputeOnlyPolicy()))

apiTool, _ := wasm.NewWASMTool("fetch_data", "Fetch external data", apiWasm,
    wasm.WithPolicy(wasm.NetworkOnlyPolicy()))

// Use in ReAct agent
agent, _ := agent.NewReAct(
    model,
    []tool.Tool{mathTool, apiTool},
)

// Use in supervisor agent
supervisor, _ := agent.NewSupervisor(
    model,
    agent.WithWorker("compute", "Computation worker", agent.NewReAct(model, []tool.Tool{mathTool})),
    agent.WithWorker("fetch", "Data fetching worker", agent.NewReAct(model, []tool.Tool{apiTool})),
)
```

### Security guarantees

WASM tools provide verifiable security guarantees:

1. **Memory isolation** - WASM modules have isolated linear memory, cannot access host memory
2. **No syscalls** - Network, filesystem, and system calls are blocked by default
3. **Resource limits** - Memory and CPU usage are strictly enforced
4. **Controlled capabilities** - All host access goes through explicitly granted WASI interfaces
5. **Sandboxed errors** - Errors and panics are contained within the module

These guarantees are enforced by the Wazero WebAssembly runtime and cannot be bypassed by malicious code.

### Performance characteristics

- **Module loading**: 1-2ms per tool creation
- **Function call overhead**: 1-5ms per invocation
- **Memory overhead**: ~1-2 MB per loaded module
- **Instance creation**: 100-500μs per fresh instance

For most agent workflows, this overhead is negligible compared to LLM inference time.

### Best practices

**Use the most restrictive policy that works:**

```go
// Good: Compute-only for pure functions
mathTool, _ := wasm.NewWASMTool("math", "desc", wasmBytes,
    wasm.WithPolicy(wasm.ComputeOnlyPolicy()))

// Avoid: Permissive when compute-only would work
mathTool, _ := wasm.NewWASMTool("math", "desc", wasmBytes,
    wasm.WithPolicy(wasm.PermissiveSandboxPolicy()))  // Too permissive
```

**Set appropriate resource limits:**

```go
policy := wasm.ComputeOnlyPolicy()
policy.MaxMemoryBytes = 10 * 1024 * 1024  // 10 MB for small computations
policy.TimeoutDuration = 2 * time.Second   // Fast timeout for simple ops

tool, _ := wasm.NewWASMTool("fast_op", "desc", wasmBytes,
    wasm.WithPolicy(policy))
```

**Use deterministic policies for reproducible operations:**

```go
// Ensure cryptographic operations are reproducible
hashTool, _ := wasm.NewWASMTool("hash", "desc", wasmBytes,
    wasm.WithPolicy(wasm.DeterministicPolicy()))
```

**Document security expectations:**

```go
// Clear documentation of capabilities
tool, _ := wasm.NewWASMTool(
    "api_client",
    "Fetches data from external APIs. Requires network access. No filesystem or random access.",
    wasmBytes,
    wasm.WithPolicy(wasm.NetworkOnlyPolicy()),
)
```

### Example: Calculator tool

See the [wasm_tool example](https://github.com/hupe1980/agentmesh/tree/main/examples/wasm_tool) for a complete working example of a WASM-based calculator tool integrated with a ReAct agent.

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
- Tool runs gain a `core.ToolContext` exposing session state and artifact helpers.
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

---

## Related

- **[Middleware System](/middleware/)** - Extend tool execution with caching, timeouts, circuit breakers, and audit logging
- **[Agents](/agents/)** - Build agents that use tools
- **[WASM Sandboxing](/wasm-sandboxing/)** - Run untrusted tools in secure sandboxes
