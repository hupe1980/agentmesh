package pregel

// Codec defines the interface for serializing and deserializing pregel messages.
// Implementations can provide different serialization formats (JSON, GOB, MessagePack, etc.)
// to balance between performance, type preservation, and cross-language compatibility.
//
// Built-in Implementations:
//
// JSONCodec (default):
//   - Cross-language compatible
//   - Human-readable
//   - Numbers become float64
//   - Best for: multi-language systems, debugging
//
// GOBCodec:
//   - Go-only
//   - Preserves exact types (int stays int)
//   - Faster and smaller than JSON
//   - Best for: pure-Go distributed systems, type-sensitive state
//
// Custom Codecs:
//   - Implement this interface for MessagePack, Protobuf, etc.
//   - Must be thread-safe
//   - Should handle all types in state.Updates (map[string]any)
type Codec interface {
	// Encode serializes a message to bytes.
	// Must be thread-safe.
	Encode(v any) ([]byte, error)

	// Decode deserializes bytes into a message.
	// Must be thread-safe.
	Decode(data []byte, v any) error
}
