package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/pkg/pregel"
	"github.com/redis/go-redis/v9"
)

// MessageBus implements pregel.MessageBus using Redis for distributed execution.
// It provides persistent, multi-process message delivery with automatic cleanup.
//
// Design:
//   - Each vertex mailbox is stored as a Redis list (LPUSH/RPOP for FIFO queue)
//   - Frontier tracking uses a Redis set for O(1) membership checks
//   - Messages are JSON-serialized for cross-language compatibility
//   - Supports TTL for automatic cleanup of stale mailboxes
//   - Thread-safe: Redis handles concurrent access
//
// Redis Keys:
//   - mailbox:{namespace}:{vertex} - List of serialized messages for vertex
//   - frontier:{namespace} - Set of vertices with pending messages
//
// Performance Considerations:
//   - Uses pipelining for batch operations
//   - Connection pooling handled by redis client
//   - Automatic retry with exponential backoff
//
// Limitations:
//   - No combiner support (Redis atomic operations would be complex)
//   - No backpressure (Redis lists are unbounded)
//   - Requires external Redis server
type MessageBus[M any] struct {
	client    *redis.Client
	namespace string
	ttl       time.Duration
	closed    bool
}

// Options configures Redis message bus behavior.
type Options struct {
	// Namespace isolates multiple graphs using the same Redis instance.
	// Defaults to "agentmesh" if empty.
	Namespace string

	// TTL sets expiration time for mailbox keys to prevent memory leaks.
	// Defaults to 24 hours. Set to 0 to disable expiration.
	TTL time.Duration

	// MaxRetries controls retry behavior for transient Redis errors.
	// Defaults to 3 retries with exponential backoff.
	MaxRetries int

	// DialTimeout sets connection timeout.
	// Defaults to 5 seconds.
	DialTimeout time.Duration

	// ReadTimeout sets timeout for read operations.
	// Defaults to 3 seconds.
	ReadTimeout time.Duration

	// WriteTimeout sets timeout for write operations.
	// Defaults to 3 seconds.
	WriteTimeout time.Duration
}

// NewMessageBus creates a Redis-backed message bus for distributed execution.
//
// addr: Redis server address (host:port), e.g., "localhost:6379"
// password: Redis password (empty string if no auth required)
// db: Redis database number (0-15)
// opts: Optional configuration (can be nil for defaults)
//
// Example:
//
//	bus := redis.NewMessageBus[MyMessage]("localhost:6379", "", 0, &redis.Options{
//	    Namespace: "mygraph",
//	    TTL: 1 * time.Hour,
//	})
//	defer bus.Close()
func NewMessageBus[M any](addr, password string, db int, opts *Options) *MessageBus[M] {
	if opts == nil {
		opts = &Options{}
	}

	// Set defaults
	if opts.Namespace == "" {
		opts.Namespace = "agentmesh"
	}
	if opts.TTL == 0 {
		opts.TTL = 24 * time.Hour
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = 3
	}
	if opts.DialTimeout == 0 {
		opts.DialTimeout = 5 * time.Second
	}
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 3 * time.Second
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = 3 * time.Second
	}

	client := redis.NewClient(&redis.Options{
		Addr:            addr,
		Password:        password,
		DB:              db,
		MaxRetries:      opts.MaxRetries,
		DialTimeout:     opts.DialTimeout,
		ReadTimeout:     opts.ReadTimeout,
		WriteTimeout:    opts.WriteTimeout,
		PoolSize:        10,                             // Connection pool size
		MinIdleConns:    2,                              // Maintain idle connections
		ConnMaxIdleTime: 5 * time.Minute,                // Close idle connections after 5 min
		PoolTimeout:     opts.ReadTimeout + time.Second, // Wait for connection from pool
	})

	return &MessageBus[M]{
		client:    client,
		namespace: opts.Namespace,
		ttl:       opts.TTL,
	}
}

// mailboxKey returns the Redis key for a vertex's mailbox
func (bus *MessageBus[M]) mailboxKey(vertex string) string {
	return fmt.Sprintf("mailbox:%s:%s", bus.namespace, vertex)
}

// frontierKey returns the Redis key for the frontier set
func (bus *MessageBus[M]) frontierKey() string {
	return fmt.Sprintf("frontier:%s", bus.namespace)
}

// Send delivers messages to target vertices.
// Uses Redis pipelining for batch efficiency.
// Messages are JSON-serialized and stored in Redis lists.
func (bus *MessageBus[M]) Send(ctx context.Context, messages []pregel.Message[M]) error {
	if len(messages) == 0 {
		return nil
	}

	if bus.closed {
		return fmt.Errorf("message bus is closed")
	}

	// Use pipeline for batch operations
	pipe := bus.client.Pipeline()

	for _, msg := range messages {
		if msg.To == "" {
			continue
		}

		// Serialize message to JSON
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to serialize message to %q: %w", msg.To, err)
		}

		// Add message to vertex's mailbox (LPUSH for FIFO with RPOP)
		mailboxKey := bus.mailboxKey(msg.To)
		pipe.LPush(ctx, mailboxKey, data)

		// Set TTL to prevent memory leaks
		if bus.ttl > 0 {
			pipe.Expire(ctx, mailboxKey, bus.ttl)
		}

		// Add vertex to frontier set
		pipe.SAdd(ctx, bus.frontierKey(), msg.To)
	}

	// Execute pipeline
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to send messages: %w", err)
	}

	return nil
}

