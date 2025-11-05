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

AgentMesh provides high-level agent constructors for common patterns like ReAct and RAG, while also exposing the underlying graph builder for custom workflows. All agents are compiled into executable graphs that run on the Pregel BSP engine.

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

// Create ReAct agent
compiled, err := agent.NewReActAgent(
    openai.NewModel(),
    []tool.Tool{searchTool, calcTool},
    agent.WithMaxIterations(5),
)

// Execute
results, err := compiled.Invoke(ctx, messages)
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

// Execute
results, err := compiled.Invoke(ctx, messages)
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
builder.Node("classify", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    // Classify the user's intent
    msgs := s.MessagesSnapshot()
    category := classifyIntent(msgs)
    
    return &graph.NodeResult{
        Updates: map[string]any{"category": category},
    }, nil
})

builder.Node("handle_support", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    // Handle support queries
    response := message.NewAIMessage(message.NewTextPart("Support response..."))
    return &graph.NodeResult{
        Messages: []message.Message{response},
    }, nil
})

builder.Node("handle_sales", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
results, err := compiled.Invoke(ctx, messages)
```

### Node functions

Nodes receive a `StateWriter` and return a `NodeResult`:

```go
RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
builder.Node("research", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    // Execute subgraph
    messages, err := researchGraph.Invoke(ctx, s.MessagesSnapshot())
    if err != nil {
        return nil, err
    }
    
    return &graph.NodeResult{
        Messages: messages,
    }, nil
})
```

See `examples/subgraph` for a complete demonstration.
