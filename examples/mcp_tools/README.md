# Example: MCP Tools

## Overview
Demonstrates using MCP (Model Context Protocol) tools with AgentMesh. Shows how to integrate standardized tools that work across different LLM frameworks.

## Key Concepts
- **Model Context Protocol**: Standardized tool interface
- **MCP Server Integration**: Connect to MCP-compatible tools
- **Tool Interoperability**: Tools work across frameworks
- **In-Memory Transport**: Testing without network

## Prerequisites
```bash
go get github.com/modelcontextprotocol/go-sdk/mcp
export OPENAI_API_KEY="sk-..."
```

## Running
```bash
cd examples/mcp_tools
go run main.go
```

## Expected Output
```
=== MCP Tools Integration Example ===

Creating in-memory MCP server with tools...
✓ MCP server started
  Tools available: 1
    - sum: Add two numbers

Converting MCP tools to AgentMesh format...
✓ Tools converted: 1

Building ReAct agent with MCP tools...
✓ Agent created

Query: "What is 123 + 456?"

Agent thinking...
[Tool Call] sum(a=123, b=456)
[Tool Result] 579

Agent response:
"The sum of 123 and 456 is 579."
```

## Code Walkthrough

### 1. Define MCP Tool with Types
```go
type sumInput struct {
    A float64 `json:"a" jsonschema:"first operand"`
    B float64 `json:"b" jsonschema:"second operand"`
}

type sumOutput struct {
    Result float64 `json:"result" jsonschema:"sum of a and b"`
}

func sumTool(_ context.Context, _ *mcp.CallToolRequest, in sumInput) (*mcp.CallToolResult, sumOutput, error) {
    return nil, sumOutput{Result: in.A + in.B}, nil
}
```

### 2. Create MCP Server
```go
func newInMemoryMCP() *mcp.InMemoryTransport {
    client, server := mcp.NewInMemoryTransport()
    
    mcpServer := mcp.NewServer(server)
    mcp.AddTool(mcpServer, "sum", "Add two numbers", sumTool)
    
    go mcpServer.Serve()
    
    return client
}
```

### 3. Create MCP Client
```go
mcpTransport := newInMemoryMCP()
mcpClient, _ := mcp.NewClient(mcpTransport)

// Initialize client
_, _ = mcpClient.Initialize(ctx, mcp.InitializeRequest{
    ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
    ClientInfo: mcp.Implementation{
        Name:    "agentmesh-client",
        Version: "1.0.0",
    },
})
```

### 4. Convert MCP Tools to AgentMesh
```go
import mcptool "github.com/hupe1980/agentmesh/pkg/tool/mcp"

toolset, _ := mcptool.NewToolset(mcpClient)
```

### 5. Use with ReAct Agent
```go
reactAgent := agent.NewReAct(model, toolset)

result, _ := reactAgent.Invoke(ctx, []message.Message{
    message.NewUserMessage("What is 123 + 456?"),
})
```

## MCP Tool Definition

### Type-Safe Input/Output
```go
// Input schema
type weatherInput struct {
    Location string `json:"location" jsonschema:"description=City name"`
    Units    string `json:"units" jsonschema:"description=Temperature units,enum=celsius|fahrenheit"`
}

// Output schema
type weatherOutput struct {
    Temperature float64 `json:"temperature"`
    Conditions  string  `json:"conditions"`
}
```

### Tool Handler
```go
func weatherTool(ctx context.Context, req *mcp.CallToolRequest, in weatherInput) (*mcp.CallToolResult, weatherOutput, error) {
    // Implement tool logic
    weather := fetchWeather(in.Location, in.Units)
    
    return nil, weatherOutput{
        Temperature: weather.Temp,
        Conditions:  weather.Conditions,
    }, nil
}
```

### Register Tool
```go
mcp.AddTool(mcpServer, "get_weather", "Get current weather", weatherTool)
```

## What This Example Teaches
- ✅ MCP protocol integration
- ✅ Standardized tool interfaces
- ✅ Type-safe tool definitions
- ✅ Cross-framework compatibility
- ✅ In-memory testing setup

## MCP vs Native Tools

### Native AgentMesh Tool
```go
tool.NewFuncTool("sum", "Add numbers", func(ctx context.Context, args sumArgs) (map[string]any, error) {
    return map[string]any{"result": args.A + args.B}, nil
})
```

### MCP Tool
```go
mcp.AddTool(server, "sum", "Add numbers", sumTool)
// Can be used by any MCP-compatible framework
```

## Production MCP Integration

### External MCP Server
```go
// Connect to remote MCP server
conn, _ := grpc.Dial("mcp-server:50051", grpc.WithInsecure())
mcpClient := mcp.NewClient(conn)
```

### MCP Tool Discovery
```go
// List available tools
toolsResp, _ := mcpClient.ListTools(ctx, &mcp.ListToolsRequest{})

for _, tool := range toolsResp.Tools {
    fmt.Printf("Tool: %s - %s\n", tool.Name, tool.Description)
}
```

### Error Handling
```go
result, out, err := toolHandler(ctx, req, input)
if err != nil {
    return &mcp.CallToolResult{
        IsError: true,
        Content: []mcp.Content{{
            Type: "text",
            Text: fmt.Sprintf("Error: %v", err),
        }},
    }, out, nil
}
```

## Next Steps
- Connect to external MCP servers
- Build custom MCP tools
- Share tools across LLM frameworks
- See **examples/basic_agent** for native tool patterns

## See Also
- [pkg/tool/mcp](../../pkg/tool/mcp) - MCP integration
- [pkg/tool](../../pkg/tool) - Native tool creation
- [MCP Specification](https://modelcontextprotocol.io/)
- [examples/basic_agent](../basic_agent) - Tool basics
