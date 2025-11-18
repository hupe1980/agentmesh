package pregel

import "encoding/json"

// JSONCodec implements Codec using standard JSON encoding.
// This is the default codec for cross-language compatibility.
//
// Characteristics:
//   - Human-readable format
//   - Cross-language compatible (Python, Node.js, etc.)
//   - Slower than binary formats
//   - Type coercion: numbers become float64, maps lose concrete types
//
// Limitations:
//   - All numeric types are decoded as float64
//   - map[string]int becomes map[string]any with float64 values
//   - Custom types need json.Marshaler/json.Unmarshaler
//
// Use JSONCodec when:
//   - You need cross-language compatibility
//   - You're okay with float64 for all numbers
//   - Human-readable debugging is valuable
type JSONCodec struct{}

// NewJSONCodec creates a new JSON codec.
func NewJSONCodec() *JSONCodec {
	return &JSONCodec{}
}

// Encode serializes v to JSON bytes.
func (c *JSONCodec) Encode(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Decode deserializes JSON bytes into v.
func (c *JSONCodec) Decode(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
