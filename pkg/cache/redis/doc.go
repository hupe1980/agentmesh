// Package redis provides a distributed semantic cache using Redis with vector search.
//
// This implementation requires Redis Stack or Redis Enterprise with the RediSearch module
// for vector similarity search capabilities.
//
// # Basic Usage
//
//	import (
//	    "github.com/hupe1980/agentmesh/pkg/cache/redis"
//	    "github.com/redis/go-redis/v9"
//	)
//
//	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	cache := redis.NewCache(client, embedder, redis.WithKeyPrefix("llm:cache:"))
//
// # Requirements
//
// - Redis Stack 7.2+ or Redis Enterprise with RediSearch module
// - go-redis/v9 client library
//
// # Features
//
//   - Distributed caching across multiple instances
//   - Vector similarity search using RediSearch
//   - Automatic TTL expiration
//   - Configurable key prefixes for namespace isolation
package redis
