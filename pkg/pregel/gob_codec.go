package pregel

import (
	"bytes"
	"encoding/gob"
)

// GOBCodec implements Codec using Go's gob encoding.
// This codec preserves exact Go types but is Go-only (not cross-language compatible).
//
// Characteristics:
//   - Binary format (smaller than JSON)
//   - Faster than JSON
//   - Preserves exact types: int stays int, not float64
//   - Go-only: Cannot be used with Python/Node.js workers
//   - Requires type registration for interfaces/custom types
//
// Advantages over JSON:
//   - No type coercion: int, int64, float32, float64 are distinct
//   - map[string]int stays map[string]int
//   - Better performance for complex structures
//
// Limitations:
//   - Not human-readable
//   - Go ecosystem only
//   - Complex types need gob.Register() calls
//
// Use GOBCodec when:
//   - All workers are Go processes
//   - You need exact type preservation
//   - Performance is critical
//   - You don't need human-readable messages
//
// Example:
//
//	// Register custom types (if needed)
//	gob.Register(MyCustomType{})
//
//	bus := redis.NewMessageBus[state.Updates]("localhost:6379", "", 0, &redis.Options{
//	    Codec: pregel.NewGOBCodec(),
//	})
type GOBCodec struct{}

// NewGOBCodec creates a new GOB codec.
func NewGOBCodec() *GOBCodec {
	return &GOBCodec{}
}

// Encode serializes v to GOB bytes.
func (c *GOBCodec) Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode deserializes GOB bytes into v.
func (c *GOBCodec) Decode(data []byte, v any) error {
	buf := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buf)
	return decoder.Decode(v)
}
