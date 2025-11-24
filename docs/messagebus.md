# Message Bus Abstraction

## Overview

The Message Bus abstraction decouples message delivery from the Pregel runtime, enabling distributed execution and pluggable storage backends.

## Architecture

```
┌─────────────────────────────────────────┐
│         Runtime[S, M]                   │
│  ┌───────────────────────────────────┐  │
│  │   Execution Logic                 │  │
│  │   - Superstep coordination        │  │
│  │   - Worker pool management        │  │
│  │   - Aggregator finalization       │  │
│  └───────────────┬───────────────────┘  │
│                  │                       │
│                  ▼                       │
│  ┌───────────────────────────────────┐  │
│  │      MessageBus[M]                │  │
│  │   (Pluggable Interface)           │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
                  │
        ┌─────────┴─────────┬─────────────┐
        ▼                   ▼             ▼
┌───────────────┐  ┌───────────────┐  ┌──────────┐
│ InMemory      │  │ Redis         │  │ gRPC     │
│ MessageBus    │  │ MessageBus    │  │ MessageBus│
└───────────────┘  └───────────────┘  └──────────┘
```

## Interface

```go
type MessageBus[M any] interface {
    // Send delivers messages to target vertices with backpressure.
    // For bounded mailboxes (maxSize > 0), Send blocks when full until 
    // space is available or context is cancelled.
    // Returns context error if cancelled during blocking send.
    // Messages are NEVER dropped silently.
    Send(ctx context.Context, messages []Message[M]) error
    
    // Receive retrieves and removes all messages for a vertex
    Receive(vertex string) ([]Message[M], error)
    
    // Clear removes all messages for a vertex
    Clear(vertex string) error
    
    // Close releases resources
    Close() error
}
```

## Usage

### Default (In-Memory)

```go
// Automatically uses InMemoryMessageBus
runtime := pregel.NewRuntime(graph, events,
    pregel.WithMaxMailboxSize[S, M](100),
    pregel.WithCombiner[S, M](myCombiner),
)
```

### Custom Message Bus

```go
// Use custom message bus implementation
bus := NewRedisMessageBus[MyMessage](redisClient, "graph:mailbox")

runtime := pregel.NewRuntime(graph, events,
    pregel.WithMessageBus[S, M](bus),
)
```

## Built-in Implementation

### InMemoryMessageBus

Single-process message delivery with bounded mailboxes and backpressure.

**Features:**
- Thread-safe concurrent access (32 sharded locks)
- Backpressure: Send blocks when mailbox is full (never drops messages)
- Context-aware: Returns error on timeout/cancellation
- Optional message combiner to reduce memory pressure
- Zero external dependencies

**Behavior:**
- **All mailboxes are bounded**: If `maxSize <= 0`, defaults to `DefaultMaxMailboxSize` (10000)
- **Backpressure**: Send blocks when mailbox is full, unblocks when space available
- **Context cancellation**: Returns error, guarantees no message corruption
- **Message combiner**: Automatically merges messages for same target when channel ≥75% full

**Configuration:**
```go
bus := pregel.NewInMemoryMessageBus[MyMessage](
    100,        // Max 100 messages per vertex (0 or negative = defaults to 10000)
    combiner,   // Optional message combiner (nil = no combining)
)
```

**Example with context timeout:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := bus.Send(ctx, messages)
if err != nil {
    // Handle timeout or cancellation
    log.Printf("Send failed: %v", err)
}
```

## Custom Implementations

### Redis Backend (Example)

```go
type RedisMessageBus[M any] struct {
    client *redis.Client
    prefix string
}

func (bus *RedisMessageBus[M]) Send(messages []Message[M]) error {
    pipe := bus.client.Pipeline()
    for _, msg := range messages {
        key := fmt.Sprintf("%s:%s", bus.prefix, msg.To)
        data, _ := json.Marshal(msg)
        pipe.RPush(ctx, key, data)
    }
    _, err := pipe.Exec(ctx)
    return err
}

func (bus *RedisMessageBus[M]) Receive(vertex string) ([]Message[M], error) {
    key := fmt.Sprintf("%s:%s", bus.prefix, vertex)
    data, err := bus.client.LRange(ctx, key, 0, -1).Result()
    if err != nil {
        return nil, err
    }
    bus.client.Del(ctx, key)
    
    var messages []Message[M]
    for _, item := range data {
        var msg Message[M]
        json.Unmarshal([]byte(item), &msg)
        messages = append(messages, msg)
    }
    return messages, nil
}
```

### gRPC Backend (Example)

```go
type GRPCMessageBus[M any] struct {
    conn   *grpc.ClientConn
    client pb.MessageServiceClient
}

