# Checkpoint Signing Example

This example demonstrates how to use HMAC-SHA256 checkpoint signing to ensure checkpoint integrity and prevent tampering.

## Overview

Checkpoint signing provides cryptographic verification that checkpoints haven't been tampered with. This is critical for:

- **Security**: Prevent privilege escalation through checkpoint modification
- **Integrity**: Detect unauthorized state manipulation
- **Compliance**: Maintain audit trails with tamper-evident storage

## Features Demonstrated

1. **Basic Signing**: Sign checkpoints on save, verify on load
2. **Tampering Detection**: Automatic detection of modified checkpoints
3. **Key Management**: How different keys produce different signatures
4. **Production Integration**: Using signed checkpoints in graph workflows

## How It Works

### Signing Process

When a checkpoint is saved with signing enabled:

1. A deterministic representation of the checkpoint is created
2. HMAC-SHA256 signature is computed using the signing key
3. Signature is stored with the checkpoint

### Verification Process

When a checkpoint is loaded with signing enabled:

1. Expected signature is recomputed from checkpoint data
2. Stored signature is compared using constant-time comparison
3. Load fails if signatures don't match (tampering detected)

### Signed Fields

The signature covers:
- RunID
- Superstep
- Version
- Timestamp
- State (all keys and values, sorted)
- CompletedNodes (sorted)
- PausedNodes (sorted)

**Not signed**: Messages (too large), Signature field itself (circular)

## Usage

### Basic Setup

```go
// Generate secure signing key (32+ bytes recommended)
signingKey := make([]byte, 32)
rand.Read(signingKey)

// Create checkpointer with signing enabled
checkpointer := checkpoint.NewInMemoryCheckpointer(
    checkpoint.WithSigning(signingKey),
)
```

### Graph Workflow with Signing

```go
compiled, err := workflow.Compile(ctx,
    graph.WithModel(model),
)

result, err := graph.Last(compiled.Run(ctx, messages,
    graph.WithCheckpointer(checkpointer),
    graph.WithRunID("secure-run"),
))
```

### Manual Signature Verification

```go
// Sign checkpoint
signature, err := checkpoint.SignCheckpoint(cp, signingKey)
cp.Signature = signature

// Verify checkpoint
err := checkpoint.VerifyCheckpoint(cp, signingKey)
if err != nil {
    // Tampering detected!
}
```

## Running the Example

```bash
cd examples/checkpoint_signing
go run main.go
```

## Expected Output

```
=== Checkpoint Signing Example ===

1. Basic Checkpoint Signing and Verification
✓ Generated signing key (32 bytes)
✓ Checkpoint saved with signature
✓ Checkpoint loaded and verified (signature: 32 bytes)
  State: map[counter:42 status:running]

2. Tampering Detection
✓ Saved checkpoint with balance: 1000
⚠ Simulating tampering attack
✓ Tampering detected! Error: checkpoint signature verification failed

3. Wrong Key Detection
✓ Saved checkpoint with Key #1
⚠ Attempting to load with Key #2 (wrong key)
✓ Verification would fail: signature mismatch

4. Production Graph Execution with Signed Checkpoints
✓ Generated production signing key
✓ Starting workflow (runID: production-run)
✓ Workflow completed
✓ Checkpoint signed and verified (32 bytes signature)
✓ Manual signature verification successful
```

## Security Considerations

### Key Management

1. **Generate Secure Keys**: Use `crypto/rand` to generate keys (minimum 32 bytes)
2. **Store Securely**: Never commit keys to version control
3. **Rotate Regularly**: Implement key rotation policies
4. **Environment Variables**: Load keys from secure environment variables

```go
signingKey := []byte(os.Getenv("CHECKPOINT_SIGNING_KEY"))
if len(signingKey) < 32 {
    log.Fatal("Invalid signing key: must be at least 32 bytes")
}
```

### Production Best Practices

1. **Use Persistent Storage**: Combine with DynamoDB/SQL checkpointers for durability
2. **Monitor Verification Failures**: Alert on signature mismatches (potential attacks)
3. **Audit Logging**: Log all checkpoint save/load operations
4. **Key Backup**: Maintain secure backups of signing keys

### Threat Model

**Protects Against**:
- Unauthorized checkpoint modification
- Privilege escalation via state tampering
- Replay attacks with old checkpoints (via timestamp signing)

**Does NOT Protect Against**:
- Key theft (if attacker has the key, they can sign checkpoints)
- Deletion attacks (signing doesn't prevent checkpoint deletion)
- Runtime memory tampering (signing only protects stored checkpoints)

## Implementation Details

### HMAC-SHA256

- **Algorithm**: HMAC (Hash-based Message Authentication Code) with SHA-256
- **Output**: 32-byte signature
- **Security**: Provides cryptographic authentication and integrity

### Deterministic Encoding

To ensure consistent signatures across platforms:
- Uses `binary.BigEndian` for integer encoding
- Sorts all map keys and string slices
- Validates timestamps and supersteps are non-negative

### Timing Attack Resistance

Uses `hmac.Equal()` for constant-time signature comparison to prevent timing attacks.

## Related Examples

- [Checkpointing](../checkpointing/) - Basic checkpoint usage without signing
- [Time Travel](../time_travel/) - Loading checkpoints from specific supersteps
- [Input Validation](../input_validation/) - Message validation for DoS prevention

## References

- [SECURITY.md](../../SECURITY.md) - Security considerations and threat model
- [Checkpoint Package](../../pkg/checkpoint/) - API documentation
- [HMAC RFC 2104](https://datatracker.ietf.org/doc/html/rfc2104) - HMAC specification
