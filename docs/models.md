---
layout: doc
title: Models
description: Connect LLM providers through a unified interface with streaming and tool calling support.
permalink: /models/
hero:
  title: Connect language models
  description: Integrate OpenAI, Anthropic, Gemini, LangChainGo, or custom providers with consistent streaming and tool calling.
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
      - title: Gemini
        url: "#gemini"
      - title: LangChainGo
        url: "#langchaingo"
  - title: Tool binding
    url: "#tool-binding"
  - title: Streaming
    url: "#streaming"
  - title: Model routing
    url: "#model-routing"
    children:
      - title: Cost-based
        url: "#cost-based-routing"
      - title: Capability-based
        url: "#capability-routing"
      - title: Fallback
        url: "#fallback-routing"
      - title: Composite
        url: "#composite-routing"
  - title: Custom models
    url: "#custom-models"
---

## Overview {#overview}

AgentMesh abstracts language models behind a common `model.Model` interface that uses Go 1.24+ iterators for unified streaming:

```go
type Model interface {
    Generate(ctx context.Context, req *Request) iter.Seq2[*Response, error]
    Capabilities() Capabilities
}
```

The `Request` struct bundles messages, tools, system prompt, and other options:

```go
type Request struct {
    Messages     []message.Message  // Conversation history
    Tools        []tool.Tool        // Available tools for function calling
    SystemPrompt string             // Per-request system instruction
    OutputSchema *schema.OutputSchema // Structured output schema
    Stream       bool               // Enable streaming mode
    Metadata     map[string]any     // Provider-specific options
}
```

The iterator-based API unifies streaming and blocking modes:
- **Streaming**: Iterate over partial responses as they arrive
- **Blocking**: Use `model.Last()` to get only the final response
- **Batch collection**: Use `model.Collect()` to gather all responses

### Response Structure

Models return a `*model.Response` with rich metadata:

```go
type Response struct {
    Message      message.Message // The actual message content
    Reasoning    string          // Native reasoning (o1/o3, Gemini 2.0, Claude)
    FinishReason string          // Why generation stopped
    Logprobs     *Logprobs       // Token probabilities (OpenAI)
    Usage        *UsageInfo      // Token consumption tracking
    Metadata     map[string]any  // Provider-specific metadata
    Partial      bool            // true for streaming chunks, false for final
}

type UsageInfo struct {
    PromptTokens     int // Input tokens
    CompletionTokens int // Output tokens
    ReasoningTokens  int // Reasoning tokens (o1/o3)
    TotalTokens      int // Sum of all tokens
}
```

### Model Capabilities

All models expose their features via `Capabilities()`:

```go
caps := model.Capabilities()

// Discover what the model supports
if caps.Tools {
    // Can use BindTools()
}
if caps.NativeReasoning {
    // Response.Reasoning will be populated
}
if caps.Vision {
    // Can send image.ImagePart
}

type Capabilities struct {
    Streaming           bool     // Supports incremental responses
    Tools               bool     // Supports function calling
    StructuredOutput    bool     // Supports JSON schema
    NativeReasoning     bool     // Exposes internal reasoning
    Logprobs            bool     // Provides token probabilities
    Vision              bool     // Accepts images
    Audio               bool     // Accepts audio
    MaxContextTokens    int      // Context window size
    MaxOutputTokens     int      // Max generation length
    SupportedModalities []string // Input types: "text", "image", "audio"
}
```

### Optional Interfaces

Models may implement additional interfaces for feature configuration:

```go
// ToolAware enables function calling
type ToolAware interface {
    BindTools(tools ...tool.Tool) Model
}

// StructuredOutput enables JSON schema validation
type StructuredOutput interface {
    WithStructuredOutput(schema map[string]any) Model
}
```

**Always check `Capabilities()` before using optional interfaces** to ensure the model supports the feature.

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

### Gemini {#gemini}

The Gemini adapter integrates Google's Gemini models via the official SDK:

```go
import "github.com/hupe1980/agentmesh/pkg/model/gemini"

model, err := gemini.NewModel(ctx,
    gemini.WithModel("gemini-2.0-flash-exp"),
    gemini.WithMaxOutputTokens(4096),
    gemini.WithTemperature(0.7),
)
if err != nil {
    log.Fatal(err)
}

compiled, err := agent.NewReActAgent(model, tools)
```

Configuration options:

```go
gemini.NewModel(ctx,
    gemini.WithModel("gemini-2.0-flash-exp"), // Model name
    gemini.WithMaxOutputTokens(4096),         // Max output tokens
    gemini.WithTemperature(0.7),              // Randomness (0-1)
    gemini.WithTopP(0.95),                    // Nucleus sampling
    gemini.WithTopK(40),                      // Top-k sampling
    gemini.WithAPIKey("your-api-key"),        // Optional if set in env
)
```

