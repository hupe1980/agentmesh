package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	memorystore "github.com/hupe1980/agentmesh/pkg/vectorstore/memory"
)

// Metadata keys for VectorStore documents.
const (
	metaKeyMessageType = "_msg_type"
	metaKeyMessageData = "_msg_data"
	metaKeyTimestamp   = "_timestamp"
)

// VectorMemoryOptions configures VectorMemory behavior.
type VectorMemoryOptions struct {
	// Store is an optional VectorStore backend.
	// If nil, an in-memory store will be created automatically.
	Store vectorstore.VectorStore
}

// VectorMemory implements semantic search over stored messages using vector embeddings.
// It uses a VectorStore backend for persistent/scalable storage.
type VectorMemory struct {
	embedder    embedding.Embedder
	store       vectorstore.VectorStore
	ownsStore   bool // true if we created the store (need to close it)
	sessionsMu  sync.RWMutex
	sessionsSet map[string]struct{} // track known sessions
}

// NewVectorMemory creates a new vector-based memory store.
// The embedder is required for generating embeddings.
// Optional VectorMemoryOptions can provide a custom VectorStore backend.
func NewVectorMemory(embedder embedding.Embedder, opts ...func(*VectorMemoryOptions)) *VectorMemory {
	options := &VectorMemoryOptions{}
	for _, opt := range opts {
		opt(options)
	}

	vm := &VectorMemory{
		embedder:    embedder,
		sessionsSet: make(map[string]struct{}),
	}

	if options.Store != nil {
		vm.store = options.Store
		vm.ownsStore = false
	} else {
		// Create default in-memory store
		vm.store = memorystore.New()
		vm.ownsStore = true
	}

	return vm
}

// WithStore sets a custom VectorStore backend.
func WithStore(store vectorstore.VectorStore) func(*VectorMemoryOptions) {
	return func(o *VectorMemoryOptions) {
		o.Store = store
	}
}

// Store persists messages with their vector embeddings.
func (vm *VectorMemory) Store(ctx context.Context, sessionID string, messages []message.Message) error {
	if len(messages) == 0 {
		return nil
	}

	// Track session
	vm.sessionsMu.Lock()
	vm.sessionsSet[sessionID] = struct{}{}
	vm.sessionsMu.Unlock()

	// Extract text from messages for embedding
	texts := make([]string, len(messages))
	for i, msg := range messages {
		texts[i] = extractText(msg)
	}

	// Generate embeddings
	embeddings, err := vm.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// Create documents for vector store
	baseTime := time.Now()
	docs := make([]vectorstore.Document, len(messages))

	for i, msg := range messages {
		// Serialize message for storage
		msgData, err := serializeMessage(msg)
		if err != nil {
			return fmt.Errorf("failed to serialize message: %w", err)
		}

		ts := baseTime.Add(time.Duration(i) * time.Nanosecond)
		docs[i] = vectorstore.Document{
			ID:        uuid.New().String(),
			Content:   texts[i],
			Embedding: embeddings[i],
			Timestamp: ts,
			Metadata: vectorstore.Metadata{
				metaKeyMessageType: string(msg.Type()),
				metaKeyMessageData: msgData,
				metaKeyTimestamp:   ts.Format(time.RFC3339Nano),
			},
		}
	}

	// Store in vector store with session as namespace
	return vm.store.Add(ctx, docs, func(o *vectorstore.AddOptions) {
		o.Namespace = sessionID
	})
}

// Recall retrieves messages using semantic search or filters.
//
//nolint:gocyclo // Message filtering and retrieval requires multiple conditions
func (vm *VectorMemory) Recall(ctx context.Context, sessionID string, filter RecallFilter) ([]message.Message, error) {
	filter.Normalize()

	var searchOpts vectorstore.SearchOptions
	searchOpts.Namespace = sessionID
	searchOpts.K = filter.K * 3 // Fetch more to allow for post-filtering
	searchOpts.MinScore = filter.MinScore

	// Build metadata filter for message types
	if len(filter.Types) > 0 {
		typeStrs := make([]any, len(filter.Types))
		for i, t := range filter.Types {
			typeStrs[i] = string(t)
		}
		searchOpts.Filter = vectorstore.In(metaKeyMessageType, typeStrs...)
	}

	var docs []vectorstore.Document
	var err error

	if filter.Query != "" {
		// Semantic search with query embedding
		queryEmbedding, embErr := vm.embedder.Embed(ctx, filter.Query)
		if embErr != nil {
			return nil, fmt.Errorf("failed to embed query: %w", embErr)
		}
		docs, err = vm.store.Search(ctx, queryEmbedding, searchOpts)
	} else {
		// No query - fetch all documents and sort by timestamp
		// Use a zero vector to get all documents (most stores will return by recency)
		dims := vm.embedder.Dimensions()
		zeroVec := make(embedding.Vector, dims)
		searchOpts.MinScore = 0 // Don't filter by score when no query
		docs, err = vm.store.Search(ctx, zeroVec, searchOpts)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to search vector store: %w", err)
	}

	// Post-filter and convert documents to messages
	results := make([]message.Message, 0, len(docs))
	for _, doc := range docs {
		// Apply time filters
		if filter.After != nil && doc.Timestamp.Before(*filter.After) {
			continue
		}
		if filter.Before != nil && doc.Timestamp.After(*filter.Before) {
			continue
		}

		// Apply custom metadata filter
		if len(filter.Metadata) > 0 {
			docMeta := extractStringMetadata(doc.Metadata)
			if !matchesMetadata(docMeta, filter.Metadata) {
				continue
			}
		}

		// Deserialize message
		msg, err := deserializeMessage(doc.Metadata)
		if err != nil {
			continue // Skip corrupt entries
		}

		results = append(results, msg)
		if len(results) >= filter.K {
			break
		}
	}

	// If no query, sort by timestamp descending
	if filter.Query == "" && len(results) > 0 {
		// We need to re-sort since vector store might not preserve order
		entries := make([]*docWithTimestamp, len(results))
		for i, msg := range results {
			entries[i] = &docWithTimestamp{msg: msg, ts: docs[i].Timestamp}
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].ts.After(entries[j].ts)
		})
		for i, e := range entries {
			results[i] = e.msg
		}
	}

	return results, nil
}

