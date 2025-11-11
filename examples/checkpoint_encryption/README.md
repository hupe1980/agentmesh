# Checkpoint Encryption Example

Demonstrates how to encrypt checkpoints to protect sensitive data at rest using AES-256-GCM encryption.

## Features Demonstrated

- Basic AES-256-GCM encryption
- Key rotation with `MultiKeyCheckpointer`
- Password-derived encryption keys
- Backwards compatibility with unencrypted checkpoints

## Running the Example

```bash
go run main.go
```

With custom password:
```bash
CHECKPOINT_PASSWORD="your-secure-password" go run main.go
```

## Output

```
=== Checkpoint Encryption Example ===

1. Basic Encryption with AES-256-GCM
Generated encryption key: abc...xyz
  Processing sensitive data...
  Node: secure_node completed
  ✓ Checkpoint is encrypted (AES-256-GCM)
  ✓ Sensitive data is protected at rest

2. Key Rotation Example
  Step 1: Saving checkpoint with OLD key...
  ✓ Checkpoint saved with old key
  Step 2: Rotating to NEW key (with old key as fallback)...
  Step 3: Loading checkpoint (automatically tries keys)...
  ✓ Successfully loaded checkpoint encrypted with old key
  Step 4: Re-saving checkpoint with NEW key...
  ✓ Checkpoint now encrypted with new key
  ✓ Key rotation complete!

3. Password-Derived Encryption Key
  Password: demo-password-12345
  Derived key (first 16 bytes): a1b2c3d4...
  ✓ Checkpoint encrypted using password-derived key
  ✓ Successfully loaded: password-protected data
  ✓ Wrong password correctly rejected
```

## What This Demonstrates

### 1. Basic Encryption

Checkpoints containing sensitive data (credit cards, SSNs, passwords) are encrypted before being stored:

```go
encryptedCheckpointer, _ := checkpoint.NewEncryptedCheckpointer(
    baseCheckpointer,
    checkpoint.EncryptionConfig{
        Key: key, // 32-byte key for AES-256
    },
)
```

### 2. Key Rotation

Seamlessly rotate encryption keys without downtime:

```go
multiKeyCheckpointer, _ := checkpoint.NewMultiKeyCheckpointer(
    baseCheckpointer,
    newKey,    // Use for new checkpoints
    oldKey,    // Can still read old checkpoints
)
```

The checkpointer automatically:
- Tries the new key first
- Falls back to old keys if needed
- Re-encrypts with new key on next save

### 3. Password-Based Encryption

Derive encryption keys from passwords using PBKDF2:

```go
key := checkpoint.DeriveKeyFromPassword(password, salt)
```

**Security Notes:**
- Uses PBKDF2 with 100,000 iterations
- Requires strong passwords (12+ characters recommended)
- Salt should be unique per application
- Store salt securely (can be public, but must be consistent)

## Production Usage

### Loading Key from Environment

```go
import (
    "encoding/base64"
    "os"
)

func getEncryptionKey() ([]byte, error) {
    keyStr := os.Getenv("CHECKPOINT_ENCRYPTION_KEY")
    if keyStr == "" {
        return nil, fmt.Errorf("CHECKPOINT_ENCRYPTION_KEY not set")
    }
    
    return base64.StdEncoding.DecodeString(keyStr)
}
```

### Generating Secure Keys

```bash
# Generate 32-byte key
openssl rand -base64 32

# Or in Go:
key := make([]byte, 32)
rand.Read(key)
fmt.Println(base64.StdEncoding.EncodeToString(key))
```

### Key Management Best Practices

1. **Never hardcode keys** - Always load from environment or secret manager
2. **Use 32-byte keys** - Required for AES-256
3. **Rotate regularly** - Use `MultiKeyCheckpointer` for seamless rotation
4. **Secure storage** - Use AWS KMS, Vault, or other secret management
5. **Backup keys** - Losing keys means losing all encrypted checkpoints

## See Also

- [AWS KMS Integration](../checkpoint_encryption_kms/) - Cloud key management
- [Vault Integration](../checkpoint_encryption_vault/) - Enterprise secret management
- [SECURITY.md](../../SECURITY.md) - Full security hardening guide