// Receive retrieves and removes all messages for the given vertex.
// Uses RPOP in a loop to drain the mailbox efficiently.
func (bus *MessageBus[M]) Receive(vertex string) ([]pregel.Message[M], error) {
	if bus.closed {
		return nil, fmt.Errorf("message bus is closed")
	}

	ctx := context.Background()
	mailboxKey := bus.mailboxKey(vertex)

	// Get all messages from the list (drain it)
	var messages []pregel.Message[M]

	for {
		// RPOP removes and returns last element (FIFO with LPUSH)
		data, err := bus.client.RPop(ctx, mailboxKey).Result()
		if errors.Is(err, redis.Nil) {
			// No more messages
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to receive message from %q: %w", vertex, err)
		}

		// Deserialize message
		var msg pregel.Message[M]
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			return nil, fmt.Errorf("failed to deserialize message from %q: %w", vertex, err)
		}

		messages = append(messages, msg)
	}

	if len(messages) == 0 {
		return nil, nil
	}

	return messages, nil
}

// Clear removes all messages for the given vertex without returning them.
// Deletes the mailbox key and removes vertex from frontier.
func (bus *MessageBus[M]) Clear(vertex string) error {
	if bus.closed {
		return fmt.Errorf("message bus is closed")
	}

	ctx := context.Background()

	// Use pipeline for atomic operations
	pipe := bus.client.Pipeline()
	pipe.Del(ctx, bus.mailboxKey(vertex))
	pipe.SRem(ctx, bus.frontierKey(), vertex)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear mailbox for %q: %w", vertex, err)
	}

	return nil
}

// Pending returns the vertices that have messages waiting.
// Reads from the frontier set and returns all members.
func (bus *MessageBus[M]) Pending() ([]string, error) {
	if bus.closed {
		return nil, fmt.Errorf("message bus is closed")
	}

	ctx := context.Background()
	frontierKey := bus.frontierKey()

	// Get all members from frontier set
	vertices, err := bus.client.SMembers(ctx, frontierKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending vertices: %w", err)
	}

	// Clear frontier after reading (similar to InMemoryMessageBus behavior)
	if len(vertices) > 0 {
		if err := bus.client.Del(ctx, frontierKey).Err(); err != nil {
			return nil, fmt.Errorf("failed to clear frontier: %w", err)
		}
	}

	if len(vertices) == 0 {
		return nil, nil
	}

	return vertices, nil
}

// Close releases Redis connection resources.
// After Close, all operations will fail.
func (bus *MessageBus[M]) Close() error {
	if bus.closed {
		return nil
	}

	bus.closed = true
	return bus.client.Close()
}

// Ping verifies Redis connectivity.
// Returns error if Redis is unreachable.
func (bus *MessageBus[M]) Ping(ctx context.Context) error {
	return bus.client.Ping(ctx).Err()
}

// CleanNamespace removes all mailboxes and frontier for this namespace.
// Useful for cleanup between test runs or graph executions.
// WARNING: This deletes all data for the namespace!
func (bus *MessageBus[M]) CleanNamespace(ctx context.Context) error {
	if bus.closed {
		return fmt.Errorf("message bus is closed")
	}

	// Find all mailbox keys for this namespace
	pattern := fmt.Sprintf("mailbox:%s:*", bus.namespace)
	var cursor uint64
	var keys []string

	for {
		var scanKeys []string
		var err error

		scanKeys, cursor, err = bus.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("failed to scan mailbox keys: %w", err)
		}

		keys = append(keys, scanKeys...)

		if cursor == 0 {
			break
		}
	}

	// Add frontier key
	keys = append(keys, bus.frontierKey())

	// Delete all keys
	if len(keys) > 0 {
		if err := bus.client.Del(ctx, keys...).Err(); err != nil {
			return fmt.Errorf("failed to delete namespace keys: %w", err)
		}
	}

	return nil
}

// Stats returns statistics about Redis message bus state.
// Note: This operation is expensive as it scans all mailbox keys.
func (bus *MessageBus[M]) Stats(ctx context.Context) (pregel.MessageBusStats, error) {
	if bus.closed {
		return pregel.MessageBusStats{}, fmt.Errorf("message bus is closed")
	}

	stats := pregel.MessageBusStats{}

	// Scan all mailbox keys for this namespace
	pattern := fmt.Sprintf("mailbox:%s:*", bus.namespace)
	var cursor uint64

	for {
		var keys []string
		var err error

		keys, cursor, err = bus.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return stats, fmt.Errorf("failed to scan mailbox keys: %w", err)
		}

		for _, key := range keys {
			length, err := bus.client.LLen(ctx, key).Result()
			if err != nil {
				continue // Skip errors for individual keys
			}

			if length > 0 {
				stats.VerticesWithMessages++
				stats.TotalMessages += int(length)
				if int(length) > stats.LargestMailbox {
					stats.LargestMailbox = int(length)
				}
			}
		}

		if cursor == 0 {
			break
		}
	}

	return stats, nil
}
