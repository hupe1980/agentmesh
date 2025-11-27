package checkpoint

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

// Encryptor defines the interface for encryption/decryption operations
type Encryptor interface {
	// Encrypt encrypts plaintext and returns ciphertext
	Encrypt(plaintext []byte) ([]byte, error)

	// Decrypt decrypts ciphertext and returns plaintext
	Decrypt(ciphertext []byte) ([]byte, error)

	// Algorithm returns the algorithm identifier
	Algorithm() string
}

// AES256GCMEncryptor implements AES-256-GCM encryption
type AES256GCMEncryptor struct {
	gcm cipher.AEAD
}

// NewAES256GCMEncryptor creates an AES-256-GCM encryptor
func NewAES256GCMEncryptor(key []byte) (*AES256GCMEncryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes for AES-256, got %d bytes", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &AES256GCMEncryptor{gcm: gcm}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM.
// Returns the ciphertext with prepended nonce.
func (e *AES256GCMEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	// Generate random nonce
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and authenticate
	ciphertext := e.gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM.
// Expects the nonce to be prepended to the ciphertext.
func (e *AES256GCMEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := e.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// Algorithm returns the encryption algorithm identifier.
func (e *AES256GCMEncryptor) Algorithm() string {
	return "aes-256-gcm"
}

// EncryptedCheckpointer wraps a base checkpointer with encryption
type EncryptedCheckpointer struct {
	base      Checkpointer
	encryptor Encryptor
}

// NewEncryptedCheckpointer creates an encrypted checkpointer with the given encryptor
func NewEncryptedCheckpointer(base Checkpointer, encryptor Encryptor) (*EncryptedCheckpointer, error) {
	if encryptor == nil {
		return nil, fmt.Errorf("encryptor is required")
	}

	return &EncryptedCheckpointer{
		base:      base,
		encryptor: encryptor,
	}, nil
}

// Save encrypts and saves a checkpoint.
// The checkpoint data is serialized, encrypted using the configured encryptor,
// and stored with the encryption algorithm metadata.
func (ec *EncryptedCheckpointer) Save(ctx context.Context, checkpoint *Checkpoint) error {
	// Serialize checkpoint
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Encrypt data
	encrypted, err := ec.encrypt(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt checkpoint: %w", err)
	}

	// Create wrapper checkpoint with encrypted payload
	encryptedCP := &Checkpoint{
		RunID:     checkpoint.RunID,
		Superstep: checkpoint.Superstep,
		Metadata: map[string]any{
			"encrypted":  true,
			"algorithm":  ec.encryptor.Algorithm(),
			"payload":    base64.StdEncoding.EncodeToString(encrypted),
			"created_at": checkpoint.Metadata["created_at"],
		},
	}

	return ec.base.Save(ctx, encryptedCP)
}

// Load retrieves and decrypts a checkpoint.
// It loads the encrypted checkpoint, verifies the algorithm matches,
// and decrypts the data using the configured encryptor.
func (ec *EncryptedCheckpointer) Load(ctx context.Context, runID string) (*Checkpoint, error) {
	// Load potentially encrypted checkpoint
	encryptedCP, err := ec.base.Load(ctx, runID)
	if err != nil {
		return nil, err
	}

	// Check if encrypted
	encrypted, ok := encryptedCP.Metadata["encrypted"].(bool)
	if !ok || !encrypted {
		// Not encrypted, return as-is (backwards compatibility)
		return encryptedCP, nil
	}

	// Validate algorithm matches
	algorithm, ok := encryptedCP.Metadata["algorithm"].(string)
	if ok && algorithm != ec.encryptor.Algorithm() {
		return nil, fmt.Errorf("checkpoint encrypted with %s but checkpointer configured for %s", algorithm, ec.encryptor.Algorithm())
	}

	// Decode base64 payload
	payloadStr, ok := encryptedCP.Metadata["payload"].(string)
	if !ok {
		return nil, fmt.Errorf("encrypted checkpoint missing payload")
	}

	encryptedData, err := base64.StdEncoding.DecodeString(payloadStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted payload: %w", err)
	}

	// Decrypt data
	data, err := ec.decrypt(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt checkpoint: %w", err)
	}

	// Unmarshal original checkpoint
	checkpoint := &Checkpoint{}
	if err := json.Unmarshal(data, checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return checkpoint, nil
}

// List returns all checkpoints for the given runID by delegating to the base checkpointer.
// Note: The returned list contains metadata only; actual checkpoint data requires Load().
func (ec *EncryptedCheckpointer) List(ctx context.Context, runID string) ([]*Checkpoint, error) {
	return ec.base.List(ctx, runID)
}

// Delete removes all checkpoints for the given runID by delegating to the base checkpointer.
func (ec *EncryptedCheckpointer) Delete(ctx context.Context, runID string) error {
	return ec.base.Delete(ctx, runID)
}

// LoadAtSuperstep retrieves and decrypts a checkpoint at a specific superstep.
// It loads the encrypted checkpoint, verifies the algorithm matches,
// and decrypts the data using the configured encryptor.
func (ec *EncryptedCheckpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*Checkpoint, error) {
	cp, err := ec.base.LoadAtSuperstep(ctx, runID, superstep)
	if err != nil {
		return nil, err
	}

	// Check if encrypted
	encrypted, ok := cp.Metadata["encrypted"].(bool)
	if !ok || !encrypted {
		return cp, nil
	}

	// Validate algorithm matches
	algorithm, ok := cp.Metadata["algorithm"].(string)
	if ok && algorithm != ec.encryptor.Algorithm() {
		return nil, fmt.Errorf("checkpoint encrypted with %s but checkpointer configured for %s", algorithm, ec.encryptor.Algorithm())
	}

	// Decrypt if needed (reuse Load logic)
	payloadStr, ok := cp.Metadata["payload"].(string)
	if !ok {
		return nil, fmt.Errorf("encrypted checkpoint missing payload")
	}

	encryptedData, err := base64.StdEncoding.DecodeString(payloadStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted payload: %w", err)
	}

	data, err := ec.decrypt(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt checkpoint: %w", err)
	}

	checkpoint := &Checkpoint{}
	if err := json.Unmarshal(data, checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return checkpoint, nil
}

// ListPendingApprovals returns all checkpoints with pending approvals by delegating to the base checkpointer.
func (ec *EncryptedCheckpointer) ListPendingApprovals(ctx context.Context) ([]*Checkpoint, error) {
	return ec.base.ListPendingApprovals(ctx)
}

// GetApprovalHistory returns the approval history for a specific run by delegating to the base checkpointer.
func (ec *EncryptedCheckpointer) GetApprovalHistory(ctx context.Context, runID string) ([]ApprovalRecord, error) {
	return ec.base.GetApprovalHistory(ctx, runID)
}

// encrypt encrypts data using the configured encryptor
func (ec *EncryptedCheckpointer) encrypt(plaintext []byte) ([]byte, error) {
	return ec.encryptor.Encrypt(plaintext)
}

// decrypt decrypts data using the configured encryptor
func (ec *EncryptedCheckpointer) decrypt(ciphertext []byte) ([]byte, error) {
	return ec.encryptor.Decrypt(ciphertext)
}

// DeriveKeyFromPassword derives a 32-byte key from a password
// Use this to generate keys from user-provided passwords
func DeriveKeyFromPassword(password string, salt []byte) []byte {
	if salt == nil {
		salt = []byte("agentmesh-checkpoint-salt") // Default salt
	}
	return pbkdf2.Key([]byte(password), salt, 100000, 32, sha256.New)
}

// MultiKeyCheckpointer supports key rotation by trying multiple keys
type MultiKeyCheckpointer struct {
	base       Checkpointer
	currentKey []byte
	oldKeys    [][]byte // Previous keys for decryption
}

// NewMultiKeyCheckpointer creates a checkpointer that supports key rotation
func NewMultiKeyCheckpointer(base Checkpointer, currentKey []byte, oldKeys ...[]byte) (*MultiKeyCheckpointer, error) {
	if len(currentKey) != 32 {
		return nil, fmt.Errorf("current key must be 32 bytes for AES-256")
	}

	for i, key := range oldKeys {
		if len(key) != 32 {
			return nil, fmt.Errorf("old key %d must be 32 bytes for AES-256", i)
		}
	}

	return &MultiKeyCheckpointer{
		base:       base,
		currentKey: currentKey,
		oldKeys:    oldKeys,
	}, nil
}

// Save encrypts and saves a checkpoint using the primary (current) key.
// The checkpoint is always encrypted with the first key in the keys list.
func (mkc *MultiKeyCheckpointer) Save(ctx context.Context, checkpoint *Checkpoint) error {
	// Always save with current key
	encryptor, err := NewAES256GCMEncryptor(mkc.currentKey)
	if err != nil {
		return err
	}
	ec, err := NewEncryptedCheckpointer(mkc.base, encryptor)
	if err != nil {
		return err
	}
	return ec.Save(ctx, checkpoint)
}

// Load attempts to decrypt a checkpoint by trying each key in order.
// It first tries the current key, then falls back to old keys.
// Returns the first successfully decrypted checkpoint or an error if all keys fail.
func (mkc *MultiKeyCheckpointer) Load(ctx context.Context, runID string) (*Checkpoint, error) {
	// Try current key first
	cp, err := mkc.tryDecrypt(ctx, runID, mkc.currentKey)
	if err == nil {
		return cp, nil
	}

	// Try old keys
	for i, oldKey := range mkc.oldKeys {
		cp, err := mkc.tryDecrypt(ctx, runID, oldKey)
		if err == nil {
			// Re-encrypt with current key on next save
			return cp, nil
		}
		// Log that we tried this key
		_ = i // suppress unused variable
	}

	return nil, fmt.Errorf("failed to decrypt checkpoint with any of the %d keys", len(mkc.oldKeys)+1)
}

// List returns all checkpoints for the given runID by delegating to the base checkpointer.
// Note: The returned list contains metadata only; actual checkpoint data requires Load().
func (mkc *MultiKeyCheckpointer) List(ctx context.Context, runID string) ([]*Checkpoint, error) {
	return mkc.base.List(ctx, runID)
}

// Delete removes all checkpoints for the given runID by delegating to the base checkpointer.
func (mkc *MultiKeyCheckpointer) Delete(ctx context.Context, runID string) error {
	return mkc.base.Delete(ctx, runID)
}

// LoadAtSuperstep attempts to decrypt a checkpoint at a specific superstep by trying each key in order.
// Returns the first successfully decrypted checkpoint or an error if all keys fail.
func (mkc *MultiKeyCheckpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*Checkpoint, error) {
	// Try current key first
	cp, err := mkc.tryDecryptAtSuperstep(ctx, runID, superstep, mkc.currentKey)
	if err == nil {
		return cp, nil
	}

	// Try old keys
	for _, oldKey := range mkc.oldKeys {
		cp, err := mkc.tryDecryptAtSuperstep(ctx, runID, superstep, oldKey)
		if err == nil {
			return cp, nil
		}
	}

	return nil, fmt.Errorf("failed to decrypt checkpoint at superstep %d with any key: %w", superstep, err)
}

func (mkc *MultiKeyCheckpointer) tryDecrypt(ctx context.Context, runID string, key []byte) (*Checkpoint, error) {
	encryptor, err := NewAES256GCMEncryptor(key)
	if err != nil {
		return nil, err
	}
	ec, err := NewEncryptedCheckpointer(mkc.base, encryptor)
	if err != nil {
		return nil, err
	}
	return ec.Load(ctx, runID)
}

func (mkc *MultiKeyCheckpointer) tryDecryptAtSuperstep(ctx context.Context, runID string, superstep int64, key []byte) (*Checkpoint, error) {
	encryptor, err := NewAES256GCMEncryptor(key)
	if err != nil {
		return nil, err
	}
	ec, err := NewEncryptedCheckpointer(mkc.base, encryptor)
	if err != nil {
		return nil, err
	}
	return ec.LoadAtSuperstep(ctx, runID, superstep)
}

// ListPendingApprovals returns all checkpoints with pending approvals by delegating to the base checkpointer.
func (mkc *MultiKeyCheckpointer) ListPendingApprovals(ctx context.Context) ([]*Checkpoint, error) {
	return mkc.base.ListPendingApprovals(ctx)
}

// GetApprovalHistory returns the approval history for a specific run by delegating to the base checkpointer.
func (mkc *MultiKeyCheckpointer) GetApprovalHistory(ctx context.Context, runID string) ([]ApprovalRecord, error) {
	return mkc.base.GetApprovalHistory(ctx, runID)
}
