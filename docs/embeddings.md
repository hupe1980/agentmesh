---
layout: doc
title: Embeddings
description: Convert text to vectors for semantic search and similarity detection.
permalink: /embeddings/
hero:
  title: Text Embeddings
  description: Convert text into dense vectors for semantic search, similarity detection, and RAG pipelines.
  primary_cta:
    label: Create an embedder
    href: "#embedders"
  secondary_cta:
    label: API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/embedding"
    external: true
sidebar:
  - title: What are Embeddings?
    url: "#what-are-embeddings"
  - title: Embedders
    url: "#embedders"
  - title: Use Cases
    url: "#use-cases"
  - title: Best Practices
    url: "#best-practices"
---

## What are Embeddings? {#what-are-embeddings}

Embeddings are dense vector representations of text that capture semantic meaning. Unlike simple keyword matching, embeddings allow you to find conceptually similar content even when different words are used.

**Key Properties**:
- **Dense vectors**: Typically 256-3072 dimensions of floating-point numbers
- **Semantic similarity**: Similar concepts have similar vectors (measured by cosine similarity)
- **Fixed size**: All text inputs produce vectors of the same dimensionality
- **Continuous space**: Smooth transitions between related concepts

**Example**:
```
"dog" → [0.23, -0.45, 0.67, ..., 0.12]  (384 dimensions)
"puppy" → [0.25, -0.43, 0.69, ..., 0.15]  (similar vector!)
"cat" → [0.31, -0.38, 0.58, ..., 0.19]   (somewhat similar)
"car" → [-0.12, 0.67, -0.34, ..., 0.89]  (very different)
```

---

## Use Cases

### 1. Semantic Search

Find documents based on meaning rather than exact keywords:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/embedding/openai"
    "github.com/hupe1980/agentmesh/pkg/memory"
)

// Create embedder
embedder := openai.NewEmbedder(func(o *openai.Options) {
    o.Model = "text-embedding-3-small"
})

// Create vector memory for semantic search
vectorMem := memory.NewVectorMemory(embedder)

// Store documents
vectorMem.Add(ctx, "session1", message.NewHumanMessage("Python is a programming language"))
vectorMem.Add(ctx, "session1", message.NewHumanMessage("JavaScript runs in browsers"))
vectorMem.Add(ctx, "session1", message.NewHumanMessage("Dogs are popular pets"))

// Semantic search - finds Python document even though query uses different words
results, _ := vectorMem.Recall(ctx, "session1", &memory.RecallFilter{
    Query: "coding language",
    K:     2,
})
// Results: "Python is a programming language", "JavaScript runs in browsers"
```

### 2. Similarity Detection

Find duplicate or related content:

```go
func checkSimilarity(embedder embedding.Embedder, text1, text2 string) (float64, error) {
    vec1, err := embedder.Embed(ctx, text1)
    if err != nil {
        return 0, err
    }
    
    vec2, err := embedder.Embed(ctx, text2)
    if err != nil {
        return 0, err
    }
    
    return cosineSimilarity(vec1, vec2), nil
}

// Check if two customer inquiries are similar
similarity, _ := checkSimilarity(embedder,
    "How do I reset my password?",
    "I forgot my login credentials",
)
// similarity ≈ 0.85 (high similarity, likely same intent)
```

### 3. Retrieval-Augmented Generation (RAG)

Enhance LLM responses with relevant context:

```go
import "github.com/hupe1980/agentmesh/pkg/agent"

// Create RAG agent with vector memory
embedder := openai.NewEmbedder()
vectorMem := memory.NewVectorMemory(embedder)

// Load knowledge base
vectorMem.Add(ctx, "kb", message.NewHumanMessage("AgentMesh uses Pregel BSP for graph execution"))
vectorMem.Add(ctx, "kb", message.NewHumanMessage("Checkpointing enables time-travel debugging"))
vectorMem.Add(ctx, "kb", message.NewHumanMessage("Tools allow agents to call external APIs"))

// Create retriever
retriever := &memoryRetriever{memory: vectorMem, sessionID: "kb"}

// RAG agent automatically finds relevant docs and includes them in context
ragAgent, _ := agent.NewRAGAgent(model, retriever)

