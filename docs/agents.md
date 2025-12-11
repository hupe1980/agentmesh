---
layout: doc
title: Agents
description: Build ReAct agents, RAG agents, and custom graph-based workflows.
permalink: /agents/
example: basic_agent
hero:
  title: Build intelligent agent workflows
  description: Create agents using pre-built patterns or compose custom graphs with nodes, edges, and conditional routing.
  primary_cta:
    label: Create a ReAct agent
    href: "#react-agent"
  secondary_cta:
    label: API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/agent"
    external: true
sidebar:
  - title: ReAct agent
    url: "#react-agent"
  - title: Supervisor agent
    url: "#supervisor-agent"
  - title: Reflection agent
    url: "#reflection-agent"
  - title: RAG agent
    url: "#rag-agent"
  - title: Conversational agent
    url: "#conversational-agent"
  - title: Custom graphs
    url: "#custom-graphs"
  - title: Conditional routing
    url: "#conditional-routing"
  - title: Parallel execution
    url: "#parallel-execution"
  - title: Subgraphs
    url: "#subgraphs"
---

AgentMesh provides high-level agent constructors for common patterns like ReAct, RAG, and Supervisor multi-agent coordination, while also exposing the underlying graph builder for custom workflows. All agents are compiled into executable graphs that run on the Pregel BSP engine.

---

## ReAct agent {#react-agent}

The **ReAct (Reasoning and Acting)** pattern creates an agent that iteratively:
1. Reasons about the task
2. Decides which tool to use
3. Observes the result
4. Repeats until the answer is found

This is the most common pattern for multi-step problem solving with tool use.

```go
import (
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
    "github.com/hupe1980/agentmesh/pkg/tool"
)

// Create tools
searchTool, _ := tool.NewFuncTool("search", "Search the web", searchFunc)
calcTool, _ := tool.NewFuncTool("calculator", "Perform calculations", calcFunc)

// Create ReAct agent (returns *message.Graph)
reactAgent, err := agent.NewReAct(
    openai.NewModel(),
    agent.WithTools(searchTool, calcTool),
    agent.WithMaxIterations(5),
)

// Execute with iterator pattern
for msg, err := range reactAgent.Run(ctx, messages) {
    if err != nil {
        log.Fatal(err)
    }
    // Process each message
    fmt.Println(msg.Content())
}
```

### Configuration options

```go
agent.NewReAct(model,
    agent.WithTools(searchTool, calcTool),    // Add tools
    agent.WithSupervisorMaxIterations(10),              // Max reasoning-action cycles
    agent.WithInstructions("You are helpful"), // Instructions
    agent.WithOutputSchema(schema),      // Structured output
    agent.WithGraphMiddleware(middleware...),  // Graph middleware
    agent.WithModelMiddleware(middleware...),  // Model middleware
    agent.WithToolMiddleware(middleware...),   // Tool middleware
)
```

> **See also:** [Middleware System](/middleware/) for caching, retries, rate limiting, circuit breakers, and more.
```

### Dynamic instructions

Instructions support Go `text/template` syntax with automatic state substitution. Placeholders are replaced with values from the graph state at runtime:

```go
// Define state keys
var UserNameKey = graph.NewKey[string]("userName", "")
var TaskKey = graph.NewKey[string]("task", "")

// Use placeholders in instructions - they resolve from graph state
agent.NewReAct(model,
    agent.WithInstructions("You are helping {{.userName}}. Current task: {{.task}}"),
)

