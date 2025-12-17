# Message Retention Example

## Overview
Demonstrates conversation history management with message retention policies using typed list keys. Shows how to limit message history to prevent out-of-memory issues in long-running agent conversations.

## Key Concepts
- **Message Retention**: Limit history size using `ListKey` with maxSize option
- **Automatic Pruning**: Older messages removed when limit exceeded
- **Memory Efficiency**: Prevent OOM in long conversations
- **Context Window Management**: Keep relevant history for LLMs
- **Token Limit Prevention**: Avoid exceeding LLM limits

## Running
```bash
cd examples/message_retention
go run main.go
```

## Expected Output
```
=== With Unlimited Messages (maxSize=0) ===
Messages retained: 4 (maxSize=0)
✓ All messages retained (unlimited)

=== With MaxSize=2 ===
Messages retained: 2 (maxSize=2)
✓ Older messages automatically pruned

=== Recommended Production Configuration ===
For long-running agents, set maxSize to 100-1000
This prevents OOM while retaining sufficient context
Messages retained: 4 (maxSize=100)
✓ Older messages automatically pruned
```

## Code Walkthrough

### 1. Create ListKey with Message Limit
```go
// Unlimited messages (no limit)
var UnlimitedKey = graph.NewListKey[message.Message]("messages")

// Limited to 100 messages
var LimitedKey = graph.NewListKey[message.Message]("messages", 
    graph.WithMaxSize(100),
)
```

### 2. Create Graph with Key
```go
g := graph.New(LimitedKey)

g.Node("agent", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    messages := graph.GetList(scope, LimitedKey)
    
    // Process messages...
    response := message.NewAIMessageFromText("Hello!")
    
    // Append to messages list
    return graph.Reply(response).End()
}, graph.END)

g.Start("agent")
```

### 3. Automatic Pruning Behavior
When the message limit is exceeded, the oldest messages are automatically removed:

```go
// Create key with max 10 messages
var MessagesKey = graph.NewListKey[message.Message]("messages",
    graph.WithMaxSize(10),
)

// After adding 15 messages:
//  - Messages 1-5 are automatically pruned
//  - Messages 6-15 are retained (last 10)
```

### 4. Access Messages from State
```go
g.Node("processor", func(ctx context.Context, input graph.NodeInput[any]) (graph.Command, error) {
    messages := graph.GetList(input, MessagesKey)
    fmt.Printf("Message count: %d\n", len(messages))
    
    return graph.ToEnd(), nil
}, graph.END)
```

## Message Lifecycle

### Without Retention (no maxSize)
```
Message 1 → Message 2 → Message 3 → ... → Message 1000
All messages kept in memory
```

### With Retention (maxSize=10)
```
Message 1-90: Automatically pruned
Message 91-100: Retained (last 10)
```

## What This Example Teaches
- ✅ Type-safe message history management with `ListKey`
- ✅ Memory efficiency through automatic pruning
- ✅ Context window control for LLMs
- ✅ Long conversation handling
- ✅ Production-ready retention policies

## Use Cases

### Chat Applications
```go
// Keep last 100 messages (~50 conversation turns)
var ChatMessagesKey = graph.NewListKey[message.Message]("messages",
    graph.WithMaxSize(100),
)
```

### Production Agents
```go
// Larger buffer for complex workflows
var AgentMessagesKey = graph.NewListKey[message.Message]("messages",
    graph.WithMaxSize(200),
)
```

### LLM Context Management
```go
// GPT-4: ~8K tokens ≈ 40-50 messages
var ContextKey = graph.NewListKey[message.Message]("messages",
    graph.WithMaxSize(50),
)
```

## Related Examples
- [State Builder](../state_builder/) - Basic typed key registration
- [Checkpointing](../checkpointing/) - Persist message history
- [Basic Agent](../basic_agent/) - Simple agent with message handling
