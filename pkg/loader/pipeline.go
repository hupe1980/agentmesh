package loader

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/vectorstore"
)

// Pipeline orchestrates the document ingestion flow: load → split → store.
type Pipeline struct {
	loader   Loader
	splitter Splitter
	store    vectorstore.VectorStore
	opts     PipelineOptions
}

// PipelineOptions configures the pipeline behavior.
type PipelineOptions struct {
	// BatchSize is the number of documents to process at once.
	BatchSize int

	// Namespace for storing documents in the vector store.
	Namespace string

	// OnProgress is called after each batch is processed.
	OnProgress func(processed, total int)
}

// PipelineOption configures a Pipeline.
type PipelineOption func(*PipelineOptions)

// WithPipelineBatchSize sets the batch size for processing.
func WithPipelineBatchSize(size int) PipelineOption {
	return func(o *PipelineOptions) {
		o.BatchSize = size
	}
}

// WithPipelineNamespace sets the namespace for storage.
func WithPipelineNamespace(ns string) PipelineOption {
	return func(o *PipelineOptions) {
		o.Namespace = ns
	}
}

// WithPipelineProgress sets a progress callback.
func WithPipelineProgress(fn func(processed, total int)) PipelineOption {
	return func(o *PipelineOptions) {
		o.OnProgress = fn
	}
}

// NewPipeline creates an ingestion pipeline.
func NewPipeline(loader Loader, splitter Splitter, store vectorstore.VectorStore, opts ...PipelineOption) *Pipeline {
	options := PipelineOptions{
		BatchSize: 100,
	}
	for _, opt := range opts {
		opt(&options)
	}

	return &Pipeline{
		loader:   loader,
		splitter: splitter,
		store:    store,
		opts:     options,
	}
}

// Run executes the pipeline: load documents, split them, and store in the vector store.
func (p *Pipeline) Run(ctx context.Context) error {
	// Load documents
	docs, err := p.loader.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load documents: %w", err)
	}

	// Split documents if splitter is provided
	var chunks []Document
	if p.splitter != nil {
		chunks, err = p.splitter.SplitDocuments(docs)
		if err != nil {
			return fmt.Errorf("failed to split documents: %w", err)
		}
	} else {
		chunks = docs
	}

	if len(chunks) == 0 {
		return nil
	}

	// Convert to vectorstore documents and add to store
	total := len(chunks)
	for i := 0; i < total; i += p.opts.BatchSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		end := i + p.opts.BatchSize
		if end > total {
			end = total
		}

		batch := chunks[i:end]
		vsDocs := make([]vectorstore.Document, len(batch))
		for j, doc := range batch {
			vsDocs[j] = vectorstore.Document{
				Content:  doc.Content,
				Metadata: doc.Metadata,
			}
			if doc.Source != "" {
				if vsDocs[j].Metadata == nil {
					vsDocs[j].Metadata = make(map[string]any)
				}
				vsDocs[j].Metadata["source"] = doc.Source
			}
		}

		addOpts := []func(*vectorstore.AddOptions){}
		if p.opts.Namespace != "" {
			addOpts = append(addOpts, func(o *vectorstore.AddOptions) { o.Namespace = p.opts.Namespace })
		}

		if err := p.store.Add(ctx, vsDocs, addOpts...); err != nil {
			return fmt.Errorf("failed to add documents to store: %w", err)
		}

		if p.opts.OnProgress != nil {
			p.opts.OnProgress(end, total)
		}
	}

	return nil
}

// RunWithTransform executes the pipeline with a custom transform function.
func (p *Pipeline) RunWithTransform(ctx context.Context, transform func(Document) (Document, error)) error {
	// Load documents
	docs, err := p.loader.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load documents: %w", err)
	}

	// Apply transformation
	transformed := make([]Document, 0, len(docs))
	for _, doc := range docs {
		t, err := transform(doc)
		if err != nil {
			return fmt.Errorf("transform failed: %w", err)
		}
		transformed = append(transformed, t)
	}

	// Split documents if splitter is provided
	var chunks []Document
	if p.splitter != nil {
		chunks, err = p.splitter.SplitDocuments(transformed)
		if err != nil {
			return fmt.Errorf("failed to split documents: %w", err)
		}
	} else {
		chunks = transformed
	}

	if len(chunks) == 0 {
		return nil
	}

	// Convert to vectorstore documents and add to store
	total := len(chunks)
	for i := 0; i < total; i += p.opts.BatchSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		end := min(i+p.opts.BatchSize, total)

		batch := chunks[i:end]
		vsDocs := make([]vectorstore.Document, len(batch))
		for j, doc := range batch {
			vsDocs[j] = vectorstore.Document{
				Content:  doc.Content,
				Metadata: doc.Metadata,
			}
			if doc.Source != "" {
				if vsDocs[j].Metadata == nil {
					vsDocs[j].Metadata = make(map[string]any)
				}
				vsDocs[j].Metadata["source"] = doc.Source
			}
		}

		addOpts := []func(*vectorstore.AddOptions){}
		if p.opts.Namespace != "" {
			addOpts = append(addOpts, func(o *vectorstore.AddOptions) { o.Namespace = p.opts.Namespace })
		}

		if err := p.store.Add(ctx, vsDocs, addOpts...); err != nil {
			return fmt.Errorf("failed to add documents to store: %w", err)
		}

		if p.opts.OnProgress != nil {
			p.opts.OnProgress(end, total)
		}
	}

	return nil
}
