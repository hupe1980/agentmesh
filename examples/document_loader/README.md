# Document Loader Example

This example demonstrates the document loading and ingestion pipeline in AgentMesh.

## Overview

The `pkg/loader` package provides utilities for:
- Loading documents from various sources (files, directories, URLs, readers)
- Splitting documents into chunks suitable for embedding
- Creating ingestion pipelines that orchestrate load → split → store workflows

## Quick Start

```go
package main

import (
    "context"
    "log"

    "github.com/hupe1980/agentmesh/pkg/embedding"
    "github.com/hupe1980/agentmesh/pkg/loader"
    "github.com/hupe1980/agentmesh/pkg/vectorstore"
    "github.com/hupe1980/agentmesh/pkg/vectorstore/memory"
)

func main() {
    ctx := context.Background()

    // Create an embedder (use your preferred embedding provider)
    embedder := // ... your embedder

    // Create a vector store
    store := vectorstore.NewEmbeddingStore(memory.New(), embedder)

    // Create a directory loader
    docLoader := loader.NewDirectoryLoader("./docs",
        loader.WithPattern("*.md"),
        loader.WithRecursive(true),
    )

    // Create a text splitter
    splitter := loader.NewRecursiveCharacterSplitter(1000, 100)

    // Create the ingestion pipeline
    pipeline := loader.NewPipeline(docLoader, splitter, store,
        loader.WithPipelineBatchSize(50),
        loader.WithPipelineProgress(func(processed, total int) {
            log.Printf("Progress: %d/%d documents", processed, total)
        }),
    )

    // Run the pipeline
    if err := pipeline.Run(ctx); err != nil {
        log.Fatal(err)
    }

    log.Println("Documents ingested successfully!")
}
```

## Loaders

### ReaderLoader

Load from any `io.Reader`:

```go
data := strings.NewReader("Some content to load")
l := loader.NewReaderLoader(data,
    loader.WithReaderSource("my-source"),
    loader.WithReaderMetadata(map[string]any{"key": "value"}),
)
```

### FileLoader

Load a single file:

```go
l := loader.NewFileLoader("/path/to/document.txt")
```

### DirectoryLoader

Load all files from a directory:

```go
l := loader.NewDirectoryLoader("/path/to/docs",
    loader.WithPattern("*.md"),       // Only markdown files
    loader.WithRecursive(true),       // Include subdirectories
)
```

### StringLoader

Load from a string literal:

```go
l := loader.NewStringLoader("Direct content", loader.WithStringSource("inline"))
```

### URLLoader

Load from HTTP URLs:

```go
l := loader.NewURLLoader("https://example.com/document.txt")
```

### Custom Loader

Implement the `Loader` interface or use `Func`:

```go
l := loader.Func(func(ctx context.Context) ([]loader.Document, error) {
    // Custom loading logic
    return []loader.Document{
        {Content: "Custom content", Source: "custom"},
    }, nil
})
```

## Splitters

### RecursiveCharacterSplitter

Recursively splits text by multiple separators:

```go
// ChunkSize: 1000, ChunkOverlap: 100
splitter := loader.NewRecursiveCharacterSplitter(1000, 100,
    loader.WithSeparators([]string{"\n\n", "\n", ". ", " "}),
)
```

### TokenSplitter

Splits by approximate token count:

```go
// TokensPerChunk: 500, Overlap: 50
splitter := loader.NewTokenSplitter(500, 50)
```

## Pipeline

The `Pipeline` orchestrates the full ingestion workflow:

```go
pipeline := loader.NewPipeline(
    docLoader,  // Loader
    splitter,   // Splitter (optional, nil for no splitting)
    store,      // VectorStore
    loader.WithPipelineBatchSize(100),
    loader.WithPipelineNamespace("my-namespace"),
    loader.WithPipelineProgress(func(processed, total int) {
        fmt.Printf("Progress: %d/%d\n", processed, total)
    }),
)

if err := pipeline.Run(ctx); err != nil {
    log.Fatal(err)
}
```

### Pipeline with Transform

Apply custom transformations before storing:

```go
err := pipeline.RunWithTransform(ctx, func(doc loader.Document) (loader.Document, error) {
    // Transform the document (e.g., clean text, extract metadata)
    doc.Content = strings.ToLower(doc.Content)
    return doc, nil
})
```

## Multi-Modal Assets

For multi-modal content (images, audio, etc.), use the `embedding.Asset` type:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/embedding"
    "github.com/hupe1980/agentmesh/pkg/message"
)

// Convert from message.FilePart to Asset
filePart := message.FilePart{
    Data:     imageData,
    MimeType: "image/png",
    Name:     "diagram.png",
}

asset := embedding.FromFilePart(filePart)

// Use with MultiModalEmbedder
if mmEmbedder, ok := embedder.(embedding.MultiModalEmbedder); ok {
    vector, err := mmEmbedder.EmbedAsset(ctx, asset)
    // ...
}
```

## Document Structure

```go
type Document struct {
    Content  string         // Text content
    Source   string         // Source identifier (file path, URL, etc.)
    Metadata map[string]any // Additional metadata
}

// Helper methods
doc := loader.NewDocument("content").
    WithSource("my-source").
    WithMetadata("key", "value")
```
