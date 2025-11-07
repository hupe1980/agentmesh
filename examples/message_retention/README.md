# Example: Message Retention

## Overview
Demonstrates conversation history management with message retention policies. Shows how to limit message history to prevent out-of-memory issues in long-running agent conversations.

## Key Concepts
- **Message Retention**: Limit history size
- **Automatic Pruning**: Remove old messages
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
=== Message Retention Example ===

Test 1: Unlimited Messages
  Sending 100 messages...
  Message history size: 100
  ✓ All messages retained

Test 2: Limited to 10 Messages
  Sending 100 messages...
  Message history size: 10
  ✓ Only last 10 messages retained
  Oldest message: "Message 91"
  Newest message: "Message 100"

Test 3: Dynamic Retention
  Initial limit: 5
  Sending 10 messages...
  History size: 5
  
  Increasing limit to 20
  Sending 10 more messages...
  History size: 15
  ✓ Retention policy updated
```

## Code Walkthrough

### 1. Create State with Message Limit
```go
// Unlimited messages
state := graph.NewGraphState(0)

// Limited to 50 messages
state := graph.NewGraphState(50)
```

### 2. Automatic Pruning
```go
// When limit exceeded, oldest messages are automatically removed
state := graph.NewGraphState(10)

// After adding 15 messages:
//  - Messages 1-5 are pruned
//  - Messages 6-15 are retained
```

### 3. Using StateBuilder
```go
state := graph.NewStateBuilder().
    WithMessages(100).  // Max 100 messages
    Build()
```

## Message Lifecycle

### Without Retention (0 = Unlimited)
```
Message 1 → Message 2 → Message 3 → ... → Message 1000
All messages kept in memory
```

### With Retention (Max 10)
```
Message 1-90: Pruned
Message 91-100: Retained
```

## What This Example Teaches
- ✅ Message history management
- ✅ Memory efficiency
- ✅ Automatic pruning
- ✅ Context window control
- ✅ Long conversation handling

## Use Cases

### Chat Applications
```go
// Keep last 50 turns
state := graph.NewGraphState(100) // 100 messages = ~50 turns
```

### Production Agents
```go
// Limit memory usage
state := graph.NewGraphState(200)
```

### LLM Context Management
```go
// GPT-4: ~8K tokens ≈ 40-50 messages
state := graph.NewGraphState(50)

// GPT-3.5: ~4K tokens ≈ 20-30 messages
state := graph.NewGraphState(30)
```

## Production Considerations

### Token Counting
```go
// Estimate tokens before sending to LLM
func estimateTokens(messages []message.Message) int {
    total := 0
    for _, msg := range messages {
        // Rough estimate: 1 token ≈ 4 characters
        text := getMessageText(msg)
        total += len(text) / 4
    }
    return total
}
```

### Dynamic Adjustment
```go
// Adjust retention based on token count
messages := state.MessagesSnapshot()
tokens := estimateTokens(messages)

if tokens > 7000 {
    // Approaching limit, reduce history
    state.SetMaxMessages(len(messages) / 2)
}
```

### Important Message Preservation
```go
// Keep system messages regardless of limit
func preserveSystemMessages(messages []message.Message) []message.Message {
    preserved := make([]message.Message, 0)
    for _, msg := range messages {
        if msg.Role() == message.RoleSystem {
            preserved = append(preserved, msg)
        }
    }
    return append(preserved, getRecentMessages(messages, 40)...)
}
```

## Next Steps
- Implement smart message pruning
- Add token counting
- Create conversation summarization
- See **examples/basic_agent** for ReAct patterns

## See Also
- [pkg/graph](../../pkg/graph) - GraphState API
- [pkg/message](../../pkg/message) - Message types
- [examples/basic_agent](../basic_agent) - Agent basics
