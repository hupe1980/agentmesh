---
layout: doc
title: Agent-to-Agent Protocol
permalink: /a2a/
hero:
  title: Agent-to-Agent (A2A) Protocol
  description: Enable multi-agent collaboration across different systems with the A2A protocol.
  primary_cta:
    label: View example
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples/a2a_integration"
    external: true
  secondary_cta:
    label: API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/a2a"
    external: true
sidebar:
  - title: Overview
    url: "#overview"
  - title: Server setup
    url: "#server-setup"
  - title: Client setup
    url: "#client-setup"
  - title: Use cases
    url: "#use-cases"
  - title: Best practices
    url: "#best-practices"
---

# Agent-to-Agent (A2A) Protocol

The A2A protocol enables AgentMesh agents to communicate with external agents, creating multi-agent systems that span different frameworks and platforms.

---

## Overview {#overview}

AgentMesh provides bidirectional A2A integration:

- **Server** - Expose AgentMesh agents as A2A services
- **Client** - Use external A2A agents as tools in AgentMesh workflows

**Key features**:
- ✅ gRPC and JSON-RPC transport
- ✅ Dynamic tool discovery
- ✅ Automatic schema conversion
- ✅ Streaming support
- ✅ Cross-platform compatibility

---

## Server Setup {#server-setup}

Expose an AgentMesh agent as an A2A service.

### Basic Server

```go
import (
    "net"
    
    "github.com/hupe1980/agentmesh/pkg/a2a"
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/a2aproject/a2a-go/a2agrpc"
    "github.com/a2aproject/a2a-go/a2asrv"
    "google.golang.org/grpc"
)

func main() {
    // 1. Create your AgentMesh agent
    compiled, err := agent.NewReActAgent(
        model,
        tools,
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // 2. Wrap as A2A executor
    executor := a2a.NewExecutor(compiled,
        a2a.WithName("research-agent"),
        a2a.WithDescription("Searches and analyzes research papers"),
    )
    
    // 3. Create A2A request handler
    requestHandler := a2asrv.NewHandler(executor)
    
    // 4. Create gRPC handler
    grpcHandler := a2agrpc.NewHandler(requestHandler)
    
    // 5. Start gRPC server
    listener, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatal(err)
    }
    
    server := grpc.NewServer()
    a2agrpc.RegisterAgentServer(server, grpcHandler)
    
    log.Println("A2A server listening on :50051")
    if err := server.Serve(listener); err != nil {
        log.Fatal(err)
    }
}
```

### Server with Custom Metadata

```go
executor := a2a.NewExecutor(compiled,
    a2a.WithName("data-analyst"),
    a2a.WithDescription("Analyzes datasets and generates insights"),
    a2a.WithVersion("1.0.0"),
    a2a.WithMetadata(map[string]string{
        "provider": "acme-corp",
        "region":   "us-east-1",
        "tier":     "premium",
    }),
)
```

### Server with Tool Filtering

Expose only specific tools via A2A:

```go
executor := a2a.NewExecutor(compiled,
    a2a.WithAllowedTools([]string{
        "search_papers",
        "summarize_paper",
        "extract_citations",
    }),
)
```

---

## Client Setup {#client-setup}

Use external A2A agents as tools in AgentMesh.

### Basic Client

```go
import (
    "github.com/hupe1980/agentmesh/pkg/a2a"
    "github.com/hupe1980/agentmesh/pkg/agent"
)

func main() {
    // 1. Create A2A client
    client := a2a.NewClient("localhost:50051",
        a2a.WithInsecure(), // For development
    )
    defer client.Close()
    
    // 2. Create bridge to convert A2A agent to tools
    bridge := a2a.NewBridge(client)
    
    // 3. Discover available tools
    tools, err := bridge.GetTools(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Discovered %d tools from A2A agent", len(tools))
    
    // 4. Use tools in your agent
    compiled, err := agent.NewReActAgent(
        localModel,
        tools,  // Tools from remote A2A agent
    )
    
    // 5. Execute workflow that uses remote tools
    messages, err := graph.Collect(compiled.Run(ctx, messages))
    if err != nil {
        log.Fatal(err)
    }
}
```

### Multiple A2A Agents

Combine tools from multiple A2A services:

```go
// Connect to multiple A2A agents
researchClient := a2a.NewClient("research-agent:50051")
dataClient := a2a.NewClient("data-agent:50051")
weatherClient := a2a.NewClient("weather-agent:50051")

// Get tools from each
researchTools, _ := a2a.NewBridge(researchClient).GetTools(ctx)
dataTools, _ := a2a.NewBridge(dataClient).GetTools(ctx)
weatherTools, _ := a2a.NewBridge(weatherClient).GetTools(ctx)

// Combine all tools
allTools := append(append(researchTools, dataTools...), weatherTools...)

// Create agent with all remote tools
compiled, _ := agent.NewReActAgent(model, allTools)
```

