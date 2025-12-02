package redis

import (
	"context"
	"crypto/tls"
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
//   - Messages are serialized using a pluggable Codec (default: JSON)
//   - Supports TTL for automatic cleanup of stale mailboxes
//   - Thread-safe: Redis handles concurrent access
//
// Redis Keys:
//   - mailbox:{namespace}:{vertex} - List of serialized messages for vertex
//
// Performance Considerations:
//   - Uses pipelining for batch operations
//   - Connection pooling handled by redis client
//   - Automatic retry with exponential backoff
//
// Serialization:
//   - Default: JSON codec (cross-language, but numbers become float64)
//   - Pluggable: Can use GOB, MessagePack, etc. via Codec interface
//
// Limitations:
//   - No combiner support (Redis atomic operations would be complex)
//   - No backpressure (Redis lists are unbounded)
//   - Requires external Redis server
//
// Frontier Tracking:
//
//	Frontier tracking is handled by Runtime's shardedFrontier, not by MessageBus.
//	This simplifies distributed deployments and allows Runtime to use optimized
//	lock-free frontier data structures.
type MessageBus[M any] struct {
	client    *redis.Client
	namespace string
	ttl       time.Duration
	codec     pregel.Codec
	closed    bool
}

// Options configures Redis message store behavior.
type Options struct {
	// Namespace isolates multiple graphs using the same Redis instance.
	// Defaults to "agentmesh" if empty.
	Namespace string

	// TTL sets expiration time for mailbox keys to prevent memory leaks.
	// Defaults to 24 hours. Set to 0 to disable expiration.
	TTL time.Duration

	// Codec specifies the serialization format for messages.
	// Defaults to JSONCodec if nil.
	//
	// JSON (default): Cross-language compatible, numbers become float64
	// GOB: Go-only, preserves exact types, faster
	// MessagePack: Cross-language, faster than JSON, better type support
	Codec pregel.Codec

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

	// TLSConfig enables TLS/SSL encryption for Redis connections.
	// STRONGLY RECOMMENDED for production deployments to prevent:
	//   - Network eavesdropping (messages sent in cleartext)
	//   - Man-in-the-middle attacks
	//   - Credential theft (password sent unencrypted)
	//
	// Security Best Practices:
	//   ✅ REQUIRED for production: Always use TLS when Redis is not on localhost
	//   ✅ Use TLS 1.3 minimum (set MinVersion: tls.VersionTLS13)
	//   ✅ Verify server certificates (do not set InsecureSkipVerify: true)
	//   ✅ Use strong cipher suites (default is usually fine)
	//
	// Example - Production TLS Config:
	//   tlsConfig := &tls.Config{
	//       MinVersion: tls.VersionTLS13,
	//       // Server certificate verification (default, recommended)
	//       InsecureSkipVerify: false,
	//   }
	//
	// Example - Development with Self-Signed Certs:
	//   tlsConfig := &tls.Config{
	//       MinVersion: tls.VersionTLS12,
	//       InsecureSkipVerify: true, // Only for dev/testing!
	//   }
	//
	// If nil, connection will be unencrypted (only safe for localhost).
	TLSConfig *tls.Config
}

// NewMessageBus creates a Redis-backed message store for distributed execution.
//
// addr: Redis server address (host:port), e.g., "localhost:6379"
// password: Redis password (empty string if no auth required)
// db: Redis database number (0-15)
// opts: Optional configuration (can be nil for defaults)
//
// Security:
//   - TLS is REQUIRED when using password authentication (production)
//   - Returns error if password is set but TLSConfig is nil
//   - Only localhost without password is allowed to skip TLS (development)
//
// Example (Development - localhost, no auth, no TLS):
//
//	store, err := redis.NewMessageBus[MyMessage]("localhost:6379", "", 0, &redis.Options{
//	    Namespace: "mygraph",
//	    TTL: 1 * time.Hour,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
//
// Example (Production - with TLS and authentication):
//
//	tlsConfig := &tls.Config{
//	    MinVersion: tls.VersionTLS13,
//	    // InsecureSkipVerify: false (default - verify server cert)
//	}
//
//	store, err := redis.NewMessageBus[MyMessage](
//	    "redis.example.com:6380",
//	    "your-secure-password",
//	    0,
//	    &redis.Options{
//	        Namespace: "production-graph",
//	        TTL:       1 * time.Hour,
//	        TLSConfig: tlsConfig,
//	    },
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
func NewMessageBus[M any](addr, password string, db int, opts *Options) (*MessageBus[M], error) {
	if opts == nil {
		opts = &Options{}
	}

	// SECURITY: Require TLS when using password authentication
	// This prevents credential theft and network eavesdropping in production
	if password != "" && opts.TLSConfig == nil {
		return nil, fmt.Errorf("pregel/redis: TLS required when using password authentication (set Options.TLSConfig)")
	}

	// Set defaults
	if opts.Namespace == "" {
		opts.Namespace = "agentmesh"
	}
	if opts.TTL == 0 {
		opts.TTL = 24 * time.Hour
	}
	if opts.Codec == nil {
		opts.Codec = pregel.NewJSONCodec() // Default to JSON for cross-language compatibility
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
		TLSConfig:       opts.TLSConfig,                 // Enable TLS if provided
		PoolSize:        10,                             // Connection pool size
		MinIdleConns:    2,                              // Maintain idle connections
		ConnMaxIdleTime: 5 * time.Minute,                // Close idle connections after 5 min
		PoolTimeout:     opts.ReadTimeout + time.Second, // Wait for connection from pool
	})

	return &MessageBus[M]{
		client:    client,
		namespace: opts.Namespace,
		ttl:       opts.TTL,
		codec:     opts.Codec,
	}, nil
}