// At runtime, set state values before invoking the agent
state.Set(UserNameKey, "Alice")
state.Set(TaskKey, "analyze sales data")
// Instructions resolve to: "You are helping Alice. Current task: analyze sales data"
```

**Available template features:**
- `{{.keyName}}` - Substitute value from graph state
- `{{default "fallback" .value}}` - Use fallback if value is nil/empty
- `{{.Name | upper}}` - Convert to uppercase
- `{{.Name | lower}}` - Convert to lowercase
- `{{if .condition}}...{{end}}` - Conditionals

**Dynamic provider alternative:**

For complex logic, use a provider function:

```go
agent.WithInstructionsFromFunc(func(ctx context.Context, scope graph.ReadOnlyScope) (string, error) {
    userName := graph.Get(scope, UserNameKey)
    if userName == "" {
        return "You are a helpful assistant.", nil
    }
    return fmt.Sprintf("You are helping %s.", userName), nil
})
```

### How it works

The ReAct agent compiles into a graph with a reasoning-action loop:

<div class="mermaid">
flowchart LR
    START((START)) --> Model
    Model -->|"tool calls"| Tools
    Tools --> Model
    Model -->|"final answer"| END((END))
    
    style START fill:#22c55e,stroke:#16a34a,color:#fff
    style END fill:#ef4444,stroke:#dc2626,color:#fff
    style Model fill:#3b82f6,stroke:#2563eb,color:#fff
    style Tools fill:#8b5cf6,stroke:#7c3aed,color:#fff
</div>

**Architecture:**

1. **Model node**: Uses `model.Executor` to generate response or tool calls
   - Delegates execution to executor (handles observability, streaming)
   - Routes to "tool" if tool calls present, otherwise routes to END

2. **Tool node**: Uses `tool.Executor` to execute requested tools
   - Parallel execution via `ParallelExecutor` by default
   - Formats results as ToolMessages
   - Routes back to model node

3. **Executor pattern benefits**:
   - Clean separation: nodes handle orchestration, executors handle execution
   - Reusable: same executors work in graphs, chains, or direct calls
   - Extensible: custom executors (retry, caching) without modifying nodes
   - Efficient: Arguments stay as JSON strings (no extra conversions)

---

## Supervisor agent {#supervisor-agent}

The **Supervisor Agent Pattern** creates a coordinator that routes tasks to specialized worker agents based on the user's query. This enables building complex multi-agent systems with clean separation of concerns.

```go
import (
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
)

model := openai.NewModel()

// Create specialized worker agents
mathAgent, _ := agent.NewReAct(
    model,
    agent.WithInstructions("You are a math expert. Solve problems with clear steps."),
    agent.WithMaxIterations(5),
)

codeAgent, _ := agent.NewReAct(
    model,
    agent.WithInstructions("You are a programming expert. Write clean, documented code."),
    agent.WithMaxIterations(5),
)

historyAgent, _ := agent.NewReAct(
    model,
    agent.WithInstructions("You are a history expert. Provide factual answers with dates."),
    agent.WithMaxIterations(5),
)

// Create supervisor that routes to specialists
supervisor, err := agent.NewSupervisor(
    model,
    agent.WithWorker("math", "Expert in mathematics and calculations", mathAgent),
    agent.WithWorker("code", "Expert in programming and software development", codeAgent),
    agent.WithWorker("history", "Expert in historical facts and events", historyAgent),
    agent.WithSupervisorInstructions("Route queries to the appropriate specialist"),
    agent.WithSupervisorMaxIterations(10),
    agent.WithWorkerRetries(2),
)

