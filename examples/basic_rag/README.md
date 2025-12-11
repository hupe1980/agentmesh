# Basic RAG Example

This example demonstrates Retrieval-Augmented Generation (RAG) using AgentMesh. RAG combines document retrieval with LLM generation to provide grounded, accurate answers based on a knowledge base.

## What It Shows

- Setting up an in-memory vector store with OpenAI embeddings
- Adding documents to create a knowledge base
- Creating a RAG agent that retrieves relevant context before generating
- Streaming responses for real-time output
- Using the `agent.NewRAG` builder for clean configuration

## How It Works

1. **Embedding Store Setup**: Documents are embedded using OpenAI's embedding model and stored in an in-memory vector store.

2. **Retrieval**: When a question is asked, the retriever finds the most semantically similar documents.

3. **Generation**: The RAG agent combines the retrieved documents with the question and generates a grounded response.

## Running the Example

```bash
export OPENAI_API_KEY="sk-..."
go run main.go
```

## Expected Output

```
=== AgentMesh Basic RAG Example ===

Setting up vector store with OpenAI embeddings...
Added 8 documents to knowledge base

Question: What execution model does AgentMesh use and why?

Answer: AgentMesh uses Pregel's Bulk Synchronous Parallel (BSP) model for 
deterministic execution. This model ensures consistent, reproducible behavior
across agent runs...

✅ RAG agent completed successfully!
```

## Key Components

### Retriever Configuration

```go
retriever := retrieval.NewVectorStoreRetriever(store, embedder,
    retrieval.WithK(3),        // Retrieve top 3 most relevant documents
    retrieval.WithMinScore(0), // Include all matches
)
```

### RAG Agent Creation

```go
ragAgent, err := agent.NewRAG(
    openai.NewModel(),
    retriever,
    agent.WithInstructions("You are a helpful assistant..."),
    agent.WithStreaming(true), // Enable streaming for real-time output
)
```

### Streaming Execution

```go
for msg, err := range ragAgent.Run(ctx, messages) {
    // Handle streaming chunks vs final messages
    switch m := msg.(type) {
    case *message.AIMessageChunk:
        // Stream partial output as it arrives
        fmt.Print(m.String())
    case *message.AIMessage:
        // Final complete message (already in state)
        // Skip printing to avoid duplication
    }
}
```

**Note:** When streaming is enabled:
- `*message.AIMessageChunk` - Partial output streamed in real-time
- `*message.AIMessage` - Final complete message (already in state, don't print to avoid duplication)

## Learn More

- [RAG Documentation](../../docs/agents.md)
- [Vector Store Example](../vectorstore/)
- [Streaming Example](../streaming/)
