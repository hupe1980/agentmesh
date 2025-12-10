---
layout: doc
title: Testing
permalink: /testing/
hero:
  title: Testing AgentMesh Applications
  description: Mock implementations, builders, and assertion helpers for testing AI agents and workflows.
sidebar:
  - title: Overview
    url: "#overview"
  - title: Mock Model
    url: "#mock-model"
  - title: Mock Tool
    url: "#mock-tool"
  - title: Conversation Recorder
    url: "#conversation-recorder"
  - title: Assertions
    url: "#assertions"
  - title: Scenarios
    url: "#scenarios"
  - title: Other Mocks
    url: "#other-mocks"
  - title: Best Practices
    url: "#best-practices"
---

## Overview {#overview}

The `pkg/testutil` package provides comprehensive testing utilities for AgentMesh applications:

- **MockModel** - Configurable mock LLM with builder pattern
- **MockTool** - Configurable mock tools with sequential results
- **ConversationRecorder** - Capture and assert on model interactions
- **Assertions** - Domain-specific matchers for messages and graphs
- **Scenarios** - Pre-built test configurations for common patterns

### Installation

The testutil package is included with AgentMesh:

```go
import "github.com/hupe1980/agentmesh/pkg/testutil"
```

---

## Mock Model {#mock-model}

### Basic Usage

Create a mock model that returns a simple response:

```go
model := testutil.NewModelBuilder().
    WithResponse("Hello, world!").
    Build()
```

### Sequential Responses

Configure multiple responses for multi-turn conversations:

```go
model := testutil.NewModelBuilder().
    WithResponses("First response", "Second response", "Third response").
    Build()
```

The model returns responses in order, repeating the last one if exhausted.

### Tool Calls

Simulate the model requesting tool execution:

```go
model := testutil.NewModelBuilder().
    WithToolCalls(message.ToolCall{
        ID:        "call_1",
        Name:      "search",
        Type:      "function",
        Arguments: `{"query": "weather in NYC"}`,
    }).
    WithResponse("Based on the search results, it's sunny in NYC.").
    Build()
```

### Error Simulation

Test error handling:

```go
model := testutil.NewModelBuilder().
    WithError(errors.New("API rate limit exceeded")).
    Build()
```

### Delay Simulation

Test timeout handling:

```go
model := testutil.NewModelBuilder().
    WithResponse("delayed response").
    WithDelay(500 * time.Millisecond).
    Build()
```

### Streaming Mode

Enable streaming for chunk-by-chunk responses:

```go
model := testutil.NewModelBuilder().
    WithResponse("This will be streamed in chunks").
    WithStreaming(true).
    Build()
```

### Custom Capabilities

Configure model capabilities:

```go
model := testutil.NewModelBuilder().
    WithCapabilities(model.Capabilities{
        Streaming:        true,
        Tools:            true,
        StructuredOutput: true,
        MaxContextTokens: 128000,
    }).
    WithResponse("response").
    Build()
```

### Custom Generator

For advanced scenarios, provide a custom generate function:

```go
model := testutil.NewModelBuilder().
    WithGenerator(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
        return func(yield func(*model.Response, error) bool) {
            // Custom logic based on request
            msg := message.NewAIMessageFromText("custom response")
            yield(&model.Response{Message: msg}, nil)
        }
    }).
    Build()
```

### Struct Literal Syntax

For simple cases, you can also use struct literals:

```go
model := &testutil.MockModel{
    GenerateFunc: testutil.WrapSimpleGenerate(
        func(ctx context.Context, msgs []message.Message) (message.Message, error) {
            return message.NewAIMessageFromText("hello"), nil
        },
    ),
}
```

---

## Mock Tool {#mock-tool}

### Basic Usage

Create a mock tool with a static result:

```go
tool := testutil.NewToolBuilder("search").
    WithDescription("Search the web").
    WithResult("search results for your query").
    Build()
```

### Sequential Results

Return different results on each call:

```go
tool := testutil.NewToolBuilder("counter").
    WithResults("1", "2", "3").
    Build()
```

### Error Simulation

Test tool error handling:

```go
tool := testutil.NewToolBuilder("flaky_api").
    WithError(errors.New("connection timeout")).
    Build()
```

### Custom Call Function

Implement dynamic behavior:

```go
tool := testutil.NewToolBuilder("calculator").
    WithCall(func(ctx context.Context, args string) (any, error) {
        var input struct {
            Expression string `json:"expression"`
        }
        json.Unmarshal([]byte(args), &input)
        // Evaluate expression...
        return "42", nil
    }).
    Build()
```

### Typed JSON Handler

Use generics for type-safe argument parsing:

```go
type SearchArgs struct {
    Query string `json:"query"`
    Limit int    `json:"limit"`
}

tool := testutil.NewToolBuilder("search").
    WithCall(testutil.WithJSONHandler(func(ctx context.Context, args SearchArgs) (any, error) {
        return fmt.Sprintf("Results for: %s (limit: %d)", args.Query, args.Limit), nil
    })).
    Build()
```

### Tool Schema

Provide a JSON schema for the tool:

```go
tool := testutil.NewToolBuilder("search").
    WithDefinition(&tool.Definition{
        Name:        "search",
        Description: "Search the web",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "query": map[string]any{"type": "string"},
            },
            "required": []string{"query"},
        },
    }).
    WithResult("results").
    Build()
```

---

## Conversation Recorder {#conversation-recorder}

The `ConversationRecorder` captures all model requests and responses for analysis:

### Setup

```go
recorder := testutil.NewConversationRecorder()

model := testutil.NewModelBuilder().
    WithRecorder(recorder).
    WithResponse("Hello!").
    Build()

// Run your agent with this model...
```

### Assertions

```go
// Assert on request/response counts
recorder.AssertRequestCount(t, 1)
recorder.AssertResponseCount(t, 1)

// Assert content was present in conversation
recorder.AssertContains(t, "user query")
recorder.AssertContains(t, "Hello")

// Assert tool calls were made
recorder.AssertToolCallMade(t, "search")
recorder.AssertToolCallMade(t, "calculator")
```

### Access Raw Data

```go
// Get all requests
requests := recorder.Requests()
for _, req := range requests {
    fmt.Printf("Request with %d messages\n", len(req.Messages))
}

// Get all responses
responses := recorder.Responses()
for _, resp := range responses {
    fmt.Printf("Response: %s\n", resp.Message.String())
}

// Get all tool calls made
toolCalls := recorder.ToolCalls()
for _, tc := range toolCalls {
    fmt.Printf("Tool: %s, Args: %s\n", tc.Name, tc.Arguments)
}
```

### Reset Between Tests

```go
recorder.Reset()
```

---

## Assertions {#assertions}

### Message Matchers

Assert on message sequences with type-safe matchers:

```go
testutil.AssertMessages(t, messages,
    testutil.IsHuman("What's the weather?"),
    testutil.IsAI(testutil.HasToolCall("get_weather")),
    testutil.IsTool(testutil.Contains("sunny")),
    testutil.IsAI(testutil.Contains("The weather is sunny")),
)
```

### Available Matchers

```go
// Match by message type and content
testutil.IsHuman("exact text")
testutil.IsAI(matcher)
testutil.IsTool(matcher)
testutil.IsSystem(matcher)

// Content matchers
testutil.Contains("substring")
testutil.HasToolCall("tool_name")
testutil.MatchesRegex(`\d+ degrees`)
```

### Async Assertions

For testing concurrent operations:

```go
// Assert condition becomes true within timeout
testutil.AssertEventually(t, func() bool {
    return recorder.RequestCount() >= 3
}, 5*time.Second, "expected at least 3 requests")

// Assert condition never becomes true
testutil.AssertNever(t, func() bool {
    return hasError
}, 100*time.Millisecond, "should not error")
```

---

## Scenarios {#scenarios}

Pre-built test scenarios for common patterns:

### Simple Response

```go
scenario := testutil.SimpleResponseScenario("Hello, how can I help?")

// Use scenario.Model in your agent
agent, _ := agent.NewReAct(scenario.Model)

// Assert using scenario.Recorder
scenario.Recorder.AssertRequestCount(t, 1)
```

### Tool Calling

```go
scenario := testutil.ToolCallingScenario(
    "search",                    // tool name
    "Found 10 results",          // tool result
    "Based on the results...",   // final response
)

// scenario.Model will first request tool call, then respond
// scenario.Tools contains the mock tool
```

### Error Handling

```go
scenario := testutil.ErrorScenario(errors.New("API unavailable"))

// Test that your agent handles errors gracefully
```

### Retry Pattern

```go
scenario := testutil.RetryScenario(
    2,                           // fail count
    errors.New("temporary"),     // error to return
    "success after retries",     // final response
)

// Model fails twice, then succeeds
```