### Tool Filtering

Use only specific tools from an A2A agent:

```go
bridge := a2a.NewBridge(client)

// Get all tools
allTools, _ := bridge.GetTools(ctx)

// Filter to specific tools
selectedTools := filterTools(allTools, []string{
    "search_papers",
    "summarize_paper",
})

compiled, _ := agent.NewReActAgent(model, selectedTools)
```

---

## Use Cases {#use-cases}

### 1. Specialized Agent Orchestration

Coordinator agent that delegates to specialized A2A agents:

```go
func createCoordinator() (*graph.Compiled, error) {
    // Connect to specialized agents
    researchTools, _ := a2a.NewBridge(
        a2a.NewClient("research-agent:50051"),
    ).GetTools(ctx)
    
    dataTools, _ := a2a.NewBridge(
        a2a.NewClient("data-agent:50051"),
    ).GetTools(ctx)
    
    writingTools, _ := a2a.NewBridge(
        a2a.NewClient("writing-agent:50051"),
    ).GetTools(ctx)
    
    // Create coordinator with all specialized tools
    return agent.NewReActAgent(
        model,
        append(append(researchTools, dataTools...), writingTools...),
    )
}

// Usage
coordinator, _ := createCoordinator()
messages, err := graph.Collect(coordinator.Run(ctx, []message.Message{
    message.NewHumanMessageFromText("Research AI trends, analyze the data, and write a report"),
}))
if err != nil {
    log.Fatal(err)
}
```

### 2. Cross-Framework Integration

Use Python LangChain agents from Go:

```python
# Python: LangChain agent exposed via A2A
from langchain_a2a import A2AServer
from langchain.agents import create_react_agent

agent = create_react_agent(llm, tools)
server = A2AServer(agent, port=50051)
server.serve()
```

```go
// Go: AgentMesh using the Python agent
client := a2a.NewClient("python-agent:50051")
tools, _ := a2a.NewBridge(client).GetTools(ctx)

compiled, _ := agent.NewReActAgent(model, tools)
```

### 3. Microservices Architecture

Each service exposes its capabilities as an A2A agent:

```
┌──────────────────┐
│  API Gateway     │
│  (AgentMesh)     │
└────────┬─────────┘
         │
    ┌────┴────┐
    │   A2A   │
    └────┬────┘
         │
    ┌────┴──────────────────────────┐
    │                               │
┌───▼─────────┐            ┌───────▼──────┐
│ Customer    │            │ Billing      │
│ Service     │            │ Service      │
│ (A2A Agent) │            │ (A2A Agent)  │
└─────────────┘            └──────────────┘
```

```go
// Gateway agent combines all services
customerTools, _ := a2a.NewBridge(a2a.NewClient("customer-svc:50051")).GetTools(ctx)
billingTools, _ := a2a.NewBridge(a2a.NewClient("billing-svc:50051")).GetTools(ctx)
inventoryTools, _ := a2a.NewBridge(a2a.NewClient("inventory-svc:50051")).GetTools(ctx)

allTools := append(append(customerTools, billingTools...), inventoryTools...)

gateway, _ := agent.NewReActAgent(model, allTools)
```

### 4. Agent Marketplace

Discover and use agents from a registry:

```go
type AgentRegistry struct {
    agents map[string]string // name -> address
}

func (r *AgentRegistry) GetAgent(name string) ([]tool.Tool, error) {
    address, ok := r.agents[name]
    if !ok {
        return nil, fmt.Errorf("agent not found: %s", name)
    }
    
    client := a2a.NewClient(address)
    bridge := a2a.NewBridge(client)
    return bridge.GetTools(ctx)
}

// Usage
registry := &AgentRegistry{
    agents: map[string]string{
        "research":   "research-agent.example.com:50051",
        "translation": "translation-agent.example.com:50051",
        "data-viz":   "dataviz-agent.example.com:50051",
    },
}

// Dynamically compose agent based on task
task := "research and translate"
tools := []tool.Tool{}

if strings.Contains(task, "research") {
    t, _ := registry.GetAgent("research")
    tools = append(tools, t...)
}
if strings.Contains(task, "translate") {
    t, _ := registry.GetAgent("translation")
    tools = append(tools, t...)
}

compiled, _ := agent.NewReActAgent(model, tools)
```

### 5. Testing & Mocking

Mock external agents for testing:

