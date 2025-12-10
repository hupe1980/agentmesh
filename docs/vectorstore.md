---
layout: doc
title: Vector Store
permalink: /vectorstore/
hero:
  title: Vector Store
  description: Store and search documents with semantic similarity and hybrid search capabilities.
  primary_cta:
    label: API reference
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/vectorstore"
    external: true
  secondary_cta:
    label: Embeddings guide →
    href: "/embeddings/"
sidebar:
  - title: Overview
    url: "#overview"
  - title: Backends
    url: "#backends"
  - title: Basic Usage
    url: "#basic-usage"
  - title: Hybrid Search
    url: "#hybrid-search"
  - title: Retrieval Pipeline
    url: "#retrieval-pipeline"
  - title: Best Practices
    url: "#best-practices"
---

# Vector Store

The vectorstore package provides a unified interface for storing and searching documents using vector similarity, with support for hybrid search combining keyword and semantic matching.

---

## Overview {#overview}

AgentMesh's vectorstore abstraction supports multiple backends with a consistent API:

**Key Features**:
- ✅ **Unified interface** - Same code works across all backends
- ✅ **Semantic search** - Find documents by meaning using embeddings
- ✅ **Hybrid search** - Combine keyword and vector search for better results
- ✅ **Metadata filtering** - Filter by document attributes
- ✅ **Namespace support** - Multi-tenant document partitioning
- ✅ **Index management** - Create, list, and delete collections

**Core Interfaces**:

```go
// VectorStore is the base interface for document storage
type VectorStore interface {
    Add(ctx context.Context, docs []Document, opts ...func(*AddOptions)) error
    Search(ctx context.Context, embedding Vector, opts SearchOptions) ([]Document, error)
    Delete(ctx context.Context, ids []string, namespace string) error
}

// TextSearcher extends VectorStore with hybrid search
type TextSearcher interface {
    VectorStore
    SearchHybrid(ctx context.Context, query string, embedding Vector, opts HybridSearchOptions) ([]Document, error)
}

// Indexer adds index lifecycle management
type Indexer interface {
    VectorStore
    CreateIndex(ctx context.Context, name string, dims int, metric Metric) error
    DeleteIndex(ctx context.Context, name string) error
    ListIndexes(ctx context.Context) ([]string, error)
}
```

---

## Backends {#backends}

### Supported Backends

| Backend | Package | Hybrid Search | Notes |
|---------|---------|:-------------:|-------|
| **Memory** | `vectorstore/memory` | ❌ | In-memory, great for testing |
| **Weaviate** | `vectorstore/weaviate` | ✅ | Native hybrid with BM25 |
| **Qdrant** | `vectorstore/qdrant` | ✅ | Query API with RRF fusion |
| **Pinecone** | `vectorstore/pinecone` | ✅ | Requires sparse encoder |
| **pgvector** | `vectorstore/pgvector` | ❌ | PostgreSQL extension |
| **S3 Vectors** | `vectorstore/s3vectors` | ❌ | AWS S3-based storage |

### Memory (In-Memory)

Best for testing and development:

```go
import "github.com/hupe1980/agentmesh/pkg/vectorstore/memory"

store := memory.New()
```

### Weaviate

Cloud-native vector database with native hybrid search:

```go
import "github.com/hupe1980/agentmesh/pkg/vectorstore/weaviate"

// From configuration (recommended)
store, err := weaviate.New(
    "http://localhost:8080",
    weaviate.WithClassName("Documents"),
    weaviate.WithAutoCreateClass(true, 1536, embedding.Cosine),
)

// From existing client
store, err := weaviate.NewFromClient(client,
    weaviate.WithClassName("Documents"),
)
```

### Qdrant

High-performance vector database with advanced filtering:

```go
import "github.com/hupe1980/agentmesh/pkg/vectorstore/qdrant"

// From address (recommended)
store, err := qdrant.NewFromAddr("localhost:6334",
    qdrant.WithCollectionName("documents"),
    qdrant.WithDimensions(1536),
    qdrant.WithAutoCreateCollection(true),
)

// From gRPC clients (for testing/custom setup)
store, err := qdrant.New(conn, pointsClient, collectionsClient,
    qdrant.WithCollectionName("documents"),
)
```

### Pinecone

Managed vector database with sparse-dense hybrid support:

```go
import "github.com/hupe1980/agentmesh/pkg/vectorstore/pinecone"

store := pinecone.New(client, indexConnection, "my-index",
    pinecone.WithMetric(embedding.Cosine),
    pinecone.WithCloud("aws"),
    pinecone.WithRegion("us-east-1"),
)
```

### pgvector

PostgreSQL-based vector storage:

```go
import "github.com/hupe1980/agentmesh/pkg/vectorstore/pgvector"

// From connection string (recommended)
store, err := pgvector.New(ctx, "postgres://user:pass@localhost/db",
    pgvector.WithTableName("documents"),
    pgvector.WithDimensions(1536),
)

// From connection pool
store, err := pgvector.NewFromPool(pool,
    pgvector.WithTableName("documents"),
)
```

---

## Basic Usage {#basic-usage}