### Timeout

```go
scenario := testutil.TimeoutScenario(100 * time.Millisecond)

// Test context cancellation handling
```

### Chained Tool Calls

```go
scenario := testutil.ChainedToolCallsScenario(
    []message.ToolCall{
        {Name: "search", Arguments: `{"q":"weather"}`},
        {Name: "format", Arguments: `{"data":"..."}`},
    },
    []string{"raw data", "formatted"},
    "Final answer based on formatted data",
)
```

---

## Other Mocks {#other-mocks}

### MockMemory

```go
mem := &testutil.MockMemory{
    AddFunc: func(ctx context.Context, msgs ...message.Message) error {
        return nil
    },
    MessagesFunc: func(ctx context.Context) ([]message.Message, error) {
        return []message.Message{
            message.NewHumanMessage("previous"),
        }, nil
    },
}
```

### MockCheckpointer

```go
cp := &testutil.MockCheckpointer{
    SaveFunc: func(ctx context.Context, state []byte, metadata map[string]any) error {
        return nil
    },
    LoadFunc: func(ctx context.Context) ([]byte, map[string]any, error) {
        return savedState, metadata, nil
    },
}
```

### MockEmbedder

```go
embedder := &testutil.MockEmbedder{
    EmbedFunc: func(ctx context.Context, texts []string) ([][]float32, error) {
        // Return mock embeddings
        embeddings := make([][]float32, len(texts))
        for i := range texts {
            embeddings[i] = []float32{0.1, 0.2, 0.3}
        }
        return embeddings, nil
    },
}
```

### MockGraph and MockNode

For testing Pregel-based workflows:

```go
graph := testutil.NewGraphBuilder().
    AddNode("start", func(ctx context.Context, state map[string]any) (map[string]any, error) {
        state["processed"] = true
        return state, nil
    }).
    AddEdge("start", "end").
    Build()
```

---

## Best Practices {#best-practices}

### 1. Use Builders for Readability

```go
// ✅ Good: Clear intent
model := testutil.NewModelBuilder().
    WithToolCalls(searchCall).
    WithResponse("Based on search...").
    Build()

// ❌ Avoid: Harder to understand
model := &testutil.MockModel{
    GenerateFunc: func(...) { /* complex logic */ },
}
```

### 2. Use Recorders for Verification

```go
recorder := testutil.NewConversationRecorder()
model := testutil.NewModelBuilder().
    WithRecorder(recorder).
    WithResponse("test").
    Build()

// ... run test ...

// Verify behavior, not just output
recorder.AssertToolCallMade(t, "expected_tool")
```

### 3. Use Scenarios for Common Patterns

```go
// ✅ Good: Reusable, self-documenting
scenario := testutil.ToolCallingScenario("search", "results", "answer")
agent, _ := agent.NewReAct(scenario.Model, agent.WithTools(scenario.Tools...))

// ❌ Avoid: Repeating setup in every test
```

### 4. Test Error Paths

```go
t.Run("handles API errors", func(t *testing.T) {
    scenario := testutil.ErrorScenario(errors.New("rate limited"))
    agent, _ := agent.NewReAct(scenario.Model)
    
    _, err := graph.Collect(agent.Run(ctx, messages))
    require.Error(t, err)
})
```

### 5. Test Timeouts

```go
t.Run("respects context timeout", func(t *testing.T) {
    scenario := testutil.TimeoutScenario(1 * time.Second)
    
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    
    _, err := graph.Collect(agent.Run(ctx, messages))
    require.ErrorIs(t, err, context.DeadlineExceeded)
})
```

### 6. Use Table-Driven Tests

```go
tests := []struct {
    name     string
    scenario *testutil.Scenario
    wantErr  bool
}{
    {"success", testutil.SimpleResponseScenario("ok"), false},
    {"error", testutil.ErrorScenario(errors.New("fail")), true},
    {"retry", testutil.RetryScenario(1, errors.New("temp"), "ok"), false},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        agent, _ := agent.NewReAct(tt.scenario.Model)
        _, err := graph.Collect(agent.Run(ctx, messages))
        if tt.wantErr {
            require.Error(t, err)
        } else {
            require.NoError(t, err)
        }
    })
}
```

---

## API Reference

For complete API documentation, see [pkg.go.dev/github.com/hupe1980/agentmesh/pkg/testutil](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/testutil).