```go
// Production: use real A2A agent
func createAgent(useMock bool) (*graph.Compiled, error) {
    var tools []tool.Tool
    
    if useMock {
        // Mock tools for testing
        tools = []tool.Tool{createMockTool("search_papers")}
    } else {
        // Real A2A client
        client := a2a.NewClient("research-agent:50051")
        tools, _ = a2a.NewBridge(client).GetTools(ctx)
    }
    
    return agent.NewReActAgent(model, tools)
}

// Test
func TestAgentWorkflow(t *testing.T) {
    agent, _ := createAgent(true) // Use mock
    messages, err := graph.Collect(agent.Run(ctx, messages))
    assert.NoError(t, err)
}
```

---

## Best Practices {#best-practices}

### 1. Handle Connection Failures

```go
client := a2a.NewClient("remote-agent:50051",
    a2a.WithTimeout(5*time.Second),
    a2a.WithRetry(3),
)

// Test connection before using
if err := client.Ping(ctx); err != nil {
    log.Printf("A2A agent unavailable: %v", err)
    // Fallback to local tools
}
```

### 2. Cache Tool Schemas

```go
type CachedBridge struct {
    bridge a2a.Bridge
    cache  map[string][]tool.Tool
    mu     sync.RWMutex
}

func (c *CachedBridge) GetTools(ctx context.Context) ([]tool.Tool, error) {
    c.mu.RLock()
    if tools, ok := c.cache["tools"]; ok {
        c.mu.RUnlock()
        return tools, nil
    }
    c.mu.RUnlock()
    
    tools, err := c.bridge.GetTools(ctx)
    if err != nil {
        return nil, err
    }
    
    c.mu.Lock()
    c.cache["tools"] = tools
    c.mu.Unlock()
    
    return tools, nil
}
```

### 3. Use TLS in Production

```go
// Server
creds, err := credentials.NewServerTLSFromFile("server.crt", "server.key")
server := grpc.NewServer(grpc.Creds(creds))

// Client
creds, err := credentials.NewClientTLSFromFile("ca.crt", "")
client := a2a.NewClient("agent:50051",
    a2a.WithTransportCredentials(creds),
)
```

### 4. Implement Health Checks

```go
// Server: implement health check
type HealthCheckExecutor struct {
    executor a2a.Executor
}

func (h *HealthCheckExecutor) Execute(ctx context.Context, req *a2a.Request) (*a2a.Response, error) {
    if req.Task == "health_check" {
        return &a2a.Response{
            Status: "healthy",
            Metadata: map[string]string{
                "uptime": time.Since(startTime).String(),
            },
        }, nil
    }
    return h.executor.Execute(ctx, req)
}

// Client: periodic health checks
go func() {
    ticker := time.NewTicker(30 * time.Second)
    for range ticker.C {
        if err := client.Ping(ctx); err != nil {
            log.Printf("Health check failed: %v", err)
            metrics.RecordAgentDown("remote-agent")
        }
    }
}()
```

### 5. Monitor Performance

```go
type InstrumentedBridge struct {
    bridge a2a.Bridge
}

func (i *InstrumentedBridge) GetTools(ctx context.Context) ([]tool.Tool, error) {
    start := time.Now()
    tools, err := i.bridge.GetTools(ctx)
    
    metrics.RecordA2ACall("get_tools", time.Since(start), err)
    
    return tools, err
}
```

### 6. Version Compatibility

```go
client := a2a.NewClient("agent:50051")

// Check version compatibility
info, err := client.GetInfo(ctx)
if info.Version != "1.0.0" {
    log.Printf("Warning: version mismatch (expected 1.0.0, got %s)", info.Version)
}

// Use tools with version awareness
tools, _ := a2a.NewBridge(client).GetTools(ctx)
```

---

## Advanced Topics

### Streaming Responses

```go
executor := a2a.NewExecutor(compiled,
    a2a.WithStreaming(true),
)

// Client receives streaming updates
stream, err := client.ExecuteStreaming(ctx, request)
for {
    chunk, err := stream.Recv()
    if err == io.EOF {
        break
    }
    fmt.Printf("Received: %s\n", chunk.Content)
}
```

### Custom Authentication

```go
// Server: validate tokens
interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    token := extractToken(ctx)
    if !validateToken(token) {
        return nil, status.Error(codes.Unauthenticated, "invalid token")
    }
    return handler(ctx, req)
}

server := grpc.NewServer(grpc.UnaryInterceptor(interceptor))

// Client: add token
client := a2a.NewClient("agent:50051",
    a2a.WithPerRPCCredentials(&tokenAuth{token: "secret"}),
)
```

---

## Examples

- **[a2a_integration](https://github.com/hupe1980/agentmesh/tree/main/examples/a2a_integration)** - Complete A2A server and client examples

---

## See Also

- [Tools Guide](/tools/) - Creating and using tools
- [Agents Guide](/agents/) - Building ReAct agents
- [API Reference](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/a2a)
- [A2A Protocol Specification](https://github.com/a2aproject/a2a-spec)
