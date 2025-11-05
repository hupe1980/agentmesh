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

AgentMesh abstracts language models behind a common `model.Model` interface:

```go
type Model interface {
    Generate(ctx context.Context, messages []message.Message) (message.Message, error)
    Stream(ctx context.Context, messages []message.Message) (*model.Stream, error)
}
```

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

model := openai.NewModel(func(o *openai.Options) {
    o.Model = "gpt-4o"
    o.Temperature = 0.7
    o.MaxTokens = 1000
})

compiled, err := agent.NewReActAgent(model, tools)
```

Configuration options:

```go
openai.NewModel(func(o *openai.Options) {
    o.Model = "gpt-4o-mini"           // Model name
    o.Temperature = 0.2               // Randomness (0-2)
    o.MaxTokens = 500                 // Max output tokens
    o.TopP = 1.0                      // Nucleus sampling
    o.FrequencyPenalty = 0.0          // Penalize repetition
    o.PresencePenalty = 0.0           // Encourage diversity
})
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

model := anthropic.NewModel(func(o *anthropic.Options) {
    o.Model = "claude-3-5-sonnet-20241022"
    o.MaxTokens = 1024
    o.Temperature = 1.0
})

compiled, err := agent.NewReActAgent(model, tools)
```

Configuration options:

```go
anthropic.NewModel(func(o *anthropic.Options) {
    o.Model = "claude-3-5-sonnet-20241022"
    o.MaxTokens = 2048
    o.Temperature = 0.5
    o.TopP = 1.0
    o.TopK = 250
})
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

All models support streaming for real-time responses:

```go
// Using graph streaming
stream := compiled.Stream(ctx, messages)
for event := range stream {
    if event.Err != nil {
        log.Printf("Error: %v", event.Err)
        continue
    }
    
    if event.Node == "model" {
        // Access partial response
        for _, msg := range event.Messages {
            fmt.Print(msg.Content())
        }
    }
}
```

Models can also be streamed directly:

```go
stream, err := model.Stream(ctx, messages)
if err != nil {
    log.Fatal(err)
}

for {
    msg, err := stream.Receive()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Print(msg.Content())
}
```

---

## Custom models {#custom-models}

Implement the `model.Model` interface to integrate custom providers:

```go
type CustomModel struct {
    client *CustomClient
}

func (m *CustomModel) Generate(ctx context.Context, messages []message.Message) (message.Message, error) {
    // Convert messages to provider format
    req := convertMessages(messages)
    
    // Call provider API
    resp, err := m.client.Complete(ctx, req)
    if err != nil {
        return nil, err
    }
    
    // Convert response to AgentMesh format
    return message.NewAIMessage(message.NewTextPart(resp.Text)), nil
}

func (m *CustomModel) Stream(ctx context.Context, messages []message.Message) (*model.Stream, error) {
    // Implement streaming logic
    // Return *model.Stream for real-time updates
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
