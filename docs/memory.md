---
layout: doc
title: Memory
permalink: /memory/
hero:
  title: Long-term Memory
  description: Store and recall conversation history with semantic vector search.
  primary_cta:
    label: API reference
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/memory"
    external: true
  secondary_cta:
    label: Embeddings guide →
    href: "/embeddings/"
sidebar:
  - title: Overview
    url: "#overview"
  - title: Vector memory
    url: "#vector-memory"
  - title: Simple memory
    url: "#simple-memory"
  - title: Use cases
    url: "#use-cases"
  - title: Best practices
    url: "#best-practices"
---

# Memory

The memory system enables agents to maintain context across multiple interactions by storing and retrieving conversation history using semantic vector search.

---

## Overview {#overview}

AgentMesh provides two memory implementations:

- **VectorMemory** - Semantic search using embeddings for intelligent recall
- **SimpleMemory** - Basic FIFO storage without semantic search

**Key features**:
- ✅ Session-based storage - Organize conversations by session/user ID
- ✅ Semantic search - Find relevant messages by meaning, not keywords
- ✅ Relevance ranking - Sort results by similarity score
- ✅ Flexible filtering - Time-based and metadata queries

---

## Vector Memory {#vector-memory}

VectorMemory uses embeddings to enable semantic search over conversation history.

### Setup

```go
import (
    "github.com/hupe1980/agentmesh/pkg/embedding/openai"
    "github.com/hupe1980/agentmesh/pkg/memory"
)

// Create embedder
embedder := openai.NewEmbedder(client,
    openai.WithModel("text-embedding-3-small"),
)

// Create vector memory
mem := memory.NewVectorMemory(embedder)
```

### Custom VectorStore Backend

By default, VectorMemory uses an in-memory store. For persistence or scalability, provide a custom `VectorStore`:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/memory"
    "github.com/hupe1980/agentmesh/pkg/vectorstore"
    memstore "github.com/hupe1980/agentmesh/pkg/vectorstore/memory"
)

// Use in-memory store (default behavior)
store := memstore.New()
mem := memory.NewVectorMemory(embedder, memory.WithStore(store))

// Or use an external backend (Pinecone, Qdrant, etc.)
// externalStore := pinecone.New(...)
// mem := memory.NewVectorMemory(embedder, memory.WithStore(externalStore))

// Don't forget to close when done
defer mem.Close()
```

The VectorStore backend handles:
- **Persistence** - Messages survive application restarts
- **Scalability** - Support millions of messages
- **Namespace isolation** - Session IDs map to namespaces

### Storing Messages

```go
import "github.com/hupe1980/agentmesh/pkg/message"

sessionID := "user-123-session-456"

messages := []message.Message{
    message.NewHumanMessageFromText("What's the pricing for the enterprise plan?"),
    message.NewAIMessageFromText("Our enterprise plan starts at $500/month with unlimited users."),
    message.NewHumanMessageFromText("What features are included?"),
    message.NewAIMessageFromText("Enterprise includes SSO, advanced analytics, priority support, and custom integrations."),
}

err := mem.Store(ctx, sessionID, messages)
```

### Recalling Messages

Retrieve relevant messages using semantic search:

```go
recalled, err := mem.Recall(ctx, sessionID, memory.RecallFilter{
    Query: "Tell me about the enterprise pricing again",
    K:     5,  // Top 5 most relevant messages
})

for _, msg := range recalled {
    fmt.Printf("%s\n", message.Stringify(msg))
}
```

**Output**:
```
[0.94] Our enterprise plan starts at $500/month with unlimited users.
[0.87] What's the pricing for the enterprise plan?
[0.72] Enterprise includes SSO, advanced analytics, priority support...
```

### Advanced Filtering

```go
last24h := time.Now().Add(-24 * time.Hour)
recalled, err := mem.Recall(ctx, sessionID, memory.RecallFilter{
    Query:    "pricing information",
    K:        10,
    MinScore: 0.7,              // Only messages with similarity > 0.7
    After:    &last24h,         // Only messages after this time
    Types:    []message.Type{message.TypeAI},  // Only AI responses
})
```

### Session Management

```go
// List all sessions
sessions, err := mem.ListSessions(ctx)

// Get session metadata
metadata, err := mem.GetSessionMetadata(ctx, sessionID)
fmt.Printf("Messages: %d, Created: %v\n", metadata.MessageCount, metadata.CreatedAt)

// Delete session
err = mem.DeleteSession(ctx, sessionID)
```

---

## Simple Memory {#simple-memory}

SimpleMemory provides basic FIFO storage without semantic search. Useful for development or when semantic search isn't needed.

### Setup

```go
mem := memory.NewSimpleMemory(
    memory.WithMaxMessages(100),  // Keep last 100 messages
)
```

### Usage

```go
// Store messages
err := mem.Store(ctx, sessionID, messages)

