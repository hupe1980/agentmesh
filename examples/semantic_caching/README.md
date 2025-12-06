# Semantic Caching Example

This example demonstrates both **exact-match caching** and **semantic caching** for LLM responses.

## Overview

AgentMesh provides two caching strategies:

### 1. Exact-Match Cache (Fast)
- Uses SHA256 hashing for instant lookups
- Perfect for repeated identical queries
- No external dependencies
- Ideal for: FAQs, deterministic queries, testing

### 2. Semantic Cache (Smart)
- Uses embeddings to find similar prompts
- Handles rephrased questions
- Requires an embedder (e.g., OpenAI text-embedding-3-small)
- Ideal for: Chatbots, customer support, varied phrasing

## Running the Example

```bash
export OPENAI_API_KEY=your_api_key_here
go run main.go
```

## Example Output

```
=== Exact-Match Cache Demo ===
Query 1: "What is Python?"
Response: [LLM response about Python]
Cache stats: Hits=0, Misses=1

Query 2: "What is Python?" (identical)
Response: [Same response, from cache]
Cache stats: Hits=1, Misses=1

Query 3: "Tell me about Python" (different wording)
Response: [New LLM call - cache miss]
Cache stats: Hits=1, Misses=2

=== Semantic Cache Demo ===
Query 1: "What is Python?"
Response: [LLM response about Python]
Cache stats: Hits=0, Misses=1

Query 2: "Tell me about Python" (85% similar)
Response: [Same response, from cache!]
Cache stats: Hits=1, Misses=1, Similarity=0.87

Query 3: "Explain Python programming" (82% similar)
Response: [Same response, from cache!]
Cache stats: Hits=2, Misses=1, Similarity=0.84
```

## Key Differences

| Feature | Exact-Match | Semantic |
|---------|-------------|----------|
| Speed | Instant (hash lookup) | Fast (embedding + similarity) |
| Memory | Low | Medium (stores embeddings) |
| Dependencies | None | Embedder required |
| Cache Hit Rate | Lower (exact only) | Higher (similar queries) |
| Cost Savings | Good | Excellent |

## When to Use Which

**Use Exact-Match when:**
- Queries are repeated exactly (FAQs, tests)
- You want zero dependencies
- Maximum speed is critical
- Simple is better

**Use Semantic when:**
- Users rephrase questions frequently
- Natural conversation flow
- Multilingual support (with multilingual embedders)
- Maximum cost savings from cache hits

## Configuration

### Exact-Match Cache (via Middleware)
```go
import modelmw "github.com/hupe1980/agentmesh/pkg/model/middleware"

// Model middleware cache - fast hash-based lookups
cacheMiddleware := modelmw.NewCacheMiddleware()

// Use with agent
agent, _ := agent.NewReAct(model, tools,
    agent.WithModelMiddleware(cacheMiddleware))
```

### Semantic Cache
```go
import (
    "github.com/hupe1980/agentmesh/pkg/cache"
    "github.com/hupe1980/agentmesh/pkg/embedding/openai"
)

embedder := openai.NewEmbedder(func(o *openai.Options) {
    o.Model = "text-embedding-3-small"
})

semanticCache := cache.NewMemory(embedder,
    cache.WithSimilarityThreshold(0.85), // 85% similar = cache hit
    cache.WithTTL(time.Hour),            // entries expire after 1h
    cache.WithMaxSize(1000))             // LRU eviction at 1000 entries
```

### Redis Backend (Distributed)
```go
import (
    "github.com/hupe1980/agentmesh/pkg/cache"
    redisCache "github.com/hupe1980/agentmesh/pkg/cache/redis"
)

redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})

semanticCache := redisCache.NewCache(redisClient, embedder,
    cache.WithSimilarityThreshold(0.85),
    redisCache.WithKeyPrefix("myapp:llm:"))
```

## Architecture

```
Request → Middleware Chain → Model
              ↓
       CacheMiddleware
              ↓
      [Check Cache] ← Exact hash match
              ↓
      Hit? → Return cached response (skip model)
      Miss? → Call model + cache response
```

```
Request → SemanticCache
              ↓
      [Embed Request] ← Convert to vector
              ↓
      [Similarity Search] ← Find similar vectors
              ↓
      >85% match? → Return cached response (skip model)
      <85% match? → Call model + cache response
```
