package model

import (
	"context"

	streamutil "github.com/hupe1980/agentmesh/internal/stream"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// StreamChunk represents a unit of streamed output from a model invocation.
// Text contains the incremental delta. When Final is true the chunk carries the
// fully assembled message in Message (if available) and signals stream completion.
// Err is non-nil when the producer encountered an error; Final will still be true
// so consumers can distinguish terminal errors from transient chunks.
type StreamChunk struct {
	Text    string
	Message message.Message
	Err     error
	Final   bool
}

// Stream exposes a channel of StreamChunk values that closes automatically once
// the model finishes producing output. Cancel aborts the underlying request if
// the consumer stops early.
type Stream struct {
	inner *streamutil.Stream[StreamChunk]
}

// NewStream constructs a Stream from the provided chunk channel and optional
// cancel function. Implementations should close the channel once a final chunk
// has been sent.
func NewStream(chunks <-chan StreamChunk, cancel context.CancelFunc) *Stream {
	cfg := streamutil.Config[StreamChunk]{
		ExtractErr: func(chunk StreamChunk) error { return chunk.Err },
		IsFinal:    func(chunk StreamChunk) bool { return chunk.Final },
	}
	return &Stream{inner: streamutil.New(chunks, cancel, cfg)}
}

// Cancel aborts the streaming operation. It is safe to call multiple times.
func (s *Stream) Cancel() {
	if s == nil || s.inner == nil {
		return
	}
	s.inner.Cancel()
}

// Next advances the stream and reports whether a chunk is available.
func (s *Stream) Next() bool {
	if s == nil || s.inner == nil {
		return false
	}
	return s.inner.Next()
}

// Current returns the most recently observed chunk.
func (s *Stream) Current() StreamChunk {
	if s == nil || s.inner == nil {
		return StreamChunk{}
	}
	return s.inner.Current()
}

// Err reports the terminal error, if any, encountered while streaming.
func (s *Stream) Err() error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Err()
}

// Model defines the contract for language model backends.
type Model interface {
	// Generate performs a blocking request returning the full response message.
	Generate(ctx context.Context, messages []message.Message) (message.Message, error)

	// Stream performs a streaming request, delivering incremental chunks until
	// completion or error. Implementations should emit a final chunk with Final
	// set to true, populate Err when failures occur, and then close the channel.
	Stream(ctx context.Context, messages []message.Message) (*Stream, error)
}

// ToolAware defines models that support tool/function calling.
type ToolAware interface {
	// BindTools returns a copy of this model with the given tools configured.
	// The model will be able to call these tools during generation.
	BindTools(tools ...tool.Tool) Model
}

// StructuredOutput defines models that support structured output generation.
type StructuredOutput interface {
	// WithStructuredOutput returns a copy of this model configured to generate
	// output conforming to the provided JSON schema. The schema parameter should
	// be a map[string]any representing a JSON Schema definition.
	//
	// When using structured output:
	// - The model will be constrained to generate valid JSON matching the schema
	// - The response will typically be in the content of the returned message
	// - Invalid outputs may result in errors depending on the implementation
	//
	// Example schema:
	//  schema := map[string]any{
	//      "type": "object",
	//      "properties": map[string]any{
	//          "name": map[string]any{"type": "string"},
	//          "age":  map[string]any{"type": "integer"},
	//      },
	//      "required": []string{"name", "age"},
	//  }
	WithStructuredOutput(schema map[string]any) Model
}
