package loader

import (
	"context"
	"maps"
)

// Document represents a loaded document before embedding.
// It contains the content text, metadata for filtering, and source information.
type Document struct {
	// Content is the text content of the document.
	Content string

	// Metadata contains arbitrary key-value pairs for filtering and context.
	Metadata map[string]any

	// Source identifies where the document came from (e.g., file path, URL).
	Source string
}

// NewDocument creates a new document with the given content.
func NewDocument(content string) Document {
	return Document{
		Content:  content,
		Metadata: make(map[string]any),
	}
}

// WithMetadata returns a copy of the document with additional metadata.
func (d Document) WithMetadata(key string, value any) Document {
	meta := make(map[string]any, len(d.Metadata)+1)
	maps.Copy(meta, d.Metadata)
	meta[key] = value

	return Document{
		Content:  d.Content,
		Metadata: meta,
		Source:   d.Source,
	}
}

// WithSource returns a copy of the document with the source set.
func (d Document) WithSource(source string) Document {
	return Document{
		Content:  d.Content,
		Metadata: d.Metadata,
		Source:   source,
	}
}

// Loader loads documents from a source.
type Loader interface {
	// Load reads documents from the source.
	Load(ctx context.Context) ([]Document, error)
}

// Func is a function adapter for Loader.
type Func func(ctx context.Context) ([]Document, error)

// Load implements the Loader interface.
func (f Func) Load(ctx context.Context) ([]Document, error) {
	return f(ctx)
}
