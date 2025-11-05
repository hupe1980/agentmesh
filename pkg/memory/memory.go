package memory

import (
	"context"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// Memory defines the interface for long-term message storage and retrieval.
// Implementations can provide simple storage or advanced semantic search capabilities.
type Memory interface {
	// Store persists messages for a given session/user identifier.
	Store(ctx context.Context, sessionID string, messages []message.Message) error

	// Recall retrieves messages for a session based on the provided filter.
	// The filter can specify semantic queries, time ranges, metadata, etc.
	Recall(ctx context.Context, sessionID string, filter RecallFilter) ([]message.Message, error)

	// Clear removes all messages for a given session.
	Clear(ctx context.Context, sessionID string) error

	// Sessions returns all session IDs that have stored messages.
	Sessions(ctx context.Context) ([]string, error)
}

// RecallFilter specifies criteria for retrieving messages from memory.
type RecallFilter struct {
	// Query is the semantic search query (for vector-based memories).
	// If empty, returns most recent messages.
	Query string

	// K is the maximum number of messages to return.
	// Default is 10 if not specified.
	K int

	// MinScore is the minimum similarity score (0.0-1.0) for vector search.
	// Messages below this threshold are filtered out.
	MinScore float64

	// After filters messages created after this time.
	After *time.Time

	// Before filters messages created before this time.
	Before *time.Time

	// Types filters messages by type (e.g., only AI or Human messages).
	Types []message.Type

	// Metadata filters messages by custom metadata fields.
	Metadata map[string]string
}

// DefaultK is the default number of messages to recall if K is not specified.
const DefaultK = 10

// Normalize ensures filter has sensible defaults.
func (f *RecallFilter) Normalize() {
	if f.K <= 0 {
		f.K = DefaultK
	}
	if f.MinScore < 0 {
		f.MinScore = 0
	}
	if f.MinScore > 1 {
		f.MinScore = 1
	}
}

// Embedder defines the interface for converting text to vector embeddings.
type Embedder interface {
	// Embed converts text into a vector embedding.
	Embed(ctx context.Context, text string) ([]float64, error)

	// EmbedBatch converts multiple texts into vector embeddings efficiently.
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)

	// Dimensions returns the dimensionality of the embedding vectors.
	Dimensions() int
}

// MessageEntry represents a stored message with metadata and embeddings.
type MessageEntry struct {
	ID        string
	SessionID string
	Message   message.Message
	Embedding []float64
	Score     float64 // Similarity score (for search results)
	Timestamp time.Time
	Metadata  map[string]string
}