### Adding Documents

```go
import (
    "github.com/hupe1980/agentmesh/pkg/vectorstore"
    "github.com/hupe1980/agentmesh/pkg/vectorstore/memory"
)

store := memory.New()

// Add documents with pre-computed embeddings
docs := []vectorstore.Document{
    {
        ID:        "doc1",
        Content:   "AgentMesh uses Pregel for graph execution",
        Embedding: []float32{0.1, 0.2, 0.3, ...},
        Metadata:  map[string]any{"category": "architecture"},
    },
    {
        ID:        "doc2", 
        Content:   "Checkpointing enables time-travel debugging",
        Embedding: []float32{0.4, 0.5, 0.6, ...},
        Metadata:  map[string]any{"category": "features"},
    },
}

err := store.Add(ctx, docs)
```

### Using EmbeddingStore

Auto-generate embeddings from text:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/embedding/openai"
    "github.com/hupe1980/agentmesh/pkg/vectorstore"
)

embedder := openai.NewEmbedder()
es := vectorstore.NewEmbeddingStore(store, embedder)

// Add text documents - embeddings generated automatically
err := es.AddTexts(ctx, []string{
    "AgentMesh uses Pregel for graph execution",
    "Checkpointing enables time-travel debugging",
}, nil)

// Search by text query
results, err := es.SearchText(ctx, "graph execution model", vectorstore.SearchOptions{
    K:        5,
    MinScore: 0.7,
})
```

### Searching Documents

```go
// Vector search
results, err := store.Search(ctx, queryEmbedding, vectorstore.SearchOptions{
    K:         10,           // Return top 10 results
    MinScore:  0.5,          // Minimum similarity threshold
    Namespace: "production", // Optional namespace
    Filter: vectorstore.Filter{
        "category": "architecture",
    },
})

for _, doc := range results {
    fmt.Printf("Score: %.3f | %s\n", doc.Score, doc.Content)
}
```

### Metadata Filtering

```go
// Exact match
results, _ := store.Search(ctx, embedding, vectorstore.SearchOptions{
    Filter: vectorstore.Filter{
        "category": "programming",
        "language": "go",
    },
})

// Filter helpers
filter := vectorstore.Eq("status", "published")
filter = vectorstore.And(filter, vectorstore.Eq("author", "john"))
```

---

## Hybrid Search {#hybrid-search}

Hybrid search combines **dense vector similarity** (semantic) with **sparse keyword matching** (BM25/TF-IDF) for improved retrieval quality.

### When to Use Hybrid Search

| Scenario | Recommended |
|----------|-------------|
| Exact term matching needed (product codes, names) | ✅ Hybrid |
| Pure semantic similarity | Vector only |
| Technical documentation with specific terms | ✅ Hybrid |
| General knowledge queries | Either |
| Limited training data overlap | ✅ Hybrid |

### Alpha Parameter

The `Alpha` parameter controls the balance between keyword and vector search:

- `Alpha = 0.0` → Pure keyword/sparse search (BM25)
- `Alpha = 0.5` → Equal weighting (default)
- `Alpha = 1.0` → Pure vector/dense search

```go
opts := vectorstore.HybridSearchOptions{
    SearchOptions: vectorstore.SearchOptions{K: 10},
    Alpha:         0.7, // 70% vector, 30% keyword
}
```

### Weaviate Hybrid Search

Weaviate has native hybrid search with BM25:

```go
import "github.com/hupe1980/agentmesh/pkg/vectorstore/weaviate"

store, _ := weaviate.New("http://localhost:8080",
    weaviate.WithClassName("Documents"),
)

// Cast to TextSearcher
textSearcher := store.(vectorstore.TextSearcher)

results, err := textSearcher.SearchHybrid(ctx, 
    "graph execution",           // Text query for BM25
    queryEmbedding,              // Dense vector
    vectorstore.HybridSearchOptions{
        SearchOptions: vectorstore.SearchOptions{K: 10},
        Alpha:         0.5,      // 50/50 blend
    },
)
```

### Qdrant Hybrid Search

Qdrant uses Query API with Reciprocal Rank Fusion:

```go
import "github.com/hupe1980/agentmesh/pkg/vectorstore/qdrant"

store, _ := qdrant.NewFromAddr("localhost:6334",
    qdrant.WithCollectionName("documents"),
)

textSearcher := store.(vectorstore.TextSearcher)

results, err := textSearcher.SearchHybrid(ctx,
    "pregel algorithm",
    queryEmbedding,
    vectorstore.HybridSearchOptions{
        SearchOptions:   vectorstore.SearchOptions{K: 10},
        Alpha:           0.5,
        FusionAlgorithm: vectorstore.FusionRRF, // or FusionRelativeScore
    },
)
```

**Fusion Algorithms**:
- `FusionRRF` - Reciprocal Rank Fusion (default, recommended)
- `FusionRelativeScore` - Distribution-Based Score Fusion

### Pinecone Hybrid Search

Pinecone requires a sparse encoder for hybrid search:

```go
import "github.com/hupe1980/agentmesh/pkg/vectorstore/pinecone"

