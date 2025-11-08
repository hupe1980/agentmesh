---
layout: doc
title: Models
description: Connect LLM providers through a unified interface with streaming and tool calling support.
permalink: /models/
hero:
  title: Connect language models
  description: Integrate OpenAI, Anthropic, LangChainGo, or custom providers with consistent streaming and tool calling.
  primary_cta:
    label: Choose an adapter
    href: "#available-models"
  secondary_cta:
    label: Model API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/model"
    external: true
sidebar:
  - title: Overview
    url: "#overview"
  - title: Available models
    url: "#available-models"
    children:
      - title: OpenAI
        url: "#openai"
      - title: Anthropic
        url: "#anthropic"
      - title: LangChainGo
        url: "#langchaingo"
  - title: Tool binding
    url: "#tool-binding"
  - title: Streaming
    url: "#streaming"
  - title: Custom models
    url: "#custom-models"
---

## Overview {#overview}

AgentMesh abstracts language models behind a common `model.Model` interface that uses Go 1.23+ iterators for unified streaming:

```go
type Model interface {
    Generate(ctx context.Context, messages []message.Message) iter.Seq2[message.Message, error]
}
```

The iterator-based API unifies streaming and blocking modes:
- **Streaming**: Iterate over partial messages as they arrive
- **Blocking**: Use `model.Last()` to get only the final response
- **Batch collection**: Use `model.Collect()` to gather all messages

Models may also implement `model.ToolAware` to support function calling:

```go
type ToolAware interface {
    BindTools(tools ...tool.Tool) Model
}
```

This design keeps agent code portable across providers while supporting provider-specific features through optional interfaces.

---

## Available models {#available-models}

### OpenAI {#openai}

The OpenAI adapter wraps the official [`openai-go`](https://github.com/openai/openai-go) SDK for Chat Completions:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
)

model := openai.NewModel(
    openai.WithModel("gpt-4o"),
    openai.WithTemperature(0.7),
    openai.WithMaxCompletionTokens(1000),
)

compiled, err := agent.NewReActAgent(model, tools)
```

Configuration options:

```go
openai.NewModel(
    openai.WithModel("gpt-4o-mini"),           // Model name
    openai.WithTemperature(0.2),               // Randomness (0-2)
    openai.WithMaxCompletionTokens(500),       // Max output tokens
)
```

The adapter supports:
- ✅ Streaming responses
- ✅ Function calling via `BindTools()`
- ✅ Parallel tool calls
- ✅ Vision models (pass `message.ImagePart`)

### Anthropic {#anthropic}

The Anthropic adapter integrates Claude models via the official SDK:

```go
import "github.com/hupe1980/agentmesh/pkg/model/anthropic"

model := anthropic.NewModel(
    anthropic.WithModel("claude-3-5-sonnet-20241022"),
    anthropic.WithMaxTokens(1024),
    anthropic.WithTemperature(1.0),
)

compiled, err := agent.NewReActAgent(model, tools)
```

Configuration options:

```go
anthropic.NewModel(
    anthropic.WithModel("claude-3-5-sonnet-20241022"),
    anthropic.WithMaxTokens(2048),
    anthropic.WithTemperature(0.5),
    anthropic.WithAPIKey("your-api-key"), // Optional if set in env
)
```

The adapter supports:
- ✅ Streaming responses  
- ✅ Function calling via `BindTools()`
- ✅ Vision models
- ✅ System prompts

### LangChainGo {#langchaingo}

Wrap any LangChainGo LLM to reuse existing integrations:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/model/langchaingo"
    "github.com/tmc/langchaingo/llms/openai"
)

llm, _ := openai.New(openai.WithModel("gpt-4"))
model := langchaingo.NewModel(llm)

compiled, err := agent.NewReActAgent(model, tools)
```

This adapter enables:
- Integration with LangChainGo's 50+ model providers
- Reuse of existing LangChainGo configurations
- Gradual migration from LangChainGo to AgentMesh

---

## Tool binding {#tool-binding}

Models that support function calling implement `model.ToolAware`:

```go
// Create tools
searchTool, _ := tool.NewFuncTool("search", "Search the web", searchFunc)
calcTool, _ := tool.NewFuncTool("calculator", "Perform calculations", calcFunc)

// Bind tools to model
model := openai.NewModel()
if toolAware, ok := model.(model.ToolAware); ok {
    model = toolAware.BindTools(searchTool, calcTool)
}
```

Agent constructors handle tool binding automatically:

```go
// Tools are bound automatically
compiled, err := agent.NewReActAgent(
    openai.NewModel(),
    []tool.Tool{searchTool, calcTool},
)
```

---

## Streaming {#streaming}

All models support streaming through the unified iterator API:

```go
// Stream model responses directly
for msg, err := range model.Generate(ctx, messages) {
    if err != nil {
        log.Printf("Error: %v", err)
        break
    }
    
    // Print partial responses as they arrive
    for _, part := range msg.Parts() {
        if text, ok := part.(message.TextPart); ok {
            fmt.Print(text.Text)
        }
    }
}
```

For blocking (non-streaming) mode, use `model.Last()`:

```go
// Get only the final response
finalMsg, err := model.Last(model.Generate(ctx, messages))
if err != nil {
    log.Fatal(err)
}
fmt.Println(finalMsg.Content())
```

Collect all intermediate messages:

```go
// Gather all messages (useful for debugging)
messages, err := model.Collect(model.Generate(ctx, messages))
if err != nil {
    log.Fatal(err)
}

for _, msg := range messages {
    fmt.Printf("Message: %s\n", msg.Content())
}
```

When using graph streaming, agents automatically handle the iterator:

```go
stream := compiled.Stream(ctx, messages)
for event := range stream {
    if event.Err != nil {
        log.Printf("Error: %v", event.Err)
        continue
    }
    
    if event.Node == "model" {
        // Access partial response from iterator
        for _, msg := range event.Messages {
            fmt.Print(msg.Content())
        }
    }
}
```

---

## Custom models {#custom-models}

Implement the `model.Model` interface to integrate custom providers using the iterator pattern:

```go
type CustomModel struct {
    client *CustomClient
}

func (m *CustomModel) Generate(ctx context.Context, messages []message.Message) iter.Seq2[message.Message, error] {
    return func(yield func(message.Message, error) bool) {
        // Convert messages to provider format
        req := convertMessages(messages)
        
        // For streaming providers, yield partial responses
        stream, err := m.client.CompleteStream(ctx, req)
        if err != nil {
            yield(nil, err)
            return
        }
        
        for chunk := range stream {
            // Convert chunk to AgentMesh format
            msg := message.NewAIMessageFromText(chunk.Text)
            
            // Yield partial message; if false returned, stop streaming
            if !yield(msg, nil) {
                return
            }
        }
        
        // For non-streaming providers, yield single final message
        // resp, err := m.client.Complete(ctx, req)
        // if err != nil {
        //     yield(nil, err)
        //     return
        // }
        // yield(message.NewAIMessage(message.NewTextPart(resp.Text)), nil)
    }
}

// Optional: Implement ToolAware for function calling
func (m *CustomModel) BindTools(tools ...tool.Tool) model.Model {
    return &CustomModel{
        client: m.client,
        tools:  tools,
    }
}
```

Use your custom model like any other:

```go
model := &CustomModel{client: myClient}
compiled, err := agent.NewReActAgent(model, tools)
```

The iterator pattern automatically supports both streaming and blocking modes through `model.Last()` and `model.Collect()` helpers.