// Execute with iterator pattern
for msg, err := range supervisor.Run(ctx, []message.Message{
    message.NewHumanMessageFromText("What is the derivative of x^2 + 3x?"),
}) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(msg.Content())
}
```

### Configuration options

```go
agent.NewSupervisor(model,
    agent.WithWorker(name, description, agent),  // Add worker agents
    agent.WithInstructions(prompt),    // Custom routing instructions
    agent.WithMaxIterations(n),        // Max routing iterations
    agent.WithWorkerRetries(n),                  // Retry failed worker invocations
    agent.WithWorkerValidation(bool),            // Validate worker results
)
```

### How it works

The supervisor agent uses **tool-based handoffs** to delegate work:

<div class="mermaid">
flowchart TB
    User["User Query"] --> Supervisor
    
    subgraph SupervisorFlow["Supervisor Agent"]
        Supervisor["Supervisor<br/><i>Routing Logic</i>"]
    end
    
    Supervisor -->|"handoff_to_math"| Math["Math Agent"]
    Supervisor -->|"handoff_to_code"| Code["Code Agent"]
    Supervisor -->|"handoff_to_history"| History["History Agent"]
    
    Math --> Result
    Code --> Result
    History --> Result
    Result["Result"] --> User2["User Response"]
    
    style User fill:#22c55e,stroke:#16a34a,color:#fff
    style Supervisor fill:#3b82f6,stroke:#2563eb,color:#fff
    style Math fill:#8b5cf6,stroke:#7c3aed,color:#fff
    style Code fill:#8b5cf6,stroke:#7c3aed,color:#fff
    style History fill:#8b5cf6,stroke:#7c3aed,color:#fff
    style Result fill:#f59e0b,stroke:#d97706,color:#fff
    style User2 fill:#22c55e,stroke:#16a34a,color:#fff
</div>

**Key benefits**:

- 🎯 **Automatic routing**: Supervisor intelligently routes to the right specialist
- 🔧 **Automatic tool creation**: Each worker gets a `handoff_to_<name>` tool
- 🔄 **Fresh context**: Workers can receive only the task, not full conversation (configurable)
- ♻️ **Retry logic**: Configurable retries for robust execution

> **Tip:** Add progress middleware to see which workers are invoked during execution. See [Middleware](/middleware/#custom-middleware) for examples.

See `examples/supervisor_agent` for a basic demonstration and `examples/blogwriter` for a complete multi-agent workflow with progress tracking.

---

## Reflection agent {#reflection-agent}

The **Reflection Agent** is a composable wrapper that adds self-critique and iterative refinement to ANY agent type. It wraps another agent and automatically improves its outputs through reflection loops.

```go
import (
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
)

model := openai.NewModel()

// Create a base agent (ReAct, RAG, Supervisor, or custom)
reactAgent, _ := agent.NewReAct(
    model,
    agent.WithInstructions("You are a helpful assistant."),
    agent.WithTools(searchTool, calcTool),
)

// Wrap with reflection capabilities
wrappedAgent, err := agent.NewReflection(
    reactAgent,                                   // ANY agent type!
    model,                                        // Model for critique (can differ)
    agent.WithReflectionMaxIterations(2),         // Max refinement cycles
    agent.WithReflectionPromptTemplate(template), // Custom critique prompt
)

// Execute - answers are automatically refined
for msg, err := range wrappedAgent.Run(ctx, messages) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(msg.Content())
}
```

### Configuration options

```go
agent.NewReflection(baseAgent, reflectionModel,
    agent.WithReflectionMaxIterations(n),         // Max refinement iterations
    agent.WithReflectionPromptTemplate(template), // Custom critique prompt
    agent.WithReflectionModelMiddleware(...),     // Middleware for reflection
    agent.WithReflectionGraphMiddleware(...),     // Middleware for the graph
)
```

### How it works

The reflection agent creates a wrapper graph with a critique loop:

<div class="mermaid">
flowchart TB
    User["User Query"] --> Agent
    
    subgraph ReflectionLoop["Reflection Loop"]
        Agent["Wrapped Agent<br/><i>Generate Answer</i>"]
        Reflection["Reflection Node<br/><i>Critique Answer</i>"]
        
        Agent --> Decision{"Max iterations<br/>reached?"}
        Decision -->|"No"| Reflection
        Reflection --> Agent
        Decision -->|"Yes"| Result
    end
    
    Result["Final Answer"] --> User2["User Response"]
    
    style User fill:#22c55e,stroke:#16a34a,color:#fff
    style Agent fill:#3b82f6,stroke:#2563eb,color:#fff
    style Reflection fill:#8b5cf6,stroke:#7c3aed,color:#fff
    style Decision fill:#f59e0b,stroke:#d97706,color:#fff
    style Result fill:#22c55e,stroke:#16a34a,color:#fff
    style User2 fill:#22c55e,stroke:#16a34a,color:#fff
</div>

**Key benefits**:

- 🔄 **Iterative improvement**: Automatically refines answers through self-critique
- 🎯 **Composable**: Works with ANY agent type (ReAct, RAG, Supervisor, custom)
- 🧠 **Meta-reasoning**: Agent reasons about its own reasoning
- 🔧 **Flexible models**: Use different models for generation and critique
- 📊 **Transparent**: Critique feedback visible in message stream

**Use cases**:
- Complex explanations requiring clarity and completeness
- Technical writing that needs accuracy verification
- Code generation with quality checks
- Research tasks requiring thoroughness

See `examples/reflection_agent` for a complete demonstration.

---

## RAG agent {#rag-agent}

The **RAG (Retrieval-Augmented Generation)** pattern creates an agent that:
1. Retrieves relevant context from a knowledge base
2. Generates a response using both the query and retrieved context

This is ideal for question-answering over large document collections.

```go
import (
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
    "github.com/hupe1980/agentmesh/pkg/retrieval/langchaingo"
)