// Query uses retrieved context
msgs := []message.Message{
    message.NewHumanMessage("How does AgentMesh handle execution?"),
}
messages, _ := agent.CollectMessages(ragAgent.Run(ctx, msgs))
// Response includes information about Pregel BSP from knowledge base
```

### 4. Clustering and Classification

Group similar items or route requests:

```go
// Route customer inquiries to appropriate department
func routeInquiry(embedder embedding.Embedder, inquiry string) string {
    vec, _ := embedder.Embed(ctx, inquiry)
    
    departments := map[string][]float64{
        "billing":   billingEmbedding,
        "technical": technicalEmbedding,
        "sales":     salesEmbedding,
    }
    
    bestDept := ""
    bestScore := 0.0
    
    for dept, deptVec := range departments {
        score := cosineSimilarity(vec, deptVec)
        if score > bestScore {
            bestScore = score
            bestDept = dept
        }
    }
    
    return bestDept
}
```

---

## Available Embedders

### SimpleEmbedder (Testing)

A deterministic, hash-based embedder for testing and development:

```go
import "github.com/hupe1980/agentmesh/pkg/embedding"

// Create simple embedder with 384 dimensions
embedder := embedding.NewSimpleEmbedder(384)

// Produces consistent, normalized vectors
vec, err := embedder.Embed(ctx, "test text")
// vec: [0.123, -0.456, 0.789, ...] (length = 384)
```

**Characteristics**:
- ✅ No API keys required
- ✅ Deterministic (same input = same output)
- ✅ Normalized vectors (magnitude = 1.0)
- ⚠️ Not semantically meaningful
- ⚠️ Testing/development only

**Use Cases**:
- Unit tests without external API dependencies
- Local development without API costs
- Proof-of-concept implementations
- CI/CD pipelines

```go
func TestVectorMemory(t *testing.T) {
    embedder := embedding.NewSimpleEmbedder(128)
    mem := memory.NewVectorMemory(embedder)
    
    // Test without OpenAI API
    mem.Add(ctx, "test", message.NewHumanMessage("hello"))
    results, err := mem.Recall(ctx, "test", &memory.RecallFilter{
        Query: "hello",
        K:     1,
    })
    
    require.NoError(t, err)
    require.Len(t, results, 1)
}
```

### OpenAI Embedder (Production)

High-quality semantic embeddings from OpenAI:

```go
import "github.com/hupe1980/agentmesh/pkg/embedding/openai"

// Basic usage with defaults
embedder := openai.NewEmbedder()

// Custom configuration
embedder := openai.NewEmbedder(func(o *openai.Options) {
    o.Model = "text-embedding-3-large"  // Higher quality
    o.Dimensions = 1024                  // Reduced dimensions for cost
})

// Embed single text
vec, err := embedder.Embed(ctx, "semantic search query")

// Batch embed for efficiency
texts := []string{
    "document 1",
    "document 2", 
    "document 3",
}
vecs, err := embedder.EmbedBatch(ctx, texts)
```

**Supported Models**:

| Model | Dimensions | Cost (per 1M tokens) | Quality | Speed |
|-------|-----------|----------------------|---------|-------|
| `text-embedding-3-small` | 1536 | $0.02 | Good | Fast |
| `text-embedding-3-large` | 3072 | $0.13 | Excellent | Slower |
| `text-embedding-ada-002` | 1536 | $0.10 | Good | Fast |

**Dimension Reduction**:

```go
// Use fewer dimensions for cost/performance trade-off
embedder := openai.NewEmbedder(func(o *openai.Options) {
    o.Model = "text-embedding-3-large"
    o.Dimensions = 256  // Reduce from 3072 to 256
})

// Still maintains good semantic quality at lower cost/size
```

**API Key Setup**:

```bash
# Set environment variable
export OPENAI_API_KEY="sk-..."

# Or in code
client := openai.NewClient(
    option.WithAPIKey("sk-..."),
)
embedder := openai.NewEmbedderFromClient(client)
```

For complete configuration options, see [pkg/embedding/openai/README.md](https://github.com/hupe1980/agentmesh/blob/main/pkg/embedding/openai/README.md).

---

## Vector Memory Integration

AgentMesh provides `VectorMemory` for semantic conversation history:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/embedding/openai"
    "github.com/hupe1980/agentmesh/pkg/memory"
    "github.com/hupe1980/agentmesh/pkg/message"
)

// Create vector memory with OpenAI embeddings
embedder := openai.NewEmbedder()
vectorMem := memory.NewVectorMemory(embedder)

// Store multi-session conversations
vectorMem.Add(ctx, "user123", message.NewHumanMessage("I love Python"))
vectorMem.Add(ctx, "user123", message.NewAIMessage("Python is great for data science!"))
vectorMem.Add(ctx, "user456", message.NewHumanMessage("JavaScript is my favorite"))

// Semantic recall across sessions
results, err := vectorMem.Recall(ctx, "user123", &memory.RecallFilter{
    Query:    "programming languages",
    K:        5,
    MinScore: 0.7,  // Only high similarity matches
})
```

### Advanced Filtering

