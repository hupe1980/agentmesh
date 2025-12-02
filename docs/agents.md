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
  - title: RAG agent
    url: "#rag-agent"
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
reactAgent, err := agent.NewReActAgent(
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
agent.NewReActAgent(model,
    agent.WithTools(searchTool, calcTool),    // Add tools
    agent.WithMaxIterations(10),              // Max reasoning-action cycles
    agent.WithSystemPrompt("You are helpful"), // System prompt
    agent.WithReActOutputSchema(schema),      // Structured output
    agent.WithGraphMiddleware(middleware...),  // Graph middleware (retry, etc.)
    agent.WithModelMiddleware(middleware...),  // Model middleware
    agent.WithToolMiddleware(middleware...),   // Tool middleware
)
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
mathAgent, _ := agent.NewReActAgent(
    model,
    agent.WithSystemPrompt("You are a math expert. Solve problems with clear steps."),
    agent.WithMaxIterations(5),
)

codeAgent, _ := agent.NewReActAgent(
    model,
    agent.WithSystemPrompt("You are a programming expert. Write clean, documented code."),
    agent.WithMaxIterations(5),
)

historyAgent, _ := agent.NewReActAgent(
    model,
    agent.WithSystemPrompt("You are a history expert. Provide factual answers with dates."),
    agent.WithMaxIterations(5),
)

// Create supervisor that routes to specialists
supervisor, err := agent.NewSupervisorAgent(
    model,
    agent.WithWorker("math", "Expert in mathematics and calculations", mathAgent),
    agent.WithWorker("code", "Expert in programming and software development", codeAgent),
    agent.WithWorker("history", "Expert in historical facts and events", historyAgent),
    agent.WithSupervisorSystemPrompt("Route queries to the appropriate specialist"),
    agent.WithSupervisorMaxIterations(10),
    agent.WithWorkerContext(false),  // Fresh context for each task
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
agent.NewSupervisorAgent(model,
    agent.WithWorker(name, description, agent),  // Add worker agents
    agent.WithSupervisorSystemPrompt(prompt),    // Custom routing instructions
    agent.WithSupervisorMaxIterations(n),        // Max routing iterations
    agent.WithWorkerContext(bool),               // Pass conversation history to workers
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

See `examples/supervisor_agent` for a complete demonstration.

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
ragAgent, err := agent.NewRAGAgent(
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
agent.NewRAGAgent(model, retriever,
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
g.Node("classify", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    messages := message.GetMessages(view)
    category := classifyIntent(messages)
    
    if category == "support" {
        return graph.Set(CategoryKey, category).To("handle_support"), nil
    }
    return graph.Set(CategoryKey, category).To("handle_sales"), nil
}, "handle_support", "handle_sales")

g.Node("handle_support", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    response := message.NewAIMessageFromText("Support response...")
    return graph.Append(message.MessagesKey, response).To(graph.END), nil
}, graph.END)

g.Node("handle_sales", func(ctx context.Context, view graph.View) (*graph.Command, error) {
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

Nodes receive a `View` and return a `Command`:

```go
g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    // Read state with typed keys
    previousValue := graph.Get(view, MyKey)
    messages := message.GetMessages(view)
    
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
g.Node("router", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    action := graph.Get(view, ActionKey)
    
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
g.Node("fanout", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    // Route to all three analysts in parallel
    return graph.To("analyst_a", "analyst_b", "analyst_c"), nil
}, "analyst_a", "analyst_b", "analyst_c")
```

---

## Parallel execution {#parallel-execution}

Nodes can fan out to parallel execution by routing to multiple targets:

```go
// Entry node fans out to three concurrent tasks
g.Node("start", func(ctx context.Context, view graph.View) (*graph.Command, error) {
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
    func(ctx context.Context, view graph.View) ([]message.Message, error) {
        messages := message.GetMessages(view)
        // Filter or transform messages for research subgraph
        return messages, nil
    },
    // OutputMapper: merge research results back to parent
    func(ctx context.Context, output message.Message) (graph.Updates, error) {
        return graph.Append(message.MessagesKey, output), nil
    },
), "synthesize")

parent.Node("synthesize", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    messages := message.GetMessages(view)
    // Synthesize research results...
    return graph.Append(message.MessagesKey, summary).To(graph.END)
}, graph.END)

parent.Start("research")
compiled, _ := parent.Build()
```

**Type Safety**: The `InputMapper[SI]` and `OutputMapper[SO]` type aliases provide clear signatures for state transformation functions.

See `examples/subgraph` for a complete demonstration.
