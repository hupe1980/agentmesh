# Example: OpenAI Embedder

## Overview
Demonstrates text-to-vector embedding using OpenAI's embedding models. Shows how to convert text into semantic vectors for similarity search and RAG (Retrieval-Augmented Generation) workflows.

## Key Concepts
- **Text Embeddings**: Convert text to vector representations
- **Semantic Search**: Find similar content
- **OpenAI Integration**: Production-grade embeddings
- **Batch Processing**: Efficient multi-text embedding

## Prerequisites
```bash
export OPENAI_API_KEY="sk-..."
```

## Running
```bash
cd examples/openai_embedder
go run main.go
```

## Expected Output
```
=== OpenAI Embedder Example ===

Creating OpenAI embedder...
✓ Embedder created
  Model: text-embedding-3-small
  Dimensions: 1536

Example 1: Single Text Embedding
  Text: "The quick brown fox jumps over the lazy dog"
  Embedding dimensions: 1536
  First 5 values: [0.023, -0.041, 0.012, 0.067, -0.019]

Example 2: Batch Embedding
  Processing 3 texts...
  Embedding 1: [1536 dims] "Machine learning is fascinating"
  Embedding 2: [1536 dims] "AI will transform the world"
  Embedding 3: [1536 dims] "The weather is nice today"

Example 3: Semantic Similarity
  Query: "Artificial intelligence applications"
  
  Comparing with documents:
    Doc 1: "Machine learning is fascinating"
      Similarity: 0.87 (High - related topic)
    Doc 2: "AI will transform the world"
      Similarity: 0.92 (Very High - highly related)
    Doc 3: "The weather is nice today"
      Similarity: 0.12 (Low - unrelated)

Most similar: Doc 2 (0.92)
```

## Code Walkthrough

### 1. Create Embedder
```go
import (
    "github.com/hupe1980/agentmesh/pkg/embedding/openai"
    openaiSDK "github.com/openai/openai-go"
)

client := openaiSDK.NewClient()

embedder := openai.NewEmbedder(client,
    openai.WithModel("text-embedding-3-small"),
)
```

### 2. Single Text Embedding
```go
vector, err := embedder.Embed(ctx, "Hello, world!")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Dimensions: %d\n", len(vector))
fmt.Printf("First values: %v\n", vector[:5])
```

### 3. Batch Embedding
```go
texts := []string{
    "First document",
    "Second document",
    "Third document",
}

embeddings, err := embedder.EmbedBatch(ctx, texts)
// Returns [][]float64 - one vector per text
```

### 4. Calculate Similarity
```go
func cosineSimilarity(a, b []float64) float64 {
    var dotProduct, normA, normB float64
    
    for i := range a {
        dotProduct += a[i] * b[i]
        normA += a[i] * a[i]
        normB += b[i] * b[i]
    }
    
    return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

similarity := cosineSimilarity(queryVector, docVector)
fmt.Printf("Similarity: %.2f\n", similarity)
```

## Embedding Models

### text-embedding-3-small
- Dimensions: 1536
- Cost: Lower
- Performance: Good for most use cases

### text-embedding-3-large
- Dimensions: 3072
- Cost: Higher
- Performance: Best quality

### text-embedding-ada-002 (Legacy)
- Dimensions: 1536
- Cost: Lowest
- Performance: Older model

## Use Cases

### Semantic Search
```go
// 1. Embed documents
docs := []string{"doc1", "doc2", "doc3"}
docEmbeddings, _ := embedder.EmbedBatch(ctx, docs)

// 2. Embed query
queryEmbedding, _ := embedder.Embed(ctx, "search query")

// 3. Find most similar
bestMatch := findMostSimilar(queryEmbedding, docEmbeddings)
```

### RAG (Retrieval-Augmented Generation)
```go
// 1. Embed knowledge base
knowledge := loadKnowledgeBase()
kbEmbeddings, _ := embedder.EmbedBatch(ctx, knowledge)

// 2. Embed user query
query := "What is AgentMesh?"
queryEmb, _ := embedder.Embed(ctx, query)

// 3. Retrieve relevant docs
relevant := retrieveTopK(queryEmb, kbEmbeddings, 5)

// 4. Generate answer with context
answer := generateWithContext(query, relevant)
```

### Clustering
```go
// Group similar documents
embeddings, _ := embedder.EmbedBatch(ctx, documents)
clusters := kMeansClustering(embeddings, numClusters)
```

### Recommendation
```go
// Find similar items
itemEmbedding, _ := embedder.Embed(ctx, currentItem)
similar := findSimilarItems(itemEmbedding, allItemEmbeddings)
```

## What This Example Teaches
- ✅ OpenAI embedding API integration
- ✅ Text-to-vector conversion
- ✅ Batch processing optimization
- ✅ Similarity calculation
- ✅ RAG workflow basics

## Performance Optimization

### Batch Processing
```go
// Good: Batch embed
embeddings, _ := embedder.EmbedBatch(ctx, texts)

// Bad: Individual embeds (slower, more expensive)
for _, text := range texts {
    embedding, _ := embedder.Embed(ctx, text)
}
```

### Caching
```go
// Cache embeddings to avoid re-computing
cache := make(map[string][]float64)

func getEmbedding(text string) ([]float64, error) {
    if cached, ok := cache[text]; ok {
        return cached, nil
    }
    
    embedding, err := embedder.Embed(ctx, text)
    if err != nil {
        return nil, err
    }
    
    cache[text] = embedding
    return embedding, nil
}
```

### Dimensionality
```go
// Check embedding dimensions
dims := embedder.Dimensions() // 1536 or 3072

// Ensure storage can handle
vectorDB.CreateIndex(dims)
```

## Next Steps
- Build semantic search application
- Implement RAG system
- Create document clustering
- See **examples/basic_agent** for agent integration

## See Also
- [pkg/embedding/openai](../../pkg/embedding/openai) - OpenAI embedder
- [pkg/embedding](../../pkg/embedding) - Embedding interface
- [pkg/retrieval](../../pkg/retrieval) - Document retrieval
- [OpenAI Embeddings Guide](https://platform.openai.com/docs/guides/embeddings)