func (bus *GRPCMessageBus[M]) Send(messages []Message[M]) error {
    pbMessages := make([]*pb.Message, len(messages))
    for i, msg := range messages {
        pbMessages[i] = &pb.Message{
            From: msg.From,
            To:   msg.To,
            Data: serializeData(msg.Data),
        }
    }
    
    _, err := bus.client.SendMessages(ctx, &pb.SendRequest{
        Messages: pbMessages,
    })
    return err
}
```

## Benefits

### 1. Distributed Execution

Run Pregel computation across multiple machines:

```go
// Coordinator node
coordinator := pregel.NewRuntime(graph, events,
    pregel.WithMessageBus[S, M](sharedBus),
    pregel.WithWorkerID("coordinator"),
)

// Worker nodes
worker1 := pregel.NewRuntime(graph, events,
    pregel.WithMessageBus[S, M](sharedBus),
    pregel.WithWorkerID("worker-1"),
)

worker2 := pregel.NewRuntime(graph, events,
    pregel.WithMessageBus[S, M](sharedBus),
    pregel.WithWorkerID("worker-2"),
)
```

### 2. Message Persistence

Debug and replay executions:

```go
bus := NewPersistedMessageBus[MyMessage](db, "run-123")
runtime := pregel.NewRuntime(graph, events,
    pregel.WithMessageBus[S, M](bus),
)

// Later: replay from persisted messages
replayBus := NewPersistedMessageBus[MyMessage](db, "run-123")
replayRuntime := pregel.NewRuntime(graph, events,
    pregel.WithMessageBus[S, M](replayBus),
)
```

### 3. Backpressure & Flow Control

**Backpressure is built into InMemoryMessageBus by default:**

```go
// With bounded mailbox (built-in backpressure)
bus := pregel.NewInMemoryMessageBus[MyMessage](
    100,  // When mailbox has 100 messages, sends block
    nil,
)

// With message combiner (reduces pressure automatically)
bus := pregel.NewInMemoryMessageBus[MyMessage](
    100,
    func(existing, incoming Message[MyMessage]) Message[MyMessage] {
        // Merge messages for same target (triggered at 75% capacity)
        return Message[MyMessage]{
            To:   existing.To,
            Data: mergeData(existing.Data, incoming.Data),
        }
    },
)

// With context timeout (prevents indefinite blocking)
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

err := bus.Send(ctx, messages)
if err != nil {
    // Handle send failure (timeout, cancellation, etc.)
}
```

**Additional rate limiting:**

```go
// Custom wrapper for external rate limiting
bus := NewRateLimitedMessageBus[MyMessage](
    innerBus,
    1000,  // Max 1000 messages/sec
)
```

### 4. Observability

Trace message flow:

```go
bus := NewInstrumentedMessageBus[MyMessage](
    innerBus,
    metricsProvider,
)
// Automatically tracks:
// - Message throughput
// - Mailbox sizes
// - Delivery latency
```

## Migration Guide

### From v1.x to v2.0

The MessageBus abstraction is **backward compatible**. Existing code works without changes:

```go
// Old code (still works)
runtime := pregel.NewRuntime(graph, events,
    pregel.WithMaxMailboxSize[S, M](100),
)

// Equivalent new code (explicit)
bus := pregel.NewInMemoryMessageBus[M](100, nil)
runtime := pregel.NewRuntime(graph, events,
    pregel.WithMessageBus[S, M](bus),
)
```

### Enabling Distributed Execution

1. **Choose a backend:** Redis, gRPC, Kafka, etc.
2. **Implement MessageBus interface**
3. **Pass to WithMessageBus option**
4. **Deploy multiple runtimes with shared bus**

## Performance Considerations

### In-Memory Bus
- **Latency:** ~10-50 nanoseconds per operation
- **Throughput:** Millions of messages/sec
- **Memory:** O(messages) per process

### Redis Bus
- **Latency:** ~1-5 milliseconds per operation
- **Throughput:** ~10k-100k messages/sec
- **Memory:** Centralized in Redis

### gRPC Bus
- **Latency:** ~0.5-2 milliseconds per operation
- **Throughput:** ~50k-200k messages/sec
- **Memory:** Distributed across workers

## Testing

### Unit Tests

```go
func TestCustomMessageBus(t *testing.T) {
    bus := NewMyMessageBus[string]()
    defer bus.Close()
    
    // Test send
    err := bus.Send([]Message[string]{
        {To: "a", Data: "msg1"},
    })
    assert.NoError(t, err)
    
    // Test receive
    msgs, err := bus.Receive("a")
    assert.NoError(t, err)
    assert.Len(t, msgs, 1)
}
```

### Integration Tests

See `pkg/pregel/messagebus_test.go` for comprehensive test suite including:
- Basic send/receive
- Mailbox size limits
- Message combiners
- Concurrent access
- Frontier management
- Error handling

## Future Enhancements

1. **Kafka MessageBus** - For event-sourced architectures
2. **SQS MessageBus** - For AWS-native deployments
3. **NATS MessageBus** - For pub/sub patterns
4. **Compression** - Reduce network bandwidth
5. **Encryption** - Secure message transmission
