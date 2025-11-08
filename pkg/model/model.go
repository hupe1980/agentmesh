package model

import (
	"context"
	"iter"

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

// Model defines the contract for language model backends using Go 1.23+ iterators.
// The unified Generate method supports both streaming and blocking consumption patterns.
type Model interface {
	// Generate performs a request that yields message chunks as an iterator.
	// For streaming, iterate over all chunks to get incremental updates.
	// For blocking, consume only the final message using Last() or similar helpers.
	//
	// Streaming usage:
	//   for msg, err := range model.Generate(ctx, messages) {
	//       if err != nil { return err }
	//       fmt.Print(msg.Content) // Process each chunk
	//   }
	//
	// Blocking usage (helper required):
	//   msg, err := Last(model.Generate(ctx, messages))
	//   if err != nil { return err }
	//   fmt.Println(msg.Content) // Process final message only
	//
	// The iterator will yield:
	// - Multiple messages with incremental content (streaming mode)
	// - A single final message (blocking mode)
	// - The last yield will contain any error encountered
	//
	// Context cancellation is respected and will stop iteration.
	Generate(ctx context.Context, messages []message.Message) iter.Seq2[message.Message, error]
}

// Last consumes an iterator and returns only the final message and error.
// This is the standard way to use Generate() in blocking/non-streaming mode.
//
// Example:
//
//	msg, err := model.Last(model.Generate(ctx, messages))
//	if err != nil {
//	    return err
//	}
//	fmt.Println(msg.Content)
func Last(seq iter.Seq2[message.Message, error]) (message.Message, error) {
	var lastMsg message.Message
	var lastErr error

	for msg, err := range seq {
		lastMsg = msg
		if err != nil {
			lastErr = err
			break
		}
	}

	return lastMsg, lastErr
}

// Collect gathers all messages from an iterator into a slice.
// The final error (if any) is returned separately.
//
// Example:
//
//	messages, err := model.Collect(model.Generate(ctx, messages))
//	if err != nil {
//	    return err
//	}
//	for _, msg := range messages {
//	    fmt.Println(msg.Content)
//	}
func Collect(seq iter.Seq2[message.Message, error]) ([]message.Message, error) {
	messages := make([]message.Message, 0)
	var lastErr error

	for msg, err := range seq {
		if err != nil {
			lastErr = err
			break
		}
		messages = append(messages, msg)
	}

	return messages, lastErr
}

// ToStream wraps an iterator in the Stream type for compatibility with non-iterator APIs.
// This allows using iterator-based model responses with APIs that expect the Stream type.
//
// Note: Prefer using the iterator directly with for-range when possible.
func ToStream(ctx context.Context, seq iter.Seq2[message.Message, error]) *Stream {
	chunks := make(chan StreamChunk, 1)
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer close(chunks)
		defer cancel()

		var lastErr error
	loop:
		for msg, err := range seq {
			if err != nil {
				lastErr = err
				break
			}

			// Check context
			select {
			case <-ctx.Done():
				lastErr = ctx.Err()
				break loop
			default:
			}

			// Extract text from message parts
			var text string
			for _, part := range msg.Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					text += textPart.Text
				}
			}

			chunks <- StreamChunk{
				Text:    text,
				Message: msg,
				Err:     nil,
				Final:   false,
			}
		}

		// Send final chunk
		chunks <- StreamChunk{
			Err:   lastErr,
			Final: true,
		}
	}()

	return NewStream(chunks, cancel)
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