// mailboxKey returns the Redis key for a vertex's mailbox
func (store *MessageBus[M]) mailboxKey(vertex string) string {
	return fmt.Sprintf("mailbox:%s:%s", store.namespace, vertex)
}

// Send delivers messages to target vertices.
// Uses Redis pipelining for batch efficiency.
// Messages are serialized and stored in Redis lists.
// Frontier tracking is handled by Runtime's shardedFrontier.
func (store *MessageBus[M]) Send(ctx context.Context, messages []pregel.Message[M]) error {
	if len(messages) == 0 {
		return nil
	}

	if store.closed {
		return fmt.Errorf("message bus is closed")
	}

	// Use pipeline for batch operations
	pipe := store.client.Pipeline()

	for _, msg := range messages {
		if msg.To == "" {
			continue
		}

		// Serialize message using codec
		data, err := store.codec.Encode(msg)
		if err != nil {
			return fmt.Errorf("failed to serialize message to %q: %w", msg.To, err)
		}

		// Add message to vertex's mailbox (LPUSH for FIFO with RPOP)
		mailboxKey := store.mailboxKey(msg.To)
		pipe.LPush(ctx, mailboxKey, data)

		// Set TTL to prevent memory leaks
		if store.ttl > 0 {
			pipe.Expire(ctx, mailboxKey, store.ttl)
		}
	}

	// Execute pipeline
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to send messages: %w", err)
	}

	return nil
}

// Receive retrieves and removes all messages for the given vertex.
// Uses RPOP in a loop to drain the mailbox efficiently.
func (store *MessageBus[M]) Receive(vertex string) ([]pregel.Message[M], error) {
	if store.closed {
		return nil, fmt.Errorf("message bus is closed")
	}

	ctx := context.Background()
	mailboxKey := store.mailboxKey(vertex)

	// Get all messages from the list (drain it)
	var messages []pregel.Message[M]

	for {
		// RPOP removes and returns last element (FIFO with LPUSH)
		data, err := store.client.RPop(ctx, mailboxKey).Result()
		if errors.Is(err, redis.Nil) {
			// No more messages
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to receive message from %q: %w", vertex, err)
		}

		// Deserialize message using codec
		var msg pregel.Message[M]
		if err := store.codec.Decode([]byte(data), &msg); err != nil {
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
// Deletes the mailbox key.
func (store *MessageBus[M]) Clear(vertex string) error {
	if store.closed {
		return fmt.Errorf("message bus is closed")
	}

	ctx := context.Background()

	// Delete mailbox key
	if err := store.client.Del(ctx, store.mailboxKey(vertex)).Err(); err != nil {
		return fmt.Errorf("failed to clear mailbox for %q: %w", vertex, err)
	}

	return nil
}

// Close releases Redis connection resources.
// After Close, all operations will fail.
func (store *MessageBus[M]) Close() error {
	if store.closed {
		return nil
	}

	store.closed = true
	return store.client.Close()
}

// Ping verifies Redis connectivity.
// Returns error if Redis is unreachable.
func (store *MessageBus[M]) Ping(ctx context.Context) error {
	return store.client.Ping(ctx).Err()
}

// CleanNamespace removes all mailboxes for this namespace.
// Useful for cleanup between test runs or graph executions.
// WARNING: This deletes all data for the namespace!
func (store *MessageBus[M]) CleanNamespace(ctx context.Context) error {
	if store.closed {
		return fmt.Errorf("message bus is closed")
	}

	// Find all mailbox keys for this namespace
	pattern := fmt.Sprintf("mailbox:%s:*", store.namespace)
	var cursor uint64
	var keys []string

	for {
		var scanKeys []string
		var err error

		scanKeys, cursor, err = store.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("failed to scan mailbox keys: %w", err)
		}

		keys = append(keys, scanKeys...)

		if cursor == 0 {
			break
		}
	}

	// Delete all keys
	if len(keys) > 0 {
		if err := store.client.Del(ctx, keys...).Err(); err != nil {
			return fmt.Errorf("failed to delete namespace keys: %w", err)
		}
	}

	return nil
}

// Stats returns statistics about Redis message store state.
// Note: This operation is expensive as it scans all mailbox keys.
func (store *MessageBus[M]) Stats(ctx context.Context) (pregel.MessageStoreStats, error) {
	if store.closed {
		return pregel.MessageStoreStats{}, fmt.Errorf("message bus is closed")
	}

	stats := pregel.MessageStoreStats{}

	// Scan all mailbox keys for this namespace
	pattern := fmt.Sprintf("mailbox:%s:*", store.namespace)
	var cursor uint64

	for {
		var keys []string
		var err error

		keys, cursor, err = store.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return stats, fmt.Errorf("failed to scan mailbox keys: %w", err)
		}

		for _, key := range keys {
			length, err := store.client.LLen(ctx, key).Result()
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
