package model

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

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
