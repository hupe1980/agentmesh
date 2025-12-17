# Example: Basic Agent

## Overview
Demonstrates the fundamentals of building a ReAct (Reasoning + Acting) agent with tool integration. This is the recommended starting point for learning AgentMesh.

## Key Concepts
- **ReAct Agent**: Combines reasoning (thinking) with acting (tool use)
- **Function Tools**: Create tools from Go functions with automatic JSON schema generation
- **Tool Integration**: Agent decides when to call tools based on conversation context
- **Message Transcript**: Access complete execution history including tool calls

## Prerequisites
```bash
export OPENAI_API_KEY="sk-..."
```

## Running
```bash
cd examples/basic_agent
go run main.go
```

## Expected Output
```
Creating weather tool from function...
✓ Tool created: get_weather
  Description: Get current weather for a location
  Schema: {"location":"string"}

Building ReAct agent with tool support...
✓ Agent created with 1 tool

Asking: What's the weather like in Paris and Tokyo?

Agent Response:
The weather in Paris is partly cloudy with a temperature of 21.5°C.
The weather in Tokyo is partly cloudy with a temperature of 21.5°C.

Tool Calls Made: 2
  1. get_weather(location="Paris")
  2. get_weather(location="Tokyo")
```

## Code Walkthrough

### 1. Define Tool Parameters
```go
type weatherArgs struct {
    Location string `json:"location" jsonschema:"description=The city or location"`
}
```

### 2. Implement Tool Function
```go
func mockWeatherLookup(_ context.Context, args weatherArgs) (map[string]any, error) {
    return map[string]any{
        "location": args.Location,
        "conditions": "Partly cloudy",
        "temperature_c": 21.5,
    }, nil
}
```

### 3. Create Tool with Schema Generation
```go
weatherTool := tool.NewFuncTool(
    "get_weather",
    "Get current weather for a location",
    mockWeatherLookup,
)
```

### 4. Build ReAct Agent
```go
reactAgent := agent.NewReAct(model, toolset)
```

### 5. Run with Messages
```go
result, _ := graph.Last(reactAgent.Run(ctx, []message.Message{
    message.NewUserMessage("What's the weather in Paris?"),
}))
```

## What This Example Teaches
- ✅ How to create tools from Go functions
- ✅ Automatic JSON schema generation from struct tags
- ✅ ReAct agent reasoning and tool selection
- ✅ Processing tool calls and results
- ✅ Message history inspection

## Next Steps
- Try modifying the tool to make real API calls
- Add more tools (calculator, database lookup, etc.)
- Experiment with different prompts
- See **examples/observability** for monitoring and tracing
- See **examples/circuit_breaker** for production resilience patterns

## See Also
- [pkg/agent](../../pkg/agent) - Agent implementations
- [pkg/tool](../../pkg/tool) - Tool creation and integration
- [examples/mcp_tools](../mcp_tools) - Model Context Protocol integration
- [examples/custom_observability](../custom_observability) - Advanced callback patterns
