// Package loader provides document loading and text splitting utilities for
// ingesting content into vector stores.
//
// # Overview
//
// The loader package handles the first stages of a typical RAG pipeline:
//   - Loading documents from various sources (files, readers, URLs)
//   - Splitting documents into chunks suitable for embedding
//   - Transforming and preprocessing content
//
// # Basic Usage
//
// Load and split a text file:
//
//	// Load from a file
//	loader := loader.NewFileLoader("document.txt")
//	docs, err := loader.Load(ctx)
//
//	// Split into chunks
//	splitter := loader.NewRecursiveCharacterSplitter(1000, 200)
//	chunks, err := splitter.SplitDocuments(docs)
//
// # Loaders
//
// The package provides several loader implementations:
//
//   - FileLoader: Loads from local files
//   - ReaderLoader: Loads from io.Reader
//   - DirectoryLoader: Loads all files from a directory
//   - StringLoader: Creates documents from strings
//
// # Splitters
//
// Text splitters divide documents into smaller chunks:
//
//   - RecursiveCharacterSplitter: Smart splitting by separators with overlap
//   - TokenSplitter: Splits by token count (for models with token limits)
//
// # Pipeline Integration
//
// Use Pipeline to orchestrate the full ingestion flow:
//
//	pipeline := loader.NewPipeline(
//	    loader.NewDirectoryLoader("./docs", loader.WithPattern("*.md")),
//	    loader.NewRecursiveCharacterSplitter(1000, 200),
//	    vectorStore,
//	)
//	err := pipeline.Run(ctx)
package loader