// Recall all messages (no semantic search)
recalled, err := mem.Recall(ctx, sessionID, memory.RecallFilter{
    K: 10,  // Last 10 messages
})

// Recall with time filter
lastHour := time.Now().Add(-1 * time.Hour)
recalled, err := mem.Recall(ctx, sessionID, memory.RecallFilter{
    After: &lastHour,
})
```

---

## Use Cases {#use-cases}

### 1. Conversational Agent Wrapper

The simplest way to add memory to an agent is using the `Conversational` wrapper:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/memory"
)

// Create any agent (ReAct, RAG, Supervisor, etc.)
reactAgent, _ := agent.NewReAct(model, agent.WithTools(tools...))

// Create memory
mem := memory.NewVectorMemory(embedder)

// Wrap with conversational memory
chatAgent, _ := agent.NewConversational(
    reactAgent,
    mem,
    agent.WithShortTermMessages(5),     // Recent messages for context
    agent.WithLongTermMessages(5),      // Semantic search for relevance
    agent.WithMinSimilarityScore(0.5),
)

// Use with a session ID
for msg, err := range chatAgent.Run(ctx, messages,
    graph.WithInitialValue(agent.SessionIDKey, "user-123"),
) {
    // Memory is handled automatically
}
```

See the [Conversational Agent](/agents/#conversational-agent) documentation for details.

### 2. Multi-Session Chatbot

Maintain separate conversation history per user:

```go
type ChatBot struct {
    memory memory.Memory
    agent  *graph.Compiled
}

func (c *ChatBot) Chat(ctx context.Context, userID, message string) (string, error) {
    sessionID := fmt.Sprintf("user-%s", userID)
    
    // Recall relevant context
    history, err := c.memory.Recall(ctx, sessionID, memory.RecallFilter{
        Query: message,
        K:     5,
    })
    if err != nil {
        return "", err
    }
    
    // Build messages with context
    messages := append(history, message.NewHumanMessageFromText(message))
    
    // Execute agent
    var lastResponse string
    for msg, err := range c.agent.Run(ctx, messages) {
        if err != nil {
            return "", err
        }
        lastResponse = msg.Content()
    }
    return lastResponse, nil
    
    // Store conversation
    err = c.memory.Store(ctx, sessionID, results)
    
    return extractResponse(results), nil
}
```

### 3. RAG with Conversation History

Combine memory with retrieval for context-aware RAG:

```go
func answerWithContext(ctx context.Context, query string, sessionID string) (string, error) {
    // 1. Recall relevant conversation history
    history, err := memory.Recall(ctx, sessionID, memory.RecallFilter{
        Query: query,
        K:     3,
    })
    
    // 2. Retrieve relevant documents
    docs, err := retriever.Retrieve(ctx, query, 5)
    
    // 3. Build context from both sources
    context := buildContext(history, docs)
    
    // 4. Generate answer
    prompt := fmt.Sprintf(`Context from previous conversations:
%s

Relevant documents:
%s

Question: %s

Answer:`, formatHistory(history), formatDocs(docs), query)
    
    return agent.Generate(ctx, prompt)
}
```

### 4. Personalized Recommendations

Use memory to track user preferences:

```go
func getRecommendations(ctx context.Context, userID string) ([]string, error) {
    sessionID := fmt.Sprintf("user-%s", userID)
    
    // Recall mentions of preferences
    last30Days := time.Now().Add(-30 * 24 * time.Hour)
    prefs, err := memory.Recall(ctx, sessionID, memory.RecallFilter{
        Query: "I like, I prefer, my favorite",
        K:     10,
        After: &last30Days,
    })
    
    // Extract preferences
    preferences := extractPreferences(prefs)
    
    // Generate recommendations
    return generateRecommendations(preferences), nil
}
```

### 5. Context Window Management

Automatically select most relevant messages when context is limited:

```go
func buildContextualPrompt(ctx context.Context, sessionID, query string, maxTokens int) ([]message.Message, error) {
    // Recall semantically relevant messages
    relevant, err := memory.Recall(ctx, sessionID, memory.RecallFilter{
        Query: query,
        K:     20,  // Get more candidates
    })
    
    // Select messages that fit in context window
    selected := []message.Message{}
    tokens := 0
    
    for _, msg := range relevant {
        msgTokens := countTokens(msg)
        if tokens+msgTokens > maxTokens {
            break
        }
        selected = append(selected, msg)
        tokens += msgTokens
    }
    
    return selected, nil
}
```

### 6. Topic-Based Retrieval

Organize memory by conversation topics:

```go
func recallByTopic(ctx context.Context, sessionID, topic string) ([]message.Message, error) {
    topicQueries := map[string]string{
        "billing":   "payment, invoice, pricing, subscription, billing",
        "support":   "help, issue, problem, error, bug",
        "features":  "feature, capability, functionality, how to",
    }
    
    query := topicQueries[topic]
    
    return memory.Recall(ctx, sessionID, memory.RecallFilter{
        Query: query,
        K:     10,
        MinScore: 0.6,
    })
}
```

### 7. Conversation Summarization

Periodically summarize old messages to save storage:

```go
func summarizeOldMessages(ctx context.Context, sessionID string) error {
    // Get messages older than 7 days
    sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)
    old, err := memory.Recall(ctx, sessionID, memory.RecallFilter{
        Before: &sevenDaysAgo,
        K:      1000,
    })
    
    if len(old) < 10 {
        return nil // Not enough to summarize
    }
    
    // Generate summary
    summary := generateSummary(old)
    
    // Store summary as system message
    summaryMsg := message.NewSystemMessageFromText(fmt.Sprintf("Previous conversation summary: %s", summary))
    err = memory.Store(ctx, sessionID, []message.Message{summaryMsg})
    
    // Delete old messages
    for _, msg := range old {
        err = memory.DeleteMessage(ctx, sessionID, msg.ID)
    }
    
    return nil
}
```

---

## Best Practices {#best-practices}

### 1. Choose the Right Memory Type

```go
// Use VectorMemory when:
// - Conversations are long (>100 messages)
// - Need semantic search ("find when we discussed pricing")
// - Building RAG systems
// - Personalization is important

vectorMem := memory.NewVectorMemory(embedder)

// Use SimpleMemory when:
// - Short conversations
// - Sequential recall is sufficient
// - Development/testing
// - Embedding costs are a concern

simpleMem := memory.NewSimpleMemory()
```

### 2. Optimize Embedding Calls

Batch embed when storing multiple messages:

```go
// ❌ Bad - embeds each message individually
for _, msg := range messages {
    mem.Store(ctx, sessionID, []message.Message{msg})
}

// ✅ Good - batch embeds all messages
mem.Store(ctx, sessionID, messages)
```

### 3. Set Appropriate K Values

```go
// Short-term context (last few exchanges)
mem.Recall(ctx, sessionID, memory.RecallFilter{
    K: 5,
})

// Comprehensive search (find all relevant)
mem.Recall(ctx, sessionID, memory.RecallFilter{
    K: 20,
    MinScore: 0.7,  // But filter by relevance
})
```

### 4. Handle Privacy & Retention

```go
// Automatic cleanup after 90 days
go func() {
    ticker := time.NewTicker(24 * time.Hour)
    for range ticker.C {
        sessions, _ := mem.ListSessions(ctx)
        for _, session := range sessions {
            metadata, _ := mem.GetSessionMetadata(ctx, session)
            if time.Since(metadata.CreatedAt) > 90*24*time.Hour {
                mem.DeleteSession(ctx, session)
            }
        }
    }
}()
```

### 5. Combine with Checkpointing

Persist memory alongside graph state:

```go
// Store conversation in memory
mem.Store(ctx, sessionID, messages)

// Also checkpoint for resumability
compiled, _ := g.Build()

seq := compiled.Run(ctx, messages,
    graph.WithRunID(sessionID),
    graph.WithCheckpointer(checkpointer),
)
```

### 6. Monitor Performance

```go
func (m *InstrumentedMemory) Recall(ctx context.Context, sessionID string, filter memory.RecallFilter) ([]message.Message, error) {
    start := time.Now()
    
    results, err := m.memory.Recall(ctx, sessionID, filter)
    
    metrics.RecordMemoryRecall(time.Since(start), len(results))
    
    return results, err
}
```

---

## Implementation Details

### Vector Memory Architecture

```
1. Store() called with messages
   ↓
2. Batch embed all message texts
   ↓
3. Store embeddings + metadata in vector store
   ↓
4. Index by sessionID

Recall() with query:
   ↓
1. Embed query
   ↓
2. Similarity search in vector store
   ↓
3. Filter by sessionID, time, type
   ↓
4. Rank by cosine similarity
   ↓
5. Return top K results
```

### Performance Characteristics

| Operation | VectorMemory | SimpleMemory |
|-----------|-------------|--------------|
| Store (100 msgs) | ~500ms | ~5ms |
| Recall (semantic) | ~100ms | N/A |
| Recall (recent) | ~100ms | ~1ms |
| Storage/msg | ~800 bytes | ~200 bytes |

---

## Examples

See the [conversational_agent example](https://github.com/hupe1980/agentmesh/tree/main/examples/conversational_agent) for memory integration.

---

## See Also

- [Embeddings Guide](/embeddings/) - Understanding vector embeddings
- [Checkpointing](/checkpointing/) - Persisting graph state
- [API Reference](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/memory)
