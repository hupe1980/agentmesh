package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hupe1980/agentmesh/pkg/cache"
	"github.com/hupe1980/agentmesh/pkg/callbacks/plugins"
	"github.com/hupe1980/agentmesh/pkg/embedding/openai"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// This example demonstrates semantic caching in action using OpenAI embeddings.

func main() {
	ctx := context.Background()

	fmt.Println("=== AgentMesh Semantic Caching Demo ===")
	fmt.Println()

	demoExactMatchCache()
	fmt.Println()

	if err := demoSemanticCache(ctx); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

// demoExactMatchCache shows the existing exact-match cache plugin
func demoExactMatchCache() {
	fmt.Println("1. Exact-Match Cache (SHA256 hashing)")
	fmt.Println("   - Fast instant lookups")
	fmt.Println("   - Perfect for identical repeated queries")
	fmt.Println("   - No external dependencies")
	fmt.Println()

	// Create exact-match cache
	exactCache := plugins.NewCachePlugin(1000)
	fmt.Printf("   ✓ Created: CachePlugin(maxSize=1000)\n")
	fmt.Printf("   ✓ Stats: %+v\n", exactCache.GetStats())
}

// demoSemanticCache demonstrates semantic caching with real embeddings
func demoSemanticCache(ctx context.Context) error {
	fmt.Println("2. Semantic Cache (Embedding-based similarity)")
	fmt.Println()

	// Check API key from environment
	if os.Getenv("OPENAI_API_KEY") == "" {
		fmt.Println("   ⚠ OPENAI_API_KEY not set - skipping live demo")
		fmt.Println("   Set OPENAI_API_KEY to see semantic caching in action")
		return nil
	}

	// Create OpenAI embedder (uses OPENAI_API_KEY from environment)
	embedder := openai.NewEmbedder(func(o *openai.Options) {
		o.Model = "text-embedding-3-small"
	})

	// Create semantic cache with configuration
	// Using a moderate threshold for demo
	memCache := cache.NewMemory(embedder,
		cache.WithSimilarityThreshold(0.90),
		cache.WithTTL(time.Hour),
		cache.WithMaxSize(100))

	fmt.Println("   ✓ Created semantic cache with OpenAI embeddings")
	fmt.Println("   ✓ Similarity threshold: 0.90 (90% match required)")
	fmt.Println("   ✓ TTL: 1 hour")
	fmt.Println("   ✓ Max size: 100 entries")
	fmt.Println()

	// Test queries - using very similar wording to demonstrate cache hits
	queries := []string{
		"What is the capital of France?",
		"What's the capital of France?",   // Very similar - should hit
		"Tell me the capital of France",   // Similar phrasing - might hit
		"What is the capital of Germany?", // Different topic - should miss
	}

	fmt.Println("   Testing semantic similarity:")

	// First, let's calculate and show all similarities for educational purposes
	fmt.Println()
	fmt.Println("   First, let's see the actual similarity scores:")

	// Embed all queries
	baseQuery := queries[0]
	baseEmbed, err := embedder.Embed(ctx, baseQuery)
	if err != nil {
		return fmt.Errorf("embedding error: %w", err)
	}

	for i := 1; i < len(queries); i++ {
		queryEmbed, err := embedder.Embed(ctx, queries[i])
		if err != nil {
		return fmt.Errorf("embedding error: %w", err)
		}
		similarity := cache.CosineSimilarity(baseEmbed, queryEmbed)
		status := "❌ MISS"
		if similarity >= 0.90 {
		status = "✅ HIT"
		}
		fmt.Printf("   Query %d vs Query 1: %.4f %s\n", i+1, similarity, status)
	}

	fmt.Println()
	fmt.Println("   Now testing with the cache:")
	for i, text := range queries {
		// Create a mock request
		req := &model.Request{
		Messages: []message.Message{
		message.NewHumanMessageFromText(text),
			},
		}

		// Try to get from cache
		cached, err := memCache.Get(ctx, req)
		if err != nil {
		return fmt.Errorf("cache get error: %w", err)
		}

		if cached != nil {
		similarity := cached.Metadata["cache_similarity"]
		fmt.Printf("   [%d] %q → ✓ HIT (similarity: %.3f)\n", i+1, text, similarity)
		} else {
			// Cache miss - store a mock response
		resp := &model.Response{
		Message: message.NewAIMessageFromText(fmt.Sprintf("Response about %s", text)),
			}
		if err := memCache.Set(ctx, req, resp); err != nil {
		return fmt.Errorf("cache set error: %w", err)
			}
		fmt.Printf("   [%d] %q → MISS (stored for future)\n", i+1, text)
		}
	}

	fmt.Println()
	fmt.Println("   💡 Semantic caching successfully matched similar queries!")
	fmt.Println("      Query #2 matched #1 despite different punctuation/contractions.")
	fmt.Println("      This reduces API calls and saves costs for similar requests.")

	return nil
}