The adapter supports:
- ✅ Streaming responses
- ✅ Function calling via `BindTools()`
- ✅ Vision models (multimodal)
- ✅ Native reasoning (Gemini 2.0)

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
// Stream model responses directly with full metadata access
for resp, err := range model.Generate(ctx, messages) {
    if err != nil {
        log.Printf("Error: %v", err)
        break
    }
    
    // Print partial content as it arrives
    for _, part := range resp.Message.Parts() {
        if text, ok := part.(message.TextPart); ok {
            fmt.Print(text.Text)
        }
    }
    
    // Access streaming reasoning (if supported)
    if resp.Reasoning != "" {
        fmt.Printf("\n[Reasoning: %s]\n", resp.Reasoning)
    }
}
```

For blocking (non-streaming) mode, use `model.Last()`:

```go
// Get only the final response with metadata
resp, err := model.Last(model.Generate(ctx, messages))
if err != nil {
    log.Fatal(err)
}

// Access message content
fmt.Println(message.Stringify(resp.Message))

// Access reasoning (for o1/o3, Gemini 2.0, Claude)
if resp.Reasoning != "" {
    fmt.Println("Reasoning:", resp.Reasoning)
}

// Track token usage
if resp.Usage != nil {
    fmt.Printf("Total tokens: %d (prompt: %d, completion: %d, reasoning: %d)\n",
        resp.Usage.TotalTokens,
        resp.Usage.PromptTokens,
        resp.Usage.CompletionTokens,
        resp.Usage.ReasoningTokens)
}

// Check finish reason
fmt.Println("Finish reason:", resp.FinishReason)
```

Collect all intermediate responses:

```go
// Gather all responses (useful for debugging streaming)
responses, err := model.Collect(model.Generate(ctx, messages))
if err != nil {
    log.Fatal(err)
}

for i, resp := range responses {
    fmt.Printf("Chunk %d: %s\n", i, message.Stringify(resp.Message))
}
```

When using graph streaming, agents automatically handle the iterator:

```go
seq := compiled.Run(ctx, messages)
for event, err := range seq {
    if err != nil {
        log.Printf("Error: %v", err)
        continue
    }
    
    if event.Node == "model" {
        // Each event contains exactly one message
        fmt.Print(message.Stringify(event.Message))
    }
}
```

### Accessing Token Probabilities

OpenAI models support token-level probability analysis:

```go
model := openai.NewModel(
    openai.WithLogprobs(true, 5), // Request top 5 alternatives per token
)

resp, err := model.Last(model.Generate(ctx, messages))
if err != nil {
    log.Fatal(err)
}