```go
// Time-based filtering
oneDayAgo := time.Now().Add(-24 * time.Hour)
results, _ := vectorMem.Recall(ctx, "session", &memory.RecallFilter{
    Query: "recent updates",
    K:     10,
    After: &oneDayAgo,  // Only messages after yesterday
})

// Message type filtering
results, _ := vectorMem.Recall(ctx, "session", &memory.RecallFilter{
    Query: "user questions",
    K:     10,
    Types: []message.Type{message.TypeHuman},  // Only user messages
})

// Metadata filtering
results, _ := vectorMem.Recall(ctx, "session", &memory.RecallFilter{
    Query: "important notes",
    K:     10,
    Metadata: map[string]string{
        "priority": "high",
        "category": "technical",
    },
})
```

### Integration with Agents

```go
// ReAct agent with long-term memory
embedder := openai.NewEmbedder()
vectorMem := memory.NewVectorMemory(embedder)

// Load historical context
vectorMem.Add(ctx, "user", message.NewHumanMessage("My project uses TypeScript"))
vectorMem.Add(ctx, "user", message.NewHumanMessage("I prefer functional programming"))

// Create agent
agent, _ := agent.NewReActAgent(model,
    agent.WithTools(searchTool, calculatorTool),
)

// Before each request, recall relevant history
history, _ := vectorMem.Recall(ctx, "user", &memory.RecallFilter{
    Query: currentUserMessage,
    K:     3,
})

// Prepend history to conversation
messages := append(history, currentMessages...)
results, err := agent.CollectMessages(agent.Run(ctx, messages))
if err != nil {
    log.Fatal(err)
}
```

---

## Best Practices

### 1. Choose Appropriate Dimensions

**High Dimensions** (1536-3072):
- ✅ Better semantic quality
- ✅ More nuanced similarity detection
- ⚠️ Higher storage costs
- ⚠️ Slower similarity searches
- **Use for**: Production semantic search, high-quality RAG

**Low Dimensions** (256-512):
- ✅ Faster similarity computation
- ✅ Lower storage requirements
- ⚠️ Slightly reduced quality
- **Use for**: Large-scale systems, real-time search

```go
// Production: balance quality and performance
embedder := openai.NewEmbedder(func(o *openai.Options) {
    o.Model = "text-embedding-3-small"  // 1536 dimensions
})

// High-scale: optimize for speed
embedder := openai.NewEmbedder(func(o *openai.Options) {
    o.Model = "text-embedding-3-large"
    o.Dimensions = 512  // Reduced from 3072
})
```

### 2. Normalize Vectors

Always normalize embeddings for cosine similarity:

```go
func normalize(vec []float64) []float64 {
    var magnitude float64
    for _, v := range vec {
        magnitude += v * v
    }
    magnitude = math.Sqrt(magnitude)
    
    normalized := make([]float64, len(vec))
    for i, v := range vec {
        normalized[i] = v / magnitude
    }
    return normalized
}

// AgentMesh embedders return normalized vectors by default
vec, _ := embedder.Embed(ctx, "text")
// magnitude(vec) == 1.0
```

### 3. Batch for Efficiency

Batch embedding reduces API calls and latency:

```go
// ❌ Inefficient: Multiple API calls
for _, doc := range documents {
    vec, _ := embedder.Embed(ctx, doc)
    store(vec)
}

// ✅ Efficient: Single batched API call
vecs, _ := embedder.EmbedBatch(ctx, documents)
for i, vec := range vecs {
    store(documents[i], vec)
}
```

**OpenAI Batch Limits**:
- Max 2048 inputs per batch
- Max 8191 tokens per input

### 4. Cache Embeddings

Embeddings are expensive - cache them:

```go
type CachedEmbedder struct {
    embedder embedding.Embedder
    cache    map[string][]float64
    mu       sync.RWMutex
}

func (ce *CachedEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
    // Check cache first
    ce.mu.RLock()
    if vec, ok := ce.cache[text]; ok {
        ce.mu.RUnlock()
        return vec, nil
    }
    ce.mu.RUnlock()
    
    // Compute and cache
    vec, err := ce.embedder.Embed(ctx, text)
    if err != nil {
        return nil, err
    }
    
    ce.mu.Lock()
    ce.cache[text] = vec
    ce.mu.Unlock()
    
    return vec, nil
}
```

### 5. Monitor Quality

Regularly validate embedding quality:

```go
func validateEmbeddings(embedder embedding.Embedder) error {
    // Test semantic similarity
    dog, _ := embedder.Embed(ctx, "dog")
    puppy, _ := embedder.Embed(ctx, "puppy")
    car, _ := embedder.Embed(ctx, "car")
    
    dogPuppySim := cosineSimilarity(dog, puppy)
    dogCarSim := cosineSimilarity(dog, car)
    
    // Similar concepts should have high similarity
    if dogPuppySim < 0.7 {
        return fmt.Errorf("dog-puppy similarity too low: %f", dogPuppySim)
    }
    
    // Unrelated concepts should have low similarity
    if dogCarSim > 0.3 {
        return fmt.Errorf("dog-car similarity too high: %f", dogCarSim)
    }
    
    return nil
}
```