// Implement SparseEncoder interface (BM25, SPLADE, etc.)
type BM25Encoder struct {
    // Your BM25 implementation
}

func (e *BM25Encoder) Encode(text string) ([]uint32, []float32, error) {
    // Return sparse vector indices and values
    return indices, values, nil
}

// Create store with sparse encoder
store := pinecone.New(client, idx, "my-index",
    pinecone.WithSparseEncoder(&BM25Encoder{}),
)

textSearcher := store.(vectorstore.TextSearcher)

results, err := textSearcher.SearchHybrid(ctx,
    "search query",
    queryEmbedding,
    vectorstore.HybridSearchOptions{
        SearchOptions: vectorstore.SearchOptions{K: 10},
        Alpha:         0.5,
    },
)
```

---

## Retrieval Pipeline {#retrieval-pipeline}

### VectorStoreRetriever

Adapt a VectorStore for RAG workflows:

```go
import "github.com/hupe1980/agentmesh/pkg/retrieval"

retriever := retrieval.NewVectorStoreRetriever(store, embedder,
    retrieval.WithK(5),
    retrieval.WithMinScore(0.7),
    retrieval.WithNamespace("production"),
    retrieval.WithFilter(vectorstore.Filter{"status": "published"}),
)

// Use with RAG agent
ragAgent, _ := agent.NewRAG(model, retriever)

for msg, err := range ragAgent.Run(ctx, messages) {
    // Agent automatically retrieves relevant context
}
```

### HybridVectorStoreRetriever

For hybrid search in RAG pipelines, use the built-in `HybridVectorStoreRetriever`:

```go
import "github.com/hupe1980/agentmesh/pkg/retrieval"

// Cast store to TextSearcher
textSearcher := store.(vectorstore.TextSearcher)

// Create hybrid retriever
hybridRetriever := retrieval.NewHybridVectorStoreRetriever(textSearcher, embedder,
    retrieval.WithHybridK(10),
    retrieval.WithHybridMinScore(0.5),
    retrieval.WithAlpha(0.7),                            // 70% vector, 30% keyword
    retrieval.WithFusionAlgorithm(vectorstore.FusionRRF),
)

// Use with RAG agent
ragAgent, _ := agent.NewRAG(model, hybridRetriever)

for msg, err := range ragAgent.Run(ctx, messages) {
    // Agent uses hybrid search for context retrieval
}
```

**Available Options**:
- `WithHybridK(k)` - Maximum documents to retrieve
- `WithHybridMinScore(score)` - Minimum similarity threshold
- `WithHybridNamespace(ns)` - Multi-tenant namespace
- `WithHybridFilter(filter)` - Metadata filtering
- `WithAlpha(alpha)` - Balance between keyword (0) and vector (1)
- `WithFusionAlgorithm(algo)` - Result fusion strategy
```

### Reranking

Improve results with a reranker:

```go
import "github.com/hupe1980/agentmesh/pkg/retrieval"

// Create base retriever
baseRetriever := retrieval.NewVectorStoreRetriever(store, embedder,
    retrieval.WithK(20), // Over-fetch for reranking
)

// Wrap with reranker
reranker := retrieval.NewReranker(baseRetriever, rerankerModel,
    retrieval.WithTopN(5),
)

docs, err := reranker.Retrieve(ctx, "your query")
```

---

## Best Practices {#best-practices}

### 1. Choose the Right Backend

| Use Case | Recommended Backend |
|----------|---------------------|
| Development/Testing | Memory |
| Production with hybrid search | Weaviate, Qdrant |
| Managed service | Pinecone |
| Existing PostgreSQL | pgvector |
| AWS ecosystem | S3 Vectors |

### 2. Optimize Chunk Size

```go
// Optimal chunk sizes vary by use case
// - Code: 500-1000 tokens
// - Documentation: 200-500 tokens  
// - Conversations: 100-300 tokens
```

### 3. Use Namespaces for Multi-Tenancy

```go
// Separate data by tenant
err := store.Add(ctx, docs, func(o *vectorstore.AddOptions) {
    o.Namespace = "tenant-123"
})

results, _ := store.Search(ctx, embedding, vectorstore.SearchOptions{
    Namespace: "tenant-123",
})
```

### 4. Set Appropriate MinScore

```go
// Higher threshold = more relevant but fewer results
results, _ := store.Search(ctx, embedding, vectorstore.SearchOptions{
    K:        10,
    MinScore: 0.75, // Only highly relevant documents
})
```

### 5. Combine Hybrid Search with Reranking

For best retrieval quality:

```go
// 1. Hybrid search with over-fetching
docs, _ := textSearcher.SearchHybrid(ctx, query, embedding,
    vectorstore.HybridSearchOptions{
        SearchOptions: vectorstore.SearchOptions{K: 20},
        Alpha:         0.5,
    },
)

// 2. Rerank to get final top results
reranked := reranker.Rerank(ctx, query, docs, 5)
```

---

## See Also

- [Embeddings](/embeddings/) - Text to vector conversion
- [Memory](/memory/) - Conversation history with semantic search
- [Agents](/agents/) - Building RAG agents
