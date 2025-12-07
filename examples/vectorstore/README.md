# VectorStore Example

This example demonstrates how to use AgentMesh's vectorstore package for semantic document storage and retrieval.

## What This Example Shows

1. **Creating a VectorStore**: Using the in-memory backend for document storage
2. **Adding Documents**: Storing text documents with automatic embedding generation
3. **Semantic Search**: Finding similar documents by text query
4. **Metadata Filtering**: Filtering search results by document metadata
5. **VectorStoreRetriever**: Creating a retriever for RAG workflows

## Running the Example

```bash
# Set your OpenAI API key (or use SimpleEmbedder for testing)
export OPENAI_API_KEY="sk-..."

# Run the example
go run main.go
```

## Key Concepts

### VectorStore vs Memory

- **VectorStore**: General-purpose document storage with vector similarity search
- **Memory**: Conversation history storage with session-based organization

Use VectorStore when you need to:
- Store and search arbitrary documents
- Build RAG (Retrieval-Augmented Generation) pipelines
- Create knowledge bases for agents

### EmbeddingStore Helper

The `EmbeddingStore` wraps a `VectorStore` with automatic embedding generation:

```go
embedder := openai.NewEmbedder()
store := memory.New()
es := vectorstore.NewEmbeddingStore(store, embedder)

// Automatically embeds text before storing
es.AddTexts(ctx, []string{"doc1", "doc2"}, nil)

// Automatically embeds query before searching
results, _ := es.SearchText(ctx, "query", vectorstore.SearchOptions{K: 5})
```

### Integration with Retrieval

Use `VectorStoreRetriever` to integrate with RAG agents:

```go
retriever := retrieval.NewVectorStoreRetriever(store, embedder,
    retrieval.WithK(5),
    retrieval.WithMinScore(0.7),
)

ragAgent, _ := agent.NewRAG(model, retriever)
```

### Available Backends

- **memory**: In-memory storage (default, for testing/development)
- **qdrant**: Qdrant vector database (production-ready with gRPC)
- **pgvector**: PostgreSQL with pgvector extension (production-ready)

```go
import "github.com/hupe1980/agentmesh/pkg/vectorstore/qdrant"

// Create Qdrant store
store, err := qdrant.New("localhost:6334",
    qdrant.WithCollectionName("documents"),
    qdrant.WithDimensions(1536),
    qdrant.WithAutoCreateCollection(true),
)
```

```go
import "github.com/hupe1980/agentmesh/pkg/vectorstore/pgvector"

// Create pgvector store
store, err := pgvector.New(ctx,
    pgvector.WithConnectionString("postgres://user:pass@localhost:5432/mydb"),
    pgvector.WithTableName("documents"),
    pgvector.WithDimensions(1536),
    pgvector.WithMetric(pgvector.MetricCosine),
)
```

## Output

```
=== AgentMesh VectorStore Example ===

Adding documents to vector store...
Added 5 documents

Searching for: 'graph execution model'
Results:
  1. Score: 0.892 - AgentMesh uses Pregel BSP for graph execution
  2. Score: 0.756 - Checkpointing enables time-travel debugging
  3. Score: 0.701 - Tools allow agents to call external APIs

Filtering by category='core':
  1. Score: 0.892 - AgentMesh uses Pregel BSP for graph execution
  2. Score: 0.756 - Checkpointing enables time-travel debugging

Using VectorStoreRetriever...
Retrieved 3 documents for RAG context
```