### 6. Handle Rate Limits

Implement retry logic for production:

```go
func embedWithRetry(embedder embedding.Embedder, text string, maxRetries int) ([]float64, error) {
    backoff := time.Second
    
    for attempt := 0; attempt < maxRetries; attempt++ {
        vec, err := embedder.Embed(ctx, text)
        if err == nil {
            return vec, nil
        }
        
        // Check if rate limited
        if strings.Contains(err.Error(), "rate_limit") {
            time.Sleep(backoff)
            backoff *= 2  // Exponential backoff
            continue
        }
        
        return nil, err  // Non-retryable error
    }
    
    return nil, fmt.Errorf("max retries exceeded")
}
```

### 7. Preprocess Text

Clean text before embedding:

```go
func preprocessText(text string) string {
    // Remove excessive whitespace
    text = strings.TrimSpace(text)
    text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
    
    // Remove special characters if needed
    text = regexp.MustCompile(`[^\w\s.,!?-]`).ReplaceAllString(text, "")
    
    // Convert to lowercase for consistency (optional)
    text = strings.ToLower(text)
    
    return text
}

// Use preprocessed text
cleanText := preprocessText(userInput)
vec, _ := embedder.Embed(ctx, cleanText)
```

### 8. Test with SimpleEmbedder

Use SimpleEmbedder for fast, reproducible tests:

```go
func TestSemanticSearch(t *testing.T) {
    // Use simple embedder - no API calls, fast tests
    embedder := embedding.NewSimpleEmbedder(256)
    mem := memory.NewVectorMemory(embedder)
    
    // Test logic without OpenAI dependency
    mem.Add(ctx, "test", message.NewHumanMessage("test message"))
    results, err := mem.Recall(ctx, "test", &memory.RecallFilter{
        Query: "test",
        K:     1,
    })
    
    require.NoError(t, err)
    require.Len(t, results, 1)
}

func BenchmarkEmbedding(b *testing.B) {
    embedder := embedding.NewSimpleEmbedder(384)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = embedder.Embed(context.Background(), "benchmark text")
    }
}
```

---

## Performance Considerations

### Storage Requirements

| Dimensions | Bytes per Vector | 1M Vectors | 10M Vectors |
|-----------|------------------|------------|-------------|
| 256 | 1 KB | 1 GB | 10 GB |
| 512 | 2 KB | 2 GB | 20 GB |
| 1536 | 6 KB | 6 GB | 60 GB |
| 3072 | 12 KB | 12 GB | 120 GB |

### Similarity Search Speed

- **Linear scan**: O(n × d) - Acceptable for < 10K vectors
- **ANN (Approximate Nearest Neighbors)**: O(log n) - Required for > 100K vectors
- **Consider**: FAISS, Annoy, or specialized vector databases for large-scale

```go
// For large-scale, consider external vector stores
// - Pinecone
// - Weaviate  
// - Qdrant
// - Milvus
// Then wrap with memory.Memory interface
```

### API Costs (OpenAI)

| Model | Cost per 1M tokens | 1K documents (avg 500 tokens each) |
|-------|-------------------|-------------------------------------|
| text-embedding-3-small | $0.02 | $0.01 |
| text-embedding-3-large | $0.13 | $0.065 |
| text-embedding-ada-002 | $0.10 | $0.05 |

**Cost Optimization**:
1. Cache embeddings aggressively
2. Use smaller models for non-critical use cases
3. Reduce dimensions with text-embedding-3 models
4. Batch requests
5. Consider self-hosted alternatives for high volume

---

## Related Resources

- [Memory Package Documentation](/memory/)
- [RAG Agent Guide](/agents/#rag-agent)
- [OpenAI Embeddings API](https://platform.openai.com/docs/guides/embeddings)
- [Vector Search Algorithms](https://www.pinecone.io/learn/vector-search/)

---

## Next Steps

1. **Start Simple**: Use `SimpleEmbedder` for testing
2. **Upgrade to OpenAI**: Add semantic capabilities for production
3. **Optimize**: Monitor performance and costs, adjust dimensions
4. **Scale**: Consider dedicated vector databases for large-scale deployments

For implementation examples, see:
- [Basic Agent Example](https://github.com/hupe1980/agentmesh/tree/main/examples/basic_agent)
- [OpenAI Embedder README](https://github.com/hupe1980/agentmesh/blob/main/pkg/embedding/openai/README.md)
- [Memory Package Tests](https://github.com/hupe1980/agentmesh/blob/main/pkg/memory/memory_test.go)