type docWithTimestamp struct {
	msg message.Message
	ts  time.Time
}

// Clear removes all messages for a session.
func (vm *VectorMemory) Clear(ctx context.Context, sessionID string) error {
	// Search for all documents in namespace and delete them
	dims := vm.embedder.Dimensions()
	zeroVec := make(embedding.Vector, dims)

	docs, err := vm.store.Search(ctx, zeroVec, vectorstore.SearchOptions{
		Namespace: sessionID,
		K:         10000, // Get all documents
		MinScore:  0,
	})
	if err != nil {
		return fmt.Errorf("failed to search for documents to clear: %w", err)
	}

	if len(docs) > 0 {
		ids := make([]string, len(docs))
		for i, doc := range docs {
			ids[i] = doc.ID
		}
		if err := vm.store.Delete(ctx, ids, sessionID); err != nil {
			return fmt.Errorf("failed to delete documents: %w", err)
		}
	}

	// Remove from session set
	vm.sessionsMu.Lock()
	delete(vm.sessionsSet, sessionID)
	vm.sessionsMu.Unlock()

	return nil
}

// Sessions returns all session IDs.
func (vm *VectorMemory) Sessions(ctx context.Context) ([]string, error) {
	vm.sessionsMu.RLock()
	defer vm.sessionsMu.RUnlock()

	sessions := make([]string, 0, len(vm.sessionsSet))
	for sessionID := range vm.sessionsSet {
		sessions = append(sessions, sessionID)
	}
	return sessions, nil
}

// Close releases resources if the store was created internally.
func (vm *VectorMemory) Close() error {
	if vm.ownsStore && vm.store != nil {
		return vm.store.Close()
	}
	return nil
}

// Helper functions

func extractText(msg message.Message) string {
	parts := msg.Parts()
	if len(parts) == 0 {
		return ""
	}

	var text string
	for _, part := range parts {
		if textPart, ok := part.(message.TextPart); ok {
			if text != "" {
				text += " "
			}
			text += textPart.Text
		}
	}
	return text
}

func containsType(types []message.Type, t message.Type) bool {
	for _, typ := range types {
		if typ == t {
			return true
		}
	}
	return false
}

func matchesMetadata(entryMeta, filterMeta map[string]string) bool {
	for key, value := range filterMeta {
		if entryMeta[key] != value {
			return false
		}
	}
	return true
}

func extractStringMetadata(meta vectorstore.Metadata) map[string]string {
	result := make(map[string]string)
	for k, v := range meta {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

// serializeMessage converts a message to JSON for storage.
func serializeMessage(msg message.Message) (string, error) {
	data := map[string]any{
		"type":  msg.Type(),
		"parts": serializeParts(msg.Parts()),
	}
	bytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func serializeParts(parts []message.Part) []map[string]any {
	result := make([]map[string]any, len(parts))
	for i, part := range parts {
		switch p := part.(type) {
		case message.TextPart:
			result[i] = map[string]any{"type": "text", "text": p.Text}
		default:
			result[i] = map[string]any{"type": "unknown"}
		}
	}
	return result
}

// extractTextFromParts extracts concatenated text from serialized parts.
func extractTextFromParts(partsData any) string {
	parts, ok := partsData.([]any)
	if !ok {
		return ""
	}

	var text string
	for _, p := range parts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if pm["type"] != "text" {
			continue
		}
		t, ok := pm["text"].(string)
		if !ok {
			continue
		}
		if text != "" {
			text += " "
		}
		text += t
	}
	return text
}

// deserializeMessage reconstructs a message from metadata.
func deserializeMessage(meta vectorstore.Metadata) (message.Message, error) {
	msgData, ok := meta[metaKeyMessageData].(string)
	if !ok {
		return nil, ErrMissingMessageData
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(msgData), &data); err != nil {
		return nil, err
	}

	msgType, ok := data["type"].(string)
	if !ok {
		return nil, ErrMissingMessageType
	}

	// Extract text from parts
	text := extractTextFromParts(data["parts"])

	switch message.Type(msgType) {
	case message.TypeHuman:
		return message.NewHumanMessageFromText(text), nil
	case message.TypeAI:
		return message.NewAIMessageFromText(text), nil
	case message.TypeSystem:
		return message.NewSystemMessageFromText(text), nil
	case message.TypeTool:
		return message.NewToolMessage("", text), nil
	default:
		return message.NewHumanMessageFromText(text), nil
	}
}