// Create retriever from vector store
retriever := langchaingo.NewRetrieverFromVectorStore(vectorStore, func(o *langchaingo.Options) {
    o.NumDocuments = 5
})

// Create RAG agent
ragAgent, err := agent.NewRAG(
    openai.NewModel(),
    retriever,
    agent.WithRAGPromptTemplate(customTemplate),
)

// Execute with iterator pattern
for msg, err := range ragAgent.Run(ctx, messages) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(msg.Content())
}
```

### Configuration options

```go
agent.NewRAG(model, retriever,
    agent.WithRAGPromptTemplate(template),  // Custom prompt template
)
```

### How it works

The RAG agent compiles into a graph with three nodes:

```
START → retrieve → generate → END
```

1. **Retrieve node**: Fetches relevant documents based on the user's query
2. **Generate node**: Creates a prompt with the query and retrieved context, then generates the response

---

## Conversational agent {#conversational-agent}

The **Conversational Agent** is a composable wrapper that adds long-term memory to ANY agent type. It automatically recalls relevant context from memory before the agent runs and stores the conversation after completion.

```go
import (
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/memory"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
)

model := openai.NewModel()

// Create embedder for semantic memory
embedder := openai.NewEmbedder(client)

// Create vector memory for semantic search
mem := memory.NewVectorMemory(embedder)

// Create a base agent (ReAct, RAG, Supervisor, or custom)
reactAgent, _ := agent.NewReAct(
    model,
    agent.WithInstructions("You are a helpful assistant."),
    agent.WithTools(tools...),
)

// Wrap with conversational memory
// Uses dual-memory approach: short-term (recent) + long-term (semantic)
chatAgent, err := agent.NewConversational(
    reactAgent,  // ANY agent type!
    mem,
    agent.WithShortTermMessages(5),       // Last 5 messages for immediate context
    agent.WithLongTermMessages(5),        // 5 semantically similar messages
    agent.WithMinSimilarityScore(0.5),    // Threshold for long-term recall
    agent.WithFailOnStoreError(false),    // Don't fail if memory store fails
)

