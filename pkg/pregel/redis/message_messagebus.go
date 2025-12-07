package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/pregel"
	"github.com/redis/go-redis/v9"
)

// MessageMessageBus is a specialized Redis message bus for message.Message types.
// It handles JSON serialization of the message.Message interface properly.
type MessageMessageBus struct {
	client    *redis.Client
	namespace string
	ttl       time.Duration
	closed    bool
}

// NewMessageMessageBus creates a Redis message bus specifically for message.Message types.
// This handles the interface serialization properly.
func NewMessageMessageBus(addr, password string, db int, opts *Options) *MessageMessageBus {
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
		PoolSize:        10,
		MinIdleConns:    2,
		ConnMaxIdleTime: 5 * time.Minute,
		PoolTimeout:     opts.ReadTimeout + time.Second,
	})

	return &MessageMessageBus{
		client:    client,
		namespace: opts.Namespace,
		ttl:       opts.TTL,
	}
}

// mailboxKey returns the Redis key for a vertex's mailbox
func (bus *MessageMessageBus) mailboxKey(vertex string) string {
	return fmt.Sprintf("mailbox:%s:%s", bus.namespace, vertex)
}

// frontierKey returns the Redis key for the frontier set
func (bus *MessageMessageBus) frontierKey() string {
	return fmt.Sprintf("frontier:%s", bus.namespace)
}

// serializablePregelMessage wraps pregel.Message[message.Message] for serialization.
type serializablePregelMessage struct {
	From string                       `json:"from"`
	To   string                       `json:"to"`
	Data *message.SerializableMessage `json:"data"`
}

// Send delivers messages to target vertices.
func (bus *MessageMessageBus) Send(ctx context.Context, messages []pregel.Message[message.Message]) error {
	if len(messages) == 0 {
		return nil
	}

	if bus.closed {
		return pregel.ErrMessageBusClosed
	}

	pipe := bus.client.Pipeline()

	for _, msg := range messages {
		if msg.To == "" {
			continue
		}

		// Convert to serializable form
		spm := serializablePregelMessage{
			From: msg.From,
			To:   msg.To,
			Data: message.ToSerializable(msg.Data),
		}

		// Serialize message to JSON
		data, err := json.Marshal(spm)
		if err != nil {
			return fmt.Errorf("failed to serialize message to %q: %w", msg.To, err)
		}

		// Add message to vertex's mailbox
		mailboxKey := bus.mailboxKey(msg.To)
		pipe.LPush(ctx, mailboxKey, data)

		// Set TTL
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
func (bus *MessageMessageBus) Receive(vertex string) ([]pregel.Message[message.Message], error) {
	if bus.closed {
		return nil, pregel.ErrMessageBusClosed
	}

	ctx := context.Background()
	mailboxKey := bus.mailboxKey(vertex)

	var messages []pregel.Message[message.Message]

	for {
		// RPOP removes and returns last element (FIFO with LPUSH)
		data, err := bus.client.RPop(ctx, mailboxKey).Result()
		if errors.Is(err, redis.Nil) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to receive message from %q: %w", vertex, err)
		}

		// Deserialize message
		var spm serializablePregelMessage
		if err := json.Unmarshal([]byte(data), &spm); err != nil {
			return nil, fmt.Errorf("failed to deserialize message from %q: %w", vertex, err)
		}

		// Convert back to message.Message
		msg, err := message.FromSerializable(spm.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to convert serializable message: %w", err)
		}

		messages = append(messages, pregel.Message[message.Message]{
			From: spm.From,
			To:   spm.To,
			Data: msg,
		})
	}

	if len(messages) == 0 {
		return nil, nil
	}

	return messages, nil
}

// Clear removes all messages for the given vertex.
func (bus *MessageMessageBus) Clear(vertex string) error {
	if bus.closed {
		return pregel.ErrMessageBusClosed
	}

	ctx := context.Background()

	pipe := bus.client.Pipeline()
	pipe.Del(ctx, bus.mailboxKey(vertex))
	pipe.SRem(ctx, bus.frontierKey(), vertex)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear mailbox for %q: %w", vertex, err)
	}

	return nil
}

// Pending returns the vertices that have messages waiting.
func (bus *MessageMessageBus) Pending() ([]string, error) {
	if bus.closed {
		return nil, pregel.ErrMessageBusClosed
	}

	ctx := context.Background()
	members, err := bus.client.SMembers(ctx, bus.frontierKey()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending vertices: %w", err)
	}

	return members, nil
}

// CleanNamespace removes all keys associated with this bus's namespace.
func (bus *MessageMessageBus) CleanNamespace() error {
	if bus.closed {
		return pregel.ErrMessageBusClosed
	}

	ctx := context.Background()

	// Find all mailbox keys for this namespace
	pattern := fmt.Sprintf("mailbox:%s:*", bus.namespace)
	iter := bus.client.Scan(ctx, 0, pattern, 0).Iterator()

	keys := []string{bus.frontierKey()} // Always clean frontier
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to scan keys: %w", err)
	}

	if len(keys) > 0 {
		if err := bus.client.Del(ctx, keys...).Err(); err != nil {
			return fmt.Errorf("failed to delete keys: %w", err)
		}
	}

	return nil
}

// Stats returns statistics about the message bus state.
func (bus *MessageMessageBus) Stats() (map[string]int, error) {
	if bus.closed {
		return nil, pregel.ErrMessageBusClosed
	}

	ctx := context.Background()
	stats := make(map[string]int)

	// Count frontier size
	frontierSize, err := bus.client.SCard(ctx, bus.frontierKey()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get frontier size: %w", err)
	}
	stats["frontier_size"] = int(frontierSize)

	// Count total mailbox keys
	pattern := fmt.Sprintf("mailbox:%s:*", bus.namespace)
	iter := bus.client.Scan(ctx, 0, pattern, 0).Iterator()
	mailboxCount := 0
	for iter.Next(ctx) {
		mailboxCount++
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan mailbox keys: %w", err)
	}
	stats["mailbox_count"] = mailboxCount

	return stats, nil
}

// Close releases resources associated with the bus.
func (bus *MessageMessageBus) Close() error {
	if bus.closed {
		return nil
	}
	bus.closed = true
	return bus.client.Close()
}
