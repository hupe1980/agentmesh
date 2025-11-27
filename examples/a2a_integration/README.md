# Example: A2A Integration

## Overview
Demonstrates Agent-to-Agent (A2A) Protocol integration. Shows how to expose AgentMesh agents as A2A services and consume external A2A agents as tools.

## Key Concepts
- **A2A Protocol**: Standardized agent communication
- **AgentCard**: Agent capability advertisement
- **A2A Server**: Expose agents via gRPC
- **A2A Client**: Consume external agents
- **Agent Composition**: Build multi-agent systems

## Structure
```
a2a_integration/
├── server/     - A2A server exposing AgentMesh agent
├── client/     - A2A client consuming external agents
└── tool_usage/ - Using A2A agents as tools
```

## Running

### 1. Start A2A Server
```bash
cd examples/a2a_integration/server
go run main.go
# Server starts on :9000 (gRPC)
# AgentCard served on :9001 (HTTP)
```

### 2. Run A2A Client
```bash
cd examples/a2a_integration/client
go run main.go --card-url=http://127.0.0.1:9001
```

### 3. Use as Tool
```bash
cd examples/a2a_integration/tool_usage
export OPENAI_API_KEY="sk-..."
go run main.go
```

## Expected Output

### Server
```
=== A2A Server Example ===

Starting AgentMesh A2A Server...
✓ gRPC server listening on :9000
✓ AgentCard server listening on :9001

Agent capabilities:
  Name: Weather Assistant
  Skills: 2
    - get_weather: Get current weather
    - forecast: Get weather forecast

Waiting for requests...
```

### Client
```
=== A2A Client Example ===

Resolving AgentCard from http://127.0.0.1:9001...
✓ Found agent: Weather Assistant
  Description: Provides weather information
  Skills: 2 available
    1. get_weather: Get current weather
    2. forecast: Get weather forecast

Invoking skill: get_weather
  Input: {"location": "Paris"}

Response:
  Success: true
  Output: {
    "temperature": 21.5,
    "conditions": "Partly cloudy",
    "location": "Paris"
  }
```

### Tool Usage
```
=== A2A Tool Integration Example ===

Converting A2A agent to AgentMesh tool...
✓ Tool created: weather_agent
  Skills available: 2

Building ReAct agent with A2A tool...
✓ Agent ready

Query: "What's the weather in Tokyo?"

Agent response:
"The weather in Tokyo is partly cloudy with a temperature of 21.5°C."

[Tool Call] weather_agent.get_weather(location="Tokyo")
```

## Code Walkthrough

### Server: Expose AgentMesh Agent

#### 1. Create Agent
```go
weatherAgent := agent.NewReAct(model, weatherToolset)
```

#### 2. Create A2A Bridge
```go
import "github.com/hupe1980/agentmesh/pkg/a2a"

bridge := a2a.NewBridge(weatherAgent,
    a2a.WithName("Weather Assistant"),
    a2a.WithDescription("Provides weather information"),
)
```

#### 3. Start Servers
```go
// Start gRPC server
server := a2a.NewServer(bridge)
go server.Serve(":9000")

// Start AgentCard HTTP server
cardServer := a2a.NewCardServer(bridge)
go cardServer.Serve(":9001")
```

### Client: Consume External A2A Agent

#### 1. Resolve AgentCard
```go
import (
    "github.com/a2aproject/a2a-go/a2aclient/agentcard"
)

card, _ := agentcard.DefaultResolver.Resolve(ctx, "http://localhost:9001")
```

#### 2. Create A2A Client
```go
import "github.com/a2aproject/a2a-go/a2aclient"

conn, _ := grpc.Dial("localhost:9000", grpc.WithInsecure())
client := a2aclient.NewClient(conn, card)
```

#### 3. Send Messages
```go
msg := a2atypes.NewMessage(a2atypes.MessageRoleUser, 
    a2atypes.TextPart{Text: "Get weather for Paris"})

resp, _ := client.SendMessage(ctx, &a2atypes.MessageSendParams{
    Message: msg,
})

// Handle response (can be Task or Message)
switch r := resp.(type) {
case *a2atypes.Task:
    // Extract from artifacts, status, or history
    var msg *a2atypes.Message
    if len(r.Artifacts) > 0 {
        // Use artifact parts
    } else if r.Status.Message != nil {
        msg = r.Status.Message
    } else if len(r.History) > 0 {
        msg = r.History[len(r.History)-1]
    }
case *a2atypes.Message:
    // Direct message response
}
```

#### 4. Stream Messages
```go
stream := client.SendStreamingMessage(ctx, &a2atypes.MessageSendParams{
    Message: msg,
})

for event, err := range stream {
    if err != nil {
        // Handle error
        break
    }
    
    // Handle different event types
    switch e := event.(type) {
    case *a2atypes.Task:
        // Task snapshot with artifacts/status/history
    case *a2atypes.Message:
        // Direct message event
    case *a2atypes.TaskStatusUpdateEvent:
        // Status update (may have embedded message)
    case *a2atypes.TaskArtifactUpdateEvent:
        // Artifact chunk (streaming content)
    }
}
```

### Tool Usage: A2A Agent as Tool

#### 1. Create A2A Tool
```go
import (
    "github.com/hupe1980/agentmesh/pkg/a2a"
    a2atool "github.com/hupe1980/agentmesh/pkg/tool/a2a"
)

// Create A2A client using AgentMesh wrapper
a2aClient, _ := a2a.NewClient(ctx, "http://localhost:9001", "react")

// Wrap as tool
a2aTool, _ := a2atool.NewTool(ctx, "http://localhost:9001", "react")
```

#### 2. Add to Toolset
```go
toolset := tool.NewToolset()
toolset.Add(a2aTool)
```

#### 3. Use with Agent
```go
reactAgent := agent.NewReAct(model, toolset)
```

## What This Example Teaches
- ✅ A2A protocol integration
- ✅ Exposing agents as services
- ✅ Consuming external agents
- ✅ Multi-agent composition
- ✅ Agent interoperability
- ✅ Proper A2A response handling (Task vs Message)
- ✅ Streaming with event type handling
- ✅ Artifact, Status, and History extraction

## AgentCard Structure

```json
{
  "name": "Weather Assistant",
  "description": "Provides weather information",
  "url": "http://localhost:9000",
  "skills": [
    {
      "name": "get_weather",
      "description": "Get current weather",
      "input_schema": {...},
      "output_schema": {...}
    }
  ]
}
```

## Production Considerations

### Security
```go
// Use TLS for production
creds := credentials.NewServerTLSFromFile("cert.pem", "key.pem")
server := grpc.NewServer(grpc.Creds(creds))
```

### Load Balancing
```go
// Multiple A2A server instances
servers := []string{
    "agent1.example.com:9000",
    "agent2.example.com:9000",
}
// Use load balancer or client-side balancing
```

### Service Discovery
```go
// Register with service registry
// Use Consul, etcd, or Kubernetes service discovery
```

## Next Steps
- Build multi-agent systems
- Implement custom A2A skills
- Add authentication and authorization
- See **examples/basic_agent** for single agent patterns

## See Also
- [pkg/a2a](../../pkg/a2a) - A2A integration
- [pkg/tool/a2a](../../pkg/tool/a2a) - A2A tool adapter
- [A2A Protocol Spec](https://github.com/a2aproject/a2a)
- [examples/basic_agent](../basic_agent) - Agent basics
