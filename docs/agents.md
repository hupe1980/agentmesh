---
layout: doc
title: Agents
description: Build ReAct agents, RAG agents, and custom graph-based workflows.
permalink: /agents/
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

// Create ReAct agent (returns graph.MessageRunnable)
agent, err := agent.NewReActAgent(
    openai.NewModel(),
    agent.WithTools(searchTool, calcTool),
    agent.WithMaxIterations(5),
)

// Execute and collect messages
messages, err := graph.CollectMessages(agent.Run(ctx, messages))
if err != nil {
    log.Fatal(err)
}
```

### Configuration options

```go
agent.NewReActAgent(model, tools,
    agent.WithMaxIterations(10),          // Max reasoning-action cycles
    agent.WithRetryPolicy(retryPolicy),   // Configure retry behavior
)
```

### How it works

The ReAct agent compiles into a graph with three nodes:

```
START → model → tools → model → END
         ↓              ↑
         └──────────────┘
```

1. **Model node**: Generates response or tool calls
2. **Tool node**: Executes requested tools in parallel
3. **Conditional routing**: Loops back to model if tools were called, otherwise proceeds to END

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

// Execute and collect messages
messages, err := graph.CollectMessages(supervisor.Run(ctx, []message.Message{
    message.NewHumanMessageFromText("What is the derivative of x^2 + 3x?"),
}))
if err != nil {
    log.Fatal(err)
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

1. **Supervisor receives query**: Analyzes the user's request
2. **Routes to specialist**: Uses `HandoffToAgent` tool to delegate
3. **Worker processes task**: Specialist agent handles the specific domain
4. **Returns result**: Supervisor receives worker output and returns to user

```
User Query → Supervisor (routing logic)
                ↓
            HandoffToAgent tool
                ↓
        Specialist Worker Agent
                ↓
            Result → User
```

**Key benefits**:

- 🎯 **Automatic routing**: Supervisor intelligently routes to the right specialist
- 🔧 **Automatic tool creation**: Each worker gets a `handoff_to_<name>` tool
- 🔄 **Fresh context**: Workers can receive only the task, not full conversation (configurable)
- ♻️ **Retry logic**: Configurable retries for robust execution
- ✨ **Clean API**: Functional options pattern for configuration

### Use cases

- **Customer support**: Route to billing, technical, or sales specialists
- **Research teams**: Delegate to data analyst, researcher, or summarizer
- **Code review**: Route to security, performance, or style reviewers
- **Multi-domain Q&A**: Math, history, science specialists

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
compiled, err := agent.NewRAGAgent(
    openai.NewModel(),
    retriever,
    agent.WithRAGPromptTemplate(customTemplate),
)

// Execute and collect results
events, err := graph.Collect(compiled.Run(ctx, messages))
if err != nil {
    log.Fatal(err)
}
messages := graph.ExtractMessages(events)
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

For complete control over workflow logic, build custom graphs using the graph builder:

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

builder := graph.NewBuilder()

// Add nodes
builder.Node("classify", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    // Classify the user's intent
    msgs := s.MessagesSnapshot()
    category := classifyIntent(msgs)
    
    return &graph.NodeResult{
        Updates: map[string]any{"category": category},
    }, nil
})

builder.Node("handle_support", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    // Handle support queries
    response := message.NewAIMessage(message.NewTextPart("Support response..."))
    return &graph.NodeResult{
        Messages: []message.Message{response},
    }, nil
})

builder.Node("handle_sales", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    // Handle sales queries
    response := message.NewAIMessage(message.NewTextPart("Sales response..."))
    return &graph.NodeResult{
        Messages: []message.Message{response},
    }, nil
})

// Define flow
builder.AddEdge("START", "classify")
builder.AddConditionalEdges("classify", func(result *graph.NodeResult) []string {
    category := result.Updates["category"].(string)
    if category == "support" {
        return []string{"handle_support"}
    }
    return []string{"handle_sales"}
})
builder.AddEdge("handle_support", "END")
builder.AddEdge("handle_sales", "END")

// Compile and execute
compiled, err := builder.Compile()
messages, err := graph.CollectMessages(compiled.Run(ctx, messages))
if err != nil {
    log.Fatal(err)
}
```

### Node functions

Nodes receive a `Writer` and return a `NodeResult`:

```go
RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    // Read state
    previousValue := s.Get("key")
    messages := s.MessagesSnapshot()
    
    // Process...
    
    // Return updates
    return &graph.NodeResult{
        Messages: []message.Message{newMessage},
        Updates: map[string]any{
            "key": newValue,
            "counter": 1,  // Will be summed if using BinaryOpChannel
        },
    }, nil
}
```

---

## Conditional routing {#conditional-routing}

Direct execution flow dynamically based on node outputs:

```go
builder.AddConditionalEdges("router", func(result *graph.NodeResult) []string {
    // Route based on node output
    switch result.Updates["action"].(string) {
    case "approve":
        return []string{"approver"}
    case "reject":
        return []string{"rejector"}
    case "escalate":
        return []string{"human_review"}
    default:
        return []string{"default_handler"}
    }
})
```

Routes can return multiple node names for parallel execution:

```go
builder.AddConditionalEdges("fanout", func(result *graph.NodeResult) []string {
    // Execute all three analysts in parallel
    return []string{"analyst_a", "analyst_b", "analyst_c"}
})
```

---

## Parallel execution {#parallel-execution}

Nodes with the same predecessors automatically execute in parallel:

```go
// These three nodes execute concurrently
builder.AddEdge("START", "fetch_data_a")
builder.AddEdge("START", "fetch_data_b")
builder.AddEdge("START", "fetch_data_c")

// All converge to aggregator
builder.AddEdge("fetch_data_a", "aggregator")
builder.AddEdge("fetch_data_b", "aggregator")
builder.AddEdge("fetch_data_c", "aggregator")
```

The aggregator waits for all predecessors to complete before executing.

---

## Subgraphs {#subgraphs}

Compose complex workflows from reusable graph components:

```go
// Create a research subgraph
researchGraph := createResearchGraph()

// Embed in parent graph
builder.Node("research", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    // Execute subgraph
    messages, err := graph.CollectMessages(researchGraph.Run(ctx, s.MessagesSnapshot()))
    if err != nil {
        return nil, err
    }
    
    return &graph.NodeResult{
        Messages: messages,
    }, nil
})
```

See `examples/subgraph` for a complete demonstration.
