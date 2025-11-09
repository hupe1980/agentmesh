// Package redis provides a Redis-backed implementation of pregel.MessageBus
// for distributed graph execution.
//
// The Redis message bus enables multi-process, multi-node graph execution by
// storing vertex mailboxes in Redis. This allows scaling graph execution
// across multiple workers while maintaining message delivery semantics.
//
// # Basic Usage
//
//	import (
//	    "github.com/hupe1980/agentmesh/pkg/pregel"
//	    predis "github.com/hupe1980/agentmesh/pkg/pregel/redis"
//	)
//
//	// Create Redis message bus
//	bus := predis.NewMessageBus[MyMessageType](
//	    "localhost:6379",  // Redis address
//	    "",                // Password (empty if no auth)
//	    0,                 // Database number
//	    &predis.Options{
//	        Namespace: "mygraph",
//	        TTL: 1 * time.Hour,
//	    },
//	)
//	defer bus.Close()
//
//	// Use with pregel runtime
//	runtime := pregel.NewRuntime(graph, bus, opts...)
//
// # Architecture
//
// The Redis message bus uses the following Redis data structures:
//
//   - Lists (LPUSH/RPOP): Store messages for each vertex in FIFO order
//   - Sets (SADD/SMEMBERS): Track which vertices have pending messages (frontier)
//   - TTL: Automatic cleanup of stale mailboxes
//
// # Features
//
//   - Thread-safe: Redis handles concurrent access
//   - Persistent: Messages survive process restarts
//   - Scalable: Multiple workers can share the same Redis instance
//   - Automatic cleanup: TTL prevents memory leaks
//   - Connection pooling: Efficient resource usage
//
// # Limitations
//
//   - No combiner support (complex to implement atomically in Redis)
//   - No backpressure (Redis lists are unbounded)
//   - Requires external Redis server
//   - JSON serialization overhead
//
// # Configuration
//
// The Options struct controls Redis connection and behavior:
//
//   - Namespace: Isolates multiple graphs in the same Redis instance
//   - TTL: Automatic expiration of mailbox keys
//   - MaxRetries: Retry transient errors
//   - Timeouts: Control connection, read, and write timeouts
//
// # Operations
//
//   - Send: Batch messages using Redis pipelining
//   - Receive: Drain all messages from a vertex's mailbox
//   - Pending: Get list of vertices with messages
//   - Clear: Remove all messages for a vertex
//   - CleanNamespace: Delete all data for a namespace
//
// # Testing
//
// For testing, use testcontainers to spin up a Redis instance:
//
//	import (
//	    "github.com/testcontainers/testcontainers-go/modules/redis"
//	)
//
//	container, err := redis.Run(ctx, "redis:7-alpine")
//	addr, err := container.Endpoint(ctx, "")
//	bus := predis.NewMessageBus[MyMessage](addr, "", 0, nil)
package redis