// Execute with a session ID (required)
for msg, err := range chatAgent.Run(ctx, messages,
    graph.WithInitialValue(agent.SessionIDKey, "user-123-session"),
) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(msg.Content())
}
```

### Configuration options

The conversational agent uses a **dual-memory approach**:
- **Short-term memory**: Recent N messages (recency-based) - immediate context
- **Long-term memory**: Semantically similar messages from history (relevance-based)

```go
agent.NewConversational(baseAgent, memory,
    agent.WithShortTermMessages(5),       // Recent messages (default: 5)
    agent.WithLongTermMessages(5),        // Semantic search results (default: 5)
    agent.WithMinSimilarityScore(0.5),    // Threshold for long-term recall (default: 0.5)
    agent.WithFailOnStoreError(false),    // Fail if memory storage fails (default: false)
)
```

| Option | Description | Default |
|--------|-------------|---------|
| `WithShortTermMessages(n)` | Number of recent messages to always include | 5 |
| `WithLongTermMessages(n)` | Number of semantically similar messages | 5 |
| `WithMinSimilarityScore(s)` | Minimum similarity for long-term recall | 0.5 |
| `WithFailOnStoreError(b)` | Fail if memory storage fails | false |

### Session ID

A session ID **must** be provided at runtime using `graph.WithInitialValue`:

```go
// Each user/session gets its own memory context
chatAgent.Run(ctx, messages,
    graph.WithInitialValue(agent.SessionIDKey, "user-123-session"),
)
```

### How it works

The conversational agent creates a wrapper graph with memory integration:

<div class="mermaid">
flowchart LR
    START((START)) --> Recall["Memory Recall<br/><i>Semantic Search</i>"]
    Recall --> Agent["Wrapped Agent<br/><i>ReAct/RAG/etc.</i>"]
    Agent --> Store["Memory Store<br/><i>Save Exchange</i>"]
    Store --> END((END))
    
    style START fill:#22c55e,stroke:#16a34a,color:#fff
    style END fill:#ef4444,stroke:#dc2626,color:#fff
    style Recall fill:#8b5cf6,stroke:#7c3aed,color:#fff
    style Agent fill:#3b82f6,stroke:#2563eb,color:#fff
    style Store fill:#8b5cf6,stroke:#7c3aed,color:#fff
</div>

**Key benefits**:

- 🧠 **Semantic recall**: Finds relevant past messages by meaning, not keywords
- 🔄 **Composable**: Works with ANY agent type (ReAct, RAG, Supervisor, Reflection)
- 📝 **Automatic storage**: Stores conversation exchanges after each interaction
- 🔒 **Session isolation**: Each session has its own memory context
- ⚡ **Non-blocking**: Memory errors don't fail the agent by default

**Use cases**:
- Multi-turn conversations requiring context
- Personalized assistants that remember user preferences
- Customer support with conversation history
- Long-running agent sessions

See the [Memory Guide](/memory/) for more on memory types and configuration.

---

## Structured output {#structured-output}

Agents can return structured JSON responses matching a defined schema. This is useful when you need predictable, type-safe output from your agent.

### Native structured output

If your model supports native structured output (like OpenAI's `response_format`), the agent uses it directly:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/schema"
)

// Define your output structure
type AnalysisResult struct {
    Sentiment  string   `json:"sentiment" jsonschema:"required,enum=positive,neutral,negative"`
    Confidence float64  `json:"confidence" jsonschema:"required,minimum=0,maximum=1"`
    Keywords   []string `json:"keywords" jsonschema:"required"`
}

// Create output schema
outputSchema, err := schema.NewOutputSchema("analysis_result", AnalysisResult{})
if err != nil {
    log.Fatal(err)
}

// Create agent with structured output
reactAgent, err := agent.NewReAct(model,
    agent.WithOutputSchema(&outputSchema),
)
```

### Tool-based fallback

For models that **don't support native structured output** but **do support tool calling**, AgentMesh automatically uses a tool-based fallback. It injects a `set_model_response` tool that instructs the model to return structured data via tool calling:

```go
// This works even with models that don't have native structured output support
// as long as they support tool calling
reactAgent, err := agent.NewReAct(model,
    agent.WithOutputSchema(&outputSchema),
    agent.WithTools(searchTool, calcTool), // Your other tools work alongside
)
```

**How it works:**
1. Agent checks model capabilities at creation time
2. If `StructuredOutput: false` but `Tools: true`, injects `SetModelResponseTool`
3. The tool instructs the model to call it with the final response matching the schema
4. The tool validates and returns the structured response

**Behavior matrix:**

| Model Capabilities | Behavior |
|--------------------|----------|
| `StructuredOutput: true` | Uses native structured output |
| `StructuredOutput: false`, `Tools: true` | Uses `set_model_response` tool fallback |
| `StructuredOutput: false`, `Tools: false` | Schema passed to model (may not work) |

### Custom set_model_response tool

If you need custom behavior, you can provide your own `set_model_response` tool - the agent won't add a duplicate:

```go
import "github.com/hupe1980/agentmesh/pkg/tool"

// Create custom set_model_response tool
customTool, err := tool.NewSetModelResponseTool(&outputSchema)
if err != nil {
    log.Fatal(err)
}

// Agent uses your tool instead of creating one
reactAgent, err := agent.NewReAct(model,
    agent.WithOutputSchema(&outputSchema),
    agent.WithTools(customTool, searchTool),
)
```

---

## Custom graphs {#custom-graphs}

For complete control over workflow logic, build custom graphs using the graph API:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/message"
)

// Define typed keys
var CategoryKey = graph.NewKey[string]("category", "")

// Create message graph for agent workflows
g := message.NewGraphBuilder(CategoryKey)

