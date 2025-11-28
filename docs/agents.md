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

// Create ReAct agent (returns graph.MessageRunnable)
agent, err := agent.NewReActAgent(
    openai.NewModel(),
    agent.WithTools(searchTool, calcTool),
    agent.WithMaxIterations(5),
)

// Execute and collect messages
messages, err := agent.CollectMessages(agent.Run(ctx, messages))
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

**Under the hood:**

```go
// ReActAgent creates executors and nodes
modelExecutor := model.NewExecutor(mdl, model.WithExecutorName("react_model"))
toolExecutor := tool.NewParallelExecutor(toolRegistry)

modelNode := agent.NewModelNode(modelExecutor,
    agent.WithModelSystemPrompt(systemPrompt),
    agent.WithModelTools(tools...))

toolNode := agent.NewToolNode(toolExecutor)
```

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
messages, err := agent.CollectMessages(supervisor.Run(ctx, []message.Message{
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

**Execution flow:**

1. **Supervisor receives query**: Analyzes the user's request
2. **Routes to specialist**: Uses `HandoffToAgent` tool to delegate
3. **Worker processes task**: Specialist agent handles the specific domain
4. **Returns result**: Supervisor receives worker output and returns to user

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
import (
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/graph"
)

var MessagesKey = agent.MessagesKey  // From agent package

builder := graph.NewBuilder()

// Add nodes
builder.AddNodeFunc("classify", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    // Classify the user's intent
    msgs := state.GetFromView(view, MessagesKey)
    category := classifyIntent(msgs)
    
    return &graph.NodeResult{
        Updates: map[string]any{"category": category},
    }, nil
})

builder.AddNodeFunc("handle_support", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    // Handle support queries
    response := message.NewAIMessage(message.NewTextPart("Support response..."))
    return &graph.NodeResult{
        Updates: map[string]any{
            agent.MessagesKey.Name(): []message.Message{response},
        },
    }, nil
})

builder.AddNodeFunc("handle_sales", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    // Handle sales queries
    response := message.NewAIMessage(message.NewTextPart("Sales response..."))
    return &graph.NodeResult{
        Updates: map[string]any{
            agent.MessagesKey.Name(): []message.Message{response},
        },
    }, nil
})

// Classifier node uses Command pattern for routing
g.AddNode(&graph.BaseNode{
    NodeName:        "classify",
    DeclaredTargets: []string{"handle_support", "handle_sales"},
    Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        // Classify the query...
        category := "support"  // or "sales"
        
        cmd := command.New().With(command.SetValue(categoryKey, category))
        
        if category == "support" {
            return cmd.To("handle_support")
        } else {
            return cmd.To("handle_sales")
        }
    },
})

g.SetEntryPoint("classify")

// Compile and execute
compiled, err := builder.Compile()
messages, err := agent.CollectMessages(compiled.Run(ctx, messages))
if err != nil {
    log.Fatal(err)
}
```

### Node functions

Nodes receive a `ReadView` and return a `NodeResult`:

```go
var (
    KeyName     = state.NewKey("key", "")
    MessagesKey = agent.MessagesKey  // From agent package
)

RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    // Read state with typed keys
    previousValue := state.GetFromView(view, KeyName)
    messages := state.GetFromView(view, MessagesKey)
    
    // Process...
    
    // Return updates
    return &graph.NodeResult{
        Updates: map[string]any{
            "key": newValue,
            "counter": 1,  // Will be summed if using BinaryOpChannel
            agent.MessagesKey.Name(): []message.Message{newMessage},
        },
    }, nil
}
```

---

## Conditional routing {#conditional-routing}

Direct execution flow dynamically using tuple returns:

```go
g.AddNode(&graph.BaseNode{
    NodeName:        "router",
    DeclaredTargets: []string{"approver", "rejector", "human_review", "default_handler"},
    Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        action := state.GetFromView(view, ActionKey)
        var nextNode string
        switch action {
        case "approve":
            nextNode = "approver"
        case "reject":
            nextNode = "rejector"
        case "escalate":
            nextNode = "human_review"
        default:
            nextNode = "default_handler"
        }
        return command.New().To(nextNode)
    },
})
```

Nodes can declare multiple targets for potential parallel execution:

```go
g.AddNode(&graph.BaseNode{
    NodeName:        "fanout",
    DeclaredTargets: []string{"analyst_a", "analyst_b", "analyst_c"},
    Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        // Use Command pattern for parallel execution
        return command.New().To("analyst_a", "analyst_b", "analyst_c")
    },
})
```

---

## Parallel execution {#parallel-execution}

Nodes with declared targets can fan out to parallel execution:

```go
// Entry node fans out to three concurrent tasks
g.AddNode(&graph.BaseNode{
    NodeName:        "start",
    DeclaredTargets: []string{"fetch_data_a", "fetch_data_b", "fetch_data_c"},
    Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        // Use Command pattern for parallel execution
        return command.New().To("fetch_data_a", "fetch_data_b", "fetch_data_c")
    },
})

// Each fetch task routes to aggregator
g.AddNode(&graph.BaseNode{
    NodeName:        "fetch_data_a",
    DeclaredTargets: []string{"aggregator"},
    Fn:              fetchAFunc,
})
// fetch_data_b and fetch_data_c similar...

g.SetEntryPoint("start")
```

The aggregator waits for all incoming nodes to complete before executing.

---

## Subgraphs {#subgraphs}

Compose complex workflows from reusable graph components:

```go
// Create a research subgraph
researchGraph := createResearchGraph()

var MessagesKey = agent.MessagesKey  // From agent package

// Embed in parent graph
builder.AddNodeFunc("research", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    // Get current messages
    msgs := state.GetFromView(view, MessagesKey)
    
    // Execute subgraph
    messages, err := agent.CollectMessages(researchGraph.Run(ctx, msgs))
    if err != nil {
        return nil, err
    }
    
    return &graph.NodeResult{
        Messages: messages,
    }, nil
})
```

See `examples/subgraph` for a complete demonstration.
