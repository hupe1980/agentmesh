# Message Retention Example

## Overview
Demonstrates conversation history management with message retention policies using typed list keys. Shows how to limit message history to prevent out-of-memory issues in long-running agent conversations.

## Key Concepts
- **Message Retention**: Limit history size using `ListKey` maxSize parameter
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
// Unlimited messages (maxSize=0)
var UnlimitedKey = state.NewListKey[message.Message]("__messages__", 0)

// Limited to 100 messages
var LimitedKey = state.NewListKey[message.Message]("__messages__", 100)
```

### 2. Register Key with State Manager
```go
builder := state.NewManagerBuilder()
state.RegisterListKey(builder, messagesKey)

mgr := builder.Build()
g, err := graph.NewGraph(mgr)
```

### 3. Automatic Pruning Behavior
When the message limit is exceeded, the oldest messages are automatically removed:

```go
messagesKey := state.NewListKey[message.Message]("__messages__", 10)

// After adding 15 messages:
//  - Messages 1-5 are automatically pruned
//  - Messages 6-15 are retained (last 10)
```

### 4. Access Messages from State
```go
view, _ := manager.CreateReadView(ctx)
messages := state.GetFromView(view, messagesKey.Key) // ListKey embeds Key[[]T]
fmt.Printf("Message count: %d\n", len(messages))
```

## Message Lifecycle

### Without Retention (maxSize=0)
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
var ChatMessagesKey = state.NewListKey[message.Message]("__messages__", 100)
```

### Production Agents
```go
// Larger buffer for complex workflows
var AgentMessagesKey = state.NewListKey[message.Message]("__messages__", 200)
```

### LLM Context Management
```go
// GPT-4: ~8K tokens ≈ 40-50 messages
var ContextKey = state.NewListKey[message.Message]("__messages__", 50)
```

## Related Examples
- [State Management](../state_builder/) - Basic typed key registration
- [Checkpointing](../checkpointing/) - Persist message history
- [Basic Agent](../basic_agent/) - Simple agent with message handling
