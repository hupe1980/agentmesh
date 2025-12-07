// Package checkpoint provides sentinel and structured errors for the checkpoint package.
package checkpoint

import (
	"errors"
	"fmt"
)

// =============================================================================
// Sentinel Errors
// =============================================================================

var (
	// ErrNilCheckpoint is returned when a checkpoint is nil.
	ErrNilCheckpoint = errors.New("checkpoint: checkpoint is nil")

	// ErrEmptyRunID is returned when a run ID is empty.
	ErrEmptyRunID = errors.New("checkpoint: runID is empty")

	// ErrCiphertextTooShort is returned when ciphertext is too short.
	ErrCiphertextTooShort = errors.New("checkpoint: ciphertext too short")

	// ErrEncryptorRequired is returned when an encryptor is required.
	ErrEncryptorRequired = errors.New("checkpoint: encryptor is required")

	// ErrMissingPayload is returned when encrypted checkpoint is missing payload.
	ErrMissingPayload = errors.New("checkpoint: encrypted checkpoint missing payload")

	// ErrInvalidKeySize is returned when the key size is invalid.
	ErrInvalidKeySize = errors.New("checkpoint: current key must be 32 bytes for AES-256")

	// ErrDatabaseRequired is returned when database connection is required.
	ErrDatabaseRequired = errors.New("checkpoint/sql: database connection is required")

	// ErrNotImplemented is returned when a feature is not implemented.
	ErrNotImplemented = errors.New("checkpoint: not yet implemented")

	// ErrKMSKeyIDRequired is returned when KMS key ID is required.
	ErrKMSKeyIDRequired = errors.New("checkpoint/awskms: key ID is required")

	// ErrKMSClientRequired is returned when KMS client is required.
	ErrKMSClientRequired = errors.New("checkpoint/awskms: client is required")

	// ErrVaultClientRequired is returned when Vault client is required.
	ErrVaultClientRequired = errors.New("checkpoint/vault: client is required")

	// ErrVaultKeyNameRequired is returned when Vault key name is required.
	ErrVaultKeyNameRequired = errors.New("checkpoint/vault: key name is required")

	// ErrVaultMissingCiphertext is returned when Vault response is missing ciphertext.
	ErrVaultMissingCiphertext = errors.New("checkpoint/vault: encrypted checkpoint missing ciphertext")

	// ErrVaultMissingPlaintext is returned when Vault response is missing plaintext.
	ErrVaultMissingPlaintext = errors.New("checkpoint/vault: decryption response missing plaintext")
)

// =============================================================================
// Structured Errors
// =============================================================================

// RunNotFoundError represents an error when a run ID is not found.
// Use errors.As to extract the RunID field for programmatic handling.
type RunNotFoundError struct {
	// RunID is the run ID that was not found.
	RunID string
}

// Error implements the error interface.
func (e *RunNotFoundError) Error() string {
	return fmt.Sprintf("checkpoint: no checkpoints found for runID %q", e.RunID)
}

// Is enables comparison with sentinel errors.
func (e *RunNotFoundError) Is(target error) bool {
	_, ok := target.(*RunNotFoundError)
	return ok
}
