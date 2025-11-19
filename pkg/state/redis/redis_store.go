package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/redis/go-redis/v9"
)

// Store implements state.Store using Redis as the backend.
// Values are serialized to JSON for storage.
type Store struct {
	client redis.UniversalClient
	prefix string
}

// Option configures Store behavior.
type Option func(*Store)

// WithKeyPrefix sets a namespace prefix for all keys (default: "agentmesh:state:").
func WithKeyPrefix(prefix string) Option {
	return func(rs *Store) {
		rs.prefix = prefix
	}
}

// NewStore creates a Redis-backed state store.
// The client can be a regular Redis client, cluster client, or sentinel client.
//
// Example:
//
//	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	store := redis.NewStore(client, redis.WithKeyPrefix("myapp:"))
func NewStore(client redis.UniversalClient, opts ...Option) *Store {
	rs := &Store{
		client: client,
		prefix: "agentmesh:state:",
	}

	for _, opt := range opts {
		opt(rs)
	}

	return rs
}

// prefixKey adds the namespace prefix to a key.
func (rs *Store) prefixKey(key string) string {
	return rs.prefix + key
}

// Get retrieves a value from Redis and deserializes it.
func (rs *Store) Get(ctx context.Context, key string) (any, error) {
	data, err := rs.client.Get(ctx, rs.prefixKey(key)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, state.ErrKeyNotFound
		}
		return nil, fmt.Errorf("redis get failed: %w", err)
	}

	var value any
	if err := json.Unmarshal([]byte(data), &value); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	return value, nil
}

// Set serializes a value to JSON and stores it in Redis.
func (rs *Store) Set(ctx context.Context, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("json marshal failed: %w", err)
	}

	if err := rs.client.Set(ctx, rs.prefixKey(key), data, 0).Err(); err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}

	return nil
}

// Delete removes a key from Redis.
func (rs *Store) Delete(ctx context.Context, key string) error {
	result := rs.client.Del(ctx, rs.prefixKey(key))
	if err := result.Err(); err != nil {
		return fmt.Errorf("redis del failed: %w", err)
	}

	if result.Val() == 0 {
		return state.ErrKeyNotFound
	}

	return nil
}

// Keys returns all keys in the store matching the prefix.
func (rs *Store) Keys(ctx context.Context) ([]string, error) {
	var keys []string
	pattern := rs.prefix + "*"

	iter := rs.client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		// Strip prefix from key
		fullKey := iter.Val()
		if len(fullKey) >= len(rs.prefix) {
			keys = append(keys, fullKey[len(rs.prefix):])
		}
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("redis scan failed: %w", err)
	}

	return keys, nil
}

// Snapshot creates a point-in-time capture of all state.
// This performs a full scan of Redis keys matching the prefix.
func (rs *Store) Snapshot(ctx context.Context) (map[string]any, error) {
	keys, err := rs.Keys(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	snapshot := make(map[string]any, len(keys))
	for _, key := range keys {
		value, err := rs.Get(ctx, key)
		if err != nil {
			if errors.Is(err, state.ErrKeyNotFound) {
				// Key was deleted between scan and get, skip it
				continue
			}
			return nil, fmt.Errorf("failed to get key %q: %w", key, err)
		}
		snapshot[key] = value
	}

	return snapshot, nil
}

// Restore loads state from a snapshot.
// This performs a pipeline write for better performance.
func (rs *Store) Restore(ctx context.Context, snapshot map[string]any) error {
	if len(snapshot) == 0 {
		return nil
	}

	// Use pipeline for efficient batch writes
	pipe := rs.client.Pipeline()

	for key, value := range snapshot {
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("json marshal failed for key %q: %w", key, err)
		}
		pipe.Set(ctx, rs.prefixKey(key), data, 0)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline exec failed: %w", err)
	}

	return nil
}

// Close closes the Redis connection.
func (rs *Store) Close() error {
	return rs.client.Close()
}

// Ping checks if the Redis connection is alive.
func (rs *Store) Ping(ctx context.Context) error {
	return rs.client.Ping(ctx).Err()
}

// Clear removes all keys with the configured prefix.
// WARNING: This is a destructive operation. Use with caution.
func (rs *Store) Clear(ctx context.Context) error {
	keys, err := rs.Keys(ctx)
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	if len(keys) == 0 {
		return nil
	}

	// Convert to prefixed keys
	prefixedKeys := make([]string, len(keys))
	for i, key := range keys {
		prefixedKeys[i] = rs.prefixKey(key)
	}

	if err := rs.client.Del(ctx, prefixedKeys...).Err(); err != nil {
		return fmt.Errorf("redis del failed: %w", err)
	}

	return nil
}

// Compile-time check that Store implements state.Store.
var _ state.Store = (*Store)(nil)