if resp.Logprobs != nil {
    for _, tokenInfo := range resp.Logprobs.Content {
        // Main token chosen
        fmt.Printf("Token: %s, Log Probability: %.3f\n",
            tokenInfo.Token, tokenInfo.Logprob)
        
        // Alternative tokens considered
        for _, alt := range tokenInfo.TopLogprobs {
            fmt.Printf("  Alt: %s (%.3f)\n", alt.Token, alt.Logprob)
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

func (m *CustomModel) Generate(ctx context.Context, messages []message.Message) iter.Seq2[*model.Response, error] {
    return func(yield func(*model.Response, error) bool) {
        // Convert messages to provider format
        req := convertMessages(messages)
        
        // For streaming providers, yield partial responses
        stream, err := m.client.CompleteStream(ctx, req)
        if err != nil {
            yield(nil, err)
            return
        }
        
        var totalTokens int
        for chunk := range stream {
            // Convert chunk to AgentMesh format
            msg := message.NewAIMessageFromText(chunk.Text)
            
            // Build response with metadata
            resp := &model.Response{
                Message:      msg,
                Reasoning:    chunk.Reasoning,     // If your provider supports it
                FinishReason: chunk.FinishReason,  // e.g., "stop", "length"
                Usage: &model.UsageInfo{
                    PromptTokens:     chunk.PromptTokens,
                    CompletionTokens: chunk.CompletionTokens,
                    TotalTokens:      chunk.TotalTokens,
                },
            }
            
            // Yield response; if false returned, stop streaming
            if !yield(resp, nil) {
                return
            }
        }
        
        // For non-streaming providers, yield single final response
        // resp, err := m.client.Complete(ctx, req)
        // if err != nil {
        //     yield(nil, err)
        //     return
        // }
        // 
        // yield(&model.Response{
        //     Message: message.NewAIMessage(message.NewTextPart(resp.Text)),
        //     Usage: &model.UsageInfo{
        //         PromptTokens:     resp.Usage.PromptTokens,
        //         CompletionTokens: resp.Usage.CompletionTokens,
        //         TotalTokens:      resp.Usage.TotalTokens,
        //     },
        //     FinishReason: resp.FinishReason,
        // }, nil)
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

**Key Points for Custom Implementations:**
- Return `iter.Seq2[*model.Response, error]` (note the pointer)
- Populate `Usage` field for token tracking and cost monitoring
- Set `FinishReason` to indicate why generation stopped
- Set `Reasoning` field if your provider exposes internal reasoning
- Use `Metadata` map for provider-specific information
- Yield `nil` error for successful chunks, non-nil error to stop iteration

---

## Model Routing {#model-routing}

AgentMesh provides a flexible model routing system to intelligently select models based on query characteristics, capabilities, and availability.

### Router Interface

Routers implement a simple interface that selects the best model for a request:

```go
type Router interface {
    Route(ctx context.Context, req *Request) (Model, error)
}
```

### RoutedModel Wrapper

Wrap any router to make it transparent as a `Model`:

```go
router := model.NewCostBasedRouter(cheapModel, expensiveModel)
routedModel := model.NewRoutedModel(router)

// Use like any other model
agent, _ := agent.NewReActAgent(routedModel, tools)
```

### Cost-Based Routing {#cost-based-routing}

Route simple queries to cheaper models and complex queries to premium models:

```go
cheapModel, _ := openai.NewChatModel(apiKey, openai.WithModel("gpt-4o-mini"))
premiumModel, _ := openai.NewChatModel(apiKey, openai.WithModel("gpt-4o"))

router := model.NewCostBasedRouter(cheapModel, premiumModel,
    model.WithComplexityThreshold(0.5), // 0.0-1.0 scale
)

// Simple queries → gpt-4o-mini
// Complex queries → gpt-4o
```

The built-in `HeuristicEstimator` considers:
- Query length and word count
- Complexity keywords ("analyze", "compare", "explain why")
- Multi-turn conversation context
- Tool binding presence

### Capability-Based Routing {#capability-routing}

Automatically route requests to models with required capabilities:

```go
textModel, _ := openai.NewChatModel(apiKey, openai.WithModel("gpt-4o-mini"))
visionModel, _ := openai.NewChatModel(apiKey, openai.WithModel("gpt-4o"))

router := model.NewCapabilityRouter(
    model.WithCapabilityModel(textModel),
    model.WithCapabilityModel(visionModel),
)

// Text-only requests → gpt-4o-mini (cheaper)
// Requests with images → gpt-4o (has Vision capability)
```

### Fallback Routing {#fallback-routing}

Build resilient pipelines with circuit breaker pattern:

```go
primary, _ := openai.NewChatModel(apiKey, openai.WithModel("gpt-4o"))
backup, _ := anthropic.NewChatModel(claudeKey, anthropic.WithModel("claude-3-5-sonnet"))

router := model.NewFallbackRouter(primary, backup,
    model.WithFailureThreshold(5),          // Open circuit after 5 failures
    model.WithResetTimeout(30*time.Second), // Try primary again after 30s
)

// Automatic failover with health tracking
```

Circuit breaker states:
- **Closed**: All requests go to primary
- **Open**: All requests go to fallback (after threshold failures)
- **Half-Open**: Probe primary with single request to test recovery

### Composite Routing {#composite-routing}

Chain multiple routing strategies:

```go
// First check capabilities, then apply cost optimization
capabilityRouter := model.NewCapabilityRouter(textModel, visionModel)
costRouter := model.NewCostBasedRouter(cheapModel, expensiveModel)

composite := model.NewCompositeRouter(capabilityRouter, costRouter)
```

### Conditional Routing {#conditional-routing}

Route based on custom logic:

```go
router := model.NewConditionalRouter(func(ctx context.Context, req *model.Request) Model {
    // Check metadata for routing hints
    if priority, ok := req.Metadata["priority"].(string); ok && priority == "high" {
        return premiumModel
    }
    return standardModel
})
```

### Weighted Routing {#weighted-routing}

Distribute load across models:

```go
router := model.NewWeightedRouter(
    model.WeightedModel{Model: modelA, Weight: 70}, // 70% traffic
    model.WeightedModel{Model: modelB, Weight: 30}, // 30% traffic
)
```

### Complete Example

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/model"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
)

func main() {
    ctx := context.Background()

    // Create models
    mini, _ := openai.NewChatModel(apiKey, openai.WithModel("gpt-4o-mini"))
    full, _ := openai.NewChatModel(apiKey, openai.WithModel("gpt-4o"))

    // Build routing chain: cost → fallback → model
    costRouter := model.NewCostBasedRouter(mini, full,
        model.WithComplexityThreshold(0.5),
    )
    
    resilientRouter := model.NewFallbackRouter(
        model.NewRoutedModel(costRouter),
        full, // Always fallback to full model
        model.WithFailureThreshold(3),
        model.WithResetTimeout(time.Minute),
    )

    // Use routed model transparently
    routedModel := model.NewRoutedModel(resilientRouter)
    agent, _ := agent.NewReActAgent(routedModel, tools)

    // Execute - routing happens automatically
    for result, err := range agent.Run(ctx, messages) {
        // ...
    }
}
```

See the [`examples/model_router`](../examples/model_router/) directory for a complete working example.