// Add nodes using fluent API
g.Node("classify", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    messages := message.GetMessages(scope)
    category := classifyIntent(messages)
    
    if category == "support" {
        return graph.Set(CategoryKey, category).To("handle_support"), nil
    }
    return graph.Set(CategoryKey, category).To("handle_sales"), nil
}, "handle_support", "handle_sales")

g.Node("handle_support", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    response := message.NewAIMessageFromText("Support response...")
    return graph.Append(message.MessagesKey, response).To(graph.END), nil
}, graph.END)

g.Node("handle_sales", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    response := message.NewAIMessageFromText("Sales response...")
    return graph.Append(message.MessagesKey, response).To(graph.END), nil
}, graph.END)

g.Start("classify")

// Compile and execute
compiled, _ := g.Build()

for result, err := range compiled.Run(ctx, messages) {
    if err != nil {
        log.Fatal(err)
    }
    // Process results
    fmt.Println(result.Content())
}
```

### Node functions

Nodes receive a `Scope[O]` (which embeds `ReadOnlyScope`) and return a `Command`:

```go
g.Node("process", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    // Read state with typed keys
    previousValue := graph.Get(scope, MyKey)
    messages := message.GetMessages(scope)
    
    // Process...
    
    // Return updates and routing
    return graph.Set(MyKey, newValue).
        Append(message.MessagesKey, newMessage).
        To("next_node"), nil
}, "next_node")
```

---

## Conditional routing {#conditional-routing}

Direct execution flow dynamically using commands:

```go
g.Node("router", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    action := graph.Get(scope, ActionKey)
    
    switch action {
    case "approve":
        return graph.To("approver"), nil
    case "reject":
        return graph.To("rejector"), nil
    case "escalate":
        return graph.To("human_review"), nil
    default:
        return graph.To("default_handler"), nil
    }
}, "approver", "rejector", "human_review", "default_handler")
```

Nodes can route to multiple targets for parallel execution:

```go
g.Node("fanout", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    // Route to all three analysts in parallel
    return graph.To("analyst_a", "analyst_b", "analyst_c"), nil
}, "analyst_a", "analyst_b", "analyst_c")
```

---

## Parallel execution {#parallel-execution}

Nodes can fan out to parallel execution by routing to multiple targets:

```go
// Entry node fans out to three concurrent tasks
g.Node("start", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.To("fetch_data_a", "fetch_data_b", "fetch_data_c"), nil
}, "fetch_data_a", "fetch_data_b", "fetch_data_c")

// Each fetch task routes to aggregator
g.Node("fetch_data_a", fetchAFunc, "aggregator")
g.Node("fetch_data_b", fetchBFunc, "aggregator")
g.Node("fetch_data_c", fetchCFunc, "aggregator")

g.Node("aggregator", aggregateFunc, graph.END)

g.Start("start")
```

The aggregator waits for all incoming nodes to complete before executing.

---

## Subgraphs {#subgraphs}

Compose complex workflows from reusable graph components using `graph.Subgraph()`:

```go
// Create a research subgraph
researchSub := createResearchGraph()
compiledResearch, _ := researchSub.Build()

// Create parent graph
parent := message.NewGraphBuilder()

// Embed subgraph as a node using graph.Subgraph with mappers
parent.Node("research", graph.Subgraph(
    compiledResearch,
    // InputMapper: transform parent messages to subgraph input
    func(ctx context.Context, scope graph.Scope[string]) ([]message.Message, error) {
        messages := message.GetMessages(scope)
        // Filter or transform messages for research subgraph
        return messages, nil
    },
    // OutputMapper: merge research results back to parent
    func(ctx context.Context, output message.Message) (graph.Updates, error) {
        return graph.Append(message.MessagesKey, output), nil
    },
), "synthesize")

parent.Node("synthesize", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    messages := message.GetMessages(scope)
    // Synthesize research results...
    return graph.Append(message.MessagesKey, summary).To(graph.END)
}, graph.END)

parent.Start("research")
compiled, _ := parent.Build()
```

**Type Safety**: The `InputMapper[SI]` and `OutputMapper[SO]` type aliases provide clear signatures for state transformation functions.

See `examples/subgraph` for a complete demonstration.
