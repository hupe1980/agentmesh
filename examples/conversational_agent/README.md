# Conversational Agent Example

This example demonstrates how to wrap any agent with long-term memory for multi-turn conversations with context awareness.

## Overview

The **Conversational Agent** pattern adds semantic memory to any agent (ReAct, RAG, Supervisor, etc.) enabling:

- **Context recall**: Automatically recalls relevant past messages before the agent runs
- **Conversation storage**: Stores the exchange after each interaction
- **Session isolation**: Each user/session has its own memory context

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                  Conversational Agent                        │
├─────────────────────────────────────────────────────────────┤
│  START → Memory Recall → Wrapped Agent → Memory Store → END │
│                              │                               │
│                         ReAct/RAG/                           │
│                       Supervisor/etc.                        │
└─────────────────────────────────────────────────────────────┘
```

## Running the Example

```bash
export OPENAI_API_KEY=your-key-here
go run main.go
```

## Key Features

### Session ID (Required)

Each conversation needs a session ID to isolate memory:

```go
chatAgent.Run(ctx, messages,
    graph.WithInitialValue(agent.SessionIDKey, "user-123-session"),
)
```

### Configuration Options

```go
agent.NewConversational(reactAgent, mem,
    agent.WithMaxRecallMessages(10),      // Recall up to N relevant messages
    agent.WithMinSimilarityScore(0.7),    // Minimum similarity for recall
    agent.WithFailOnStoreError(false),    // Don't fail if storage fails
)
```

### Memory Types

- **VectorMemory**: Semantic search using embeddings (recommended for production)
- **SimpleMemory**: Basic FIFO storage without semantic search (good for testing)

## Example Output

```
=== Turn 1: Setting up context ===
Agent: Nice to meet you, Alice! San Francisco is a wonderful city...

=== Turn 2: Using tool ===
Agent: Let me check the weather in San Francisco for you...
Agent: The weather in San Francisco is currently sunny at 72°F.

=== Turn 3: Recalling from memory ===
Agent: Your name is Alice and you live in San Francisco.

=== Conversation complete! ===
The agent remembered context from earlier turns using semantic memory.
```

## See Also

- [Memory Guide](../../docs/memory.md) - Understanding memory types
- [Agents Documentation](../../docs/agents.md#conversational-agent) - Full API reference
