package pregel

import (
	"bytes"
	"encoding/gob"
)

// GOBCodec implements Codec using Go's gob encoding.
// This codec preserves exact Go types but is Go-only (not cross-language compatible).
//
// ⚠️ SECURITY WARNING: GOB DESERIALIZATION WITH UNTRUSTED DATA
//
// The gob codec can be unsafe when deserializing untrusted data. While gob itself
// does not directly execute code during deserialization, it CAN:
//
//  1. Allocate arbitrary amounts of memory (DoS via memory exhaustion)
//  2. Create deeply nested structures (DoS via stack exhaustion)
//  3. Trigger panics in custom UnmarshalBinary methods
//  4. Cause resource exhaustion through large slice/map allocations
//
// SECURITY GUIDELINES:
//
//	✅ SAFE: Use GOBCodec when all these conditions are met:
//	   - All message producers are trusted Go processes under your control
//	   - Message bus (Redis/etc) is on a private network with access controls
//	   - Network communication is encrypted (TLS) and authenticated
//	   - No external/user-generated data flows into the message bus
//
//	❌ UNSAFE: Do NOT use GOBCodec if:
//	   - Message bus is exposed to untrusted networks
//	   - External services can send messages to your Redis instance
//	   - User-generated content flows through the message bus
//	   - You cannot guarantee message source authenticity
//
//	🛡️ RECOMMENDED: For production distributed systems:
//	   - Use JSONCodec instead (more restrictive, safer with untrusted data)
//	   - Add message signing/authentication (HMAC) to verify source
//	   - Implement message size limits at the transport layer
//	   - Use network segmentation and firewall rules
//	   - Enable Redis AUTH and TLS for all connections
//
// Threat Model Example:
//
//	If an attacker gains write access to your Redis instance, they could inject
//	malicious gob-encoded messages that cause:
//	  - Memory exhaustion (large allocations)
//	  - CPU exhaustion (deeply nested structures)
//	  - Application crashes (panics in deserialization)
//
// Mitigation Strategies:
//
//  1. Network Isolation: Keep message bus on private network
//  2. Authentication: Require Redis AUTH password
//  3. Encryption: Use TLS for all Redis connections
//  4. Message Signing: Add HMAC signatures to detect tampering
//  5. Size Limits: Configure Redis maxmemory-policy
//  6. Monitoring: Alert on unusual message sizes or deserialization errors
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
//   - Security concerns with untrusted data (see warning above)
//
// Use GOBCodec when:
//   - All workers are trusted Go processes under your control
//   - Message bus is on a secure, private network
//   - You need exact type preservation
//   - Performance is critical
//   - You don't need human-readable messages
//
// Use JSONCodec instead when:
//   - You need cross-language compatibility
//   - You cannot guarantee message source authenticity
//   - Message bus might be exposed to untrusted networks
//   - Security is more important than performance
//
// Example (Secure Setup):
//
//	// 1. Register custom types (if needed)
//	gob.Register(MyCustomType{})
//
//	// 2. Use TLS and authentication
//	tlsConfig := &tls.Config{
//	    MinVersion: tls.VersionTLS13,
//	}
//
//	bus, err := redis.NewMessageBus[state.Updates]("localhost:6379", "your-password", 0, &redis.Options{
//	    Codec:     pregel.NewGOBCodec(),
//	    TLSConfig: tlsConfig,
//	    // ... other security options
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 3. Consider adding message signing
//	signer := NewHMACSigner(secretKey)
//	signedBus := NewSignedMessageBus(bus, signer)
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
