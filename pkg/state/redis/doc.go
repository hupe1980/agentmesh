// Package redis provides a Redis-backed implementation of state.Store.
//
// Store uses JSON serialization for values and supports all Redis
// deployment modes: standalone, cluster, and sentinel.
//
// Example usage:
//
//	import (
//		"github.com/hupe1980/agentmesh/pkg/state"
//		stateRedis "github.com/hupe1980/agentmesh/pkg/state/redis"
//		"github.com/redis/go-redis/v9"
//	)
//
//	// Create Redis client
//	client := redis.NewClient(&redis.Options{
//		Addr: "localhost:6379",
//	})
//
//	// Create Redis store
//	store := stateRedis.NewStore(client)
//
//	// Create manager with Redis backend
//	manager := state.NewManager(
//		state.WithStore(store),
//	)
//
// For distributed deployments, use Redis Cluster:
//
//	client := redis.NewClusterClient(&redis.ClusterOptions{
//		Addrs: []string{"localhost:7000", "localhost:7001", "localhost:7002"},
//	})
//	store := stateRedis.NewStore(client)
//
// Key Prefixing:
//
// By default, all keys are prefixed with "agentmesh:state:" for namespace isolation.
// You can customize the prefix:
//
//	store := stateRedis.NewStore(client,
//		stateRedis.WithKeyPrefix("myapp:state:"),
//	)
//
// Performance Considerations:
//
//   - Snapshot() performs a full scan of keys - may be slow for large datasets
//   - Restore() uses pipelined writes for better performance
//   - Consider using Redis persistence (AOF/RDB) for durability
//   - For multi-region deployments, consider Redis Enterprise Active-Active
//
// Serialization:
//
// Values are serialized to JSON. Complex types (structs, maps, slices) are
// supported. Ensure your types are JSON-serializable.
package redis
